## Local Graphify on NornicDB

- [x] Install Graphify with Neo4j driver, streaming JSON and MCP extras in an isolated uv tool environment.
- [x] Verify the local NornicDB Bolt endpoint and Graphify's stock Neo4j push on a scoped code graph.
- [x] Extract the full repository code graph without an LLM and import it using bounded, idempotent batches.
- [x] Cover implicit Graphify endpoint nodes and batched upserts with isolated regression tests.
- [x] Verify persisted relationship counts and workspace MCP queries, then document refresh commands.