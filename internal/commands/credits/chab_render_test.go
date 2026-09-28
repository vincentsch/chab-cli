package credits

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

func TestChabCreditsRenderUsesBackendFields(t *testing.T) {
	expires := "2026-11-14T10:00:00+00:00"
	nodes := balanceDetailNodes(readservice.Balance{SpendableBalance: 980, Debt: 10, Expires: &expires})
	var buf bytes.Buffer
	output.Detail{Nodes: nodes}.RenderPlain(&buf, &buf)
	text := buf.String()
	for _, want := range []string{"spendable_balance\t980", "debt\t10", "expires\t2026-11-14T10:00:00+00:00"} {
		if !strings.Contains(text, want) {
			t.Fatalf("plain balance output missing %q in:\n%s", want, text)
		}
	}

	operationID := "op_123"
	table := transactionTable([]transactionJSON{{
		ID:                        "ctx_1",
		Kind:                      "usage",
		Amount:                    -10,
		ResultingSpendableBalance: 980,
		Description:               "Credits were used.",
		OccurredAt:                "2026-08-16T10:00:00+00:00",
		OperationID:               &operationID,
	}})
	var rows bytes.Buffer
	table.RenderPlain(&rows, &rows)
	for _, want := range []string{"ctx_1", "usage", "-10", "980", "op_123"} {
		if !strings.Contains(rows.String(), want) {
			t.Fatalf("transaction output missing %q in:\n%s", want, rows.String())
		}
	}
}

func TestGuestBalanceShowsPromotionalCreditsWithoutRelabelingPaidBalance(t *testing.T) {
	var balance readservice.Balance
	if err := json.Unmarshal([]byte(`{"spendable_balance":0,"debt":0,"expires":null,"free_credits":{"available":75,"held":5,"remaining":80}}`), &balance); err != nil {
		t.Fatal(err)
	}
	if balance.SpendableBalance != 0 || balance.FreeCredits == nil || balance.FreeCredits.Available != 75 {
		t.Fatalf("guest balance = %#v", balance)
	}
	var rendered bytes.Buffer
	output.Detail{Nodes: balanceDetailNodes(balance)}.RenderPlain(&rendered, &rendered)
	if !strings.Contains(rendered.String(), "free_credits_available\t75") || !strings.Contains(rendered.String(), "spendable_balance\t0") {
		t.Fatalf("guest balance output = %s", rendered.String())
	}
}
