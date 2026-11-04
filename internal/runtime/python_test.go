package runtime

import (
	"os/exec"
	"testing"
)

func TestPythonPrefetchHelper(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 required for dependency-free helper regression tests")
	}
	output, err := exec.Command(python, "testdata/prefetch_helper_test.py").CombinedOutput()
	if err != nil {
		t.Fatalf("prefetch helper: %v\n%s", err, output)
	}
}
