package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Setup bootstraps the pinned uv, Python, and Kev runtime.
type Setup struct {
	Paths       Paths
	Pins        Pins
	Runner      Runner
	Fetcher     Fetcher
	Log         io.Writer
	DiskFree    func(string) (uint64, error)
	MemoryBytes func() (uint64, error)
}

// KevDir is the extracted Kev source directory.
func (s *Setup) KevDir() string {
	return filepath.Join(s.Paths.RuntimeDir, "kev", s.Pins.KevSHA)
}

// VenvPython is the Python interpreter inside Kev's uv-managed venv.
func (s *Setup) VenvPython() string {
	return filepath.Join(s.KevDir(), ".venv", "bin", "python")
}

// EnsureUV installs and verifies the pinned uv binary.
func (s *Setup) EnsureUV(ctx context.Context) error {
	uvPath := filepath.Join(s.Paths.UVDir, "uv")
	if _, err := os.Stat(uvPath); err == nil {
		out, runErr := s.runCapture(ctx, uvPath, "--version")
		if runErr == nil && strings.Contains(out, s.Pins.UVVersion) {
			return nil
		}
		if runErr != nil && (errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)) {
			return runErr
		}
	}

	url := fmt.Sprintf("https://github.com/astral-sh/uv/releases/download/%s/uv-aarch64-apple-darwin.tar.gz", s.Pins.UVVersion)
	tmp, sum, err := s.download(ctx, url, s.Paths.UVDir)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	if !strings.EqualFold(sum, s.Pins.UVSHA256) {
		return fmt.Errorf("uv checksum mismatch: expected %s, got %s", s.Pins.UVSHA256, sum)
	}
	if err := extractUV(tmp, uvPath); err != nil {
		return err
	}

	out, err := s.runCapture(ctx, uvPath, "--version")
	if err != nil {
		return fmt.Errorf("verify uv: %w", err)
	}
	if !strings.Contains(out, s.Pins.UVVersion) {
		return fmt.Errorf("uv version = %q, want %q", strings.TrimSpace(out), s.Pins.UVVersion)
	}
	return nil
}

// EnsurePython installs Python 3.13 into the app runtime directory.
func (s *Setup) EnsurePython(ctx context.Context) error {
	if err := os.MkdirAll(s.Paths.PythonDir, 0o755); err != nil {
		return err
	}
	env := envFor(
		"UV_PYTHON_INSTALL_DIR", s.Paths.PythonDir,
		"UV_CACHE_DIR", s.Paths.UVCacheDir,
	)
	args := []string{"python", "install", "3.13"}
	if err := s.Runner.Run(ctx, filepath.Join(s.Paths.UVDir, "uv"), args, env, s.Log, s.Log); err != nil {
		return fmt.Errorf("uv python install: %w", err)
	}
	return nil
}

// EnsureKev downloads the pinned Kev source and syncs its locked dependencies.
func (s *Setup) EnsureKev(ctx context.Context) error {
	kevDir := s.KevDir()
	if s.kevMarkerValid(kevDir) {
		return nil
	}

	url := "https://codeload.github.com/jaredpalmer/kev/tar.gz/" + s.Pins.KevSHA
	tmp, _, err := s.download(ctx, url, filepath.Dir(kevDir))
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	if err := extractTree(tmp, kevDir); err != nil {
		return err
	}

	env := envFor(
		"UV_PROJECT_ENVIRONMENT", filepath.Join(kevDir, ".venv"),
		"UV_PYTHON_INSTALL_DIR", s.Paths.PythonDir,
		"UV_CACHE_DIR", s.Paths.UVCacheDir,
	)
	args := []string{
		"--directory", kevDir,
		"sync", "--frozen", "--no-dev", "--extra", "serve", "--python", "3.13",
	}
	if err := s.Runner.Run(ctx, filepath.Join(s.Paths.UVDir, "uv"), args, env, s.Log, s.Log); err != nil {
		return fmt.Errorf("uv sync: %w", err)
	}

	marker := fmt.Sprintf(`{"sha":%q,"installed_at":%q}`+"\n", s.Pins.KevSHA, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(kevDir, ".gavel-installed"), []byte(marker), 0o600); err != nil {
		return err
	}
	return nil
}

func (s *Setup) kevMarkerValid(kevDir string) bool {
	data, err := os.ReadFile(filepath.Join(kevDir, ".gavel-installed"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), s.Pins.KevSHA)
}

func (s *Setup) runCapture(ctx context.Context, name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	if err := s.Runner.Run(ctx, name, args, nil, &stdout, &stderr); err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (s *Setup) download(ctx context.Context, url, dir string) (path, sum string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	body, err := s.Fetcher.Get(ctx, url)
	if err != nil {
		return "", "", fmt.Errorf("download %s: %w", url, err)
	}
	defer body.Close()

	f, err := os.CreateTemp(dir, ".gavel-download-*")
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), body); err != nil {
		os.Remove(f.Name())
		return "", "", fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Sync(); err != nil {
		os.Remove(f.Name())
		return "", "", err
	}
	return f.Name(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func extractUV(tarPath, dst string) error {
	tr, closeFn, err := openTar(tarPath)
	if err != nil {
		return err
	}
	defer closeFn()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if filepath.Base(hdr.Name) != "uv" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Chmod(dst, 0o755)
	}
	return fmt.Errorf("uv binary not found in archive")
}

func extractTree(tarPath, dst string) error {
	tr, closeFn, err := openTar(tarPath)
	if err != nil {
		return err
	}
	defer closeFn()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		rel, ok := stripFirstComponent(hdr.Name)
		if !ok || rel == "" {
			continue
		}
		target, err := safeJoin(dst, rel)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode().Perm()
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func openTar(path string) (*tar.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	closeFn := func() {
		gz.Close()
		f.Close()
	}
	return tar.NewReader(gz), closeFn, nil
}

func stripFirstComponent(name string) (string, bool) {
	i := strings.IndexByte(name, '/')
	if i < 0 {
		return "", false
	}
	return name[i+1:], true
}

func safeJoin(root, rel string) (string, error) {
	clean := filepath.Clean(rel)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", rel)
	}
	dst := filepath.Join(root, clean)
	if !strings.HasPrefix(dst, root+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", rel)
	}
	return dst, nil
}
