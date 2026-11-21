#!/bin/sh
# Compare foco scan with peer scanners on redact-secret-benchmarks.
#
#   sh bench/redact-secret/run.sh [foco binary]
#
# Clones the benchmark at a pinned commit, materializes its corpus once, then
# times one whole-directory scan per tool (best of 5) and scores its spans.
# Peers (gitleaks, betterleaks, trufflehog) run when they are on PATH.
set -eu
FOCO=${1:-foco}
COMMIT=019a450a4dbecf7d4dc0941b06ca4fbcb726cd0e
HERE=$(cd "$(dirname "$0")" && pwd)
WORK=${FOCO_REDACT_BENCH:-${XDG_CACHE_HOME:-$HOME/.cache}/foco-bench/redact-secret}
mkdir -p "$WORK"
if [ ! -d "$WORK/checkout/.git" ]; then
	git init -q "$WORK/checkout"
	git -C "$WORK/checkout" fetch -q --depth 1 https://github.com/redact-secret/redact-secret-benchmarks "$COMMIT"
	git -C "$WORK/checkout" -c advice.detachedHead=false checkout -q FETCH_HEAD
	(cd "$WORK/checkout" && npm ci --no-audit --no-fund >/dev/null && node --import tsx scripts/generate-fixtures.mjs --ensure >/dev/null)
fi
[ -f "$WORK/index.json" ] || python3 "$HERE/materialize.py" "$WORK/checkout" "$WORK"

best_of_five() {
	best=
	for _ in 1 2 3 4 5; do
		start=$(date +%s%N)
		"$@" >/dev/null 2>&1 || true
		elapsed=$(( ($(date +%s%N) - start) / 1000000 ))
		if [ -z "$best" ] || [ "$elapsed" -lt "$best" ]; then best=$elapsed; fi
	done
	echo "$best"
}

cd "$WORK"
ms=$(best_of_five "$FOCO" scan --json fx)
"$FOCO" scan --json fx > foco.jsonl || true
sed 's#"path":"fx/#"path":"#' foco.jsonl > foco.norm.jsonl
echo "foco $ms ms $(python3 "$HERE/score.py" . foco.norm.jsonl)"
if command -v gitleaks >/dev/null; then
	ms=$(best_of_five gitleaks dir fx --no-banner --exit-code 0 -f json -r /dev/null)
	gitleaks dir fx --no-banner --exit-code 0 -f json -r gitleaks.json >/dev/null 2>&1 || true
	python3 "$HERE/adapters.py" gitleaks . gitleaks.json > gitleaks.jsonl
	echo "gitleaks $ms ms $(python3 "$HERE/score.py" . gitleaks.jsonl)"
fi
if command -v betterleaks >/dev/null; then
	ms=$(best_of_five betterleaks dir fx --no-banner --exit-code 0 -f json -r /dev/null)
	betterleaks dir fx --no-banner --exit-code 0 -f json -r betterleaks.json >/dev/null 2>&1 || true
	python3 "$HERE/adapters.py" betterleaks . betterleaks.json > betterleaks.jsonl
	echo "betterleaks $ms ms $(python3 "$HERE/score.py" . betterleaks.jsonl)"
fi
if command -v trufflehog >/dev/null; then
	ms=$(best_of_five trufflehog filesystem fx --json --no-update --no-verification)
	trufflehog filesystem fx --json --no-update --no-verification > trufflehog.json 2>/dev/null || true
	python3 "$HERE/adapters.py" trufflehog . trufflehog.json > trufflehog.jsonl
	echo "trufflehog $ms ms $(python3 "$HERE/score.py" . trufflehog.jsonl)"
fi
