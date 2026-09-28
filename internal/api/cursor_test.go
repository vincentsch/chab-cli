package api

import (
	"encoding/json"
	"testing"
)

func TestCursorPaginationValidatesCompleteShape(t *testing.T) {
	for _, raw := range []string{
		`{"limit":25,"has_more":true,"next_cursor":"opaque","prev_cursor":null}`,
		`{"limit":25,"has_more":false,"next_cursor":null,"prev_cursor":"previous"}`,
		`{"limit":25,"has_more":true,"next_cursor":"opaque"}`,
	} {
		if _, err := decodeCursorPagination(json.RawMessage(raw), 200, ResponseMeta{}); err != nil {
			t.Fatalf("valid cursor rejected: %s", raw)
		}
	}
	for _, raw := range []string{
		`null`, `{}`, `{"limit":25,"has_more":true,"prev_cursor":null}`, `{"limit":25,"has_more":true,"next_cursor":null,"prev_cursor":null}`,
		`{"limit":25,"has_more":false,"next_cursor":"stale","prev_cursor":null}`, `{"limit":25,"has_more":null,"next_cursor":null,"prev_cursor":null}`,
		`{"limit":101,"has_more":false,"next_cursor":null,"prev_cursor":null}`, `{"limit":25,"has_more":true,"next_cursor":12,"prev_cursor":null}`,
	} {
		if _, err := decodeCursorPagination(json.RawMessage(raw), 200, ResponseMeta{}); err == nil {
			t.Fatalf("invalid cursor accepted: %s", raw)
		}
	}
}
