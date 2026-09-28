package mcpcmd

import (
	"testing"

	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

func TestCommandTree(t *testing.T) {
	cmd := NewCommand(&cmdutil.Factory{Version: "test"})
	if cmd.Use != "mcp" || cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
		t.Fatalf("mcp command missing help fields: %#v", cmd)
	}
	serve, _, err := cmd.Find([]string{"serve"})
	if err != nil || serve == nil || serve.Use != "serve" {
		t.Fatalf("Find(serve) = %v, %v", serve, err)
	}
	if serve.Short == "" || serve.Long == "" || serve.Example == "" {
		t.Fatalf("serve command missing help fields: %#v", serve)
	}
}
