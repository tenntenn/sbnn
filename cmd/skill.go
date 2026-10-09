package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/skills"
	"github.com/tenntenn/sbnn/version"
)

var (
	skillDir   string
	skillForce bool
	skillList  bool

	skillRefresh string
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Print or install the agent skill for sbnn",
	Long: `Print or install the agent skill bundled with sbnn.

The skill is vendor neutral: it is a plain Markdown file with YAML front
matter that describes the sbnn review workflow in terms of the sbnn command line,
so any coding agent can use it. Install it where your agent looks for skills,
or point at it from AGENTS.md.

  $ sbnn skill                                   # print SKILL.md
  $ sbnn skill --list                            # list the files of the skill
  $ sbnn skill --install .claude/skills          # this project only
  $ sbnn skill --install ~/.claude/skills        # all your projects
  $ sbnn skill --install .agents/skills          # then link it from AGENTS.md
  $ sbnn skill --install ~/.claude/skills --force  # replace an older copy
  $ sbnn skill --refresh ~/.claude/skills        # replace it only if this sbnn is newer

--install writes the skill as a "sbnn" directory inside the directory you name
(<dir>/sbnn/SKILL.md) and refuses to overwrite an existing file without --force.

--refresh is the safe form of --force: it compares the release recorded in the
installed SKILL.md with this sbnn's, reinstalls only when this sbnn is newer (or
the installed copy records no release), and leaves a newer skill alone, saying
that sbnn should be upgraded. A source build ("dev") never overwrites a skill
that records a release. Every skill sbnn prints or installs records the release
of the sbnn that wrote it.

For an agent that reads AGENTS.md instead of a skills directory, install the
file anywhere and point at it:

  ## Reviewing changes

  Before showing a diff to a human, read .agents/skills/sbnn/SKILL.md.

An agent with no skill support at all can be told about sbnn directly:

  $ sbnn skill >> AGENTS.md`,
	Args:         cobra.NoArgs,
	RunE:         runSkill,
	SilenceUsage: true,
}

func init() {
	f := skillCmd.Flags()
	f.StringVar(&skillDir, "install", "", "Directory to install the skill into")
	f.BoolVar(&skillForce, "force", false, "Overwrite existing files")
	f.BoolVar(&skillList, "list", false, "List the files of the skill")
	f.StringVar(&skillRefresh, "refresh", "", "Directory holding an installed skill to update, unless it is newer than this sbnn")
	skillCmd.MarkFlagsMutuallyExclusive("refresh", "install", "list", "force")
}

func runSkill(_ *cobra.Command, _ []string) error {
	switch {
	case skillList:
		return fs.WalkDir(skills.FS(), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			fmt.Println(path)
			return nil
		})
	case skillRefresh != "":
		return refreshSkill(skillRefresh, version.Version, os.Stdout)
	case skillDir != "":
		return installSkill(skillDir, skillForce, version.Version)
	default:
		md, err := skills.Markdown()
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(skills.Stamp(md, version.Version))
		return err
	}
}

// installSkill copies the embedded skill into dir, keeping its directory
// name so that it lands as <dir>/sbnn/SKILL.md. Every file but SKILL.md is
// copied as it is; SKILL.md records ver.
func installSkill(dir string, force bool, ver string) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	written := 0
	err = fs.WalkDir(skills.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(root, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !force {
			if _, err := os.Stat(dst); err == nil {
				return fmt.Errorf("%s already exists (pass --force to overwrite)", dst)
			}
		}
		b, err := fs.ReadFile(skills.FS(), path)
		if err != nil {
			return err
		}
		if path == skills.Name+"/SKILL.md" {
			b = skills.Stamp(b, ver)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "sbnn: wrote", dst)
		written++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sbnn: installed the sbnn skill (%d file(s)) into %s\n", written, root)
	return nil
}

// refreshSkill brings the skill installed under dir up to date with this
// binary, unless the installed one comes from a newer sbnn. The decision is
// skills.Decide's; what happened goes to w.
func refreshSkill(dir, ver string, w io.Writer) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(root, skills.Name, "SKILL.md")
	installed, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w (install it with: sbnn skill --install %s)", err, dir)
	}
	md, err := skills.Markdown()
	if err != nil {
		return err
	}
	switch skills.Decide(installed, skills.Stamp(md, ver), ver) {
	case skills.Newer:
		fmt.Fprintf(w, "the installed skill (%s) is newer than this sbnn (%s) and was left as it is; upgrade sbnn: go install github.com/tenntenn/sbnn@latest\n",
			describeVersion(skills.Version(installed)), describeVersion(ver))
		return nil
	case skills.Current:
		fmt.Fprintf(w, "the installed skill is up to date (%s)\n", describeVersion(ver))
		return nil
	}
	if err := installSkill(dir, true, ver); err != nil {
		return err
	}
	fmt.Fprintf(w, "refreshed the installed skill (%s -> %s); read %s again\n",
		describeVersion(skills.Version(installed)), describeVersion(ver), path)
	return nil
}

func describeVersion(v string) string {
	if v == "" || v == "dev" {
		return "no release"
	}
	return "v" + v
}
