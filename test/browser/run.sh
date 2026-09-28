#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
work="$root/_output/browser"
apps="$work/ext-apps"
playwright="$work/playwright"
revision=82221c0c8ce7661efa6771c9d461511b1650495f
host_port=${BASIC_HOST_PORT:-18080}
proxy_port=${BROWSER_PROXY_PORT:-18082}
host_url="http://127.0.0.1:$host_port"
proxy_url="http://127.0.0.1:$proxy_port"

mkdir -p "$work"
mkdir -p "$playwright"
cp "$root/test/browser/package.json" "$playwright/package.json"
cp "$root/test/browser/basic-host.spec.mjs" "$playwright/basic-host.spec.mjs"
if ! curl -sS --connect-timeout 2 -o /dev/null "$MCP_SERVER_URL"; then
  echo "MCP_SERVER_URL is not reachable: $MCP_SERVER_URL" >&2
  exit 1
fi
if [[ ! -d "$apps/.git" ]]; then
  git clone https://github.com/modelcontextprotocol/ext-apps.git "$apps"
fi
git -C "$apps" fetch --depth 1 origin "$revision"
git -C "$apps" checkout --detach "$revision"

npm install --prefix "$playwright"
npx --prefix "$playwright" playwright install chromium
npm install --prefix "$apps"
npm --prefix "$apps/examples/basic-host" run build

SERVERS="[\"$proxy_url/mcp\"]" HOST_PORT="$host_port" SANDBOX_PORT=8081 "$apps/node_modules/.bin/bun" --watch "$apps/examples/basic-host/serve.ts" &
host_pid=$!
MCP_SERVER_URL="$MCP_SERVER_URL" BASIC_HOST_URL="$host_url" BROWSER_PROXY_PORT="$proxy_port" node "$root/test/browser/proxy.mjs" &
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
  curl -fsS "$proxy_url" >/dev/null && break
  sleep 1
done
(
  cd "$playwright"
  BROWSER_TEST_URL="$proxy_url" npx playwright test basic-host.spec.mjs
)
