// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// Package msgpack wraps vmihailenco/msgpack/v5 with the small bit of
// post-processing the Python side requires: msgpack maps with string keys
// come back as map[string]any (not map[any]any) so caller code can do clean
// claim-style lookups.
package msgpack

import (
	"github.com/vmihailenco/msgpack/v5"
)

// Decode unpacks raw msgpack bytes into a Go any.
//
// vmihailenco/msgpack v5 returns map[string]interface{} for msgpack maps
// whose keys are all strings, so the Python `auth:checks:{provider}` blobs
// (which are dicts of string→list[string]/string) deserialize naturally.
//
// The returned value is one of:
//
//   - map[string]any (msgpack map with string keys)
//   - []any           (msgpack array)
//   - string, int64, uint64, float64, bool, nil
//   - []byte          (msgpack bin)
func Decode(raw []byte) (any, error) {
	var v any
	if err := msgpack.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// MustDecode is the panic-on-error variant — for fixtures and tests.
func MustDecode(raw []byte) any {
	v, err := Decode(raw)
	if err != nil {
		panic(err)
	}
	return v
}

// Encode serializes v to MessagePack bytes for the admin persistence layer
// and interoperability fixtures. The resolver remains read-only.
func Encode(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}
