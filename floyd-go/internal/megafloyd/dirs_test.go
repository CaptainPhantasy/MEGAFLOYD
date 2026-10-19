package megafloyd

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnv unsets every variable this package reads and restores it after the
// test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		HomeEnv, ConfigEnv, DataEnv, CacheEnv, IdentityEnv,
		ProfileEnv, SkillsEnv, ActiveEnv,
		ChassisGlobalConfigEnv, ChassisGlobalDataEnv, ChassisCacheDirEnv,
		ChassisIdentityEnv, ChassisProfileEnv, ChassisSkillsDirEnv,
	} {
		old, had := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(name, old)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}

func TestHomeDirDefault(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", "/home/tester")

	got, err := HomeDir()
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}
	want := filepath.Join("/home/tester", ".megafloyd")
	if got != want {
		t.Fatalf("HomeDir = %q, want %q", got, want)
	}
}

func TestHomeDirOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv(HomeEnv, "~/alt-megafloyd")

	got, err := HomeDir()
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	want := filepath.Join(userHome, "alt-megafloyd")
	if got != want {
		t.Fatalf("HomeDir = %q, want %q", got, want)
	}
}

func TestResolveDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", "/home/tester")

	d, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	home := filepath.Join("/home/tester", ".megafloyd")
	for _, tc := range []struct{ got, want, label string }{
		{d.Home, home, "Home"},
		{d.Config, filepath.Join(home, "config"), "Config"},
		{d.Data, filepath.Join(home, "data"), "Data"},
		{d.Cache, filepath.Join(home, "cache"), "Cache"},
		{d.State, filepath.Join(home, "state"), "State"},
		{d.Logs, filepath.Join(home, "logs"), "Logs"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.label, tc.got, tc.want)
		}
	}
}

func TestResolveMegafloydOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv(HomeEnv, "/srv/megafloyd")
	t.Setenv(ConfigEnv, "/srv/cfg")
	t.Setenv(DataEnv, "/srv/data")
	t.Setenv(CacheEnv, "/srv/cache")

	d, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Config != "/srv/cfg" || d.Data != "/srv/data" || d.Cache != "/srv/cache" {
		t.Fatalf("overrides not honored: %+v", d)
	}
}

func TestResolveChassisFallback(t *testing.T) {
	clearEnv(t)
	t.Setenv(HomeEnv, "/srv/megafloyd")
	t.Setenv(ChassisGlobalConfigEnv, "/legacy/config")
	t.Setenv(ChassisGlobalDataEnv, "/legacy/data")
	t.Setenv(ChassisCacheDirEnv, "/legacy/cache")

	d, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Config != "/legacy/config" || d.Data != "/legacy/data" || d.Cache != "/legacy/cache" {
		t.Fatalf("chassis fallbacks not honored: %+v", d)
	}
}

func TestResolveMegafloydWinsOverChassis(t *testing.T) {
	clearEnv(t)
	t.Setenv(HomeEnv, "/srv/megafloyd")
	t.Setenv(ConfigEnv, "/new/config")
	t.Setenv(ChassisGlobalConfigEnv, "/legacy/config")

	d, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Config != "/new/config" {
		t.Fatalf("Config = %q, want /new/config", d.Config)
	}
}

func TestEnsure(t *testing.T) {
	clearEnv(t)
	base := t.TempDir()
	t.Setenv(HomeEnv, base)

	d, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := d.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for _, dir := range d.All() {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}
}
