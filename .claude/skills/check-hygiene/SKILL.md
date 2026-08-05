---
name: check-hygiene
description: Check that the public repository holds no secret, no private development note, and no compiled binary. Use before a release, before a force push, or when asked whether the repository is clean.
allowed-tools: Bash, Read
---

# Check the public repository

This repository is public. A secret, a private note, or a compiled binary in a tracked
file is visible to everyone and it stays in the history.

Epic 1 adds `scripts/check-hygiene.sh` and continuous integration runs it. Until that
script exists, run the checks below by hand.

## Run the script

```sh
scripts/check-hygiene.sh
```

It exits non-zero and it names the file and the pattern that matched.

## The checks, by hand

Every check reads the tracked file list, not the working tree. An ignored file is not the
concern; a committed one is.

### A large file or a compiled binary

```sh
git ls-files | grep -v '^vendor/' | while read -r f; do
  s=$(wc -c <"$f"); [ "$s" -gt 2097152 ] && echo "$s $f"
done
```

Anything over 2 MB that is not under `internal/ui/static/brand/` is a defect.

### A private development note

```sh
git ls-files | grep -E '(^|/)(TODOS|HYPERPLAN|AGENTS)\.md$'
git ls-files | grep -E '^\.(gstack|omc|sisyphus|openagent)/'
```

`CLAUDE.md` and `.claude/` are tracked deliberately, so they are not in this check.
`docs/specs/` is tracked deliberately too.

### A secret

```sh
git grep -nE 'tskey-[a-z]+-[A-Za-z0-9]{22,}|AKIA[0-9A-Z]{16}|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' \
  -- ':!vendor' ':!scripts/check-hygiene.sh' ':!.claude/skills/check-hygiene' \
     ':!docs/specs/features/01-desktop-client-removal.md'
```

The patterns match the shape of a real credential, not the word. The repository holds
about fifteen documented placeholders — `tskey-auth-xxxxx` in `README.md`,
`tskey-auth-xxx` and `tskey-secret` in tests — and none of them match. A check that
failed on those would fail every run, and a check that always fails gets removed.

A match is a defect even in a test fixture. Add a new placeholder that is obviously fake
and obviously short, so it cannot match.

### The build output

```sh
git ls-files | grep -E '^(hydrascale|dist/|gui/build/bin/)'
```

Nothing should match.

## When a check fails

Remove the file with `git rm --cached` and add it to `.gitignore`. Then tell the operator
whether the file was ever pushed. If it was, the content is in the public history and
`git rm` does not remove it from there. Removing it from the history rewrites every
commit and breaks every fork, so it is the operator's decision, not yours.
