package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ModelInfo struct {
	Name    string `json:"name"`
	Backend string `json:"backend"`
	Dtype   string `json:"dtype"`
	Device  string `json:"device"`
}

// WaitReady polls health for up to timeout, detecting early child exits.
func (m *Manager) WaitReady(ctx context.Context, timeout time.Duration) (ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := m.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	var lastErr error
	for {
		if exited, err := m.ExitError(); exited {
			return ModelInfo{}, fmt.Errorf("Kev exited: %v; model cache may be incomplete; re-run setup", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL()+"/v1/models", nil)
		if err != nil {
			return ModelInfo{}, err
		}
		resp, err := client.Do(req)
		if err == nil {
			var data struct {
				Models []ModelInfo `json:"models"`
			}
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("health: %s", resp.Status)
			} else {
				err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
				if err == nil && len(data.Models) == 0 {
					err = fmt.Errorf("health: no models")
				}
			}
			resp.Body.Close()
			if err == nil {
				return data.Models[0], nil
			}
		}
		lastErr = err
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ModelInfo{}, fmt.Errorf("Kev not ready: %v: %w", lastErr, ctx.Err())
		case <-timer.C:
		}
	}
}
