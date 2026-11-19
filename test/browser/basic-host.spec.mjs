import { expect, test } from "@playwright/test";

// The loopback proxy uses a short-lived, self-signed test certificate.
test.use({ ignoreHTTPSErrors: true });

test("namespaces_list renders in the MCP Apps basic host", async ({ page }) => {
  await page.goto(process.env.BROWSER_TEST_URL || "https://127.0.0.1:18082");
  const toolSelect = page.locator("select").nth(1);
  await expect(toolSelect.locator('option[value="namespaces_list"]')).toHaveCount(1, { timeout: 30_000 });
  await toolSelect.selectOption("namespaces_list");
  await page.locator("textarea").fill("{}");
  await page.getByRole("button", { name: "Call Tool" }).click();

  const sandbox = page.frameLocator("iframe");
  await expect(sandbox.locator("iframe")).toHaveCount(1, { timeout: 30_000 });
  const app = sandbox.frameLocator("iframe");
  await expect(app.locator("table")).toBeVisible();
  await expect(app.getByRole("columnheader", { name: "Name" })).toBeVisible();

  const names = await app.locator("tbody tr td:first-child").allTextContents();
  expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b)));
});

test("empty resources_list renders an empty state in the MCP Apps basic host", async ({ page }) => {
  await page.goto(process.env.BROWSER_TEST_URL || "https://127.0.0.1:18082");
  const toolSelect = page.locator("select").nth(1);
  await expect(toolSelect.locator('option[value="resources_list"]')).toHaveCount(1, { timeout: 30_000 });
  await toolSelect.selectOption("resources_list");
  await page.locator("textarea").fill(JSON.stringify({ apiVersion: "v1", kind: "Pod", labelSelector: "mcp-app-test=does-not-exist" }));
  await page.getByRole("button", { name: "Call Tool" }).click();
  const sandbox = page.frameLocator("iframe");
  await expect(sandbox.locator("iframe")).toHaveCount(1, { timeout: 30_000 });
  await expect(sandbox.frameLocator("iframe").getByRole("status")).toHaveText("No results found.");
});
