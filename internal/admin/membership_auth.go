// SPDX-License-Identifier: AGPL-3.0-only
package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// MembershipAuth is a full replacement of this membership's provider overrides.
// Empty checks inherit every tenant rule. An individual claim can be replaced,
// but not deleted or made optional with null. Provider admission stays separate.
type MembershipAuth struct {
	Checks map[string]map[string]any `json:"checks"`
}

func (p *MembershipAuth) validate() error {
	if p.Checks == nil {
		return invalid("checks must be an object; use {} to inherit tenant rules")
	}
	for _, checks := range p.Checks {
		if len(checks) == 0 {
			return invalid("Provider overrides must contain claim checks; omit the provider to inherit its rules")
		}
	}
	policy := PolicyInput{Checks: p.Checks}
	return policy.validate()
}

func membershipAuth(ctx context.Context, tx *sql.Tx, id, slug string) (any, error) {
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT ut.auth_checks FROM public.user_tenants ut JOIN public.tenants t ON t.id=ut.tenant_id WHERE ut.user_id=$1 AND t.slug=$2`, id, slug).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, missing()
	}
	if err != nil {
		return nil, err
	}
	var checks map[string]map[string]any
	if err := json.Unmarshal(raw, &checks); err != nil {
		return nil, err
	}
	return MembershipAuth{Checks: checks}, nil
}

func saveMembershipAuth(ctx context.Context, tx *sql.Tx, id, slug string, input MembershipAuth) error {
	raw, err := json.Marshal(input.Checks)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE public.user_tenants SET auth_checks=$3::jsonb WHERE user_id=$1 AND tenant_id=(SELECT id FROM public.tenants WHERE slug=$2)`, id, slug, string(raw))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return missing()
	}
	return nil
}
