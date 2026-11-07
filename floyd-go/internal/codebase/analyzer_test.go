package codebase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestAnalyzeCodebaseAndSearchRespectIgnoredFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fixtureFile(t, root, "src/main.ts", "import helper from './helper';\nimport { tool } from '@scope/pkg/tool';\nconst Needle = tool;")
	fixtureFile(t, root, "main.py", "import os\nfrom requests import get")
	fixtureFile(t, root, "node_modules/pkg/index.ts", "Needle should not appear")
	result, err := AnalyzeCodebase(root, AnalyzeOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, result.TotalFiles)
	require.Equal(t, 5, result.TotalLinesOfCode)
	require.Equal(t, map[string]int{"TypeScript": 1, "Python": 1}, result.FilesByLanguage)
	require.Contains(t, result.Dependencies, DependencyInfo{Name: "@scope/pkg", Type: "import", Source: "src/main.ts", ImportPath: "@scope/pkg/tool", IsExternal: true})
	require.Contains(t, result.Dependencies, DependencyInfo{Name: "os", Type: "import", Source: "main.py", ImportPath: "os", IsExternal: false})
	matches, err := FindFilesByContent(root, "needle", SearchOptions{FileExtensions: []string{"TS"}})
	require.NoError(t, err)
	require.Equal(t, []ContentMatch{{Path: "src/main.ts", Line: 3, Content: "const Needle = tool;"}}, matches)
	matches, err = FindFilesByContent(root, "needle", SearchOptions{CaseSensitive: true})
	require.NoError(t, err)
	require.Empty(t, matches)
	_, err = FindFilesByContent(root, "[", SearchOptions{})
	require.Error(t, err)
	_, err = AnalyzeCodebase(filepath.Join(root, "missing"), AnalyzeOptions{})
	require.ErrorContains(t, err, "Directory does not exist")
}

func TestProjectDependencyManifests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fixtureFile(t, root, "package.json", `{"dependencies":{"react":"^19.0.0"},"devDependencies":{"typescript":"~5.8.0"}}`)
	fixtureFile(t, root, "requirements.txt", "# test fixture\nrequests==2.32.0\npytest\n")
	fixtureFile(t, root, "Gemfile", "source 'https://rubygems.org'\ngem 'rack', '~> 3.0'\ngem 'rake'\n")
	dependencies, err := AnalyzeProjectDependencies(root)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"react": "^19.0.0", "typescript (dev)": "~5.8.0", "requests": "2.32.0", "pytest": "latest", "rack": "~> 3.0", "rake": "latest"}, dependencies)
}
