package megafloyd

import (
	"os"
	"testing"
)

func TestGetPrefersMegafloyd(t *testing.T) {
	clearEnv(t)
	t.Setenv(IdentityEnv, "mega")
	t.Setenv(ChassisIdentityEnv, "legacy")

	if got := Get("IDENTITY"); got != "mega" {
		t.Fatalf("Get(IDENTITY) = %q, want mega", got)
	}
}

func TestGetFallsBackToChassis(t *testing.T) {
	clearEnv(t)
	t.Setenv(ChassisIdentityEnv, "legacy")

	if got := Get("IDENTITY"); got != "legacy" {
		t.Fatalf("Get(IDENTITY) = %q, want legacy", got)
	}
}

func TestGetEmpty(t *testing.T) {
	clearEnv(t)

	if got := Get("IDENTITY"); got != "" {
		t.Fatalf("Get(IDENTITY) = %q, want empty", got)
	}
}

func TestChassisEnvDirectories(t *testing.T) {
	clearEnv(t)
	d := Dirs{Config: "/m/config", Data: "/m/data", Cache: "/m/cache"}
	env := ChassisEnv(d)

	for name, want := range map[string]string{
		ChassisGlobalConfigEnv: "/m/config",
		ChassisGlobalDataEnv:   "/m/data",
		ChassisCacheDirEnv:     "/m/cache",
		ChassisIdentityEnv:     DisplayName,
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
	for _, name := range []string{ChassisProfileEnv, ChassisSkillsDirEnv} {
		if _, ok := env[name]; ok {
			t.Errorf("%s should be absent when unset, got %q", name, env[name])
		}
	}
}

func TestChassisEnvMirrorsOptionalValues(t *testing.T) {
	clearEnv(t)
	t.Setenv(IdentityEnv, "megafloyd")
	t.Setenv(ProfileEnv, "1")
	t.Setenv(SkillsEnv, "/m/skills")

	env := ChassisEnv(Dirs{Config: "/m/config", Data: "/m/data", Cache: "/m/cache"})
	for name, want := range map[string]string{
		ChassisIdentityEnv:  "megafloyd",
		ChassisProfileEnv:   "1",
		ChassisSkillsDirEnv: "/m/skills",
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
}

// TestApplyChassisEnvOverwritesInheritedDirectories pins the post-rebrand
// contract: the MegaFloyd layout outranks any FLOYD_* value inherited from
// the chassis, while profile and skills keep their inherited values.
func TestApplyChassisEnvOverwritesInheritedDirectories(t *testing.T) {
	clearEnv(t)
	t.Setenv(ChassisGlobalConfigEnv, "/inherited/config")
	t.Setenv(ChassisGlobalDataEnv, "/inherited/data")
	t.Setenv(ChassisCacheDirEnv, "/inherited/cache")
	t.Setenv(ChassisIdentityEnv, "inherited identity")
	t.Setenv(ChassisProfileEnv, "1")
	t.Setenv(ChassisSkillsDirEnv, "/inherited/skills")

	ApplyChassisEnv(Dirs{Config: "/m/config", Data: "/m/data", Cache: "/m/cache"})

	for name, want := range map[string]string{
		ChassisGlobalConfigEnv: "/m/config",
		ChassisGlobalDataEnv:   "/m/data",
		ChassisCacheDirEnv:     "/m/cache",
		ChassisIdentityEnv:     DisplayName,
		ChassisProfileEnv:      "1",
		ChassisSkillsDirEnv:    "/inherited/skills",
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// TestApplyChassisEnvIsIdempotent checks that a repeated apply reproduces
// the first one: the same four mirror names, the same values, and no
// profile or skills export.
func TestApplyChassisEnvIsIdempotent(t *testing.T) {
	clearEnv(t)
	d := Dirs{Config: "/m/config", Data: "/m/data", Cache: "/m/cache"}
	ApplyChassisEnv(d)

	applied := ApplyChassisEnv(d)

	wantExported := []string{
		ChassisGlobalConfigEnv,
		ChassisGlobalDataEnv,
		ChassisCacheDirEnv,
		ChassisIdentityEnv,
	}
	if len(applied) != len(wantExported) {
		t.Fatalf("second apply exported %v, want only %v", applied, wantExported)
	}
	exported := make(map[string]bool, len(applied))
	for _, name := range applied {
		exported[name] = true
	}
	for _, name := range wantExported {
		if !exported[name] {
			t.Errorf("second apply did not export %s", name)
		}
	}

	for name, want := range map[string]string{
		ChassisGlobalConfigEnv: "/m/config",
		ChassisGlobalDataEnv:   "/m/data",
		ChassisCacheDirEnv:     "/m/cache",
		ChassisIdentityEnv:     DisplayName,
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q after second apply, want %q", name, got, want)
		}
	}
	if _, ok := os.LookupEnv(ChassisProfileEnv); ok {
		t.Errorf("%s was exported without %s set", ChassisProfileEnv, ProfileEnv)
	}
	if _, ok := os.LookupEnv(ChassisSkillsDirEnv); ok {
		t.Errorf("%s was exported without %s set", ChassisSkillsDirEnv, SkillsEnv)
	}
}

func TestApplyChassisEnvMegafloydWins(t *testing.T) {
	clearEnv(t)
	t.Setenv(ConfigEnv, "/m/config")
	t.Setenv(ChassisGlobalConfigEnv, "/inherited/config")

	d, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	ApplyChassisEnv(d)

	if got := os.Getenv(ChassisGlobalConfigEnv); got != "/m/config" {
		t.Fatalf("%s = %q, want /m/config", ChassisGlobalConfigEnv, got)
	}
}
