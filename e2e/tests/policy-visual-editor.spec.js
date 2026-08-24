import { expect, test } from "@playwright/test";

// Covers the "Use the Visual editor of Policy" manual page: docs/manual/policy-visual-editor.md.
// Every test opens the jbones tailnet, a real read-and-write Tailscale tailnet, and stages
// an edit in the browser only. No test clicks Push, so no edit here reaches the control
// server.

test.beforeEach(async ({ page }) => {
  await page.goto("/#/policy");
  await page.locator('[data-id="jbones"]').click();
  await page.getByRole("tab", { name: "Visual" }).click();
});

test("lists every section of the Visual editor with its live entry count", async ({ page }) => {
  const counts = {
    groups: "0",
    hosts: "0",
    tagOwners: "0",
    ipsets: "0",
    rules: "1",
    ssh: "1",
    autoApprovers: "0",
    nodeAttrs: "0",
    postures: "0",
    tests: "0",
  };
  for (const [key, count] of Object.entries(counts)) {
    await expect(page.locator(`.setrow[data-nav="${key}"] .mono`)).toHaveText(count);
  }
});

test("opens every section without tripping the Visual tab into an error state", async ({ page }) => {
  const keys = ["groups", "hosts", "tagOwners", "ipsets", "rules", "ssh", "autoApprovers", "nodeAttrs", "postures", "tests"];
  for (const key of keys) {
    await page.locator(`.setrow[data-nav="${key}"]`).click();
  }

  await expect(page.getByRole("tab", { name: "Visual" })).toBeEnabled();
  await expect(page.getByRole("button", { name: "Discard", exact: true })).toBeDisabled();
});

test("stages an SSH access rule from the New rule row and discards it", async ({ page }) => {
  await page.locator('.setrow[data-nav="ssh"]').click();

  const sshCount = page.locator('.setrow[data-nav="ssh"] .mono');
  await expect(sshCount).toHaveText("1");

  const discard = page.getByRole("button", { name: "Discard", exact: true });
  await expect(discard).toBeDisabled();

  await page.getByLabel("New rule source").fill("group:review-test");
  await page.getByLabel("New rule destination").fill("tag:review-test");
  await page.getByLabel("New rule users").fill("autogroup:nonroot");
  await page.getByRole("button", { name: "Add", exact: true }).click();

  await expect(sshCount).toHaveText("2");
  await expect(discard).toBeEnabled();

  // Leave the tailnet's policy document exactly as this test found it.
  await discard.click();
  await expect(sshCount).toHaveText("1");
  await expect(discard).toBeDisabled();
});

test("adds a test and runs it against the control server", async ({ page }) => {
  await page.locator('.setrow[data-nav="tests"]').click();

  const testsCount = page.locator('.setrow[data-nav="tests"] .mono');
  await expect(testsCount).toHaveText("0");

  await page.getByLabel("New test source").fill("smoke@example.test");
  await page.getByLabel("New test expected result").selectOption("accept");
  await page.getByLabel("New test destination").fill("100.64.0.1:443");
  await page.getByRole("button", { name: "Add", exact: true }).click();

  await expect(testsCount).toHaveText("1");

  await page.getByRole("button", { name: "Run", exact: true }).click();
  await expect(page.locator('.setentry[data-section="tests"] .dot.ok')).toBeVisible();
  await expect(page.getByText("pass")).toBeVisible();

  // Leave the tailnet's policy document exactly as this test found it.
  const discard = page.getByRole("button", { name: "Discard", exact: true });
  await discard.click();
  await expect(testsCount).toHaveText("0");
  await expect(discard).toBeDisabled();
});

// Review finding: adding a route CIDR sends a malformed request body. The control server
// answers 400, the console shows the raw error, and the Visual tab locks with Discard
// disabled, so only a page reload recovers. This test asserts the intended behaviour and
// stays skipped until the add action is fixed (#348).
test.fixme("stages an Auto-approvers route from the New route CIDR field", async ({ page }) => {
  await page.locator('.setrow[data-nav="autoApprovers"]').click();

  const autoApproversCount = page.locator('.setrow[data-nav="autoApprovers"] .mono');
  await expect(autoApproversCount).toHaveText("0");

  await page.getByLabel("New route CIDR").fill("10.20.0.0/24");
  await page.getByRole("button", { name: "Add a route", exact: true }).click();

  await expect(autoApproversCount).toHaveText("1");
  await expect(page.getByRole("button", { name: "Discard", exact: true })).toBeEnabled();
});

// Review finding: adding a node attributes entry fails server-side with no visible
// feedback; the fields clear and the count stays at zero. This test asserts the intended
// behaviour and stays skipped until the add action is fixed (#348).
test.fixme("stages a Node attributes entry from the New entry row", async ({ page }) => {
  await page.locator('.setrow[data-nav="nodeAttrs"]').click();

  const nodeAttrsCount = page.locator('.setrow[data-nav="nodeAttrs"] .mono');
  await expect(nodeAttrsCount).toHaveText("0");

  await page.getByLabel("New entry target").fill("tag:review-test");
  await page.getByLabel("New entry attribute").fill("funnel");
  await page.getByRole("button", { name: "Add", exact: true }).click();

  await expect(nodeAttrsCount).toHaveText("1");
  await expect(page.getByRole("button", { name: "Discard", exact: true })).toBeEnabled();
});

// Review finding: adding a posture serializes the entry as a malformed array instead of a
// name/expression map. The control server rejects it, and the Visual tab locks until the
// page reloads. This test asserts the intended behaviour and stays skipped until the add
// action is fixed (#345).
test.fixme("stages a posture from the New posture row", async ({ page }) => {
  await page.locator('.setrow[data-nav="postures"]').click();

  const posturesCount = page.locator('.setrow[data-nav="postures"] .mono');
  await expect(posturesCount).toHaveText("0");

  await page.getByLabel("New posture name").fill("posture:review-test");
  await page.getByLabel("New posture expression").fill("node:os == 'linux'");
  await page.getByRole("button", { name: "Add", exact: true }).click();

  await expect(posturesCount).toHaveText("1");
  await expect(page.getByRole("button", { name: "Discard", exact: true })).toBeEnabled();
});

// Review finding: a failing test blocks Push for the whole document, contradicting
// FR-vadv-14 ("a failing test does not block Push"). This test asserts the intended
// behaviour and stays skipped until Validate stops treating a failing test as a document
// error (#353).
test.fixme("leaves Push available with a failing test staged", async ({ page }) => {
  await page.locator('.setrow[data-nav="tests"]').click();

  await page.getByLabel("New test source").fill("smoke-fail@example.test");
  await page.getByLabel("New test expected result").selectOption("deny");
  await page.getByLabel("New test destination").fill("100.64.0.2:443");
  await page.getByRole("button", { name: "Add", exact: true }).click();

  await page.getByRole("button", { name: "Run", exact: true }).click();
  await expect(page.locator('.setentry[data-section="tests"] .dot.crit')).toBeVisible();

  await page.getByRole("button", { name: "Validate", exact: true }).click();
  await expect(page.getByRole("button", { name: "Push", exact: true })).toBeEnabled();
});
