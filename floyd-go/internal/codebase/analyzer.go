package codebase

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	ferrors "github.com/legacy-ai/floyd/internal/errors"
	"github.com/legacy-ai/floyd/internal/fsops"
)

type FileInfo struct {
	Path         string
	Extension    string
	Language     string
	Size         int64
	LineCount    int
	LastModified time.Time
}

type DependencyInfo struct {
	Name       string
	Type       string
	Source     string
	ImportPath string
	IsExternal bool
}

type ProjectStructure struct {
	Root             string
	TotalFiles       int
	FilesByLanguage  map[string]int
	TotalLinesOfCode int
	Directories      map[string][]string
	Dependencies     []DependencyInfo
}

type AnalyzeOptions struct {
	IgnorePatterns []string
	MaxFiles       int
	MaxSizePerFile int64
}

var defaultIgnorePatterns = []string{
	"node_modules",
	"dist",
	"build",
	".git",
	".floyd",
	".vscode",
	".idea",
	"coverage",
	"*.min.js",
	"*.bundle.js",
	"*.map",
}

var extensionToLanguage = map[string]string{
	"ts":    "TypeScript",
	"tsx":   "TypeScript (React)",
	"js":    "JavaScript",
	"jsx":   "JavaScript (React)",
	"py":    "Python",
	"java":  "Java",
	"c":     "C",
	"cpp":   "C++",
	"cs":    "C#",
	"go":    "Go",
	"rs":    "Rust",
	"php":   "PHP",
	"rb":    "Ruby",
	"swift": "Swift",
	"kt":    "Kotlin",
	"scala": "Scala",
	"html":  "HTML",
	"css":   "CSS",
	"scss":  "SCSS",
	"less":  "Less",
	"json":  "JSON",
	"md":    "Markdown",
	"yml":   "YAML",
	"yaml":  "YAML",
	"xml":   "XML",
	"sql":   "SQL",
	"sh":    "Shell",
	"bat":   "Batch",
	"ps1":   "PowerShell",
}

func AnalyzeCodebase(directory string, options AnalyzeOptions) (*ProjectStructure, error) {
	ignorePatterns := options.IgnorePatterns
	if len(ignorePatterns) == 0 {
		ignorePatterns = defaultIgnorePatterns
	}
	maxFiles := options.MaxFiles
	if maxFiles == 0 {
		maxFiles = 1000
	}
	maxSize := options.MaxSizePerFile
	if maxSize == 0 {
		maxSize = 1024 * 1024
	}

	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Directory does not exist: %s", directory),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileNotFound,
				Resolution: []string{
					"Please provide a valid directory path.",
				},
			},
		)
	}

	structure := &ProjectStructure{
		Root:            directory,
		FilesByLanguage: map[string]int{},
		Directories:     map[string][]string{},
		Dependencies:    []DependencyInfo{},
	}

	ignoreRegexes := compileIgnorePatterns(ignorePatterns)
	allFiles, err := fsops.FindFiles(directory, fsops.FindOptions{Recursive: true, IncludeDirectories: false})
	if err != nil {
		return nil, ferrors.CreateUserError(
			fmt.Sprintf("Failed to scan codebase: %v", err),
			ferrors.UserErrorOptions{
				Category: ferrors.ErrorCategoryFileSystem,
				Cause:    err,
			},
		)
	}

	filtered := make([]string, 0, len(allFiles))
	for _, file := range allFiles {
		rel, err := filepath.Rel(directory, file)
		if err != nil {
			continue
		}
		if isIgnored(rel, ignoreRegexes) {
			continue
		}
		filtered = append(filtered, file)
		if len(filtered) >= maxFiles {
			break
		}
	}

	structure.TotalFiles = len(filtered)

	processed := 0
	skipped := 0

	for _, file := range filtered {
		fileInfo, err := os.Stat(file)
		if err != nil {
			skipped++
			continue
		}
		if fileInfo.Size() > maxSize {
			skipped++
			continue
		}

		relPath, _ := filepath.Rel(directory, file)
		dirPath := filepath.Dir(relPath)
		structure.Directories[dirPath] = append(structure.Directories[dirPath], relPath)

		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
		language := extensionToLanguage[ext]
		if language == "" {
			language = "Other"
		}
		structure.FilesByLanguage[language]++

		content, err := fsops.ReadTextFile(file)
		if err != nil {
			skipped++
			continue
		}
		lines := strings.Split(content, "\n")
		structure.TotalLinesOfCode += len(lines)

		deps := findDependencies(content, relPath, ext)
		structure.Dependencies = append(structure.Dependencies, deps...)

		processed++
		if processed%50 == 0 {
			slog.Debug("Analyzed files", "count", processed)
		}
	}

	slog.Info("Codebase analysis complete", "analyzed", processed, "skipped", skipped)
	return structure, nil
}

func AnalyzeProjectDependencies(directory string) (map[string]string, error) {
	deps := map[string]string{}

	packageJSON := filepath.Join(directory, "package.json")
	if fsops.FileExists(packageJSON) {
		content, err := fsops.ReadTextFile(packageJSON)
		if err == nil {
			depMap := parsePackageJSON(content)
			for k, v := range depMap {
				deps[k] = v
			}
		}
	}

	requirements := filepath.Join(directory, "requirements.txt")
	if fsops.FileExists(requirements) {
		content, err := fsops.ReadTextFile(requirements)
		if err == nil {
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "==", 2)
				name := strings.TrimSpace(parts[0])
				version := "latest"
				if len(parts) == 2 {
					version = strings.TrimSpace(parts[1])
				}
				if name != "" {
					deps[name] = version
				}
			}
		}
	}

	gemfile := filepath.Join(directory, "Gemfile")
	if fsops.FileExists(gemfile) {
		content, err := fsops.ReadTextFile(gemfile)
		if err == nil {
			gemRegex := regexp.MustCompile(`^\s*gem\s+['"]([^'"]+)['"]\s*(?:,\s*['"]([^'"]+)['"]\s*)?`)
			matches := gemRegex.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				name := match[1]
				version := "latest"
				if len(match) > 2 && match[2] != "" {
					version = match[2]
				}
				deps[name] = version
			}
		}
	}

	return deps, nil
}

type ContentMatch struct {
	Path    string
	Line    int
	Content string
}

type SearchOptions struct {
	CaseSensitive  bool
	FileExtensions []string
	MaxResults     int
	IgnorePatterns []string
}

func FindFilesByContent(directory, searchTerm string, options SearchOptions) ([]ContentMatch, error) {
	ignorePatterns := options.IgnorePatterns
	if len(ignorePatterns) == 0 {
		ignorePatterns = defaultIgnorePatterns
	}
	maxResults := options.MaxResults
	if maxResults == 0 {
		maxResults = 100
	}

	flags := "i"
	if options.CaseSensitive {
		flags = ""
	}
	regex, err := regexp.Compile("(?" + flags + ")" + searchTerm)
	if err != nil {
		return nil, err
	}

	ignoreRegexes := compileIgnorePatterns(ignorePatterns)
	allFiles, err := fsops.FindFiles(directory, fsops.FindOptions{Recursive: true})
	if err != nil {
		return nil, err
	}

	results := []ContentMatch{}
	for _, file := range allFiles {
		rel, err := filepath.Rel(directory, file)
		if err != nil {
			continue
		}
		if isIgnored(rel, ignoreRegexes) {
			continue
		}

		if len(options.FileExtensions) > 0 {
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
			if !contains(options.FileExtensions, ext) {
				continue
			}
		}

		content, err := fsops.ReadTextFile(file)
		if err != nil {
			continue
		}

		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if regex.MatchString(line) {
				results = append(results, ContentMatch{
					Path:    rel,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
				if len(results) >= maxResults {
					return results, nil
				}
			}
		}
	}

	return results, nil
}

func compileIgnorePatterns(patterns []string) []*regexp.Regexp {
	regexes := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re := regexp.MustCompile(globToRegex(pattern))
		regexes = append(regexes, re)
	}
	return regexes
}

func globToRegex(pattern string) string {
	pattern = regexp.QuoteMeta(pattern)
	pattern = strings.ReplaceAll(pattern, "\\*", ".*")
	pattern = strings.ReplaceAll(pattern, "\\?", ".")
	return pattern
}

func isIgnored(path string, ignoreRegexes []*regexp.Regexp) bool {
	for _, re := range ignoreRegexes {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func findDependencies(content, filePath, extension string) []DependencyInfo {
	deps := []DependencyInfo{}
	if content == "" || !isCodeFile(extension) {
		return deps
	}

	switch extension {
	case "js", "jsx", "ts", "tsx":
		esImportRegex := regexp.MustCompile(`import\s+(?:[\w\s{},*]*\s+from\s+)?['"]([^'"]+)['"]`)
		for _, match := range esImportRegex.FindAllStringSubmatch(content, -1) {
			deps = append(deps, DependencyInfo{
				Name:       packageName(match[1]),
				Type:       "import",
				Source:     filePath,
				ImportPath: match[1],
				IsExternal: isExternalDependency(match[1]),
			})
		}

		requireRegex := regexp.MustCompile(`(?:const|let|var)\s+(?:[\w\s{},*]*)\s*=\s*require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
		for _, match := range requireRegex.FindAllStringSubmatch(content, -1) {
			deps = append(deps, DependencyInfo{
				Name:       packageName(match[1]),
				Type:       "require",
				Source:     filePath,
				ImportPath: match[1],
				IsExternal: isExternalDependency(match[1]),
			})
		}
	case "py":
		importRegex := regexp.MustCompile(`(?m)^\s*import\s+(\S+)|\s*from\s+(\S+)\s+import`)
		for _, match := range importRegex.FindAllStringSubmatch(content, -1) {
			importPath := match[1]
			if importPath == "" {
				importPath = match[2]
			}
			if importPath != "" {
				deps = append(deps, DependencyInfo{
					Name:       strings.Split(importPath, ".")[0],
					Type:       "import",
					Source:     filePath,
					ImportPath: importPath,
					IsExternal: isExternalPythonModule(importPath),
				})
			}
		}
	case "java":
		importRegex := regexp.MustCompile(`(?m)^\s*import\s+([^;]+);`)
		for _, match := range importRegex.FindAllStringSubmatch(content, -1) {
			importPath := match[1]
			deps = append(deps, DependencyInfo{
				Name:       strings.Split(importPath, ".")[0],
				Type:       "import",
				Source:     filePath,
				ImportPath: importPath,
				IsExternal: true,
			})
		}
	case "rb":
		requireRegex := regexp.MustCompile(`(?m)^\s*require\s+['"]([^'"]+)['"]`)
		for _, match := range requireRegex.FindAllStringSubmatch(content, -1) {
			importPath := match[1]
			deps = append(deps, DependencyInfo{
				Name:       importPath,
				Type:       "require",
				Source:     filePath,
				ImportPath: importPath,
				IsExternal: true,
			})
		}
	}

	return deps
}

func isCodeFile(extension string) bool {
	switch extension {
	case "js", "jsx", "ts", "tsx", "py", "java", "c", "cpp", "cs", "go", "rs", "php", "rb", "swift", "kt", "scala":
		return true
	default:
		return false
	}
}

func packageName(importPath string) string {
	if strings.HasPrefix(importPath, ".") || strings.HasPrefix(importPath, "/") {
		return "internal"
	}
	if strings.HasPrefix(importPath, "@") {
		parts := strings.Split(importPath, "/")
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
	}
	return strings.Split(importPath, "/")[0]
}

func isExternalDependency(importPath string) bool {
	return !(strings.HasPrefix(importPath, ".") || strings.HasPrefix(importPath, "/"))
}

func isExternalPythonModule(importPath string) bool {
	stdlib := map[string]struct{}{
		"os": {}, "sys": {}, "re": {}, "math": {}, "datetime": {}, "time": {}, "random": {},
		"json": {}, "csv": {}, "collections": {}, "itertools": {}, "functools": {},
		"pathlib": {}, "shutil": {}, "glob": {}, "pickle": {}, "urllib": {}, "http": {},
		"logging": {}, "argparse": {}, "unittest": {}, "subprocess": {}, "threading": {},
		"multiprocessing": {}, "typing": {}, "enum": {}, "io": {}, "tempfile": {},
	}
	module := strings.Split(importPath, ".")[0]
	_, ok := stdlib[module]
	return !ok && !strings.HasPrefix(importPath, ".")
}

func parsePackageJSON(content string) map[string]string {
	result := map[string]string{}

	depBlock := func(key string) map[string]string {
		block := map[string]string{}
		re := regexp.MustCompile(`"` + key + `"\s*:\s*\{([^}]*)\}`)
		matches := re.FindStringSubmatch(content)
		if len(matches) < 2 {
			return block
		}
		entries := strings.Split(matches[1], ",")
		for _, entry := range entries {
			pair := strings.SplitN(entry, ":", 2)
			if len(pair) != 2 {
				continue
			}
			name := strings.Trim(strings.TrimSpace(pair[0]), "\"")
			version := strings.Trim(strings.TrimSpace(pair[1]), "\"")
			if name != "" {
				block[name] = version
			}
		}
		return block
	}

	for k, v := range depBlock("dependencies") {
		result[k] = v
	}
	for k, v := range depBlock("devDependencies") {
		result[k+" (dev)"] = v
	}

	return result
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
