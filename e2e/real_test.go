package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jdtobe/gavel/internal/app"
	"github.com/jdtobe/gavel/internal/config"
	"github.com/jdtobe/gavel/internal/kevapi"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/sidecar"
)

// TestRealKev opts into multi-GB downloads and a real Metal model load.
func TestRealKev(t *testing.T) {
	if os.Getenv("GAVEL_E2E") != "1" {
		t.Skip("set GAVEL_E2E=1 to consent to the real Kev download and smoke test")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("requires macOS Apple Silicon")
	}
	t.Setenv("GAVEL_HOME", t.TempDir())
	p, err := appruntime.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	s := appruntime.NewSetup(p)
	manager := &sidecar.Manager{Paths: p, Python: s.VenvPython(), Log: io.Discard}
	a := app.New(app.Deps{Paths: p, Config: config.Default(), Runtime: s, Sidecar: manager})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		a.Stop(context.Background())
		if t.Failed() {
			t.Logf("Sidecar log tail:\n%s", strings.Join(manager.Logs(), "\n"))
		}
	})
	url, err := a.StartConsole(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Setup(ctx, []string{appruntime.Model08B}, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Status().Backend; got != "mlx" {
		t.Fatalf("expected real Metal/MLX backend, got %q", got)
	}
	body := `{"state":"My shoes arrived late, in the wrong size, and I was charged twice.","questions":{"department":{"type":"choice","instructions":"Which team should help?","criteria":{"returns":"Refunds","shipping":"Delivery","billing":"Charges"}},"escalate":{"type":"noul","instructions":"Does this need urgent human attention?"},"frustration":{"type":"score","instructions":"How frustrated?","criteria":["Calm","Frustrated","Very angry"]}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/systemone", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, data)
	}
	var response kevapi.Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	for id, kind := range map[string]kevapi.QuestionType{"department": kevapi.TypeChoice, "escalate": kevapi.TypeNoul, "frustration": kevapi.TypeScore} {
		if response.Answers[id].Type != kind {
			t.Fatalf("missing %s answer", kind)
		}
	}
	resp.Body.Close()
	// Trap proxy-aware external HTTP attempts during ordinary restart. This
	// complements the explicit offline-env unit tests without changing the host.
	var networkAttempts atomic.Int64
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkAttempts.Add(1)
		http.Error(w, "unexpected external request during offline launch", 502)
	}))
	defer trap.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, trap.URL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(key, "127.0.0.1,localhost")
	}
	if err := a.Restart(ctx); err != nil {
		t.Fatalf("offline restart: %v", err)
	}
	if a.Status().Backend != "mlx" {
		t.Fatal("offline restart lost MLX backend")
	}
	retry, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/systemone", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	retryResponse, err := http.DefaultClient.Do(retry)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(retryResponse.Body)
	retryResponse.Body.Close()
	if retryResponse.StatusCode != 200 {
		t.Fatalf("offline inference: %d %s", retryResponse.StatusCode, data)
	}
	if networkAttempts.Load() != 0 {
		t.Fatalf("ordinary restart attempted %d external HTTP requests", networkAttempts.Load())
	}
	t.Logf("Verified %s / %s; all three answer types; offline restart and inference; no proxy-observed network requests", a.Status().Backend, a.Status().Dtype)
	// A deliberately empty cache must fail offline, not silently download.
	// Point HF_HOME at a new empty directory rather than changing any cache files.
	if err := manager.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	manager.Paths.HFHome = t.TempDir()
	missingCtx, missingCancel := context.WithTimeout(ctx, 90*time.Second)
	err = a.Restart(missingCtx)
	missingCancel()
	if err == nil || !strings.Contains(err.Error(), "re-run setup") {
		t.Fatalf("missing cache should fail with setup guidance: %v", err)
	}
	if networkAttempts.Load() != 0 {
		t.Fatalf("incomplete offline cache attempted %d external HTTP requests", networkAttempts.Load())
	}
	saved, err := config.Load(p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Model != appruntime.Model08B {
		t.Fatal("failed offline start changed active model")
	}
	t.Log("Verified incomplete-cache error and unchanged active model with no proxy-observed network requests")
	if err := a.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exited, _ := manager.ExitError(); !exited {
		t.Fatal("sidecar still running")
	}
}
