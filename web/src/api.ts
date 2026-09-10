// SPDX-License-Identifier: AGPL-3.0-only
export interface Envelope<T> {
  revision: number;
  data: T;
}
export interface Tenant {
  id: string;
  slug: string;
  name: string;
  enabled: boolean;
  metadata: { logo?: string; default_workspace?: string };
}
export interface User {
  id: string;
  email: string;
  name: string;
  enabled: boolean;
  default_tenant_slug: string;
  memberships: string[];
}
export type Checks = Record<string, Record<string, string | string[]> | null>;
export interface Policy {
  auto_add: boolean;
  providers: string[];
  checks: Checks;
}
export interface Status {
  schema_version: number;
  propagation_bound_ms: number;
  admission: string;
  providers: {
    name: string;
    configured: boolean;
    supported_claim_examples: string[];
  }[];
}
export interface Session {
  operator: string;
  expires_at: string;
  csrf_token: string;
}
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
let csrf = "";
export async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
  revision?: number,
): Promise<T> {
  const response = await fetch(`/api/auth-admin/v1${path}`, {
    method,
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(csrf ? { "X-CSRF-Token": csrf } : {}),
      ...(revision !== undefined ? { "If-Match": `"${revision}"` } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const data = await response
    .json()
    .catch(() => ({ error: "Unexpected server response" }));
  if (!response.ok) {
    if (response.status === 401 && path !== "/session")
      window.dispatchEvent(new Event("auth-expired"));
    throw new ApiError(
      response.status,
      data.code ?? "request_failed",
      data.error ?? "Request failed",
    );
  }
  return data as T;
}
export async function session(): Promise<Session> {
  const s = await request<Session>("/session/me");
  csrf = s.csrf_token;
  return s;
}
export async function login(operator: string, token: string): Promise<Session> {
  const s = await request<Session>("/session", "POST", { operator, token });
  csrf = s.csrf_token;
  return s;
}
export async function logout() {
  await request("/session", "DELETE");
  csrf = "";
}
