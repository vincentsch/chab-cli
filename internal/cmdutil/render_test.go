package cmdutil_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/output"
)

func TestWriteCommandResultJSONUsesSharedStableJSON(t *testing.T) {
	var stdout bytes.Buffer
	cmd := renderCommand(&stdout, true)
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: map[string]string{"html": "<tag>&value>"},
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "human\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true, JSON: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "\n  \"html\":") ||
		!strings.Contains(got, `\u003ctag\u003e\u0026value\u003e`) ||
		!strings.HasSuffix(got, "\n") {
		t.Fatalf("stdout is not shared stable JSON: %q", got)
	}
}

func TestWriteCommandResultTransformPrecedenceAndInput(t *testing.T) {
	var stdout bytes.Buffer
	cmd := renderCommand(&stdout, true)
	if err := cmd.Root().PersistentFlags().Set("jq", ".html"); err != nil {
		t.Fatal(err)
	}
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: map[string]string{"html": "<tag>&value>"},
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "human\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true, JSON: true, JQ: true, Template: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if got, want := stdout.String(), "\"<tag>&value>\"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestWriteCommandResultTemplateOutput(t *testing.T) {
	var stdout bytes.Buffer
	cmd := renderCommand(&stdout, false)
	if err := cmd.Root().PersistentFlags().Set("template", "{{.name}}"); err != nil {
		t.Fatal(err)
	}
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine:  map[string]string{"name": "Acme"},
		Supports: cmdutil.OutputSupport{JSON: true, Template: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if got, want := stdout.String(), "Acme\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestWriteCommandResultRequestedUndeclaredModeDoesNotFallThrough(t *testing.T) {
	var stdout bytes.Buffer
	cmd := renderCommand(&stdout, false)
	if err := cmd.Root().PersistentFlags().Set("jq", "."); err != nil {
		t.Fatal(err)
	}
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: map[string]string{"name": "Acme"},
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "human\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true, JSON: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestWriteCommandResultPlainSplitsDataAndProse(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := renderCommand(&stdout, false)
	cmd.SetErr(&stderr)
	if err := cmd.Root().PersistentFlags().Set("plain", "true"); err != nil {
		t.Fatal(err)
	}
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Human: cmdutil.HumanOutput{Plain: func(data, prose io.Writer) {
			_, _ = io.WriteString(data, "key\tvalue\n")
			_, _ = io.WriteString(prose, "guidance\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true, Plain: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if stdout.String() != "key\tvalue\n" || stderr.String() != "guidance\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestWriteCommandResultHumanRedactsSanitizesAndWritesOnce(t *testing.T) {
	var stdout countingRenderWriter
	cmd := renderCommand(&stdout, false)
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "Key: ak_test|super-secret\n")
			_, _ = io.WriteString(w, "Team: Bad\x1b[31mName\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if stdout.writes != 1 {
		t.Fatalf("writes = %d, want 1", stdout.writes)
	}
	if got, want := stdout.String(), "Key: ak_test|[REDACTED]\nTeam: Bad Name\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestWriteCommandResultHumanTerminalModeAndPager(t *testing.T) {
	t.Run("mode", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		if err := cmd.Root().PersistentFlags().Set("no-ansi", "true"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{}
		var gotMode string
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{RenderMode: func(w io.Writer, mode output.TerminalMode) {
				gotMode = strings.Join([]string{
					boolText(mode.Color),
					boolText(mode.ANSI),
					boolText(mode.Sanitize),
				}, ",")
				_, _ = io.WriteString(w, "human\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if gotMode != "false,false,true" {
			t.Fatalf("mode = %s", gotMode)
		}
	})

	t.Run("pager", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmd := renderCommand(&stdout, false)
		cmd.SetErr(&stderr)
		var pagerInput string
		factory := &cmdutil.Factory{
			StdoutIsTerminal: func() bool { return true },
			TerminalHeight:   func() (int, bool) { return 1, true },
			LookupEnv:        func(string) (string, bool) { return "page -x", true },
			RunPager: func(_ context.Context, argv []string, input []byte, _ io.Writer, _ io.Writer) error {
				if strings.Join(argv, " ") != "page -x" {
					t.Fatalf("pager argv = %v", argv)
				}
				pagerInput = string(input)
				return nil
			},
		}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				_, _ = io.WriteString(w, "one\ntwo\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if stdout.String() != "" || pagerInput != "one\ntwo\n" {
			t.Fatalf("stdout=%q pagerInput=%q", stdout.String(), pagerInput)
		}
	})
}

func TestWriteCommandResultPagerFallbackAndExitError(t *testing.T) {
	t.Run("default byte buffer stdout is non terminal", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		factory := &cmdutil.Factory{
			TerminalHeight: func() (int, bool) { return 1, true },
			LookupEnv:      func(string) (string, bool) { return "pager", true },
			RunPager: func(context.Context, []string, []byte, io.Writer, io.Writer) error {
				t.Fatalf("pager should not run for default non-terminal stdout")
				return nil
			},
		}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				_, _ = io.WriteString(w, "one\ntwo\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if stdout.String() != "one\ntwo\n" {
			t.Fatalf("stdout=%q", stdout.String())
		}
	})

	t.Run("non tty suppresses", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		called := false
		factory := &cmdutil.Factory{
			StdoutIsTerminal: func() bool { return false },
			TerminalHeight:   func() (int, bool) { return 1, true },
			LookupEnv:        func(string) (string, bool) { return "pager", true },
			RunPager: func(context.Context, []string, []byte, io.Writer, io.Writer) error {
				called = true
				return nil
			},
		}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				_, _ = io.WriteString(w, "one\ntwo\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if called || stdout.String() != "one\ntwo\n" {
			t.Fatalf("called=%t stdout=%q", called, stdout.String())
		}
	})

	t.Run("invalid pager spec falls back", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		called := false
		factory := &cmdutil.Factory{
			StdoutIsTerminal: func() bool { return true },
			TerminalHeight:   func() (int, bool) { return 1, true },
			LookupEnv:        func(string) (string, bool) { return `"unterminated`, true },
			RunPager: func(context.Context, []string, []byte, io.Writer, io.Writer) error {
				called = true
				return nil
			},
		}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				_, _ = io.WriteString(w, "one\ntwo\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if called || stdout.String() != "one\ntwo\n" {
			t.Fatalf("called=%t stdout=%q", called, stdout.String())
		}
	})

	t.Run("unknown height threshold", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			lineCount int
			wantPage  bool
		}{
			{name: "at threshold", lineCount: 40, wantPage: false},
			{name: "above threshold", lineCount: 41, wantPage: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var stdout bytes.Buffer
				cmd := renderCommand(&stdout, false)
				paged := false
				factory := &cmdutil.Factory{
					StdoutIsTerminal: func() bool { return true },
					TerminalHeight:   func() (int, bool) { return 0, false },
					LookupEnv:        func(string) (string, bool) { return "pager", true },
					RunPager: func(_ context.Context, _ []string, input []byte, _ io.Writer, _ io.Writer) error {
						paged = true
						if got := strings.Count(string(input), "\n"); got != tc.lineCount {
							t.Fatalf("pager line count = %d, want %d", got, tc.lineCount)
						}
						return nil
					},
				}
				rendered := strings.Repeat("line\n", tc.lineCount)
				err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
					Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
						_, _ = io.WriteString(w, rendered)
					}},
					Supports: cmdutil.OutputSupport{Human: true},
				})
				if err != nil {
					t.Fatalf("WriteCommandResult() error = %v", err)
				}
				if paged != tc.wantPage {
					t.Fatalf("paged=%t, want %t", paged, tc.wantPage)
				}
				if tc.wantPage {
					if stdout.String() != "" {
						t.Fatalf("stdout=%q, want pager to consume output", stdout.String())
					}
				} else if stdout.String() != rendered {
					t.Fatalf("stdout=%q, want direct output", stdout.String())
				}
			})
		}
	})

	t.Run("start failure falls back", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		factory := &cmdutil.Factory{
			StdoutIsTerminal: func() bool { return true },
			TerminalHeight:   func() (int, bool) { return 1, true },
			LookupEnv:        func(string) (string, bool) { return "missing", true },
			RunPager: func(context.Context, []string, []byte, io.Writer, io.Writer) error {
				return &cmdutil.PagerStartError{Err: errors.New("missing")}
			},
		}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				_, _ = io.WriteString(w, "one\ntwo\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if stdout.String() != "one\ntwo\n" {
			t.Fatalf("stdout=%q", stdout.String())
		}
	})

	t.Run("nonzero returns", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		factory := &cmdutil.Factory{
			StdoutIsTerminal: func() bool { return true },
			TerminalHeight:   func() (int, bool) { return 1, true },
			LookupEnv:        func(string) (string, bool) { return "pager", true },
			RunPager: func(context.Context, []string, []byte, io.Writer, io.Writer) error {
				return &cmdutil.PagerExitError{Err: errors.New("exit 2")}
			},
		}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				_, _ = io.WriteString(w, "one\ntwo\n")
			}},
			Supports: cmdutil.OutputSupport{Human: true},
		})
		if err == nil || !strings.Contains(err.Error(), "pager failed") {
			t.Fatalf("error = %v", err)
		}
		if stdout.String() != "" {
			t.Fatalf("stdout=%q, want no replay", stdout.String())
		}
	})
}

func TestWriteCommandResultExactRedactionForTransformAndPlain(t *testing.T) {
	t.Run("transform", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		if err := cmd.Root().PersistentFlags().Set("jq", ".note"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{Rungrad: exactRegistry{}}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Machine:  map[string]string{"note": "visible-secret"},
			Supports: cmdutil.OutputSupport{JSON: true, JQ: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if strings.Contains(stdout.String(), "visible-secret") || !strings.Contains(stdout.String(), "[EXACT]") {
			t.Fatalf("stdout=%q", stdout.String())
		}
	})

	t.Run("plain", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, false)
		if err := cmd.Root().PersistentFlags().Set("plain", "true"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{Rungrad: exactRegistry{}}
		err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Plain: func(data, prose io.Writer) {
				_, _ = io.WriteString(data, "note\tvisible-secret\n")
			}},
			Supports: cmdutil.OutputSupport{Plain: true},
		})
		if err != nil {
			t.Fatalf("WriteCommandResult() error = %v", err)
		}
		if strings.Contains(stdout.String(), "visible-secret") || !strings.Contains(stdout.String(), "[EXACT]") {
			t.Fatalf("stdout=%q", stdout.String())
		}
	})
}

func TestWriteCommandResultHumanOnlyIgnoresJSONFlag(t *testing.T) {
	var stdout bytes.Buffer
	cmd := renderCommand(&stdout, true)
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: map[string]string{"unexpected": "json"},
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "human\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if got, want := stdout.String(), "human\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestWriteCommandResultPropagatesWriteErrors(t *testing.T) {
	wantErr := errors.New("write failed")
	cmd := renderCommand(failingWriter{err: wantErr}, false)
	factory := &cmdutil.Factory{}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "human\n")
		}},
		Supports: cmdutil.OutputSupport{Human: true},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WriteCommandResult() error = %v, want %v", err, wantErr)
	}
}

func TestWriteResultRetainedDelegationSemantics(t *testing.T) {
	t.Run("non-nil machine is JSON capable", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, true)
		factory := &cmdutil.Factory{}
		if err := factory.WriteResult(cmd, map[string]string{"ok": "yes"}, cmdutil.HumanOutput{}); err != nil {
			t.Fatalf("WriteResult() error = %v", err)
		}
		if !strings.Contains(stdout.String(), "\n  \"ok\": \"yes\"\n") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("nil machine is human only", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, true)
		factory := &cmdutil.Factory{}
		if err := factory.WriteResult(cmd, nil, cmdutil.HumanOutput{Render: func(w io.Writer) {
			_, _ = io.WriteString(w, "human\n")
		}}); err != nil {
			t.Fatalf("WriteResult() error = %v", err)
		}
		if got, want := stdout.String(), "human\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	})
}

func TestWriteResultWithMetaOptInAndOriginalSupport(t *testing.T) {
	t.Run("no flag preserves machine bytes", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, true)
		factory := &cmdutil.Factory{}
		if err := factory.WriteResultWithMeta(cmd, map[string]string{"ok": "yes"}, api.ResponseMeta{RequestID: "req-hidden"}, false, cmdutil.HumanOutput{}); err != nil {
			t.Fatalf("WriteResultWithMeta() error = %v", err)
		}
		if strings.Contains(stdout.String(), `"meta"`) || !strings.Contains(stdout.String(), `"ok": "yes"`) {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("json wraps data and meta", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, true)
		if err := cmd.Root().PersistentFlags().Set("include-meta", "true"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{}
		if err := factory.WriteResultWithMeta(cmd, map[string]string{"ok": "yes"}, api.ResponseMeta{RequestID: "req-visible"}, false, cmdutil.HumanOutput{}); err != nil {
			t.Fatalf("WriteResultWithMeta() error = %v", err)
		}
		for _, want := range []string{`"data":`, `"ok": "yes"`, `"meta":`, `"request_id": "req-visible"`} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout missing %q: %s", want, stdout.String())
			}
		}
	})

	t.Run("jq and template see wrapper root", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			flag  string
			value string
			want  string
		}{
			{name: "jq", flag: "jq", value: ".meta.request_id", want: "\"req-transform\"\n"},
			{name: "template", flag: "template", value: "{{.data.ok}} {{.meta.request_id}}", want: "yes req-transform\n"},
		} {
			t.Run(test.name, func(t *testing.T) {
				var stdout bytes.Buffer
				cmd := renderCommand(&stdout, false)
				if err := cmd.Root().PersistentFlags().Set("include-meta", "true"); err != nil {
					t.Fatal(err)
				}
				if err := cmd.Root().PersistentFlags().Set(test.flag, test.value); err != nil {
					t.Fatal(err)
				}
				factory := &cmdutil.Factory{}
				if err := factory.WriteResultWithMeta(cmd, map[string]string{"ok": "yes"}, api.ResponseMeta{RequestID: "req-transform"}, false, cmdutil.HumanOutput{}); err != nil {
					t.Fatalf("WriteResultWithMeta() error = %v", err)
				}
				if got := stdout.String(); got != test.want {
					t.Fatalf("stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("nil machine remains human only", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, true)
		if err := cmd.Root().PersistentFlags().Set("include-meta", "true"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{}
		if err := factory.WriteResultWithMeta(cmd, nil, api.ResponseMeta{RequestID: "req-hidden"}, false, cmdutil.HumanOutput{
			Render: func(w io.Writer) { _, _ = io.WriteString(w, "human\n") },
		}); err != nil {
			t.Fatalf("WriteResultWithMeta() error = %v", err)
		}
		if got := stdout.String(); got != "human\n" {
			t.Fatalf("stdout = %q", got)
		}
	})

	t.Run("write error propagates", func(t *testing.T) {
		wantErr := errors.New("write failed")
		cmd := renderCommand(failingWriter{err: wantErr}, true)
		if err := cmd.Root().PersistentFlags().Set("include-meta", "true"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{}
		err := factory.WriteResultWithMeta(cmd, map[string]bool{"ok": true}, api.ResponseMeta{}, false, cmdutil.HumanOutput{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}

func TestWriteResultDryRunPreviewDispatch(t *testing.T) {
	preview := output.DryRunPreview{
		Method: "POST",
		Path:   "/projects",
		Body: []output.DryRunValue{
			{Name: "name", Value: "Demo"},
			{Name: "Authorization", Value: "Bearer ak_preview|secret-value", Secret: true},
		},
		Idempotency: &output.DryRunIdempotency{Source: "generated"},
	}

	t.Run("json", func(t *testing.T) {
		var stdout bytes.Buffer
		cmd := renderCommand(&stdout, true)
		factory := &cmdutil.Factory{}
		if err := factory.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: preview.Render,
			Plain:  preview.RenderPlain,
		}); err != nil {
			t.Fatalf("WriteResult() error = %v", err)
		}
		if !strings.Contains(stdout.String(), "\n  \"method\": \"POST\"") ||
			!strings.Contains(stdout.String(), "\"Authorization\"") ||
			strings.Contains(stdout.String(), "secret-value") ||
			strings.Contains(stdout.String(), "Bearer ak_preview") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("plain", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmd := renderCommand(&stdout, false)
		cmd.SetErr(&stderr)
		if err := cmd.Root().PersistentFlags().Set("plain", "true"); err != nil {
			t.Fatal(err)
		}
		factory := &cmdutil.Factory{}
		if err := factory.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: preview.Render,
			Plain:  preview.RenderPlain,
		}); err != nil {
			t.Fatalf("WriteResult() error = %v", err)
		}
		if !strings.Contains(stdout.String(), "Body.Authorization\t[REDACTED]\n") ||
			!strings.Contains(stdout.String(), "Idempotency key\tgenerated on send\n") ||
			strings.Contains(stdout.String(), "secret-value") ||
			stderr.String() != "" {
			t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	})
}

func TestWriteCommandResultJSONAppliesSecretRegistry(t *testing.T) {
	var stdout bytes.Buffer
	cmd := renderCommand(&stdout, true)
	factory := &cmdutil.Factory{Rungrad: exactRegistry{}}

	err := factory.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine:  map[string]string{"note": "visible-secret"},
		Supports: cmdutil.OutputSupport{JSON: true},
	})
	if err != nil {
		t.Fatalf("WriteCommandResult() error = %v", err)
	}
	if strings.Contains(stdout.String(), "visible-secret") || !strings.Contains(stdout.String(), "[EXACT]") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRenderAPIErrorJSONCompactShapes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantCode    string
		wantRequest bool
	}{
		{name: "api", err: &api.Error{Code: "invalid_api_token", Message: "Invalid.", RequestID: "req-1", Details: map[string][]string{"token": {"bad"}}}, wantCode: "invalid_api_token", wantRequest: true},
		{name: "protocol", err: &api.ProtocolError{Detail: "response envelope data is null", RequestID: "req-2"}, wantCode: "api_protocol_error", wantRequest: true},
		{name: "transport", err: &api.TransportError{Err: errors.New("dial tcp failed"), Attempts: 1}, wantCode: "api_network_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if !cmdutil.RenderAPIError(&stderr, tt.err, true) {
				t.Fatalf("RenderAPIError() = false")
			}
			var envelope struct {
				Error map[string]any `json:"error"`
			}
			if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
				t.Fatalf("stderr is not JSON: %v\n%s", err, stderr.String())
			}
			if envelope.Error["code"] != tt.wantCode {
				t.Fatalf("error object = %#v", envelope.Error)
			}
			if _, ok := envelope.Error["request_id"]; ok != tt.wantRequest {
				t.Fatalf("request_id presence = %t, want %t in %#v", ok, tt.wantRequest, envelope.Error)
			}
			for _, rejected := range []string{"type", "status", "retry_after", "rate_limit"} {
				if _, ok := envelope.Error[rejected]; ok {
					t.Fatalf("error object included %q: %#v", rejected, envelope.Error)
				}
			}
		})
	}
}

func TestRenderAPIErrorLocalAndUsageErrorsAreNotAPIOutput(t *testing.T) {
	for _, err := range []error{
		errors.New("local failure"),
		&api.UsageError{Field: "path", Detail: "bad path"},
	} {
		var stderr bytes.Buffer
		if cmdutil.RenderAPIError(&stderr, err, true) {
			t.Fatalf("RenderAPIError(%T) = true", err)
		}
		if stderr.String() != "" {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
		if cmdutil.RenderAPIError(nil, err, true) {
			t.Fatalf("RenderAPIError(nil, %T) = true", err)
		}
	}
}

func renderCommand(stdout io.Writer, jsonMode bool) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.PersistentFlags().Bool("json", false, "")
	cmd.PersistentFlags().Bool("include-meta", false, "")
	cmd.PersistentFlags().String("jq", "", "")
	cmd.PersistentFlags().String("template", "", "")
	cmd.PersistentFlags().Bool("plain", false, "")
	cmd.PersistentFlags().Bool("no-color", false, "")
	cmd.PersistentFlags().Bool("no-ansi", false, "")
	cmd.PersistentFlags().Bool("no-pager", false, "")
	if jsonMode {
		if err := cmd.PersistentFlags().Set("json", "true"); err != nil {
			panic(err)
		}
	}
	cmd.SetOut(stdout)
	return cmd
}

type countingRenderWriter struct {
	bytes.Buffer
	writes int
}

func (w *countingRenderWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.Buffer.Write(p)
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type exactRegistry struct{}

func (exactRegistry) RegisterSecret(string) {}

func (exactRegistry) RedactJSON(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("visible-secret"), []byte("[EXACT]"))
}

func (exactRegistry) RedactText(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("visible-secret"), []byte("[EXACT]"))
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
