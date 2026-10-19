// Package intelligence provides code intelligence capabilities including
// symbol extraction, indexing, and semantic search.
package intelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/fsext"
)

// SymbolKind represents the type of a code symbol
type SymbolKind string

const (
	SymbolKindFunction  SymbolKind = "function"
	SymbolKindMethod    SymbolKind = "method"
	SymbolKindStruct    SymbolKind = "struct"
	SymbolKindInterface SymbolKind = "interface"
	SymbolKindClass     SymbolKind = "class"
	SymbolKindVariable  SymbolKind = "variable"
	SymbolKindConstant  SymbolKind = "constant"
	SymbolKindType      SymbolKind = "type"
	SymbolKindEnum      SymbolKind = "enum"
	SymbolKindImport    SymbolKind = "import"
	SymbolKindField     SymbolKind = "field"
)

// Symbol represents a code symbol (function, class, etc.)
type Symbol struct {
	Name      string     `json:"name"`
	Kind      SymbolKind `json:"kind"`
	File      string     `json:"file"`
	Line      int        `json:"line"`
	EndLine   int        `json:"end_line,omitempty"`
	Signature string     `json:"signature,omitempty"`
	Docstring string     `json:"docstring,omitempty"`
	Parent    string     `json:"parent,omitempty"`   // Parent symbol (e.g., class for method)
	Receiver  string     `json:"receiver,omitempty"` // Receiver type for methods
	Exported  bool       `json:"exported"`
	Embedded  bool       `json:"embedded,omitempty"` // Is this embedded in another type?
}

// SymbolIndex manages symbol extraction and querying
type SymbolIndex struct {
	mu      sync.RWMutex
	symbols map[string][]Symbol  // file path -> symbols
	byName  map[string][]*Symbol // symbol name -> pointers
	files   map[string]time.Time // file path -> last indexed
	rootDir string
}

// NewSymbolIndex creates a new symbol index
func NewSymbolIndex(rootDir string) *SymbolIndex {
	return &SymbolIndex{
		symbols: make(map[string][]Symbol),
		byName:  make(map[string][]*Symbol),
		files:   make(map[string]time.Time),
		rootDir: rootDir,
	}
}

// IndexFile indexes symbols in a single file
func (si *SymbolIndex) IndexFile(ctx context.Context, path string) error {
	// Check if file needs reindexing
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	si.mu.RLock()
	lastIndexed, exists := si.files[path]
	si.mu.RUnlock()

	if exists && info.ModTime().Before(lastIndexed) {
		return nil // Already up to date
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Extract symbols based on file extension
	var symbols []Symbol
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		symbols = extractGoSymbols(string(content), path)
	case ".ts", ".tsx":
		symbols = extractTypeScriptSymbols(string(content), path)
	case ".js", ".jsx":
		symbols = extractJavaScriptSymbols(string(content), path)
	case ".py":
		symbols = extractPythonSymbols(string(content), path)
	default:
		return nil // Unsupported file type
	}

	si.mu.Lock()
	defer si.mu.Unlock()

	// Remove old symbols from byName index
	for _, old := range si.symbols[path] {
		if ptrs, ok := si.byName[old.Name]; ok {
			// Filter out old pointers
			var newPtrs []*Symbol
			for _, p := range ptrs {
				if p.File != path {
					newPtrs = append(newPtrs, p)
				}
			}
			if len(newPtrs) == 0 {
				delete(si.byName, old.Name)
			} else {
				si.byName[old.Name] = newPtrs
			}
		}
	}

	// Store new symbols
	si.symbols[path] = symbols
	si.files[path] = time.Now()

	// Add to byName index
	for i := range symbols {
		si.byName[symbols[i].Name] = append(si.byName[symbols[i].Name], &symbols[i])
	}

	return nil
}

// IndexDir indexes all supported files in a directory
func (si *SymbolIndex) IndexDir(ctx context.Context, dir string, maxDepth int) error {
	// Walk directory
	files, _, err := fsext.ListDirectory(dir, nil, maxDepth, 10000)
	if err != nil {
		return fmt.Errorf("list directory: %w", err)
	}

	var errs []error
	for _, file := range files {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Only process supported file types
		ext := strings.ToLower(filepath.Ext(file))
		switch ext {
		case ".go", ".ts", ".tsx", ".js", ".jsx", ".py":
			if err := si.IndexFile(ctx, file); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", file, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("indexing errors: %v", errs)
	}
	return nil
}

// Query searches for symbols matching the given name (fuzzy match)
func (si *SymbolIndex) Query(name string, limit int) []Symbol {
	si.mu.RLock()
	defer si.mu.RUnlock()

	nameLower := strings.ToLower(name)
	var results []Symbol

	// Exact matches first
	if ptrs, ok := si.byName[name]; ok {
		for _, p := range ptrs {
			results = append(results, *p)
		}
	}

	// Prefix matches
	for symName, ptrs := range si.byName {
		if strings.EqualFold(symName, name) {
			continue // Already added
		}
		if strings.HasPrefix(strings.ToLower(symName), nameLower) {
			for _, p := range ptrs {
				results = append(results, *p)
			}
		}
		if limit > 0 && len(results) >= limit {
			break
		}
	}

	// Contains matches (if still under limit)
	if limit == 0 || len(results) < limit {
		for symName, ptrs := range si.byName {
			if strings.Contains(strings.ToLower(symName), nameLower) {
				if !strings.HasPrefix(strings.ToLower(symName), nameLower) {
					for _, p := range ptrs {
						results = append(results, *p)
					}
				}
			}
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	// Sort by relevance (exact match, prefix, contains)
	sort.Slice(results, func(i, j int) bool {
		iExact := strings.EqualFold(results[i].Name, name)
		jExact := strings.EqualFold(results[j].Name, name)
		if iExact != jExact {
			return iExact
		}
		iPrefix := strings.HasPrefix(strings.ToLower(results[i].Name), nameLower)
		jPrefix := strings.HasPrefix(strings.ToLower(results[j].Name), nameLower)
		if iPrefix != jPrefix {
			return iPrefix
		}
		return results[i].Name < results[j].Name
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// QueryByKind returns symbols of a specific kind
func (si *SymbolIndex) QueryByKind(kind SymbolKind, limit int) []Symbol {
	si.mu.RLock()
	defer si.mu.RUnlock()

	var results []Symbol
	for _, symbols := range si.symbols {
		for _, sym := range symbols {
			if sym.Kind == kind {
				results = append(results, sym)
			}
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// QueryByFile returns all symbols in a specific file
func (si *SymbolIndex) QueryByFile(path string) []Symbol {
	si.mu.RLock()
	defer si.mu.RUnlock()

	symbols, ok := si.symbols[path]
	if !ok {
		return nil
	}

	// Return a copy
	result := make([]Symbol, len(symbols))
	copy(result, symbols)
	return result
}

// Stats returns index statistics
func (si *SymbolIndex) Stats() IndexStats {
	si.mu.RLock()
	defer si.mu.RUnlock()

	stats := IndexStats{
		Files:  len(si.files),
		ByKind: make(map[SymbolKind]int),
	}

	for _, symbols := range si.symbols {
		stats.TotalSymbols += len(symbols)
		for _, sym := range symbols {
			stats.ByKind[sym.Kind]++
		}
	}

	return stats
}

// IndexStats contains statistics about the symbol index
type IndexStats struct {
	Files        int                `json:"files"`
	TotalSymbols int                `json:"total_symbols"`
	ByKind       map[SymbolKind]int `json:"by_kind"`
}

// Clear removes all symbols from the index
func (si *SymbolIndex) Clear() {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.symbols = make(map[string][]Symbol)
	si.byName = make(map[string][]*Symbol)
	si.files = make(map[string]time.Time)
}

// extractGoSymbols extracts symbols from Go source code
func extractGoSymbols(content, path string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Function declaration: func name(params) (returns) {
	funcRegex := regexp.MustCompile(`^func\s+(?:\(([^)]+)\)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*\(([^)]*)\)|\s+([^{]+))?\s*\{`)
	// Type declaration: type Name struct/interface {
	typeRegex := regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+(struct|interface)\s*\{`)
	// Interface method: Method(params) (returns)
	interfaceMethodRegex := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*\(([^)]*)\)|\s+([^{;]+))?\s*[{;]?`)
	// Struct field: Field type
	structFieldRegex := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z0-9_.*\[\]]+)`)
	// Const/var: const/var Name = value
	constVarRegex := regexp.MustCompile(`^(?:const|var)\s+(?:\(([^)]+)\)|([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*))`)
	// Import
	importRegex := regexp.MustCompile(`^import\s+(?:\(([^)]+)\)|"([^"]+)")`)

	var inStruct bool
	var structName string
	var inInterface bool
	var interfaceName string
	var inMultiLineConst bool

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Check for multi-line const/var end
		if inMultiLineConst && trimmed == ")" {
			inMultiLineConst = false
			continue
		}

		// Function
		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			receiver := matches[1]
			name := matches[2]
			params := matches[3]
			returns := matches[4]
			if returns == "" {
				returns = matches[5]
			}

			signature := fmt.Sprintf("func %s(%s)", name, params)
			if returns != "" {
				signature += " " + returns
			}

			kind := SymbolKindFunction
			if receiver != "" {
				kind = SymbolKindMethod
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      kind,
				File:      path,
				Line:      i + 1,
				Signature: signature,
				Receiver:  receiver,
				Exported:  isExported(name),
			})
			continue
		}

		// Type (struct/interface)
		if matches := typeRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			kind := matches[2]

			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     SymbolKind(kind),
				File:     path,
				Line:     i + 1,
				Exported: isExported(name),
			})

			if kind == "struct" {
				inStruct = true
				structName = name
			} else {
				inInterface = true
				interfaceName = name
			}
			continue
		}

		// End of struct/interface
		if trimmed == "}" {
			if inStruct {
				inStruct = false
				structName = ""
			}
			if inInterface {
				inInterface = false
				interfaceName = ""
			}
			continue
		}

		// Interface method
		if inInterface {
			if matches := interfaceMethodRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				if name != "" && !strings.HasPrefix(trimmed, "//") {
					params := matches[2]
					returns := matches[3]
					if returns == "" {
						returns = matches[4]
					}

					signature := fmt.Sprintf("%s(%s)", name, params)
					if returns != "" {
						signature += " " + returns
					}

					symbols = append(symbols, Symbol{
						Name:      name,
						Kind:      SymbolKindMethod,
						File:      path,
						Line:      i + 1,
						Signature: signature,
						Parent:    interfaceName,
						Exported:  isExported(name),
					})
				}
			}
			continue
		}

		// Struct field
		if inStruct && !strings.HasPrefix(trimmed, "//") {
			if matches := structFieldRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				fieldType := matches[2]

				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      SymbolKindField,
					File:      path,
					Line:      i + 1,
					Signature: name + " " + fieldType,
					Parent:    structName,
					Exported:  isExported(name),
				})
			}
			continue
		}

		// Const/var
		if matches := constVarRegex.FindStringSubmatch(line); matches != nil {
			if matches[1] != "" {
				// Multi-line const/var
				inMultiLineConst = true
			} else if matches[2] != "" {
				names := strings.Split(matches[2], ",")
				for _, n := range names {
					n = strings.TrimSpace(n)
					kind := SymbolKindVariable
					if strings.HasPrefix(trimmed, "const") {
						kind = SymbolKindConstant
					}
					symbols = append(symbols, Symbol{
						Name:     n,
						Kind:     kind,
						File:     path,
						Line:     i + 1,
						Exported: isExported(n),
					})
				}
			}
			continue
		}

		// Import
		if matches := importRegex.FindStringSubmatch(line); matches != nil {
			if matches[1] != "" {
				// Multi-line import - just mark the line
				symbols = append(symbols, Symbol{
					Name: "import block",
					Kind: SymbolKindImport,
					File: path,
					Line: i + 1,
				})
			} else if matches[2] != "" {
				symbols = append(symbols, Symbol{
					Name: matches[2],
					Kind: SymbolKindImport,
					File: path,
					Line: i + 1,
				})
			}
		}
	}

	return symbols
}

// extractTypeScriptSymbols extracts symbols from TypeScript source code
func extractTypeScriptSymbols(content, path string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Function: function name(params): return_type {
	funcRegex := regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*:\s*([^{]+))?\s*\{`)
	// Arrow function: const name = (params) =>
	arrowRegex := regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:async\s+)?\(([^)]*)\)(?:\s*:\s*([^=]+))?\s*=>`)
	// Class: class Name {
	classRegex := regexp.MustCompile(`^(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// Interface: interface Name {
	interfaceRegex := regexp.MustCompile(`^(?:export\s+)?interface\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// Type: type Name =
	typeRegex := regexp.MustCompile(`^(?:export\s+)?type\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`)
	// Enum: enum Name {
	enumRegex := regexp.MustCompile(`^(?:export\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// Method: name(params): return_type {
	methodRegex := regexp.MustCompile(`^\s*(?:public|private|protected|readonly|static|async|\s)+\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*:\s*([^{]+))?\s*\{`)

	var inClass bool
	var className string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Function
		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			params := matches[2]
			returnType := matches[3]

			signature := fmt.Sprintf("function %s(%s)", name, params)
			if returnType != "" {
				signature += ": " + strings.TrimSpace(returnType)
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      SymbolKindFunction,
				File:      path,
				Line:      i + 1,
				Signature: signature,
				Exported:  strings.Contains(line, "export"),
			})
			continue
		}

		// Arrow function
		if matches := arrowRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			params := matches[2]
			returnType := matches[3]

			signature := fmt.Sprintf("%s(%s)", name, params)
			if returnType != "" {
				signature += ": " + strings.TrimSpace(returnType)
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      SymbolKindFunction,
				File:      path,
				Line:      i + 1,
				Signature: signature,
				Exported:  strings.Contains(line, "export"),
			})
			continue
		}

		// Class
		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			inClass = true
			className = name

			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     SymbolKindClass,
				File:     path,
				Line:     i + 1,
				Exported: strings.Contains(line, "export"),
			})
			continue
		}

		// Interface
		if matches := interfaceRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]

			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     SymbolKindInterface,
				File:     path,
				Line:     i + 1,
				Exported: strings.Contains(line, "export"),
			})
			continue
		}

		// Type alias
		if matches := typeRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]

			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     SymbolKindType,
				File:     path,
				Line:     i + 1,
				Exported: strings.Contains(line, "export"),
			})
			continue
		}

		// Enum
		if matches := enumRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]

			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     SymbolKindEnum,
				File:     path,
				Line:     i + 1,
				Exported: strings.Contains(line, "export"),
			})
			continue
		}

		// Method (inside class)
		if inClass {
			if matches := methodRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				// Skip constructor keyword itself
				if name != "constructor" && name != "get" && name != "set" && name != "static" {
					params := matches[2]
					returnType := matches[3]

					signature := fmt.Sprintf("%s(%s)", name, params)
					if returnType != "" {
						signature += ": " + strings.TrimSpace(returnType)
					}

					symbols = append(symbols, Symbol{
						Name:      name,
						Kind:      SymbolKindMethod,
						File:      path,
						Line:      i + 1,
						Signature: signature,
						Parent:    className,
						Exported:  true, // Methods in exported classes are exported
					})
				}
			}
		}

		// End of class
		if trimmed == "}" && inClass {
			inClass = false
			className = ""
		}
	}

	return symbols
}

// extractJavaScriptSymbols extracts symbols from JavaScript source code
func extractJavaScriptSymbols(content, path string) []Symbol {
	// JavaScript is similar to TypeScript but without types
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	funcRegex := regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*\{`)
	arrowRegex := regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:async\s+)?\(([^)]*)\)\s*=>`)
	classRegex := regexp.MustCompile(`^(?:export\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	methodRegex := regexp.MustCompile(`^\s*(?:async\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*\{`)

	var inClass bool
	var className string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			params := matches[2]

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      SymbolKindFunction,
				File:      path,
				Line:      i + 1,
				Signature: fmt.Sprintf("function %s(%s)", name, params),
				Exported:  strings.Contains(line, "export"),
			})
			continue
		}

		if matches := arrowRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			params := matches[2]

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      SymbolKindFunction,
				File:      path,
				Line:      i + 1,
				Signature: fmt.Sprintf("%s(%s)", name, params),
				Exported:  strings.Contains(line, "export"),
			})
			continue
		}

		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			inClass = true
			className = name

			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     SymbolKindClass,
				File:     path,
				Line:     i + 1,
				Exported: strings.Contains(line, "export"),
			})
			continue
		}

		if inClass {
			if matches := methodRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				if name != "constructor" && name != "get" && name != "set" {
					params := matches[2]

					symbols = append(symbols, Symbol{
						Name:      name,
						Kind:      SymbolKindMethod,
						File:      path,
						Line:      i + 1,
						Signature: fmt.Sprintf("%s(%s)", name, params),
						Parent:    className,
						Exported:  true,
					})
				}
			}
		}

		if trimmed == "}" && inClass {
			inClass = false
			className = ""
		}
	}

	return symbols
}

// extractPythonSymbols extracts symbols from Python source code
func extractPythonSymbols(content, path string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Function: def name(params) -> return_type:
	funcRegex := regexp.MustCompile(`^def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*->\s*([^:]+))?\s*:`)
	// Async function
	asyncFuncRegex := regexp.MustCompile(`^async\s+def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*->\s*([^:]+))?\s*:`)
	// Class: class Name(Base):
	classRegex := regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*\(([^)]*)\))?\s*:`)
	// Method (indented def)
	methodRegex := regexp.MustCompile(`^\s+def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)(?:\s*->\s*([^:]+))?\s*:`)

	var inClass bool
	var className string
	var classIndent int

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Calculate indentation
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		// If we're back to class level or less, we're no longer in a class
		if inClass && indent <= classIndent {
			inClass = false
			className = ""
		}

		// Class
		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			baseClass := matches[2]

			inClass = true
			className = name
			classIndent = indent

			signature := fmt.Sprintf("class %s", name)
			if baseClass != "" {
				signature += "(" + baseClass + ")"
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      SymbolKindClass,
				File:      path,
				Line:      i + 1,
				Signature: signature,
				Exported:  !strings.HasPrefix(name, "_"),
			})
			continue
		}

		// Async function
		if matches := asyncFuncRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			params := matches[2]
			returnType := matches[3]

			signature := fmt.Sprintf("async def %s(%s)", name, params)
			if returnType != "" {
				signature += " -> " + strings.TrimSpace(returnType)
			}

			kind := SymbolKindFunction
			if inClass {
				kind = SymbolKindMethod
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      kind,
				File:      path,
				Line:      i + 1,
				Signature: signature,
				Parent:    className,
				Exported:  !strings.HasPrefix(name, "_"),
			})
			continue
		}

		// Method (indented def in class)
		if inClass {
			if matches := methodRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				params := matches[2]
				returnType := matches[3]

				signature := fmt.Sprintf("def %s(%s)", name, params)
				if returnType != "" {
					signature += " -> " + strings.TrimSpace(returnType)
				}

				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      SymbolKindMethod,
					File:      path,
					Line:      i + 1,
					Signature: signature,
					Parent:    className,
					Exported:  !strings.HasPrefix(name, "_"),
				})
				continue
			}
		}

		// Top-level function
		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			params := matches[2]
			returnType := matches[3]

			signature := fmt.Sprintf("def %s(%s)", name, params)
			if returnType != "" {
				signature += " -> " + strings.TrimSpace(returnType)
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      SymbolKindFunction,
				File:      path,
				Line:      i + 1,
				Signature: signature,
				Exported:  !strings.HasPrefix(name, "_"),
			})
		}
	}

	return symbols
}

// isExported checks if a Go identifier is exported
func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
