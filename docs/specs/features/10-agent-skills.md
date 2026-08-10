---
id: agent-skills
feature: Agent skills for the operator, and the two command defects they expose
epic: "Epic 10: Agent skills"
status: issued
issues: [247, 248, 249, 250, 251, 252]
mockups: []
---

## Purpose

An operator runs a coding agent on the host. The agent must send a command to one tailnet
rather than to the host. Today the agent guesses. It reads `hydrascale --help`, it finds
the command `switch`, and it believes that the command changes the default namespace. The
command changes nothing. The agent then runs a bare command, the command reaches the
host network, and the result misleads the operator.

This feature set ships two skills that state the correct forms, and it corrects the two
commands that state an incorrect form today.

A skill is a Markdown file that a coding agent loads. The agent reads the skill when the
task matches the description in the file. A skill costs no new network surface and no new
privilege, because the agent runs the same commands that the operator runs.

## The two defects

### `hydrascale switch` changes no state

`cmd/hydrascale/main.go:389` declares the command with the summary
`Switch default namespace`. The body validates the identifier, computes the namespace
name, prints one line, and returns:

> ```go
> nsName := namespaces.GetNamespaceName(tailnetID)
> fmt.Printf("Switched to tailnet %s (namespace: %s)\n", tailnetID, nsName)
> return nil
> ```

The command writes no file and it changes no process. A child process cannot move its
parent shell into a namespace, so the summary describes work that the command cannot
perform.

### The help of `hydrascale env` states a procedure that fails

`cmd/hydrascale/main.go:757` declares the command. The `Long` field states:

> ```
> Print shell commands that configure the environment for a tailnet namespace.
> Use with eval to set up your shell:
>
>   eval $(hydrascale env personal)
>   curl http://my-tailscale-host:8080
> ```

The command prints three exports: `HYDRASCALE_TAILNET`, `HYDRASCALE_NAMESPACE`, and
`TAILSCALE_SOCKET`. No export moves the shell into the namespace. The `curl` on the second
line reaches the host network. The only line that routes the traffic is a comment:

> ```
> #   sudo ip netns exec %s <command>
> ```

## User stories

- As an operator, I want the agent to send a command to the tailnet I name, so that the
  result describes that tailnet.
- As an operator, I want the agent to read the tailnet identifiers from the host, so that
  I do not repeat them in every request.
- As an operator, I want the agent to refuse a command that stops a tailnet, so that a
  request does not disconnect the host.
- As an operator, I want one command that installs the skills, so that I do not clone the
  repository.
- As a contributor, I want a test that fails when a skill names a command that the binary
  does not hold, so that the skill and the binary stay equal.

## Functional requirements

### The command corrections

- **FR-skills-1** — `hydrascale switch <id>` states that it changes no state.
- **FR-skills-2** — `hydrascale switch <id>` prints the two forms that route a command:
  `sudo hydrascale exec <id> -- <command>` and
  `sudo hydrascale tailscale <id> -- <arguments>`.
- **FR-skills-3** — The summary of `hydrascale switch` reads
  `Print the namespace name for a tailnet (changes no state)`.
- **FR-skills-4** — `hydrascale switch <id>` exits 0 for a configured tailnet, so an
  existing script keeps its exit status.
- **FR-skills-5** — The `Long` help of `hydrascale env` holds no `eval` example that is
  followed by a bare command.
- **FR-skills-6** — `hydrascale env <id>` prints a shell function named `hstn` that runs
  `sudo hydrascale exec <id> -- "$@"`.
- **FR-skills-7** — The `Long` help of `hydrascale env` states that an environment
  variable does not move the shell into the namespace.
- **FR-skills-36** — `hydrascale env <id>` prints a comment above the function `hstn` that
  states that `hydrascale exec` needs root.
- **FR-skills-37** — `hydrascale switch <id>` prints the line
  `Both forms run ip netns exec, which needs root.`

### The skill files

- **FR-skills-8** — The repository holds `skills/tailnet-exec/SKILL.md` and
  `skills/hydrascale-setup/SKILL.md`.
- **FR-skills-9** — Each skill file holds a YAML front matter block with the keys `name`
  and `description`.
- **FR-skills-10** — `skills/tailnet-exec/SKILL.md` names the five routing forms: `exec`,
  `tailscale`, `ping`, `ssh`, and `wrap`.
- **FR-skills-11** — `skills/tailnet-exec/SKILL.md` states that `exec` and `tailscale`
  need the separator `--`, and that `ping` and `ssh` do not.
- **FR-skills-12** — `skills/tailnet-exec/SKILL.md` tells the agent to run
  `hydrascale list` and `hydrascale status` before the first routed command.
- **FR-skills-13** — `skills/tailnet-exec/SKILL.md` states that `hydrascale list` reads
  the configuration file, so it names a tailnet that is not connected.
- **FR-skills-14** — `skills/tailnet-exec/SKILL.md` states that `hydrascale switch`
  changes no state.
- **FR-skills-15** — `skills/tailnet-exec/SKILL.md` holds one section for a daemon on
  another host, with the form `ssh <host> hydrascale exec <id> -- <command>`.
- **FR-skills-16** — `skills/hydrascale-setup/SKILL.md` declares `allowed-tools` that
  hold the read commands only: `status`, `list`, `diff`, `env`, and `version`.
- **FR-skills-17** — `skills/hydrascale-setup/SKILL.md` tells the agent to print a
  mutating command for the operator to run, and to run no mutating command.
- **FR-skills-18** — `skills/hydrascale-setup/SKILL.md` states that a configuration file
  that holds no `access` block loses every path between two tailnets under the mode
  `enforce`, and that the operator sets the mode `observe` first.
- **FR-skills-19** — `skills/hydrascale-setup/SKILL.md` states that the console has no
  authentication, that it binds a loopback address, and that an SSH forward is the way to
  reach it from another machine.
- **FR-skills-20** — Each skill file follows `.claude/rules/ste.md`.

### The install command

- **FR-skills-21** — `skills/embed.go` embeds the directory `skills` in the binary.
- **FR-skills-22** — `hydrascale skills install` writes each skill to
  `$HOME/.claude/skills/<name>/SKILL.md`.
- **FR-skills-23** — `hydrascale skills install` exits 1 with a message when the
  effective user identifier is 0, because the skill belongs to the account of the
  operator rather than to root.
- **FR-skills-24** — `hydrascale skills install` writes no file that exists, and it
  states the path it kept.
- **FR-skills-25** — `hydrascale skills install --force` writes a file that exists.
- **FR-skills-26** — `hydrascale skills install --dry-run` prints each path and writes no
  file.
- **FR-skills-27** — `hydrascale skills install --dir <path>` writes to `<path>` rather
  than to `$HOME/.claude/skills`.
- **FR-skills-28** — `hydrascale skills install` prints one line per file that it wrote.
- **FR-skills-29** — `hydrascale skills list` prints the name and the description of each
  embedded skill.

### The drift test

- **FR-skills-30** — A Go test reads each embedded skill file and fails when the front
  matter holds no `name` key or no `description` key.
- **FR-skills-31** — A Go test extracts each `hydrascale <command>` string from each
  embedded skill file, and it fails when the command tree holds no such command.
- **FR-skills-32** — A Go test runs `skills install` against a temporary directory and
  asserts the written paths.

### The repository and the documentation

- **FR-skills-33** — `.claude/skills/tailnet-exec` and `.claude/skills/hydrascale-setup`
  are symbolic links to the matching directory under `skills/`.
- **FR-skills-34** — `README.md` holds one section that states the command
  `hydrascale skills install` and what the two skills do.
- **FR-skills-35** — `scripts/check-hygiene.sh` passes with the new files.

## User flows

### The operator installs the skills

1. The operator installs the binary.
2. The operator runs `hydrascale skills install`.
3. The command writes two files under `$HOME/.claude/skills`.
4. The operator starts the coding agent.
5. The operator asks the agent to reach a peer in one tailnet.
6. The agent loads `tailnet-exec`, runs `hydrascale list`, and runs
   `hydrascale exec <id> -- <command>`.

### The agent answers a setup question

**Warning — step 4 prints a command that the agent does not run.**

1. The operator asks the agent why a tailnet is down.
2. The agent loads `hydrascale-setup`.
3. The agent runs `hydrascale status` and `hydrascale diff`.
4. The agent prints the mutating command, the precondition, and the risk.
5. The operator reads the risk and runs the command.

## Behaviour rules

- A skill states a form that the binary holds. It states no form that a later release
  will add.
- A skill quotes a command exactly. It writes no paraphrase of a command.
- `hydrascale skills install` writes under the home directory of the invoking account and
  under no other path.
- The setup skill prints a mutating command. It runs none.
- A correction to a command changes the text that the command prints. It removes no
  command, because version 1.0 is released.

## Data touched

No entity changes. `hydrascale skills install` writes files under `$HOME/.claude/skills`.

## Interfaces

| Interface | Kind | Purpose |
|---|---|---|
| `hydrascale skills install` | Command | Write the embedded skills to the skill directory of the operator. |
| `hydrascale skills list` | Command | Print the name and the description of each embedded skill. |

The daemon gains no route. The control socket and the console are unchanged.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The account holds no `$HOME`. | The command exits 1 and names the variable `HOME`. |
| `$HOME/.claude/skills` does not exist. | The command creates the directory at mode 0755. |
| A skill file exists at the target path. | The command keeps the file, prints the path, and exits 0. `--force` writes the file. |
| The operator runs the command under `sudo`. | The command exits 1, because the effective user identifier is 0. The message names the account that owns the skill. |
| The daemon does not run. | `tailnet-exec` reports that `hydrascale status` failed. The routed command still runs, because `exec` needs the namespace rather than the daemon. |
| The configuration file names a tailnet that holds no namespace. | `hydrascale exec` fails with the message of `ip netns exec`. The skill tells the agent to read `hydrascale status` for the state of that tailnet. |
| The agent asks for a command that stops a tailnet. | The setup skill prints the command and the risk. The agent runs no such command. |

## Acceptance criteria

- [ ] `hydrascale switch <id>` states that it changes no state, and it prints the `exec`
      form and the `tailscale` form.
- [ ] `hydrascale env <id>` prints the function `hstn`, and its help holds no `eval`
      example that is followed by a bare command.
- [ ] `skills/tailnet-exec/SKILL.md` and `skills/hydrascale-setup/SKILL.md` exist and hold
      the keys `name` and `description`.
- [ ] `hydrascale skills install` writes two files under a temporary directory that
      `--dir` names.
- [ ] `hydrascale skills install` exits 1 under `sudo`.
- [ ] `hydrascale skills install` keeps a file that exists, and `--force` writes it.
- [ ] The drift test fails when a skill names a command that the command tree does not
      hold.
- [ ] `.claude/skills/tailnet-exec` resolves to `skills/tailnet-exec`.
- [ ] `README.md` states the command `hydrascale skills install`.
- [ ] `gofmt -l .` prints nothing, `go vet ./...` passes, and `go test -race ./...`
      passes.
- [ ] `scripts/check-hygiene.sh` passes.

## Out of scope

- A Model Context Protocol server. The command line interface already holds every form
  that a skill needs, so a server adds a transport and no capability.
- A remote mode that reads a configured host. The skill states the `ssh` form instead.
- A skill for a harness other than a Markdown skill reader.
- A change to the console, to the control socket, or to the reconciler.
- The removal of `hydrascale switch`. Version 1.0 is released, so the command stays and
  its text changes.
