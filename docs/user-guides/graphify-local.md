# Local Graphify Code Graph

Graphify indexes the repository with local AST parsing and stores the queryable
graph in `graphify-out/` (ignored by Git). The graph is also imported into the
local NornicDB server through its Neo4j Bolt endpoint at `127.0.0.1:7687`.
The VS Code workspace has an ignored `.vscode/mcp.json` pointing Copilot Chat
at the local Graphify graph; it does not require database credentials.

Install the official Graphify tool with its optional Neo4j, streaming JSON,
and MCP dependencies:

```sh
brew install uv
uv tool install --with neo4j --with ijson 'graphifyy[mcp]'
```

From the repository root, build and query the code graph without an LLM or
external service:

```sh
"$(uv tool dir --bin)/graphify" extract . --code-only --no-cluster
"$(uv tool dir --bin)/graphify" query "which query routers call the same helpers?"
"$(uv tool dir --bin)/graphify" path "StorageExecutor" "Execute"
```

To refresh the graph after changing code, run `graphify update . --no-cluster`
(using `"$(uv tool dir --bin)/graphify"` if it is not on `PATH`). Then push the
updated graph to NornicDB. Set `NEO4J_PASSWORD` in the terminal without writing
it to this repository; the import is idempotent and accepts `--batch-size`.

```sh
NEO4J_PASSWORD=your-local-password "$(uv tool dir)/graphifyy/bin/python" \
  scripts/graphify_local.py --graph graphify-out/graph.json
```

The importer matches Graphify's node IDs, labels, relationship types, and scalar
properties, including implicit edge endpoints. It groups writes into bounded
`UNWIND` batches because Graphify's stock Neo4j exporter sends one Bolt query
per node and edge. Re-running `MERGE` updates existing records. Only code is
indexed: docs and PDFs are skipped, so no LLM/API key is required.