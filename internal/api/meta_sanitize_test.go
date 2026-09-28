package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeRawMetaPreservesValuesAndReservesPagination(t *testing.T) {
	input := map[string]json.RawMessage{
		"opaque":     json.RawMessage(`{"opaque":"opaque","dup":"opaque","dup":"safe","number":9007199254740993}`),
		"other":      json.RawMessage(`["opaque",true,null]`),
		"pagination": json.RawMessage(`{"current_page":1}`),
		"safe":       json.RawMessage(`"safe"`),
	}
	transform := func(value string) string {
		switch value {
		case "opaque", "other":
			return "pagination"
		default:
			return value
		}
	}

	got, err := sanitizeRawMeta(input, transform)
	if err != nil {
		t.Fatalf("sanitizeRawMeta() error = %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("sanitized member count = %d, want 4: %#v", len(got), got)
	}
	if string(got["pagination"]) != `{"current_page":1}` ||
		string(got["pagination~2"]) != `{"pagination":"pagination","dup":"pagination","dup~2":"safe","number":9007199254740993}` ||
		string(got["pagination~3"]) != `["pagination",true,null]` ||
		string(got["safe"]) != `"safe"` {
		t.Fatalf("sanitized metadata = %#v", got)
	}
	if string(input["opaque"]) != `{"opaque":"opaque","dup":"opaque","dup":"safe","number":9007199254740993}` {
		t.Fatalf("input mutated: %s", input["opaque"])
	}
}

func TestSanitizeRawMetaFailsClosed(t *testing.T) {
	got, err := sanitizeRawMeta(map[string]json.RawMessage{
		"safe":   json.RawMessage(`"value"`),
		"broken": json.RawMessage(`{"unterminated":`),
	}, strings.ToUpper)
	if err == nil {
		t.Fatal("sanitizeRawMeta() error = nil")
	}
	if got != nil {
		t.Fatalf("sanitizeRawMeta() = %#v, want nil", got)
	}
}
