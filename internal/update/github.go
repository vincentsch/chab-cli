package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.github.com"

// DefaultFetcher resolves real GitHub releases with a bounded timeout.
func DefaultFetcher() Fetcher {
	return NewGitHubFetcher(DefaultBaseURL, 10*time.Second)
}

// NewGitHubFetcher returns a Fetcher hitting baseURL; tests pass an httptest URL.
func NewGitHubFetcher(baseURL string, timeout time.Duration) Fetcher {
	client := &http.Client{Timeout: timeout}
	url := strings.TrimRight(baseURL, "/") + "/repos/" + Repository + "/releases?per_page=30"
	return func(ctx context.Context) ([]Release, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "chab-version-check")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("contact GitHub releases: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("GitHub releases returned status %d", resp.StatusCode)
		}
		var releases []Release
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&releases); err != nil {
			return nil, fmt.Errorf("decode GitHub releases: %w", err)
		}
		// A successful decode of the first JSON value is not enough here:
		// trailing bytes or a second JSON value mean GitHub returned malformed
		// data for this command, and version --check should fail closed.
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			if err != nil {
				return nil, fmt.Errorf("decode GitHub releases: %w", err)
			}
			return nil, fmt.Errorf("decode GitHub releases: trailing data")
		}
		return releases, nil
	}
}
