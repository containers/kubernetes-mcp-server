import https from "node:https";
import { readFileSync } from "node:fs";

const mcpURL = new URL(process.env.MCP_SERVER_URL);
const hostURL = new URL(process.env.BASIC_HOST_URL || "https://127.0.0.1:18080");
const sandboxURL = new URL(process.env.BASIC_SANDBOX_URL || "https://127.0.0.1:8081");
const port = Number(process.env.BROWSER_PROXY_PORT || "18082");
const sandboxPort = Number(process.env.BROWSER_SANDBOX_PROXY_PORT || "18083");

const tls = {
  key: readFileSync(process.env.BROWSER_PROXY_KEY),
  cert: readFileSync(process.env.BROWSER_PROXY_CERT),
};

for (const target of [mcpURL, hostURL, sandboxURL]) {
  if (target.protocol !== "https:" || target.hostname !== "127.0.0.1" || target.username || target.password) {
    throw new Error("Browser-test upstreams must use HTTPS on 127.0.0.1");
  }
}

function forward(request, response, target) {
  // Accept only origin-form paths. Never resolve a client URL against the
  // upstream URL: absolute and protocol-relative URLs could change its host.
  if (!request.url.startsWith("/") || request.url.startsWith("//") || request.url.includes("\\")) {
    response.writeHead(400, { "content-type": "text/plain; charset=utf-8" });
    response.end("Bad Request\n");
    return;
  }
  const headers = { ...request.headers, host: target.host };
  // The browser origin is the test proxy. Do not forward it to the local MCP
  // endpoint: its SDK correctly rejects a cross-origin request as a DNS
  // rebinding defense, while this proxy is the same-origin test boundary.
  delete headers.origin;
  const upstream = https.request({
    hostname: "127.0.0.1",
    port: target.port || 443,
    path: request.url,
    ca: tls.cert,
    method: request.method,
    headers,
  }, (upstreamResponse) => {
    response.writeHead(upstreamResponse.statusCode, upstreamResponse.headers);
    upstreamResponse.pipe(response);
  });
  upstream.on("error", (err) => {
    console.error("Browser-test proxy upstream request failed:", err);
    response.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    response.end("Bad Gateway\n");
  });
  request.pipe(upstream);
}

https.createServer(tls, (request, response) => {
  forward(request, response, request.url.startsWith("/mcp") ? mcpURL : hostURL);
}).listen(port, "127.0.0.1", () => console.log(`Browser-test proxy: https://127.0.0.1:${port}`));

// Keep the sandbox on a separate origin, as required by the MCP Apps host.
https.createServer(tls, (request, response) => {
  forward(request, response, sandboxURL);
}).listen(sandboxPort, "127.0.0.1", () => console.log(`Browser-test sandbox proxy: https://127.0.0.1:${sandboxPort}`));
