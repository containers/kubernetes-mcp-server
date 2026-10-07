import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";

// run.sh copies these tests into _output/browser/playwright before running them.
const assets = new URL("../../../pkg/mcpapps/", import.meta.url);
const template = readFileSync(new URL("standard.html", assets), "utf8");
const styles = readFileSync(new URL("styles.css", assets), "utf8");
const script = readFileSync(new URL("standard.js", assets), "utf8");

async function openApp(page, kind = "table") {
  await page.setContent("<iframe></iframe>");
  await page.evaluate(() => {
    window.messages = [];
    window.addEventListener("message", event => window.messages.push(event.data));
  });
  const frame = page.frames()[1];
  await frame.setContent(template.replace("{{KIND}}", kind).replace("{{STYLES}}", styles).replace("{{SCRIPT}}", script));
  return frame;
}

async function send(page, message) {
  await page.evaluate(message => document.querySelector("iframe").contentWindow.postMessage({ jsonrpc: "2.0", ...message }, "*"), message);
}

async function result(page, structuredContent) {
  await send(page, { method: "ui/notifications/tool-result", params: { structuredContent } });
}

async function reportedSize(page) {
  return page.evaluate(() => window.messages.filter(m => m.method === "ui/notifications/size-changed").at(-1)?.params);
}

test("table and metrics report content growth, shrinkage, and stop on teardown", async ({ page }) => {
  for (const kind of ["table", "metrics"]) {
    const app = await openApp(page, kind);
    await send(page, { id: 1, result: { protocolVersion: "2026-01-26" } });
    await result(page, { items: [{ Name: "short" }] });
    await expect(app.locator("tbody tr")).toHaveCount(1);
    const shortHeight = await app.evaluate(() => Math.ceil(document.body.getBoundingClientRect().height));
    await expect.poll(async () => (await reportedSize(page))?.height).toBe(shortHeight);
    const short = await reportedSize(page);
    await result(page, { items: Array.from({ length: 20 }, (_, i) => ({ Name: "row-" + i })) });
    await expect.poll(async () => (await reportedSize(page))?.height || 0).toBeGreaterThan(short.height);
    const tall = await reportedSize(page);
    // Simulate the host resizing the iframe to the reported height.
    await page.locator("iframe").evaluate((frame, height) => { frame.style.height = height + "px"; }, tall.height);
    await result(page, { items: [{ Name: "short" }] });
    await expect.poll(async () => (await reportedSize(page))?.height).toBe(short.height);
    const count = await page.evaluate(() => window.messages.filter(m => m.method === "ui/notifications/size-changed").length);
    await result(page, { items: [{ Name: "other" }] });
    await app.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    expect(await page.evaluate(() => window.messages.filter(m => m.method === "ui/notifications/size-changed").length)).toBe(count);
    await send(page, { id: 2, method: "ui/resource-teardown", params: {} });
    await expect.poll(() => page.evaluate(() => window.messages.some(m => m.id === 2 && m.result))).toBe(true);
    await result(page, { items: [] });
    await app.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    expect(await page.evaluate(() => window.messages.filter(m => m.method === "ui/notifications/size-changed").length)).toBe(count);
  }
});

test("resource details report expansion and collapse sizes", async ({ page }) => {
  const app = await openApp(page, "resource");
  await send(page, { id: 1, result: { protocolVersion: "2026-01-26" } });
  await result(page, { apiVersion: "v1", kind: "Service", metadata: { name: "web" },
    spec: { ports: Array.from({ length: 10 }, (_, i) => ({ port: 80 + i, targetPort: 8080 + i })) } });
  await expect(app.getByText("Specification", { exact: true })).toBeVisible();
  const collapsedHeight = await app.evaluate(() => Math.ceil(document.body.getBoundingClientRect().height));
  await expect.poll(async () => (await reportedSize(page))?.height).toBe(collapsedHeight);
  const collapsed = await reportedSize(page);
  await app.getByText("Specification", { exact: true }).click();
  await expect.poll(async () => (await reportedSize(page))?.height || 0).toBeGreaterThan(collapsed.height);
  await app.getByText("Specification", { exact: true }).click();
  await expect.poll(async () => (await reportedSize(page))?.height).toBe(collapsed.height);
});

test("shared table preserves column order and supports both sort directions", async ({ page }) => {
  const app = await openApp(page);
  await result(page, { columns: ["Name", "Status"], items: [{ Name: "z", Status: "Active" }, { Name: "a", Status: "Pending" }] });
  await expect(app.getByRole("columnheader")).toHaveText(["Name", "Status"]);
  await expect(app.locator("tbody tr td:first-child")).toHaveText(["a", "z"]);
  await app.getByRole("button", { name: "Name", exact: true }).click();
  await expect(app.locator("tbody tr td:first-child")).toHaveText(["z", "a"]);
});

test("shared table handles empty results and recovers from errors", async ({ page }) => {
  const app = await openApp(page);
  await result(page, { items: [] });
  await expect(app.locator("#app")).toHaveText("No results found.");
  await send(page, { method: "ui/notifications/tool-result", params: { isError: true, content: [{ type: "text", text: "Forbidden" }] } });
  await expect(app.locator("#app")).toHaveText("Unable to load data: Forbidden");
  await result(page, { items: [{ Name: "recovered" }] });
  await expect(app.locator("tbody")).toHaveText("recovered");
});

test("malformed rows produce diagnostics and nested cells remain readable", async ({ page }) => {
  const app = await openApp(page);
  await result(page, { items: [null] });
  await expect(app.locator("#app")).toContainText("row 1 is invalid");
  await result(page, { items: [{ Object: { Kind: "Pod", Name: "example" } }] });
  await expect(app.locator("tbody")).toHaveText('{"Kind":"Pod","Name":"example"}');
});

test("shared table displays full Kubernetes objects as compact rows", async ({ page }) => {
  const app = await openApp(page);
  await result(page, { items: [{
    apiVersion: "v1", kind: "Pod",
    metadata: { name: "example", namespace: "team-a", labels: { z: "last", a: "first" } },
    spec: { containers: [{ name: "app", image: "example:latest" }] },
    status: { phase: "Running" },
  }] });
  await expect(app.getByRole("columnheader")).toHaveText(["Name", "Namespace", "Status", "Age", "Labels", "apiVersion", "kind"]);
  await expect(app.locator("tbody td")).toHaveText(["example", "team-a", "Running", "", "a=first,z=last", "v1", "Pod"]);
  await app.getByRole("button", { name: "Name", exact: true }).click();
  await expect(app.locator("tbody td:first-child")).toHaveText("example");
});

test("metric headers and cells render untrusted content as text", async ({ page }) => {
  const app = await openApp(page, "metrics");
  const header = '<img src="data:,invalid" onerror="window.injected=true">';
  await result(page, { columns: [header], items: [{ [header]: "<script>bad()</script>" }] });
  await expect(app.getByRole("columnheader")).toHaveText(header);
  await expect(app.locator("tbody")).toHaveText("<script>bad()</script>");
  await expect(app.locator("th img, td script")).toHaveCount(0);
});

test("metrics sort CPU and memory quantities numerically", async ({ page }) => {
  const app = await openApp(page, "metrics");
  await result(page, { columns: ["Name", "CPU", "Memory"], items: [
    { Name: "a", CPU: "100m", Memory: "100Mi" },
    { Name: "b", CPU: "20m", Memory: "20Mi" },
    { Name: "c", CPU: "1", Memory: "1Gi" },
    { Name: "d", CPU: "500000n", Memory: "512Ki" },
  ] });
  await app.getByRole("button", { name: "CPU", exact: true }).click();
  await expect(app.locator("tbody tr td:nth-child(2)")).toHaveText(["500000n", "20m", "100m", "1"]);
  await app.getByRole("button", { name: "Memory", exact: true }).click();
  await expect(app.locator("tbody tr td:nth-child(3)")).toHaveText(["512Ki", "20Mi", "100Mi", "1Gi"]);
});

test("resource app shows one structured object without network dependencies", async ({ page }) => {
  const requests = [];
  await page.route("**/*", route => { requests.push(route.request().url()); return route.abort(); });
  const app = await openApp(page, "resource");
  const resource = { apiVersion: "v1", kind: "Pod", metadata: { name: "example" } };
  await result(page, resource);
  await expect(app.getByRole("heading", { name: "Pod example", exact: true })).toBeVisible();
  await expect(app.locator("pre")).not.toBeVisible();
  await app.getByText("Raw resource", { exact: true }).click();
  await expect(app.locator("pre")).toHaveText(JSON.stringify(resource, null, 2));
  expect(requests).toEqual([]);
});

test("Pod details show overview, containers, and conditions", async ({ page }) => {
  const app = await openApp(page, "resource");
  await result(page, {
    apiVersion: "v1", kind: "Pod", metadata: { name: "web", namespace: "team-a", labels: { app: "web" } },
    spec: { nodeName: "worker-1", containers: [{ name: "web", image: "nginx:latest" }, { name: "sidecar", image: "busybox:latest" }],
      initContainers: [{ name: "setup", image: "busybox:latest" }] },
    status: { phase: "Running", podIP: "10.0.0.2", containerStatuses: [
      { name: "web", ready: true, restartCount: 2, state: { running: {} } },
      { name: "sidecar", ready: false, restartCount: 0, state: { waiting: { reason: "ImagePullBackOff", message: "Image unavailable" } } },
    ], conditions: [{ type: "Ready", status: "False", reason: "ContainersNotReady", message: "Sidecar not ready" }] },
  });
  await expect(app.getByRole("region", { name: "Overview", exact: true })).toContainText("team-a");
  await expect(app.getByRole("region", { name: "Overview", exact: true })).toContainText("worker-1");
  await expect(app.getByRole("region", { name: "Containers", exact: true }).locator("tbody tr")).toHaveText([
    "webnginx:latestYes2running", "sidecarbusybox:latestNo0ImagePullBackOffImage unavailable",
  ]);
  await expect(app.getByRole("region", { name: "Init containers", exact: true })).toContainText("Unknown");
  await expect(app.getByRole("region", { name: "Conditions", exact: true })).toContainText("Sidecar not ready");
  await expect(app.locator("pre")).not.toBeVisible();
});

test("generic resource details expand nested fields and safely render metadata", async ({ page }) => {
  const app = await openApp(page, "resource");
  const unsafe = '<img src="data:,invalid" onerror="window.injected=true">';
  await result(page, { apiVersion: "v1", kind: "Service", metadata: { name: unsafe, labels: { [unsafe]: "literal" } },
    spec: { type: "ClusterIP", ports: [{ port: 80, targetPort: 8080 }] } });
  await expect(app.getByRole("heading", { level: 1 })).toHaveText("Service " + unsafe);
  await expect(app.getByRole("region", { name: "Labels", exact: true })).toContainText(unsafe);
  await app.getByText("Specification", { exact: true }).click();
  await expect(app.getByText("ClusterIP", { exact: true })).toBeVisible();
  await expect(app.getByText("8080", { exact: true })).toBeVisible();
  await expect(app.locator("img")).toHaveCount(0);
  await result(page, { apiVersion: "v1", kind: "Namespace", metadata: { name: "replacement" } });
  await expect(app.getByRole("heading", { level: 1 })).toHaveText("Namespace replacement");
  await expect(app.getByText("Specification", { exact: true })).toHaveCount(0);
});

for (const kind of ["table", "metrics", "resource"]) {
  test(`${kind} responds to ping and recovers from tool cancellation`, async ({ page }) => {
    const app = await openApp(page, kind);
    for (const id of [0, 1, "host-ping"]) {
      await send(page, { id, method: "ping" });
      await expect.poll(() => page.evaluate(id => window.messages.find(m => m.id === id && Object.hasOwn(m, "result")), id))
        .toEqual({ jsonrpc: "2.0", id, result: {} });
    }
    await send(page, { method: "ui/notifications/tool-cancelled" });
    await expect(app.getByRole("status")).toHaveText("Tool execution cancelled.");
    const reason = '<img src="invalid" onerror="window.injected=true">';
    await send(page, { method: "ui/notifications/tool-cancelled", params: { reason } });
    await expect(app.getByRole("status")).toHaveText("Tool execution cancelled: " + reason);
    await expect(app.locator("img")).toHaveCount(0);
    await result(page, kind === "resource"
      ? { apiVersion: "v1", kind: "Pod", metadata: { name: "recovered" } }
      : { items: [{ Name: "recovered" }] });
    await expect(app.locator("#app")).toContainText("recovered");
    await expect(app.getByRole("status")).toHaveCount(0);
  });

  test(`${kind} initializes, rejects non-parent messages, and acknowledges teardown`, async ({ page }) => {
    const app = await openApp(page, kind);
    await expect.poll(() => page.evaluate(() => window.messages.find(m => m.method === "ui/initialize")?.params.appInfo.name)).toBe("kubernetes-mcp-server-" + kind);
    await send(page, { id: 1, result: { protocolVersion: "2026-01-26" } });
    await expect.poll(() => page.evaluate(() => window.messages.some(m => m.method === "ui/notifications/initialized"))).toBe(true);
    const rejected = app.locator("#app");
    await app.evaluate(() => {
      window.postMessage({ jsonrpc: "2.0", id: 1, error: { message: "spoofed" } }, "*");
      return new Promise(resolve => window.addEventListener("message", resolve, { once: true }));
    });
    await expect(rejected).toHaveText("Loading data…");
    await send(page, { id: 1, error: { message: "initialization failed" } });
    await expect(rejected).toHaveText("Unable to load data: initialization failed");
    for (const id of [1, 2]) {
      await send(page, { id, method: "ui/resource-teardown", params: {} });
      await expect.poll(() => page.evaluate(id => window.messages.find(m => m.id === id && Object.hasOwn(m, "result"))?.result, id)).toEqual({});
    }
  });
}

for (const colorScheme of ["light", "dark"]) {
  test(`host theme overrides ${colorScheme} OS preference and survives partial updates`, async ({ page }) => {
    await page.emulateMedia({ colorScheme });
    const app = await openApp(page);
    const background = () => app.evaluate(() => getComputedStyle(document.body).backgroundColor);
    const light = "rgb(255, 255, 255)", dark = "rgb(31, 41, 55)";
    await expect.poll(background).toBe(colorScheme === "light" ? light : dark);
    const theme = colorScheme === "light" ? "dark" : "light";
    await send(page, { id: 1, result: { protocolVersion: "2026-01-26", hostContext: { theme } } });
    await expect.poll(background).toBe(theme === "light" ? light : dark);
    await send(page, { method: "ui/notifications/host-context-changed", params: { locale: "en-US" } });
    await expect.poll(() => app.evaluate(() => document.documentElement.dataset.theme)).toBe(theme);
    await expect.poll(background).toBe(theme === "light" ? light : dark);
    await send(page, { method: "ui/notifications/host-context-changed", params: { theme: colorScheme } });
    await expect.poll(background).toBe(colorScheme === "light" ? light : dark);
  });
}
