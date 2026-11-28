package deletionguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInnocentCommandsAreNotDestructive captures commands a live harness
// session reported as "Permanent deletion is blocked" even though they remove
// nothing. They must parse as non-destructive.
func TestInnocentCommandsAreNotDestructive(t *testing.T) {
	commands := []string{
		"for t in python3 git jq rsync node; do printf \"%s: \" \"$t\"; command -v $t 2>/dev/null || echo MISSING; done; echo \"--- gh candidates ---\"; ls -l /opt/homebrew/bin/gh /usr/local/bin/gh /usr/bin/gh 2>/dev/null || echo \"no gh binary found\"",
		"echo \"== policy files ==\"; for f in MegaFloyd.md megafloyd.md MEGAFLOYD.md AGENTS.md CLAUDE.md FLOYD.md; do [ -f \"$f\" ] && echo \"FOUND $f ($(wc -c <\"$f\") bytes)\" || echo \"absent $f\"; done; echo; echo \"== docs ==\"; ls docs 2>/dev/null | head -30 || echo \"no docs dir\"; echo; echo \"== ssot search ==\"; ls docs/*SSOT* *SSOT* 2>/dev/null || echo \"no SSOT at root/docs\"; echo; echo \"== top-level counts ==\"; git ls-files | wc -l; echo \"tracked files above\"",
		"echo \"== TOOLS ==\"; for t in python3 git gh jq rsync; do printf \"%s: \" \"$t\"; command -v $t || echo MISSING; done; echo; echo \"== GITEA LOCAL PROBE (python, no curl) ==\"; python3 - <<'PY'\nimport json,urllib.request\nfor url in [\"http://localhost:3000/api/v1/version\",\"http://127.0.0.1:3000/api/v1/version\"]:\n    try:\n        with urllib.request.urlopen(url, timeout=3) as r:\n            print(url, \"->\", r.status, r.read(120).decode(\"utf-8\",\"replace\"))\n    except Exception as e:\n        print(url, \"-> ERROR\", type(e).__name__, str(e)[:120])\nPY",
		`[ -f "$f" ]`,
		`command -v $t`,
		`command -V $t`,
		`command -pv $t`,
		`for t in a b; do command -v $t; done`,
	}
	for i, command := range commands {
		c := Classify(command, "/tmp")
		if c.IsDestructive() {
			t.Errorf("command %d classified destructive: %q", i, c.Unresolved)
		}
	}
}

// TestPotentialDispatchStaysDestructive pins the fail-closed cases the two
// fixes above must not weaken.
func TestPotentialDispatchStaysDestructive(t *testing.T) {
	commands := []string{
		`command $t`,
		`command -p $t`,
		`builtin $t`,
		`exec $t`,
		`rm -rf "$dir"`,
		`rm -f a`,
		`rm "$target"`,
		`rm -r "$target"`,
	}
	for i, command := range commands {
		c := Classify(command, "/tmp")
		if !c.IsDestructive() {
			t.Errorf("command %d (%q) lost its destructive classification", i, command)
		}
	}
}

// gapFixture lays out the files a yolo session was using when the Bash gate
// blocked it, under the names that session used.
type gapFixture struct {
	base, repo, verdicts, verdicts2, gitTest, quarantine string
}

func newGapFixture(t *testing.T) gapFixture {
	t.Helper()
	base := t.TempDir()
	f := gapFixture{
		base:       base,
		repo:       filepath.Join(base, "gaptest-repo"),
		verdicts:   filepath.Join(base, "gap_verdicts.py"),
		verdicts2:  filepath.Join(base, "gap_verdicts2.py"),
		gitTest:    filepath.Join(base, "gap_git_test.py"),
		quarantine: filepath.Join(base, "quarantine-gap-fixtures"),
	}
	if err := os.MkdirAll(filepath.Join(f.repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(f.repo, "net.py"), f.verdicts, f.verdicts2, f.gitTest} {
		if err := os.WriteFile(name, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// TestBlockedCommandsFromYoloSessionAreNotDestructive holds one repro for each
// command a yolo session could not run because the Bash gate read it as
// permanent deletion. Each is the shape that session sent.
func TestBlockedCommandsFromYoloSessionAreNotDestructive(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	for _, command := range []string{
		"git -C " + f.repo + " status --short",
		"git -C " + f.repo + " add net.py",
		"cd " + f.repo + " && echo test > b.txt",
		"rm " + f.verdicts,
		"rm -r " + f.repo,
	} {
		c := Classify(command, f.base)
		if c.IsDestructive() {
			t.Errorf("%q classified as permanent deletion: %q", command, c.Unresolved)
		}
		if c.Effect != EffectNone {
			t.Errorf("%q has effect %s, want none", command, c.Effect)
		}
	}
}

// TestMoveIntoDirectoryCreatedBySameCommandIsQuarantine is the repro for the
// quarantine move that session could not run: the destination directory did not
// exist while the command was being read, because the same command creates it.
func TestMoveIntoDirectoryCreatedBySameCommandIsQuarantine(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	command := "mkdir -p " + f.quarantine + " && mv " + f.repo + " " + f.verdicts + " " +
		f.verdicts2 + " " + f.gitTest + " " + f.quarantine + "/ 2>/dev/null; ls " + f.base +
		" | grep -i gap; echo moved"
	c := Classify(command, f.base)
	if c.IsDestructive() {
		t.Fatalf("quarantine move classified as permanent deletion: %q", c.Unresolved)
	}
	if c.Effect != EffectQuarantine {
		t.Fatalf("effect %s, want quarantine", c.Effect)
	}
	want := CanonicalTargets([]string{f.repo, f.verdicts, f.verdicts2, f.gitTest})
	if strings.Join(c.Targets, "\n") != strings.Join(want, "\n") {
		t.Errorf("targets %v, want %v", c.Targets, want)
	}
}

// TestUnforcedRemovalIsNotDestructive covers the rm forms the permission mode
// alone governs.
func TestUnforcedRemovalIsNotDestructive(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`rm a`,
		`rm a b`,
		`rm -r a`,
		`rm -R a`,
		`rm -i a`,
		`rm -v a`,
		`rm -rv a b`,
		`rm --recursive a`,
		`rm --interactive=always a`,
		`rm -- a`,
		`sudo rm a`,
		`rm a; rm -r b`,
	} {
		if c := Classify(command, "/tmp"); c.IsDestructive() {
			t.Errorf("%q classified as permanent deletion: %q", command, c.Unresolved)
		}
	}
}

// TestLoosenedClassificationsKeepTheirBoundaries pins what the repros above
// must not admit: each command destroys data that existed before it.
func TestLoosenedClassificationsKeepTheirBoundaries(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	for _, tc := range []struct{ name, command, target string }{
		{"git clean behind options", "git -C " + f.repo + " clean -fd", ""},
		{"git reset behind options", "git -C " + f.repo + " reset --hard", ""},
		{"git rm behind options", "git -C " + f.repo + " rm net.py", ""},
		{"git branch delete behind options", "git -C " + f.repo + " branch -D main", ""},
		{"unknown git option", "git --unknown " + f.repo + " status", ""},
		{"nonliteral git subcommand", `git -C "$repo" "$command"`, ""},
		{"cd then truncate an existing file", "cd " + f.repo + " && echo test > net.py", filepath.Join(f.repo, "net.py")},
		{"rm -f", "rm -f " + f.verdicts, f.verdicts},
		{"rm -rf", "rm -rf " + f.repo, f.repo},
		{"rm --force", "rm --force " + f.verdicts, f.verdicts},
		{"rm with an abbreviated force", "rm --for " + f.verdicts, f.verdicts},
		{"rm with an unknown flag", "rm -x " + f.verdicts, f.verdicts},
		{"rm with a nonliteral target", `rm "$target"`, ""},
		{"rm -r with a nonliteral target", `rm -r "$target"`, ""},
		{"mkdir in the background", "mkdir -p " + f.quarantine + " & mv a b c " + f.quarantine + "/", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := Classify(tc.command, f.base)
			if !c.IsDestructive() {
				t.Fatalf("%q lost its destructive classification", tc.command)
			}
			if tc.target != "" && strings.Join(c.Targets, "\n") != tc.target {
				t.Errorf("targets %v, want %q", c.Targets, tc.target)
			}
		})
	}
}

// TestMoveOverExistingQuarantineCopyStaysDestructive pins that creating the
// destination does not excuse replacing what is already inside it.
func TestMoveOverExistingQuarantineCopyStaysDestructive(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	existing := filepath.Join(f.quarantine, "gaptest-repo")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	c := Classify("mkdir -p "+f.quarantine+" && mv "+f.repo+" "+f.quarantine+"/", f.base)
	if !c.IsDestructive() {
		t.Fatal("a move that replaces an existing quarantine copy was not classified destructive")
	}
	if strings.Join(c.Targets, "\n") != existing {
		t.Errorf("targets %v, want %q", c.Targets, existing)
	}
}

// TestCreatedDirectoryIsUsableForTheRestOfTheCommand covers the other shapes
// of a move or copy into a directory the command creates.
func TestCreatedDirectoryIsUsableForTheRestOfTheCommand(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	fresh := filepath.Join(base, "fresh")
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(base, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	quarantine := EffectQuarantine
	for _, tc := range []struct {
		name, command string
		effect        Effect
	}{
		{"separated by a semicolon", "mkdir -p " + fresh + "; mv a b " + fresh + "/", quarantine},
		{"nested parents", "mkdir -p " + filepath.Join(fresh, "sub") + " && mv a " + filepath.Join(fresh, "sub") + "/", quarantine},
		{"target option", "mkdir -p " + fresh + " && mv -t " + fresh + " a b", quarantine},
		{"copy", "mkdir -p " + fresh + " && cp a b " + fresh + "/", EffectNone},
		{"relative to a proven cd", "cd " + base + " && mkdir -p fresh && mv a b fresh/", quarantine},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Classify(tc.command, base)
			if c.IsDestructive() {
				t.Fatalf("%q classified as permanent deletion: %q", tc.command, c.Unresolved)
			}
			if c.Effect != tc.effect {
				t.Errorf("%q has effect %s, want %s", tc.command, c.Effect, tc.effect)
			}
		})
	}
}

// TestGitOptionsBeforeSubcommand covers the read-only forms that reach git
// with options first.
func TestGitOptionsBeforeSubcommand(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	for _, command := range []string{
		"git -C " + f.repo + " diff",
		"git -C " + f.repo + " init",
		"git -C " + f.repo + " log --oneline -5",
		"git -C " + f.repo + " checkout -b topic",
		"git -C " + f.repo + " switch -c topic",
		"git -C " + f.repo + " branch",
		"git --no-pager -C " + f.repo + " diff",
		"git -c core.pager=cat status",
		`git -C "$repo" status --short`,
		`git -c "$setting" add net.py`,
		"git --work-tree=" + f.repo + " status",
		"git -C " + f.repo + " --version",
	} {
		if c := Classify(command, f.base); c.IsDestructive() {
			t.Errorf("%q classified as destructive: %q", command, c.Unresolved)
		}
	}
}
