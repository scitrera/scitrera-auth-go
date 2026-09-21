# Admission and automatic enrollment

Access requires an enabled user and an enabled tenant membership, followed by
that tenant's provider and claim checks. **Auto-add defaults off for every tenant,
including existing tenants.** An email-domain association alone grants no access.
This replaces the original domain-only admission path,
which allowed access without creating user or membership records.

## Tenant auto-add

In tenant configuration, enable **Automatically add eligible users to this tenant**
and save the sign-in policy. The API exposes `auto_add: true|false`; storage is a
MessagePack boolean in `public.tenant_config` under `auth:auto_add`. Missing, null
or malformed stored values are off. No schema migration is needed. This setting
is independent of the platform's Aether KV `auth:auto_add_valid` flag.

Enrollment requires all of the following:

1. The authenticated account's email domain is associated with this enabled tenant.
2. The tenant explicitly enables auto-add.
3. The provider is allowed and all configured claim checks pass.
4. The provider supplies organization authority: for `google`, a boolean
   `email_verified: true` and a nonempty `hd` matching an explicitly configured
   Google hosted-domain rule; for `azure`, `entra` or `microsoft`, an organization
   `tid` UUID matching an explicitly configured tenant-ID rule. The personal
   Microsoft account tenant is excluded. Other provider names currently support
   explicit memberships but not automatic enrollment.
5. If the trusted gateway requests a tenant, it matches the associated tenant.

The existing OIDC layer verifies the token before claims reach the resolver.
For Google's distinction between Workspace authority and unhosted accounts, see
[Google's verification guidance](https://developers.google.com/identity/gsi/web/guides/verify-google-id-token).
Entra organization checks use the tenant ID described in
[Microsoft's claim reference](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference).
The MT registry retains its existing normalized-email identity contract; enrollment
does not introduce a new provider-subject linking scheme. Configure only trusted
organization providers and the intended tenant-ID/hosted-domain allowlists.

Enrollment runs on the **first authenticated access verification** that needs the
membership. The public OAuth callback establishes the browser session; callback
or `/checkz` alone does not create MT records. An untargeted verification can
consider the email-domain tenant; a tenant-targeted check cannot enroll a user
into an unrelated tenant as a side effect.

A new user is created enabled with the enrolled tenant as their default. An
existing enabled user can acquire the eligible membership; their name, default,
other memberships and metadata are preserved. Disabled users are never re-enabled.
The same transaction creates the membership, advances the shared revision, and
records a `system:auto-add` audit event. Enrolled users appear in the admin Users
list and tenant filter. No platform services, licenses, or MemoryLayer users are
provisioned.

The transaction takes the same revision lock as operator/platform writes and
rereads the domain, enabled state, user and policy before insertion. Concurrent
sign-ins create one user/membership and one audit event. Any failure rolls back
all enrollment writes. A committed disable or changed rule cannot be bypassed by
a cached earlier auto-add decision. Other instances see memberships through the
normal one-second revision reconciliation; the enrolling instance invalidates
its user cache immediately.

Turning auto-add **off** stops new enrollments; it does not remove existing members.
While auto-add remains on, removing an eligible user's membership allows them to
rejoin at the next access check. To stop that, disable auto-add before removing
the membership, change eligibility, or disable the global user. There is no
separate per-tenant deny list in this release.

## Google Workspace and selected Gmail accounts

Google `auth:checks:google.hd` accepts a scalar or list, for example
`["example.com", "second.example.org", ""]`. Nonempty entries are authoritative
hosted domains. They do not automatically create email-domain associations;
associate each eligible email domain with the tenant separately.

An explicit `""` option permits a missing/empty `hd` only for an **existing enabled
member**. This supports selected personal Gmail users alongside Workspace users.
It never permits auto-add, even if `gmail.com` is associated or the `hd` rule is
removed. Null/non-string claims and wrong nonempty domains do not match the blank
option. All other claim checks remain mandatory.

Leave `gmail.com` unassociated and manually create each selected user/membership.
The Google client must permit those accounts. In the dashboard, enable **Restrict
Google hosted domains**, enter one domain per line, then optionally select
**Allow accounts without a hosted domain (registered members only)**. Blank
textarea lines are ignored. `hd: [""]` allows only registered unhosted members;
`hd: []` denies Google access entirely. Removing the `hd` key removes the member
restriction but does not provide the organization rule required for auto-add.

## Preserved member and machine behavior

Existing enabled memberships are filtered by all provider/claim checks and the
requested-tenant gate. Disabled users/tenants are rejected. A permitted requested
tenant pins the downstream identity. Empty provider allowlists are unrestricted
among configured providers. Legacy missing-provider and malformed-allowlist
behavior is preserved for existing memberships; auto-add rejects absent provider
names and malformed policies. Member access does not gain an additional automatic
`email_verified` requirement.

Membership removal clears an invalid stored default without deleting the user or
other memberships. Disabling a tenant clears defaults pointing to it. In the
resolved identity, a default excluded by policy falls back to the first permitted
tenant in slug order.

Machine methods bypass the user registry and retain Aether credential/ACL behavior;
they do not enroll users or gain operator access. Incoming Scitrera user headers
are explicitly cleared for machine requests to prevent spoofed extras surviving
reverse-proxy forwarding. Session/access-log dashboards and global OAuth client
editing remain deferred.

## Selected external members

An administrator can replace selected claim requirements on an existing
user–tenant membership, for example accepting a support user's organization ID
without changing tenant-wide enrollment rules. See the [membership
API](api.md#membership-claim-overrides). The provider allowlist and every
non-overridden check remain mandatory. Auto-add never reads, writes or restores
overrides, even after an earlier membership was removed.
