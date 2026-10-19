package megafloydweb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveUsesExplicitPaths(t *testing.T) {
	dir := t.TempDir()
	server := writeFixture(t, filepath.Join(dir, "main.mjs"))
	bundle := filepath.Join(dir, "dist-web")
	writeFixture(t, filepath.Join(bundle, "index.html"))

	paths, err := Resolve(ResolveOptions{
		NodePath:   "/opt/node",
		ServerPath: server,
		BundlePath: bundle,
	})

	require.NoError(t, err)
	require.Equal(t, Paths{Node: "/opt/node", Server: server, Bundle: bundle}, paths)
}

func TestResolveUsesEnvironment(t *testing.T) {
	dir := t.TempDir()
	server := writeFixture(t, filepath.Join(dir, "main.mjs"))
	bundle := filepath.Join(dir, "dist-web")
	writeFixture(t, filepath.Join(bundle, "index.html"))

	t.Setenv(nodeEnv, "/opt/node")
	t.Setenv(scriptEnv, server)
	t.Setenv(bundleEnv, bundle)

	paths, err := Resolve(ResolveOptions{})

	require.NoError(t, err)
	require.Equal(t, Paths{Node: "/opt/node", Server: server, Bundle: bundle}, paths)
}

func TestResolveFindsPackageRootLayout(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "package.json"))
	server := writeFixture(t, filepath.Join(root, "dist", "main.mjs"))
	bundle := filepath.Join(root, "dist-web")
	writeFixture(t, filepath.Join(bundle, "index.html"))

	t.Setenv(nodeEnv, "/opt/node")
	t.Setenv(scriptEnv, "")
	t.Setenv(bundleEnv, "")
	t.Setenv(codeRootEnv, root)

	paths, err := Resolve(ResolveOptions{
		CWD:        t.TempDir(),
		Executable: filepath.Join(t.TempDir(), "megafloyd"),
	})

	require.NoError(t, err)
	require.Equal(t, server, paths.Server)
	require.Equal(t, bundle, paths.Bundle)
}

func TestResolveRejectsMissingBundle(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "package.json"))
	writeFixture(t, filepath.Join(root, "dist", "main.mjs"))

	t.Setenv(nodeEnv, "/opt/node")
	t.Setenv(scriptEnv, "")
	t.Setenv(bundleEnv, "")
	t.Setenv(codeRootEnv, root)

	_, err := Resolve(ResolveOptions{
		CWD:        t.TempDir(),
		Executable: filepath.Join(t.TempDir(), "megafloyd"),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "web bundle not found")
}

func TestResolveRejectsMissingServer(t *testing.T) {
	t.Setenv(nodeEnv, "/opt/node")
	t.Setenv(scriptEnv, "")
	t.Setenv(bundleEnv, "")
	t.Setenv(codeRootEnv, t.TempDir())

	_, err := Resolve(ResolveOptions{
		CWD:        t.TempDir(),
		Executable: filepath.Join(t.TempDir(), "megafloyd"),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "node web server not found")
}

func TestResolveRejectsMissingExplicitScript(t *testing.T) {
	_, err := Resolve(ResolveOptions{
		NodePath:   "/opt/node",
		ServerPath: filepath.Join(t.TempDir(), "missing.mjs"),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "does not point at a file")
}

func TestResolveReportsMissingNode(t *testing.T) {
	t.Setenv(nodeEnv, "")
	t.Setenv("PATH", "")

	_, err := Resolve(ResolveOptions{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "node executable not found")
}

func writeFixture(t *testing.T, path string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("// fixture\n"), 0o600))
	return path
}
