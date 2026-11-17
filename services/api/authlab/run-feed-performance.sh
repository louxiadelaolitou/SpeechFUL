#!/bin/sh
# Baseline is a complete archive/checkout, not current code with a list flag.
set -eu
baseline=${REQ002_BASELINE_SOURCE:?Full frozen source checkout is required}
output=${REQ002_PERFORMANCE_DIRECTORY:?New artifact directory is required}
source_label=${REQ002_SOURCE:?Current SHA plus explicit local difference is required}
expected=1d829b2623a6c32830ee1b74fae6c99a436ad3b3
[ "$(cat "$baseline/.req002-frozen-baseline")" = "$expected" ] || { echo 'Wrong frozen source provenance' >&2; exit 2; }
[ ! -e "$output" ] || { echo 'Refusing existing performance artifacts' >&2; exit 2; }
mkdir -p "$output"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
current=$(CDPATH= cd -- "$script_dir/../../.." && pwd)
cp "$current/services/api/internal/authprivacyhttp/feed_batch_performance_test.go" "$baseline/services/api/internal/authprivacyhttp/feed_batch_performance_test.go"
cp "$current/services/api/internal/authprivacyhttp/feed_performance_clock_linux_test.go" "$baseline/services/api/internal/authprivacyhttp/feed_performance_clock_linux_test.go"
cp "$current/services/api/internal/authprivacyhttp/feed_performance_clock_other_test.go" "$baseline/services/api/internal/authprivacyhttp/feed_performance_clock_other_test.go"
export FEED_TEST_RACE=0 FEED_TEST_PATTERN='^TestRealHTTPSFeedBatchPerformancePostgres$' REQ002_PERFORMANCE=1
status=0
for round in 1 2 3; do
    modes='legacy batch'
    [ "$round" != 2 ] || modes='batch legacy'
    for mode in $modes; do
        tree=$current
        label=$source_label
        if [ "$mode" = legacy ]; then tree=$baseline; label=$expected; fi
        # Preserve context alongside every sample set. Do not discard rounds
        # based on their latency or silently retry under the same artifact name.
        { date -u; uptime; ps -eo pid,pcpu,pmem,comm; } >"$output/$mode-$round-environment.txt"
        REQ002_SOURCE="$label" REQ002_PERFORMANCE_MODE="$mode" REQ002_PERFORMANCE_ROUND="$round" \
            REQ002_PERFORMANCE_OUTPUT="$output/$mode-$round.json" \
            sh "$tree/services/api/authlab/run-feed-isolated.sh" >"$output/$mode-$round.log" 2>&1 || status=1
    done
done
python3 "$script_dir/compare-feed-performance.py" "$output" || status=1
exit "$status"
