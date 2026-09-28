package output_test

import (
	"bytes"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
)

func TestMutationSummaryRender(t *testing.T) {
	summary := output.MutationSummary{
		Action:   "Created",
		Resource: "project",
		Name:     "Demo",
		Fields: []output.Node{
			output.Field("ID", "01HY0000000000000000000000"),
			output.Field("Status", "active"),
		},
		Notes: []output.Node{
			output.Field("Replayed", "true"),
		},
	}
	var buf bytes.Buffer
	summary.Render(&buf)
	want := "" +
		"Created project Demo\n" +
		"  ID: 01HY0000000000000000000000\n" +
		"  Status: active\n" +
		"Replayed: true\n"
	if buf.String() != want {
		t.Fatalf("MutationSummary.Render() = %q, want %q", buf.String(), want)
	}
}

func TestMutationSummaryOmitsEmptyHeadlineParts(t *testing.T) {
	var buf bytes.Buffer
	output.MutationSummary{
		Action:   "Updated",
		Resource: "project",
		Fields:   []output.Node{output.Field("ID", "p1")},
	}.Render(&buf)
	if got, want := buf.String(), "Updated project\n  ID: p1\n"; got != want {
		t.Fatalf("MutationSummary.Render() = %q, want %q", got, want)
	}
}

func TestMutationSummaryRenderPlain(t *testing.T) {
	summary := output.MutationSummary{
		Action:   "Created",
		Resource: "project",
		Name:     "Demo",
		Fields: []output.Node{
			output.Field("ID", "01HY"),
			output.Field("Status", "active"),
		},
		Notes: []output.Node{
			output.Guidance("Review in the web app."),
			output.Field("Replayed", "true"),
		},
	}
	var data, prose bytes.Buffer
	summary.RenderPlain(&data, &prose)
	wantData := "" +
		"Created\tproject Demo\n" +
		"ID\t01HY\n" +
		"Status\tactive\n" +
		"Replayed\ttrue\n"
	if data.String() != wantData {
		t.Fatalf("data = %q, want %q", data.String(), wantData)
	}
	if got, want := prose.String(), "Review in the web app.\n"; got != want {
		t.Fatalf("prose = %q, want %q", got, want)
	}
}

func TestMutationSummaryRenderPlainBareHeadline(t *testing.T) {
	var data, prose bytes.Buffer
	output.MutationSummary{Action: "Deleted"}.RenderPlain(&data, &prose)
	if got, want := data.String(), "Deleted\n"; got != want {
		t.Fatalf("data = %q, want %q", got, want)
	}
	if prose.String() != "" {
		t.Fatalf("prose = %q, want empty", prose.String())
	}
}
