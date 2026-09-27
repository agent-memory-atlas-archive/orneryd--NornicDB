#!/usr/bin/env python3
"""Stream a Graphify code graph into local NornicDB over Neo4j Bolt."""

import argparse
import os
import re
from collections import defaultdict
from pathlib import Path

import ijson
from neo4j import GraphDatabase


def graph_items(path, key):
    with path.open("rb") as graph_file:
        yield from ijson.items(graph_file, f"{key}.item", use_float=True)


def scalar_properties(data, *, edge=False):
    return {
        key: value for key, value in data.items()
        if isinstance(value, (str, int, float, bool))
        and not key.startswith("_")
        and (not edge or key not in ("source", "target"))
    }


def node_label(data):
    return re.sub(r"[^A-Za-z0-9_]", "", data.get("file_type", "Entity").capitalize()) or "Entity"


def relationship_type(data):
    return re.sub(r"[^A-Z0-9_]", "_", data.get("relation", "RELATED_TO").upper().replace(" ", "_").replace("-", "_")) or "RELATED_TO"


def import_graph(graph_path, uri, user, password, batch_size):
    labels = {}
    node_batches = defaultdict(list)
    edge_batches = defaultdict(list)
    node_count = edge_count = 0

    with GraphDatabase.driver(uri, auth=(user, password)) as driver:
        with driver.session() as session:
            def write_nodes(label, rows):
                session.run(
                    f"UNWIND $rows AS row MERGE (n:{label} {{id: row.id}}) SET n += row.props",
                    rows=rows,
                ).consume()
                rows.clear()

            def write_edges(source_label, target_label, relation, rows):
                session.run(
                    f"UNWIND $rows AS row "
                    f"MATCH (a:{source_label} {{id: row.src}}), (b:{target_label} {{id: row.tgt}}) "
                    f"MERGE (a)-[r:{relation}]->(b) SET r += row.props",
                    rows=rows,
                ).consume()
                rows.clear()

            for data in graph_items(graph_path, "nodes"):
                label = node_label(data)
                node_id = data["id"]
                labels[node_id] = label
                rows = node_batches[label]
                rows.append({"id": node_id, "props": {**scalar_properties(data), "id": node_id}})
                node_count += 1
                if len(rows) >= batch_size:
                    write_nodes(label, rows)

            for data in graph_items(graph_path, "edges"):
                for node_id in (data["source"], data["target"]):
                    if node_id not in labels:
                        labels[node_id] = "Entity"
                        rows = node_batches["Entity"]
                        rows.append({"id": node_id, "props": {"id": node_id}})
                        node_count += 1
                        if len(rows) >= batch_size:
                            write_nodes("Entity", rows)

            for label, rows in node_batches.items():
                if rows:
                    write_nodes(label, rows)
                session.run(
                    f"CREATE INDEX graphify_{label.lower()}_id IF NOT EXISTS FOR (n:{label}) ON (n.id)"
                ).consume()

            print(f"Imported {node_count} nodes", flush=True)

            for data in graph_items(graph_path, "edges"):
                source = data["source"]
                target = data["target"]
                source_label, target_label = labels[source], labels[target]
                relation = relationship_type(data)
                rows = edge_batches[(source_label, target_label, relation)]
                rows.append({"src": source, "tgt": target, "props": scalar_properties(data, edge=True)})
                edge_count += 1
                if len(rows) >= batch_size:
                    write_edges(source_label, target_label, relation, rows)
                if edge_count % 10000 == 0:
                    print(f"Imported {edge_count} edges", flush=True)

            for (source_label, target_label, relation), rows in edge_batches.items():
                if rows:
                    write_edges(source_label, target_label, relation, rows)

    print(f"Imported {node_count} nodes and {edge_count} edges into {uri}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--graph", type=Path, default=Path("graphify-out/graph.json"))
    parser.add_argument("--uri", default="bolt://127.0.0.1:7687")
    parser.add_argument("--user", default="admin")
    parser.add_argument("--batch-size", type=int, default=1000)
    args = parser.parse_args()
    if args.batch_size < 1:
        parser.error("--batch-size must be positive")
    if not args.graph.is_file():
        parser.error(f"graph not found: {args.graph}")
    password = os.environ.get("NEO4J_PASSWORD")
    if not password:
        parser.error("set NEO4J_PASSWORD in the environment")
    import_graph(args.graph, args.uri, args.user, password, args.batch_size)


if __name__ == "__main__":
    main()