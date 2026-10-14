#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
work="$root/_output/browser"
apps="$work/ext-apps"
playwright="$work/playwright"
revision=82221c0c8ce7661efa6771c9d461511b1650495f
host_port=${BASIC_HOST_PORT:-18080}
proxy_port=${BROWSER_PROXY_PORT:-18082}
sandbox_proxy_port=${BROWSER_SANDBOX_PROXY_PORT:-18083}
host_url="https://127.0.0.1:$host_port"
proxy_url="https://127.0.0.1:$proxy_port"
sandbox_proxy_url="https://127.0.0.1:$sandbox_proxy_port"
proxy_key="$work/proxy-key.pem"
proxy_cert="$work/proxy-cert.pem"

mkdir -p "$work"
mkdir -p "$playwright"
cp "$root/test/browser/package.json" "$playwright/package.json"
cp "$root/test/browser/basic-host.spec.mjs" "$playwright/basic-host.spec.mjs"
if ! curl --cacert "$proxy_cert" -sS --connect-timeout 2 -o /dev/null "$MCP_SERVER_URL"; then
  echo "MCP_SERVER_URL is not reachable: $MCP_SERVER_URL" >&2
  exit 1
fi
if [[ ! -d "$apps/.git" ]]; then
  git clone https://github.com/modelcontextprotocol/ext-apps.git "$apps"
fi
git -C "$apps" fetch --depth 1 origin "$revision"
git -C "$apps" checkout --detach "$revision"

# The pinned example hardcodes HTTP in its sandbox URL and referrer allowlist.
# Use HTTPS for both, preserving the loopback allowlist and separate origins.
BROWSER_SANDBOX_URL="$sandbox_proxy_url/sandbox.html" node --input-type=module - "$apps" "$revision" <<'JS'
import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
const replacements = [
  ["implementation.ts", '"http://localhost:8081/sandbox.html"', JSON.stringify(process.env.BROWSER_SANDBOX_URL)],
  ["sandbox.ts", "const ALLOWED_REFERRER_PATTERN = /^http:", "const ALLOWED_REFERRER_PATTERN = /^https:"],
  ["../serve.ts", 'import express from "express";', 'import https from "node:https";\nimport { readFileSync } from "node:fs";\nimport express from "express";\nconst tls = { key: readFileSync(process.env.BROWSER_PROXY_KEY!), cert: readFileSync(process.env.BROWSER_PROXY_CERT!) };'],
];
for (const [file, original, replacement] of replacements) {
  const relativePath = file === "../serve.ts" ? "examples/basic-host/serve.ts" : `examples/basic-host/src/${file}`;
  const path = `${process.argv[2]}/${relativePath}`;
  const source = execFileSync("git", ["-C", process.argv[2], "show", `${process.argv[3]}:${relativePath}`], { encoding: "utf8" });
  if (!source.includes(original)) throw new Error(`Basic host HTTPS setup has changed in ${file}`);
  let updated = source.replace(original, replacement);
  if (file === "../serve.ts") {
    for (const app of ["hostApp", "sandboxApp"]) {
      const port = app === "hostApp" ? "HOST_PORT" : "SANDBOX_PORT";
      const listener = `${app}.listen(${port}, (err) => {
  if (err) {
    console.error("Error starting server:", err);
    process.exit(1);
  }`;
      if (!updated.includes(listener)) throw new Error(`Basic host listener has changed for ${app}`);
      updated = updated.replace(listener, `https.createServer(tls, ${app}).on("error", (err) => {
  console.error("Error starting server:", err);
  process.exit(1);
}).listen(${port}, "127.0.0.1", () => {`);
    }
    updated = updated.replaceAll("http://localhost:", "https://localhost:");
  }
  writeFileSync(path, updated);
}
JS

npm install --prefix "$playwright"
npx --prefix "$playwright" playwright install chromium
npm install --prefix "$apps"
npm --prefix "$apps/examples/basic-host" run build

SERVERS="[\"$proxy_url/mcp\"]" HOST_PORT="$host_port" SANDBOX_PORT=8081 BROWSER_PROXY_KEY="$proxy_key" BROWSER_PROXY_CERT="$proxy_cert" "$apps/node_modules/.bin/bun" --watch "$apps/examples/basic-host/serve.ts" &
host_pid=$!
MCP_SERVER_URL="$MCP_SERVER_URL" BASIC_HOST_URL="$host_url" BROWSER_PROXY_PORT="$proxy_port" BROWSER_SANDBOX_PROXY_PORT="$sandbox_proxy_port" BROWSER_PROXY_KEY="$proxy_key" BROWSER_PROXY_CERT="$proxy_cert" node "$root/test/browser/proxy.mjs" &
proxy_pid=$!

kill_tree() {
  local pid=$1 child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$child"
  done
  kill "$pid" 2>/dev/null || true
}

cleanup() {
  kill_tree "$host_pid"
  kill_tree "$proxy_pid"
}
trap cleanup EXIT

for _ in {1..30}; do
  curl --cacert "$proxy_cert" -fsS "$proxy_url" >/dev/null && \
    curl --cacert "$proxy_cert" -fsS "$sandbox_proxy_url/sandbox.html" >/dev/null && break
  sleep 1
done
curl --cacert "$proxy_cert" -fsS "$proxy_url" >/dev/null || { echo "Browser-test proxy did not become ready"; exit 1; }
curl --cacert "$proxy_cert" -fsS "$sandbox_proxy_url/sandbox.html" >/dev/null || { echo "Browser-test sandbox proxy did not become ready"; exit 1; }
(
  cd "$playwright"
  BROWSER_TEST_URL="$proxy_url" npx playwright test basic-host.spec.mjs
)
