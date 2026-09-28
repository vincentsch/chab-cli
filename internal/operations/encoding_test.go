package operations

import "testing"

func TestCanonicalizeJSONSortsKeysAndPreservesNull(t *testing.T) {
	got, err := CanonicalizeJSON([]byte(`{ "b": true, "a": null }`))
	if err != nil {
		t.Fatalf("CanonicalizeJSON() error = %v", err)
	}
	if string(got) != `{"a":null,"b":true}` {
		t.Fatalf("canonical JSON = %s", got)
	}
}

func TestCanonicalizeJSONPreservesNumberRepresentation(t *testing.T) {
	got, err := CanonicalizeJSON([]byte(`{"a":1.0,"b":1}`))
	if err != nil {
		t.Fatalf("CanonicalizeJSON() error = %v", err)
	}
	if string(got) != `{"a":1.0,"b":1}` {
		t.Fatalf("canonical JSON = %s", got)
	}
}

func TestRequestDryRunDetectsBooleanTrueOnly(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "true", raw: `{"dry_run":true}`, want: true},
		{name: "false", raw: `{"dry_run":false}`},
		{name: "null", raw: `{"dry_run":null}`},
		{name: "absent", raw: `{"query":"docs"}`},
		{name: "non-object", raw: `["dry_run"]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := RequestDryRun([]byte(test.raw))
			if err != nil {
				t.Fatalf("RequestDryRun() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("RequestDryRun() = %t, want %t", got, test.want)
			}
		})
	}

	if _, err := RequestDryRun([]byte(`{"dry_run":"true"}`)); err == nil {
		t.Fatalf("RequestDryRun() accepted non-boolean dry_run")
	}
}
