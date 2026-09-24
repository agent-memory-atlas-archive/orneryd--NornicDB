## 1. Establish TCK and issue baseline

- [x] 1.1 Pin official TCK revision/checksum/license and record the complete feature/scenario/step inventory.
- [x] 1.2 Implement Go step bindings, typed result/error comparison and observable side-effect checks with negative-control tests.
- [x] 1.3 Run fresh-fixture autocommit and explicit-transaction scenarios over the production Bolt server; consume results and verify transaction completion.
- [x] 1.4 Pin a Neo4j patch image/digest and validate the runner and fixed differential corpus.
- [ ] 1.5 Import every reproduction/variant for the 35 scoped issues, including #452's comment; record exact TCK matches or local-only coverage.
	- Progress: issue-specific local regressions now cover #447's UNWIND/OPTIONAL MATCH result/null matrix, #449's hidden first/secondary sort keys with LIMIT, #450's property/alias collision and controls, #451's ordered WITH aggregation and control, #452's bare-UNWIND ordering/collect cases plus the later comment case, #453's list-comprehension filter/projection variants, #454's division/modulo/power/type cases including WITH, #455's relationship MERGE ON CREATE/ON MATCH persistence/readback, #456's REMOVE/label SET with count and property readback, #457's correlated CALL outer-company scope, #458's date/duration component access through WITH, #459's hidden `toLower` ORDER BY with LIMIT, #460's reduce after WITH with the standalone control, #461's single-autocommit relationship visibility through pattern reads, MATCH SET, and persistent reopen, #462's relationship property arithmetic/string expressions with readback, #463's aggregate arithmetic after AVG, #464's aggregate alias through WITH ORDER BY, #465's startNode/endNode property access with a WITH-bound control, #466's selected/computed/all-property map projections, #467's nested WITH-map access, keys, and parameter control, #468's inline collected-list slice/WITH-alias slice/subscript, #469's OPTIONAL MATCH collection size and empty-list row, #471's round precision plus ceil/floor/round float result types, #474's expression-valued `SET +=` map plus direct property-assignment control, #475's stored-list projection/subscript/head reads and predicates, #476's persisted datetime comparison vs string control and ordering, #477's lexical string min/max with numeric control, #478's list-subscript inline MATCH/MERGE with map-row control, and #479's SET followed by `RETURN sum()` with repeated updates and stored readback. Existing `TestSetRoutesConverge` covers #470 null-removal behavior across Memory/server stacks and autocommit/explicit transactions. The available local cases are verified; exact upstream TCK mappings are recorded in the machine-readable ledger `testing/cypher/tck/testdata/issues.json`, self-validated by `TestIssuesLedger_*` (matrix completeness, reproduction-ID existence in `pkg/cypher`/`pkg/storage`, and scenario-reference existence in the vendored corpus); families without an attached scenario remain "family" status until exact IDs are attached.
	- Additional verified local case: #480 chained `SET a:Extra:Hot` matches the comma form and is idempotent in autocommit and explicit transactions (`TestGH480_ChainedLabelSetMatchesCommaForm`).
	- Additional verified local case: #481 `OPTIONAL MATCH ... count(r)` groups per outer `x` (zero-count row for misses) in both modes, with non-aggregated control and pattern-comprehension workaround agreement (`TestGH481_OptionalMatchCountGroupsPerOuterRow`).
	- Exact TCK matches verified against the vendored corpus (revision `370fe27f`): #470 → `clauses/set/Set2.feature` [1][2][3] (SET property to null removes it); #477 → `expressions/aggregation/Aggregation2.feature` [7][8] (min/max over strings); #464 → `clauses/return-orderby/ReturnOrderBy6.feature` [2] (returned alias inside an aggregation ORDER BY item); #450 → `clauses/with-orderBy/WithOrderBy4.feature` [7] (alias shadows an existing variable); #481 → `clauses/match/Match8.feature` [2] (counting rows after MATCH, MERGE, OPTIONAL MATCH). Local-only is recorded with reasons where the corpus demonstrably lacks the shape: no `round(` anywhere (Neo4j-extension #471), no CALL {} subquery (Neo4j-extension #457), no map projections (Neo4j-extension #466), no startNode/endNode property access (#465), plus storage/concurrency/perf invariants (#448/#461/#487).
- [ ] 1.6 Check in exact known failures; fail on new/changed failures, missing scenarios, unexpected passes and harness errors.
- [ ] 1.7 Add required CI workflow, artifacts and runnable make targets; configure required-check enforcement during rollout.
		- Progress: the conformance workflow triggers on `pkg/**`, runs both `make cypher-conformance` and `make cypher-differential`, and uploads diagnostics with `if: always()`. Required-check enforcement is repository branch protection and remains to be verified.
- [ ] 1.8 Record test/coverage/benchmark baseline and regenerate the divergence candidate ledger with reproducible inputs.
	- Progress: `./scripts/benchmark_northwind_vs_neo4j.sh` completed 2026-09-24 with `GRAPH_ONLY=1`, deterministic seed 42, 48k products, 48k orders, and 10 measured iterations after 2 warmups; all seed counts and four query fingerprints matched. The generated comparison is at `scripts/benchmark_reports/20260924_111115/comparison.md` (the report directory is gitignored). Test/coverage baselines and the divergence-candidate ledger still need tracked, reproducible records.

## 2. Share execution context and lexical contracts

- [ ] 2.1 Extract common preparation while retaining public/internal cache, limits and transaction differences.
		- Progress: top-level and internal execution share duplicate RETURN column validation (`TestExecuteInternal_RejectsDuplicateReturnColumns`). Internal fragments still need inherited scope-aware preparation; applying the full top-level semantic validator rejects valid correlated subqueries.
- [ ] 2.2 Introduce bound scope, inherited parameters, source spans and cancellation propagation without text substitution.
- [ ] 2.3 Converge quote/comment/bracket-aware scanning and prove complete shape/fragment consumption.
- [ ] 2.4 Add typed dispatch outcomes, effect-boundary tests and unresolved-expression instrumentation.

## 3. Repair mutations and storage visibility

- [x] 3.1 Reproduce then fix #462/#474 through scoped assignment and recursively evaluated map values; enforce rollback on evaluation failure.
- [x] 3.2 Reproduce then fix #455/#456/#470/#480; verify persisted graph, MERGE branches, null removal and label updates.
- [ ] 3.3 Build storage contract tests across Memory, Badger, production wrappers and transaction/multidb views.
- [ ] 3.4 Reproduce then fix #461 and #448; verify atomic publication, embedding flush visibility, reopen and any existing-data repair needs.
	- #461 progress: `TestGH461_AutocommitCreatedRelationshipVisibleAcrossTransactionsAndReopen` verifies one auto-commit CREATE containing nodes+edge, six snapshot pattern forms, subsequent implicit MATCH SET/readback, and persistent reopen. The legacy existing-record repair assessment remains open.
	- #448 progress: root cause was `UpdateNodeEmbedding` staging the write-back in `nodeCache` without maintaining the cache-side label index, so `GetNodesByLabel`/`NodeCountByLabel`/`StreamNodesByLabelProjected` skipped engine label-index rows shadowed by the cache — whole in-flight batches vanished from scans until flush. Fix: embedding write-backs now maintain the label index via `syncNodeLabelIndexForEmbeddingLocked` (idempotent label add when labels are unchanged, full re-sync only on label divergence). Regression `TestGH448_StagedEmbeddingWritebackKeepsLabelScanComplete` covers 64 committed nodes with one staged 32-node batch: complete scans/counts/projected reads pre-flush, staged-embedding visibility, and post-flush persistence. New `BenchmarkAsyncEngine_UpdateNodeEmbedding` measured the staged path: full-sync implementation 2160 ns/op 15 allocs → targeted helper 617 ns/op 10 allocs (3.5x faster) on M2 Max; profile confirms no bucket-scan frames remain on this path.
- [ ] 3.5 Normalize typed property access and capability forwarding; cover #475's storage leg and individual/batch mutation equivalence.
- [ ] 3.6 Verify own writes, stable snapshots, rollback, authorization, cache isolation and cancellation.

## 4. Connect ANTLR fallback

- [ ] 4.1 Audit grammar/AST representation and implement rule adapters for first RETURN/MATCH/WHERE/UNWIND/WITH/mutation slices.
- [ ] 4.2 Execute fallback through shared operations without reentering the legacy text dispatcher.
- [ ] 4.3 Add forced route selection for tests and extend the exact baseline with fallback results.
- [ ] 4.4 Prove effect-free misses fall back and runtime failures never replay statements.

## 5. Converge expressions

- [ ] 5.1 Fold computed-row and predicate evaluators into shared typed semantics; retain measured compiled fast leaves.
- [ ] 5.2 Fix list comprehension, numeric, reduce, postfix access, map and temporal issue families in all expression positions.
- [ ] 5.3 Propagate typed errors through all migrated callers; remove expression-text and implicit-null error fallthrough.
- [ ] 5.4 Verify CASE/COALESCE short-circuiting, null/undefined distinction, scope and parameter handling.

## 6. Converge composition

- [ ] 6.1 Fix aggregate discovery/grouping, projection aliases, ordered collections and full sort-key evaluation.
- [ ] 6.2 Fix UNWIND/OPTIONAL MATCH cardinality and correlated CALL; migrate UNION/FOREACH to bound child contexts.
		- Progress: the reported FOREACH composed-write regressions are covered by `TestForeach_ComposedWritesRetainBindings` and `TestForeach_ComposedMutationShapes`; migrating FOREACH/UNION away from query-text re-entry remains open.
- [ ] 6.3 Complete #447/#449–#452/#457/#459/#463/#464/#468/#469/#478/#479/#481 regressions and applicable TCK cases.
- [ ] 6.4 Preserve write/read barriers and prohibit LIMIT from skipping required writes, sorting or grouping.

## 7. Stream snapshot reads

- [x] 7.1 Implement projected snapshot-visible iterators including pending mutations and early termination.
		- Verified for Badger transactions, Async, WAL, Composite, and Namespaced wrappers; tests cover pending updates/deletes, begin-snapshot replay, projection, deduplication, and early stop.
- [x] 7.2 Reproduce #487 and demonstrate per-shape latency/allocation improvements with visit counters and profiles.
	- Verified on Apple M2 Max with repeated `BenchmarkStatementRouting` runs: `simple_match_limit` 13.0 µs / 6.0 KB / 112 allocs in explicit tx vs 23.6 µs / 11.4 KB / 185 allocs autocommit; `property_lookup` 295 µs / 301 KB / 2,264 allocs vs 1.11 ms / 1.34 MB / 14,225; `filter_count` 202 µs / 295 KB / 2,124 vs 833 µs / 1.30 MB / 12,109; final `one_hop_limit` 111 µs / 101 KB / 1,339 vs 129 µs / 98.6 KB / 1,328; `return_literal` 3.28 µs / 2.61 KB / 44 vs 3.76 µs / 2.21 KB / 44. Added query-level and explicit-snapshot visit-count checks. CPU profiles drove bounded transaction-local caches for MVCC adjacency IDs, resumable label prefixes, endpoint nodes, and visible edge bodies; the final profile no longer shows adjacency-index iteration or edge-version lookup among the leading application costs. Full `pkg/storage` and `pkg/cypher` suites pass. Profile timings are hardware-specific; retain the benchmark for remeasurement on target hosts.
- [x] 7.3 Verify snapshot isolation, cancellation, buffer lifetime and performance of existing correct workloads.
	- Verified by begin-snapshot replay and adjacency consistency regressions, graceful snapshot-expiration cancellation/reader-release coverage, and wrapper tests that retain projected callback nodes/properties after iteration. Bounded-prefix/endpoint/edge cache tests pass under `-race`; the five-shape #487 benchmark matrix is recorded under 7.2.

## 8. Retire remaining divergence

- [ ] 8.1 Converge non-policy DDL and relevant node/edge kernels with contract tests and benchmarks.
		- Progress: unsupported `DROP` statements now return syntax errors, and missing INDEX/CONSTRAINT behavior has regressions. Full DDL/kernel contract coverage and benchmarks remain open.
- [ ] 8.2 Complete all remaining pinned-core TCK gaps and preserve Neo4j/Nornic extension suites.
- [ ] 8.3 Remove superseded handlers/evaluators/text-substitution paths; document retained optimizations and public API adapters.
- [ ] 8.4 Resolve every Cypher-relevant report candidate and document unrelated deferrals.

## 9. Qualify completion

- [ ] 9.1 Pass full tests, race suite, coverage gates, lint, build and scoped performance/retrieval checks.
- [ ] 9.2 Emit redacted informational fallback reports and a reproducible optimization backlog.
- [ ] 9.3 Attach fixing commits/PRs and passing evidence to every scoped issue; verify closure criteria.
- [ ] 9.4 Update compatibility/parser-mode docs, public API examples and CHANGELOG; synchronize verified OpenSpec contracts.
- [ ] 9.5 Review final completion evidence, close verified issues through the normal repository workflow, and archive the completed program.
