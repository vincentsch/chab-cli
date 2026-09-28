package output

import (
	"bytes"
	"io"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// TerminalMode describes output-layer terminal capabilities available to human
// renderers. The final human boundary always removes control bytes; the fields
// are retained for renderers that need to decide whether to emit styling.
type TerminalMode struct {
	Color    bool
	ANSI     bool
	Sanitize bool
}

// RenderHuman buffers the renderer's output and applies redaction
// defense-in-depth.
func RenderHuman(mode TerminalMode, render func(io.Writer, TerminalMode)) []byte {
	return renderHumanWithTransform(mode, render, redact.Bytes)
}

// renderHumanWithTransform applies the byte transform before sanitization so
// redactors can still see the renderer's original bytes, including any control
// bytes that will be removed later.
func renderHumanWithTransform(mode TerminalMode, render func(io.Writer, TerminalMode), transform func([]byte) []byte) []byte {
	if render == nil {
		return nil
	}
	var buf bytes.Buffer
	render(&buf, mode)
	out := buf.Bytes()
	if transform != nil {
		out = transform(out)
	}
	return sanitizeControlBytes(out)
}

// PlainBytes buffers copy-safe data and prose streams separately, then applies
// Chab regex redaction and control-byte sanitization.
func PlainBytes(render func(data, prose io.Writer)) (data, prose []byte) {
	var dataBuf, proseBuf bytes.Buffer
	if render != nil {
		render(&dataBuf, &proseBuf)
	}
	return sanitizeControlBytes(redact.Bytes(dataBuf.Bytes())), sanitizeControlBytes(redact.Bytes(proseBuf.Bytes()))
}

// WritePlain buffers copy-safe data and prose streams separately, redacts
// both, then writes data to stdout and prose/guidance to stderr.
func WritePlain(data, prose io.Writer, render func(data, prose io.Writer)) error {
	dataOut, proseOut := PlainBytes(render)
	if _, err := data.Write(dataOut); err != nil {
		return err
	}
	if _, err := prose.Write(proseOut); err != nil {
		return err
	}
	return nil
}

// WriteHuman preserves the simple helper shape for callers/tests that only need
// default undecorated human output.
func WriteHuman(w io.Writer, render func(io.Writer)) error {
	buf := RenderHuman(TerminalMode{}, func(w io.Writer, _ TerminalMode) {
		if render != nil {
			render(w)
		}
	})
	_, err := w.Write(buf)
	return err
}
