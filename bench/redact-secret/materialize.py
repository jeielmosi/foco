#!/usr/bin/env python3
"""Write the redact-secret-benchmarks corpus to disk for whole-directory scans.

Usage: materialize.py <redact-secret-benchmarks checkout> <out dir>

Writes <out>/fx/<suite>/<path> for every scored fixture and <out>/index.json
with each fixture's secret spans ([start, end) UTF-8 bytes, plus the authored
envelope when present). Like the benchmark's reports, calibration-only suites
and T0 (pending evidence) fixtures are left out.
"""
import json
import os
import sys

checkout, out = sys.argv[1], sys.argv[2]
index = []
for suite in json.load(open(os.path.join(checkout, 'benchmarks', 'categories.json'))):
    if suite.get('calibrationOnly'):
        continue
    corpus = json.load(open(os.path.join(checkout, suite['corpus'])))
    for fixture in corpus['fixtures']:
        assessment = fixture.get('assessment', {})
        if assessment.get('tier') == 'T0':
            continue
        relative = os.path.join(suite['id'], fixture['path'])
        target = os.path.join(out, 'fx', relative)
        os.makedirs(os.path.dirname(target), exist_ok=True)
        with open(target, 'wb') as handle:
            handle.write(fixture['content'].encode('utf-8'))
        secrets = []
        for expected in fixture.get('expected', []):
            if expected.get('role', 'secret') != 'secret':
                continue
            span = [expected['start'], expected['end']]
            envelope = expected.get('envelope')
            if envelope:
                span += [envelope['start'], envelope['end']]
            secrets.append(span)
        index.append({'path': relative, 'suite': suite['id'], 'kind': assessment.get('kind'),
                      'tier': assessment.get('tier'), 'secrets': secrets})
json.dump(index, open(os.path.join(out, 'index.json'), 'w'))
positives = sum(1 for item in index if item['secrets'])
print('%d fixtures, %d with secrets, %d controls, %d secret spans' % (
    len(index), positives, len(index) - positives, sum(len(item['secrets']) for item in index)))
