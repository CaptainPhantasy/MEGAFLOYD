// Package extensibility handles loading of agents, skills, and plugins from the AGENT_SKILLS_PROMPT_INDEX.md
package extensibility

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IndexEntry represents a single entry from the extensibility index
type IndexEntry struct {
	Name        string
	Description string
	Path        string
	Category    string // agent, skill, or plugin
	Tags        []string
}

// LoadIndex loads entries from AGENT_SKILLS_PROMPT_INDEX.md
func LoadIndex(indexPath string) ([]IndexEntry, error) {
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("index file does not exist: %s", indexPath)
	}

	file, err := os.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open index file: %w", err)
	}
	defer file.Close()

	var entries []IndexEntry
	scanner := bufio.NewScanner(file)
	
	var currentCategory string
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if strings.HasPrefix(line, "## 📍") {
			categoryHeader := strings.TrimPrefix(line, "## 📍 ")
			if strings.Contains(categoryHeader, "COMMANDS") {
				currentCategory = "command"
			} else if strings.Contains(categoryHeader, "SYSTEM SKILLS") {
				currentCategory = "skill"
			} else if strings.Contains(categoryHeader, "LOCAL AGENT STORAGE") {
				currentCategory = "agent"
			} else {
				continue
			}
		} else if strings.HasPrefix(line, "#### ") || strings.HasPrefix(line, "#### `/") {
			// Parse agent skill or command definition
			name := strings.TrimPrefix(strings.TrimPrefix(line, "#### "), "`/")
			name = strings.Split(name, "`")[0] // extract name from markdown
			
			// Next few lines contain description and path
			description := ""
			path := ""
			
			for i := 0; i < 5 && scanner.Scan(); i++ {
				nextLine := strings.TrimSpace(scanner.Text())
				
				// Skip until we find a field that starts with dash, indicating content
				if strings.HasPrefix(nextLine, "- ") {
					if strings.Contains(nextLine, "**Purpose:**") || strings.Contains(nextLine, "Purpose:") {
						descStart := strings.Index(nextLine, ": ")
						if descStart != -1 && len(nextLine) > descStart+2 {
							description = strings.TrimSpace(nextLine[descStart+2:])
						}
					} else if strings.Contains(nextLine, "**Path:**") || strings.Contains(nextLine, "Path:") {
						pathStart := strings.Index(nextLine, ": ")
						if pathStart != -1 && len(nextLine) > pathStart+2 {
							path = strings.TrimSpace(nextLine[pathStart+2:])
							// Clean up markdown formatting
							path = strings.ReplaceAll(path, "`", "")
						}
					}
					
					if description != "" && path != "" {
						break
					}
				}
			}
			
			if name != "" && path != "" {
				// Determine exact category based on the path
				cat := currentCategory
				if cat == "command" {
					cat = "agent" // commands are actually agents in this context
				}
				
				entry := IndexEntry{
					Name:        name,
					Description: description,
					Path:        cleanPath(path),
					Category:    cat,
				}
				entries = append(entries, entry)
			}
		} else if strings.Contains(line, "**File:**") && strings.Contains(line, "Location:") {
			// Alternative format: agent definition
			nameLine := strings.Replace(line, "**File:** ", "", 1)
			nameEnd := strings.Index(nameLine, " - ")
			if nameEnd != -1 {
				name := nameLine[:nameEnd]
				descStart := strings.Index(nameLine, " - ")
				if descStart != -1 && len(nameLine) > descStart+3 {
					desc := nameLine[descStart+3:]
					locStart := strings.Index(line, "Location:")
					var path string
					if locStart != -1 {
						path = strings.TrimSpace(nameLine[locStart+9:]) // 9 for "Location: "
						locEnd := strings.Index(path, "```")
						if locEnd != -1 {
							path = path[:locEnd]
						}
						path = strings.TrimSpace(strings.ReplaceAll(path, "`", ""))
					}
					
					if name != "" && path != "" {
						entry := IndexEntry{
							Name:        name,
							Description: strings.TrimSuffix(desc, "-"),
							Path:        cleanPath(path),
							Category:    "agent",
						}
						entries = append(entries, entry)
					}
				}
			}
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading index file: %w", err)
	}
	
	return entries, nil
}

// LoadItem loads the contents of a specific item from the index entry
func LoadItem(entry IndexEntry) (string, error) {
	// Resolve path (handle ~, relative paths)
	absPath := resolvePath(entry.Path)
	
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read item from path '%s': %w", absPath, err)
	}
	
	return string(content), nil
}

// resolvePath converts a relative or tilde path to an absolute path
func resolvePath(path string) string {
	// Expand ~/ paths
	if strings.HasPrefix(path, "~/") {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}
	
	// Handle relative to project root
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
		// For now, assuming relative paths are relative to current working directory
		wd, _ := os.Getwd()
		return filepath.Join(wd, path)
	}
	
	return path
}

func cleanPath(path string) string {
	return strings.TrimSpace(strings.ReplaceAll(path, "`", ""))
}

// GetEntriesByCategory returns entries filtered by category
func GetEntriesByCategory(entries []IndexEntry, category string) []IndexEntry {
	var filtered []IndexEntry
	for _, entry := range entries {
		if strings.ToLower(entry.Category) == strings.ToLower(category) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}