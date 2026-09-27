import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from scripts.graphify_local import import_graph, main, node_label, relationship_type, scalar_properties


class RecordingSession:
    def __init__(self):
        self.queries = []

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False

    def run(self, query, **params):
        self.queries.append((query, json.loads(json.dumps(params))))
        return self

    def consume(self):
        return None


class RecordingDriver:
    def __init__(self):
        self.connection = RecordingSession()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False

    def session(self):
        return self.connection


class GraphifyLocalTest(unittest.TestCase):
    def test_graphify_labels_and_scalar_properties(self):
        self.assertEqual(node_label({"file_type": "source-file"}), "Sourcefile")
        self.assertEqual(relationship_type({"relation": "calls-to"}), "CALLS_TO")
        self.assertEqual(scalar_properties({"name": "x", "weight": 1.25, "_origin": "ast", "metadata": {}}),
                         {"name": "x", "weight": 1.25})
        self.assertEqual(scalar_properties({"source": "a", "target": "b", "relation": "calls"}, edge=True),
                 {"relation": "calls"})

    def test_import_includes_implicit_endpoints_and_batches(self):
        graph = {
            "nodes": [{"id": "source", "file_type": "code", "label": "Source", "_origin": "ast"}],
            "edges": [{"source": "source", "target": "implicit", "relation": "calls", "confidence": "EXTRACTED"}],
        }
        driver = RecordingDriver()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "graph.json"
            path.write_text(json.dumps(graph), encoding="utf-8")
            with patch("scripts.graphify_local.GraphDatabase.driver", return_value=driver):
                import_graph(path, "bolt://127.0.0.1:7687", "admin", "test", 1)

        queries = driver.connection.queries
        self.assertIn(("UNWIND $rows AS row MERGE (n:Code {id: row.id}) SET n += row.props",
                       {"rows": [{"id": "source", "props": {"id": "source", "file_type": "code", "label": "Source"}}]}), queries)
        self.assertIn(("UNWIND $rows AS row MERGE (n:Entity {id: row.id}) SET n += row.props",
                       {"rows": [{"id": "implicit", "props": {"id": "implicit"}}]}), queries)
        self.assertIn(("UNWIND $rows AS row MATCH (a:Code {id: row.src}), (b:Entity {id: row.tgt}) "
                       "MERGE (a)-[r:CALLS]->(b) SET r += row.props",
                       {"rows": [{"src": "source", "tgt": "implicit", "props": {
                           "relation": "calls", "confidence": "EXTRACTED"}}]}), queries)

    def test_import_flushes_full_and_partial_batches(self):
        graph = {
            "nodes": [{"id": node_id, "file_type": "code"} for node_id in ("a", "b", "c")],
            "edges": [
                {"source": "a", "target": "new1", "relation": "calls"},
                {"source": "b", "target": "new2", "relation": "calls"},
                {"source": "c", "target": "new1", "relation": "imports"},
            ],
        }
        driver = RecordingDriver()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "graph.json"
            path.write_text(json.dumps(graph), encoding="utf-8")
            with patch("scripts.graphify_local.GraphDatabase.driver", return_value=driver):
                import_graph(path, "bolt://127.0.0.1:7687", "admin", "test", 2)

        queries = driver.connection.queries
        self.assertEqual([len(params["rows"]) for query, params in queries if "MERGE (n:Code" in query], [2, 1])
        self.assertEqual([len(params["rows"]) for query, params in queries if "MERGE (n:Entity" in query], [2])
        self.assertEqual([len(params["rows"]) for query, params in queries if "[r:CALLS]" in query], [2])
        self.assertEqual([len(params["rows"]) for query, params in queries if "[r:IMPORTS]" in query], [1])
        self.assertEqual(sum("CREATE INDEX graphify_" in query for query, _ in queries), 2)

    def test_import_reports_large_progress(self):
        driver = RecordingDriver()

        def items(_path, key):
            if key == "nodes":
                return iter([{"id": "source", "file_type": "code"}])
            return ({"source": "source", "target": "source", "relation": "calls"} for _ in range(10000))

        with patch("scripts.graphify_local.GraphDatabase.driver", return_value=driver), \
             patch("scripts.graphify_local.graph_items", side_effect=items), \
             patch("builtins.print") as output:
            import_graph(Path("unused"), "bolt://127.0.0.1:7687", "admin", "test", 250)

        output.assert_any_call("Imported 10000 edges", flush=True)
        self.assertEqual(sum(len(params["rows"]) for query, params in driver.connection.queries
                             if "[r:CALLS]" in query), 10000)

    def test_cli_rejects_invalid_inputs_and_forwards_options(self):
        with tempfile.TemporaryDirectory() as directory:
            graph = Path(directory) / "graph.json"
            graph.write_text('{"nodes":[],"edges":[]}', encoding="utf-8")
            with patch("sys.argv", ["graphify_local.py", "--batch-size", "0"]), \
                 self.assertRaises(SystemExit):
                main()
            with patch("sys.argv", ["graphify_local.py", "--graph", str(graph) + ".missing"]), \
                 self.assertRaises(SystemExit):
                main()
            with patch("sys.argv", ["graphify_local.py", "--graph", str(graph)]), \
                 patch.dict("os.environ", {"NEO4J_PASSWORD": ""}), \
                 self.assertRaises(SystemExit):
                main()
            with patch("sys.argv", ["graphify_local.py", "--graph", str(graph), "--batch-size", "2"]), \
                 patch.dict("os.environ", {"NEO4J_PASSWORD": "test"}), \
                 patch("scripts.graphify_local.import_graph") as importer:
                main()
                importer.assert_called_once_with(graph, "bolt://127.0.0.1:7687", "admin", "test", 2)


if __name__ == "__main__":
    unittest.main()