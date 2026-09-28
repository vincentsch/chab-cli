package api

import (
	"fmt"
)

// debugf writes one redacted debug line when debug output is enabled. The lock
// protects injected writers such as bytes.Buffer during concurrent requests.
func (c *Client) debugf(format string, args ...any) {
	c.debugfWith(c.newRequestRedactor(""), format, args...)
}

func (c *Client) debugfWith(redactor requestRedactor, format string, args ...any) {
	if c == nil || c.debugWriter == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	c.debugMu.Lock()
	defer c.debugMu.Unlock()
	_, _ = fmt.Fprintf(c.debugWriter, "debug: api: %s\n", redactor.redactText(line))
}
