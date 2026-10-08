(() => {
  const app = document.getElementById("app");
  const kind = document.body.dataset.appKind;
  let rows = [], columns = [], sortColumn = "", descending = false;
  let resizeObserver, resizeFrame = 0, lastSize = "", tornDown = false;
  let hostContext = {};

  function send(message) {
    parent.postMessage(message, "*");
  }

  function applyHostContext(update) {
    if (!update || typeof update !== "object" || Array.isArray(update)) return;
    hostContext = { ...hostContext, ...update };
    if (hostContext.theme === "light" || hostContext.theme === "dark") {
      document.documentElement.dataset.theme = hostContext.theme;
      document.documentElement.style.colorScheme = hostContext.theme;
    }
  }

  function startResizeReporting() {
    if (resizeObserver || tornDown) return;
    const schedule = () => {
      if (resizeFrame || tornDown) return;
      resizeFrame = requestAnimationFrame(() => {
        resizeFrame = 0;
        // Body height follows content, not the host's current iframe height,
        // so collapsed sections and shorter results can shrink the container.
        const height = Math.ceil(document.body.getBoundingClientRect().height);
        const width = Math.ceil(window.innerWidth);
        const size = width + ":" + height;
        if (size === lastSize) return;
        lastSize = size;
        send({ jsonrpc: "2.0", method: "ui/notifications/size-changed", params: { width, height } });
      });
    };
    resizeObserver = new ResizeObserver(schedule);
    resizeObserver.observe(document.body);
    resizeObserver.observe(document.documentElement);
    schedule();
  }

  function fail(message) {
    app.className = "status";
    app.setAttribute("role", "status");
    app.textContent = "Unable to load data: " + (message || "unknown error");
  }

  function text(value) {
    if (value === null || value === undefined) return "";
    return typeof value === "object" ? JSON.stringify(value) : String(value);
  }

  function quantity(value) {
    const match = String(value).match(/^([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)(n|u|m|k|K|M|G|T|P|E|Ki|Mi|Gi|Ti|Pi|Ei)?$/);
    if (!match) return NaN;
    const factors = { n: 1e-9, u: 1e-6, m: 1e-3, k: 1e3, K: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18,
      Ki: 2 ** 10, Mi: 2 ** 20, Gi: 2 ** 30, Ti: 2 ** 40, Pi: 2 ** 50, Ei: 2 ** 60 };
    return Number(match[1]) * (factors[match[2]] || 1);
  }

  function ageSeconds(value) {
    const age = text(value);
    if (!/^(?:\d+[smhdy])+$/.test(age)) return NaN;
    const units = { s: 1, m: 60, h: 3600, d: 86400, y: 365 * 86400 };
    return [...age.matchAll(/(\d+)([smhdy])/g)]
      .reduce((seconds, match) => seconds + Number(match[1]) * units[match[2]], 0);
  }

  function compare(left, right) {
    if (sortColumn === "Age") {
      const a = ageSeconds(left.Age), b = ageSeconds(right.Age);
      if (Number.isFinite(a) && Number.isFinite(b)) return a - b;
      // Keep unavailable ages last after the caller applies the sort direction.
      if (Number.isFinite(a)) return descending ? 1 : -1;
      if (Number.isFinite(b)) return descending ? -1 : 1;
    }
    if (kind === "metrics" && (sortColumn === "CPU" || sortColumn === "Memory")) {
      const a = quantity(left[sortColumn]), b = quantity(right[sortColumn]);
      if (Number.isFinite(a) && Number.isFinite(b)) return a - b;
    }
    return text(left[sortColumn]).localeCompare(text(right[sortColumn]));
  }

  function resourceRow(resource) {
    const metadata = resource.metadata;
    const row = {
      Name: metadata.name || "",
      Age: "",
      Labels: Object.entries(metadata.labels || {}).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => key + "=" + value).join(","),
      apiVersion: resource.apiVersion,
      kind: resource.kind,
    };
    if (metadata.namespace) row.Namespace = metadata.namespace;
    if (typeof resource.status?.phase === "string") row.Status = resource.status.phase;
    // YAML results contain full objects rather than API-server table cells.
    // Derive the standard list fields for these built-in types on the display
    // copy only; an unrelated CRD with the same kind must not be interpreted.
    if (resource.kind === "Deployment" && resource.apiVersion.startsWith("apps/")) {
      row.Ready = (resource.status?.readyReplicas ?? 0) + "/" + (resource.spec?.replicas ?? 1);
      row["Up-to-date"] = resource.status?.updatedReplicas ?? 0;
      row.Available = resource.status?.availableReplicas ?? 0;
    }
    if (resource.kind === "Ingress" && resource.apiVersion.startsWith("networking.k8s.io/")) {
      row.Class = resource.spec?.ingressClassName ?? "<none>";
      row.Hosts = (resource.spec?.rules || []).map(rule => rule.host).filter(Boolean).join(",") || "*";
      row.Address = [...new Set((resource.status?.loadBalancer?.ingress || [])
        .map(address => address.ip || address.hostname).filter(Boolean))].sort().join(",");
      row.Ports = resource.spec?.tls?.length ? "80, 443" : "80";
    }
    const seconds = Math.max(0, Math.floor((Date.now() - Date.parse(metadata.creationTimestamp)) / 1000));
    if (Number.isFinite(seconds)) {
      row.Age = seconds < 60 ? seconds + "s" : seconds < 3600 ? Math.floor(seconds / 60) + "m"
        : seconds < 86400 ? Math.floor(seconds / 3600) + "h" : Math.floor(seconds / 86400) + "d";
    }
    return row;
  }

  function renderTable(data) {
    if (!data || !Array.isArray(data.items)) throw new Error("expected a list of rows");
    rows = data.items.map((row, index) => {
      if (!row || typeof row !== "object" || Array.isArray(row)) throw new Error("row " + (index + 1) + " is invalid");
      // Preserve the tool's full objects; normalize only this display copy.
      return kind === "table" && row.apiVersion && row.kind && row.metadata && typeof row.metadata === "object"
        ? resourceRow(row) : row;
    });
    const declared = Array.isArray(data.columns) ? data.columns.filter(c => typeof c === "string")
      : ["Name", "Namespace", "Status", "Ready", "Up-to-date", "Available", "Class", "Hosts", "Address", "Ports",
        "Age", "Labels", "apiVersion", "kind"];
    const keys = new Set(rows.flatMap(Object.keys));
    columns = [...declared.filter(c => keys.delete(c)), ...[...keys].sort()];
    if (!columns.includes(sortColumn)) {
      sortColumn = columns.includes("Name") ? "Name" : columns[0] || "";
      descending = false;
    }
    if (!rows.length) {
      app.className = "status";
      app.setAttribute("role", "status");
      app.textContent = kind === "metrics" ? "No metrics found." : "No results found.";
      return;
    }
    rows.sort((a, b) => (descending ? -1 : 1) * compare(a, b));
    const table = document.createElement("table");
    const head = table.createTHead().insertRow();
    columns.forEach(column => {
      const th = document.createElement("th");
      th.scope = "col";
      th.setAttribute("aria-sort", column === sortColumn ? (descending ? "descending" : "ascending") : "none");
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = column;
      button.onclick = () => {
        // Match kubectl's creationTimestamp sort: oldest first for Age.
        descending = column === sortColumn ? !descending : column === "Age";
        sortColumn = column;
        renderTable({ columns, items: rows });
      };
      th.appendChild(button);
      head.appendChild(th);
    });
    const body = table.createTBody();
    rows.forEach(row => {
      const tr = body.insertRow();
      columns.forEach(column => { tr.insertCell().textContent = text(row[column]); });
    });
    app.className = "";
    app.removeAttribute("role");
    app.replaceChildren(table);
  }

  function renderResource(data) {
    if (!data || typeof data !== "object" || Array.isArray(data)) throw new Error("expected one resource object");
    const view = document.createElement("article");
    view.className = "resource-details";
    const metadata = data.metadata || {};
    const title = document.createElement("h1");
    title.textContent = [data.kind, metadata.name].filter(Boolean).join(" ") || "Resource details";
    view.appendChild(title);

    function section(name, content, expandable = false) {
      const section = document.createElement(expandable ? "details" : "section");
      section.setAttribute("aria-label", name);
      const heading = document.createElement(expandable ? "summary" : "h2");
      heading.textContent = name;
      section.append(heading, content);
      view.appendChild(section);
    }

    // Build every value as DOM text, including cluster-controlled keys.
    function fields(value) {
      if (value && typeof value === "object") {
        const entries = Object.entries(value);
        if (entries.length) {
          const list = document.createElement(Array.isArray(value) ? "ol" : "dl");
          for (const [key, item] of entries) {
            if (Array.isArray(value)) {
              const entry = document.createElement("li");
              entry.appendChild(fields(item));
              list.appendChild(entry);
            } else {
              const label = document.createElement("dt");
              label.textContent = key;
              const content = document.createElement("dd");
              content.appendChild(fields(item));
              list.append(label, content);
            }
          }
          return list;
        }
      }
      const content = document.createElement("span");
      content.textContent = value && typeof value === "object" ? "None" : text(value) || "—";
      return content;
    }

    function detailTable(columns, rows) {
      const table = document.createElement("table");
      const head = table.createTHead().insertRow();
      for (const column of columns) {
        const th = document.createElement("th");
        th.scope = "col";
        th.textContent = column;
        head.appendChild(th);
      }
      const body = table.createTBody();
      for (const row of rows) {
        const tr = body.insertRow();
        for (const value of row) tr.insertCell().textContent = text(value);
      }
      return table;
    }

    const overview = { Name: metadata.name, Kind: data.kind, "API version": data.apiVersion };
    if (metadata.namespace) overview.Namespace = metadata.namespace;
    if (metadata.creationTimestamp) overview.Created = metadata.creationTimestamp;
    if (data.status?.phase) overview.Status = data.status.phase;
    if (data.kind === "Pod") {
      if (data.spec?.nodeName) overview.Node = data.spec.nodeName;
      if (data.status?.podIP) overview["Pod IP"] = data.status.podIP;
    }
    section("Overview", fields(overview));
    if (metadata.labels) section("Labels", fields(metadata.labels));
    if (metadata.annotations) section("Annotations", fields(metadata.annotations), true);

    if (data.kind === "Pod") {
      for (const [name, specKey, statusKey] of [
        ["Containers", "containers", "containerStatuses"],
        ["Init containers", "initContainers", "initContainerStatuses"],
        ["Ephemeral containers", "ephemeralContainers", "ephemeralContainerStatuses"],
      ]) {
        const containers = data.spec?.[specKey];
        if (!Array.isArray(containers) || !containers.length) continue;
        const statuses = Array.isArray(data.status?.[statusKey]) ? data.status[statusKey] : [];
        const rows = containers.map(container => {
          if (!container || typeof container !== "object") throw new Error("invalid container");
          const status = statuses.find(status => status && status.name === container.name);
          const state = status?.state || {};
          const stateName = Object.keys(state)[0];
          const detail = state[stateName] || {};
          return [container.name, container.image, status ? (status.ready ? "Yes" : "No") : "Unknown",
            status?.restartCount ?? "—", detail.reason || stateName || "Unknown", detail.message || ""];
        });
        section(name, detailTable(["Name", "Image", "Ready", "Restarts", "State", "Message"], rows));
      }
    }
    if (Array.isArray(data.status?.conditions) && data.status.conditions.length) {
      section("Conditions", detailTable(["Type", "Status", "Reason", "Message", "Last transition"],
        data.status.conditions.map(condition => {
          if (!condition || typeof condition !== "object") throw new Error("invalid condition");
          return [condition.type, condition.status, condition.reason, condition.message, condition.lastTransitionTime];
        })));
    }
    if (data.spec) section("Specification", fields(data.spec), true);
    if (data.status) section("Status details", fields(data.status), true);
    const pre = document.createElement("pre");
    pre.textContent = JSON.stringify(data, null, 2);
    section("Raw resource", pre, true);
    app.className = "";
    app.removeAttribute("role");
    app.replaceChildren(view);
  }

  window.addEventListener("message", event => {
    if (event.source !== parent) return;
    try {
      const message = event.data;
      if (!message || message.jsonrpc !== "2.0") return;
      if (message.method === "ping") {
        if (typeof message.id === "number" || typeof message.id === "string") {
          send({ jsonrpc: "2.0", id: message.id, result: {} });
        }
        return;
      }
      if (message.id === 1 && message.method === undefined) {
        if (message.error) { fail(message.error.message || "initialization failed"); return; }
        if (message.result) {
          applyHostContext(message.result.hostContext);
          send({ jsonrpc: "2.0", method: "ui/notifications/initialized", params: {} });
          startResizeReporting();
        }
        return;
      }
      if (message.method === "ui/notifications/host-context-changed") {
        applyHostContext(message.params);
        return;
      }
      if (message.method === "ui/notifications/tool-cancelled") {
        app.className = "status";
        app.setAttribute("role", "status");
        const reason = message.params?.reason;
        app.textContent = "Tool execution cancelled" + (typeof reason === "string" && reason ? ": " + reason : ".");
        return;
      }
      if (message.method === "ui/notifications/tool-result") {
        const params = message.params || {};
        if (params.isError) {
          const content = Array.isArray(params.content) ? params.content : [];
          fail(content.filter(item => item && item.type === "text").map(item => item.text).join(" ") || "the request failed");
          return;
        }
        if (!Object.prototype.hasOwnProperty.call(params, "structuredContent")) throw new Error("the server returned no structured data");
        if (kind === "resource") renderResource(params.structuredContent);
        else renderTable(params.structuredContent);
        return;
      }
      if (message.id !== undefined && message.method === "ui/resource-teardown") {
        tornDown = true;
        resizeObserver?.disconnect();
        cancelAnimationFrame(resizeFrame);
        resizeFrame = 0;
        send({ jsonrpc: "2.0", id: message.id, result: {} });
      }
    } catch (err) {
      fail(err.message || "invalid server response");
    }
  });

  send({ jsonrpc: "2.0", id: 1, method: "ui/initialize", params: {
    protocolVersion: "2026-01-26", appCapabilities: {},
    appInfo: { name: "kubernetes-mcp-server-" + kind, version: "1.0.0" }
  } });
})();
