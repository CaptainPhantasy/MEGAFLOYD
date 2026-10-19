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
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
	for _, name := range []string{ChassisIdentityEnv, ChassisProfileEnv, ChassisSkillsDirEnv} {
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

func TestApplyChassisEnvDoesNotClobber(t *testing.T) {
	clearEnv(t)
	t.Setenv(ChassisGlobalConfigEnv, "/explicit/config")

	applied := ApplyChassisEnv(Dirs{Config: "/m/config", Data: "/m/data", Cache: "/m/cache"})
	for _, name := range applied {
		if name == ChassisGlobalConfigEnv {
			t.Fatalf("ApplyChassisEnv overwrote an explicit %s", name)
		}
	}
	if got := os.Getenv(ChassisGlobalConfigEnv); got != "/explicit/config" {
		t.Fatalf("%s = %q, want /explicit/config", ChassisGlobalConfigEnv, got)
	}
	if got := os.Getenv(ChassisGlobalDataEnv); got != "/m/data" {
		t.Fatalf("%s = %q, want /m/data", ChassisGlobalDataEnv, got)
	}
	if got := os.Getenv(ChassisCacheDirEnv); got != "/m/cache" {
		t.Fatalf("%s = %q, want /m/cache", ChassisCacheDirEnv, got)
	}
}

func TestApplyChassisEnvIsIdempotent(t *testing.T) {
	clearEnv(t)
	d := Dirs{Config: "/m/config", Data: "/m/data", Cache: "/m/cache"}
	ApplyChassisEnv(d)

	applied := ApplyChassisEnv(d)
	if len(applied) != 0 {
		t.Fatalf("second apply exported %v, want none", applied)
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
