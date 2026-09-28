package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
)

func TestTableRender(t *testing.T) {
	table := output.Table{
		Columns: []string{"ID", "Name"},
		Rows: [][]string{
			{"1", "Alpha"},
			{"200", "B"},
		},
	}
	var buf bytes.Buffer
	table.Render(&buf)
	want := "ID   Name\n1    Alpha\n200  B\n"
	if buf.String() != want {
		t.Fatalf("Table.Render() = %q, want %q", buf.String(), want)
	}
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("line has trailing whitespace: %q", line)
		}
	}
}

func TestTableRenderSanitizesHumanCells(t *testing.T) {
	table := output.Table{
		Columns: []string{"ID", "Name\x1b[31m"},
		Rows: [][]string{
			{"1", "Alpha\x1b[31mRed\a"},
		},
	}
	var buf bytes.Buffer
	table.Render(&buf)
	want := "ID  Name\n1   Alpha Red\n"
	if buf.String() != want {
		t.Fatalf("Table.Render() = %q, want %q", buf.String(), want)
	}
}

func TestTableNormalizesShortAndLongRows(t *testing.T) {
	table := output.Table{
		Columns: []string{"A", "B", "C"},
		Rows: [][]string{
			{"x", "yy", "z", "ignored"},
			{"long"},
		},
	}
	var buf bytes.Buffer
	table.Render(&buf)
	want := "A     B   C\nx     yy  z\nlong\n"
	if buf.String() != want {
		t.Fatalf("Table.Render() = %q, want %q", buf.String(), want)
	}
}

func TestTableRenderUsesUTF8ByteWidths(t *testing.T) {
	table := output.Table{
		Columns: []string{"ID", "NAME", "STATUS"},
		Rows: [][]string{
			{"1", "Café", "active"},
			{"22", "東京", "paused"},
		},
	}
	var buf bytes.Buffer
	table.Render(&buf)
	want := "ID  NAME    STATUS\n1   Café   active\n22  東京  paused\n"
	if buf.String() != want {
		t.Fatalf("Table.Render() = %q, want %q", buf.String(), want)
	}
}

func TestTableEmptyState(t *testing.T) {
	var buf bytes.Buffer
	output.Table{Columns: []string{"ID"}, Empty: "No projects found."}.Render(&buf)
	if got, want := buf.String(), "No projects found.\n"; got != want {
		t.Fatalf("empty table = %q, want %q", got, want)
	}
	buf.Reset()
	output.Table{Columns: []string{"ID"}}.Render(&buf)
	if buf.String() != "" {
		t.Fatalf("empty table without message = %q, want empty", buf.String())
	}
}

func TestTableRenderPlain(t *testing.T) {
	table := output.Table{
		Columns: []string{"ID", "Name"},
		Rows: [][]string{
			{"1", "Alpha\tOne"},
			{"200", "B\nTwo"},
		},
	}
	var data, prose bytes.Buffer
	table.RenderPlain(&data, &prose)
	want := "ID\tName\n1\tAlpha One\n200\tB Two\n"
	if data.String() != want {
		t.Fatalf("plain table = %q, want %q", data.String(), want)
	}
	if prose.String() != "" {
		t.Fatalf("prose = %q, want empty", prose.String())
	}
}

func TestTableRenderPlainEmptyStateToProse(t *testing.T) {
	var data, prose bytes.Buffer
	output.Table{Columns: []string{"ID"}, Empty: "No\tprojects\nfound."}.RenderPlain(&data, &prose)
	if data.String() != "" {
		t.Fatalf("data = %q, want empty", data.String())
	}
	if got, want := prose.String(), "No projects found.\n"; got != want {
		t.Fatalf("prose = %q, want %q", got, want)
	}
}
