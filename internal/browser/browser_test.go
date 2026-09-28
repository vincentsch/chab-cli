package browser

import (
	"context"
	"reflect"
	"testing"
)

func TestCommandArgvByGOOS(t *testing.T) {
	rawURL := `https://example.test/login?code=A B;rm -rf /`
	tests := []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"open", rawURL}},
		{"windows", []string{"rundll32", "url.dll,FileProtocolHandler", rawURL}},
		{"linux", []string{"xdg-open", rawURL}},
		{"freebsd", []string{"xdg-open", rawURL}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			cmd := command(context.Background(), tt.goos, rawURL)
			got := cmd.Args
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("argv = %#v, want %#v", got, tt.want)
			}
			if cmd.Args[len(cmd.Args)-1] != rawURL {
				t.Fatalf("URL was not the trailing discrete argument: %#v", cmd.Args)
			}
		})
	}
}
