package profile

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

func TestNativeFamiliesOwnDocumentedChildren(t *testing.T) {
	factory := &cmdutil.Factory{}
	for _, test := range []struct {
		name     string
		command  *cobra.Command
		children []string
	}{
		{name: "profile", command: NewProfileCommand(factory), children: []string{"create", "delete", "list", "show", "use"}},
		{name: "config", command: NewConfigCommand(factory), children: []string{"get", "list", "path", "set"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.command == nil {
				t.Fatal("constructor returned nil")
			}
			var children []string
			for _, child := range test.command.Commands() {
				if child.IsAvailableCommand() {
					children = append(children, child.Name())
				}
			}
			sort.Strings(children)
			if !reflect.DeepEqual(children, test.children) {
				t.Fatalf("children = %v, want %v", children, test.children)
			}
		})
	}
}

func TestStoredAuthModelSanitizesCredentialDerivedMetadata(t *testing.T) {
	key := "opaque-command-local-key"
	for _, field := range []string{"display_id", "key_name", "team_display_id", "team_name"} {
		t.Run(field, func(t *testing.T) {
			record := auth.ProfileAuth{APIKey: key}
			switch field {
			case "display_id":
				record.DisplayID = key
			case "key_name":
				record.KeyName = key
			case "team_display_id":
				record.TeamDisplayID = key
			case "team_name":
				record.TeamName = key
			}
			model := storedAuthFor(&auth.File{Profiles: map[string]auth.ProfileAuth{"local": record}}, "local")
			if !model.Present || model.DisplayID == nil || model.KeyName == nil || model.TeamDisplayID == nil || model.TeamName == nil {
				t.Fatalf("%s did not preserve present/non-null shape", field)
			}
			data, err := json.Marshal(model)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), key) || strings.Contains(string(data), "api_key") {
				t.Fatalf("%s model exposed credential-bearing data", field)
			}
		})
	}

	missing := storedAuthFor(&auth.File{Profiles: map[string]auth.ProfileAuth{}}, "local")
	if missing.Present || missing.DisplayID != nil || missing.KeyName != nil || missing.TeamDisplayID != nil || missing.TeamName != nil || missing.LastValidatedAt != nil {
		t.Fatalf("missing stored auth shape = %#v", missing)
	}
}
