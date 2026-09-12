// SPDX-License-Identifier: AGPL-3.0-only
export type Area = "tenants" | "users" | "status";
export interface AdminRoute {
  area: Area;
  selected: string;
  creating: boolean;
  q: string;
  tenant: string;
  enabled: "" | "true" | "false";
  offset: number;
  sessionOffset: number;
  notFound?: boolean;
}
export const defaultRoute = (area: Area = "tenants"): AdminRoute => ({
  area,
  selected: "",
  creating: false,
  q: "",
  tenant: "",
  enabled: "",
  offset: 0,
  sessionOffset: 0,
});
export function readRoute(pathname: string, search: string): AdminRoute {
  const route = defaultRoute();
  if (pathname !== "/admin" && !pathname.startsWith("/admin/"))
    return { ...route, notFound: true };
  const segments = pathname.slice("/admin".length).split("/").filter(Boolean);
  if (
    segments.length > 2 ||
    (segments[0] && !["tenants", "users", "status"].includes(segments[0]))
  )
    return { ...route, notFound: true };
  route.area = (segments[0] as Area) || "tenants";
  try {
    route.selected = decodeURIComponent(segments[1] || "");
  } catch {
    return { ...route, notFound: true };
  }
  if (
    route.selected.includes("/") ||
    (route.area === "status" && route.selected)
  )
    return { ...route, notFound: true };
  if (route.area === "status") return route;
  const query = new URLSearchParams(search);
  route.q = (query.get("q") ?? "").trim();
  route.tenant =
    route.area === "users"
      ? (query.get("tenant") ?? "").trim().toLowerCase()
      : "";
  const enabled = query.get("enabled");
  if (enabled === "true" || enabled === "false") route.enabled = enabled;
  const offset = query.get("offset") ?? "0";
  if (/^\d+$/.test(offset) && Number(offset) <= 1000000)
    route.offset = Number(offset);
  route.creating = !route.selected && query.get("action") === "create";
  const sessionOffset = query.get("session_offset") ?? "0";
  if (
    route.area === "users" &&
    route.selected &&
    /^\d+$/.test(sessionOffset) &&
    Number(sessionOffset) <= 1000000
  )
    route.sessionOffset = Number(sessionOffset);
  return route;
}
export function routeURL(route: AdminRoute): string {
  const params = new URLSearchParams();
  if (route.q) params.set("q", route.q);
  if (route.area === "users" && route.tenant)
    params.set("tenant", route.tenant);
  if (route.enabled) params.set("enabled", route.enabled);
  if (route.offset) params.set("offset", String(route.offset));
  if (route.area === "users" && route.selected && route.sessionOffset)
    params.set("session_offset", String(route.sessionOffset));
  if (route.creating && !route.selected) params.set("action", "create");
  const query = route.area !== "status" ? params.toString() : "";
  return `/admin/${route.area}${route.selected ? `/${encodeURIComponent(route.selected)}` : ""}${query ? `?${query}` : ""}`;
}
