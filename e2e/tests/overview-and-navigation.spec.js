import { expect, test } from "@playwright/test";

// Covers the "Get oriented in the console" manual page: docs/manual/first-run.md.

test("lands on Overview and lists every tailnet with its reconciler state", async ({ page }) => {
  await page.goto("/");

  await expect(page.locator("#view-heading")).toHaveText("Overview");

  // The statistics row reports a tailnet count and a peer count as machine values.
  const stats = page.locator(".stat");
  await expect(stats.filter({ hasText: "Tailnets" }).locator(".stat-value")).toHaveText(/^\d+$/);
  await expect(stats.filter({ hasText: "Peers" }).locator(".stat-value")).toHaveText(/^\d+$/);

  // The reconciler state renders as a coloured dot plus a lowercase word, per the console's
  // state-rendering convention.
  const reconciler = stats.filter({ hasText: "Reconciler" }).locator(".stat-value");
  await expect(reconciler.locator(".dot")).toBeVisible();

  // Every declared tailnet draws a node in the topology diagram, labelled with its id.
  const topology = page.getByRole("img", { name: "The topology of every tailnet, the host, and the internet." });
  await expect(topology).toBeVisible();
  await expect(page.getByRole("button", { name: /^jbones,/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^havoc,/ })).toBeVisible();
});

test("navigates to every view from the main navigation bar", async ({ page }) => {
  await page.goto("/");

  const nav = page.getByRole("navigation", { name: "Main" });
  const heading = page.locator("#view-heading");

  const views = [
    ["namespaces", "Namespaces"],
    ["access", "Access"],
    ["policy", "Policy"],
    ["activity", "Activity"],
    ["settings", "Settings"],
    ["overview", "Overview"],
  ];

  for (const [dataView, expectedHeading] of views) {
    await nav.locator(`.nav-link[data-view="${dataView}"]`).click();
    await expect(heading).toHaveText(expectedHeading);
    await expect(nav.locator(`.nav-link[data-view="${dataView}"]`)).toHaveAttribute("aria-current", "page");
  }
});
