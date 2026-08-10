package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"hydrascale/skills"
)

// skillsGeteuid returns the effective user identifier of the process. A test replaces it,
// because a test process cannot change its own identifier.
var skillsGeteuid = os.Geteuid

// skillDirectoryMode is the mode of $HOME/.claude/skills and of each directory under it.
const skillDirectoryMode fs.FileMode = 0o755

// skillFileMode is the mode of a SKILL.md that the command writes.
const skillFileMode fs.FileMode = 0o644

func skillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage the skills that a coding agent loads",
		Long: `Manage the skills that a coding agent loads.

A skill is one Markdown file that states how the agent performs one task. The binary holds
the skill set, so a host needs no checkout of the Hydrascale repository.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(skillsInstallCmd())
	cmd.AddCommand(skillsListCmd())
	return cmd
}

func skillsInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write the embedded skills to the skill directory of the operator",
		Long: `Write each embedded skill to $HOME/.claude/skills/<name>/SKILL.md.

The command writes under the home directory of the invoking account and under no other
path. Run it as the account that runs the coding agent. Run it without sudo.

The command keeps a file that exists. Pass --force to write that file.`,
		Args: cobra.NoArgs,
		// main prints the error and exits 1, so cobra prints neither the error nor the
		// usage text. A second copy of the message hides the first.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := cmd.Flags().GetString("dir")
			if err != nil {
				return err
			}
			force, err := cmd.Flags().GetBool("force")
			if err != nil {
				return err
			}
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return err
			}
			return installSkills(cmd.OutOrStdout(), dir, force, dryRun)
		},
	}
	cmd.Flags().String("dir", "", "Write to this directory rather than to $HOME/.claude/skills")
	cmd.Flags().Bool("force", false, "Write a file that exists")
	cmd.Flags().Bool("dry-run", false, "Print each path and write no file")
	return cmd
}

func skillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "Print the name and the description of each embedded skill",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := skills.All()
			if err != nil {
				return err
			}
			for _, skill := range set {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", skill.Name, skill.Description)
			}
			return nil
		},
	}
}

// installSkills writes each embedded skill to the skill directory of the operator.
// installSkills prints one line for each path to out. dir names the target directory, and
// an empty dir selects $HOME/.claude/skills. force writes a file that exists. dryRun
// prints each path and writes no file.
//
// installSkills returns an error in each of these cases:
//   - The effective user identifier is 0.
//   - dir is empty and the environment holds no HOME.
//   - A write fails.
//
// installSkills attempts every skill and returns the errors together.
func installSkills(out io.Writer, dir string, force, dryRun bool) error {
	// The coding agent runs as the operator and reads the skill directory of the operator,
	// so a write as root would place the skill where the agent never looks.
	if skillsGeteuid() == 0 {
		return fmt.Errorf(
			"the effective user identifier is 0, and the skill belongs to the account of the operator rather than to root. Run \"hydrascale skills install\" as %s. Run it without sudo",
			operatorAccount())
	}

	base := dir
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return errors.New(
				"the environment holds no HOME, so the command cannot find the skill directory. Set HOME, or name a directory with --dir")
		}
		base = filepath.Join(home, ".claude", "skills")
	}

	set, err := skills.All()
	if err != nil {
		return err
	}

	if !dryRun {
		if err := makeSkillDirectory(base); err != nil {
			return err
		}
	}

	var errs []error
	for _, skill := range set {
		directory := filepath.Join(base, skill.Name)
		path := filepath.Join(directory, "SKILL.md")

		if _, statErr := os.Stat(path); statErr == nil {
			if !force {
				fmt.Fprintf(out, "kept %s\n", path)
				continue
			}
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("read %s: %w", path, statErr))
			continue
		}

		if dryRun {
			fmt.Fprintf(out, "[dry-run] wrote %s\n", path)
			continue
		}
		if err := makeSkillDirectory(directory); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.WriteFile(path, skill.Content, skillFileMode); err != nil {
			errs = append(errs, fmt.Errorf("write %s: %w", path, err))
			continue
		}
		fmt.Fprintf(out, "wrote %s\n", path)
	}
	return errors.Join(errs...)
}

// makeSkillDirectory creates the directory at skillDirectoryMode when the directory is
// absent. makeSkillDirectory keeps the mode of a directory that exists, because that
// directory belongs to the operator. makeSkillDirectory returns an error when the create
// fails.
func makeSkillDirectory(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.MkdirAll(path, skillDirectoryMode); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	// MkdirAll subtracts the umask of the process from the mode, so the command sets the
	// mode again. The skill directory holds skillDirectoryMode.
	if err := os.Chmod(path, skillDirectoryMode); err != nil {
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	return nil
}

// operatorAccount returns the name of the account that owns the skill. operatorAccount
// reads SUDO_USER, which sudo sets to the account that called it. operatorAccount returns
// the phrase "the account of the operator" when the environment holds no SUDO_USER.
func operatorAccount() string {
	if account := os.Getenv("SUDO_USER"); account != "" {
		return account
	}
	return "the account of the operator"
}
