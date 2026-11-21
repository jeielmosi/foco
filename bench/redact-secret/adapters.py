#!/usr/bin/env python3
"""Turn a peer scanner's report into findings.jsonl for score.py.

Usage: adapters.py <tool> <out dir> <report> > findings.jsonl

gitleaks and betterleaks: their JSON report; the secret is located on its
reported line, nearest the reported column. trufflehog: its JSON lines; Raw
is located in the file, nearest the reported line.
"""
import json
import sys

tool, out, report = sys.argv[1], sys.argv[2], sys.argv[3]
prefix = out.rstrip('/') + '/fx/'


def emit(path, value, line, column=1):
    relative = ('/' + path).split('/fx/', 1)[1]
    data = open(prefix + relative, 'rb').read()
    needle = value.encode('utf-8')
    if not needle:
        return
    lines = data.split(b'\n')
    near = sum(len(text) + 1 for text in lines[:max(0, line - 1)]) + max(0, column - 1)
    best, position = None, data.find(needle)
    while position >= 0:
        if best is None or abs(position - near) < abs(best - near):
            best = position
        position = data.find(needle, position + 1)
    if best is not None:
        print(json.dumps({'path': relative, 'start': best, 'end': best + len(needle)}))


if tool in ('gitleaks', 'betterleaks'):
    for row in json.load(open(report)):
        emit(row['File'], row['Secret'] or row['Match'], row['StartLine'], row['StartColumn'])
elif tool == 'trufflehog':
    for text in open(report):
        try:
            row = json.loads(text)
        except ValueError:
            continue
        source = row['SourceMetadata']['Data']['Filesystem']
        emit(source['file'], row.get('Raw', ''), source.get('line', 1))
else:
    sys.exit('unknown tool ' + tool)
