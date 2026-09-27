#!/usr/bin/env python3
"""List structural convergence candidates from a Graphify code graph."""

import argparse
import json
from collections import defaultdict
from itertools import combinations
from pathlib import Path

import ijson


def graph_items(path, key):
    with path.open("rb") as graph_file:
        yield from ijson.items(graph_file, f"{key}.item")


def analyze(graph_path, components):
    symbols = {}
    groups = defaultdict(list)
    node_counts = defaultdict(int)
    calls = defaultdict(set)
    call_counts = defaultdict(int)

    for node in graph_items(graph_path, "nodes"):
        source_file = node.get("source_file", "")
        component = source_file.rsplit("/", 1)[0]
        if component not in components or not source_file.endswith(".go") or source_file.endswith("_test.go"):
            continue
        node_counts[component] += 1
        label = node.get("label", "")
        if not label.endswith("()"):
            continue
        symbol = {"id": node["id"], "file": source_file, "location": node.get("source_location", "")}
        symbols[node["id"]] = symbol
        groups[(component, label)].append(symbol)

    for edge in graph_items(graph_path, "edges"):
        if edge.get("relation") != "calls":
            continue
        caller = edge.get("source")
        callee = edge.get("target")
        if caller not in symbols:
            continue
        component = symbols[caller]["file"].rsplit("/", 1)[0]
        call_counts[component] += 1
        calls[caller].add(callee)

    candidates = []
    for (component, label), members in sorted(groups.items()):
        files = {member["file"] for member in members}
        if len(files) < 2:
            continue
        members.sort(key=lambda member: (member["file"], member["location"], member["id"]))
        pairs = []
        for left, right in combinations(members, 2):
            if left["file"] == right["file"]:
                continue
            shared = calls[left["id"]] & calls[right["id"]]
            if shared:
                pairs.append({"left": left, "right": right, "shared_callee_ids": sorted(shared)})
        if not pairs:
            continue
        candidates.append({
            "component": component,
            "label": label,
            "pairs": pairs,
        })

    return {
        "components": [{"name": component, "nodes": node_counts[component], "call_edges": call_counts[component]}
                       for component in sorted(components)],
        "candidates": candidates,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--graph", type=Path, default=Path("graphify-out/graph.json"))
    parser.add_argument("--component", action="append", required=True, help="Go package path, e.g. pkg/cypher")
    args = parser.parse_args()
    if not args.graph.is_file():
        parser.error(f"graph not found: {args.graph}")
    print(json.dumps(analyze(args.graph, set(args.component)), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()