package cmdutil

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
)

// apiMaxPerPage mirrors the public API cap and keeps callers from asking the
// server for larger pages while fetching --limit or --all results.
const apiMaxPerPage = 100

// ListMode describes the paging strategy a list command should use.
type ListMode int

const (
	// ListModeDefault is only a defensive placeholder; ResolveListPlan should
	// resolve every valid invocation to one of the concrete modes below.
	ListModeDefault ListMode = iota
	// ListModeAll fetches pages until the API says there are no more.
	ListModeAll
	// ListModeLimit fetches up to a maximum total row count across pages.
	ListModeLimit
	// ListModeExactPage fetches exactly one API page.
	ListModeExactPage
)

// DefaultListMode lets each list command choose how no explicit size flag is
// resolved.
type DefaultListMode int

const (
	// DefaultResolvedLimit makes "no size flag" behave like --limit using a
	// command-specific resolved config value.
	DefaultResolvedLimit DefaultListMode = iota
	// DefaultFirstAPIPage makes "no size flag" fetch the first API-default page.
	DefaultFirstAPIPage
)

// PaginationFlags is the shared local flag target for paginated list commands.
type PaginationFlags struct {
	Limit   int
	Page    int
	PerPage int
	All     bool
}

// CursorPaginationFlags is the public Chab cursor-pagination flag target.
type CursorPaginationFlags struct {
	Limit    int
	All      bool
	Cursor   string
	PageSize int
}

// PaginationInput is the flag-independent form of PaginationFlags. Set fields
// distinguish omitted options from explicit zero, empty, or false values.
type PaginationInput struct {
	Limit      int
	LimitSet   bool
	Page       int
	PageSet    bool
	PerPage    int
	PerPageSet bool
	All        bool
	AllSet     bool
}

// ListFlagError reports local list flag validation failures.
type ListFlagError struct {
	Detail string
}

func (e *ListFlagError) Error() string {
	if e == nil {
		return "invalid list options"
	}
	return "invalid list options: " + e.Detail
}

func (e *ListFlagError) ExitCode() int {
	return 1
}

// ListPlan is the resolved pagination strategy for one command invocation.
type ListPlan struct {
	Mode    ListMode
	Page    int
	PerPage int
	Limit   int
	// DefaultMode records how the caller resolved the no-explicit-size case.
	// FetchPages does not need it, but tests and future callers can inspect the
	// plan without re-reading command flags.
	DefaultMode DefaultListMode
}

// CursorListPlan is the resolved strategy for cursor-paginated Chab lists.
type CursorListPlan struct {
	All      bool
	Limit    int
	LimitSet bool
	Cursor   string
	PageSize int
}

// ListOutcome contains the fetched rows plus metadata needed by human hints.
type ListOutcome[T any] struct {
	Rows []T
	Mode ListMode
	Page int
	Meta api.ResponseMeta
}

// RegisterPaginationFlags binds shared list-size flags as local command flags.
func RegisterPaginationFlags(cmd *cobra.Command, pf *PaginationFlags) {
	flags := cmd.Flags()
	flags.IntVar(&pf.Limit, "limit", 0, "maximum total number of items to fetch across pages")
	flags.BoolVar(&pf.All, "all", false, "fetch every page")
	flags.IntVar(&pf.Page, "page", 0, "fetch one exact page number")
	flags.IntVar(&pf.PerPage, "per-page", 0, "items per page (1-100) for exact page mode")
}

// RegisterCursorPaginationFlags binds Chab cursor list-size flags.
func RegisterCursorPaginationFlags(cmd *cobra.Command, pf *CursorPaginationFlags) {
	flags := cmd.Flags()
	flags.IntVar(&pf.Limit, "limit", 0, "maximum total number of items to fetch")
	flags.BoolVar(&pf.All, "all", false, "fetch every cursor page")
	flags.StringVar(&pf.Cursor, "cursor", "", "opaque cursor returned by the API")
	flags.IntVar(&pf.PageSize, "page-size", 0, "items per API request (1-100)")
}

// ResolveListPlan validates the parsed flags and resolves the active list mode.
func ResolveListPlan(cmd *cobra.Command, pf *PaginationFlags, defaultMode DefaultListMode, resolvedDefaultLimit int) (ListPlan, error) {
	flags := cmd.Flags()
	return ResolveListPlanFromInput(PaginationInput{
		Limit:      pf.Limit,
		LimitSet:   flags.Changed("limit"),
		Page:       pf.Page,
		PageSet:    flags.Changed("page"),
		PerPage:    pf.PerPage,
		PerPageSet: flags.Changed("per-page"),
		All:        pf.All,
		AllSet:     flags.Changed("all"),
	}, defaultMode, resolvedDefaultLimit)
}

// ResolveListPlanFromInput validates parsed pagination values and resolves the
// active list mode without depending on Cobra flag state.
func ResolveListPlanFromInput(input PaginationInput, defaultMode DefaultListMode, resolvedDefaultLimit int) (ListPlan, error) {
	// Cobra marks --all=false as changed, so the parsed Boolean value—not the
	// Changed bit alone—decides whether all-pages mode is enabled.
	allSet := input.AllSet && input.All
	// Page and per-page are one combined "exact page" mode, so they can be used
	// together but not mixed with --all or --limit.
	exactSet := input.PageSet || input.PerPageSet

	if input.PerPageSet && (input.PerPage < 1 || input.PerPage > apiMaxPerPage) {
		return ListPlan{}, &ListFlagError{Detail: "--per-page must be between 1 and 100"}
	}
	if input.PageSet && input.Page < 1 {
		return ListPlan{}, &ListFlagError{Detail: "--page must be at least 1"}
	}
	if input.LimitSet && input.Limit < 1 {
		return ListPlan{}, &ListFlagError{Detail: "--limit must be at least 1"}
	}
	if (allSet && input.LimitSet) || (allSet && exactSet) || (input.LimitSet && exactSet) {
		return ListPlan{}, &ListFlagError{Detail: "use only one of --all, --limit, or --page/--per-page"}
	}

	plan := ListPlan{DefaultMode: defaultMode}
	switch {
	case allSet:
		plan.Mode = ListModeAll
	case exactSet:
		plan.Mode = ListModeExactPage
		if input.PageSet {
			plan.Page = input.Page
		} else {
			plan.Page = 1
		}
		if input.PerPageSet {
			plan.PerPage = input.PerPage
		}
	case input.LimitSet:
		plan.Mode = ListModeLimit
		plan.Limit = input.Limit
	default:
		// Commands choose their own no-size default. Project list uses the
		// configured default limit, while later commands can keep the API's first
		// page behavior without duplicating flag validation.
		switch defaultMode {
		case DefaultResolvedLimit:
			plan.Mode = ListModeLimit
			plan.Limit = resolvedDefaultLimit
		case DefaultFirstAPIPage:
			plan.Mode = ListModeExactPage
			plan.Page = 1
		default:
			plan.Mode = ListModeDefault
		}
	}
	return plan, nil
}

// ResolveCursorListPlan validates cursor-pagination flags.
func ResolveCursorListPlan(cmd *cobra.Command, pf *CursorPaginationFlags) (CursorListPlan, error) {
	flags := cmd.Flags()
	limitSet := flags.Changed("limit")
	allSet := flags.Changed("all") && pf.All
	pageSizeSet := flags.Changed("page-size")

	if limitSet && pf.Limit < 1 {
		return CursorListPlan{}, &ListFlagError{Detail: "--limit must be at least 1"}
	}
	if pageSizeSet && (pf.PageSize < 1 || pf.PageSize > apiMaxPerPage) {
		return CursorListPlan{}, &ListFlagError{Detail: "--page-size must be between 1 and 100"}
	}
	return CursorListPlan{
		All:      allSet,
		Limit:    pf.Limit,
		LimitSet: limitSet,
		Cursor:   pf.Cursor,
		PageSize: pf.PageSize,
	}, nil
}

// FetchPages runs the resolved plan against a caller-owned typed page fetcher.
func FetchPages[T any](plan ListPlan, fetch func(page, perPage int) ([]T, api.ResponseMeta, error)) (ListOutcome[T], error) {
	rows := make([]T, 0)
	outcome := ListOutcome[T]{Rows: rows, Mode: plan.Mode, Page: plan.Page}
	var finalMeta api.ResponseMeta
	var totalAttempts int
	var allWaits []time.Duration

	// Each page response owns the final-response fields for that page
	// (request id, rate-limit headers, pagination). The aggregate keeps the
	// last page's fields, then overwrites attempts/waits with the whole loop's
	// physical request totals.
	recordMeta := func(meta api.ResponseMeta) {
		finalMeta = meta
		totalAttempts += meta.Attempts
		allWaits = append(allWaits, meta.RetryWaits...)
	}

	switch plan.Mode {
	case ListModeExactPage:
		got, meta, err := fetch(plan.Page, plan.PerPage)
		if err != nil {
			return ListOutcome[T]{}, err
		}
		recordMeta(meta)
		outcome.Rows = append(rows, got...)
		finalMeta.Attempts = totalAttempts
		finalMeta.RetryWaits = allWaits
		outcome.Meta = finalMeta
		return outcome, nil
	case ListModeAll:
		page := 1
		for {
			got, meta, err := fetch(page, apiMaxPerPage)
			if err != nil {
				return ListOutcome[T]{}, err
			}
			recordMeta(meta)
			rows = append(rows, got...)
			pg := meta.PaginationForTraversal()
			// Missing pagination means "single page" for first-class commands.
			// len(got)==0 is a guard against a buggy API that keeps has_more true
			// without returning progress.
			if pg == nil || !pg.HasMore || len(got) == 0 {
				break
			}
			page++
		}
	case ListModeLimit:
		perPage := min(plan.Limit, apiMaxPerPage)
		page := 1
		for len(rows) < plan.Limit {
			got, meta, err := fetch(page, perPage)
			if err != nil {
				return ListOutcome[T]{}, err
			}
			recordMeta(meta)
			rows = append(rows, got...)
			pg := meta.PaginationForTraversal()
			// Stop on the server's last page, missing pagination, or lack of
			// progress. The final slice below enforces the caller's total limit if
			// the last fetched page overfilled it.
			if pg == nil || !pg.HasMore || len(got) == 0 {
				break
			}
			page++
		}
		if len(rows) > plan.Limit {
			rows = rows[:plan.Limit]
		}
	default:
		return ListOutcome[T]{}, &ListFlagError{Detail: "no list mode resolved"}
	}

	finalMeta.Attempts = totalAttempts
	finalMeta.RetryWaits = allWaits
	outcome.Rows = rows
	outcome.Meta = finalMeta
	return outcome, nil
}

// FetchCursorPages runs a cursor plan against a typed Chab page fetcher.
func FetchCursorPages[T any](plan CursorListPlan, fetch func(cursor string, limit int) ([]T, api.ResponseMeta, error)) (ListOutcome[T], error) {
	rows := make([]T, 0)
	outcome := ListOutcome[T]{Rows: rows}
	var finalMeta api.ResponseMeta
	var totalAttempts int
	var allWaits []time.Duration

	recordMeta := func(meta api.ResponseMeta) {
		finalMeta = meta
		totalAttempts += meta.Attempts
		allWaits = append(allWaits, meta.RetryWaits...)
	}

	cursor := plan.Cursor
	seenCursors := map[string]bool{}
	if cursor != "" {
		seenCursors[cursor] = true
	}
	for {
		requestLimit := plan.PageSize
		if requestLimit == 0 && plan.LimitSet {
			requestLimit = min(plan.Limit-len(rows), apiMaxPerPage)
		}
		if requestLimit < 0 {
			requestLimit = 0
		}
		got, meta, err := fetch(cursor, requestLimit)
		if err != nil {
			return ListOutcome[T]{}, err
		}
		recordMeta(meta)
		rows = append(rows, got...)

		if plan.LimitSet && len(rows) >= plan.Limit {
			rows = rows[:plan.Limit]
			break
		}
		pg := meta.CursorForTraversal()
		if !plan.All {
			break
		}
		if pg == nil {
			if cursor != "" {
				return ListOutcome[T]{}, cursorTraversalProtocolError(meta, "cursor-paginated continuation response omitted pagination metadata")
			}
			break
		}
		if !pg.HasMore {
			break
		}
		if pg.NextCursor == nil || *pg.NextCursor == "" {
			return ListOutcome[T]{}, cursorTraversalProtocolError(meta, "cursor-paginated response advertised more results without a next_cursor")
		}
		if len(got) == 0 {
			return ListOutcome[T]{}, cursorTraversalProtocolError(meta, "cursor-paginated response advertised more results without returning rows")
		}
		nextCursor := *pg.NextCursor
		if seenCursors[nextCursor] {
			return ListOutcome[T]{}, cursorTraversalProtocolError(meta, "cursor-paginated response repeated a cursor")
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}

	finalMeta.Attempts = totalAttempts
	finalMeta.RetryWaits = allWaits
	outcome.Rows = rows
	outcome.Meta = finalMeta
	return outcome, nil
}

func cursorTraversalProtocolError(meta api.ResponseMeta, detail string) error {
	status := meta.HTTPStatus
	if status == 0 {
		status = http.StatusOK
	}
	return &api.ProtocolError{
		Detail:    detail,
		Status:    status,
		RequestID: meta.RequestID,
		Meta:      meta,
	}
}

// PaginationHint returns the human pagination hint line, or an empty string.
func PaginationHint(mode ListMode, shown, requestedPage int, pg *api.Pagination) string {
	if mode == ListModeAll || pg == nil {
		return ""
	}

	if mode == ListModeExactPage {
		page := pg.CurrentPage
		if page == 0 {
			// Some endpoints may omit current_page while still returning useful
			// pagination metadata; in that case the requested page is the clearest
			// label for the human hint.
			page = requestedPage
		}
		omitted := pg.HasMore || pg.Total > shown
		if !omitted {
			return ""
		}
		if pg.Total > 0 {
			return fmt.Sprintf("Page %d: showing %d of %d", page, shown, pg.Total)
		}
		return fmt.Sprintf("Page %d: showing %d", page, shown)
	}

	if pg.Total > 0 {
		if shown >= pg.Total {
			return ""
		}
		return fmt.Sprintf("Showing %d of %d - use --all to fetch all", shown, pg.Total)
	}
	if pg.HasMore {
		return fmt.Sprintf("Showing %d - use --all to fetch all", shown)
	}
	return ""
}

// CursorPaginationHint returns the human hint for cursor-paginated lists.
func CursorPaginationHint(plan CursorListPlan, shown int, pg *api.CursorPagination) string {
	if plan.All || pg == nil || !pg.HasMore {
		return ""
	}
	if shown == 0 {
		return "More results are available - use --all to fetch all"
	}
	return fmt.Sprintf("Showing %d - use --all to fetch all", shown)
}
