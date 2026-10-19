package megafloydauth

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewManagerBuildsFileBackedSlot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cfg := Config{
		OAuthHost: "https://auth.example.test",
		BaseURL:   "https://api.example.test",
		Version:   "test",
		HomeDir:   home,
	}
	manager, err := NewManager(cfg)
	require.NoError(t, err)

	name, err := StorageName(CredentialKey(cfg.OAuthHost, cfg.BaseURL))
	require.NoError(t, err)
	require.Equal(t, name, manager.Name)

	path, err := manager.CredentialPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "credentials", name+".json"), path)

	client, ok := manager.Flow.(*Client)
	require.True(t, ok)
	require.Equal(t, cfg.OAuthHost, client.Config.OAuthHost)
}

func TestNewManagerRequiresHost(t *testing.T) {
	t.Parallel()

	_, err := NewManager(Config{HomeDir: t.TempDir()})
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestNewManagerRequiresCredentialsDirectory(t *testing.T) {
	t.Parallel()

	_, err := NewManager(Config{
		OAuthHost:      "https://auth.example.test",
		HomeDir:        "",
		CredentialsDir: "",
	})
	require.ErrorIs(t, err, ErrNotConfigured)
}
