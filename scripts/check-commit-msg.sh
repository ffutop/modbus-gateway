#!/usr/bin/env sh
# Validate commit messages against ENGINEERING_STANDARDS.md (Git commit message standard).
#
# Usage:
#   scripts/check-commit-msg.sh <message-file>     # check one message (commit-msg hook)
#   scripts/check-commit-msg.sh --range <rev>...    # check non-merge commits selected by git rev-list args
#   scripts/check-commit-msg.sh --message "<text>" # check a literal message (e.g. PR title)

set -u

PATTERN='^(feat|fix|docs|refactor|perf|test|build|ci|style|chore|revert)(\([a-z0-9._/-]+\))?!?: [^ ].*$'

# check_message <label> <text>; prints problems and returns non-zero on failure.
check_message() {
	label=$1
	text=$2
	# Git stores a trailing newline; anything beyond one line is a violation.
	lines=$(printf '%s' "$text" | sed -e '$a\' | grep -c '')
	first=$(printf '%s\n' "$text" | head -n 1)
	status=0
	if [ "$lines" -ne 1 ]; then
		echo "[$label] commit message must be a single line (found $lines lines, no body or trailers)" >&2
		status=1
	fi
	if printf '%s' "$text" | grep -qi 'co-authored-by'; then
		echo "[$label] commit message must not contain 'Co-Authored-By'" >&2
		status=1
	fi
	if ! printf '%s\n' "$first" | grep -Eq "$PATTERN"; then
		echo "[$label] commit message must match '<type>[(scope)][!]: <description>'" >&2
		echo "[$label]   types: feat fix docs refactor perf test build ci style chore revert" >&2
		echo "[$label]   got: $first" >&2
		status=1
	fi
	return $status
}

case "${1:-}" in
--range)
	shift
	[ $# -ge 1 ] || { echo "usage: $0 --range <rev>..." >&2; exit 2; }
	failed=0
	for sha in $(git rev-list --no-merges "$@"); do
		msg=$(git log -1 --format=%B "$sha" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}')
		check_message "$(git rev-parse --short "$sha")" "$msg" || failed=1
	done
	exit $failed
	;;
--message)
	[ $# -eq 2 ] || { echo "usage: $0 --message <text>" >&2; exit 2; }
	check_message "message" "$2"
	;;
"" | -h | --help)
	sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
*)
	# Drop comment lines and trailing blank lines the way git's cleanup does.
	msg=$(grep -v '^#' "$1" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}')
	check_message "commit-msg" "$msg"
	;;
esac
