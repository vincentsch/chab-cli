package output_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
)

func TestDetailRender(t *testing.T) {
	doc := output.Detail{Nodes: []output.Node{
		output.Field("Profile", "local"),
		output.Line("free prose"),
		output.Section("Capabilities",
			output.Node{
				Value: "projects (1 of 2 available)",
				Children: []output.Node{
					output.Field("projects.read", "Read projects (available)"),
				},
			},
		),
		output.Node{
			Label: "Project scope",
			Value: "selected",
			Children: []output.Node{
				output.Field("Selected count", "5"),
				output.Line("Note: capped"),
			},
		},
		output.Field("Empty", ""),
	}}

	var buf bytes.Buffer
	doc.Render(&buf)
	want := "" +
		"Profile: local\n" +
		"free prose\n" +
		"Capabilities:\n" +
		"  projects (1 of 2 available)\n" +
		"    projects.read: Read projects (available)\n" +
		"Project scope: selected\n" +
		"  Selected count: 5\n" +
		"  Note: capped\n" +
		"Empty: \n"
	if buf.String() != want {
		t.Fatalf("Detail.Render() = %q, want %q", buf.String(), want)
	}
}

func TestDetailRenderSanitizesHumanCells(t *testing.T) {
	doc := output.Detail{Nodes: []output.Node{
		output.Field("Stored API key", "qa_display (QA key \x1b[31mred\a)"),
		output.Field("Status\nForged", "active\nInjected\tvalue"),
		output.Section("Unsafe\x1b[31mSection",
			output.Line("Bad\x1b[31mName\a"),
		),
	}}

	var buf bytes.Buffer
	doc.Render(&buf)
	want := "" +
		"Stored API key: qa_display (QA key red )\n" +
		"Status Forged: active Injected value\n" +
		"Unsafe Section:\n" +
		"  Bad Name \n"
	if buf.String() != want {
		t.Fatalf("Detail.Render() = %q, want %q", buf.String(), want)
	}
}

func TestDetailEmptyRendersNothing(t *testing.T) {
	var buf bytes.Buffer
	output.Detail{}.Render(&buf)
	if buf.String() != "" {
		t.Fatalf("empty detail = %q, want empty", buf.String())
	}
}

func TestDetailRenderPlain(t *testing.T) {
	doc := output.Detail{Nodes: []output.Node{
		output.Field("Profile", "local"),
		output.Field("Unsafe", "Bad\x1b[31mName"),
		output.Line("free prose"),
		output.Section("Capabilities",
			output.Node{
				Value: "projects (1 of 2 available)",
				Children: []output.Node{
					output.Field("projects.read", "Read\tprojects\n(available)"),
				},
			},
		),
		output.Node{
			Label: "Project scope",
			Value: "selected",
			Children: []output.Node{
				output.Field("Selected count", "5"),
				output.Guidance("Note:\tcapped\nsummary"),
			},
		},
		output.Field("Empty", ""),
	}}

	var data, prose bytes.Buffer
	doc.RenderPlain(&data, &prose)
	wantData := "" +
		"Profile\tlocal\n" +
		"Unsafe\tBad Name\n" +
		"free prose\n" +
		"Capabilities\tprojects (1 of 2 available)\n" +
		"Capabilities.projects.read\tRead projects (available)\n" +
		"Project scope\tselected\n" +
		"Project scope.Selected count\t5\n" +
		"Empty\t\n"
	if data.String() != wantData {
		t.Fatalf("data = %q, want %q", data.String(), wantData)
	}
	if got, want := prose.String(), "Note: capped summary\n"; got != want {
		t.Fatalf("prose = %q, want %q", got, want)
	}
}

func TestWriteHumanRedactsAndWritesOnce(t *testing.T) {
	var writer countingWriter
	err := output.WriteHuman(&writer, func(w io.Writer) {
		_, _ = io.WriteString(w, "Key: ak_test|super-secret\n")
	})
	if err != nil {
		t.Fatalf("WriteHuman() error = %v", err)
	}
	if writer.writes != 1 {
		t.Fatalf("writes = %d, want 1", writer.writes)
	}
	if got, want := writer.String(), "Key: ak_test|[REDACTED]\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderHumanSanitizesWhenRequested(t *testing.T) {
	got := output.RenderHuman(output.TerminalMode{Sanitize: true}, func(w io.Writer, _ output.TerminalMode) {
		_, _ = io.WriteString(w, "Team: Bad\x1b[31mName\n")
		_, _ = io.WriteString(w, "Key: ak_test|super-secret\n")
	})
	want := "Team: Bad Name\nKey: ak_test|[REDACTED]\n"
	if string(got) != want {
		t.Fatalf("human = %q, want %q", string(got), want)
	}
}

func TestSanitizeControlBytes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "csi", in: "A\x1b[31m B", want: "A B"},
		{name: "osc bel", in: "A\x1b]0;title\a B", want: "A B"},
		{name: "osc st", in: "A\x1b]0;title\x1b\\ B", want: "A B"},
		{name: "dcs st", in: "A\x1bPdata\x1b\\ B", want: "A B"},
		{name: "dcs unicode payload", in: "A\x1bPÜberdata\x1b\\ B", want: "A B"},
		{name: "generic escape", in: "A\x1b(B C", want: "A C"},
		{name: "plain controls", in: "A\x00\x01 B\x7f C", want: "A B C"},
		{name: "crlf and carriage return", in: "A\r\nB\r C", want: "A\nB C"},
		{name: "tabs newlines and utf8", in: "A\tB\nCafé 東京", want: "A\tB\nCafé 東京"},
		{name: "c1 byte", in: "A\x80 B", want: "A B"},
		{name: "utf8 c1 control", in: "A\u0085 B", want: "A B"},
		{name: "utf8 c1 csi", in: "A\u009b31m B", want: "A B"},
		{name: "utf8 c1 osc st", in: "A\u009d0;title\u009c B", want: "A B"},
		{name: "utf8 c1 osc unicode payload", in: "A\u009d0;Übertitle\u009c B", want: "A B"},
		{name: "following space collapse", in: "A\x1b[31m B", want: "A B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(output.SanitizeControlBytes([]byte(tt.in))); got != tt.want {
				t.Fatalf("SanitizeControlBytes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeInlineText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "record delimiters", in: "A\tB\nC\r\nD", want: "A B C D"},
		{name: "terminal controls", in: "A\x1b[31m B\a C", want: "A B C"},
		{name: "repeated delimiters", in: "A\t\n\r\n B", want: "A B"},
		{name: "printable unicode", in: "Café 東京", want: "Café 東京"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := output.SanitizeInlineText(tt.in); got != tt.want {
				t.Fatalf("SanitizeInlineText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderHumanAndWritePlainRedact(t *testing.T) {
	human := output.RenderHuman(output.TerminalMode{Color: true, ANSI: true}, func(w io.Writer, mode output.TerminalMode) {
		if !mode.Color || !mode.ANSI {
			t.Fatalf("mode = %#v, want color and ansi", mode)
		}
		_, _ = io.WriteString(w, "Key: ak_test|super-secret\n")
	})
	if got, want := string(human), "Key: ak_test|[REDACTED]\n"; got != want {
		t.Fatalf("human = %q, want %q", got, want)
	}

	var data, prose countingWriter
	err := output.WritePlain(&data, &prose, func(data, prose io.Writer) {
		_, _ = io.WriteString(data, "Key\tak_test|super-secret\n")
		_, _ = io.WriteString(data, "Unsafe\tBad\x1b[31mName\n")
		_, _ = io.WriteString(prose, "Hint ak_test|super-secret\n")
		_, _ = io.WriteString(prose, "Unsafe\x1b[31mHint\n")
	})
	if err != nil {
		t.Fatalf("WritePlain() error = %v", err)
	}
	if data.writes != 1 || prose.writes != 1 {
		t.Fatalf("writes data/prose = %d/%d, want 1/1", data.writes, prose.writes)
	}
	if got, want := data.String(), "Key\tak_test|[REDACTED]\nUnsafe\tBad Name\n"; got != want {
		t.Fatalf("data = %q, want %q", got, want)
	}
	if got, want := prose.String(), "Hint ak_test|[REDACTED]\nUnsafe Hint\n"; got != want {
		t.Fatalf("prose = %q, want %q", got, want)
	}
}

type countingWriter struct {
	bytes.Buffer
	writes int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.Buffer.Write(p)
}
