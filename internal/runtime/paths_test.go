package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathsAt(t *testing.T) {
	home := t.TempDir()
	p, err := ResolvePathsAt(home)
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != home {
		t.Fatalf("Home = %q", p.Home)
	}
	want := map[string]string{
		"ConfigFile":       filepath.Join(home, "config.json"),
		"RuntimeDir":       filepath.Join(home, "runtime"),
		"LogsDir":          filepath.Join(home, "logs"),
		"CacheDir":         filepath.Join(home, "cache"),
		"HFHome":           filepath.Join(home, "cache", "huggingface"),
		"UVCacheDir":       filepath.Join(home, "cache", "uv"),
		"PythonDir":        filepath.Join(home, "runtime", "python"),
		"UVDir":            filepath.Join(home, "runtime", "uv"),
		"LockFile":         filepath.Join(home, "runtime", "gavel.lock"),
		"SidecarStateFile": filepath.Join(home, "runtime", "sidecar.json"),
	}
	got := map[string]string{
		"ConfigFile":       p.ConfigFile,
		"RuntimeDir":       p.RuntimeDir,
		"LogsDir":          p.LogsDir,
		"CacheDir":         p.CacheDir,
		"HFHome":           p.HFHome,
		"UVCacheDir":       p.UVCacheDir,
		"PythonDir":        p.PythonDir,
		"UVDir":            p.UVDir,
		"LockFile":         p.LockFile,
		"SidecarStateFile": p.SidecarStateFile,
	}
	for name, wantPath := range want {
		if got[name] != wantPath {
			t.Errorf("%s = %q, want %q", name, got[name], wantPath)
		}
	}
}

func TestResolvePathsUsesGavelHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GAVEL_HOME", home)
	p, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != home {
		t.Fatalf("Home = %q", p.Home)
	}
	if p.ConfigFile != filepath.Join(home, "config.json") {
		t.Fatalf("ConfigFile = %q", p.ConfigFile)
	}
}

func TestResolvePathsDefaultsToUserDirs(t *testing.T) {
	t.Setenv("GAVEL_HOME", "")
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	p, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != filepath.Join(configDir, "gavel") {
		t.Fatalf("Home = %q", p.Home)
	}
	if p.CacheDir != filepath.Join(cacheDir, "gavel") {
		t.Fatalf("CacheDir = %q", p.CacheDir)
	}
	if p.HFHome != filepath.Join(cacheDir, "gavel", "huggingface") {
		t.Fatalf("HFHome = %q", p.HFHome)
	}
}
