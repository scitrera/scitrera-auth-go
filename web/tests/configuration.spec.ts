// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test("first operator, tenant/domain/policy/user/membership workflows and persistence", async ({
  page,
}, testInfo) => {
  const credentials = JSON.parse(
    readFileSync(process.env.AUTH_BROWSER_TOKEN_FILE!, "utf8"),
  );
  const [name, token] = Object.entries(credentials.operators)[0] as [
    string,
    string,
  ];
  const tenant = `browser-${Date.now()}`;
  const email = `person@${tenant}.example.com`;
  const save = async (name: string) => {
    const refreshed = page.waitForResponse(
      (r) =>
        r.request().method() === "GET" &&
        new RegExp(
          name === "Save user" ? "/users/[^/?]+$" : "/tenants/[^/?]+/auth$",
        ).test(r.url()),
    );
    const response = page.waitForResponse(
      (r) =>
        r.request().method() === "PUT" &&
        r.url().includes("/api/auth-admin/v1/"),
    );
    await page.getByRole("button", { name, exact: true }).click();
    expect((await response).status()).toBe(200);
    // Wait for the post-save data refresh before starting a second write.
    await refreshed;
  };
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const dashboardURL = `${process.env.AUTH_BROWSER_URL ?? "http://127.0.0.1:9082"}/admin/`;
  await page.route("https://links.example.test/", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: `<a href="${dashboardURL}">Open admin</a>`,
    }),
  );
  await page.goto("https://links.example.test/");
  await page.getByRole("link", { name: "Open admin" }).click();
  await expect(
    page.getByRole("heading", { name: "Operator sign-in" }),
  ).toBeVisible();
  await page.getByText("First operator setup").click();
  await expect(
    page.getByText("scitrera-auth-proxy bootstrap", { exact: false }),
  ).toBeVisible();
  await page.getByLabel("Operator name").fill(name);
  await page.getByLabel("Operator token").fill(token);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Tenants", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Create tenant" }).click();
  let modal = page.getByRole("dialog");
  await modal.getByLabel("Name", { exact: true }).fill("Example Research");
  await modal.getByLabel("Slug").fill(tenant);
  await modal.getByRole("button", { name: "Create", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: tenant })
    .getByRole("button", { name: "Configure" })
    .click();
  await expect(
    page.getByLabel("Automatically add eligible users to this tenant", {
      exact: true,
    }),
  ).not.toBeChecked();
  await page.getByLabel("New domain").fill(`${tenant}.example.com`);
  await page.getByRole("button", { name: "Add domain", exact: true }).click();
  await expect(
    page.getByText(`${tenant}.example.com`, { exact: true }),
  ).toBeVisible();
  await page.getByLabel("Tenant name").fill("Example Research Lab");
  await save("Save profile");
  await expect(page.getByLabel("Tenant name")).toHaveValue(
    "Example Research Lab",
  );
  await page
    .getByLabel("Azure tenant IDs (tid)")
    .fill("11111111-1111-4111-8111-111111111111");
  await page
    .getByLabel("Restrict Google hosted domains", { exact: true })
    .check();
  await page
    .getByLabel("Google hosted domains (hd)", { exact: true })
    .fill("example.com\nsecond.example.org\n\n");
  await page
    .getByLabel(
      "Allow accounts without a hosted domain (registered members only)",
      { exact: true },
    )
    .check();
  await page
    .getByLabel("Automatically add eligible users to this tenant", {
      exact: true,
    })
    .check();
  await save("Save sign-in policy");
  await expect(page.getByLabel("Azure tenant IDs (tid)")).toHaveValue(
    "11111111-1111-4111-8111-111111111111",
  );
  await expect(
    page.getByLabel("Google hosted domains (hd)", { exact: true }),
  ).toHaveValue("example.com\nsecond.example.org\n\n");
  await expect(
    page.getByLabel(
      "Allow accounts without a hosted domain (registered members only)",
      { exact: true },
    ),
  ).toBeChecked();
  const storedGoogle = await page.evaluate(async (slug) => {
    const response = await fetch(`/api/auth-admin/v1/tenants/${slug}/auth`);
    return (await response.json()).data.checks.google;
  }, tenant);
  expect(storedGoogle.hd).toEqual(["example.com", "second.example.org", ""]);
  // Both editor directions preserve multiple domains, the explicit blank and unknown claims.
  await page
    .getByLabel("Edit all claim checks as JSON", { exact: true })
    .check();
  const rawChecks = JSON.parse(
    await page
      .getByLabel("Provider claim checks", { exact: true })
      .inputValue(),
  );
  rawChecks.google.custom = "preserved";
  await page
    .getByLabel("Provider claim checks", { exact: true })
    .fill(JSON.stringify(rawChecks));
  await page
    .getByLabel("Edit all claim checks as JSON", { exact: true })
    .uncheck();
  await expect(
    page.getByLabel("Google hosted domains (hd)", { exact: true }),
  ).toHaveValue("example.com\nsecond.example.org");
  await expect(
    page.getByLabel(
      "Allow accounts without a hosted domain (registered members only)",
      { exact: true },
    ),
  ).toBeChecked();
  await save("Save sign-in policy");
  expect(
    await page.evaluate(
      async (slug) =>
        (await (await fetch(`/api/auth-admin/v1/tenants/${slug}/auth`)).json())
          .data.checks.google.custom,
      tenant,
    ),
  ).toBe("preserved");
  await expect(page).toHaveURL(new RegExp(`/admin/tenants/${tenant}$`));
  await page.reload();
  await expect(
    page.getByRole("heading", { name: tenant, exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Tenant name")).toHaveValue(
    "Example Research Lab",
  );
  await expect(page.getByLabel("Azure tenant IDs (tid)")).toHaveValue(
    "11111111-1111-4111-8111-111111111111",
  );
  await expect(
    page.getByLabel("Automatically add eligible users to this tenant", {
      exact: true,
    }),
  ).toBeChecked();
  expect(
    await page.evaluate(
      async (slug) =>
        (await (await fetch(`/api/auth-admin/v1/tenants/${slug}/auth`)).json())
          .data.auto_add,
      tenant,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("tenant-desktop.png"),
    fullPage: true,
  });
  // API failure feedback: duplicate normalized domain remains visible and unchanged.
  await page
    .getByLabel("New domain")
    .fill(`${tenant.toUpperCase()}.EXAMPLE.COM.`);
  await page.getByRole("button", { name: "Add domain", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("already exists");
  // Another operator commits while this form stays open: UI must show a conflict.
  await page.evaluate(async () => {
    const me = await fetch("/api/auth-admin/v1/session/me").then((r) =>
      r.json(),
    );
    const status = await fetch("/api/auth-admin/v1/status").then((r) =>
      r.json(),
    );
    await fetch("/api/auth-admin/v1/tenants", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": me.csrf_token,
        "If-Match": `"${status.revision}"`,
      },
      body: JSON.stringify({
        slug: `concurrent-${Date.now()}`,
        name: "Concurrent tenant",
        enabled: true,
      }),
    });
  });
  await page.getByLabel("Tenant name").fill("Stale edit");
  await page.getByRole("button", { name: "Save profile" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Configuration changed" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Reload configuration" }).click();
  await expect(page.getByLabel("Tenant name")).toHaveValue(
    "Example Research Lab",
  );
  await page.getByRole("button", { name: "Users", exact: true }).click();
  await page.getByRole("button", { name: "Create user" }).click();
  modal = page.getByRole("dialog");
  await modal.getByLabel("Name", { exact: true }).fill("Example User");
  await modal.getByLabel("Email", { exact: true }).fill(email);
  await modal.getByRole("button", { name: "Create", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: email })
    .getByRole("button", { name: "Configure" })
    .click();
  await page.getByLabel("Tenant slug to add").fill(tenant);
  await page
    .getByRole("button", { name: "Add membership", exact: true })
    .click();
  await expect(
    page.getByRole("listitem").filter({ hasText: tenant }),
  ).toBeVisible();
  await page.getByLabel("Default tenant", { exact: true }).selectOption(tenant);
  await save("Save user");
  await expect(page.getByLabel("Default tenant", { exact: true })).toHaveValue(
    tenant,
  );
  await page.getByLabel("User enabled").uncheck();
  await save("Save user");
  await expect(page.getByLabel("User enabled")).not.toBeChecked();
  await page
    .getByRole("button", { name: `Remove membership ${tenant}`, exact: true })
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Confirm removal" })
    .click();
  await expect(page.getByLabel("Default tenant", { exact: true })).toHaveValue(
    "",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: testInfo.outputPath("user-mobile.png"),
    fullPage: true,
  });
  await expect(
    page.getByRole("button", { name: "Save user", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Tenants", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: tenant })
    .getByRole("button", { name: "Configure" })
    .click();
  await page
    .getByLabel("Automatically add eligible users to this tenant", {
      exact: true,
    })
    .uncheck();
  await page.getByLabel("Azure tenant IDs (tid)").fill("");
  await expect(
    page.getByLabel("Google hosted domains (hd)", { exact: true }),
  ).toHaveValue("example.com\nsecond.example.org");
  await save("Save sign-in policy");
  await expect(page.getByLabel("Azure tenant IDs (tid)")).toHaveValue("");
  expect(
    await page.evaluate(
      async (slug) =>
        (await (await fetch(`/api/auth-admin/v1/tenants/${slug}/auth`)).json())
          .data.auto_add,
      tenant,
    ),
  ).toBe(false);
  await page
    .getByRole("button", { name: `Remove ${tenant}.example.com`, exact: true })
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Confirm removal" })
    .click();
  await expect(
    page.getByText("No domains associated.", { exact: false }),
  ).toBeVisible();
  await page.getByLabel("Tenant enabled").uncheck();
  await save("Save profile");
  await expect(page.getByLabel("Tenant enabled")).not.toBeChecked();
  await page.screenshot({
    path: testInfo.outputPath("tenant-mobile.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Service status", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Service status" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Operator sign-in" }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Operator sign-in" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("tenant user filters and detail URLs survive reload and browser history", async ({
  page,
}) => {
  const credentials = JSON.parse(
    readFileSync(process.env.AUTH_BROWSER_TOKEN_FILE!, "utf8"),
  );
  const [name, token] = Object.entries(credentials.operators)[0] as [
    string,
    string,
  ];
  await page.goto("/admin/users");
  await page.getByLabel("Operator name").fill(name);
  await page.getByLabel("Operator token").fill(token);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Users", exact: true }),
  ).toBeVisible();
  const tag = `filter-${Date.now()}`;
  const fixture = await page.evaluate(async (tag) => {
    const me = await (await fetch("/api/auth-admin/v1/session/me")).json();
    const write = async (path: string, body: unknown) => {
      const status = await (await fetch("/api/auth-admin/v1/status")).json();
      const response = await fetch(`/api/auth-admin/v1${path}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": me.csrf_token,
          "If-Match": `"${status.revision}"`,
        },
        body: JSON.stringify(body),
      });
      if (!response.ok) throw new Error(`Fixture ${path}: ${response.status}`);
    };
    const alpha = `${tag}-a`,
      beta = `${tag}-b`;
    for (const slug of [alpha, beta])
      await write("/tenants", { slug, name: slug, enabled: true });
    const emails = [
      `alice@${tag}.example.com`,
      `bob@${tag}.example.com`,
      `carol@${tag}.example.com`,
    ];
    for (let i = 0; i < emails.length; i++)
      await write("/users", {
        email: emails[i],
        name: ["Alice", "Bob", "Carol"][i],
        enabled: i !== 2,
      });
    const users = (
      await (await fetch(`/api/auth-admin/v1/users?q=${tag}`)).json()
    ).data as { id: string; email: string }[];
    const id = (email: string) => users.find((u) => u.email === email)!.id;
    await write(`/users/${id(emails[0])}/memberships`, { tenant_slug: alpha });
    await write(`/users/${id(emails[1])}/memberships`, { tenant_slug: beta });
    await write(`/users/${id(emails[2])}/memberships`, { tenant_slug: alpha });
    await write(`/users/${id(emails[2])}/memberships`, { tenant_slug: beta });
    return { alpha, beta, emails, alice: id(emails[0]) };
  }, tag);
  await page.goto(`/admin/tenants/${fixture.alpha}`);
  await expect(page.getByLabel("Tenant name", { exact: true })).toHaveValue(
    fixture.alpha,
  );
  await page.getByRole("button", { name: "View users", exact: true }).click();
  await expect(page).toHaveURL(
    new RegExp(`/admin/users\\?tenant=${fixture.alpha}$`),
  );
  await expect(page.getByLabel("Tenant filter", { exact: true })).toHaveValue(
    fixture.alpha,
  );
  await expect(
    page.getByRole("row").filter({ hasText: fixture.emails[0] }),
  ).toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: fixture.emails[2] }),
  ).toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: fixture.emails[1] }),
  ).toHaveCount(0);
  await page
    .getByRole("row")
    .filter({ hasText: fixture.emails[0] })
    .getByRole("button", { name: "Configure" })
    .click();
  await expect(page).toHaveURL(
    new RegExp(`/admin/users/${fixture.alice}\\?tenant=${fixture.alpha}$`),
  );
  await page.reload();
  await expect(page.getByLabel("Email", { exact: true })).toHaveValue(
    fixture.emails[0],
  );
  await page.goBack();
  await expect(page.getByLabel("Tenant filter", { exact: true })).toHaveValue(
    fixture.alpha,
  );
  await page.goBack();
  await expect(page.getByLabel("Tenant name", { exact: true })).toHaveValue(
    fixture.alpha,
  );
  await page.goForward();
  await expect(page.getByLabel("Tenant filter", { exact: true })).toHaveValue(
    fixture.alpha,
  );
  await page.getByLabel("Search users", { exact: true }).fill("carol");
  await page
    .getByLabel("Enabled filter", { exact: true })
    .selectOption("false");
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(
    page.getByRole("row").filter({ hasText: fixture.emails[2] }),
  ).toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: fixture.emails[0] }),
  ).toHaveCount(0);
  const query = new URL(page.url()).searchParams;
  expect(query.get("q")).toBe("carol");
  expect(query.get("tenant")).toBe(fixture.alpha);
  expect(query.get("enabled")).toBe("false");
  await page.reload();
  await expect(page.getByLabel("Search users", { exact: true })).toHaveValue(
    "carol",
  );
  await expect(page.getByLabel("Enabled filter", { exact: true })).toHaveValue(
    "false",
  );
  await page.goBack();
  await expect(page.getByLabel("Search users", { exact: true })).toHaveValue(
    "",
  );
  await expect(page.getByLabel("Enabled filter", { exact: true })).toHaveValue(
    "",
  );
  await expect(
    page.getByRole("row").filter({ hasText: fixture.emails[0] }),
  ).toBeVisible();
  expect(await page.evaluate(() => window.history.state.authAdmin.tenant)).toBe(
    fixture.alpha,
  );
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  // Wide rows scroll inside the table; configuration remains reachable.
  await page
    .getByRole("row")
    .filter({ hasText: fixture.emails[0] })
    .getByRole("button", { name: "Configure" })
    .click();
  await expect(page.getByLabel("Email", { exact: true })).toHaveValue(
    fixture.emails[0],
  );
});
