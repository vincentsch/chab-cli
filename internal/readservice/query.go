package readservice

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// Getter is the API read surface needed by project and credits operations.
type Getter interface {
	Get(ctx context.Context, path string, query url.Values, out any) (api.ResponseMeta, error)
}

// WhoamiGetter is the API read surface needed by identity operations.
type WhoamiGetter interface {
	Whoami(ctx context.Context) (api.WhoamiData, api.ResponseMeta, error)
}

// ProjectUsageError reports local project read validation failures.
type ProjectUsageError struct {
	Detail string
}

func (e *ProjectUsageError) Error() string {
	if e == nil {
		return "project command error"
	}
	return "project command error: " + e.Detail
}

func (e *ProjectUsageError) ExitCode() int {
	return 1
}

// CreditsUsageError reports local credits read validation failures.
type CreditsUsageError struct {
	Detail string
}

func (e *CreditsUsageError) Error() string {
	if e == nil {
		return "credits command error"
	}
	return "credits command error: " + e.Detail
}

func (e *CreditsUsageError) ExitCode() int {
	return 1
}

// ProjectListOptions are the project list filters and size controls.
type ProjectListOptions struct {
	Page CursorPaginationOptions
}

// ProjectListQuery validates project list filters and maps them to API query
// names. Chab project list accepts only cursor pagination today, so there are
// no command-owned filters.
func ProjectListQuery(opts ProjectListOptions) (url.Values, error) {
	return url.Values{}, nil
}

// ProjectListPlan resolves cursor project list pagination.
func ProjectListPlan(_ config.Runtime, opts ProjectListOptions) (cmdutil.CursorListPlan, error) {
	input := opts.Page
	if input.Limit.Set && input.Limit.Value < 1 {
		return cmdutil.CursorListPlan{}, &ProjectUsageError{Detail: "--limit must be at least 1"}
	}
	if input.PageSize.Set && (input.PageSize.Value < 1 || input.PageSize.Value > 100) {
		return cmdutil.CursorListPlan{}, &ProjectUsageError{Detail: "--page-size must be between 1 and 100"}
	}
	return cmdutil.CursorListPlan{
		All:      input.All.Set && input.All.Value,
		Limit:    input.Limit.Value,
		LimitSet: input.Limit.Set,
		Cursor:   input.Cursor.Value,
		PageSize: input.PageSize.Value,
	}, nil
}

// FetchProjectList executes a project list plan with a caller-supplied query.
func FetchProjectList(ctx context.Context, client Getter, plan cmdutil.CursorListPlan, baseQuery url.Values) (cmdutil.ListOutcome[Project], error) {
	return cmdutil.FetchCursorPages(plan, func(cursor string, limit int) ([]Project, api.ResponseMeta, error) {
		q := CloneValues(baseQuery)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		var rows []Project
		meta, err := client.Get(ctx, api.Path("projects"), q, &rows)
		if err != nil {
			return nil, api.ResponseMeta{}, err
		}
		return rows, meta, nil
	})
}

// GetProject fetches one project by opaque id.
func GetProject(ctx context.Context, client Getter, id string) (Project, api.ResponseMeta, error) {
	if id == "" {
		return Project{}, api.ResponseMeta{}, &ProjectUsageError{Detail: "project id must not be empty"}
	}
	var wrapped struct {
		Project *Project `json:"project"`
	}
	meta, err := client.Get(ctx, api.Path("projects", id), nil, &wrapped)
	if err != nil {
		return Project{}, meta, err
	}
	if wrapped.Project == nil {
		return Project{}, meta, &api.ProtocolError{Detail: "project response missing data.project", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return *wrapped.Project, meta, nil
}

// ValidProjectStatus reports whether status is a locally known project status.
func ValidProjectStatus(status string) bool {
	switch status {
	case "active", "paused", "archived":
		return true
	default:
		return false
	}
}

// TransactionListOptions are credits transaction filters and size controls.
type TransactionListOptions struct {
	Page CursorPaginationOptions
}

// TransactionQuery validates transaction filters and maps them to API query
// names.
func TransactionQuery(opts TransactionListOptions) (url.Values, error) {
	return url.Values{}, nil
}

// TransactionListPlan resolves credits transaction pagination. Absence uses
// the API's first page default.
func TransactionListPlan(opts TransactionListOptions) (cmdutil.CursorListPlan, error) {
	input := opts.Page
	if input.Limit.Set && input.Limit.Value < 1 {
		return cmdutil.CursorListPlan{}, &CreditsUsageError{Detail: "--limit must be at least 1"}
	}
	if input.PageSize.Set && (input.PageSize.Value < 1 || input.PageSize.Value > 100) {
		return cmdutil.CursorListPlan{}, &CreditsUsageError{Detail: "--page-size must be between 1 and 100"}
	}
	return cmdutil.CursorListPlan{
		All:      input.All.Set && input.All.Value,
		Limit:    input.Limit.Value,
		LimitSet: input.Limit.Set,
		Cursor:   input.Cursor.Value,
		PageSize: input.PageSize.Value,
	}, nil
}

// FetchTransactions executes a credits transaction list plan.
func FetchTransactions(ctx context.Context, client Getter, plan cmdutil.CursorListPlan, baseQuery url.Values) (cmdutil.ListOutcome[Transaction], error) {
	return cmdutil.FetchCursorPages(plan, func(cursor string, limit int) ([]Transaction, api.ResponseMeta, error) {
		q := CloneValues(baseQuery)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		var rows []Transaction
		meta, err := client.Get(ctx, "credits/transactions", q, &rows)
		if err != nil {
			return nil, api.ResponseMeta{}, err
		}
		return rows, meta, nil
	})
}

// GetBalance fetches the team credits balance.
func GetBalance(ctx context.Context, client Getter) (Balance, api.ResponseMeta, error) {
	var data Balance
	meta, err := client.Get(ctx, "credits", nil, &data)
	return data, meta, err
}

// GetWhoami fetches the authenticated identity projection.
func GetWhoami(ctx context.Context, client WhoamiGetter) (Whoami, api.ResponseMeta, error) {
	data, meta, err := client.Whoami(ctx)
	if err != nil {
		return Whoami{}, api.ResponseMeta{}, err
	}
	return WhoamiProjection(data), meta, nil
}

// GetMCPWhoami fetches the safer identity projection for local MCP tools.
func GetMCPWhoami(ctx context.Context, client WhoamiGetter) (MCPWhoami, api.ResponseMeta, error) {
	data, meta, err := client.Whoami(ctx)
	if err != nil {
		return MCPWhoami{}, api.ResponseMeta{}, err
	}
	return MCPWhoamiProjection(data), meta, nil
}

// ValidTransactionType reports whether value is a locally known transaction type.
func ValidTransactionType(value string) bool {
	switch value {
	case "credit_added", "credit_spent", "debt", "adjustment", "other":
		return true
	default:
		return false
	}
}

// ValidTransactionSort reports whether value is a locally known sort value.
func ValidTransactionSort(value string) bool {
	switch value {
	case "created_at", "-created_at":
		return true
	default:
		return false
	}
}

// ParseTransactionTimestamp enforces the strict timestamp contract used by
// credits transaction filters.
func ParseTransactionTimestamp(flag, value string) (string, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil || strings.ContainsAny(value, ".,") || !validTransactionTimestampOffset(value) {
		return "", &CreditsUsageError{Detail: fmt.Sprintf(
			"%s must be a second-precision RFC3339 timestamp like 2026-05-19T10:15:30Z or 2026-05-19T12:15:30+02:00", flag)}
	}
	return t.Format(time.RFC3339), nil
}

func validTransactionTimestampOffset(value string) bool {
	if strings.HasSuffix(value, "Z") {
		return true
	}
	if len(value) < 6 {
		return false
	}
	offset := value[len(value)-6:]
	if offset[0] != '+' && offset[0] != '-' {
		return false
	}
	if offset[3] != ':' {
		return false
	}
	if !twoDigits(offset[1], offset[2]) || !twoDigits(offset[4], offset[5]) {
		return false
	}
	hour := int(offset[1]-'0')*10 + int(offset[2]-'0')
	minute := int(offset[4]-'0')*10 + int(offset[5]-'0')
	return hour <= 23 && minute <= 59
}

func twoDigits(a, b byte) bool {
	return a >= '0' && a <= '9' && b >= '0' && b <= '9'
}

// CloneValues deep-copies url.Values before request-local mutations.
func CloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, set := range values {
		cloned[key] = append([]string(nil), set...)
	}
	return cloned
}

func paginationInput(opts PaginationOptions) cmdutil.PaginationInput {
	return cmdutil.PaginationInput{
		Limit:      opts.Limit.Value,
		LimitSet:   opts.Limit.Set,
		Page:       opts.Page.Value,
		PageSet:    opts.Page.Set,
		PerPage:    opts.PerPage.Value,
		PerPageSet: opts.PerPage.Set,
		All:        opts.All.Value,
		AllSet:     opts.All.Set,
	}
}
