package api

import (
	"bytes"
	"encoding/json"
)

// CursorPagination is the public Chab cursor pagination contract.
type CursorPagination struct {
	Limit      int     `json:"limit"`
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
	PrevCursor *string `json:"prev_cursor"`
}

func decodeCursorPagination(raw json.RawMessage, status int, meta ResponseMeta) (*CursorPagination, *ProtocolError) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, paginationProtocolError(status, meta, nil)
	}
	for _, key := range []string{"limit", "has_more", "next_cursor"} {
		if _, ok := fields[key]; !ok {
			return nil, paginationProtocolError(status, meta, nil)
		}
	}
	var value CursorPagination
	if json.Unmarshal(raw, &value) != nil || value.Limit < 1 || value.Limit > 100 || bytes.Equal(bytes.TrimSpace(fields["has_more"]), []byte("null")) {
		return nil, paginationProtocolError(status, meta, nil)
	}
	if value.HasMore && (value.NextCursor == nil || *value.NextCursor == "") || !value.HasMore && value.NextCursor != nil {
		return nil, paginationProtocolError(status, meta, nil)
	}
	return &value, nil
}
