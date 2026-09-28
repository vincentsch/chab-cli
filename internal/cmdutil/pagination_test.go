package cmdutil_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

func TestResolveListPlanModesAndDefaults(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		defaultMode  cmdutil.DefaultListMode
		defaultLimit int
		want         cmdutil.ListPlan
	}{
		{
			name:         "resolved default limit",
			defaultMode:  cmdutil.DefaultResolvedLimit,
			defaultLimit: 37,
			want:         cmdutil.ListPlan{Mode: cmdutil.ListModeLimit, Limit: 37, DefaultMode: cmdutil.DefaultResolvedLimit},
		},
		{
			name:        "first API page default",
			defaultMode: cmdutil.DefaultFirstAPIPage,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeExactPage, Page: 1, DefaultMode: cmdutil.DefaultFirstAPIPage},
		},
		{
			name:        "page only",
			args:        []string{"--page", "4"},
			defaultMode: cmdutil.DefaultResolvedLimit,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeExactPage, Page: 4, DefaultMode: cmdutil.DefaultResolvedLimit},
		},
		{
			name:        "per page only",
			args:        []string{"--per-page", "25"},
			defaultMode: cmdutil.DefaultResolvedLimit,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeExactPage, Page: 1, PerPage: 25, DefaultMode: cmdutil.DefaultResolvedLimit},
		},
		{
			name:        "page and per page",
			args:        []string{"--page", "3", "--per-page", "100"},
			defaultMode: cmdutil.DefaultFirstAPIPage,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeExactPage, Page: 3, PerPage: 100, DefaultMode: cmdutil.DefaultFirstAPIPage},
		},
		{
			name:        "limit",
			args:        []string{"--limit", "101"},
			defaultMode: cmdutil.DefaultFirstAPIPage,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeLimit, Limit: 101, DefaultMode: cmdutil.DefaultFirstAPIPage},
		},
		{
			name:        "all",
			args:        []string{"--all"},
			defaultMode: cmdutil.DefaultResolvedLimit,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeAll, DefaultMode: cmdutil.DefaultResolvedLimit},
		},
		{
			name:         "explicit all false uses default",
			args:         []string{"--all=false"},
			defaultMode:  cmdutil.DefaultResolvedLimit,
			defaultLimit: 37,
			want:         cmdutil.ListPlan{Mode: cmdutil.ListModeLimit, Limit: 37, DefaultMode: cmdutil.DefaultResolvedLimit},
		},
		{
			name:        "explicit all false permits limit",
			args:        []string{"--all=false", "--limit", "3"},
			defaultMode: cmdutil.DefaultFirstAPIPage,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeLimit, Limit: 3, DefaultMode: cmdutil.DefaultFirstAPIPage},
		},
		{
			name:        "explicit all false permits exact page",
			args:        []string{"--all=false", "--page", "2"},
			defaultMode: cmdutil.DefaultResolvedLimit,
			want:        cmdutil.ListPlan{Mode: cmdutil.ListModeExactPage, Page: 2, DefaultMode: cmdutil.DefaultResolvedLimit},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePlan(tt.args, tt.defaultMode, tt.defaultLimit)
			if err != nil {
				t.Fatalf("ResolveListPlan() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveListPlan() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveListPlanRejectsConflictsAndNumericBounds(t *testing.T) {
	maxInt := strconv.Itoa(math.MaxInt)
	valid := [][]string{
		{"--page", "1"},
		{"--page", maxInt},
		{"--per-page", "1"},
		{"--per-page", "100"},
		{"--limit", "1"},
		{"--limit", maxInt},
	}
	for _, args := range valid {
		if _, err := resolvePlan(args, cmdutil.DefaultResolvedLimit, 25); err != nil {
			t.Fatalf("ResolveListPlan(%v) error = %v", args, err)
		}
	}

	invalid := [][]string{
		{"--all", "--limit", "1"},
		{"--all", "--page", "1"},
		{"--all", "--per-page", "10"},
		{"--limit", "1", "--page", "1"},
		{"--limit", "1", "--per-page", "10"},
		{"--all", "--limit", "1", "--page", "1", "--per-page", "10"},
		{"--page", "0"},
		{"--page", "-1"},
		{"--per-page", "0"},
		{"--per-page", "101"},
		{"--limit", "0"},
		{"--limit", "-1"},
	}
	for _, args := range invalid {
		if _, err := resolvePlan(args, cmdutil.DefaultResolvedLimit, 25); err == nil {
			t.Fatalf("ResolveListPlan(%v) error = nil", args)
		} else {
			var flagErr *cmdutil.ListFlagError
			if !errors.As(err, &flagErr) {
				t.Fatalf("ResolveListPlan(%v) error type = %T, want ListFlagError", args, err)
			}
		}
	}
}

func TestFetchPagesStrategiesAndStops(t *testing.T) {
	t.Run("exact page preserves omitted per page", func(t *testing.T) {
		var calls [][2]int
		outcome, err := cmdutil.FetchPages(cmdutil.ListPlan{Mode: cmdutil.ListModeExactPage, Page: 4}, func(page, perPage int) ([]int, api.ResponseMeta, error) {
			calls = append(calls, [2]int{page, perPage})
			return []int{4}, api.ResponseMeta{RequestID: "exact"}, nil
		})
		if err != nil || !reflect.DeepEqual(calls, [][2]int{{4, 0}}) || !reflect.DeepEqual(outcome.Rows, []int{4}) {
			t.Fatalf("FetchPages() = %#v, calls=%v, err=%v", outcome, calls, err)
		}
	})

	t.Run("all pages uses API cap", func(t *testing.T) {
		var calls [][2]int
		outcome, err := cmdutil.FetchPages(cmdutil.ListPlan{Mode: cmdutil.ListModeAll}, func(page, perPage int) ([]int, api.ResponseMeta, error) {
			calls = append(calls, [2]int{page, perPage})
			if page == 1 {
				return make([]int, 100), api.ResponseMeta{Pagination: &api.Pagination{HasMore: true}}, nil
			}
			return []int{101}, api.ResponseMeta{Pagination: &api.Pagination{HasMore: false}}, nil
		})
		if err != nil || !reflect.DeepEqual(calls, [][2]int{{1, 100}, {2, 100}}) || len(outcome.Rows) != 101 {
			t.Fatalf("FetchPages() rows=%d calls=%v err=%v", len(outcome.Rows), calls, err)
		}
	})

	for _, limit := range []int{100, 101} {
		t.Run("limit "+strconv.Itoa(limit), func(t *testing.T) {
			var calls [][2]int
			outcome, err := cmdutil.FetchPages(cmdutil.ListPlan{Mode: cmdutil.ListModeLimit, Limit: limit}, func(page, perPage int) ([]int, api.ResponseMeta, error) {
				calls = append(calls, [2]int{page, perPage})
				return make([]int, 100), api.ResponseMeta{Pagination: &api.Pagination{HasMore: true}}, nil
			})
			wantCalls := 1
			if limit == 101 {
				wantCalls = 2
			}
			if err != nil || len(calls) != wantCalls || len(outcome.Rows) != limit {
				t.Fatalf("FetchPages(limit=%d) rows=%d calls=%v err=%v", limit, len(outcome.Rows), calls, err)
			}
			for _, call := range calls {
				if call[1] != 100 {
					t.Fatalf("per page = %d, want 100", call[1])
				}
			}
		})
	}

	stops := []struct {
		name string
		meta api.ResponseMeta
		rows []int
	}{
		{name: "missing pagination", rows: []int{1}},
		{name: "zero progress", rows: []int{}, meta: api.ResponseMeta{Pagination: &api.Pagination{HasMore: true}}},
	}
	for _, tt := range stops {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			outcome, err := cmdutil.FetchPages(cmdutil.ListPlan{Mode: cmdutil.ListModeAll}, func(page, perPage int) ([]int, api.ResponseMeta, error) {
				calls++
				return tt.rows, tt.meta, nil
			})
			if err != nil || calls != 1 {
				t.Fatalf("FetchPages() calls=%d err=%v", calls, err)
			}
			if outcome.Rows == nil {
				t.Fatal("FetchPages() returned nil rows")
			}
		})
	}
}

func TestFetchPagesDiscardsPartialOutcomeOnError(t *testing.T) {
	wantErr := errors.New("page failed")
	calls := 0
	outcome, err := cmdutil.FetchPages(cmdutil.ListPlan{Mode: cmdutil.ListModeAll}, func(page, perPage int) ([]int, api.ResponseMeta, error) {
		calls++
		if calls == 1 {
			return []int{1}, api.ResponseMeta{Pagination: &api.Pagination{HasMore: true}}, nil
		}
		return nil, api.ResponseMeta{}, wantErr
	})
	if !errors.Is(err, wantErr) || calls != 2 || outcome.Rows != nil || outcome.Meta.RequestID != "" {
		t.Fatalf("FetchPages() = %#v, calls=%d, err=%v", outcome, calls, err)
	}
}

func TestFetchPagesAggregatesAttemptsAndKeepsFinalMetadata(t *testing.T) {
	waits1 := []time.Duration{time.Second}
	waits2 := []time.Duration{2 * time.Second, 3 * time.Second}
	retryWait1 := 4 * time.Second
	retryWait2 := 5 * time.Second
	limit := int64(50)
	remaining := int64(20)
	reset := int64(1770000000)
	replayed1 := false
	replayed2 := true
	finalRawMeta := map[string]json.RawMessage{"cursor": json.RawMessage(`"final"`)}
	calls := 0
	outcome, err := cmdutil.FetchPages(cmdutil.ListPlan{Mode: cmdutil.ListModeAll}, func(page, perPage int) ([]int, api.ResponseMeta, error) {
		calls++
		if page == 1 {
			return []int{1}, api.ResponseMeta{
				RequestID:          "first",
				HeaderRequestID:    "first-header",
				EnvelopeRequestID:  "first-envelope",
				HTTPStatus:         200,
				RetryAfter:         api.RetryAfter{Wait: &retryWait1, Raw: "4"},
				IdempotencyUsed:    false,
				IdempotentReplayed: &replayed1,
				Attempts:           2,
				RetryWaits:         waits1,
				Pagination:         &api.Pagination{CurrentPage: 1, HasMore: true},
				RawMeta:            map[string]json.RawMessage{"cursor": json.RawMessage(`"first"`)},
			}, nil
		}
		return []int{2}, api.ResponseMeta{
			RequestID:         "final",
			HeaderRequestID:   "final-header",
			EnvelopeRequestID: "final-envelope",
			HTTPStatus:        206,
			RateLimit: api.RateLimit{
				Limit:            &limit,
				Remaining:        &remaining,
				Reset:            &reset,
				RawLimit:         "50",
				RawRemaining:     "20",
				RawReset:         "1770000000",
				LimitPresent:     true,
				RemainingPresent: true,
				ResetPresent:     true,
			},
			RetryAfter:         api.RetryAfter{Wait: &retryWait2, Raw: "5", Present: true},
			IdempotencyUsed:    true,
			IdempotentReplayed: &replayed2,
			Attempts:           3,
			RetryWaits:         waits2,
			Pagination:         &api.Pagination{CurrentPage: 2, HasMore: false},
			RawMeta:            finalRawMeta,
		}, nil
	})
	if err != nil {
		t.Fatalf("FetchPages() error = %v", err)
	}
	if calls != 2 || outcome.Meta.RequestID != "final" || outcome.Meta.HeaderRequestID != "final-header" || outcome.Meta.EnvelopeRequestID != "final-envelope" || outcome.Meta.HTTPStatus != 206 || outcome.Meta.Pagination == nil || outcome.Meta.Pagination.CurrentPage != 2 {
		t.Fatalf("final metadata = %#v", outcome.Meta)
	}
	if outcome.Meta.RateLimit.Limit != &limit || outcome.Meta.RateLimit.Remaining != &remaining || outcome.Meta.RateLimit.Reset != &reset || outcome.Meta.RateLimit.RawLimit != "50" || outcome.Meta.RateLimit.RawRemaining != "20" || outcome.Meta.RateLimit.RawReset != "1770000000" ||
		!outcome.Meta.RateLimit.LimitPresent || !outcome.Meta.RateLimit.RemainingPresent || !outcome.Meta.RateLimit.ResetPresent {
		t.Fatalf("final rate-limit metadata = %#v", outcome.Meta.RateLimit)
	}
	if outcome.Meta.RetryAfter.Wait != &retryWait2 || outcome.Meta.RetryAfter.Raw != "5" || !outcome.Meta.RetryAfter.Present || !outcome.Meta.IdempotencyUsed || outcome.Meta.IdempotentReplayed != &replayed2 || !reflect.DeepEqual(outcome.Meta.RawMeta, finalRawMeta) {
		t.Fatalf("final response metadata = %#v", outcome.Meta)
	}
	if outcome.Meta.Attempts != 5 || !reflect.DeepEqual(outcome.Meta.RetryWaits, []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}) {
		t.Fatalf("aggregate metadata = %#v", outcome.Meta)
	}
}

func TestFetchCursorPagesRejectsMissingContinuationMetadata(t *testing.T) {
	next := "cursor-2"
	calls := 0
	outcome, err := cmdutil.FetchCursorPages(cmdutil.CursorListPlan{All: true}, func(cursor string, limit int) ([]int, api.ResponseMeta, error) {
		calls++
		if calls == 1 {
			if cursor != "" {
				t.Fatalf("first cursor = %q, want empty", cursor)
			}
			return []int{1}, api.ResponseMeta{
				RequestID:        "first",
				CursorPagination: &api.CursorPagination{Limit: 25, HasMore: true, NextCursor: &next},
			}, nil
		}
		if cursor != next {
			t.Fatalf("second cursor = %q, want %q", cursor, next)
		}
		return []int{2}, api.ResponseMeta{RequestID: "second"}, nil
	})
	var protoErr *api.ProtocolError
	if !errors.As(err, &protoErr) || calls != 2 || outcome.Rows != nil {
		t.Fatalf("FetchCursorPages() = %#v, calls=%d, err=%T %v", outcome, calls, err, err)
	}
}

func TestPaginationHint(t *testing.T) {
	tests := []struct {
		name          string
		mode          cmdutil.ListMode
		shown         int
		requestedPage int
		pagination    *api.Pagination
		want          string
	}{
		{name: "all suppressed", mode: cmdutil.ListModeAll, shown: 1, pagination: &api.Pagination{HasMore: true}},
		{name: "missing metadata", mode: cmdutil.ListModeLimit, shown: 1},
		{name: "exact current page known total", mode: cmdutil.ListModeExactPage, shown: 3, requestedPage: 9, pagination: &api.Pagination{CurrentPage: 2, Total: 10, HasMore: true}, want: "Page 2: showing 3 of 10"},
		{name: "exact requested page unknown total", mode: cmdutil.ListModeExactPage, shown: 3, requestedPage: 9, pagination: &api.Pagination{HasMore: true}, want: "Page 9: showing 3"},
		{name: "exact nothing omitted", mode: cmdutil.ListModeExactPage, shown: 3, requestedPage: 1, pagination: &api.Pagination{CurrentPage: 1, Total: 3}},
		{name: "limit known total", mode: cmdutil.ListModeLimit, shown: 3, pagination: &api.Pagination{Total: 10, HasMore: true}, want: "Showing 3 of 10 - use --all to fetch all"},
		{name: "limit unknown total", mode: cmdutil.ListModeLimit, shown: 3, pagination: &api.Pagination{HasMore: true}, want: "Showing 3 - use --all to fetch all"},
		{name: "limit reached total", mode: cmdutil.ListModeLimit, shown: 10, pagination: &api.Pagination{Total: 10}},
		{name: "limit no more unknown", mode: cmdutil.ListModeLimit, shown: 3, pagination: &api.Pagination{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cmdutil.PaginationHint(tt.mode, tt.shown, tt.requestedPage, tt.pagination); got != tt.want {
				t.Fatalf("PaginationHint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func resolvePlan(args []string, defaultMode cmdutil.DefaultListMode, defaultLimit int) (cmdutil.ListPlan, error) {
	cmd := &cobra.Command{Use: "list"}
	var flags cmdutil.PaginationFlags
	cmdutil.RegisterPaginationFlags(cmd, &flags)
	if err := cmd.ParseFlags(args); err != nil {
		return cmdutil.ListPlan{}, err
	}
	return cmdutil.ResolveListPlan(cmd, &flags, defaultMode, defaultLimit)
}
