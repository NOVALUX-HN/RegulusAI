package httpapi

// H10: golden route list — Handler serves the documented /v1/* set and
// docs/dashboard.md names every served route. Adding a route without
// documenting it (or vice versa) trips here first.
import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoldenRouteList(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()
	routes := []string{
		"/v1/status", "/v1/version", "/v1/metrics", "/v1/almanac/summary",
		"/v1/evolution", "/v1/receipts", "/v1/executions", "/v1/git",
		"/v1/observations", "/v1/lessons", "/v1/traces", "/v1/budget",
		"/v1/projects",
	}
	for _, path := range routes {
		code, body := get(t, srv, path)
		if code == http.StatusNotFound {
			t.Fatalf("GET %s = 404, route must be served (%s)", path, body)
		}
	}
	_, thisFile, _, _ := runtime.Caller(0)
	doc, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "dashboard.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range routes {
		if !strings.Contains(string(doc), "GET "+path) {
			t.Fatalf("docs/dashboard.md must name %s", path)
		}
	}
}
