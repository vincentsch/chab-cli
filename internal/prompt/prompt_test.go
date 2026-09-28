package prompt_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/prompt"
)

func TestTextPrefillSequentialAndCRLF(t *testing.T) {
	var stderr bytes.Buffer
	p := prompt.New(bytes.NewBufferString("\r\nentered\r\n"), &stderr, true)
	first, err := p.Text("Profile", "local")
	if err != nil {
		t.Fatalf("Text(first) error = %v", err)
	}
	second, err := p.Text("Base URL", "http://localhost")
	if err != nil {
		t.Fatalf("Text(second) error = %v", err)
	}
	if first != "local" || second != "entered" {
		t.Fatalf("values = %q, %q", first, second)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Profile [local]: ")) || !bytes.Contains(stderr.Bytes(), []byte("Base URL [http://localhost]: ")) {
		t.Fatalf("prompts = %q", stderr.String())
	}
}

func TestTextEOFBeforeBytes(t *testing.T) {
	p := prompt.New(bytes.NewReader(nil), io.Discard, true)
	if _, err := p.Text("Profile", "local"); !errors.Is(err, io.EOF) {
		t.Fatalf("Text() error = %v, want EOF", err)
	}
}

func TestTextWithDisplayKeepsRealBlankDefault(t *testing.T) {
	var stderr bytes.Buffer
	p := prompt.New(bytes.NewBufferString("\r\n"), &stderr, true)
	value, err := p.TextWithDisplay("Base URL", "https://real.example.test", "[REDACTED]")
	if err != nil {
		t.Fatalf("TextWithDisplay() error = %v", err)
	}
	if value != "https://real.example.test" {
		t.Fatalf("TextWithDisplay() = %q", value)
	}
	if stderr.String() != "Base URL [[REDACTED]]: " {
		t.Fatalf("display prompt = %q", stderr.String())
	}
}

func TestTextWithDisplayFailsBeforeReadingWhenPromptCannotBeWritten(t *testing.T) {
	wantErr := errors.New("prompt output failed")
	var consumed bytes.Buffer
	p := prompt.New(
		io.TeeReader(strings.NewReader("\n"), &consumed),
		promptErrorWriter{err: wantErr},
		true,
	)
	if _, err := p.TextWithDisplay("Base URL", "real", "display"); !errors.Is(err, wantErr) {
		t.Fatalf("TextWithDisplay() error = %v", err)
	}
	if consumed.Len() != 0 {
		t.Fatalf("TextWithDisplay() consumed input after prompt failure")
	}
}

func TestSecretFallbackAndReadPiped(t *testing.T) {
	var stderr bytes.Buffer
	p := prompt.New(bytes.NewBufferString("secret\r\nrest"), &stderr, true)
	value, err := p.Secret("API key")
	if err != nil {
		t.Fatalf("Secret() error = %v", err)
	}
	if value != "secret" {
		t.Fatalf("Secret() = %q", value)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("API key: ")) {
		t.Fatalf("secret prompt missing: %q", stderr.String())
	}

	piped := prompt.New(bytes.NewBufferString("whole\ninput"), io.Discard, false)
	got, err := piped.ReadPiped()
	if err != nil {
		t.Fatalf("ReadPiped() error = %v", err)
	}
	if got != "whole\ninput" {
		t.Fatalf("ReadPiped() = %q", got)
	}
}

func TestIsTerminalReaderFalseForBuffersAndPipes(t *testing.T) {
	if prompt.IsTerminalReader(bytes.NewReader(nil)) {
		t.Fatalf("buffer reported as terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()
	if prompt.IsTerminalReader(r) {
		t.Fatalf("pipe reported as terminal")
	}
}

func TestConfirmGrammar(t *testing.T) {
	tests := []struct {
		name  string
		input string
		def   prompt.Default
		want  bool
	}{
		{name: "lower y", input: "y\n", want: true},
		{name: "lower yes", input: "yes\n", want: true},
		{name: "upper yes", input: "YES\n", want: true},
		{name: "upper y", input: "Y\n", want: true},
		{name: "lower n", input: "n\n", want: false},
		{name: "lower no", input: "no\n", want: false},
		{name: "upper n", input: "N\n", want: false},
		{name: "blank default no", input: "\n", def: prompt.DefaultNo, want: false},
		{name: "blank default yes", input: "\n", def: prompt.DefaultYes, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := prompt.New(bytes.NewBufferString(tt.input), io.Discard, true)
			got, err := p.Confirm("Continue?", tt.def)
			if err != nil {
				t.Fatalf("Confirm() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Confirm() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestConfirmRepromptsAndEOFDenies(t *testing.T) {
	var stderr bytes.Buffer
	p := prompt.New(bytes.NewBufferString("maybe\ny\n"), &stderr, true)
	got, err := p.Confirm("Continue?", prompt.DefaultNo)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if !got {
		t.Fatalf("Confirm() = false, want true")
	}
	if strings.Count(stderr.String(), "Continue? [y/N]: ") != 2 {
		t.Fatalf("prompt count/output = %q", stderr.String())
	}

	stderr.Reset()
	p = prompt.New(bytes.NewReader(nil), &stderr, true)
	got, err = p.Confirm("Continue?", prompt.DefaultNo)
	if err != nil || got {
		t.Fatalf("Confirm(EOF) = %t, %v; want false, nil", got, err)
	}

	stderr.Reset()
	p = prompt.New(bytes.NewBufferString("mayb"), &stderr, true)
	got, err = p.Confirm("Continue?", prompt.DefaultNo)
	if err != nil || got {
		t.Fatalf("Confirm(partial EOF) = %t, %v; want false, nil", got, err)
	}
	if strings.Count(stderr.String(), "Continue? [y/N]: ") != 2 {
		t.Fatalf("partial EOF prompt output = %q", stderr.String())
	}
}

func TestConfirmFailsBeforeReadingWhenPromptCannotBeWritten(t *testing.T) {
	wantErr := errors.New("prompt output failed")
	var consumed bytes.Buffer
	p := prompt.New(
		io.TeeReader(strings.NewReader("yes\n"), &consumed),
		promptErrorWriter{err: wantErr},
		true,
	)

	got, err := p.Confirm("Continue?", prompt.DefaultNo)
	if got || !errors.Is(err, wantErr) {
		t.Fatalf("Confirm() = %t, %v; want false, wrapped writer error", got, err)
	}
	if consumed.Len() != 0 {
		t.Fatalf("Confirm() consumed input after prompt failure: %q", consumed.String())
	}

	t.Run("re-prompt", func(t *testing.T) {
		var consumed bytes.Buffer
		writer := &promptSequenceWriter{failAt: 2, err: wantErr}
		p := prompt.New(
			io.TeeReader(strings.NewReader("maybe\nyes\n"), &consumed),
			writer,
			true,
		)

		got, err := p.Confirm("Continue?", prompt.DefaultNo)
		if got || !errors.Is(err, wantErr) {
			t.Fatalf("Confirm() = %t, %v; want false, wrapped re-prompt writer error", got, err)
		}
		if consumed.String() != "maybe\n" {
			t.Fatalf("Confirm() consumed input past failed re-prompt: %q", consumed.String())
		}
		if writer.writes != 2 {
			t.Fatalf("prompt writes = %d, want 2", writer.writes)
		}
	})
}

func TestConfirmSuffixes(t *testing.T) {
	var stderr bytes.Buffer
	p := prompt.New(bytes.NewBufferString("\n"), &stderr, true)
	if _, err := p.Confirm("Continue?", prompt.DefaultNo); err != nil {
		t.Fatalf("Confirm(DefaultNo) error = %v", err)
	}
	if !strings.Contains(stderr.String(), "Continue? [y/N]: ") {
		t.Fatalf("DefaultNo suffix missing: %q", stderr.String())
	}

	stderr.Reset()
	p = prompt.New(bytes.NewBufferString("\n"), &stderr, true)
	if _, err := p.Confirm("Continue?", prompt.DefaultYes); err != nil {
		t.Fatalf("Confirm(DefaultYes) error = %v", err)
	}
	if !strings.Contains(stderr.String(), "Continue? [Y/n]: ") {
		t.Fatalf("DefaultYes suffix missing: %q", stderr.String())
	}
}

func TestSelect(t *testing.T) {
	var stderr bytes.Buffer
	p := prompt.New(bytes.NewBufferString("2\n"), &stderr, true)
	index, value, err := p.Select("Choose", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if index != 1 || value != "b" {
		t.Fatalf("Select() = %d, %q; want 1, b", index, value)
	}
	if !strings.Contains(stderr.String(), "Choose\n") || !strings.Contains(stderr.String(), "  1) a\n") {
		t.Fatalf("Select() prompt missing menu: %q", stderr.String())
	}

	for _, input := range []string{"0\n2\n", "x\n2\n"} {
		t.Run(input, func(t *testing.T) {
			var stderr bytes.Buffer
			p := prompt.New(bytes.NewBufferString(input), &stderr, true)
			index, value, err := p.Select("Choose", []string{"a", "b"})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if index != 1 || value != "b" {
				t.Fatalf("Select() = %d, %q; want 1, b", index, value)
			}
			if strings.Count(stderr.String(), "Selection [1-2]: ") != 2 {
				t.Fatalf("Select() did not re-prompt: %q", stderr.String())
			}
		})
	}

	p = prompt.New(bytes.NewReader(nil), io.Discard, true)
	index, value, err = p.Select("Choose", []string{"a"})
	if !errors.Is(err, io.EOF) || index != 0 || value != "" {
		t.Fatalf("Select(EOF) = %d, %q, %v; want 0, empty, EOF", index, value, err)
	}

	if _, _, err := p.Select("Choose", nil); err == nil {
		t.Fatalf("Select(empty) error = nil")
	}
}

type promptErrorWriter struct {
	err error
}

func (w promptErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type promptSequenceWriter struct {
	writes int
	failAt int
	err    error
}

func (w *promptSequenceWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return len(p), nil
}
