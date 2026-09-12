// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test("browser sessions: pagination history, individual and bulk revocation", async ({
  page,
}) => {
  test.skip(
    !process.env.AUTH_BROWSER_SESSIONS_FIXTURE,
    "requires disposable Go/PostgreSQL/Valkey fixture",
  );
  const fixture = JSON.parse(
    readFileSync(process.env.AUTH_BROWSER_SESSIONS_FIXTURE!, "utf8"),
  );
  const credentials = JSON.parse(readFileSync(fixture.token_file, "utf8"));
  const [operator, token] = Object.entries(credentials.operators)[0] as [
    string,
    string,
  ];
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`${fixture.base_url}/admin/users/${fixture.user_id}`);
  await page.getByLabel("Operator name").fill(operator);
  await page.getByLabel("Operator token").fill(token);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  const panel = page.locator(".browser-sessions");
  await expect(
    panel.getByRole("heading", { name: "Browser sessions" }),
  ).toBeVisible();
  await expect(
    panel.getByRole("button", { name: "Revoke session", exact: true }),
  ).toHaveCount(50);
  await panel.getByRole("button", { name: "Next sessions" }).click();
  await expect(page).toHaveURL(/session_offset=50/);
  await expect(
    panel.getByRole("button", { name: "Revoke session", exact: true }),
  ).toHaveCount(1);
  await page.reload();
  await expect(
    panel.getByRole("button", { name: "Revoke session", exact: true }),
  ).toHaveCount(1);
  await page.goBack();
  await expect(page).not.toHaveURL(/session_offset=/);
  await expect(
    panel.getByRole("button", { name: "Revoke session", exact: true }),
  ).toHaveCount(50);
  await panel
    .getByRole("button", { name: "Revoke session", exact: true })
    .first()
    .click();
  let dialog = page.getByRole("alertdialog");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).not.toBeVisible();
  await panel
    .getByRole("button", { name: "Revoke session", exact: true })
    .first()
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Revoke session", exact: true })
    .click();
  await expect(
    panel.getByText("Session revoked.", { exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByRole("button", { name: "Next sessions" }),
  ).not.toBeVisible();
  await panel
    .getByRole("button", { name: "Revoke all sessions", exact: true })
    .click();
  dialog = page.getByRole("alertdialog");
  await dialog
    .getByRole("button", { name: "Revoke all sessions", exact: true })
    .click();
  await expect(
    panel.getByText("All previous sessions revoked.", { exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByText("No valid sessions on this page."),
  ).toBeVisible();
  await page.reload();
  await expect(
    panel.getByText("No valid sessions on this page."),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
