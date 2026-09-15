// SPDX-License-Identifier: AGPL-3.0-only
import { useState, type FormEvent } from "react";
import { Plus, Trash2 } from "lucide-react";
import {
  request,
  type Envelope,
  type Policy,
  type Status,
  type Tenant,
  type Checks,
} from "./api";
import { Button } from "./components/ui/button";
import { Input, Textarea } from "./components/ui/input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "./components/ui/card";
import { ConfirmButton, ErrorMessage, Field, Loading, useLoad } from "./common";
import { overlayClaims, readGoogleHostedDomains } from "./policy";

export function TenantEditor({
  slug,
  reload,
  onSaved,
  status,
}: {
  slug: string;
  reload: number;
  onSaved: (revision: number) => void;
  status: Status;
}) {
  const { data, error } = useLoad(
    async () => {
      const [tenant, domains, policy] = await Promise.all([
        request<Envelope<Tenant>>(`/tenants/${slug}`),
        request<Envelope<string[]>>(`/tenants/${slug}/domains`),
        request<Envelope<Policy>>(`/tenants/${slug}/auth`),
      ]);
      return { tenant, domains, policy };
    },
    [slug, reload],
    true,
  );
  if (error) return <ErrorMessage error={error} />;
  if (!data) return <Loading />;
  return (
    <div className="editor-grid">
      <Profile tenant={data.tenant} onSaved={onSaved} />
      <Domains slug={slug} domains={data.domains} onSaved={onSaved} />
      <PolicyForm
        slug={slug}
        policy={data.policy}
        status={status}
        onSaved={onSaved}
      />
    </div>
  );
}
function Profile({
  tenant,
  onSaved,
}: {
  tenant: Envelope<Tenant>;
  onSaved: (revision: number) => void;
}) {
  const [name, setName] = useState(tenant.data.name),
    [enabled, setEnabled] = useState(tenant.data.enabled);
  const [logo, setLogo] = useState(tenant.data.metadata.logo ?? ""),
    [workspace, setWorkspace] = useState(
      tenant.data.metadata.default_workspace ?? "",
    );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const saved = await request<Envelope<unknown>>(
        `/tenants/${tenant.data.slug}`,
        "PUT",
        { name, enabled, metadata: { logo, default_workspace: workspace } },
        tenant.revision,
      );
      onSaved(saved.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Tenant profile</CardTitle>
        <CardDescription>
          Auth registration and presentation settings.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} className="form">
          <Field label="Tenant name">
            <Input
              required
              maxLength={200}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <div className="field">
            <span>Tenant slug</span>
            <code>{tenant.data.slug}</code>
          </div>
          <Field
            label="Logo URL"
            hint={`Optional HTTPS image URL for applications and the public login page at /login?tenant=${tenant.data.slug}.`}
          >
            <Input
              type="url"
              placeholder="https://example.com/logo.svg"
              value={logo}
              onChange={(e) => setLogo(e.target.value)}
            />
          </Field>
          <Field
            label="Default workspace"
            hint="Optional workspace identifier passed to applications."
          >
            <Input
              value={workspace}
              onChange={(e) => setWorkspace(e.target.value)}
            />
          </Field>
          <label className="check">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            Tenant enabled
          </label>
          {!enabled && (
            <p className="notice">
              Disabling this tenant blocks user access and clears stored user
              defaults for it.
            </p>
          )}
          <ErrorMessage error={error} />
          <Button loading={busy}>Save profile</Button>
        </form>
      </CardContent>
    </Card>
  );
}
function Domains({
  slug,
  domains,
  onSaved,
}: {
  slug: string;
  domains: Envelope<string[]>;
  onSaved: (revision: number) => void;
}) {
  const [value, setValue] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  async function add(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await request<Envelope<unknown>>(
        `/tenants/${slug}/domains`,
        "POST",
        { domain: value },
        domains.revision,
      );
      onSaved(result.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Email domains</CardTitle>
        <CardDescription>A domain can belong to one tenant.</CardDescription>
      </CardHeader>
      <CardContent className="form">
        <p className="notice">
          Associated domains identify eligible users when auto-add is enabled in
          the sign-in policy. With auto-add off, existing membership is
          required.
        </p>
        {domains.data.length === 0 ? (
          <p className="muted">
            No domains associated. Only registered users with a membership can
            access this tenant.
          </p>
        ) : (
          <ul className="rows">
            {domains.data.map((d) => (
              <li key={d}>
                <code>{d}</code>
                <ConfirmButton
                  label={`Remove ${d}?`}
                  onConfirm={async () => {
                    const saved = await request<Envelope<unknown>>(
                      `/tenants/${slug}/domains/${encodeURIComponent(d)}`,
                      "DELETE",
                      undefined,
                      domains.revision,
                    );
                    onSaved(saved.revision);
                  }}
                >
                  <Trash2 size={14} />
                  <span className="sr-only">Remove {d}</span>
                </ConfirmButton>
              </li>
            ))}
          </ul>
        )}
        <form onSubmit={add} className="form">
          <Field label="New domain">
            <Input
              required
              value={value}
              placeholder="example.com"
              onChange={(e) => setValue(e.target.value)}
            />
          </Field>
          <ErrorMessage error={error} />
          <Button variant="outline" loading={busy}>
            <Plus />
            Add domain
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
function PolicyForm({
  slug,
  policy,
  status,
  onSaved,
}: {
  slug: string;
  policy: Envelope<Policy>;
  status: Status;
  onSaved: (revision: number) => void;
}) {
  const [providers, setProviders] = useState(policy.data.providers),
    [autoAdd, setAutoAdd] = useState(policy.data.auto_add),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  const tid = policy.data.checks.azure?.tid;
  const [azure, setAzure] = useState(
    Array.isArray(tid) ? tid.join("\n") : (tid ?? ""),
  );
  const [draftChecks, setDraftChecks] = useState(policy.data.checks);
  const [google, setGoogle] = useState(() =>
    readGoogleHostedDomains(policy.data.checks.google?.hd),
  );
  const [advanced, setAdvanced] = useState(false),
    [raw, setRaw] = useState(JSON.stringify(policy.data.checks, null, 2));
  const names = [
    ...new Set([
      ...status.providers.map((p) => p.name),
      ...policy.data.providers,
      ...Object.keys(policy.data.checks),
    ]),
  ];
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const checks: Checks = advanced
        ? JSON.parse(raw)
        : overlayClaims(draftChecks, azure, google);
      if (
        typeof checks !== "object" ||
        checks === null ||
        Array.isArray(checks)
      )
        throw new Error("Checks must be a JSON object keyed by provider.");
      // Removed provider keys in the advanced editor explicitly clear their config.
      for (const p of Object.keys(policy.data.checks)) {
        if (!(p in checks)) checks[p] = null;
      }
      const result = await request<Envelope<unknown>>(
        `/tenants/${slug}/auth`,
        "PUT",
        { auto_add: autoAdd, providers, checks },
        policy.revision,
      );
      onSaved(result.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card className="policy-card">
      <CardHeader>
        <CardTitle>Sign-in policy</CardTitle>
        <CardDescription>
          Provider allowlist and required claims for this tenant.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} className="form">
          <div className="posture">
            <span className="eyebrow">Saved provider policy</span>
            <strong>
              {policy.data.providers.length
                ? `Only ${policy.data.providers.join(", ")}`
                : "Any configured login provider"}
            </strong>
            <p>
              All configured claim checks must match. An empty allowlist permits
              any configured provider.
            </p>
          </div>
          <label className="check">
            <input
              type="checkbox"
              checked={autoAdd}
              onChange={(e) => setAutoAdd(e.target.checked)}
            />
            Automatically add eligible users to this tenant
          </label>
          <p className="notice">
            {autoAdd
              ? "Eligible users from an associated email domain are added on their first access check. Google requires a verified email and an allowed nonempty hosted domain; Entra requires an allowed organization tenant ID. Existing users keep their other memberships. Accounts without a Google hosted domain must be added manually."
              : "Auto-add is off. Only existing members can access this tenant, even when their email domain is associated."}
          </p>
          <fieldset>
            <legend>Permitted providers</legend>
            <div className="provider-options">
              {names.length ? (
                names.map((name) => {
                  const configured = status.providers.find(
                    (p) => p.name === name,
                  )?.configured;
                  return (
                    <label className="check provider" key={name}>
                      <input
                        type="checkbox"
                        checked={providers.includes(name)}
                        onChange={() =>
                          setProviders((p) =>
                            p.includes(name)
                              ? p.filter((n) => n !== name)
                              : [...p, name],
                          )
                        }
                      />
                      <span>
                        {name}
                        <small>
                          {configured
                            ? "Configured"
                            : "Not configured globally"}
                        </small>
                      </span>
                    </label>
                  );
                })
              ) : (
                <p className="muted">
                  No global login providers configured. Configure an OIDC
                  provider and restart the service; its name will appear here.
                </p>
              )}
            </div>
          </fieldset>
          <p className="muted">
            {providers.length
              ? `Only ${providers.join(", ")} will be allowed.`
              : "Zero selected: any configured provider is allowed."}
          </p>
          <label className="check">
            <input
              type="checkbox"
              checked={advanced}
              onChange={(e) => {
                try {
                  if (e.target.checked) {
                    setRaw(
                      JSON.stringify(
                        overlayClaims(draftChecks, azure, google),
                        null,
                        2,
                      ),
                    );
                  } else {
                    const parsed: Checks = JSON.parse(raw);
                    if (
                      typeof parsed !== "object" ||
                      parsed === null ||
                      Array.isArray(parsed)
                    )
                      throw new Error(
                        "Checks must be a JSON object keyed by provider.",
                      );
                    const nextGoogle = readGoogleHostedDomains(
                      parsed.google?.hd,
                    );
                    const nextAzure = parsed.azure?.tid;
                    if (
                      nextAzure !== undefined &&
                      typeof nextAzure !== "string" &&
                      (!Array.isArray(nextAzure) ||
                        !nextAzure.every((v) => typeof v === "string"))
                    )
                      throw new Error(
                        "Azure tid must be a string or list of strings.",
                      );
                    setDraftChecks(parsed);
                    setGoogle(nextGoogle);
                    setAzure(
                      Array.isArray(nextAzure)
                        ? nextAzure.join("\n")
                        : (nextAzure ?? ""),
                    );
                  }
                  setError(null);
                  setAdvanced(e.target.checked);
                } catch (e) {
                  setError(e);
                }
              }}
            />
            Edit all claim checks as JSON
          </label>
          {advanced ? (
            <Field
              label="Provider claim checks"
              hint='String or string-list values. Google hd may include "" for registered members without a hosted domain. Null or {} clears a provider.'
            >
              <Textarea
                className="code-editor"
                value={raw}
                onChange={(e) => setRaw(e.target.value)}
              />
            </Field>
          ) : (
            <div className="two-column">
              <Field
                label="Azure tenant IDs (tid)"
                hint="One tenant ID per line. Blank clears only this claim."
              >
                <Textarea
                  value={azure}
                  onChange={(e) => setAzure(e.target.value)}
                  placeholder="11111111-1111-4111-8111-111111111111"
                />
              </Field>
              <div className="form">
                <label className="check">
                  <input
                    type="checkbox"
                    checked={google.restricted}
                    onChange={(e) =>
                      setGoogle({ ...google, restricted: e.target.checked })
                    }
                  />
                  Restrict Google hosted domains
                </label>
                <Field
                  label="Google hosted domains (hd)"
                  hint="One authoritative domain per line. Blank lines are ignored; use the option below for accounts without a hosted domain."
                >
                  <Textarea
                    value={google.domains}
                    disabled={!google.restricted}
                    onChange={(e) =>
                      setGoogle({ ...google, domains: e.target.value })
                    }
                    placeholder={"example.com\nexample.org"}
                  />
                </Field>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={google.allowUnhosted}
                    disabled={!google.restricted}
                    onChange={(e) =>
                      setGoogle({ ...google, allowUnhosted: e.target.checked })
                    }
                  />
                  Allow accounts without a hosted domain (registered members
                  only)
                </label>
                <p className="notice">
                  {!google.restricted
                    ? "No hosted-domain restriction. Other provider and admission checks still apply."
                    : !google.domains.trim() && !google.allowUnhosted
                      ? "No options selected: all Google accounts are blocked for this tenant."
                      : "Accounts without a hosted domain require an enabled user and tenant membership. This option never permits domain-based admission or auto-add."}
                </p>
              </div>
            </div>
          )}
          <ErrorMessage error={error} />
          <div className="actions">
            <span className="muted">
              Changes apply to new checks within 1 second.
            </span>
            <Button loading={busy}>Save sign-in policy</Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
