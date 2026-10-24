package config

import (
	"context"
	"encoding/json"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/legacy-ai/floyd/internal/oauth"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func canonicalAccountStore(t *testing.T, base string) (*ConfigStore, SelectedModel) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MEGAFLOYD_HOME", filepath.Join(dir, "home"))
	t.Setenv("MEGAFLOYD_AUTH_HOST", "")
	t.Setenv("MEGAFLOYD_AUTH_BASE_URL", "")
	t.Setenv("FLOYD_CODE_BASE_URL", "")
	t.Setenv("FLOYD_CODE_OAUTH_HOST", "")
	t.Setenv("FLOYD_OAUTH_HOST", "")
	providers := csync.NewMap[string, ProviderConfig]()
	providers.Set("opencode-go", ProviderConfig{ID: "opencode-go", APIKey: "original-beam-fixture"})
	selected := SelectedModel{Provider: "opencode-go", Model: "beam-free", MaxTokens: 131072, ReasoningEffort: "medium"}
	cfg := &Config{Providers: providers, Options: &Options{CompanyPlatformURL: base}, Models: map[SelectedModelType]SelectedModel{SelectedModelTypeLarge: selected}}
	s := NewTestStoreWithWorkingDir(cfg, dir)
	s.globalDataPath = filepath.Join(dir, "home", "data", "floyd.json")
	s.workspacePath = filepath.Join(dir, ".megafloyd", "floyd.json")
	return s, selected
}
func TestCanonicalManagedAccountUsesNativeCredentialSlot(t *testing.T) {
	s, selected := canonicalAccountStore(t, "https://company.example")
	token := &oauth.Token{AccessToken: "account-access-fixture", RefreshToken: "account-refresh-fixture", ExpiresAt: time.Now().Add(time.Hour).Unix(), ExpiresIn: 3600, Client: &oauth.OAuthClient{ClientID: megafloydauth.DefaultClientID, TokenURL: "https://company.example/api/oauth/token"}}
	if err := s.SaveManagedAccount(ManagedAccountLogin{BaseURL: "https://company.example", OAuthHost: "https://company.example", Token: token}); err != nil {
		t.Fatal(err)
	}
	account, err := s.ManagedAccount("")
	if err != nil {
		t.Fatal(err)
	}
	if !account.Authenticated {
		t.Fatalf("account status: %+v", account)
	}
	nativeCfg, err := megafloydauth.WithStoredEndpoints(megafloydauth.FromEnv(nil))
	if err != nil {
		t.Fatal(err)
	}
	nativeCfg.Version = "test"
	native, err := megafloydauth.NewManager(nativeCfg)
	if err != nil {
		t.Fatal(err)
	}
	status, err := native.Status()
	if err != nil || !status.LoggedIn {
		t.Fatalf("native/web disagree: %+v %v", status, err)
	}
	actual, err := native.AccessToken(context.Background())
	if err != nil || actual != token.AccessToken {
		t.Fatal("native provider did not read the web login credential")
	}
	encoded, _ := json.Marshal(account)
	if strings.Contains(string(encoded), "fixture") {
		t.Fatal("account response exposed token")
	}
	stored, err := os.ReadFile(s.globalDataPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "account-access-fixture") || strings.Contains(string(stored), "account-refresh-fixture") {
		t.Fatal("duplicated native account token in Go config")
	}
	if !strings.Contains(string(stored), CompanyAuthAPIKeyScheme) {
		t.Fatal("provider did not use native credential placeholder")
	}
	if !reflect.DeepEqual(s.Config().Models[SelectedModelTypeLarge], selected) {
		t.Fatal("login changed selected model settings")
	}
	beam, _ := s.Config().Providers.Get("opencode-go")
	if beam.APIKey != "original-beam-fixture" {
		t.Fatal("login changed Beam credentials")
	}
	if err := s.LogoutManagedAccount(""); err != nil {
		t.Fatal(err)
	}
	status, err = native.Status()
	if err != nil || status.LoggedIn {
		t.Fatal("native logout disagrees with web")
	}
	account, err = s.ManagedAccount("")
	if err != nil || account.Authenticated {
		t.Fatal("web logout disagrees with native")
	}
	if !reflect.DeepEqual(s.Config().Models[SelectedModelTypeLarge], selected) {
		t.Fatal("logout changed model selection")
	}
}
func TestCanonicalManagedAccountRefreshesNativeSlot(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/oauth/token" {
			t.Error("wrong native token endpoint")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Error("wrong refresh token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"token_type":"Bearer"}`))
	}))
	defer server.Close()
	s, _ := canonicalAccountStore(t, server.URL)
	token := &oauth.Token{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Minute).Unix(), ExpiresIn: 3600}
	if err := s.SaveManagedAccount(ManagedAccountLogin{BaseURL: server.URL, OAuthHost: server.URL, Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshManagedAccount(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.ManagedAccountToken("")
	if err != nil || got == nil || got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" || calls != 1 {
		t.Fatalf("native refresh failed; calls=%d err=%v", calls, err)
	}
	stored, _ := os.ReadFile(s.globalDataPath)
	if strings.Contains(string(stored), "new-access") {
		t.Fatal("refresh duplicated token in config")
	}
}
func TestCanonicalManagedAccountFailedConfigWriteRollsBackSlot(t *testing.T) {
	s, selected := canonicalAccountStore(t, "https://company.example")
	s.globalDataPath = t.TempDir()
	err := s.SaveManagedAccount(ManagedAccountLogin{BaseURL: "https://company.example", OAuthHost: "https://company.example", Token: &oauth.Token{AccessToken: "failed-token", ExpiresAt: time.Now().Add(time.Hour).Unix(), ExpiresIn: 3600}})
	if err == nil {
		t.Fatal("expected config write failure")
	}
	got, getErr := s.ManagedAccountToken("")
	if getErr != nil || got != nil {
		t.Fatal("failed login left native credential behind")
	}
	if _, exists := s.Config().Providers.Get(ManagedAccountProvider); exists {
		t.Fatal("failed login changed live provider map")
	}
	if !reflect.DeepEqual(s.Config().Models[SelectedModelTypeLarge], selected) {
		t.Fatal("failed login changed model selection")
	}
}
func TestCanonicalAdditionalDirectoryPersistence(t *testing.T) {
	dir := t.TempDir()
	ephemeral := t.TempDir()
	persisted := t.TempDir()
	canonicalPersisted, err := filepath.EvalSymlinks(persisted)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Options: &Options{}, Providers: csync.NewMap[string, ProviderConfig]()}
	s := NewTestStoreWithWorkingDir(cfg, dir)
	s.workspacePath = filepath.Join(dir, ".megafloyd", "floyd.json")
	if _, err := s.AddWorkspaceDirectory(ephemeral, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.workspacePath); !os.IsNotExist(err) {
		t.Fatal("ephemeral directory persisted")
	}
	result, err := s.AddWorkspaceDirectory(persisted, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AdditionalDirs) != 2 {
		t.Fatal("runtime did not retain both directories")
	}
	data, err := os.ReadFile(s.workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Options struct {
			Dirs []string `json:"additional_dirs"`
		} `json:"options"`
	}
	if err = json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Options.Dirs) != 1 || stored.Options.Dirs[0] != canonicalPersisted {
		t.Fatal("persisted an ephemeral directory")
	}
}
