#!/usr/bin/env bash
# codescene-log.sh — RETIRED. It writes nothing and always exits 0.
#
# The server appends the codescene-events.jsonl record itself, for every
# validate_completion call whose `codescene` argument reports a run. A hook
# that also wrote one would count each run twice, so this script is kept only
# so that a settings.json still pointing at it does not fail: remove the
# PostToolUse entry for mcp__codescene__analyze_change_set.
# See docs/team-setup/codescene-stats.md.
cat >/dev/null 2>&1 || true
exit 0
