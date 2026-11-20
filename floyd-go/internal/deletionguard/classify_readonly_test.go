package deletionguard

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadOnlyGitAndFindCommandsAreNotDestructive covers the commands the live
// worker measured as blocked while doing ordinary doctoring work.
func TestReadOnlyGitAndFindCommandsAreNotDestructive(t *testing.T) {
	commands := []string{
		`git checkout -b doctor/x`,
		`git checkout main`,
		`git switch main`,
		`git switch -c doctor/x`,
		`git stash show stash@{0} --stat`,
		`git stash list --format="%H %s"`,
		`find . -maxdepth 2 -newermt "2026-11-18 00:00" -not -path "./node_modules/*" | head -40`,
		`find . -name "*.md"`,
		`find . -name '*.md'`,
		`git diff HEAD~1 HEAD`,
		`git branch doctor/x`,
		`git branch`,
		`git log --oneline -5`,
		`git status --porcelain=v1`,
		`git show HEAD:README.md`,
		`git rev-parse HEAD`,
		`git ls-files | wc -l`,
		`git remote -v`,
	}
	for _, command := range commands {
		c := Classify(command, "/tmp")
		if c.IsDestructive() {
			t.Errorf("%q classified destructive: %q", command, c.Unresolved)
		}
	}
}

// TestDestructiveGitCommandsStayBlocked pins the fail-closed cases the
// read-only allowance must not weaken.
func TestDestructiveGitCommandsStayBlocked(t *testing.T) {
	commands := []string{
		`git checkout -- file.txt`,
		`git checkout .`,
		`git checkout -f main`,
		`git checkout -B main`,
		`git checkout HEAD~1 file.txt`,
		`git restore file.txt`,
		`git restore --worktree file.txt`,
		`git switch --force main`,
		`git branch -D main`,
		`git stash drop stash@{0}`,
		`git stash clear`,
		`git clean -fd`,
		`git reset --hard`,
		`git rm file.txt`,
	}
	for _, command := range commands {
		c := Classify(command, "/tmp")
		if !c.IsDestructive() {
			t.Errorf("%q lost its destructive classification", command)
		}
	}
}

func TestRedirectToNewFileIsNotDestructive(t *testing.T) {
	dir := t.TempDir()
	command := "cat > newfile <<'EOF'\nhello\nEOF"
	if c := Classify(command, dir); c.IsDestructive() {
		t.Errorf("redirect to a new file classified destructive: %q (%q)", c.Unresolved, c.Reason)
	}
}

func TestRedirectTruncatingExistingFileIsDestructive(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(target, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := "cat > existing.txt <<'EOF'\nhello\nEOF"
	if c := Classify(command, dir); !c.IsDestructive() {
		t.Errorf("redirect truncating existing content was not classified destructive")
	}
}
