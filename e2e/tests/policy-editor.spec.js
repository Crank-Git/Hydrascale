import { expect, test } from "@playwright/test";

// Covers the "Read and change the upstream policy in Policy" manual page: read the
// document of a tailnet with a working credential, edit it, revert it, validate it, and
// confirm Push enables and disables around a successful Validate. This test never clicks
// Push: the sandbox's tailnets are real tailnets, and a push changes what every device in
// the tailnet reaches.

test("edits, reverts, and validates the policy document of a tailnet with a working credential", async ({ page }) => {
  await page.goto("/#/policy");
  await expect(page.locator("#view-heading")).toHaveText("Policy");

  await page.locator('[data-id="jbones"]').click();

  const doc = page.getByRole("textbox", { name: "The policy document of jbones" });
  await expect(doc).toBeVisible();

  const editedChip = page.locator(".pol-bar .chip", { hasText: "edited" });
  const discard = page.getByRole("button", { name: "Discard", exact: true });
  const validate = page.getByRole("button", { name: "Validate", exact: true });
  const push = page.getByRole("button", { name: "Push", exact: true });

  await expect(editedChip).toBeHidden();
  await expect(discard).toBeDisabled();
  await expect(push).toBeDisabled();

  const original = await doc.inputValue();

  // An edit shows the "edited" chip and enables Discard, but not Push: FR-policy-25
  // enables Push only after a successful Validate of the current text.
  await doc.fill(`${original}\n// e2e-smoke-test-edit\n`);
  await expect(editedChip).toBeVisible();
  await expect(discard).toBeEnabled();
  await expect(push).toBeDisabled();

  // Reverting to the exact original text clears the "edited" chip, because the console
  // compares the document to the original text, not to whether the field changed at all.
  await doc.fill(original);
  await expect(editedChip).toBeHidden();
  await expect(discard).toBeDisabled();

  await validate.click();
  await expect(page.getByText("The control server accepted the document.")).toBeVisible();
  await expect(push).toBeEnabled();

  // Editing again after a successful Validate disables Push again, until the operator
  // validates the new text.
  await doc.fill(`${original}\n// e2e-smoke-test-edit\n`);
  await expect(editedChip).toBeVisible();
  await expect(push).toBeDisabled();

  // Leave the tailnet's policy document exactly as this test found it.
  await doc.fill(original);
  await expect(editedChip).toBeHidden();
});
