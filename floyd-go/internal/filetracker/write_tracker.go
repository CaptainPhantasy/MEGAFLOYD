package filetracker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/db"
)

// WriteRecord captures metadata about a single file write operation.
type WriteRecord struct {
	Path      string
	Timestamp time.Time
	Operation string // "write", "edit", "create", "delete"
	Size      int64
}

// DependencyGraph tracks file-level write dependencies within a session.
// It maintains an ordered log of write operations and can compute
// reverse dependencies (which files were written after a given file).
type DependencyGraph struct {
	mu      sync.RWMutex
	records []WriteRecord
	byPath  map[string][]int // path -> indices into records
}

// NewDependencyGraph creates a new empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		records: make([]WriteRecord, 0),
		byPath:  make(map[string][]int),
	}
}

// Record adds a write operation to the graph.
func (g *DependencyGraph) Record(path, operation string, size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := len(g.records)
	g.records = append(g.records, WriteRecord{
		Path:      path,
		Timestamp: time.Now(),
		Operation: operation,
		Size:      size,
	})
	g.byPath[path] = append(g.byPath[path], idx)
}

// WrittenAfter returns all files written after the given path was last written.
// Useful for determining what files might be affected by a change.
func (g *DependencyGraph) WrittenAfter(path string) []WriteRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()

	indices, ok := g.byPath[path]
	if !ok || len(indices) == 0 {
		return nil
	}

	// Find the latest write index for this path
	latestIdx := indices[len(indices)-1]

	var result []WriteRecord
	for i := latestIdx + 1; i < len(g.records); i++ {
		result = append(result, g.records[i])
	}
	return result
}

// AllRecords returns a copy of all write records in chronological order.
func (g *DependencyGraph) AllRecords() []WriteRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make([]WriteRecord, len(g.records))
	copy(result, g.records)
	return result
}

// RecordCount returns the total number of recorded writes.
func (g *DependencyGraph) RecordCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.records)
}

// Paths returns the set of unique paths that have been written to.
func (g *DependencyGraph) Paths() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	paths := make([]string, 0, len(g.byPath))
	for p := range g.byPath {
		paths = append(paths, p)
	}
	return paths
}

// ExtendedService extends Service with write tracking and dependency graphing.
type ExtendedService interface {
	Service
	// RecordWrite records when a file was written/modified.
	RecordWrite(ctx context.Context, sessionID, path, operation string)
	// WriteHistory returns all write records for a session.
	WriteHistory(ctx context.Context, sessionID string) []WriteRecord
}

type extendedService struct {
	Service
	graphs sync.Map // sessionID -> *DependencyGraph
}

// NewExtendedService creates a service with write tracking capabilities.
func NewExtendedService(q *db.Queries) ExtendedService {
	return &extendedService{
		Service: NewService(q),
	}
}

// getOrCreateGraph lazily creates a dependency graph per session.
func (s *extendedService) getOrCreateGraph(sessionID string) *DependencyGraph {
	g, ok := s.graphs.Load(sessionID)
	if !ok {
		g = NewDependencyGraph()
		g, _ = s.graphs.LoadOrStore(sessionID, g)
	}
	return g.(*DependencyGraph)
}

// RecordWrite records a file write event and updates the dependency graph.
func (s *extendedService) RecordWrite(_ context.Context, sessionID, path, operation string) {
	absPath := path
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			absPath = filepath.Join(wd, path)
		}
	}

	var size int64
	if info, err := os.Stat(absPath); err == nil {
		size = info.Size()
	}

	graph := s.getOrCreateGraph(sessionID)
	graph.Record(relpath(path), operation, size)

	slog.Debug("File write recorded",
		"session_id", sessionID,
		"path", path,
		"operation", operation,
		"size", size,
	)
}

// WriteHistory returns all write records for a session.
func (s *extendedService) WriteHistory(_ context.Context, sessionID string) []WriteRecord {
	g, ok := s.graphs.Load(sessionID)
	if !ok {
		return nil
	}
	return g.(*DependencyGraph).AllRecords()
}
