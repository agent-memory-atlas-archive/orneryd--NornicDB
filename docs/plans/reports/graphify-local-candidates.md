# Local Graphify convergence candidates

This is a reproducible structural candidate list, not a replacement for
`DIVERGENCE_REPORT.md`. That report analyzed eight separately extracted graphs
at `994b3a68` with a generator absent from this checkout; the local graph was
extracted from the full repository. Its counts are not directly comparable.

Input: `graphify-out/graph.json` (ignored), Graphify 0.9.69, SHA-256
`54fa1bd04cf13751c7dd50ef469fdc57cbe299813e4ac3605a5cfd80898c4eb2`.
Regenerate after each graph refresh; this hash identifies only the measured
snapshot, not a checked-in baseline. With the isolated Graphify environment:

```sh
"$(uv tool dir)/graphifyy/bin/python" scripts/graphify_divergence.py \
  --component pkg/cypher --component pkg/storage --component pkg/search \
  --component pkg/nornicdb --component pkg/server --component pkg/bolt \
  --component pkg/multidb --component pkg/embed
```

| Component | Non-test Go graph nodes | Outgoing call edges |
| --- | ---: | ---: |
| pkg/bolt | 369 | 420 |
| pkg/cypher | 3,914 | 7,700 |
| pkg/embed | 167 | 99 |
| pkg/multidb | 201 | 193 |
| pkg/nornicdb | 398 | 580 |
| pkg/search | 1,044 | 1,236 |
| pkg/server | 424 | 674 |
| pkg/storage | 2,535 | 3,205 |

The analyzer found 38 same-label, cross-file symbol groups with at least one
pair sharing a `calls` target. Its JSON lists the exact files, locations, IDs
and shared callee IDs for every pair in stable order. These are review leads:
shared conversions, interface implementations and calls into an already-shared
kernel do not imply divergent behavior. For example, the three search
`.CandidateSearchGraph()` implementations call the shared GPU build kernel.
The scan took 2.30 seconds with 31.4 MB maximum resident memory on an Apple
M2 Max (`/usr/bin/time -l`); no earlier analyzer baseline exists for an A/B
comparison.

The old report's router, wrapper and source-similarity claims still require a
separate current-source comparison and disposition; this graph-only filter
cannot establish semantic equivalence or measure runtime performance.