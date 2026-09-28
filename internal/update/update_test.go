package update

import (
	"errors"
	"strings"
	"testing"
)

func TestEvaluateStatuses(t *testing.T) {
	tests := []struct {
		name            string
		local           string
		releases        []Release
		wantStatus      string
		wantLatest      string
		wantCurrent     string
		wantIsRelease   bool
		wantUpdate      bool
		wantReleasePath string
	}{
		{
			name:            "up to date",
			local:           "0.6.0",
			releases:        []Release{{Tag: "v0.6.0"}},
			wantStatus:      StatusUpToDate,
			wantLatest:      "0.6.0",
			wantCurrent:     "0.6.0",
			wantIsRelease:   true,
			wantReleasePath: "/releases/tag/v0.6.0",
		},
		{
			name:            "update available from unsorted input",
			local:           "0.6.0",
			releases:        []Release{{Tag: "v0.7.0"}, {Tag: "v0.6.0"}, {Tag: "v0.5.0"}},
			wantStatus:      StatusUpdateAvailable,
			wantLatest:      "0.7.0",
			wantCurrent:     "0.6.0",
			wantIsRelease:   true,
			wantUpdate:      true,
			wantReleasePath: "/releases/tag/v0.7.0",
		},
		{
			name:            "newer than latest",
			local:           "0.9.0",
			releases:        []Release{{Tag: "v0.7.0"}},
			wantStatus:      StatusNewerThanLatest,
			wantLatest:      "0.7.0",
			wantCurrent:     "0.9.0",
			wantIsRelease:   true,
			wantReleasePath: "/releases/tag/v0.7.0",
		},
		{
			name:            "development build",
			local:           "dev",
			releases:        []Release{{Tag: "v0.7.0"}},
			wantStatus:      StatusDevelopmentBuild,
			wantLatest:      "0.7.0",
			wantCurrent:     "dev",
			wantReleasePath: "/releases/tag/v0.7.0",
		},
		{
			name:            "leading v local version",
			local:           "v0.6.0",
			releases:        []Release{{Tag: "v0.6.0"}},
			wantStatus:      StatusUpToDate,
			wantLatest:      "0.6.0",
			wantCurrent:     "0.6.0",
			wantIsRelease:   true,
			wantReleasePath: "/releases/tag/v0.6.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.local, tt.releases)
			if err != nil {
				t.Fatalf("Evaluate returned error: %v", err)
			}
			if got.Source != Source {
				t.Fatalf("Source = %q, want %q", got.Source, Source)
			}
			if got.Repository != Repository {
				t.Fatalf("Repository = %q, want %q", got.Repository, Repository)
			}
			if got.CurrentVersion != tt.wantCurrent {
				t.Fatalf("CurrentVersion = %q, want %q", got.CurrentVersion, tt.wantCurrent)
			}
			if got.LatestVersion != tt.wantLatest {
				t.Fatalf("LatestVersion = %q, want %q", got.LatestVersion, tt.wantLatest)
			}
			if got.CurrentIsRelease != tt.wantIsRelease {
				t.Fatalf("CurrentIsRelease = %v, want %v", got.CurrentIsRelease, tt.wantIsRelease)
			}
			if got.UpdateAvailable != tt.wantUpdate {
				t.Fatalf("UpdateAvailable = %v, want %v", got.UpdateAvailable, tt.wantUpdate)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if !strings.HasPrefix(got.ReleaseURL, ReleaseWebURL) || !strings.HasSuffix(got.ReleaseURL, tt.wantReleasePath) {
				t.Fatalf("ReleaseURL = %q, want %s suffix on %s", got.ReleaseURL, tt.wantReleasePath, ReleaseWebURL)
			}
		})
	}
}

func TestEvaluateFiltersUnstableOrInvalidReleases(t *testing.T) {
	releases := []Release{
		{Tag: "v9.0.0", Draft: true},
		{Tag: "v8.0.0", Prerelease: true},
		{Tag: "v0.7.0-rc1"},
		{Tag: "v0.7.0+build"},
		{Tag: "not-a-version"},
		{Tag: "v0.6.0"},
	}
	got, err := Evaluate("0.6.0", releases)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if got.Status != StatusUpToDate {
		t.Fatalf("Status = %q, want %q", got.Status, StatusUpToDate)
	}
	if got.LatestVersion != "0.6.0" {
		t.Fatalf("LatestVersion = %q, want 0.6.0", got.LatestVersion)
	}
	if got.ReleaseURL != ReleaseWebURL+"/releases/tag/v0.6.0" {
		t.Fatalf("ReleaseURL = %q", got.ReleaseURL)
	}
}

func TestEvaluateNoUsableRelease(t *testing.T) {
	_, err := Evaluate("0.6.0", []Release{
		{Tag: "v0.7.0", Draft: true},
		{Tag: "v0.6.0", Prerelease: true},
		{Tag: "v0.6.0-rc1"},
		{Tag: "garbage"},
	})
	if !errors.Is(err, ErrNoUsableRelease) {
		t.Fatalf("err = %v, want ErrNoUsableRelease", err)
	}
}
