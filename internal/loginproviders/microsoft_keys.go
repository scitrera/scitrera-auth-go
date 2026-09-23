// SPDX-License-Identifier: AGPL-3.0-only
package loginproviders

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/sync/singleflight"
)

// One bounded key cache per configured provider, not per untrusted token/tenant.
// Unknown keys trigger refresh (rotation), with a cooldown to bound bad-kid load.
type microsoftKeys struct {
	client      *http.Client
	url         string
	now         func() time.Time
	mu          sync.RWMutex
	keys        []microsoftKey
	lastSuccess time.Time
	lastAttempt time.Time
	refresh     singleflight.Group
}
type microsoftKey struct {
	key    jose.JSONWebKey
	issuer string
}
type scopedMicrosoftKeys struct {
	keys   *microsoftKeys
	issuer string
}

func (s scopedMicrosoftKeys) VerifySignature(ctx context.Context, raw string) ([]byte, error) {
	token, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, err
	}
	if len(token.Signatures) != 1 || token.Signatures[0].Header.KeyID == "" {
		return nil, fmt.Errorf("Microsoft token requires one signature with a kid")
	}
	kid := token.Signatures[0].Header.KeyID
	keys, err := s.keys.load(ctx, false)
	if err != nil {
		return nil, err
	}
	if payload, ok := s.verify(token, kid, keys); ok {
		return payload, nil
	}
	// A known kid with an invalid signature/scope cannot become valid by fetching
	// the same key again. Only an unknown kid prompts an early rotation refresh.
	for _, key := range keys {
		if key.key.KeyID == kid {
			return nil, fmt.Errorf("Microsoft token signature or signing-key issuer is invalid")
		}
	}
	keys, err = s.keys.load(ctx, true)
	if err != nil {
		return nil, err
	}
	if payload, ok := s.verify(token, kid, keys); ok {
		return payload, nil
	}
	return nil, fmt.Errorf("no permitted Microsoft signing key verifies the token")
}

func (s scopedMicrosoftKeys) verify(token *jose.JSONWebSignature, kid string, keys []microsoftKey) ([]byte, bool) {
	for _, key := range keys {
		if key.key.KeyID != kid || (key.issuer != microsoftIssuerTemplate && key.issuer != s.issuer) {
			continue
		}
		if payload, err := token.Verify(&key.key); err == nil {
			return payload, true
		}
	}
	return nil, false
}

func (k *microsoftKeys) load(ctx context.Context, force bool) ([]microsoftKey, error) {
	k.mu.RLock()
	keys, fresh := k.keys, len(k.keys) > 0 && k.now().Sub(k.lastSuccess) < 24*time.Hour
	k.mu.RUnlock()
	if fresh && !force {
		return keys, nil
	}
	result := k.refresh.DoChan("keys", func() (any, error) {
		k.mu.Lock()
		now := k.now()
		if !force && len(k.keys) > 0 && now.Sub(k.lastSuccess) < 24*time.Hour {
			keys := k.keys
			k.mu.Unlock()
			return keys, nil
		}
		if !k.lastAttempt.IsZero() && now.Sub(k.lastAttempt) < time.Minute {
			keys, fresh := k.keys, len(k.keys) > 0 && now.Sub(k.lastSuccess) < 24*time.Hour
			k.mu.Unlock()
			if fresh {
				return keys, nil
			}
			return nil, fmt.Errorf("Microsoft signing-key refresh is temporarily unavailable")
		}
		k.lastAttempt = now
		k.mu.Unlock()
		// One cancelled login does not cancel refresh for other waiting requests.
		// The background operation has a fixed deadline and cannot run indefinitely.
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		var document struct {
			Keys []json.RawMessage `json:"keys"`
		}
		if err := fetchJSON(refreshCtx, k.client, k.url, &document); err != nil {
			return nil, err
		}
		if len(document.Keys) == 0 || len(document.Keys) > 512 {
			return nil, fmt.Errorf("invalid Microsoft signing-key count")
		}
		updated := make([]microsoftKey, 0, len(document.Keys))
		for _, raw := range document.Keys {
			var key jose.JSONWebKey
			var scope struct {
				Issuer string `json:"issuer"`
			}
			if err := json.Unmarshal(raw, &key); err != nil {
				return nil, fmt.Errorf("invalid Microsoft signing key")
			}
			if err := json.Unmarshal(raw, &scope); err != nil {
				return nil, err
			}
			rsaKey, ok := key.Key.(*rsa.PublicKey)
			if !ok || !key.Valid() || rsaKey.N.BitLen() < 2048 || key.KeyID == "" || scope.Issuer == "" || (key.Use != "" && key.Use != "sig") || (key.Algorithm != "" && key.Algorithm != "RS256") {
				continue
			}
			updated = append(updated, microsoftKey{key: key, issuer: scope.Issuer})
		}
		if len(updated) == 0 {
			return nil, fmt.Errorf("no usable Microsoft signing keys")
		}
		k.mu.Lock()
		k.keys = updated
		k.lastSuccess = k.now()
		k.mu.Unlock()
		return updated, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]microsoftKey), nil
	}
}
