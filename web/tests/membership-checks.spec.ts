// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

// This mutation test is opt-in and refuses public or implicit/default targets.
// Its harness starts an owned disposable DB and a separate loopback auth server.
test("local disposable membership override editor saves and clears", async ({
  page,
}) => {
  const target = process.env.AUTH_BROWSER_URL;
  test.skip(
    process.env.AUTH_MEMBERSHIP_BROWSER_LOCAL !== "1",
    "requires disposable local membership harness",
  );
  if (
    !target ||
    new URL(target).hostname !== "127.0.0.1" ||
    new URL(target).protocol !== "http:"
  )
    throw new Error("Requires explicit HTTP loopback disposable auth server");
  const credentials = JSON.parse(
    readFileSync(process.env.AUTH_BROWSER_TOKEN_FILE!, "utf8"),
  );
  const [name, token] = Object.entries(credentials.operators)[0] as [
    string,
    string,
  ];
  await page.goto("/admin/");
  await page.getByLabel("Operator name").fill(name);
  await page.getByLabel("Operator token").fill(token);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Tenants", exact: true }),
  ).toBeVisible();
  const tenant = `member-${Date.now()}`;
  const id = await page.evaluate(async (tenant) => {
    const base = "/api/auth-admin/v1";
    const session = await (await fetch(base + "/session/me")).json();
    async function write(path: string, body: unknown) {
      const status = await (await fetch(base + "/status")).json();
      const result = await fetch(base + path, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": session.csrf_token,
          "If-Match": `"${status.revision}"`,
        },
        body: JSON.stringify(body),
      });
      if (!result.ok)
        throw new Error(`Fixture creation failed: ${result.status}`);
    }
    await write("/tenants", { slug: tenant, name: tenant, enabled: true });
    await write("/users", {
      email: `member@${tenant}.example`,
      name: "External member",
      enabled: true,
    });
    const users = await (
      await fetch(
        base + "/users?q=" + encodeURIComponent(`member@${tenant}.example`),
      )
    ).json();
    const id = users.data[0].id;
    await write(`/users/${id}/memberships`, { tenant_slug: tenant });
    return id;
  }, tenant);
  await page.goto(`/admin/users/${id}`);
  await page
    .getByRole("button", { name: `Edit sign-in checks for ${tenant}` })
    .click();
  const editor = page.getByLabel("Membership claim overrides");
  await expect(editor).toHaveValue("{}");
  const checks = { azure: { tid: ["22222222-2222-4222-8222-222222222222"] } };
  await editor.fill(JSON.stringify(checks));
  let saved = page.waitForResponse(
    (r) =>
      r.request().method() === "PUT" &&
      r.url().endsWith(`/memberships/${tenant}/auth`),
  );
  await page
    .getByRole("button", { name: "Save membership checks", exact: true })
    .click();
  expect((await saved).status()).toBe(200);
  await expect(editor).toHaveValue(JSON.stringify(checks, null, 2));
  await page.reload();
  await page
    .getByRole("button", { name: `Edit sign-in checks for ${tenant}` })
    .click();
  await expect(editor).toHaveValue(JSON.stringify(checks, null, 2));
  await page
    .getByRole("button", { name: "Use tenant defaults", exact: true })
    .click();
  saved = page.waitForResponse(
    (r) =>
      r.request().method() === "PUT" &&
      r.url().endsWith(`/memberships/${tenant}/auth`),
  );
  await page
    .getByRole("button", { name: "Save membership checks", exact: true })
    .click();
  expect((await saved).status()).toBe(200);
  await expect(editor).toHaveValue("{}");
});
