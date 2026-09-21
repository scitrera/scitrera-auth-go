-- SPDX-License-Identifier: AGPL-3.0-only
-- Overrides belong to a specific membership and disappear when it is removed.
-- Auto-add inserts no override values and receives the empty default.
ALTER TABLE public.user_tenants ADD COLUMN auth_checks jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(auth_checks) = 'object');
