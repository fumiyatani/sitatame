#!/usr/bin/env bash
# agent-handoff.sh — minimal "human reviews, agent consumes" pipeline.
#
# Flow:
#   1. Launch sitatame against the requested base (default: HEAD~1).
#   2. The reviewer presses `s` to save & promote.
#   3. We capture the SITATAME_REVIEW=<path> stdout line.
#   4. The promoted Markdown is forwarded to whatever consumer the caller
#      passed in via $SITATAME_AGENT (defaults to plain `cat`).
#
# Usage:
#   ./examples/agent-handoff.sh                       # base = HEAD~1
#   ./examples/agent-handoff.sh origin/main           # base = origin/main
#   SITATAME_AGENT='claude --print' ./examples/agent-handoff.sh

set -euo pipefail

BASE="${1:-HEAD~1}"
AGENT="${SITATAME_AGENT:-cat}"

# `sitatame` writes everything except SITATAME_REVIEW=<path> through the alt
# screen, so stdout is clean and grep-friendly when we capture it here.
output="$(sitatame "$BASE")"
review_path="$(printf '%s\n' "$output" | awk -F= '/^SITATAME_REVIEW=/{print $2; exit}')"

if [[ -z "$review_path" ]]; then
  echo "agent-handoff: sitatame exited without saving (was 's' pressed?)" >&2
  exit 1
fi

if [[ ! -f "$review_path" ]]; then
  echo "agent-handoff: review file not found at $review_path" >&2
  exit 1
fi

echo "agent-handoff: forwarding $review_path to '$AGENT'" >&2
exec sh -c "$AGENT" < "$review_path"
