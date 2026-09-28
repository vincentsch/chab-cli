package testutil

import (
	"net/http/httptest"
	"testing"
)

func TestSafeRequestPathOmitsRawQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/gadgets/with%20space?token=secret&tag=a%2Cb", nil)
	if got, want := safeRequestPath(req), "/api/v1/gadgets/with%20space"; got != want {
		t.Fatalf("safeRequestPath() = %q, want %q", got, want)
	}
}
