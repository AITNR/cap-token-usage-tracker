package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFullModeDashboardControlLayoutInBrowser(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		browserTestUnavailable(t, "Node.js", err)
	}
	chrome, err := resolveBrowserTestChrome()
	if err != nil {
		browserTestUnavailable(t, "Google Chrome (set CHROME_PATH to its executable)", err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "node_modules", "playwright-core", "package.json")); err != nil {
		browserTestUnavailable(t, "playwright-core (run npm ci)", err)
	}

	htmlPath := filepath.Join(t.TempDir(), "dashboard.html")
	if err := os.WriteFile(htmlPath, []byte(fullDashboardHTML), 0o600); err != nil {
		t.Fatalf("write full dashboard response body: %v", err)
	}

	command := exec.Command(node, filepath.Join("..", "..", "test", "dashboard_api_key_layout.mjs"), htmlPath, chrome)
	command.Env = append(os.Environ(), "TZ=UTC")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("full-mode API Key layout browser regression failed: %v\n%s\nInstall the pinned browser-test dependency with `npm ci`; this test uses the installed Google Chrome and does not download a browser.", err, output)
	}
}
