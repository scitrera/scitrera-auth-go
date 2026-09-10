// SPDX-License-Identifier: AGPL-3.0-only
import { test } from "node:test";
import assert from "node:assert/strict";
import { defaultRoute, readRoute, routeURL } from "./routes.ts";
test("user detail URL retains tenant, search, status and pagination", () => {
  const route = {
    ...defaultRoute("users"),
    selected: "11111111-1111-4111-8111-111111111111",
    tenant: "alpha",
    q: "person+test@example.com",
    enabled: "false" as const,
    offset: 50,
  };
  const url = new URL(routeURL(route), "https://admin.example.test");
  assert.deepEqual(readRoute(url.pathname, url.search), route);
  assert.equal(readRoute(url.pathname, url.search).tenant, "alpha");
});
test("tenant new is a real slug; creation state is an explicit query action", () => {
  assert.equal(readRoute("/admin/tenants/new", "").selected, "new");
  const route = { ...defaultRoute("tenants"), creating: true, q: "example" };
  const url = new URL(routeURL(route), "https://admin.example.test");
  assert.deepEqual(readRoute(url.pathname, url.search), route);
});
test("invalid routes and pagination do not silently select an arbitrary record", () => {
  assert.equal(readRoute("/admin/users/one/extra", "").notFound, true);
  assert.equal(readRoute("/admin/users/%2fapi", "").notFound, true);
  assert.equal(readRoute("/admin/users/%ZZ", "").notFound, true);
  assert.equal(readRoute("/admin/unknown", "").notFound, true);
  assert.equal(readRoute("/admin/users", "?offset=-1").offset, 0);
  assert.equal(readRoute("/admin/users", "?offset=1000001").offset, 0);
  assert.equal(routeURL(readRoute("/admin/", "")), "/admin/tenants");
});
