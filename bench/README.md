# Benchmarks

`bench/cmd/foco-bench` is the standard way to measure a foco change. It builds
two commits, runs the same searches with both against a deterministic fake
provider, and writes a report in the format pull requests use.

    make bench                                   # main vs HEAD, small suite
    make bench BENCH_HEAD=.                      # include uncommitted changes
    make bench BENCH_SUITE=medium BENCH_FLAGS='-runs 5 -tags vectorscan'
    make bench-fetch BENCH_SUITE=large           # download corpora ahead of time

The report goes to `bench/results/report.md` (paste it under `### Benchmark`
in the pull request) and the raw samples to `bench/results/results.json`. The
run exits with an error if the head build sends any must-exclude content to the
provider.

## What it measures

- **End-to-end searches** (`BM_Search/<corpus>/q<n>`): wall time, user CPU,
  and peak RSS of `foco "<query>" <corpus>`. Each build gets one untimed
  warm-up run per search, then `-runs` timed runs (default 3), alternating
  base and head; the report shows medians.
- **Provider traffic**: requests, requests rejected for exceeding the
  provider limits, TCP connections, and whether both builds printed
  byte-identical output.
- **Detector alone** (`BM_Detect/<suite>`, on by default, `-detect=false` to
  skip): the Titus + Betterleaks union over every text file up to 1 MB in the
  suite, with one caller per CPU, plus per-file verdict agreement.

## The fake provider

It speaks System One over TLS with HTTP/2, like Command Code's Jev endpoint,
and answers from a hash of the query, the path or state, and the question ID.
The same questions therefore always get the same answers, whatever the
batching, timing or order, and output differences between builds come from the
questions foco asks.

Defaults follow Jev: 300 ms per response (`-latency`) and HTTP 400 for
requests with more than 100 questions (`-max-questions`), as Jev does. Jev's
32K-token context can't be checked exactly without its tokenizer, so requests
over 40,000 bytes of state plus the longest question are only counted as
oversized (`-warn-context`); `-max-context N` rejects them instead. Indented
code measured close to one token per byte, but the ratio varies with content,
so treat oversized counts as a risk signal and confirm against Jev. `-latency-per-kb` adds latency
proportional to request size, closer to a real model. Set a limit to 0 to
disable it.

The fake provider measures foco, not model quality: it cannot tell whether a
real model would pick better files. Confirm user-facing claims with a few
searches against a real endpoint (`foco auth`, then `foco doctor`).

## Corpora

`bench/corpora.json` pins every corpus to a commit, so results are comparable
across runs and machines. Corpora are shallow-fetched once into
`$FOCO_BENCH_CACHE` (default `~/.cache/foco-bench`).

| Suite | Corpora | Download |
|---|---|---|
| `small` | gitleaks, trufflehog, betterleaks, Titus `test-secrets.html`, croc | 40 MB on disk |
| `medium` | small plus django, rails, ruby, sentry, lucene, TypeScript | 1.1 GB on disk |
| `large` | medium plus gitlab, mongo at two commits, linux | 6.3 GB on disk |

The secret scanners and the Titus test page are full of real-looking secrets:
they exercise the detector, and the Titus page's `mustExclude` entry fails the
run if its content reaches the provider. Searches stop at foco's discovery
limit of 100,000 entries, so the large corpora measure a capped walk.

Never edit a corpus or query in place, since that silently changes the
baseline of every comparison; add a new entry instead.

## Pull requests

Label a pull request `benchmark` to have `.github/workflows/benchmark.yml` run
the small suite for its base and head commits and post the report as a
comment, updated on every push. Shared GitHub runners are noisy, so read the
relative columns, and rerun locally for claims that depend on absolute times.
Pull requests from forks get the report in the workflow's job summary and as
an artifact instead of a comment, because their token cannot comment.
