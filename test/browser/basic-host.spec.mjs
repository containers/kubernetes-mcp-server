import { expect, test } from "@playwright/test";

test("namespaces_list renders in the MCP Apps basic host", async ({ page }) => {
  await page.goto(process.env.BROWSER_TEST_URL || "http://127.0.0.1:18082");
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
