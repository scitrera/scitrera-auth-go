// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package msgpack

import (
	"reflect"
	"testing"
)

func TestRoundtrip_AuthChecksAzure(t *testing.T) {
	// Mirrors the shape Python persists for `auth:checks:azure` —
	// {"tid": ["uuid1", "uuid2"]}
	in := map[string]any{
		"tid": []any{"11111111-1111-4111-8111-111111111111", "other-tid"},
	}
	raw, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("decoded type: got %T, want map[string]any", out)
	}
	tids, ok := m["tid"].([]any)
	if !ok {
		t.Fatalf("tid: got %T", m["tid"])
	}
	if len(tids) != 2 || tids[0] != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("tid round-trip wrong: %+v", tids)
	}
}

func TestRoundtrip_ScalarString(t *testing.T) {
	// {"hd": "example.com"} — Google domain check style.
	raw, err := Encode(map[string]any{"hd": "example.com"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, _ := Decode(raw)
	want := map[string]any{"hd": "example.com"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %+v, want %+v", out, want)
	}
}

func TestDecodeNil(t *testing.T) {
	raw, _ := Encode(nil)
	out, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode nil: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil, got %+v", out)
	}
}
