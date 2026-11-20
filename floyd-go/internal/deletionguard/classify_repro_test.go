package deletionguard

import "testing"

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
		`rm a`,
	}
	for i, command := range commands {
		c := Classify(command, "/tmp")
		if !c.IsDestructive() {
			t.Errorf("command %d (%q) lost its destructive classification", i, command)
		}
	}
}
