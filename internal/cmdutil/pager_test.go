package cmdutil

import (
	"reflect"
	"testing"
)

func TestParsePagerArgv(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: `less -FRX`, want: []string{"less", "-FRX"}},
		{input: `pager "two words" 'and more'`, want: []string{"pager", "two words", "and more"}},
		{input: `pager escaped\ space`, want: []string{"pager", "escaped space"}},
	}
	for _, tt := range tests {
		got, err := parsePagerArgv(tt.input)
		if err != nil {
			t.Fatalf("parsePagerArgv(%q) error = %v", tt.input, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("parsePagerArgv(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParsePagerArgvRejectsInvalidQuoteAndEscape(t *testing.T) {
	for _, input := range []string{`less "unterminated`, `less \`} {
		if argv, err := parsePagerArgv(input); err == nil {
			t.Fatalf("parsePagerArgv(%q) = %v, want error", input, argv)
		}
	}
}

func TestPagerArgvSelection(t *testing.T) {
	oldGOOS := pagerRuntimeGOOS
	t.Cleanup(func() { pagerRuntimeGOOS = oldGOOS })
	pagerRuntimeGOOS = "linux"

	f := &Factory{LookupEnv: func(name string) (string, bool) {
		switch name {
		case "CHAB_PAGER":
			return "chab-pager -x", true
		case "PAGER":
			return "pager", true
		default:
			return "", false
		}
	}}
	argv, ok := f.pagerArgv()
	if !ok || !reflect.DeepEqual(argv, []string{"chab-pager", "-x"}) {
		t.Fatalf("pager argv = %v, %t", argv, ok)
	}

	f.LookupEnv = func(name string) (string, bool) {
		switch name {
		case "CHAB_PAGER":
			return "   ", true
		case "PAGER":
			return "pager", true
		default:
			return "", false
		}
	}
	argv, ok = f.pagerArgv()
	if !ok || !reflect.DeepEqual(argv, []string{"pager"}) {
		t.Fatalf("blank CHAB_PAGER fallback = %v, %t", argv, ok)
	}

	f.LookupEnv = func(string) (string, bool) { return "", false }
	argv, ok = f.pagerArgv()
	if !ok || !reflect.DeepEqual(argv, []string{"less", "-FRX"}) {
		t.Fatalf("unix default = %v, %t", argv, ok)
	}

	pagerRuntimeGOOS = "windows"
	argv, ok = f.pagerArgv()
	if ok || argv != nil {
		t.Fatalf("windows default = %v, %t, want none", argv, ok)
	}
}

func TestPagerArgvInvalidEnvSuppressesPager(t *testing.T) {
	f := &Factory{LookupEnv: func(name string) (string, bool) {
		if name == "CHAB_PAGER" {
			return `"unterminated`, true
		}
		return "", false
	}}
	if argv, ok := f.pagerArgv(); ok || argv != nil {
		t.Fatalf("invalid pager env = %v, %t, want none", argv, ok)
	}
}
