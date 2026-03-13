//go:build browser

package mcpapps

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-rod/rod"
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

func TestBrowser(t *testing.T) {
	suite.Run(t, new(BrowserSuite))
}
