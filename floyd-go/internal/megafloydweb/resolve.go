package megafloydweb

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Paths are the executables and assets the web handoff needs.
type Paths struct {
	Node   string
	Server string
	Bundle string
}

// ResolveOptions overrides discovery. Empty fields are discovered.
type ResolveOptions struct {
	CWD        string
	Executable string
	NodePath   string
	ServerPath string
	BundlePath string
}

// Resolve finds the Node executable, the Node server entry point, and the
// committed web bundle.
//
// MEGAFLOYD_WEB_NODE, MEGAFLOYD_WEB_SCRIPT, MEGAFLOYD_WEB_BUNDLE, and
// MEGAFLOYD_CODE_ROOT override discovery. Otherwise the caller's working
// directory and executable directory are walked upward for the repository
// layout. Every missing piece returns a clean error naming the setting that
// fixes it.
func Resolve(opts ResolveOptions) (Paths, error) {
	node, err := resolveNode(opts.NodePath)
	if err != nil {
		return Paths{}, err
	}

	server, err := resolveServer(opts)
	if err != nil {
		return Paths{}, err
	}

	bundle, err := resolveBundle(opts, server)
	if err != nil {
		return Paths{}, err
	}

	return Paths{Node: node, Server: server, Bundle: bundle}, nil
}

func resolveNode(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if node := os.Getenv(nodeEnv); node != "" {
		return node, nil
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return "", errors.New(
			"node executable not found; install Node >= 24.15.0 " +
				"or set MEGAFLOYD_WEB_NODE",
		)
	}
	return node, nil
}

func resolveServer(opts ResolveOptions) (string, error) {
	if opts.ServerPath != "" {
		return requireFile(opts.ServerPath, "MEGAFLOYD_WEB_SCRIPT")
	}
	if script := os.Getenv(scriptEnv); script != "" {
		return requireFile(script, scriptEnv)
	}

	candidates := serverCandidates(opts.CWD, opts.Executable)
	for _, candidate := range candidates {
		if isRegularFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf(
		"node web server not found; build floyd-code (dist/main.mjs) "+
			"or set MEGAFLOYD_WEB_SCRIPT or MEGAFLOYD_CODE_ROOT (checked %d locations)",
		len(candidates),
	)
}

func resolveBundle(opts ResolveOptions, server string) (string, error) {
	if opts.BundlePath != "" {
		return requireBundle(opts.BundlePath, "MEGAFLOYD_WEB_BUNDLE")
	}
	if bundle := os.Getenv(bundleEnv); bundle != "" {
		return requireBundle(bundle, bundleEnv)
	}

	var candidates []string
	if root := packageRoot(server); root != "" {
		candidates = append(candidates, filepath.Join(root, BundleDirName))
	}
	candidates = append(candidates, bundleCandidates(opts.CWD, opts.Executable)...)

	for _, candidate := range candidates {
		if isRegularFile(filepath.Join(candidate, "index.html")) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf(
		"web bundle not found; expected index.html under %s "+
			"(sync the bundle or set MEGAFLOYD_WEB_BUNDLE)",
		firstOrNone(candidates),
	)
}

func serverCandidates(cwd, exePath string) []string {
	var roots []string
	if root := os.Getenv(codeRootEnv); root != "" {
		roots = append(roots, root)
	}
	if cwd != "" {
		roots = append(roots, cwd)
	}
	if exePath != "" {
		roots = append(roots, filepath.Dir(exePath))
	}

	var candidates []string
	seen := make(map[string]struct{})
	for _, root := range roots {
		dir := root
		for range 10 {
			rels := []string{
				filepath.Join("floyd-code", "apps", "floyd-code", "dist", "main.mjs"),
				filepath.Join("apps", "floyd-code", "dist", "main.mjs"),
			}
			// A bare dist/main.mjs is only trusted at a package root,
			// so an unrelated dist directory higher up cannot match.
			if isRegularFile(filepath.Join(dir, "package.json")) {
				rels = append(rels, filepath.Join("dist", "main.mjs"))
			}
			for _, rel := range rels {
				candidate := filepath.Join(dir, rel)
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				candidates = append(candidates, candidate)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return candidates
}

func bundleCandidates(cwd, exePath string) []string {
	exeDir := ""
	if exePath != "" {
		exeDir = filepath.Dir(exePath)
	}
	var candidates []string
	for _, root := range []string{cwd, exeDir, os.Getenv(codeRootEnv)} {
		if root == "" {
			continue
		}
		dir := root
		for range 10 {
			candidates = append(candidates,
				filepath.Join(dir, "floyd-code", "apps", "floyd-code", BundleDirName),
				filepath.Join(dir, "apps", "floyd-code", BundleDirName),
			)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return candidates
}

// packageRoot walks up from a file to the nearest directory containing a
// package.json.
func packageRoot(file string) string {
	if file == "" {
		return ""
	}
	dir := filepath.Dir(file)
	for range 10 {
		if isRegularFile(filepath.Join(dir, "package.json")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

func requireFile(path, setting string) (string, error) {
	if !isRegularFile(path) {
		return "", fmt.Errorf("%s does not point at a file: %s", setting, path)
	}
	return path, nil
}

func requireBundle(path, setting string) (string, error) {
	if !isRegularFile(filepath.Join(path, "index.html")) {
		return "", fmt.Errorf("%s is not a web bundle (missing index.html): %s", setting, path)
	}
	return path, nil
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func firstOrNone(values []string) string {
	if len(values) == 0 {
		return "<no candidate>"
	}
	return values[0]
}
