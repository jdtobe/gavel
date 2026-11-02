package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
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
	t.Cleanup(func() { a.Stop(context.Background()) })
	url, err := a.StartConsole(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Setup(ctx, []string{appruntime.Model08B}, nil); err != nil {
		t.Fatal(err)
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
	if err := a.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exited, _ := manager.ExitError(); !exited {
		t.Fatal("sidecar still running")
	}
}
