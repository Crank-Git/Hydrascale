---
id: desktop-client-removal
feature: Desktop client removal and repository hygiene
epic: "Epic 1: Desktop client removal and repository hygiene"
status: built
issues: [55, 56, 57, 58, 59, 60, 61]
mockups: []
---

## Purpose

The desktop client is a Wails application in `gui/`. It is a separate Go module with a
single-file front end. It duplicates what the console will do, it cannot cross-compile,
and it forces a three-platform build matrix on every release. Version 1.0 removes it.

The repository also carries a 15 MB compiled binary, two large images, and build output.
Those account for most of the tracked bytes in a project whose source is 11 000 lines.
Version 1.0 stops tracking them.

The operator chose to remove forward and to keep the Git history. Every existing tag,
fork, and clone keeps working. The repository does not get smaller to clone. It stops
getting larger.

## What the desktop client is

| Item | Path or reference |
|---|---|
| Source | `gui/` — a separate Go module, 16 tracked files. |
| Commits | `8d49ad3` (#39), `c33f44b` (#42), `be73281` (#43), `0fd0d17` (#44), `8733fbd` (#45), `4cfff21` (#46). |
| Release wiring | The per-operating-system build matrix in `.github/workflows/release.yml`. |
| Documentation | The `Desktop GUI` section and the `Remote / GUI access` section in `README.md`. |
| Ignore rules | The `gui/frontend/dist/` and `gui/build/` exception block in `.gitignore`. |
| Version plumbing | The version string that `c33f44b` embedded for the desktop client. |

`socket_group` came from `f00b0c6` (#40). It exists for the desktop client, but it is a
daemon feature and a remote operator still needs it. Version 1.0 keeps it.

## User stories

- As a contributor, I want to clone the repository without downloading a stale binary,
  so that the clone is fast and the binary cannot be mistaken for a release.
- As the operator, I want the release workflow to build one artifact, so that a release
  cannot fail on a Windows runner.
- As the operator, I want the brand assets in the repository, so that the console can
  use them, without the private design source that produced them.
- As the operator, I want an automated check that no private note reaches the public
  repository.

## Functional requirements

- **FR-removal-1** — The repository contains no `gui/` directory.
- **FR-removal-2** — The release workflow builds the Linux daemon only.
- **FR-removal-3** — The release workflow attaches no desktop client artifact.
- **FR-removal-4** — The repository does not track a compiled binary.
- **FR-removal-5** — `.gitignore` ignores the `hydrascale` binary at the repository root.
- **FR-removal-6** — `.gitignore` contains no exception rule for `gui/`.
- **FR-removal-7** — `README.md` contains no desktop client section.
- **FR-removal-8** — `README.md` documents `socket_group` as a remote access feature
  rather than as a desktop client feature.
- **FR-removal-9** — The repository contains the brand tokens, the twelve icons, and the
  three logo files, under `internal/ui/static/brand/`.
- **FR-removal-10** — The repository contains no file from
  `branding/design_handoff_hydrascale_brand/` other than the assets that FR-removal-9
  names.
- **FR-removal-11** — `.gitignore` ignores `branding/`.
- **FR-removal-12** — A repository hygiene script fails when a tracked file matches a
  private-content pattern.
- **FR-removal-13** — The continuous integration workflow runs the repository hygiene
  script.
- **FR-removal-14** — The repository contains no `assets/logo.png` reference in
  `README.md` after the logo changes to the new mark.

## User flows

### The maintainer removes the desktop client

1. The maintainer deletes `gui/` with `git rm -r gui`.
2. The maintainer removes the build matrix from `.github/workflows/release.yml`.
3. The maintainer removes the desktop client sections from `README.md`.
4. The maintainer removes the `gui/` exception block from `.gitignore`.
5. The maintainer runs `go build ./...` and `go test ./...`.
6. The maintainer confirms that the daemon module never imported the `gui` module.

### The hygiene check runs

1. A contributor opens a pull request.
2. The workflow runs `scripts/check-hygiene.sh`.
3. The script lists every tracked file.
4. The script fails when a tracked file matches a forbidden pattern.
5. The script prints the file and the pattern that matched.

## Screens & states

This feature set has no screen.

## Behaviour rules

### The hygiene script

The script fails when a tracked file matches any of these:

| Pattern | Reason |
|---|---|
| A file larger than 2 MB that is not under `internal/ui/static/brand/`. | A binary or a build artifact. |
| `TODOS.md`, `HYPERPLAN.md`, `CLAUDE.md`, `AGENTS.md`. | A private development note. |
| A path under `.claude/`, `.gstack/`, `.omc/`, `.sisyphus/`, `.openagent/`. | Tool state. |
| A file that holds a credential shaped like a real one. | A secret. The exact patterns are below. |
| A file that contains `Claude Code`, `claude.ai`, or `Generated with Claude`. | A private tooling reference. |

The last pattern excludes the script itself, because the script names the pattern.

### The secret patterns must not match a placeholder

The repository already holds about fifteen documented placeholders: `tskey-auth-xxxxx`
in `README.md`, `tskey-auth-xxx` in `internal/config/config_test.go`, and `tskey-secret`
in `internal/api/server_test.go`. A check that fails on those fails on every run, and a
check that always fails gets removed. Match the shape of a real credential instead:

| Pattern | Matches | Does not match |
|---|---|---|
| `tskey-[a-z]+-[A-Za-z0-9]{22,}` | A real Tailscale auth key. | `tskey-auth-xxxxx`, `tskey-secret`, `tskey-abc`. |
| `AKIA[0-9A-Z]{16}` | An AWS access key id. | The word `AKIA` alone. |
| `BEGIN (RSA \|OPENSSH \|EC )?PRIVATE KEY` | A private key block. | A sentence about private keys. |

The script also skips a file that Git reports as binary, because a compiled binary
contains the placeholder strings that the source compiled into it.

A new placeholder must not match a pattern above. Use a value that is obviously fake and
obviously short.

### The brand assets

The console needs the tokens, the icons, and the logos. It does not need the React
component reference, the specimen pages, the prototypes, or `SKILL.md`. Those stay in
`branding/`, which becomes an ignored working directory.

`BRAND.md` does not go into the repository as written. It names Claude Code and it
describes the design as a proposal. Epic 9 writes `docs/DESIGN.md` from its content, for
a contributor, without those references.

The copied token files drop the Google Fonts `@import` from `tokens/fonts.css`. Epic 6
decides what replaces it.

## Data touched

No entity changes. Files move and files are deleted.

## Interfaces

None.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The daemon module imports something from `gui/`. | It does not. `gui/go.mod` is a separate module and the daemon does not require it. The build proves this. |
| A user has a bookmark to a desktop client release asset. | The asset stays on the old release. Version 1.0 does not delete a published release. |
| The hygiene script finds a pre-existing violation. | The pull request that adds the script also fixes the violation. |
| A contributor adds a large brand asset. | The script allows files under `internal/ui/static/brand/`, so an icon passes. A 5 MB image there still passes, which the review must catch. |

## Acceptance criteria

- [ ] `git ls-files gui` returns nothing.
- [ ] `git ls-files hydrascale` returns nothing.
- [ ] `go build ./...` and `go test ./...` pass.
- [ ] The release workflow file names no operating system other than Linux.
- [ ] `README.md` contains no occurrence of `Wails` or `Desktop GUI`.
- [ ] `internal/ui/static/brand/tokens/colors.css` exists and matches the source token
      file.
- [ ] `internal/ui/static/brand/icons/` contains twelve SVG files.
- [ ] `git check-ignore branding` reports that `branding` is ignored.
- [ ] `scripts/check-hygiene.sh` exits zero on the cleaned repository.
- [ ] `scripts/check-hygiene.sh` exits zero on the repository as it stands, with every
      existing placeholder in `README.md`, `internal/config/config_test.go`, and
      `internal/api/server_test.go` left unchanged.
- [ ] `scripts/check-hygiene.sh` exits non-zero when a test adds a file holding a value
      that matches `tskey-[a-z]+-[A-Za-z0-9]{22,}`.
- [ ] The continuous integration workflow runs the hygiene script.

## Out of scope

- A Git history rewrite. The operator decided against it. The 15 MB binary stays in the
  history and the clone size does not fall.
- Deletion of any published GitHub Release or tag.
- Removal of `socket_group`.

## Open questions

None.
