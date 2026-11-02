package runtime

import "fmt"

const (
	// KevSHA is the pinned Kev source revision.
	KevSHA = "2074cc44256200bb37540103fa4572901faa10f3"
	// UVVersion is the pinned uv release.
	UVVersion = "0.13.5"
	// UVSHA256 is the pinned uv-aarch64-apple-darwin tarball checksum.
	UVSHA256 = "743f85035a1996ecb350659e01cd9faea9709829be09b1b4b03c73eaf3c9cd73"
	// Model08B is the default model revision.
	Model08B = "jaredpalmer/kev-0.8b@v1.0"
	// Model4B is the opt-in model revision.
	Model4B = "jaredpalmer/kev-4b@v1.0"
)

// Pins holds the versions Gavel installs and verifies.
type Pins struct {
	KevSHA    string
	UVVersion string
	UVSHA256  string
	Model08B  string
	Model4B   string
}

// DefaultPins returns the pinned runtime values.
func DefaultPins() Pins {
	return Pins{
		KevSHA:    KevSHA,
		UVVersion: UVVersion,
		UVSHA256:  UVSHA256,
		Model08B:  Model08B,
		Model4B:   Model4B,
	}
}

const (
	model08BBytes uint64 = 1_710_000_000
	model4BBytes  uint64 = 8_850_000_000
	minMemory08B  uint64 = 8 << 30
	minMemory4B   uint64 = 32 << 30
	// RuntimeDiskBytes covers the Python/Kev runtime plus download headroom.
	RuntimeDiskBytes uint64 = 2_684_354_560
)

// ModelSize returns the approximate download size for a model revision.
func ModelSize(model string) (uint64, error) {
	switch model {
	case Model08B:
		return model08BBytes, nil
	case Model4B:
		return model4BBytes, nil
	default:
		return 0, fmt.Errorf("unknown model %q", model)
	}
}

// ModelMinMemory returns the unified-memory floor for a model revision.
func ModelMinMemory(model string) (uint64, error) {
	switch model {
	case Model08B:
		return minMemory08B, nil
	case Model4B:
		return minMemory4B, nil
	default:
		return 0, fmt.Errorf("unknown model %q", model)
	}
}

// RequiredDisk sums model downloads plus the runtime and headroom.
func RequiredDisk(models []string) (uint64, error) {
	if len(models) == 0 {
		return 0, fmt.Errorf("no models selected")
	}
	total := RuntimeDiskBytes
	for _, model := range models {
		size, err := ModelSize(model)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}
