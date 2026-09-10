-- SPDX-License-Identifier: Apache-2.0
-- Copyright Scitrera LLC. Adapted from Aether server v0.2.3 migrations.
-- Selected table definitions; no fleet/task tables or upstream permissive seeds.
CREATE SCHEMA IF NOT EXISTS auth_proxy;
SET LOCAL search_path TO auth_proxy, pg_catalog;

-- Upstream 003_acl_schema.sql
CREATE TABLE IF NOT EXISTS acl_rules
(
    rule_id        UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    principal_type VARCHAR(50)  NOT NULL, -- 'agent', 'task', 'user', 'wildcard'
    principal_id   VARCHAR(255) NOT NULL, -- Identity ID or wildcard pattern
    resource_type  VARCHAR(50)  NOT NULL, -- 'workspace', 'agent', 'permission'
    resource_id    VARCHAR(255) NOT NULL, -- Resource identifier
    access_level   INTEGER      NOT NULL, -- 0=NONE, 10=READ, 20=READWRITE, 30=MANAGE, 40=ADMIN, 50=SUPERADMIN
    granted_by     VARCHAR(255),          -- Who granted this rule
    granted_at     TIMESTAMP    NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMP,             -- Optional expiration
    reason         TEXT,                  -- Explanation for this grant
    CONSTRAINT unique_acl_rule UNIQUE (principal_type, principal_id, resource_type, resource_id)
);

-- Upstream 003_acl_schema.sql
CREATE TABLE IF NOT EXISTS acl_fallback_policies
(
    policy_id             UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    rule_category         VARCHAR(100) NOT NULL UNIQUE, -- Category of rule (e.g., 'user_workspace', 'agent_workspace')
    fallback_access_level INTEGER      NOT NULL,        -- Default access level for this category
    updated_by            VARCHAR(255),
    updated_at            TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- Upstream 007_api_tokens.sql
CREATE TABLE IF NOT EXISTS api_tokens (
    -- Primary identification
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash              VARCHAR(64) NOT NULL UNIQUE,     -- SHA256 hex hash of the token (plaintext never stored)

    -- Metadata
    name                    VARCHAR(255) NOT NULL,           -- Human-readable name for the token
    principal_type          VARCHAR(50) NOT NULL,            -- 'agent', 'task', 'user', 'orchestrator', 'workflow_engine', 'metrics_bridge'

    -- Authorization
    workspace_patterns      TEXT[] NOT NULL DEFAULT '{}',    -- Array of glob patterns ('prod-*', '*', etc.)
    scopes                  TEXT[] NOT NULL DEFAULT '{"connect"}',  -- Permissions: 'connect', 'admin', 'read', 'write'

    -- Lifecycle tracking
    created_by              VARCHAR(255) NOT NULL,           -- Identity that created this token
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Expiration and revocation
    expires_at              TIMESTAMPTZ,                     -- NULL means no expiration
    revoked                 BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at              TIMESTAMPTZ,

    -- Usage tracking
    last_used_at            TIMESTAMPTZ,

    -- Extensibility
    metadata                JSONB NOT NULL DEFAULT '{}'
);

-- Upstream 012_authority_grants_phase0.sql
CREATE TABLE IF NOT EXISTS acl_authority_grants
(
    grant_id           UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    root_grant_id      UUID         NOT NULL,
    subject_type       VARCHAR(50)  NOT NULL,
    subject_id         VARCHAR(255) NOT NULL,
    delegate_type      VARCHAR(50)  NOT NULL,
    delegate_id        VARCHAR(255) NOT NULL,
    issued_by_type     VARCHAR(50)  NOT NULL,
    issued_by_id       VARCHAR(255) NOT NULL,
    root_subject_type  VARCHAR(50)  NOT NULL,
    root_subject_id    VARCHAR(255) NOT NULL,
    parent_grant_id    UUID REFERENCES acl_authority_grants (grant_id) ON DELETE SET NULL,
    may_delegate       BOOLEAN      NOT NULL DEFAULT FALSE,
    remaining_hops     INTEGER      NOT NULL DEFAULT 0,
    workspace_scope    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    resource_scope     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    operation_scope    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    max_access_level   INTEGER      NOT NULL,
    audience_type      VARCHAR(50)  NOT NULL,
    audience_id        VARCHAR(255) NOT NULL,
    valid_while_audience_active BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at         TIMESTAMP    NOT NULL,
    renewable_until    TIMESTAMP    NOT NULL,
    renewed_at         TIMESTAMP,
    revoked            BOOLEAN      NOT NULL DEFAULT FALSE,
    revoked_at         TIMESTAMP,
    reason             TEXT,
    metadata           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMP    NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_authority_grant_renewal_window CHECK (renewable_until >= expires_at),
    CONSTRAINT chk_authority_grant_hops CHECK (remaining_hops >= 0),
    CONSTRAINT chk_authority_grant_delegate_depth CHECK (
        (may_delegate = FALSE AND remaining_hops = 0) OR
        (may_delegate = TRUE AND remaining_hops > 0)
        ),
    CONSTRAINT chk_authority_grant_revocation CHECK (
        (revoked = FALSE AND revoked_at IS NULL) OR
        (revoked = TRUE AND revoked_at IS NOT NULL)
        )
);

-- Upstream 028_acl_groups_roles.sql
CREATE TABLE IF NOT EXISTS acl_groups
(
    group_id    UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    group_name  VARCHAR(255) NOT NULL UNIQUE, -- canonical id used in subjects: "group:<group_name>"
    description TEXT,
    created_by  VARCHAR(255),
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    metadata    JSONB
);

-- Upstream 028_acl_groups_roles.sql
CREATE TABLE IF NOT EXISTS acl_roles
(
    role_id     UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    role_name   VARCHAR(255) NOT NULL UNIQUE, -- canonical id used in subjects: "role:<role_name>"
    description TEXT,
    created_by  VARCHAR(255),
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    metadata    JSONB
);

-- Upstream 028_acl_groups_roles.sql
CREATE TABLE IF NOT EXISTS acl_group_members
(
    id          UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    group_id    UUID         NOT NULL REFERENCES acl_groups (group_id) ON DELETE CASCADE,
    member_type VARCHAR(50)  NOT NULL,
    member_id   VARCHAR(255) NOT NULL,
    granted_by  VARCHAR(255),
    granted_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMP,
    CONSTRAINT unique_group_member UNIQUE (group_id, member_type, member_id)
);

-- Upstream 028_acl_groups_roles.sql
CREATE TABLE IF NOT EXISTS acl_role_assignments
(
    id            UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    role_id       UUID         NOT NULL REFERENCES acl_roles (role_id) ON DELETE CASCADE,
    assignee_type VARCHAR(50)  NOT NULL,
    assignee_id   VARCHAR(255) NOT NULL,
    granted_by    VARCHAR(255),
    granted_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMP,
    CONSTRAINT unique_role_assignment UNIQUE (role_id, assignee_type, assignee_id)
);

CREATE TABLE IF NOT EXISTS schema_version(version integer PRIMARY KEY);
INSERT INTO schema_version VALUES(1) ON CONFLICT DO NOTHING;
-- Explicit local gateway resource. Ordinary identities receive no admin rights.
INSERT INTO acl_rules(principal_type,principal_id,resource_type,resource_id,access_level,granted_by,reason)
VALUES('wildcard','_any_authenticated_user','workspace','auth-app',20,'auth-setup','User access after MT resolution')
ON CONFLICT DO NOTHING;
INSERT INTO acl_fallback_policies(rule_category,fallback_access_level,updated_by)
SELECT category,0,'auth-setup' FROM unnest(ARRAY['user_workspace','agent_workspace','task_workspace','service_workspace','user_agent','agent_agent','global_read','orchestrator_system','user_kv_scope','agent_kv_scope','task_kv_scope','service_kv_scope']) AS category
ON CONFLICT DO NOTHING;
SET LOCAL search_path TO public, pg_catalog;
