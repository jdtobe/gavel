package status

import "testing"

func TestPhaseJSONValues(t *testing.T) {
	for p, want := range map[Phase]string{PhaseSetup: "setup", PhaseDownloading: "downloading", PhaseStarting: "starting", PhaseReady: "ready", PhaseError: "error", PhaseStopping: "stopping"} {
		if string(p) != want {
			t.Fatalf("%q != %q", p, want)
		}
	}
}
