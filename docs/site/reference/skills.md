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

The binary holds two skills:

- **`hydrascale-setup`** reads the Hydrascale state of a host and reports it. It runs the
  five commands that change no state: `status`, `list`, `diff`, `env`, and `version`. It
  prints every command that changes the host, with the precondition and the risk, and it
  runs none of them.
- **`tailnet-exec`** sends a command into the namespace of one tailnet rather than to the
  host network. It states the five routing forms: `exec`, `tailscale`, `ping`, `ssh`, and
  `wrap`. It reads the tailnet identifier from `hydrascale list`.

`skills/hydrascale-setup/SKILL.md` and `skills/tailnet-exec/SKILL.md` hold the source of the
two skills. A test reads each file, and it fails when a skill names a `hydrascale` command
that the binary does not hold.
