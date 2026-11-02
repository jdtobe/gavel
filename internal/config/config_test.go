package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultModel(t *testing.T) {
	if got := Default().Model; got != "jaredpalmer/kev-0.8b@v1.0" {
		t.Fatalf("Default().Model = %q", got)
	}
}

func TestDefaultDownloadedModelsEmpty(t *testing.T) {
	if got := Default().DownloadedModels; len(got) != 0 {
		t.Fatalf("Default().DownloadedModels = %#v", got)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != Default().Model {
		t.Fatalf("Model = %q", got.Model)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{
		Model:            "jaredpalmer/kev-4b@v1.0",
		DownloadedModels: []string{"jaredpalmer/kev-0.8b@v1.0", "jaredpalmer/kev-4b@v1.0"},
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != want.Model {
		t.Fatalf("Model = %q, want %q", got.Model, want.Model)
	}
	if len(got.DownloadedModels) != len(want.DownloadedModels) {
		t.Fatalf("DownloadedModels = %#v", got.DownloadedModels)
	}
	for i := range want.DownloadedModels {
		if got.DownloadedModels[i] != want.DownloadedModels[i] {
			t.Fatalf("DownloadedModels[%d] = %q", i, got.DownloadedModels[i])
		}
	}
}

func TestLoadIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"model":"m","downloaded_models":["x"],"future":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "m" || len(got.DownloadedModels) != 1 || got.DownloadedModels[0] != "x" {
		t.Fatalf("got = %#v", got)
	}
}

func TestLoadMalformedReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestSaveCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "config.json")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSaveFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
