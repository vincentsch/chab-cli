package credits

import (
	"strconv"

	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

type balanceJSON = readservice.Balance
type transactionJSON = readservice.Transaction

// usageError marks local credits-command validation failures as exit code 1.
type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "credits command error"
	}
	return "credits command error: " + e.detail
}

func (e *usageError) ExitCode() int {
	return 1
}

// balanceDetailNodes orders the human detail to match the documented JSON
// shape. expires renders only when present; the JSON shape still carries the
// nullable key.
func balanceDetailNodes(b balanceJSON) []output.Node {
	nodes := []output.Node{
		output.Field("spendable_balance", strconv.FormatInt(b.SpendableBalance, 10)),
		output.Field("debt", strconv.FormatInt(b.Debt, 10)),
	}
	if b.Expires != nil {
		nodes = append(nodes, output.Field("expires", *b.Expires))
	}
	if b.FreeCredits != nil {
		nodes = append(nodes,
			output.Field("free_credits_available", strconv.FormatInt(b.FreeCredits.Available, 10)),
			output.Field("free_credits_held", strconv.FormatInt(b.FreeCredits.Held, 10)),
		)
	}
	return nodes
}

// transactionTable selects the compact columns for transaction list output.
// Display-width alignment inherits output.Table's settled byte-width behavior
// (internal/output/table.go); wide/multi-byte descriptions render intact.
func transactionTable(rows []transactionJSON) output.Table {
	tableRows := make([][]string, 0, len(rows))
	for _, txn := range rows {
		expiresAt := ""
		if txn.ExpiresAt != nil {
			expiresAt = *txn.ExpiresAt
		}
		operationID := ""
		if txn.OperationID != nil {
			operationID = *txn.OperationID
		}
		tableRows = append(tableRows, []string{
			txn.ID,
			txn.Kind,
			strconv.FormatInt(txn.Amount, 10),
			strconv.FormatInt(txn.ResultingSpendableBalance, 10),
			txn.Description,
			txn.OccurredAt,
			expiresAt,
			operationID,
		})
	}
	return output.Table{
		Columns: []string{"ID", "KIND", "AMOUNT", "BALANCE", "DESCRIPTION", "OCCURRED", "EXPIRES", "OPERATION"},
		Rows:    tableRows,
		Empty:   "No transactions found.",
	}
}
