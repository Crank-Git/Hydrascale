import { expect, test } from "@playwright/test";

// Covers the "Change a local rule in Access" manual page: docs/manual/access-editor.md.
// Each test starts on a fresh page load, so the console holds no staged edit at the
// start (a staged edit lives in the browser only; the console never persists one).

test.beforeEach(async ({ page }) => {
  await page.goto("/#/access");
  await expect(page.locator("#view-heading")).toHaveText("Access");
});

test("selects a source tailnet and narrows the allowed-paths diagram to it", async ({ page }) => {
  const jbonesNode = page.getByRole("button", { name: /^jbones,/ });
  await jbonesNode.click();

  await expect(page.getByText("Every other path is muted.")).toBeVisible();

  // Selecting the same node again returns the diagram to every source.
  await jbonesNode.click();
  await expect(page.getByText("Every other path is muted.")).toHaveCount(0);
});

test("stages a rule from the reachability matrix and discards it back to zero", async ({ page }) => {
  const stagedCount = page.locator(".ac-states .chip").last();
  await expect(stagedCount).toHaveText("0 staged");

  const discard = page.getByRole("button", { name: "Discard", exact: true });
  await expect(discard).toBeDisabled();

  const square = page.getByRole("button", { name: "jbones to havoc, no rule" });
  await square.click();

  await expect(stagedCount).toHaveText("1 staged");
  await expect(page.getByRole("button", { name: "jbones to havoc, allowed" })).toBeVisible();
  await expect(page.getByText("Staged edits")).toBeVisible();

  await discard.click();

  await expect(stagedCount).toHaveText("0 staged");
  await expect(page.getByRole("button", { name: "jbones to havoc, no rule" })).toBeVisible();
  await expect(page.getByText("Staged edits")).toHaveCount(0);
});
