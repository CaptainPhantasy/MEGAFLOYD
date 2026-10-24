package mcp

import (
	"crypto/sha256"
	"fmt"
	"iter"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
)

type WorkspaceScope interface{ WorkingDir() string }

type directoryScope string

func (s directoryScope) WorkingDir() string { return string(s) }

func scopePrefix(scope ...WorkspaceScope) string {
	if len(scope) == 0 || scope[0] == nil {
		return ""
	}
	if cfg, ok := scope[0].(*config.ConfigStore); ok && cfg == nil {
		return ""
	}
	directory := scope[0].WorkingDir()
	if directory == "" {
		return ""
	}
	if absolute, err := filepath.Abs(directory); err == nil {
		directory = absolute
	}
	if real, err := filepath.EvalSymlinks(directory); err == nil {
		directory = real
	}
	return fmt.Sprintf("%x\x00", sha256.Sum256([]byte(filepath.Clean(directory))))
}

func publicName(name string) string {
	if i := strings.LastIndexByte(name, 0); i >= 0 {
		return name[i+1:]
	}
	return name
}
func registryName(scope WorkspaceScope, name string) string {
	return scopePrefix(scope) + publicName(name)
}
func inScope(name string, scope ...WorkspaceScope) bool {
	prefix := scopePrefix(scope...)
	if prefix == "" {
		return !strings.ContainsRune(name, 0)
	}
	return strings.HasPrefix(name, prefix)
}

func scopedEntries[T any](entries iter.Seq2[string, T], scope ...WorkspaceScope) iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		for key, value := range entries {
			if inScope(key, scope...) && !yield(publicName(key), value) {
				return
			}
		}
	}
}

func GetStatesForWorkspace(directory string) map[string]ClientInfo {
	return GetStates(directoryScope(directory))
}
func ResourcesForWorkspace(directory string) iter.Seq2[string, []*Resource] {
	return Resources(directoryScope(directory))
}

func scopedConfigs(cfg *config.ConfigStore) config.MCPs {
	result := make(config.MCPs, len(cfg.Config().MCP))
	for name, server := range cfg.Config().MCP {
		result[registryName(cfg, name)] = server
	}
	return result
}

func scopedStateRecords(cfg *config.ConfigStore) map[string]ClientInfo {
	result := make(map[string]ClientInfo)
	for name, info := range states.Seq2() {
		if inScope(name, cfg) {
			result[name] = info
		}
	}
	return result
}

type initializationGate struct {
	mu      sync.Mutex
	started bool
	armedAt time.Time
	done    chan struct{}
	once    sync.Once
}

var gatesMu sync.Mutex
var gates = map[string]*initializationGate{}

func initGate(scope ...WorkspaceScope) *initializationGate {
	key := scopePrefix(scope...)
	gatesMu.Lock()
	defer gatesMu.Unlock()
	if gate := gates[key]; gate != nil {
		return gate
	}
	gate := &initializationGate{done: make(chan struct{})}
	gates[key] = gate
	return gate
}

type reconcileGate struct {
	mu      sync.Mutex
	running bool
	dirty   bool
}

var reconcileGatesMu sync.Mutex
var reconcileGates = map[string]*reconcileGate{}

func scopeReconcileGate(cfg *config.ConfigStore) *reconcileGate {
	key := scopePrefix(cfg)
	reconcileGatesMu.Lock()
	defer reconcileGatesMu.Unlock()
	if gate := reconcileGates[key]; gate != nil {
		return gate
	}
	gate := &reconcileGate{}
	reconcileGates[key] = gate
	return gate
}
