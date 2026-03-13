(function() {
  'use strict';

  var hp = window.htmPreact;
  var html = hp.html;
  var render = hp.render;
  var useState = hp.useState;
  var useEffect = hp.useEffect;

  var protocol = window.mcpProtocol;
  var components = window.mcpComponents;

  // Detect Kubernetes YAML from text content.
  // Some tools prepend "# comment\n" headers for the LLM — skip those before checking.
  function looksLikeYaml(text) {
    if (!text) return false;
    var lines = text.trimStart().split('\n');
    for (var i = 0; i < lines.length && i < 5; i++) {
      var line = lines[i].trimStart();
      if (line === '' || line.charAt(0) === '#') continue;
      return /^(apiVersion:|kind:|metadata:|---)/.test(line);
    }
    return false;
  }

  // Tool name injected by the server via ViewerHTMLForTool
  var embeddedToolName = window.__mcpToolName || '';

  // Apply theme to documentElement so CSS light-dark() functions work.
  function applyTheme(theme) {
    if (!theme) return;
    var root = document.documentElement;
    root.setAttribute('data-theme', theme);
    root.style.colorScheme = theme;
  }

  // Apply host-provided CSS custom properties from hostContext.styles.variables.
  function applyStyleVariables(styles) {
    if (!styles || !styles.variables) return;
    var root = document.documentElement;
    var vars = styles.variables;
    for (var key in vars) {
      if (vars.hasOwnProperty(key) && vars[key] != null) {
        root.style.setProperty(key, vars[key]);
      }
    }
  }

  // Apply all host context styling (theme + variables)
  function applyHostContext(hostContext) {
    if (!hostContext) return;
    applyTheme(hostContext.theme);
    applyStyleVariables(hostContext.styles);
  }

  function App() {
    var stateArr = useState('loading');
    var state = stateArr[0];
    var setState = stateArr[1];

    var toolNameArr = useState(embeddedToolName);
    var toolName = toolNameArr[0];
    var setToolName = toolNameArr[1];

    var resultArr = useState(null);
    var result = resultArr[0];
    var setResult = resultArr[1];

    var errorArr = useState(null);
    var appError = errorArr[0];
    var setError = errorArr[1];

    useEffect(function() {
      // Register notification handlers BEFORE initializing to avoid any timing window

      // Spec-compliant flow: host sends tool-result after calling the tool
      protocol.onNotification('ui/notifications/tool-result', function(params) {
        console.log('[mcp-app] tool-result received');
        setResult(params);
        setState('result');
      });

      // Fallback flow: host sends tool-input, viewer calls the tool via serverTools
      protocol.onNotification('ui/notifications/tool-input', function(params) {
        console.log('[mcp-app] tool-input received, toolName:', toolName);
        if (toolName) {
          var args = (params && params.arguments) || {};
          console.log('[mcp-app] calling tool via serverTools:', toolName);
          protocol.sendRequest('tools/call', { name: toolName, arguments: args }).then(function(callResult) {
            console.log('[mcp-app] tool call result received');
            setResult(callResult);
            setState('result');
          }).catch(function(err) {
            console.error('[mcp-app] tool call failed:', err);
            setError('Tool call failed: ' + (err.message || err));
            setState('error');
          });
        }
      });

      // Host sends theme/variable updates (spec-compliant hosts)
      protocol.onNotification('ui/notifications/host-context-changed', function(params) {
        console.log('[mcp-app] host-context-changed');
        applyHostContext(params);
      });

      protocol.initialize().then(function(initResult) {
        // Apply host theme and CSS variables
        if (initResult && initResult.hostContext) {
          applyHostContext(initResult.hostContext);
        }

        // Prefer hostContext.toolInfo if available (spec-compliant hosts)
        var name = embeddedToolName;
        if (initResult && initResult.hostContext && initResult.hostContext.toolInfo && initResult.hostContext.toolInfo.tool) {
          name = initResult.hostContext.toolInfo.tool.name;
        }
        console.log('[mcp-app] initialized, toolName:', name);
        setToolName(name);
        setState('ready');

      }).catch(function(err) {
        console.error('[mcp-app] initialize error:', err);
        setError(err.message || 'Failed to initialize');
        setState('error');
      });
    }, []);

    if (state === 'loading') {
      return html`<p class="status">Initializing...</p>`;
    }
    if (state === 'error') {
      return html`<p class="status">Error: ${appError}</p>`;
    }
    if (state === 'ready') {
      return html`<p class="status">Waiting for tool result...</p>`;
    }

    // state === 'result'
    if (!result) {
      return html`<p class="status">No result received</p>`;
    }

    // Extract structured content or fall back to text.
    // The server wraps plain array data in {"items": [...]} because the MCP spec
    // requires structuredContent to be a JSON object. Unwrap it here, but only
    // when the object is a plain items-wrapper (no other keys like columns/chart).
    var raw = result.structuredContent;
    var structured = raw;
    if (raw && raw.items && Array.isArray(raw.items) && Object.keys(raw).length === 1) {
      structured = raw.items;
    }
    var textContent = '';
    if (result.content && Array.isArray(result.content)) {
      for (var i = 0; i < result.content.length; i++) {
        if (result.content[i] && result.content[i].text) {
          textContent += result.content[i].text;
        }
      }
    }

    // Route to the appropriate view based on data shape
    if (structured && structured.chart && structured.columns && Array.isArray(structured.items)) {
      return html`<${components.MetricsTable} data=${structured} />`;
    }

    // Generic table for any structured array data (list operations)
    if (structured && Array.isArray(structured) && structured.length > 0 && typeof structured[0] === 'object') {
      return html`<${components.TableView} data=${structured} />`;
    }

    // Fallback: try to render structured as JSON, or raw text
    if (structured) {
      return html`<${components.GenericView} text=${JSON.stringify(structured, null, 2)} />`;
    }
    // Text-only results: detect YAML for syntax highlighting
    if (looksLikeYaml(textContent)) {
      return html`<${components.YamlView} text=${textContent} />`;
    }
    return html`<${components.GenericView} text=${textContent} />`;
  }

  // Mount
  render(html`<${App} />`, document.getElementById('app'));
})();
