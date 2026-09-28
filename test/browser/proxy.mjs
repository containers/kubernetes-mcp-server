import http from "node:http";

const mcpURL = new URL(process.env.MCP_SERVER_URL);
const hostURL = new URL(process.env.BASIC_HOST_URL || "http://127.0.0.1:18080");
const port = Number(process.env.BROWSER_PROXY_PORT || "18082");

http.createServer((request, response) => {
  const target = request.url.startsWith("/mcp") ? mcpURL : hostURL;
  const headers = { ...request.headers, host: target.host };
  // The browser origin is the test proxy. Do not forward it to the local MCP
  // endpoint: its SDK correctly rejects a cross-origin request as a DNS
  // rebinding defense, while this proxy is the same-origin test boundary.
  delete headers.origin;
  const upstream = http.request(new URL(request.url, target), {
    method: request.method,
    headers,
  }, (upstreamResponse) => {
    response.writeHead(upstreamResponse.statusCode, upstreamResponse.headers);
    upstreamResponse.pipe(response);
  });
  upstream.on("error", (err) => {
    response.writeHead(502);
    response.end(err.message);
  });
  request.pipe(upstream);
}).listen(port, "127.0.0.1", () => console.log(`Browser-test proxy: http://127.0.0.1:${port}`));
