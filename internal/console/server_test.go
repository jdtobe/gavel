package console

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jdtobe/gavel/internal/kevapi"
	"github.com/jdtobe/gavel/internal/status"
)

type fakeProvider struct{ value status.Status }

func (p fakeProvider) Status() status.Status { return p.value }
func readyProvider() fakeProvider {
	return fakeProvider{status.Status{Phase: status.PhaseReady, Model: "m"}}
}

const validBody = `{"state":"ticket","questions":{"q":{"type":"noul","instructions":"urgent?"}}}`

func TestStatusServesPhase(t *testing.T) {
	s := New(readyProvider(), func() string { return "" }, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/status", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"phase":"ready"`) {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}
func TestRootServesIndex(t *testing.T) {
	s := New(readyProvider(), func() string { return "" }, nil)
	for _, path := range []string{"/", "/static/styles.css", "/static/app.js"} {
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != 200 {
			t.Fatalf("%s: %d", path, rr.Code)
		}
		if path == "/" && (!strings.Contains(rr.Body.String(), "<!doctype html>") || !strings.Contains(rr.Header().Get("Content-Type"), "text/html")) {
			t.Fatal(rr.Body.String())
		}
	}
}

func TestProxy(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		phase      status.Phase
		code, want int
	}{
		{"valid", validBody, status.PhaseReady, 200, 200},
		{"invalid", `{"state":" "}`, status.PhaseReady, 200, 400},
		{"unready", validBody, status.PhaseStarting, 200, 503},
		{"too large", `{"state":"` + strings.Repeat("x", MaxBodyBytes) + `"}`, status.PhaseReady, 200, 413},
		{"trailing JSON", validBody + `{}`, status.PhaseReady, 200, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/v1/systemone" {
					t.Errorf("%s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("authorization forwarded")
				}
				var req kevapi.Request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Model != kevapi.ModelName {
					t.Errorf("model = %s", req.Model)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-typesafe-request-id", "req-1")
				w.WriteHeader(tc.code)
				io.WriteString(w, `{"answers":{}}`)
			}))
			defer up.Close()
			s := New(fakeProvider{status.Status{Phase: tc.phase}}, func() string { return up.URL }, nil)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/systemone", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "secret")
			s.Handler().ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("%d %s", rr.Code, rr.Body.String())
			}
			if tc.want == 200 {
				if calls != 1 || rr.Header().Get("x-typesafe-request-id") != "req-1" {
					t.Fatal("missing upstream response")
				}
			} else if calls != 0 {
				t.Fatal("invalid request forwarded")
			}
		})
	}
}

func TestProxyPassesThrough422ForOverlongState(t *testing.T) {
	const body = `{"detail":"state too long"}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(422); io.WriteString(w, body) }))
	defer up.Close()
	s := New(readyProvider(), func() string { return up.URL }, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/systemone", strings.NewReader(validBody)))
	if rr.Code != 422 || rr.Body.String() != body {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}
func TestProxyConnectionFailure(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	up.Close()
	s := New(readyProvider(), func() string { return up.URL }, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/systemone", strings.NewReader(validBody)))
	if rr.Code != 502 {
		t.Fatal(rr.Code)
	}
}
func TestWaitIdleConcurrent(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		io.WriteString(w, `{}`)
	}))
	defer up.Close()
	s := New(readyProvider(), func() string { return up.URL }, nil)
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			rr := httptest.NewRecorder()
			s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/systemone", strings.NewReader(validBody)))
			done <- struct{}{}
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("requests not concurrent")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitIdle(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- s.WaitIdle(context.Background()) }()
	select {
	case <-wait:
		t.Fatal("returned with requests active")
	default:
	}
	close(release)
	for range 2 {
		<-done
	}
	if err := <-wait; err != nil {
		t.Fatal(err)
	}
}
func TestStartBindsLoopback(t *testing.T) {
	s := New(readyProvider(), func() string { return "" }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url, err := s.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatal(url)
	}
	resp, err := http.Get(url + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}
func TestModels(t *testing.T) {
	for _, phase := range []status.Phase{status.PhaseReady, status.PhaseStarting} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/models" {
				t.Error(r.URL.Path)
			}
			io.WriteString(w, `{"models":[]}`)
		}))
		s := New(fakeProvider{status.Status{Phase: phase, Model: "m"}}, func() string { return up.URL }, nil)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/models", nil))
		up.Close()
		if rr.Code != 200 {
			t.Fatal(rr.Code)
		}
		if phase != status.PhaseReady && !strings.Contains(rr.Body.String(), `"ready":false`) {
			t.Fatal(rr.Body.String())
		}
	}
}

func TestRestartCancellationReturns503(t *testing.T) {
	entered := make(chan struct{}, 2)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		if r.Header.Get("unused") == "" {
			<-r.Context().Done()
		}
	}))
	defer up.Close()
	s := New(readyProvider(), func() string { return up.URL }, nil)
	codes := make(chan int, 2)
	for range 2 {
		go func() {
			rr := httptest.NewRecorder()
			s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/systemone", strings.NewReader(validBody)))
			codes <- rr.Code
		}()
	}
	for range 2 {
		<-entered
	}
	s.CancelInflight()
	for range 2 {
		select {
		case code := <-codes:
			if code != 503 {
				t.Fatalf("restart cancellation returned %d", code)
			}
		case <-time.After(time.Second):
			t.Fatal("request did not cancel")
		}
	}
	if err := s.WaitIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cancelledBody struct {
	ctx     context.Context
	entered chan struct{}
}

func (b cancelledBody) Read([]byte) (int, error) {
	close(b.entered)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b cancelledBody) Close() error { return nil }
func TestRestartCancelsResponseBodyBeforeCommittingStatus(t *testing.T) {
	entered := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: cancelledBody{r.Context(), entered}}, nil
	})}
	s := New(readyProvider(), func() string { return "http://unused" }, client)
	done := make(chan int, 1)
	go func() {
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/systemone", strings.NewReader(validBody)))
		done <- rr.Code
	}()
	<-entered
	s.CancelInflight()
	select {
	case code := <-done:
		if code != 503 {
			t.Fatalf("partial response returned %d instead of 503", code)
		}
	case <-time.After(time.Second):
		t.Fatal("body not cancelled")
	}
}
