package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultModel is the model selected on first run.
const DefaultModel = "jaredpalmer/kev-0.8b@v1.0"

// Config is the on-disk Gavel configuration.
type Config struct {
	Model            string   `json:"model"`
	DownloadedModels []string `json:"downloaded_models"`
}

// Default returns the first-run configuration.
func Default() Config {
	return Config{Model: DefaultModel}
}

// Load reads a config file. A missing file returns Default().
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	return c, nil
}

// Save writes the config, creating parent directories and using mode 0600.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
