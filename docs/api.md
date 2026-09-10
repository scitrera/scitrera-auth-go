# Operator API v1

Base path: `/api/auth-admin/v1/` on the private admin listener. JSON only, 64KiB
maximum request body; unknown request fields are rejected. Mutations use
parameterized SQL and transactions. Errors have `{code, error}` and a suitable
HTTP status (400 validation, 401 session, 403 origin/CSRF, 404 missing,
409 duplicate/conflict, 413 size, 415 content type, 428 missing revision,
429 sign-in rate limit, 500 storage failure). Database error details are not
returned to the browser.

## Sessions

- `POST /session`: `{operator, token}`; returns `{operator, expires_at, csrf_token}`
  and a dedicated HttpOnly cookie. Requires the exact configured Origin even
  before login. A per-instance login limiter permits a burst of 10 then 1/second.
- `GET /session/me`: same session metadata and stable CSRF token after reload.
- `DELETE /session`: deletes the server session and clears its cookie.

Every subsequent API call needs the session cookie. Writes also need
`Origin: <configured-origin>` and `X-CSRF-Token: <session csrf_token>`. The token
stays in frontend memory. Operator tokens are never accepted as query strings or
ordinary bearer credentials. GET requests with a foreign Origin are also rejected.

## Configuration resources

Reads return `{revision: integer, data: ...}` and `ETag: "revision"`. List endpoints
accept `limit` (1–200, default 100) and `offset`; results are ordered by slug/email.
List endpoints also accept `q` (case-insensitive literal substring of name and
email/slug) and `enabled=true|false`. `/users` additionally accepts `tenant=<slug>`
to filter by membership, including memberships in disabled tenants. Filtering
happens before pagination, does not duplicate users with multiple memberships,
and does not hide their other memberships. Missing tenants return an empty list;
invalid filter values return 400.

Use the received quoted revision in `If-Match` on **every** mutation, including
creates. A successful write returns `{revision, data: {saved: true}}` only after
commit. Read the resource again to see its stored representation.

| Endpoint | Methods | Body / returned data |
|---|---|---|
| `/status` | GET | Schema, revision, propagation bound, admission mode, configured provider names/examples |
| `/tenants` | GET, POST | Create `{slug, name, enabled, metadata?}`; list tenant records |
| `/tenants/{slug}` | GET, PUT | Edit `{name, enabled, metadata?}`; slug immutable |
| `/tenants/{slug}/domains` | GET, POST | String list; add `{domain}` |
| `/tenants/{slug}/domains/{domain}` | DELETE | Remove association |
| `/tenants/{slug}/auth` | GET, PUT | `{auto_add: boolean, providers: string[], checks: {provider: check-map}}` |
| `/users` | GET, POST | Create `{email, name, enabled}`; list users with memberships/default |
| `/users/{id}` | GET, PUT | Edit `{email, name, enabled, default_tenant_slug?}` |
| `/users/{id}/memberships` | GET, POST | String list; add `{tenant_slug}` |
| `/users/{id}/memberships/{slug}` | DELETE | Remove association; clear matching stored default |

Metadata accepts only `logo` (HTTPS URL/string) and `default_workspace` (string).
Omitted keys are unchanged; null deletes the named key; unrelated stored metadata
survives. A null or omitted default tenant is unchanged; `""` clears it. A nonempty
default must identify an enabled membership. New users start without a default.

Domains are trimmed, lowercased, IDNA-normalized, and stripped of one trailing
dot. Email normalization lowercases the address and IDNA-normalizes its domain.
Uniqueness is global. A conflict rolls back the whole operation; no tenant or
user is deleted when an association is removed. Hard tenant/user deletion is not
offered in v1.

## Provider policy edits

`auto_add` is an explicit boolean. GET defaults to false when unset; PUT omitted
or null leaves it unchanged, true enables enrollment and false disables it.
Strings/numbers are rejected. The flag uses MessagePack key `auth:auto_add` and
commits atomically with provider/check edits. Existing domains do not implicitly
enable auto-add. See [admission](admission.md) for supported organization claims,
first-access enrollment, existing-user preservation and disabled-user behavior.

```json
{"auto_add":true,"providers":["google"],"checks":{"google":{"hd":["example.com",""]}}}
```

This example permits eligible Workspace accounts to enroll from associated email
domains; the blank option remains limited to existing registered members.


`providers` omitted/null means unchanged; `[]` stores an unrestricted allowlist.
Configured names are shown by `/status`, while existing unconfigured names remain
editable to avoid dropping valid policy during provider maintenance.

`checks` is a map keyed by provider. Omitted provider entries are unchanged.
For a provided provider, the map replaces that provider's checks; preserve unknown
valid fields when constructing it. Null or `{}` removes the entire provider key.
Each claim value must be a nonempty string or a list of strings, except for the
explicit blank Google `hd` option described below. All claims are
ANDed. An empty claim list denies everyone for that provider. String comparisons
are exact and case-sensitive. The UI structured fields preserve other checks;
clearing the Azure field or disabling the Google hosted-domain restriction
removes just that claim.

Google `hd` accepts one domain or a list of authoritative domains. Domain options
are trimmed, lowercased, IDNA-normalized and stripped of one trailing dot.
An exact empty string (`""`) is a special option for a Google account with no
`hd` claim (or an empty string claim), requiring an enabled registered user and
an enabled membership in the candidate tenant. It never permits domain-only
admission or auto-add, never matches a different nonempty `hd`, and never bypasses
other claim checks. Null/non-string claims are rejected. No migration is needed;
the options use the existing MessagePack string/list representation.

```json
{"checks":{"google":{"hd":["example.com","second.example.org",""]}}}
```

`hd: [""]` permits only registered members without a hosted domain; `hd: []`
denies all Google accounts. A legacy scalar domain remains supported, and
`hd: ""` is equivalent to `[""]`. Removing the `hd` key removes the restriction
entirely. The dashboard represents the blank option with an explicit checkbox;
blank textarea lines are ignored. Other providers/claims still reject empty
string options.

The auto-add flag, allowlist and check keys in a write commit together. Failed storage operations
roll back every affected key, the revision increments, and the audit event.
Non-auth tenant config is never returned or modified by this API.

## Concurrency and audit

The revision is global to the MT auth registry. This intentionally produces
conservative conflicts across tenants/resources. A 409 requires reloading and
reapplying edits, not blindly retrying with a newer revision. Legacy platform
writes also advance the revision through database triggers.

Audit rows record named operator, HTTP action, resource path, field categories,
committed revision and time. They do not record before/after payloads, credentials,
raw tokens, complete claims, or config values. Audit viewing is currently a
database/operations concern, not a dashboard feature. Automatic enrollment uses
operator `system:auto-add`, action `auto_add`, and a user/membership resource path;
it records field categories and revision without raw claims or credentials.

## Dashboard URLs

The browser uses `/admin/tenants`, `/admin/tenants/{slug}`, `/admin/users`,
`/admin/users/{uuid}` and `/admin/status`. Filters use `q`, `tenant`, `enabled`
and `offset` query parameters; opening a create form adds `action=create`.
For example, `/admin/users?tenant=alpha&enabled=true` lists enabled members of
alpha, and `/admin/users/{uuid}?tenant=alpha` retains that filter when returning
to the list. Tenant configuration has a View users action that preselects its
membership filter. URLs and browser history state contain navigation/filter
state only, never operator credentials or unsaved configuration values.
