package extensibility

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmartIndexPrefersYAMLAndHonorsDisabledEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	yaml := `version: 1
skills:
  - name: review
    path: " review.md "
    tags: [code, safety]
  - name: disabled
    path: missing.md
    enabled: false
agents:
  - name: writer
    path: writer.md
plugins:
  - name: tools
    path: tools.md
`
	require.NoError(t, os.WriteFile(filepath.Join(root, YAMLSkillsFilename), []byte(yaml), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "review.md"), []byte("Review carefully."), 0o600))
	entries, err := LoadIndexSmart(filepath.Join(root, "missing-index.md"))
	require.NoError(t, err)
	require.Len(t, entries, 3)
	require.Equal(t, IndexEntry{Name: "review", Path: "review.md", Category: "skill", Tags: []string{"code", "safety"}}, entries[0])
	require.Equal(t, "agent", entries[1].Category)
	require.Equal(t, "plugin", entries[2].Category)
	require.Equal(t, []IndexEntry{entries[0]}, GetEntriesByCategory(entries, "SKILL"))
	entries[0].Path = filepath.Join(root, entries[0].Path)
	content, err := LoadItem(entries[0])
	require.NoError(t, err)
	require.Equal(t, "Review carefully.", content)
	_, err = LoadItem(IndexEntry{Path: filepath.Join(root, "absent.md")})
	require.Error(t, err)
}

func TestSmartIndexRejectsInvalidYAMLAndFallsBackOnlyWhenMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "index.md")
	require.NoError(t, os.WriteFile(path, []byte("## 📍 SYSTEM SKILLS\n#### inspect\n- Purpose: Inspect source\n- Path: `inspect.md`\n"), 0o600))
	entries, err := LoadIndexSmart(path)
	require.NoError(t, err)
	require.Equal(t, []IndexEntry{{Name: "inspect", Description: "Inspect source", Path: "inspect.md", Category: "skill"}}, entries)
	for _, content := range []string{"version: 999\n", "skills: [unterminated"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, YAMLSkillsFilename), []byte(content), 0o600))
		_, err = LoadIndexSmart(path)
		require.Error(t, err)
	}
}
