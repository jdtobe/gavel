package web

import (
	"strings"
	"testing"
)

func TestAssetsEmbedded(t *testing.T) {
	for _, name := range []string{"index.html", "styles.css", "app.js", "model.js"} {
		if _, err := FS.ReadFile(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	index, _ := FS.ReadFile("index.html")
	for _, ref := range []string{"/static/app.js", "/static/model.js", `id="state"`, `id="questions"`, `id="answers"`, `id="run"`} {
		if !strings.Contains(string(index), ref) {
			t.Errorf("missing %s", ref)
		}
	}
}
