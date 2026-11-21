#!/usr/bin/env python3
"""Score one tool's findings on the materialized corpus.

Usage: score.py <out dir> <findings.jsonl>

Each findings line is {"path": "<suite>/<path>", "start": int, "end": int}
in UTF-8 bytes. Spans are scored as in the benchmark's protocol: EXACT (a
finding equals it), COVERED (one finding contains it within its envelope),
OVERBROAD (contains it beyond the envelope), PARTIAL (overlap only) or MISS;
leaked = PARTIAL + MISS. A positive file is a fixture with secrets that has a
finding; a false alarm is a control (no secrets) with any finding.
"""
import collections
import json
import os
import sys

out, path = sys.argv[1], sys.argv[2]
index = {item['path']: item for item in json.load(open(os.path.join(out, 'index.json')))}
findings = collections.defaultdict(set)
for line in open(path):
    if line.strip():
        row = json.loads(line)
        if row['path'] in index:
            findings[row['path']].add((row['start'], row['end']))
positives = flagged = controls = alarms = 0
outcomes = collections.Counter()
for name, item in index.items():
    spans = findings.get(name, set())
    if not item['secrets']:
        controls += 1
        alarms += bool(spans)
        continue
    positives += 1
    flagged += bool(spans)
    for secret in item['secrets']:
        start, end = secret[0], secret[1]
        low, high = (secret[2], secret[3]) if len(secret) == 4 else (start, end)
        containing = [span for span in spans if span[0] <= start and end <= span[1]]
        if (start, end) in spans:
            outcomes['EXACT'] += 1
        elif any(low <= span[0] and span[1] <= high for span in containing):
            outcomes['COVERED'] += 1
        elif containing:
            outcomes['OVERBROAD'] += 1
        elif any(span[0] < end and start < span[1] for span in spans):
            outcomes['PARTIAL'] += 1
        else:
            outcomes['MISS'] += 1
total = sum(outcomes.values())
leaked = outcomes['PARTIAL'] + outcomes['MISS']
print(json.dumps({
    'positiveFiles': [flagged, positives], 'falseAlarms': [alarms, controls],
    'leakedSpans': [leaked, total], 'exactSpans': [outcomes['EXACT'], total], 'outcomes': outcomes,
}))
