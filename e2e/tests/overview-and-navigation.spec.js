import { expect, test } from "@playwright/test";

// Covers the "Get oriented in the console" manual page: docs/manual/first-run.md.

test("lands on Overview and lists every tailnet with its reconciler state", async ({ page }) => {
  await page.goto("/");

  await expect(page.locator("#view-heading")).toHaveText("Overview");

  // The verdict line answers the glance: a coloured dot, a sentence, and counts that are
  // the sums of the board columns.
  const verdict = page.locator(".verdict");
  await expect(verdict.locator(".verdict-text .dot")).toBeVisible();
  const counts = verdict.locator(".verdict-counts .count");
  await expect(counts.nth(0)).toHaveText(/^\d+ tailnets?$/);
  await expect(counts.nth(1)).toHaveText(/^\d+ peers?$/);

  // The board holds one row per tailnet. The state renders as a coloured dot plus a
  // lowercase word, per the console's state-rendering convention.
  for (const id of ["jbones", "havoc"]) {
    const row = page.locator(`.board tbody tr[data-tailnet="${id}"]`);
    await expect(row).toBeVisible();
    await expect(row.locator("td.state").first().locator(".dot")).toBeVisible();
  }

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
