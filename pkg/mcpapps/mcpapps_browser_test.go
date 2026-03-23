//go:build browser

package mcpapps

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/suite"
)

const testHarnessHTML = `<!DOCTYPE html>
<html><head><meta charset="UTF-8"></head><body>
<iframe id="viewer" src="viewer.html" style="width:100%;height:100vh;border:none;"></iframe>
<script>
window.addEventListener('message', function(e) {
    var msg = e.data;
    if (!msg || msg.jsonrpc !== '2.0') return;
    if (msg.method === 'ui/initialize' && msg.id != null) {
        e.source.postMessage({
            jsonrpc: '2.0', id: msg.id,
            result: {
                hostContext: {
                    theme: 'light',
                    toolInfo: { tool: { name: 'test_tool' } }
                }
            }
        }, '*');
    }
});
window.sendToolResult = function(data) {
    document.getElementById('viewer').contentWindow.postMessage({
        jsonrpc: '2.0',
        method: 'ui/notifications/tool-result',
        params: data
    }, '*');
};
window.sendNotification = function(method, params) {
    document.getElementById('viewer').contentWindow.postMessage({
        jsonrpc: '2.0',
        method: method,
        params: params
    }, '*');
};
</script>
</body></html>`

type BrowserSuite struct {
	suite.Suite
	browser *rod.Browser
	server  *httptest.Server
}

func (s *BrowserSuite) SetupSuite() {
	mux := http.NewServeMux()
	mux.HandleFunc("/harness", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := fmt.Fprint(w, testHarnessHTML)
		s.Require().NoError(err)
	})
	mux.HandleFunc("/viewer.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := fmt.Fprint(w, ViewerHTMLForTool("test_tool"))
		s.Require().NoError(err)
	})
	s.server = httptest.NewServer(mux)
	s.browser = rod.New().MustConnect()
}

func (s *BrowserSuite) TearDownSuite() {
	if s.browser != nil {
		s.browser.MustClose()
	}
	if s.server != nil {
		s.server.Close()
	}
}

// openViewer opens a new browser page with the test harness and returns
// the harness page and the viewer iframe. The viewer has completed the
// MCP protocol handshake and is showing "Waiting for tool result..." when
// this method returns.
func (s *BrowserSuite) openViewer() (*rod.Page, *rod.Page) {
	page := s.browser.MustPage(s.server.URL + "/harness")
	frame := page.MustElement("#viewer").MustFrame()
	frame.MustElementR(".status", "Waiting for tool result")
	return page, frame
}

// screenshotsDir is the output directory for visual captures.
// Located under _output/ which is already gitignored.
const screenshotsDir = "_output/screenshots"

// screenshot captures the viewer iframe as a PNG and saves it to _output/screenshots/<name>.png.
func (s *BrowserSuite) screenshot(page *rod.Page, name string) {
	s.T().Helper()
	dir := filepath.Join(findRepoRoot(s.T()), screenshotsDir)
	s.Require().NoError(os.MkdirAll(dir, 0o755))
	data, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	s.Require().NoError(err)
	path := filepath.Join(dir, name+".png")
	s.Require().NoError(os.WriteFile(path, data, 0o644))
	s.T().Logf("screenshot saved: %s", path)
}

// findRepoRoot walks up from the test binary's working directory to find go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

func (s *BrowserSuite) TestProtocolHandshake() {
	s.Run("viewer shows ready state after initialization", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		el := frame.MustElement(".status")
		s.Equal("Waiting for tool result...", el.MustText())
	})
}

func (s *BrowserSuite) TestTableView() {
	s.Run("renders table with correct number of rows", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [
					{name: "pod-1", namespace: "default"},
					{name: "pod-2", namespace: "kube-system"},
					{name: "pod-3", namespace: "default"}
				]
			}
		})`)
		frame.MustElement("table")
		rows := frame.MustElements("table tbody tr")
		s.Len(rows, 3)
	})

	s.Run("renders correct column headers from data keys", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "pod-1", namespace: "default"}]
			}
		})`)
		headers := frame.MustElements("table thead th")
		s.Len(headers, 2)
		s.Contains(headers[0].MustText(), "name")
		s.Contains(headers[1].MustText(), "namespace")
	})

	s.Run("renders correct cell values", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "my-pod", namespace: "production"}]
			}
		})`)
		frame.MustElement("table")
		cells := frame.MustElements("table tbody td")
		s.Equal("my-pod", cells[0].MustText())
		s.Equal("production", cells[1].MustText())
	})

	s.Run("displays item count for multiple items", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "a"}, {name: "b"}, {name: "c"}]
			}
		})`)
		count := frame.MustElement(".count")
		s.Equal("3 items", count.MustText())
	})

	s.Run("displays singular item count", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "only-one"}]
			}
		})`)
		count := frame.MustElement(".count")
		s.Equal("1 item", count.MustText())
	})
}

func (s *BrowserSuite) TestTableSorting() {
	s.Run("sorts ascending on first header click", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [
					{name: "charlie"},
					{name: "alpha"},
					{name: "bravo"}
				]
			}
		})`)
		frame.MustElement("table thead th").MustClick()
		frame.MustElementR(".sort-arrow", "\u25B2")
		rows := frame.MustElements("table tbody tr")
		s.Equal("alpha", rows[0].MustElement("td").MustText())
		s.Equal("bravo", rows[1].MustElement("td").MustText())
		s.Equal("charlie", rows[2].MustElement("td").MustText())
	})

	s.Run("sorts descending on second header click", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [
					{name: "charlie"},
					{name: "alpha"},
					{name: "bravo"}
				]
			}
		})`)
		th := frame.MustElement("table thead th")
		th.MustClick()
		frame.MustElementR(".sort-arrow", "\u25B2")
		th.MustClick()
		frame.MustElementR(".sort-arrow", "\u25BC")
		rows := frame.MustElements("table tbody tr")
		s.Equal("charlie", rows[0].MustElement("td").MustText())
		s.Equal("bravo", rows[1].MustElement("td").MustText())
		s.Equal("alpha", rows[2].MustElement("td").MustText())
	})
}

func (s *BrowserSuite) TestMetricsTable() {
	s.Run("renders chart canvas and data table", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [
					{key: "name", label: "Pod"},
					{key: "cpu", label: "CPU"},
					{key: "memory", label: "Memory"}
				],
				chart: {
					labelKey: "name",
					datasets: [
						{key: "cpu", label: "CPU (millicores)", unit: "cpu", axis: "left"},
						{key: "memory", label: "Memory (MiB)", unit: "memory", axis: "right"}
					]
				},
				items: [
					{name: "pod-1", cpu: "100m", memory: "128Mi"},
					{name: "pod-2", cpu: "250m", memory: "256Mi"}
				]
			}
		})`)
		frame.MustElement("canvas")
		rows := frame.MustElements("table tbody tr")
		s.Len(rows, 2)
	})

	s.Run("renders metrics table with custom column headers", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [
					{key: "name", label: "Pod Name"},
					{key: "cpu", label: "CPU Usage"}
				],
				chart: {
					labelKey: "name",
					datasets: [{key: "cpu", label: "CPU", unit: "cpu", axis: "left"}]
				},
				items: [{name: "pod-1", cpu: "100m"}]
			}
		})`)
		headers := frame.MustElements("table thead th")
		s.Contains(headers[0].MustText(), "Pod Name")
		s.Contains(headers[1].MustText(), "CPU Usage")
	})
}

func (s *BrowserSuite) TestGenericView() {
	s.Run("renders JSON for non-array structured content", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {key: "value", nested: {a: 1}}
		})`)
		pre := frame.MustElement("pre.raw")
		text := pre.MustText()
		s.Contains(text, "key")
		s.Contains(text, "value")
		s.Contains(text, "nested")
	})

	s.Run("renders text when no structured content", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "Hello World"}]
		})`)
		pre := frame.MustElement("pre.raw")
		s.Equal("Hello World", pre.MustText())
	})
}

func (s *BrowserSuite) TestItemsUnwrapping() {
	s.Run("unwraps items-only wrapper to array for TableView", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "pod-1"}, {name: "pod-2"}]
			}
		})`)
		frame.MustElement("table")
		rows := frame.MustElements("table tbody tr")
		s.Len(rows, 2)
	})

	s.Run("does not unwrap when items coexists with other keys", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "pod-1"}],
				extra: "should prevent unwrapping"
			}
		})`)
		// Without chart+columns, this falls through to GenericView
		pre := frame.MustElement("pre.raw")
		text := pre.MustText()
		s.Contains(text, "items")
		s.Contains(text, "extra")
	})
}

func (s *BrowserSuite) TestDataRouting() {
	s.Run("routes chart+columns+items to MetricsTable", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [{key: "name", label: "Name"}],
				chart: {
					labelKey: "name",
					datasets: [{key: "value", label: "Value", unit: "cpu", axis: "left"}]
				},
				items: [{name: "a", value: "100m"}]
			}
		})`)
		frame.MustElement("canvas")
		frame.MustElement("table")
	})

	s.Run("routes items-only to TableView not MetricsTable", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [{name: "pod-1"}]
			}
		})`)
		frame.MustElement("table")
		canvases := frame.MustElements("canvas")
		s.Len(canvases, 0, "TableView should not render a canvas")
	})

	s.Run("routes plain object to GenericView", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {message: "hello"}
		})`)
		pre := frame.MustElement("pre.raw")
		s.Contains(pre.MustText(), "hello")
	})

	s.Run("routes text-only content to GenericView", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "raw output"}]
		})`)
		pre := frame.MustElement("pre.raw")
		s.Equal("raw output", pre.MustText())
	})
}

func (s *BrowserSuite) TestSelfDescribingMetrics() {
	s.Run("renders metrics with realistic pods_top data", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [
					{key: "namespace", label: "Namespace"},
					{key: "name", label: "Pod"},
					{key: "cpu", label: "CPU"},
					{key: "memory", label: "Memory"}
				],
				chart: {
					labelKey: "name",
					datasets: [
						{key: "cpu", label: "CPU (millicores)", unit: "cpu", axis: "left"},
						{key: "memory", label: "Memory (MiB)", unit: "memory", axis: "right"}
					]
				},
				items: [
					{namespace: "default", name: "nginx-1", cpu: "100m", memory: "128Mi"},
					{namespace: "default", name: "redis-1", cpu: "50m", memory: "256Mi"},
					{namespace: "kube-system", name: "coredns", cpu: "10m", memory: "32Mi"}
				]
			}
		})`)
		frame.MustElement("canvas")
		rows := frame.MustElements("table tbody tr")
		s.Len(rows, 3)
		// Verify raw unit strings appear in table cells
		cells := frame.MustElements("table tbody td")
		cellTexts := make([]string, len(cells))
		for i, c := range cells {
			cellTexts[i] = c.MustText()
		}
		s.Contains(cellTexts, "100m")
		s.Contains(cellTexts, "128Mi")
		s.Contains(cellTexts, "256Mi")
	})

	s.Run("renders metrics with single dataset on left axis", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [{key: "name", label: "Node"}, {key: "cpu", label: "CPU"}],
				chart: {
					labelKey: "name",
					datasets: [{key: "cpu", label: "CPU (millicores)", unit: "cpu", axis: "left"}]
				},
				items: [{name: "node-1", cpu: "500m"}, {name: "node-2", cpu: "1200m"}]
			}
		})`)
		frame.MustElement("canvas")
		rows := frame.MustElements("table tbody tr")
		s.Len(rows, 2)
	})

	s.Run("table columns use labels from self-describing metadata", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [
					{key: "name", label: "Node"},
					{key: "cpu", label: "CPU (cores)"},
					{key: "memory", label: "Memory (bytes)"},
					{key: "cpu_pct", label: "CPU%"},
					{key: "mem_pct", label: "Memory%"}
				],
				chart: {
					labelKey: "name",
					datasets: [{key: "cpu", label: "CPU", unit: "cpu", axis: "left"}]
				},
				items: [{name: "node-1", cpu: "500m", memory: "2048Mi", cpu_pct: "25%", mem_pct: "50%"}]
			}
		})`)
		headers := frame.MustElements("table thead th")
		s.Len(headers, 5)
		s.Contains(headers[0].MustText(), "Node")
		s.Contains(headers[1].MustText(), "CPU (cores)")
		s.Contains(headers[2].MustText(), "Memory (bytes)")
		s.Contains(headers[3].MustText(), "CPU%")
		s.Contains(headers[4].MustText(), "Memory%")
	})
}

func (s *BrowserSuite) TestThemeApplication() {
	s.Run("applies light theme from host context", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		theme := frame.MustEval(`() => document.documentElement.getAttribute('data-theme')`).Str()
		s.Equal("light", theme)
	})

	s.Run("sets colorScheme style property", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		colorScheme := frame.MustEval(`() => document.documentElement.style.colorScheme`).Str()
		s.Equal("light", colorScheme)
	})

	s.Run("applies dark theme via host-context-changed notification", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendNotification('ui/notifications/host-context-changed', {theme: 'dark'})`)
		frame.MustWait(`() => document.documentElement.getAttribute('data-theme') === 'dark'`)
		theme := frame.MustEval(`() => document.documentElement.getAttribute('data-theme')`).Str()
		s.Equal("dark", theme)
		colorScheme := frame.MustEval(`() => document.documentElement.style.colorScheme`).Str()
		s.Equal("dark", colorScheme)
	})

	s.Run("applies CSS custom properties from host styles", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendNotification('ui/notifications/host-context-changed', {
			styles: {
				variables: {
					'--color-background-primary': '#ff0000',
					'--color-text-primary': '#00ff00'
				}
			}
		})`)
		frame.MustWait(`() => document.documentElement.style.getPropertyValue('--color-background-primary') === '#ff0000'`)
		bg := frame.MustEval(`() => document.documentElement.style.getPropertyValue('--color-background-primary')`).Str()
		s.Equal("#ff0000", bg)
		text := frame.MustEval(`() => document.documentElement.style.getPropertyValue('--color-text-primary')`).Str()
		s.Equal("#00ff00", text)
	})
}

func (s *BrowserSuite) TestYamlViewXSS() {
	s.Run("HTML tags in YAML values are escaped", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "apiVersion: v1\nkind: Pod\nmetadata:\n  annotations:\n    note: <script>alert(1)</script>"}]
		})`)
		pre := frame.MustElement("pre.raw.yaml")
		// The raw text should show the angle brackets as visible characters, not execute them
		text := pre.MustText()
		s.Contains(text, "<script>alert(1)</script>")
		// The innerHTML must NOT contain an actual <script> tag.
		// Prism tokenizes ">" as punctuation wrapped in <span>, so "&lt;script&gt;"
		// won't appear as a contiguous string — check for "&lt;script" (the opening
		// bracket escape is the security-critical part).
		inner := frame.MustEval(`() => document.querySelector('pre.raw.yaml').innerHTML`).Str()
		s.NotContains(inner, "<script>")
		s.Contains(inner, "&lt;script")
	})

	s.Run("HTML attribute injection in YAML values is escaped", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: \"<img onerror=alert(1) src=x>\""}]
		})`)
		frame.MustElement("pre.raw.yaml")
		inner := frame.MustEval(`() => document.querySelector('pre.raw.yaml').innerHTML`).Str()
		s.NotContains(inner, "<img")
		s.Contains(inner, "&lt;img")
	})

	s.Run("span injection attempt in YAML values is escaped", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: </span><script>alert(1)</script><span>"}]
		})`)
		frame.MustElement("pre.raw.yaml")
		inner := frame.MustEval(`() => document.querySelector('pre.raw.yaml').innerHTML`).Str()
		s.NotContains(inner, "<script>")
	})

	s.Run("ampersand in YAML values is escaped to prevent double-decode", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "apiVersion: v1\nkind: Pod\nmetadata:\n  annotations:\n    note: '&lt;script&gt;alert(1)&lt;/script&gt;'"}]
		})`)
		pre := frame.MustElement("pre.raw.yaml")
		// The visible text must show the literal ampersand sequences
		text := pre.MustText()
		s.Contains(text, "&lt;script&gt;")
		// In innerHTML the ampersand must be double-escaped so the browser
		// does not decode &lt; back into <
		inner := frame.MustEval(`() => document.querySelector('pre.raw.yaml').innerHTML`).Str()
		s.Contains(inner, "&amp;lt;script")
		s.NotContains(inner, "<script>")
	})
}

func (s *BrowserSuite) TestScreenshots() {
	// Set a consistent viewport for reproducible screenshots
	const width, height = 1024, 768

	s.Run("waiting state", func() {
		page, _ := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		s.screenshot(page, "01-waiting")
	})

	s.Run("table view light", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [
					{namespace: "default", name: "nginx-7c5b4f", status: "Running", restarts: "0", age: "2d"},
					{namespace: "default", name: "redis-8d3a1b", status: "Running", restarts: "1", age: "5d"},
					{namespace: "kube-system", name: "coredns-5644d7", status: "Running", restarts: "0", age: "12d"},
					{namespace: "kube-system", name: "etcd-master", status: "Running", restarts: "0", age: "12d"},
					{namespace: "monitoring", name: "prometheus-0", status: "Running", restarts: "2", age: "3d"}
				]
			}
		})`)
		frame.MustElement("table")
		s.screenshot(page, "02-table-light")
	})

	s.Run("table view dark", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendNotification('ui/notifications/host-context-changed', {theme: 'dark'})`)
		frame.MustWait(`() => document.documentElement.getAttribute('data-theme') === 'dark'`)
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				items: [
					{namespace: "default", name: "nginx-7c5b4f", status: "Running", restarts: "0", age: "2d"},
					{namespace: "default", name: "redis-8d3a1b", status: "Running", restarts: "1", age: "5d"},
					{namespace: "kube-system", name: "coredns-5644d7", status: "Running", restarts: "0", age: "12d"},
					{namespace: "kube-system", name: "etcd-master", status: "Running", restarts: "0", age: "12d"},
					{namespace: "monitoring", name: "prometheus-0", status: "Running", restarts: "2", age: "3d"}
				]
			}
		})`)
		frame.MustElement("table")
		s.screenshot(page, "03-table-dark")
	})

	s.Run("metrics view light", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [
					{key: "namespace", label: "Namespace"},
					{key: "name", label: "Pod"},
					{key: "cpu", label: "CPU"},
					{key: "memory", label: "Memory"}
				],
				chart: {
					labelKey: "name",
					datasets: [
						{key: "cpu", label: "CPU (millicores)", unit: "cpu", axis: "left"},
						{key: "memory", label: "Memory (MiB)", unit: "memory", axis: "right"}
					]
				},
				items: [
					{namespace: "default", name: "nginx-1", cpu: "250m", memory: "128Mi"},
					{namespace: "default", name: "redis-1", cpu: "100m", memory: "256Mi"},
					{namespace: "kube-system", name: "coredns", cpu: "15m", memory: "32Mi"},
					{namespace: "monitoring", name: "prometheus", cpu: "500m", memory: "512Mi"},
					{namespace: "monitoring", name: "grafana", cpu: "80m", memory: "192Mi"}
				]
			}
		})`)
		frame.MustElement("canvas")
		frame.MustElement("table")
		s.screenshot(page, "04-metrics-light")
	})

	s.Run("metrics view dark", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendNotification('ui/notifications/host-context-changed', {theme: 'dark'})`)
		frame.MustWait(`() => document.documentElement.getAttribute('data-theme') === 'dark'`)
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				columns: [
					{key: "namespace", label: "Namespace"},
					{key: "name", label: "Pod"},
					{key: "cpu", label: "CPU"},
					{key: "memory", label: "Memory"}
				],
				chart: {
					labelKey: "name",
					datasets: [
						{key: "cpu", label: "CPU (millicores)", unit: "cpu", axis: "left"},
						{key: "memory", label: "Memory (MiB)", unit: "memory", axis: "right"}
					]
				},
				items: [
					{namespace: "default", name: "nginx-1", cpu: "250m", memory: "128Mi"},
					{namespace: "default", name: "redis-1", cpu: "100m", memory: "256Mi"},
					{namespace: "kube-system", name: "coredns", cpu: "15m", memory: "32Mi"},
					{namespace: "monitoring", name: "prometheus", cpu: "500m", memory: "512Mi"},
					{namespace: "monitoring", name: "grafana", cpu: "80m", memory: "192Mi"}
				]
			}
		})`)
		frame.MustElement("canvas")
		frame.MustElement("table")
		s.screenshot(page, "05-metrics-dark")
	})

	s.Run("yaml view light", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx-7c5b4f\n  namespace: default\n  labels:\n    app: nginx\n    tier: frontend\nspec:\n  containers:\n    - name: nginx\n      image: nginx:1.25\n      ports:\n        - containerPort: 80\n          protocol: TCP\n      resources:\n        requests:\n          cpu: 100m\n          memory: 128Mi\n        limits:\n          cpu: 500m\n          memory: 256Mi\nstatus:\n  phase: Running\n  podIP: 10.244.0.5"}]
		})`)
		frame.MustElement("pre.raw.yaml")
		s.screenshot(page, "06-yaml-light")
	})

	s.Run("yaml view dark", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendNotification('ui/notifications/host-context-changed', {theme: 'dark'})`)
		frame.MustWait(`() => document.documentElement.getAttribute('data-theme') === 'dark'`)
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx-7c5b4f\n  namespace: default\n  labels:\n    app: nginx\n    tier: frontend\nspec:\n  containers:\n    - name: nginx\n      image: nginx:1.25\n      ports:\n        - containerPort: 80\n          protocol: TCP\n      resources:\n        requests:\n          cpu: 100m\n          memory: 128Mi\n        limits:\n          cpu: 500m\n          memory: 256Mi\nstatus:\n  phase: Running\n  podIP: 10.244.0.5"}]
		})`)
		frame.MustElement("pre.raw.yaml")
		s.screenshot(page, "07-yaml-dark")
	})

	s.Run("generic view with JSON", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendToolResult({
			structuredContent: {
				cluster: "production",
				version: {major: "1", minor: "28", gitVersion: "v1.28.4"},
				platform: "linux/amd64",
				nodes: 5,
				totalPods: 42
			}
		})`)
		frame.MustElement("pre.raw")
		s.screenshot(page, "08-generic-json")
	})

	s.Run("generic view with plain text", func() {
		page, frame := s.openViewer()
		defer page.MustClose()
		page.MustSetViewport(width, height, 0, false)
		page.MustEval(`() => window.sendToolResult({
			content: [{type: "text", text: "deployment.apps/nginx scaled to 3 replicas\ndeployment.apps/nginx condition met"}]
		})`)
		frame.MustElement("pre.raw")
		s.screenshot(page, "09-generic-text")
	})
}

func TestBrowser(t *testing.T) {
	suite.Run(t, new(BrowserSuite))
}
