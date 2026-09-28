package output

import "unicode/utf8"

// sanitizeControlBytes removes terminal-control bytes from output modes that
// promise copy-safe or no-ANSI text. It preserves tabs and newlines because
// plain output uses them as structure.
func sanitizeControlBytes(in []byte) []byte {
	return SanitizeControlBytes(in)
}

// SanitizeControlBytes removes terminal controls and other unsafe control bytes
// while preserving tabs, newlines, and valid printable UTF-8 text.
func SanitizeControlBytes(in []byte) []byte {
	var out []byte
	for i := 0; i < len(in); {
		switch b := in[i]; {
		case b == 0x1b:
			next := consumeEscape(in, i)
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, next)
		case b == 0x9b:
			next := consumeCSI(in, i+1)
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, next)
		case b == 0x9d:
			next := consumeStringControl(in, i+1)
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, next)
		case b == '\t' || b == '\n':
			out = append(out, b)
			i++
		case b == '\r':
			if i+1 < len(in) && in[i+1] == '\n' {
				i++
				continue
			}
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, i+1)
		case b >= utf8.RuneSelf:
			r, size := utf8.DecodeRune(in[i:])
			if r != utf8.RuneError || size > 1 {
				// C1 controls may arrive as valid two-byte UTF-8 instead of raw
				// control bytes, so inspect the decoded rune before copying it.
				if isC1ControlRune(r) {
					next := i + size
					switch r {
					case '\u009b':
						next = consumeCSI(in, next)
					case '\u009d':
						next = consumeStringControl(in, next)
					}
					out = appendSanitizedSeparator(out)
					i = skipFollowingSpace(in, next)
					continue
				}
				out = append(out, in[i:i+size]...)
				i += size
				continue
			}
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, i+1)
		case isControlByte(b):
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, i+1)
		default:
			out = append(out, b)
			i++
		}
	}
	return out
}

// SanitizeInlineText removes terminal controls and collapses record delimiters
// so an untrusted scalar can be embedded in a human or plain output cell.
func SanitizeInlineText(value string) string {
	in := SanitizeControlBytes([]byte(value))
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); {
		switch in[i] {
		case '\t', '\n', '\r':
			out = appendSanitizedSeparator(out)
			i = skipFollowingSpace(in, i+1)
		default:
			out = append(out, in[i])
			i++
		}
	}
	return string(out)
}

// consumeEscape returns the first byte after an ESC sequence. It handles the
// common bracketed forms first, then falls back to generic ESC sequences with
// intermediate bytes such as charset selectors.
func consumeEscape(in []byte, start int) int {
	i := start + 1
	if i >= len(in) {
		return i
	}
	switch in[i] {
	case '[':
		return consumeCSI(in, i+1)
	case ']', 'P', '^', '_', 'X':
		return consumeStringControl(in, i+1)
	default:
		if in[i] >= 0x20 && in[i] <= 0x2f {
			for i < len(in) && in[i] >= 0x20 && in[i] <= 0x2f {
				i++
			}
			if i < len(in) {
				i++
			}
			return i
		}
		return i + 1
	}
}

// consumeCSI returns the first byte after a control sequence introducer. An
// incomplete sequence consumes the rest of the input so partial terminal
// controls are not leaked.
func consumeCSI(in []byte, start int) int {
	for i := start; i < len(in); i++ {
		if in[i] >= 0x40 && in[i] <= 0x7e {
			return i + 1
		}
	}
	return len(in)
}

// consumeStringControl returns the first byte after an OSC/DCS-style string
// control. These controls can end with BEL, ESC\, or the 8-bit ST byte.
func consumeStringControl(in []byte, start int) int {
	for i := start; i < len(in); {
		switch in[i] {
		case 0x07:
			return i + 1
		case 0x1b:
			if i+1 < len(in) && in[i+1] == '\\' {
				return i + 2
			}
		case 0x9c:
			return i + 1
		}
		if in[i] >= utf8.RuneSelf {
			r, size := utf8.DecodeRune(in[i:])
			if r == '\u009c' && size > 1 {
				return i + size
			}
			// Skip a complete valid rune. A continuation byte can equal raw ST,
			// but it is payload data and must not terminate the sequence.
			if r != utf8.RuneError || size > 1 {
				i += size
				continue
			}
		}
		i++
	}
	return len(in)
}

// appendSanitizedSeparator keeps printable runs separated after removing a
// control sequence without turning adjacent removed controls into extra spaces.
func appendSanitizedSeparator(out []byte) []byte {
	if len(out) == 0 {
		return append(out, ' ')
	}
	switch out[len(out)-1] {
	case ' ', '\t', '\n':
		return out
	default:
		return append(out, ' ')
	}
}

// skipFollowingSpace prevents a removed control plus an existing literal space
// from becoming two spaces in the sanitized output.
func skipFollowingSpace(in []byte, i int) int {
	if i < len(in) && in[i] == ' ' {
		return i + 1
	}
	return i
}

// isControlByte reports C0, DEL, and raw C1 bytes that should not reach human
// or copy-safe output.
func isControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f)
}

// isC1ControlRune catches the same C1 range after valid UTF-8 decoding.
func isC1ControlRune(r rune) bool {
	return r >= '\u0080' && r <= '\u009f'
}

func sanitizeControlString(value string) string {
	return string(sanitizeControlBytes([]byte(value)))
}
