package credits

import (
	"net/url"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

// buildTransactionQuery validates changed filter flags and maps them to the API
// query names. It is intentionally pure so runTransactions can call it before
// runtime resolution, credential lookup, or HTTP setup.
func buildTransactionQuery(cmd *cobra.Command, flags *transactionsFlags) (url.Values, error) {
	query, err := readservice.TransactionQuery(transactionListOptions(cmd, flags))
	if err != nil {
		return nil, creditsUsageError(err)
	}
	return query, nil
}

func validTransactionType(value string) bool {
	return readservice.ValidTransactionType(value)
}

func validTransactionSort(value string) bool {
	return readservice.ValidTransactionSort(value)
}

// parseTransactionTimestamp enforces the stricter timestamp contract used by
// the API filters. Go's RFC3339 parser accepts fractional seconds and can
// normalize impossible numeric offsets, so those checks must inspect the
// original string in addition to parsing it.
func parseTransactionTimestamp(flag, value string) (string, error) {
	got, err := readservice.ParseTransactionTimestamp(flag, value)
	if err != nil {
		return "", creditsUsageError(err)
	}
	return got, nil
}

// cloneValues deep-copies url.Values before the page loop adds page/per_page.
// The map and its slices are mutable, and each request must start from the same
// filter set without leaking pagination keys into the next iteration.
func cloneValues(values url.Values) url.Values {
	return readservice.CloneValues(values)
}

func transactionListOptions(cmd *cobra.Command, flags *transactionsFlags) readservice.TransactionListOptions {
	return readservice.TransactionListOptions{
		Page: readservice.CursorPaginationOptions{
			Limit:    readservice.OptionalInt{Value: flags.Page.Limit, Set: cmd.Flags().Changed("limit")},
			All:      readservice.OptionalBool{Value: flags.Page.All, Set: cmd.Flags().Changed("all")},
			Cursor:   readservice.OptionalString{Value: flags.Page.Cursor, Set: cmd.Flags().Changed("cursor")},
			PageSize: readservice.OptionalInt{Value: flags.Page.PageSize, Set: cmd.Flags().Changed("page-size")},
		},
	}
}

func creditsUsageError(err error) error {
	if local, ok := err.(*readservice.CreditsUsageError); ok {
		return &usageError{detail: local.Detail}
	}
	return err
}
