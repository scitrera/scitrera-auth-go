// SPDX-License-Identifier: AGPL-3.0-only
import { useState, useEffect, type FormEvent } from "react";
import { Trash2 } from "lucide-react";
import { request, type Envelope, type User } from "./api";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "./components/ui/card";
import { ConfirmButton, ErrorMessage, Field, Loading, useLoad } from "./common";
import { UserSessions } from "./UserSessions";

export function UserEditor({
  id,
  reload,
  onSaved,
  sessionOffset,
  onSessionOffset,
}: {
  id: string;
  reload: number;
  onSaved: (revision: number) => void;
  sessionOffset: number;
  onSessionOffset: (offset: number) => void;
}) {
  const { data, error } = useLoad(
    () => request<Envelope<User>>(`/users/${id}`),
    [id, reload],
    true,
  );
  if (error) return <ErrorMessage error={error} />;
  if (!data) return <Loading />;
  return (
    <>
      <UserForm user={data} onSaved={onSaved} />
      <UserSessions
        key={id}
        userID={id}
        offset={sessionOffset}
        onOffset={onSessionOffset}
        reload={reload}
      />
    </>
  );
}
function UserForm({
  user,
  onSaved,
}: {
  user: Envelope<User>;
  onSaved: (revision: number) => void;
}) {
  const [name, setName] = useState(user.data.name),
    [email, setEmail] = useState(user.data.email),
    [enabled, setEnabled] = useState(user.data.enabled),
    [defaultTenant, setDefaultTenant] = useState(user.data.default_tenant_slug),
    [membership, setMembership] = useState("");
  useEffect(() => {
    if (defaultTenant && !user.data.memberships.includes(defaultTenant))
      setDefaultTenant("");
  }, [user.data.memberships, defaultTenant]);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await request<Envelope<unknown>>(
        `/users/${user.data.id}`,
        "PUT",
        { name, email, enabled, default_tenant_slug: defaultTenant },
        user.revision,
      );
      onSaved(result.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function add(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await request<Envelope<unknown>>(
        `/users/${user.data.id}/memberships`,
        "POST",
        { tenant_slug: membership },
        user.revision,
      );
      onSaved(result.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="editor-grid">
      <Card>
        <CardHeader>
          <CardTitle>User profile</CardTitle>
          <CardDescription>
            Disabling a user blocks access across all tenants.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={save} className="form">
            <Field label="User name">
              <Input
                required
                maxLength={200}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field label="Email">
              <Input
                required
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </Field>
            <label className="check">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              User enabled
            </label>
            <Field
              label="Default tenant"
              hint="Only an enabled membership can be a default. Without a stored default, the first permitted tenant is used."
            >
              <select
                value={defaultTenant}
                onChange={(e) => setDefaultTenant(e.target.value)}
              >
                <option value="">No stored default</option>
                {user.data.memberships.map((slug) => (
                  <option key={slug} value={slug}>
                    {slug}
                  </option>
                ))}
              </select>
            </Field>
            <ErrorMessage error={error} />
            <Button loading={busy}>Save user</Button>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Tenant memberships</CardTitle>
          <CardDescription>
            Removing an association keeps the global user and other memberships.
            If tenant auto-add remains enabled, an eligible user can rejoin.
          </CardDescription>
        </CardHeader>
        <CardContent className="form">
          {user.data.memberships.length ? (
            <ul className="rows">
              {user.data.memberships.map((slug) => (
                <li key={slug}>
                  <code>{slug}</code>
                  <ConfirmButton
                    label={`Remove membership in ${slug}?`}
                    onConfirm={async () => {
                      const result = await request<Envelope<unknown>>(
                        `/users/${user.data.id}/memberships/${slug}`,
                        "DELETE",
                        undefined,
                        user.revision,
                      );
                      onSaved(result.revision);
                    }}
                  >
                    <Trash2 />
                    <span className="sr-only">Remove membership {slug}</span>
                  </ConfirmButton>
                </li>
              ))}
            </ul>
          ) : (
            <p className="muted">
              No memberships. Add one manually, or eligible organization
              accounts can join a tenant with auto-add enabled.
            </p>
          )}
          <form className="form" onSubmit={add}>
            <Field
              label="Tenant slug to add"
              hint="Use the slug of an existing auth tenant."
            >
              <Input
                required
                value={membership}
                onChange={(e) => setMembership(e.target.value)}
                placeholder="example"
              />
            </Field>
            <Button variant="outline" loading={busy}>
              Add membership
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
