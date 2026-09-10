-- SPDX-License-Identifier: AGPL-3.0-only
-- Legacy platform schema, synthetic integration tests only; no records.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Function to automatically update 'updated_at' timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS
$$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Tenants Table
CREATE TABLE IF NOT EXISTS tenants
(
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug       TEXT UNIQUE                                NOT NULL, -- Short, unique identifier for the tenant
    name       TEXT                                       NOT NULL, -- Longer, descriptive name for the tenant
    metadata   JSONB,
    enabled    BOOLEAN          DEFAULT TRUE              NOT NULL,
    created_at TIMESTAMPTZ      DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ      DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Index on slug for quick lookups
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants (slug);
CREATE INDEX IF NOT EXISTS idx_tenants_enabled ON tenants (enabled);

-- Trigger for tenants updated_at
DROP TRIGGER IF EXISTS trigger_tenants_updated_at ON tenants;
CREATE TRIGGER trigger_tenants_updated_at
    BEFORE UPDATE
    ON tenants
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- Users Table
CREATE TABLE IF NOT EXISTS users
(
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email               TEXT UNIQUE                                NOT NULL,
    name                TEXT                                       NOT NULL,
    metadata            JSONB,
    enabled             BOOLEAN          DEFAULT TRUE              NOT NULL,
    default_tenant_slug TEXT,  -- Stores the slug of the user's default tenant
    created_at          TIMESTAMPTZ      DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at          TIMESTAMPTZ      DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT fk_default_tenant
        FOREIGN KEY (default_tenant_slug)
            REFERENCES tenants (slug)
            ON UPDATE CASCADE  -- If tenant slug changes, update it here
            ON DELETE SET NULL -- If default tenant is deleted, set this to NULL
);

-- Index on email for quick lookups
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_enabled ON users (enabled);
CREATE INDEX IF NOT EXISTS idx_users_default_tenant_slug ON users (default_tenant_slug);


-- Trigger for users updated_at
DROP TRIGGER IF EXISTS trigger_users_updated_at ON users;
CREATE TRIGGER trigger_users_updated_at
    BEFORE UPDATE
    ON users
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- User-Tenants Junction Table (Many-to-Many relationship)
CREATE TABLE IF NOT EXISTS user_tenants
(
    user_id    uuid                                  NOT NULL,
    tenant_id  uuid                                  NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, tenant_id),
    CONSTRAINT fk_user
        FOREIGN KEY (user_id)
            REFERENCES users (id)
            ON DELETE CASCADE, -- If user is deleted, remove their tenant memberships
    CONSTRAINT fk_tenant
        FOREIGN KEY (tenant_id)
            REFERENCES tenants (id)
            ON DELETE CASCADE  -- If tenant is deleted, remove users from it
);

-- Indexes for faster joins and lookups
CREATE INDEX IF NOT EXISTS idx_user_tenants_user_id ON user_tenants (user_id);
CREATE INDEX IF NOT EXISTS idx_user_tenants_tenant_id ON user_tenants (tenant_id);

-- Tenant Key-Value Store Table
-- This table is intended to be partitioned by 'tenant_slug'.
-- The actual DDL for creating partitions (e.g., LIST or HASH) depends on your specific PostgreSQL version and requirements.
-- Example: PARTITION BY HASH (tenant_slug); and then create individual partition tables.
CREATE TABLE IF NOT EXISTS tenant_kv_store
(
    tenant_slug TEXT                                  NOT NULL,
    key         TEXT                                  NOT NULL,
    value       BYTEA,              -- Store various data types as bytes (allow encrypt)
    created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_slug, key), -- Ensures key is unique per tenant
    CONSTRAINT fk_kv_tenant_slug
        FOREIGN KEY (tenant_slug)
            REFERENCES tenants (slug)
            ON DELETE CASCADE       -- If tenant is deleted, remove their KV pairs
);

-- Indexes for tenant_kv_store
CREATE INDEX IF NOT EXISTS idx_tenant_kv_store_tenant_slug ON tenant_kv_store (tenant_slug);
CREATE INDEX IF NOT EXISTS idx_tenant_kv_store_tenant_slug_key ON tenant_kv_store (tenant_slug, key);
-- Covered by PK

-- Trigger for tenant_kv_store updated_at
DROP TRIGGER IF EXISTS trigger_tenant_kv_store_updated_at ON tenant_kv_store;
CREATE TRIGGER trigger_tenant_kv_store_updated_at
    BEFORE UPDATE
    ON tenant_kv_store
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- COMMENT ON TABLE tenants IS 'Stores tenant information for the multi-tenant application.';
-- COMMENT ON COLUMN tenants.slug IS 'Short, URL-friendly, unique identifier for the tenant.';
-- COMMENT ON COLUMN tenants.metadata IS 'Arbitrary JSON data associated with the tenant.';
--
-- COMMENT ON TABLE users IS 'Stores user information.';
-- COMMENT ON COLUMN users.email IS 'Unique email address for the user, used for login.';
-- COMMENT ON COLUMN users.default_tenant_slug IS 'Slug of the tenant this user defaults to.';
-- COMMENT ON COLUMN users.metadata IS 'Arbitrary JSON data associated with the user.';
--
-- COMMENT ON TABLE user_tenants IS 'Maps users to tenants, establishing membership (many-to-many).';
--
-- COMMENT ON TABLE tenant_kv_store IS 'Partitioned table storing key-value pairs specific to each tenant.';
-- COMMENT ON COLUMN tenant_kv_store.tenant_slug IS 'The slug of the tenant owning this key-value pair. Used for partitioning.';
-- COMMENT ON COLUMN tenant_kv_store.value IS 'The value, stored as JSONB to allow for flexible data types.';
--

--
-- Tenant CONFIG Key-Value Store Table
-- This is same design as general KV store BUT intended for configuration parameters
-- This table is intended to be partitioned by 'tenant_slug'.
-- The actual DDL for creating partitions (e.g., LIST or HASH) depends on your specific PostgreSQL version and requirements.
-- Example: PARTITION BY HASH (tenant_slug); and then create individual partition tables.
CREATE TABLE IF NOT EXISTS tenant_config
(
    tenant_slug TEXT                                  NOT NULL,
    key         TEXT                                  NOT NULL,
    value       BYTEA,              -- Store various data types as bytes (allow encrypt)
    created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_slug, key), -- Ensures key is unique per tenant
    CONSTRAINT fk_kv_tenant_slug
        FOREIGN KEY (tenant_slug)
            REFERENCES tenants (slug)
            ON DELETE CASCADE       -- If tenant is deleted, remove their KV pairs
);

-- Indexes for tenant_config
CREATE INDEX IF NOT EXISTS idx_tenant_config_tenant_slug ON tenant_config (tenant_slug);
CREATE INDEX IF NOT EXISTS idx_tenant_config_tenant_slug_key ON tenant_config (tenant_slug, key);
-- Covered by PK

-- Trigger for tenant_config updated_at
DROP TRIGGER IF EXISTS trigger_tenant_config_updated_at ON tenant_config;
CREATE TRIGGER trigger_tenant_config_updated_at
    BEFORE UPDATE
    ON tenant_config
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- Tenant Domains Table
-- This table tracks valid domains associated with each tenant
CREATE TABLE IF NOT EXISTS tenant_domains
(
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_slug TEXT                                       NOT NULL,
    domain      TEXT                                       NOT NULL,
    created_at  TIMESTAMPTZ      DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at  TIMESTAMPTZ      DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT fk_tenant_domains_tenant_slug
        FOREIGN KEY (tenant_slug)
            REFERENCES tenants (slug)
            ON DELETE CASCADE, -- If tenant is deleted, remove their domains
    CONSTRAINT uq_tenant_domains_domain
        UNIQUE (domain)        -- Ensures domain is unique across all tenants
);

-- Indexes for tenant_domains
CREATE INDEX IF NOT EXISTS idx_tenant_domains_tenant_slug ON tenant_domains (tenant_slug);
CREATE INDEX IF NOT EXISTS idx_tenant_domains_domain ON tenant_domains (domain);

-- Trigger for tenant_domains updated_at
DROP TRIGGER IF EXISTS trigger_tenant_domains_updated_at ON tenant_domains;
CREATE TRIGGER trigger_tenant_domains_updated_at
    BEFORE UPDATE
    ON tenant_domains
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();
