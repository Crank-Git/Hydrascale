#!/usr/bin/env bash
# check-hygiene.sh fails when a tracked file holds a secret, a private development note,
# tool state, or a large binary. The repository is public, so a committed match stays in
# the history for everyone.
#
# The script reads the tracked file list. An ignored file is not the concern; a tracked
# one is. The script exits 0 when the repository is clean, and 1 when it is not. On a
# failure the script prints the file and the pattern that matched.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

status=0

# report prints one defect and records the failure.
report() {
	printf 'hygiene: %s: matches %s\n' "$1" "$2"
	status=1
}

# A file larger than this is a binary or a build artifact.
max_bytes=2097152

# Issue #58 keeps the brand assets, so the size rule holds one exception.
brand_prefix='internal/ui/static/brand/'

while IFS= read -r -d '' file; do
	case "$file" in
	"$brand_prefix"*) continue ;;
	esac
	[ -f "$file" ] || continue
	size=$(wc -c <"$file")
	size=${size//[[:space:]]/}
	if [ "$size" -gt "$max_bytes" ]; then
		report "$file" "larger than 2 MB ($size bytes)"
	fi
done < <(git ls-files -z)

# CLAUDE.md, .claude/ and docs/specs/ are tracked deliberately, so no rule names them.
note_pattern='(^|/)(TODOS|HYPERPLAN|AGENTS)\.md$'
tool_pattern='^\.(gstack|omc|sisyphus|openagent)/'

for pattern in "$note_pattern" "$tool_pattern"; do
	while IFS= read -r file; do
		[ -n "$file" ] || continue
		report "$file" "$pattern"
	done < <(git ls-files | grep -E "$pattern" || true)
done

# Each pattern matches the shape of a real credential rather than the word. The
# repository holds documented placeholders such as tskey-auth-xxxxx, and a check that
# fails on those fails on every run.
secret_patterns=(
	'tskey-[a-z]+-[A-Za-z0-9]{22,}'
	'AKIA[0-9A-Z]{16}'
	'BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY'
)

# The script names every pattern, so git grep skips the script itself. git grep -I skips
# a file that Git reports as binary, because a compiled binary holds the strings that its
# source compiled into it.
for pattern in "${secret_patterns[@]}"; do
	while IFS= read -r hit; do
		[ -n "$hit" ] || continue
		# The report prints the file and the line number, and never the value that
		# matched, because the continuous integration log is public.
		report "$hit" "$pattern"
	done < <(git grep -nIE "$pattern" -- ':!scripts/check-hygiene.sh' | cut -d: -f1,2 | sort -u || true)
done

if [ "$status" -ne 0 ]; then
	printf 'hygiene: the repository holds content that must stay private.\n' >&2
fi

exit "$status"
