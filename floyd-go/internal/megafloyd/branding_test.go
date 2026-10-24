package megafloyd

import "testing"

func TestCanonicalDisplayIdentityKeepsCommandAndPaths(t *testing.T) {
	t.Setenv(IdentityEnv, "")
	d := Dirs{Config: "/own/config", Data: "/own/data", Cache: "/own/cache"}
	got := ChassisEnv(d)
	if got[ChassisIdentityEnv] != "MegaFloyd" {
		t.Fatalf("identity = %q", got[ChassisIdentityEnv])
	}
	if Name != "megafloyd" || DefaultHomeDirName != ".megafloyd" {
		t.Fatal("display name changed executable or state path")
	}
	t.Setenv(IdentityEnv, "Owner persona")
	if ChassisEnv(d)[ChassisIdentityEnv] != "Owner persona" {
		t.Fatal("explicit identity was overwritten")
	}
}
