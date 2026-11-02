// Package status defines the shared app state exposed to the TUI and browser.
package status

type Phase string

const (
	PhaseSetup       Phase = "setup"
	PhaseDownloading Phase = "downloading"
	PhaseStarting    Phase = "starting"
	PhaseReady       Phase = "ready"
	PhaseError       Phase = "error"
	PhaseStopping    Phase = "stopping"
)

type Status struct {
	Phase      Phase  `json:"phase"`
	Model      string `json:"model"`
	Backend    string `json:"backend"`
	Dtype      string `json:"dtype"`
	SidecarURL string `json:"sidecar_url"`
	Error      string `json:"error"`
}
type Provider interface{ Status() Status }
type ModelState struct {
	Name           string
	Cached, Active bool
}
