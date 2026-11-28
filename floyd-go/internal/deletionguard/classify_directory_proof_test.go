package deletionguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// shellQuote wraps an absolute fixture path in single quotes so the shell
// grammar reads it as one literal token even when the temporary directory name
// contains characters a bare word would split on.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// TestDeletionDirectoryProofClassification covers how a proven working
// directory changes classification. A cd proves its result only when it names a
// literal existing directory on the left side of an && chain, so that the rest
// of the chain runs only after the cd succeeded: a rename inside that one
// directory is quarantine, while an unproven cd leaves destructive targets
// unresolved.
func TestDeletionDirectoryProofClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		prepare    func(t *testing.T, base string) (workingDir, command string)
		effect     Effect
		targets    []string // nil asserts nothing; an empty slice asserts no targets
		unresolved bool
	}{
		{
			name: "proven cd makes an in-directory rename quarantine",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "rename")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "cd " + shellQuote(dir) + " && mv a b"
			},
			effect:  EffectQuarantine,
			targets: []string{"tree/a"},
		},
		{
			name: "cd into a directory created later in the chain stays destructive",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "created")
				require.NoError(t, os.MkdirAll(work, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(work, "f"), []byte("data"), 0o600))
				fresh := filepath.Join(work, "fresh")
				return work, "mkdir -p " + shellQuote(fresh) + " && cd " + shellQuote(fresh) + " && mv ../f ./m"
			},
			effect:     EffectPermanentDeletion,
			targets:    []string{},
			unresolved: true,
		},
		{
			name: "semicolon-separated cd does not prove the next statement",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "semicolon")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				return work, "cd " + shellQuote(dir) + "; mv a b"
			},
			effect:     EffectPermanentDeletion,
			unresolved: true,
		},
		{
			name: "a later proven cd cannot make an earlier unproven removal look provable",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "masking")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "cd tree; rm -f a; cd " + shellQuote(work) + " && true"
			},
			effect:     EffectPermanentDeletion,
			unresolved: true,
		},
		{
			name: "proven cd names the exact removal target",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "removal")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "cd " + shellQuote(dir) + " && rm -f a"
			},
			effect:  EffectPermanentDeletion,
			targets: []string{"tree/a"},
		},
		{
			name: "proven cd resolves an existing copy destination",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "copy")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				for _, name := range []string{"a", "b"} {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
				}
				return work, "cd " + shellQuote(dir) + " && cp a b"
			},
			effect:  EffectPermanentDeletion,
			targets: []string{"tree/b"},
		},
		{
			name: "proven cd keeps a new redirect file harmless",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "redirect")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				return work, "cd " + shellQuote(dir) + " && echo x > new-file"
			},
			effect:  EffectNone,
			targets: []string{},
		},
		{
			name: "subshell cd does not change the caller directory",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "subshell")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(work, "a"), []byte("data"), 0o600))
				return work, "(cd " + shellQuote(dir) + "); mv a b"
			},
			effect:  EffectQuarantine,
			targets: []string{"a"},
		},
		{
			name: "a group's first statement inherits the proof",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "group")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "cd " + shellQuote(dir) + " && { rm -f a; }"
			},
			effect:  EffectPermanentDeletion,
			targets: []string{"tree/a"},
		},
		{
			name: "a subshell's first statement inherits the proof",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "subshell-inherit")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "cd " + shellQuote(dir) + " && (rm -f a)"
			},
			effect:  EffectPermanentDeletion,
			targets: []string{"tree/a"},
		},
		{
			name: "a subshell may prove its own directory",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "subshell-own")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "(cd " + shellQuote(dir) + " && rm -f a)"
			},
			effect:  EffectPermanentDeletion,
			targets: []string{"tree/a"},
		},
		{
			name: "a later statement in a subshell does not inherit a proof earned there",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "subshell-later")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "(cd " + shellQuote(dir) + "; rm -f a)"
			},
			effect:     EffectPermanentDeletion,
			unresolved: true,
		},
		{
			name: "a negated cd proves nothing",
			prepare: func(t *testing.T, base string) (string, string) {
				work := filepath.Join(base, "negated")
				dir := filepath.Join(work, "tree")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("data"), 0o600))
				return work, "! cd " + shellQuote(dir) + " && rm -f a"
			},
			effect:     EffectPermanentDeletion,
			unresolved: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			work, command := tc.prepare(t, t.TempDir())
			classification := Classify(command, work)
			require.Equal(t, tc.effect, classification.Effect, "%s: %+v", command, classification)
			require.Equal(t, tc.unresolved, classification.Unresolved != "", "%s: %s", command, classification.Unresolved)
			require.Equal(t, tc.effect == EffectPermanentDeletion, classification.IsDestructive(), command)
			if tc.targets != nil {
				expected := make([]string, 0, len(tc.targets))
				for _, name := range tc.targets {
					expected = append(expected, filepath.Join(work, filepath.FromSlash(name)))
				}
				require.Equal(t, CanonicalTargets(expected), classification.Targets, classification.Reason)
			}
		})
	}
}

// TestDeletionDirectoryProofStaysWithinAndChain covers the lifetime of the
// proof: a cd proved for one && chain must not make a later ;-separated
// statement look provable.
func TestDeletionDirectoryProofStaysWithinAndChain(t *testing.T) {
	t.Parallel()
	work := t.TempDir()
	dir := filepath.Join(work, "tree")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	classification := Classify("cd "+shellQuote(dir)+" && rm -f a; rm -f b", work)
	require.True(t, classification.IsDestructive())
	require.NotEmpty(t, classification.Unresolved, "the proof must not outlive its && chain")
}
