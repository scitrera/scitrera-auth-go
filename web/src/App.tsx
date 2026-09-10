// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState, type FormEvent } from "react";
import {
  Building2,
  Users,
  Activity,
  ShieldCheck,
  LogOut,
  Plus,
  RefreshCw,
  ArrowLeft,
  ArrowRight,
} from "lucide-react";
import {
  ApiError,
  login,
  logout,
  request,
  session,
  type Envelope,
  type Session,
  type Status,
  type Tenant,
  type User,
} from "./api";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import {
  ErrorMessage,
  Field,
  Loading,
  Empty,
  Enabled,
  useLoad,
} from "./common";
import { TenantEditor } from "./TenantEditor";
import { UserEditor } from "./UserEditor";
import { defaultRoute, type Area, type AdminRoute } from "./routes";
import { useRoute } from "./useRoute";

export default function App() {
  const [operator, setOperator] = useState<Session | null>(),
    [error, setError] = useState<unknown>(null);
  useEffect(() => {
    session().then(setOperator, (e) => {
      setOperator(null);
      if (!(e instanceof ApiError && e.status === 401)) setError(e);
    });
    const expired = () => setOperator(null);
    window.addEventListener("auth-expired", expired);
    return () => window.removeEventListener("auth-expired", expired);
  }, []);
  if (operator === undefined) return <Loading />;
  if (!operator) return <Login onLogin={setOperator} initialError={error} />;
  return <Dashboard operator={operator} onLogout={() => setOperator(null)} />;
}
function Login({
  onLogin,
  initialError,
}: {
  onLogin: (s: Session) => void;
  initialError: unknown;
}) {
  const [name, setName] = useState("operator"),
    [token, setToken] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(initialError);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      onLogin(await login(name, token));
    } catch (e) {
      setError(e);
    } finally {
      setToken("");
      setBusy(false);
    }
  }
  return (
    <main className="login-page">
      <section className="login-card">
        <div className="brand-mark">
          <ShieldCheck size={28} />
        </div>
        <div className="eyebrow">Scitrera Auth</div>
        <h1>Operator sign-in</h1>
        <p>Manage tenant access, users, and sign-in policies.</p>
        <form onSubmit={submit} className="form">
          <Field label="Operator name">
            <Input
              autoComplete="username"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label="Operator token">
            <Input
              autoComplete="off"
              type="password"
              required
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          </Field>
          <ErrorMessage error={error} />
          <Button loading={busy}>Sign in</Button>
        </form>
        <details>
          <summary>First operator setup</summary>
          <p>On the server, create a private credentials file:</p>
          <code>
            scitrera-auth-proxy bootstrap --token-file operators.json --operator
            operator
          </code>
          <p>
            Set SCITRERA_AUTH_ADMIN_TOKEN_FILE to that file and start the
            service. Use its operator name and token here.
          </p>
        </details>
        <small>
          Dedicated operator credentials required. Tenant sign-in does not grant
          administration access.
        </small>
      </section>
    </main>
  );
}
function Dashboard({
  operator,
  onLogout,
}: {
  operator: Session;
  onLogout: () => void;
}) {
  const [route, changeRoute] = useRoute();
  const { area, selected, creating, offset } = route;
  const [editorReset, setEditorReset] = useState(0);
  const [reload, setReload] = useState(0),
    [toast, setToast] = useState(""),
    [error, setError] = useState<unknown>(null);
  const selectRecord = (id: string) =>
    changeRoute({ ...route, selected: id, creating: false });
  const setOffset = (offset: number) => changeRoute({ ...route, offset });
  const setCreating = (creating: boolean) =>
    changeRoute({ ...route, creating }, !creating);
  const status = useLoad(
    () => request<Envelope<Status>>("/status"),
    [reload],
    true,
  );
  const list = useLoad(
    () =>
      area === "status"
        ? Promise.resolve(undefined)
        : request<Envelope<(Tenant | User)[]>>(
            `/${area}?${new URLSearchParams({ limit: "50", offset: String(offset), q: route.q, enabled: route.enabled, ...(area === "users" ? { tenant: route.tenant } : {}) })}`,
          ),
    [area, reload, offset, route.q, route.enabled, route.tenant],
  );
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(""), 6000);
    return () => clearTimeout(timer);
  }, [toast]);
  const saved = (revision: number) => {
    setToast(
      `Saved revision ${revision}. New auth checks apply changes within 1 second.`,
    );
    setReload((n) => n + 1);
  };
  const navigate = (next: Area) => {
    changeRoute(defaultRoute(next));
    setError(null);
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="/admin/">
          <ShieldCheck />
          <span>
            Scitrera <strong>Auth</strong>
          </span>
        </a>
        <span className="eyebrow sidebar-label">Configuration</span>
        <nav aria-label="Main navigation">
          {(
            [
              { name: "tenants", label: "Tenants", icon: Building2 },
              { name: "users", label: "Users", icon: Users },
              { name: "status", label: "Service status", icon: Activity },
            ] as const
          ).map((item) => (
            <button
              key={item.name}
              className={area === item.name ? "active" : ""}
              onClick={() => navigate(item.name)}
            >
              <item.icon size={18} />
              {item.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-footer">
          <span className="status enabled">
            <span className="dot" />
            Operator session
          </span>
          <strong>{operator.operator}</strong>
          <a className="muted" href="/admin/source.tar.gz">
            Source code · AGPL-3.0
          </a>
          <small>
            Expires{" "}
            {new Date(operator.expires_at).toLocaleTimeString([], {
              hour: "2-digit",
              minute: "2-digit",
            })}
          </small>
          <Button
            variant="ghost"
            onClick={async () => {
              try {
                await logout();
                onLogout();
              } catch (e) {
                setError(e);
              }
            }}
          >
            <LogOut />
            Sign out
          </Button>
        </div>
      </aside>
      <main className="main">
        <header className="topbar">
          <span>
            Authentication /{" "}
            {area === "status"
              ? "Service status"
              : area === "tenants"
                ? "Tenants"
                : "Users"}
            {selected && ` / ${selected}`}
          </span>
          <span className="mono">
            {status.data ? `Revision ${status.data.revision}` : "Connecting…"}
          </span>
        </header>
        <div className="content">
          <div className="page-title">
            <div>
              <div className="eyebrow">Auth registry</div>
              <h1>
                {selected
                  ? area === "tenants"
                    ? selected
                    : "User configuration"
                  : area === "status"
                    ? "Service status"
                    : area === "tenants"
                      ? "Tenants"
                      : "Users"}
              </h1>
              <p>
                {area === "tenants"
                  ? "Register tenants and define how people sign in."
                  : area === "users"
                    ? "Manage auth identities and tenant memberships."
                    : "Configured providers and policy propagation."}
              </p>
            </div>
            <div className="actions">
              {selected && (
                <Button variant="outline" onClick={() => selectRecord("")}>
                  <ArrowLeft />
                  Back
                </Button>
              )}
              {selected && area === "tenants" && (
                <Button
                  variant="outline"
                  onClick={() =>
                    changeRoute({ ...defaultRoute("users"), tenant: selected })
                  }
                >
                  <Users /> View users
                </Button>
              )}
              <Button
                variant="outline"
                aria-label="Reload configuration"
                onClick={() => {
                  setReload((n) => n + 1);
                  setEditorReset((n) => n + 1);
                  setError(null);
                }}
              >
                <RefreshCw />
              </Button>
              {!selected && area !== "status" && (
                <Button onClick={() => setCreating(true)}>
                  <Plus />
                  Create {area === "tenants" ? "tenant" : "user"}
                </Button>
              )}
            </div>
          </div>
          <ErrorMessage error={error ?? status.error ?? list.error} />
          {!selected && area !== "status" && !route.notFound && (
            <ListFilters
              key={`${area}:${route.q}:${route.tenant}:${route.enabled}`}
              route={route}
              onApply={(filters) =>
                changeRoute({
                  ...route,
                  ...filters,
                  offset: 0,
                  creating: false,
                })
              }
            />
          )}
          {route.notFound ? (
            <Empty>
              <h2>Page not found</h2>
              <Button onClick={() => navigate("tenants")}>Open tenants</Button>
            </Empty>
          ) : area === "status" ? (
            status.data ? (
              <StatusView status={status.data} />
            ) : (
              <Loading />
            )
          ) : selected ? (
            status.data ? (
              area === "tenants" ? (
                <TenantEditor
                  key={`${selected}:${editorReset}`}
                  slug={selected}
                  reload={reload}
                  onSaved={saved}
                  status={status.data.data}
                />
              ) : (
                <UserEditor
                  key={`${selected}:${editorReset}`}
                  id={selected}
                  reload={reload}
                  onSaved={saved}
                />
              )
            ) : (
              <Loading />
            )
          ) : !list.data ? (
            <Loading />
          ) : list.data.data.length === 0 ? (
            <Empty>
              <Building2 size={32} />
              <h2>
                {route.q || route.tenant || route.enabled
                  ? "No matching records"
                  : offset
                    ? "No more records"
                    : `No ${area} yet`}
              </h2>
              <p>
                {area === "tenants"
                  ? "Create your first auth tenant to configure access."
                  : "Create a user, then add their tenant memberships."}
              </p>
              {offset > 0 && (
                <Button
                  variant="outline"
                  onClick={() => setOffset(Math.max(0, offset - 50))}
                >
                  Previous page
                </Button>
              )}
            </Empty>
          ) : (
            <>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{area === "tenants" ? "Tenant" : "User"}</th>
                      <th>{area === "tenants" ? "Slug" : "Email"}</th>
                      <th>Status</th>
                      <th>
                        <span className="sr-only">Actions</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {list.data.data.map((row) => (
                      <tr key={row.id}>
                        <td>
                          <strong>{row.name}</strong>
                        </td>
                        <td className="mono">
                          {"slug" in row ? row.slug : row.email}
                        </td>
                        <td>
                          <Enabled value={row.enabled} />
                        </td>
                        <td>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() =>
                              selectRecord("slug" in row ? row.slug : row.id)
                            }
                          >
                            Configure <ArrowRight />
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="pagination">
                <span>
                  {offset + 1}–{offset + list.data.data.length}
                </span>
                <Button
                  variant="outline"
                  disabled={offset === 0}
                  onClick={() => setOffset(Math.max(0, offset - 50))}
                >
                  Previous
                </Button>
                <Button
                  variant="outline"
                  disabled={list.data.data.length < 50}
                  onClick={() => setOffset(offset + 50)}
                >
                  Next
                </Button>
              </div>
            </>
          )}
        </div>
      </main>
      {toast && (
        <div className="toast" role="status">
          <ShieldCheck size={18} />
          {toast}
        </div>
      )}
      {creating && list.data && (
        <CreateForm
          area={area === "tenants" ? "tenants" : "users"}
          revision={list.data.revision}
          onCancel={() => setCreating(false)}
          onSaved={(r) => {
            changeRoute({ ...route, creating: false, offset: 0 }, true);
            saved(r);
          }}
        />
      )}
    </div>
  );
}

function ListFilters({
  route,
  onApply,
}: {
  route: AdminRoute;
  onApply: (filters: Pick<AdminRoute, "q" | "tenant" | "enabled">) => void;
}) {
  const [q, setQuery] = useState(route.q),
    [tenant, setTenant] = useState(route.tenant),
    [enabled, setEnabled] = useState(route.enabled);
  return (
    <form
      className="filter-bar"
      aria-label="List filters"
      onSubmit={(e) => {
        e.preventDefault();
        onApply({ q: q.trim(), tenant: tenant.trim().toLowerCase(), enabled });
      }}
    >
      <Field label={route.area === "users" ? "Search users" : "Search tenants"}>
        <Input
          value={q}
          maxLength={200}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={
            route.area === "users" ? "Name or email" : "Name or slug"
          }
        />
      </Field>
      {route.area === "users" && (
        <Field
          label="Tenant filter"
          hint="Filter by membership in this tenant slug."
        >
          <Input
            value={tenant}
            onChange={(e) => setTenant(e.target.value)}
            placeholder="All tenants"
          />
        </Field>
      )}
      <Field label="Enabled filter">
        <select
          value={enabled}
          onChange={(e) => setEnabled(e.target.value as AdminRoute["enabled"])}
        >
          <option value="">All statuses</option>
          <option value="true">Enabled</option>
          <option value="false">Disabled</option>
        </select>
      </Field>
      <Button variant="outline">Apply filters</Button>
      {(route.q || route.tenant || route.enabled) && (
        <Button
          type="button"
          variant="ghost"
          onClick={() => onApply({ q: "", tenant: "", enabled: "" })}
        >
          Clear filters
        </Button>
      )}
    </form>
  );
}

function CreateForm({
  area,
  revision,
  onCancel,
  onSaved,
}: {
  area: "tenants" | "users";
  revision: number;
  onCancel: () => void;
  onSaved: (r: number) => void;
}) {
  const [name, setName] = useState(""),
    [key, setKey] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await request<Envelope<unknown>>(
        `/${area}`,
        "POST",
        {
          name,
          enabled: true,
          ...(area === "tenants" ? { slug: key } : { email: key }),
        },
        revision,
      );
      onSaved(result.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="modal-backdrop">
      <section
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-label={`Create ${area === "tenants" ? "tenant" : "user"}`}
      >
        <h2>Create {area === "tenants" ? "tenant" : "user"}</h2>
        <p>
          {area === "tenants"
            ? "Registers authentication settings. Application services are provisioned separately."
            : "Create an auth identity, then add memberships from its configuration page."}
        </p>
        <form className="form" onSubmit={submit}>
          <Field label="Name">
            <Input
              autoFocus
              required
              maxLength={200}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label={area === "tenants" ? "Slug" : "Email"}>
            <Input
              required
              type={area === "users" ? "email" : "text"}
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder={
                area === "tenants" ? "example" : "person@example.com"
              }
            />
          </Field>
          <ErrorMessage error={error} />
          <div className="actions">
            <Button
              type="button"
              variant="outline"
              onClick={onCancel}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button loading={busy}>Create</Button>
          </div>
        </form>
      </section>
    </div>
  );
}
function StatusView({ status }: { status: Envelope<Status> }) {
  return (
    <div className="form">
      <div className="metrics">
        <div>
          <span>Saved revision</span>
          <strong>{status.revision}</strong>
        </div>
        <div>
          <span>Propagation bound</span>
          <strong>{status.data.propagation_bound_ms / 1000}s</strong>
        </div>
        <div>
          <span>Schema version</span>
          <strong>{status.data.schema_version}</strong>
        </div>
      </div>
      <section className="posture">
        <h2>Admission policy</h2>
        <p>
          Access requires an enabled user and permitted tenant membership.
          Explicit tenant auto-add can create those records for eligible
          organization accounts. Auto-add defaults off.
        </p>
        <p>
          Each instance checks the database revision at least every second when
          requests arrive. If it cannot confirm freshness, new user checks fail
          closed. Requests already in progress may finish under their earlier
          settings.
        </p>
      </section>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Login provider</th>
              <th>Configuration</th>
              <th>Example claims</th>
            </tr>
          </thead>
          <tbody>
            {status.data.providers.map((p) => (
              <tr key={p.name}>
                <td>{p.name}</td>
                <td>{p.configured ? "Configured" : "Incomplete"}</td>
                <td>{p.supported_claim_examples.join(", ")}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!status.data.providers.length && (
          <Empty>No global OIDC login providers configured.</Empty>
        )}
      </div>
      <p className="muted">
        OAuth clients and secrets are configured by the operator environment.
        Provider status describes startup configuration; it is not a live
        identity-provider health check.
      </p>
    </div>
  );
}
