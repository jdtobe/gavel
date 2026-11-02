package runtime

import "testing"

func TestNewSetupHasDefaults(t *testing.T) {
	p, _ := ResolvePathsAt(t.TempDir())
	s := NewSetup(p)
	if s.Runner == nil || s.Fetcher == nil || s.DiskFree == nil || s.MemoryBytes == nil || s.Pins.KevSHA != KevSHA {
		t.Fatal("missing defaults")
	}
	if s.Installed() {
		t.Fatal("empty runtime installed")
	}
}
