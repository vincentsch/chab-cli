package feature_test

import (
	"reflect"
	"testing"

	"github.com/vincentsch/chab-cli/internal/feature"
)

func TestShellOwnedGroups(t *testing.T) {
	want := []feature.RootGroup{
		{ID: "auth", Title: "Authentication & identity:"},
		{ID: "resources", Title: "Projects & credits:"},
		{ID: "config", Title: "Profiles & configuration:"},
		{ID: "api", Title: "Raw API access:"},
		{ID: "utilities", Title: "Diagnostics & utilities:"},
	}

	got := feature.ShellOwnedGroups()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ShellOwnedGroups() = %#v, want %#v", got, want)
	}

	got[0] = feature.RootGroup{ID: "changed", Title: "Changed:"}
	if second := feature.ShellOwnedGroups(); !reflect.DeepEqual(second, want) {
		t.Fatalf("ShellOwnedGroups() returned mutable state: %#v", second)
	}
}
