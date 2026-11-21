# redact-secret-benchmarks comparison

Runs `foco scan` and peer scanners on the corpus of
[redact-secret-benchmarks](https://github.com/redact-secret/redact-secret-benchmarks)
at a pinned commit and scores their byte spans with that project's span
protocol (EXACT, COVERED, OVERBROAD, PARTIAL, MISS; leaked = PARTIAL + MISS).

    sh bench/redact-secret/run.sh bin/foco

The first run clones the benchmark (needs git, Node 22+ and npm to generate
its fixtures) into `~/.cache/foco-bench/redact-secret`. Each tool scans the
materialized corpus once as a whole directory; the time is the best of five.
gitleaks, betterleaks and trufflehog run when they are on `PATH`.

| Metric | Meaning |
|---|---|
| positive files | fixtures with a secret that get any finding |
| false alarms | controls (fixtures with no secret) that get any finding |
| leaked spans | secret spans with no single finding covering them |
| exact spans | secret spans with a finding of exactly the same range |

Caveats, also stated by the benchmark: the corpus is synthetic and maintained
by the redact-secret project, partly generated from its own detector registry,
so it is not a neutral basis for ranking tools. About 1,470 of the 3,749
controls are negative twins of a positive that differ in one structural
property (length, prefix, alphabet, delimiter, checksum), which only a
provider-specific format check can tell apart; generic rules flag both.
`score.py` counts every finding on a control as a false alarm, which may be
stricter than other harnesses.
