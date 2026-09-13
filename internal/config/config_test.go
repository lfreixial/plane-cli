package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoadAndPermissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "config.json")
	want := Config{BaseURL: "https://example.com", APIKey: "secret", Workspace: "team", Project: "ENG"}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got != want {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions: %o", info.Mode().Perm())
		}
	}
	want.Project = "OPS"
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(p)
	if got.Project != "OPS" {
		t.Fatal("replacement failed")
	}
}

func TestEnvDoesNotMutateStoredConfig(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "env-secret")
	t.Setenv("PLANE_WORKSPACE", "env-workspace")
	c := Config{APIKey: "stored-secret", Workspace: "stored-workspace"}
	effective := c.WithEnv()
	if effective.APIKey != "env-secret" || effective.Workspace != "env-workspace" {
		t.Fatal(effective)
	}
	if c.APIKey != "stored-secret" {
		t.Fatal("stored config mutated")
	}
}

func TestMissingAndMalformed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(p)
	if err != nil || c.BaseURL != "https://api.plane.so" {
		t.Fatalf("config=%v err=%v", c, err)
	}
	if err := os.WriteFile(p, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected malformed config error")
	}
}
