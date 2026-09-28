package readservice

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

type recordingGetter struct {
	calls []readCall
	pages []transactionPage
}

type readCall struct {
	path  string
	query url.Values
}

type transactionPage struct {
	rows []Transaction
	meta api.ResponseMeta
}

func (g *recordingGetter) Get(_ context.Context, path string, query url.Values, out any) (api.ResponseMeta, error) {
	g.calls = append(g.calls, readCall{path: path, query: CloneValues(query)})
	switch path {
	case "credits":
		return api.ResponseMeta{RequestID: "req-balance"}, roundTrip(out, Balance{SpendableBalance: 980, Debt: 10, Expires: stringPtr("2026-11-14T10:00:00+00:00")})
	case "credits/transactions":
		page := g.pages[0]
		g.pages = g.pages[1:]
		return page.meta, roundTrip(out, page.rows)
	default:
		return api.ResponseMeta{}, nil
	}
}

func TestChabCreditsBalanceAndCursorTransactions(t *testing.T) {
	getter := &recordingGetter{
		pages: []transactionPage{
			{
				rows: []Transaction{{ID: "ctx_1", Kind: "usage", Amount: -10, ResultingSpendableBalance: 990, Description: "Used.", OccurredAt: "2026-08-16T10:00:00+00:00"}},
				meta: api.ResponseMeta{CursorPagination: &api.CursorPagination{Limit: 2, HasMore: true, NextCursor: stringPtr("next"), PrevCursor: nil}},
			},
			{
				rows: []Transaction{{ID: "ctx_2", Kind: "purchase", Amount: 100, ResultingSpendableBalance: 1090, Description: "Added.", OccurredAt: "2026-08-16T11:00:00+00:00"}},
				meta: api.ResponseMeta{CursorPagination: &api.CursorPagination{Limit: 2, HasMore: false, NextCursor: nil, PrevCursor: stringPtr("prev")}},
			},
		},
	}

	balance, meta, err := GetBalance(context.Background(), getter)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.SpendableBalance != 980 || balance.Debt != 10 || balance.Expires == nil || meta.RequestID != "req-balance" {
		t.Fatalf("balance = %#v meta = %#v", balance, meta)
	}

	plan := cmdutil.CursorListPlan{All: true, PageSize: 2}
	outcome, err := FetchTransactions(context.Background(), getter, plan, url.Values{})
	if err != nil {
		t.Fatalf("FetchTransactions: %v", err)
	}
	if got := []string{getter.calls[1].query.Get("limit"), getter.calls[2].query.Get("cursor"), getter.calls[2].query.Get("limit")}; !reflect.DeepEqual(got, []string{"2", "next", "2"}) {
		t.Fatalf("queries = %#v calls = %#v", got, getter.calls)
	}
	if len(outcome.Rows) != 2 || outcome.Rows[0].ID != "ctx_1" || outcome.Rows[1].ID != "ctx_2" {
		t.Fatalf("rows = %#v", outcome.Rows)
	}
}

func roundTrip(out any, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func stringPtr(value string) *string {
	return &value
}
