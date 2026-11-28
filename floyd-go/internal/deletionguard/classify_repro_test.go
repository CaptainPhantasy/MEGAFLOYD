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

// TestMisclassifiedCommandsFromYoloSessionAreNotDestructive holds one repro for
// each command a yolo session could not run because the Bash gate read it as
// permanent deletion although it removes nothing that existed before it. Each
// is the shape that session sent. The two rm commands that session sent are
// removals: they classify as deletion (TestRemovalIsAlwaysClassifiedAsDeletion)
// and reach the tool through the gate, where yolo lets them through.
func TestMisclassifiedCommandsFromYoloSessionAreNotDestructive(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	for _, command := range []string{
		"git -C " + f.repo + " status --short",
		"git -C " + f.repo + " add net.py",
		"cd " + f.repo + " && echo test > b.txt",
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

// TestRemovalIsAlwaysClassifiedAsDeletion pins that an rm removes data with or
// without -f. Neither the force flag nor the permission mode plays any part in
// the classification; the gate decides, per mode, whether to ask about it.
func TestRemovalIsAlwaysClassifiedAsDeletion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tree"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		command   string
		target    string
		recursive bool
	}{
		{"rm file", "file", false},
		{"rm -r tree", "tree", true},
		{"rm -R -v tree", "tree", true},
		{"rm -i file", "file", false},
		{"rm -- file", "file", false},
		{"rm --interactive=never file", "file", false},
		{"rm --recursive tree", "tree", true},
		{"rm -d file", "file", false},
		{"rm -fr tree", "tree", true},
		{"rm -i -f file", "file", false},
		{"rmdir tree", "tree", false},
		{"unlink file", "file", false},
	} {
		t.Run(tc.command, func(t *testing.T) {
			t.Parallel()
			c := Classify(tc.command, dir)
			if !c.IsDestructive() || c.Unresolved != "" {
				t.Fatalf("%q classified as %+v", tc.command, c)
			}
			if got, want := strings.Join(c.Targets, "\n"), filepath.Join(dir, tc.target); got != want {
				t.Errorf("%q targets %q, want %q", tc.command, got, want)
			}
			if c.Recursive != tc.recursive {
				t.Errorf("%q recursive %v, want %v", tc.command, c.Recursive, tc.recursive)
			}
		})
	}
	// A removal whose targets cannot be proven fails closed.
	for _, command := range []string{`rm "$target"`, `rm -r "$target"`, `ls | xargs rm`, `find . -exec rm {} +`} {
		if c := Classify(command, dir); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
}

// TestGitOptionsNeedABuiltInSubcommand covers git with options before the
// subcommand. Only a built-in subcommand may follow them: an option can define
// an alias that replaces any other name, and a few -c settings can run another
// program or pull in more configuration.
func TestGitOptionsNeedABuiltInSubcommand(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	for _, command := range []string{
		// Not a built-in, so on some git it may be an alias.
		"git -C " + f.repo + " erase",
		"git -c alias.erase='!rm -f important.txt' -C " + f.repo + " erase",
		// An option that is not modeled.
		"git --exec-path /elsewhere status",
		"git --exec-path=/elsewhere status",
		"git --unknown " + f.repo + " status",
		// A -c or --config-env setting that can dispatch or include.
		"git -c alias.status=cat status",
		"git -c ALIAS.status=cat status",
		"git -c include.path=/elsewhere/gitconfig status",
		"git -c includeIf.gitdir:/elsewhere/.path=/elsewhere/gitconfig status",
		"git -c core.hooksPath=/elsewhere add net.py",
		"git -c core.fsmonitor=/elsewhere/hook status",
		"git -c core.hookspath=/elsewhere status",
		"git --config-env=core.hooksPath=SETTING status",
		"git --config-env=alias.status=SETTING status",
		// Git ignores whitespace around a key.
		"git -c 'core.hooksPath =/elsewhere' commit",
		"git -c ' alias.status =cat' status",
		// A setting that cannot be read.
		`git -c "$setting" add net.py`,
		`git --config-env="$setting" status`,
		"git -c '' add net.py",
	} {
		if c := Classify(command, f.base); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
	for _, command := range []string{
		"git -c core.pager=cat status",
		"git -c user.name=x -C " + f.repo + " log",
		"git --config-env=user.email=SETTING -C " + f.repo + " commit",
	} {
		if c := Classify(command, f.base); c.IsDestructive() {
			t.Errorf("%q classified as destructive: %q", command, c.Unresolved)
		}
	}
}

// TestUnprovenDirectoryChangesStayGated covers the directory changes a cd proof
// must not cover: a negated cd, pushd and popd, an evaluated script, a
// pipeline and a background statement.
func TestUnprovenDirectoryChangesStayGated(t *testing.T) {
	t.Parallel()
	f := newGapFixture(t)
	for _, command := range []string{
		"! cd " + f.repo + " && rm -f a",
		"pushd " + f.repo + " && rm -f a",
		"popd && rm -f a",
		"command pushd " + f.repo + " && rm -f a",
		"eval 'cd() { true; }'; cd " + f.repo + " && rm -f a",
		"builtin eval 'cd " + f.repo + "'; rm -f a",
		`eval "$script" && rm -f a`,
		"cd " + f.repo + " | cat && rm -f a",
		"cd " + f.repo + " & rm -f a",
	} {
		if c := Classify(command, f.base); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
}

// TestDirectoryProofInGroupsAndSubshells covers where a proven directory
// carries. The first statement of a group or subshell runs in the directory it
// was entered with, so it inherits that proof, and a subshell may prove its own.
// A later statement in the same list does not inherit: it may run after an
// earlier cd failed.
func TestDirectoryProofInGroupsAndSubshells(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	tree := filepath.Join(base, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(tree, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ command, targets string }{
		{"cd tree && rm -f a", "a"},
		{"cd tree && { rm -f a; }", "a"},
		{"cd tree && (rm -f a)", "a"},
		{"(cd tree && rm -f a)", "a"},
		{"cd tree && { echo x; rm -f a; rm -f b; }", "a b"},
		{"cd tree && (echo x; rm -f a)", "a"},
		{`echo "$(cd tree && rm -f a)"`, "a"},
	} {
		c := Classify(tc.command, base)
		if !c.IsDestructive() || c.Unresolved != "" {
			t.Errorf("%q classified as %+v", tc.command, c)
			continue
		}
		var want []string
		for _, name := range strings.Fields(tc.targets) {
			want = append(want, filepath.Join(tree, name))
		}
		if got := strings.Join(c.Targets, "\n"); got != strings.Join(want, "\n") {
			t.Errorf("%q targets %q, want %q", tc.command, got, want)
		}
	}
	for _, command := range []string{
		"cd tree; { rm -f a; }",
		"cd tree; (rm -f a)",
		"(cd tree; rm -f a)",
		"cd tree && { cd sub; rm -f a; }",
		"cd tree && (cd sub; rm -f a)",
		"cd tree || (rm -f a)",
		"if true; then (cd tree && rm -f a); fi",
	} {
		if c := Classify(command, base); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
}

// TestRedirectOntoAnEarlierMoveIsUnresolved: a file moved earlier in the
// command is not there yet when the command is read, so a redirect onto it
// looks harmless and would destroy the only copy.
func TestRedirectOntoAnEarlierMoveIsUnresolved(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "stash"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "valuable.txt"), []byte("only copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"mv valuable.txt stash/ && cd stash && printf replacement > valuable.txt",
		"mv valuable.txt stash/ && echo x > stash/valuable.txt",
	} {
		if c := Classify(command, base); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
	if c := Classify("mv valuable.txt stash/ && cd stash && printf replacement > other.txt", base); c.IsDestructive() {
		t.Errorf("a redirect beside the moved file classified as destructive: %q", c.Unresolved)
	}
}

// TestSymlinkDestinations covers a move or copy whose destination is a
// symbolic link. A link the same command creates does not exist while the
// command is read, so the classifier cannot see the file behind it. It follows
// such a link only when ln gave a literal target. Any other link leaves the
// real destination unknown.
func TestSymlinkDestinations(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	tree, dest := filepath.Join(base, "tree"), filepath.Join(base, "dest")
	for _, name := range []string{tree, dest} {
		if err := os.MkdirAll(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "a"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	behind := filepath.Join(dest, "a")
	created := "cd tree && ln -s ../dest link && mv ../a link/"

	// A file already behind the link would be overwritten.
	if err := os.WriteFile(behind, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Classify(created, base)
	if !c.IsDestructive() || c.Unresolved != "" {
		t.Fatalf("%q classified as %+v", created, c)
	}
	if got := strings.Join(c.Targets, "\n"); got != behind {
		t.Errorf("%q targets %q, want the real file %q", created, got, behind)
	}

	// With nothing behind it the same move is quarantine, to the real place.
	if err := os.Remove(behind); err != nil {
		t.Fatal(err)
	}
	if c := Classify(created, base); c.IsDestructive() || c.Effect != EffectQuarantine {
		t.Errorf("%q classified as %+v", created, c)
	}

	// A link that was already there, or one ln created from a target that is
	// not literal, is not followed.
	if err := os.Symlink("dest", filepath.Join(tree, "existing")); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"cd tree && mv ../a existing/",
		"cd tree && mv -T ../a existing",
		"cd tree && cp ../a existing/",
		"cd tree && cp -T ../a existing",
		`cd tree && ln -s "$target" link && mv ../a link/`,
		`cd tree && ln -s ../dest "$name" && mv ../a link/`,
		"cd tree && ln -s -t . ../dest link && mv ../a link/",
	} {
		if c := Classify(command, base); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
	// Naming the real path is exact.
	if c := Classify("mv a dest/", base); c.IsDestructive() || c.Effect != EffectQuarantine {
		t.Errorf("%q classified as %+v", "mv a dest/", c)
	}
}

// TestCreatedDirectoryNeedsTheCommandsOwnScope: a mkdir inside a subshell, a
// branch or a nested script does not vouch for a later statement.
func TestCreatedDirectoryNeedsTheCommandsOwnScope(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(base, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range []string{
		"(mkdir -p fresh) && mv a b fresh/",
		"if true; then mkdir -p fresh; fi; mv a b fresh/",
		"sh -c 'mkdir -p fresh' && mv a b fresh/",
		"eval 'mkdir -p fresh' && mv a b fresh/",
		"mkdir -p fresh & mv a b fresh/",
	} {
		if c := Classify(command, base); !c.IsDestructive() || c.Unresolved == "" {
			t.Errorf("%q classified as %+v", command, c)
		}
	}
	if c := Classify("mkdir -p fresh && mv a b fresh/", base); c.IsDestructive() || c.Effect != EffectQuarantine {
		t.Errorf("%q classified as %+v", "mkdir -p fresh && mv a b fresh/", c)
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
			t.Parallel()
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
		"git --work-tree=" + f.repo + " status",
		"git -C " + f.repo + " --version",
	} {
		if c := Classify(command, f.base); c.IsDestructive() {
			t.Errorf("%q classified as destructive: %q", command, c.Unresolved)
		}
	}
}
