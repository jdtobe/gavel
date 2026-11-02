package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Runner runs an external command.
type Runner interface {
	Run(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// Fetcher downloads a URL.
type Fetcher interface {
	Get(ctx context.Context, url string) (io.ReadCloser, error)
}

// HTTPFetcher downloads with net/http.
type HTTPFetcher struct {
	Client *http.Client
}

// Get implements Fetcher.
func (f HTTPFetcher) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// envFor returns the current environment with the given key/value pairs
// replacing any existing values.
func envFor(kv ...string) []string {
	env := os.Environ()
	for i := 0; i+1 < len(kv); i += 2 {
		key, value := kv[i], kv[i+1]
		prefix := key + "="
		replaced := false
		for j, entry := range env {
			if strings.HasPrefix(entry, prefix) {
				env[j] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}
	return env
}
