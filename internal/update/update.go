// Package update owns release lookup evaluation for chab version --check.
package update

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	Source        = "github_release"
	Repository    = "vincentsch/chab-cli"
	ReleaseWebURL = "https://github.com/" + Repository

	StatusUpToDate         = "up_to_date"
	StatusUpdateAvailable  = "update_available"
	StatusDevelopmentBuild = "development_build"
	StatusNewerThanLatest  = "newer_than_latest"
)

// ErrNoUsableRelease means the lookup succeeded but no stable SemVer release
// was found to compare against. The version command maps it to exit 2.
var ErrNoUsableRelease = errors.New("no stable release found for " + Repository)

// Release is the subset of a GitHub release this CLI needs.
type Release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// Result is the update object owned by the release playbook, in documented
// field order.
type Result struct {
	Source           string `json:"source"`
	Repository       string `json:"repository"`
	CurrentVersion   string `json:"current_version"`
	LatestVersion    string `json:"latest_version"`
	CurrentIsRelease bool   `json:"current_is_release"`
	UpdateAvailable  bool   `json:"update_available"`
	Status           string `json:"status"`
	ReleaseURL       string `json:"release_url"`
}

// Fetcher resolves GitHub releases for version --check. Injected so tests cover
// update outcomes without contacting GitHub.
type Fetcher func(ctx context.Context) ([]Release, error)

// Evaluate selects the highest stable release and computes the update result
// for the local version. It returns ErrNoUsableRelease when no stable release
// is present, which the caller maps to exit 2.
func Evaluate(localVersion string, releases []Release) (Result, error) {
	var bestTag, bestDisplay string
	for _, r := range releases {
		// GitHub's list order is not part of the CLI contract, so filter to
		// stable release tags and compute the maximum ourselves.
		if r.Draft || r.Prerelease {
			continue
		}
		display := normalizeDisplay(r.Tag)
		if !isReleaseVersion(display) {
			continue
		}
		if bestTag == "" || semver.Compare(compareToken(display), compareToken(bestDisplay)) > 0 {
			bestTag, bestDisplay = r.Tag, display
		}
	}
	if bestTag == "" {
		return Result{}, ErrNoUsableRelease
	}

	current := normalizeDisplay(localVersion)
	res := Result{
		Source:           Source,
		Repository:       Repository,
		CurrentVersion:   current,
		LatestVersion:    bestDisplay,
		CurrentIsRelease: isReleaseVersion(current),
		ReleaseURL:       ReleaseWebURL + "/releases/tag/" + bestTag,
	}

	switch {
	case !res.CurrentIsRelease:
		// A development build can still report the latest release; it is not an
		// update candidate because there is no stable local version to compare.
		res.Status = StatusDevelopmentBuild
	default:
		switch cmp := semver.Compare(compareToken(current), compareToken(bestDisplay)); {
		case cmp < 0:
			res.Status = StatusUpdateAvailable
			res.UpdateAvailable = true
		case cmp == 0:
			res.Status = StatusUpToDate
		default:
			res.Status = StatusNewerThanLatest
		}
	}
	return res, nil
}

// normalizeDisplay trims exactly one leading "v" for artifact/JSON display.
func normalizeDisplay(v string) string {
	return strings.TrimPrefix(v, "v")
}

// compareToken re-adds exactly one leading "v" for golang.org/x/mod/semver.
func compareToken(display string) string {
	return "v" + display
}

// isReleaseVersion reports whether display is a stable release SemVer.
func isReleaseVersion(display string) bool {
	t := compareToken(display)
	return semver.IsValid(t) && semver.Prerelease(t) == "" && semver.Build(t) == ""
}
