// SPDX-License-Identifier: AGPL-3.0-only
import { useState, type FormEvent } from "react";
import { request, type Envelope, type Checks } from "./api";
import { Button } from "./components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "./components/ui/card";
import { ErrorMessage, Field, Loading, useLoad } from "./common";

interface MembershipAuth {
  checks: Checks;
}

export function MembershipAuthEditor({
  userID,
  tenant,
  onSaved,
}: {
  userID: string;
  tenant: string;
  onSaved: (revision: number) => void;
}) {
  const path = `/users/${userID}/memberships/${tenant}/auth`;
  const { data, error } = useLoad(
    () => request<Envelope<MembershipAuth>>(path),
    [path],
    true,
  );
  if (error) return <ErrorMessage error={error} />;
  if (!data) return <Loading />;
  return (
    <MembershipAuthForm
      key={`${path}:${data.revision}`}
      path={path}
      tenant={tenant}
      policy={data}
      onSaved={onSaved}
    />
  );
}

function MembershipAuthForm({
  path,
  tenant,
  policy,
  onSaved,
}: {
  path: string;
  tenant: string;
  policy: Envelope<MembershipAuth>;
  onSaved: (revision: number) => void;
}) {
  const [draft, setDraft] = useState(
    JSON.stringify(policy.data.checks, null, 2),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const checks: unknown = JSON.parse(draft);
      if (!checks || typeof checks !== "object" || Array.isArray(checks))
        throw new Error(
          "Enter a provider-to-claims object, or {} to inherit tenant rules.",
        );
      const result = await request<Envelope<unknown>>(
        path,
        "PUT",
        { checks },
        policy.revision,
      );
      onSaved(result.revision);
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Sign-in checks for {tenant}</CardTitle>
        <CardDescription>
          Override selected claims for this user in this tenant. Other tenant
          checks and permitted providers still apply. Only an administrator can
          set these overrides. They are removed with the membership and never
          restored by auto-add.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form className="form" onSubmit={save}>
          <Field
            label="Membership claim overrides"
            hint={
              'Map provider names to claim checks, for example {"azure":{"tid":["organization-id"]}}. Use {} to inherit all tenant rules.'
            }
          >
            <textarea
              rows={8}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              spellCheck={false}
            />
          </Field>
          <ErrorMessage error={error} />
          <Button loading={busy}>Save membership checks</Button>
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={() => setDraft("{}")}
          >
            Use tenant defaults
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
