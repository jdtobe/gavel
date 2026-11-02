package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type runCall struct {
	name string
	args []string
	env  []string
}

type fakeRunner struct {
	calls  []runCall
	stdout string
	err    error
	called bool
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error {
	f.called = true
	f.calls = append(f.calls, runCall{name: name, args: append([]string(nil), args...), env: append([]string(nil), env...)})
	if f.stdout != "" {
		_, _ = io.WriteString(stdout, f.stdout)
	}
	return f.err
}

type fakeFetcher struct {
	body []byte
	err  error
	urls []string
}

func (f *fakeFetcher) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	f.urls = append(f.urls, url)
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

type tarEntry struct {
	name string
	body string
	mode int64
}

func tarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: mode, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uvTarGz(t *testing.T) []byte {
	t.Helper()
	return tarGz(t, []tarEntry{{name: "uv-aarch64-apple-darwin/uv", body: "#!/bin/sh\necho uv\n", mode: 0o755}})
}

func kevTarGz(t *testing.T) []byte {
	t.Helper()
	prefix := "kev-" + DefaultPins().KevSHA + "/"
	return tarGz(t, []tarEntry{
		{name: prefix + "pyproject.toml", body: "[project]\nname = \"kev\"\n"},
		{name: prefix + "uv.lock", body: "version = 1\n"},
		{name: prefix + "kev/__init__.py", body: ""},
	})
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newTestSetup(t *testing.T) *Setup {
	t.Helper()
	paths, err := ResolvePathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Setup{
		Paths:       paths,
		Pins:        DefaultPins(),
		Runner:      &fakeRunner{stdout: "uv " + DefaultPins().UVVersion},
		Fetcher:     &fakeFetcher{},
		Log:         io.Discard,
		DiskFree:    func(string) (uint64, error) { return 1 << 40, nil },
		MemoryBytes: func() (uint64, error) { return 64 << 30, nil },
	}
}

func envHas(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func TestEnsureUVRejectsBadChecksum(t *testing.T) {
	s := newTestSetup(t)
	s.Pins.UVSHA256 = strings.Repeat("0", 64)
	s.Fetcher = &fakeFetcher{body: uvTarGz(t)}
	err := s.EnsureUV(context.Background())
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if s.Runner.(*fakeRunner).called {
		t.Fatal("runner must not run after a checksum mismatch")
	}
}

func TestEnsureUVInstallsAndVerifies(t *testing.T) {
	s := newTestSetup(t)
	body := uvTarGz(t)
	s.Fetcher = &fakeFetcher{body: body}
	s.Pins.UVSHA256 = sha256Hex(body)
	if err := s.EnsureUV(context.Background()); err != nil {
		t.Fatal(err)
	}

	uvPath := filepath.Join(s.Paths.UVDir, "uv")
	info, err := os.Stat(uvPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	fetcher := s.Fetcher.(*fakeFetcher)
	wantURL := "https://github.com/astral-sh/uv/releases/download/" + DefaultPins().UVVersion + "/uv-aarch64-apple-darwin.tar.gz"
	if len(fetcher.urls) != 1 || fetcher.urls[0] != wantURL {
		t.Fatalf("urls = %#v", fetcher.urls)
	}
	runner := s.Runner.(*fakeRunner)
	if len(runner.calls) != 1 || runner.calls[0].name != uvPath || strings.Join(runner.calls[0].args, " ") != "--version" {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestEnsureUVSkipsWhenVersionMatches(t *testing.T) {
	s := newTestSetup(t)
	if err := os.MkdirAll(s.Paths.UVDir, 0o755); err != nil {
		t.Fatal(err)
	}
	uvPath := filepath.Join(s.Paths.UVDir, "uv")
	if err := os.WriteFile(uvPath, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureUV(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Fetcher.(*fakeFetcher).urls); got != 0 {
		t.Fatalf("fetch calls = %d", got)
	}
	runner := s.Runner.(*fakeRunner)
	if len(runner.calls) != 1 || runner.calls[0].args[0] != "--version" {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestEnsureUVRejectsUnexpectedVersion(t *testing.T) {
	s := newTestSetup(t)
	body := uvTarGz(t)
	s.Fetcher = &fakeFetcher{body: body}
	s.Pins.UVSHA256 = sha256Hex(body)
	s.Runner = &fakeRunner{stdout: "uv 9.9.9"}
	err := s.EnsureUV(context.Background())
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsurePythonUsesUV(t *testing.T) {
	s := newTestSetup(t)
	if err := s.EnsurePython(context.Background()); err != nil {
		t.Fatal(err)
	}
	runner := s.Runner.(*fakeRunner)
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[0]
	if call.name != filepath.Join(s.Paths.UVDir, "uv") {
		t.Fatalf("name = %q", call.name)
	}
	if got := strings.Join(call.args, " "); got != "python install 3.13" {
		t.Fatalf("args = %q", got)
	}
	if !envHas(call.env, "UV_PYTHON_INSTALL_DIR="+s.Paths.PythonDir) {
		t.Fatalf("missing UV_PYTHON_INSTALL_DIR in %#v", call.env)
	}
	if !envHas(call.env, "UV_CACHE_DIR="+s.Paths.UVCacheDir) {
		t.Fatalf("missing UV_CACHE_DIR in %#v", call.env)
	}
}

func TestEnsureKevSyncs(t *testing.T) {
	s := newTestSetup(t)
	s.Fetcher = &fakeFetcher{body: kevTarGz(t)}
	if err := s.EnsureKev(context.Background()); err != nil {
		t.Fatal(err)
	}

	kevDir := s.KevDir()
	wantURL := "https://codeload.github.com/jaredpalmer/kev/tar.gz/" + DefaultPins().KevSHA
	fetcher := s.Fetcher.(*fakeFetcher)
	if len(fetcher.urls) != 1 || fetcher.urls[0] != wantURL {
		t.Fatalf("urls = %#v", fetcher.urls)
	}
	if _, err := os.Stat(filepath.Join(kevDir, "pyproject.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(kevDir, "uv.lock")); err != nil {
		t.Fatal(err)
	}

	runner := s.Runner.(*fakeRunner)
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[0]
	if call.name != filepath.Join(s.Paths.UVDir, "uv") {
		t.Fatalf("name = %q", call.name)
	}
	wantArgs := "--directory " + kevDir + " sync --frozen --no-dev --extra serve --python 3.13"
	if got := strings.Join(call.args, " "); got != wantArgs {
		t.Fatalf("args = %q, want %q", got, wantArgs)
	}
	if !envHas(call.env, "UV_PROJECT_ENVIRONMENT="+filepath.Join(kevDir, ".venv")) {
		t.Fatalf("missing UV_PROJECT_ENVIRONMENT in %#v", call.env)
	}
	if !envHas(call.env, "UV_PYTHON_INSTALL_DIR="+s.Paths.PythonDir) {
		t.Fatalf("missing UV_PYTHON_INSTALL_DIR in %#v", call.env)
	}
	if !envHas(call.env, "UV_CACHE_DIR="+s.Paths.UVCacheDir) {
		t.Fatalf("missing UV_CACHE_DIR in %#v", call.env)
	}

	marker, err := os.ReadFile(filepath.Join(kevDir, ".gavel-installed"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(marker), DefaultPins().KevSHA) {
		t.Fatalf("marker = %q", marker)
	}
}

func TestEnsureKevSkipsWhenMarkerMatches(t *testing.T) {
	s := newTestSetup(t)
	kevDir := s.KevDir()
	if err := os.MkdirAll(kevDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := `{"sha":"` + DefaultPins().KevSHA + `"}`
	if err := os.WriteFile(filepath.Join(kevDir, ".gavel-installed"), []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureKev(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Fetcher.(*fakeFetcher).urls); got != 0 {
		t.Fatalf("fetch calls = %d", got)
	}
	if got := len(s.Runner.(*fakeRunner).calls); got != 0 {
		t.Fatalf("runner calls = %d", got)
	}
}

func TestEnsureKevReinstallsWhenMarkerSHAStale(t *testing.T) {
	s := newTestSetup(t)
	kevDir := s.KevDir()
	if err := os.MkdirAll(kevDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kevDir, ".gavel-installed"), []byte(`{"sha":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Fetcher = &fakeFetcher{body: kevTarGz(t)}
	if err := s.EnsureKev(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Fetcher.(*fakeFetcher).urls); got != 1 {
		t.Fatalf("fetch calls = %d", got)
	}
	if got := len(s.Runner.(*fakeRunner).calls); got != 1 {
		t.Fatalf("runner calls = %d", got)
	}
}

func TestKevPaths(t *testing.T) {
	s := newTestSetup(t)
	if got := s.KevDir(); got != filepath.Join(s.Paths.RuntimeDir, "kev", DefaultPins().KevSHA) {
		t.Fatalf("KevDir = %q", got)
	}
	if got := s.VenvPython(); got != filepath.Join(s.KevDir(), ".venv", "bin", "python") {
		t.Fatalf("VenvPython = %q", got)
	}
}

func TestModelSize(t *testing.T) {
	cases := map[string]uint64{
		DefaultPins().Model08B: 1_710_000_000,
		DefaultPins().Model4B:  8_850_000_000,
	}
	for model, want := range cases {
		got, err := ModelSize(model)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("ModelSize(%q) = %d, want %d", model, got, want)
		}
	}
	if _, err := ModelSize("unknown"); err == nil {
		t.Fatal("expected an error for an unknown model")
	}
}

func TestModelMinMemory(t *testing.T) {
	cases := map[string]uint64{
		DefaultPins().Model08B: 8 << 30,
		DefaultPins().Model4B:  32 << 30,
	}
	for model, want := range cases {
		got, err := ModelMinMemory(model)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("ModelMinMemory(%q) = %d, want %d", model, got, want)
		}
	}
	if _, err := ModelMinMemory("unknown"); err == nil {
		t.Fatal("expected an error for an unknown model")
	}
}

func TestRequiredDisk(t *testing.T) {
	got, err := RequiredDisk([]string{DefaultPins().Model08B, DefaultPins().Model4B})
	if err != nil {
		t.Fatal(err)
	}
	want := RuntimeDiskBytes + 1_710_000_000 + 8_850_000_000
	if got != want {
		t.Fatalf("RequiredDisk = %d, want %d", got, want)
	}
	if _, err := RequiredDisk(nil); err == nil {
		t.Fatal("expected an error for an empty selection")
	}
}

func TestCheckDisk(t *testing.T) {
	s := newTestSetup(t)
	s.DiskFree = func(string) (uint64, error) { return 100, nil }
	if err := s.CheckDisk(s.Paths.RuntimeDir, 50); err != nil {
		t.Fatal(err)
	}
	err := s.CheckDisk(s.Paths.RuntimeDir, 200)
	if err == nil {
		t.Fatal("expected a shortfall error")
	}
	if !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), "free") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckDiskPropagatesError(t *testing.T) {
	s := newTestSetup(t)
	s.DiskFree = func(string) (uint64, error) { return 0, io.ErrUnexpectedEOF }
	if err := s.CheckDisk(s.Paths.RuntimeDir, 1); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCheckMemory(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		bytes   uint64
		wantErr bool
	}{
		{name: "0.8B at 8 GiB", model: DefaultPins().Model08B, bytes: 8 << 30},
		{name: "0.8B at 7 GiB", model: DefaultPins().Model08B, bytes: 7 << 30, wantErr: true},
		{name: "4B at 32 GiB", model: DefaultPins().Model4B, bytes: 32 << 30},
		{name: "4B at 16 GiB", model: DefaultPins().Model4B, bytes: 16 << 30, wantErr: true},
		{name: "unknown model", model: "unknown", bytes: 64 << 30, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSetup(t)
			s.MemoryBytes = func() (uint64, error) { return tc.bytes, nil }
			err := s.CheckMemory(tc.model)
			if tc.wantErr && err == nil {
				t.Fatal("expected an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}
