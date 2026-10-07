# Agent skills

A skill is one Markdown file that states how a coding agent does one task. The binary holds
the skill set, so a host needs no copy of this repository.

## Install the skills

Run the command as the account that runs the coding agent. Run it without `sudo`:

```bash
hydrascale skills install
```

The command writes each skill to `$HOME/.claude/skills/<name>/SKILL.md`. It refuses to run
as root, because the coding agent reads the skill directory of the operator and not the
directory of root.

| Flag | Effect |
|---|---|
| `--dir <path>` | Writes to this directory rather than to `$HOME/.claude/skills`. |
| `--force` | Writes over a skill file that exists. Without it, the command keeps that file. |
| `--dry-run` | Prints each path and writes no file. |

The command creates a missing directory at the mode `0755` and writes each file at the mode
`0644`. When the environment holds no `HOME`, pass `--dir`.

`hydrascale skills list` prints the name and the description of each skill.

## The skills

The binary holds three skills:

- **`hydrascale-setup`** reads the Hydrascale state of a host and reports it. It runs the
  five commands that change no state: `status`, `list`, `diff`, `env`, and `version`. It
  prints every command that changes the host, with the precondition and the risk, and it
  runs none of them.
- **`tailnet-exec`** sends a command into the namespace of one tailnet rather than to the
  host network. It states the five routing forms: `exec`, `tailscale`, `ping`, `ssh`, and
  `wrap`. It reads the tailnet identifier from `hydrascale list`.
- **`hydrascale-troubleshoot`** finds the cause of a fault with read-only commands. It holds
  five diagnoses: IPv6, a direct connection, DNS, a displaced jump rule, and a rejected
  credential. For each cause it prints the command that repairs it, and it runs none of
  them.

`skills/hydrascale-setup/SKILL.md`, `skills/tailnet-exec/SKILL.md`, and
`skills/hydrascale-troubleshoot/SKILL.md` hold the source of the three skills. A test
reads each file. The test fails when a skill states one of these:

- A `hydrascale` command that the binary does not hold, or a flag that the command does
  not declare.
- A configuration key in backticks that the daemon does not read.
- An event type in backticks that the daemon does not record.
- A source line in the form `file.go:NN`.
- A link to a page that this site does not hold, or a path under `docs/site/`.

A second test fails when a directory under `skills/` holds no content test.
