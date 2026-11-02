package runtime

import (
	"io"
	"os"
	"path/filepath"
)

// NewSetup creates the real runtime dependencies. It never downloads anything.
func NewSetup(paths Paths) *Setup {
	return &Setup{Paths: paths, Pins: DefaultPins(), Runner: ExecRunner{}, Fetcher: HTTPFetcher{}, Log: io.Discard, DiskFree: defaultDiskFree, MemoryBytes: defaultMemoryBytes}
}

// Installed checks local runtime files only, without any network checks.
func (s *Setup) Installed() bool {
	if !s.kevMarkerValid(s.KevDir()) {
		return false
	}
	for _, path := range []string{filepath.Join(s.Paths.UVDir, "uv"), s.VenvPython()} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}
