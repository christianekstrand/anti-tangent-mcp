#!/usr/bin/env bash
# Asserts the retired codescene-log.sh writes nothing and exits 0.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
out=$(mktemp -d)
export ANTI_TANGENT_STATS_DIR="$out"

payload='{"tool_response":[{"type":"text","text":"{\"results\":[{\"verdict\":\"degraded\",\"findings\":[]}],\"quality_gates\":\"failed\"}"}]}'
printf '%s' "$payload" | "$here/codescene-log.sh" || { echo "FAIL: non-zero exit"; exit 1; }
echo 'not json at all' | "$here/codescene-log.sh" || { echo "FAIL: non-zero exit on bad input"; exit 1; }
"$here/codescene-log.sh" </dev/null || { echo "FAIL: non-zero exit on empty stdin"; exit 1; }
[ -z "$(ls -A "$out")" ] || { echo "FAIL: the retired hook wrote into the stats dir"; exit 1; }

echo OK
