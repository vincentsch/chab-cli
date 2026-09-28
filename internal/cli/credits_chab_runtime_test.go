package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestCreditsCommandsUseChabRuntimeContract(t *testing.T) {
	state, apiKey := configuredChabCreditsState(t)
	expires := "2026-11-14T10:00:00+00:00"
	nextCursor := "next-page"
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/credits":
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"spendable_balance": 980,
					"debt":              10,
					"expires":           expires,
				},
				"request_id": "req-balance",
			})
		case "/v1/credits/transactions":
			cursor := r.URL.Query().Get("cursor")
			rows := []map[string]any{{
				"id":                          "ctx-first",
				"occurred_at":                 "2026-08-16T10:00:00+00:00",
				"kind":                        "usage",
				"amount":                      -10,
				"resulting_spendable_balance": 980,
				"expires_at":                  nil,
				"operation_id":                "op_first",
				"purchase_id":                 nil,
				"description":                 "Credits were used.",
			}}
			meta := map[string]any{
				"limit":       1,
				"has_more":    true,
				"next_cursor": nextCursor,
				"prev_cursor": nil,
			}
			if cursor == nextCursor {
				rows = []map[string]any{{
					"id":                          "ctx-second",
					"occurred_at":                 "2026-08-16T10:05:00+00:00",
					"kind":                        "usage",
					"amount":                      -5,
					"resulting_spendable_balance": 975,
					"expires_at":                  nil,
					"operation_id":                "op_second",
					"purchase_id":                 nil,
					"description":                 "More credits were used.",
				}}
				meta["has_more"] = false
				meta["next_cursor"] = nil
				meta["prev_cursor"] = "prev-page"
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data":       rows,
				"meta":       meta,
				"request_id": "req-transactions",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
	})

	balance := runChabCredits(t, state, server, "credits", "balance", "--json")
	if balance.ExitCode != cli.ExitSuccess || balance.Err != nil || balance.Stderr != "" {
		t.Fatalf("balance result = %#v", balance)
	}
	var balanceBody map[string]any
	if err := json.Unmarshal([]byte(balance.Stdout), &balanceBody); err != nil {
		t.Fatalf("balance JSON error = %v: %s", err, balance.Stdout)
	}
	if balanceBody["spendable_balance"] != float64(980) || balanceBody["debt"] != float64(10) || balanceBody["expires"] != expires {
		t.Fatalf("balance JSON = %#v", balanceBody)
	}

	transactions := runChabCredits(t, state, server, "credits", "transactions", "--all", "--page-size", "1", "--json", "--include-meta")
	if transactions.ExitCode != cli.ExitSuccess || transactions.Err != nil || transactions.Stderr != "" {
		t.Fatalf("transactions result = %#v", transactions)
	}
	var txBody struct {
		Data []struct {
			ID                        string `json:"id"`
			Kind                      string `json:"kind"`
			Amount                    int64  `json:"amount"`
			ResultingSpendableBalance int64  `json:"resulting_spendable_balance"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
			Cursor    struct {
				Limit      int     `json:"limit"`
				HasMore    bool    `json:"has_more"`
				NextCursor *string `json:"next_cursor"`
				PrevCursor *string `json:"prev_cursor"`
			} `json:"cursor"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(transactions.Stdout), &txBody); err != nil {
		t.Fatalf("transactions JSON error = %v: %s", err, transactions.Stdout)
	}
	if len(txBody.Data) != 2 || txBody.Data[0].ID != "ctx-first" || txBody.Data[1].ID != "ctx-second" {
		t.Fatalf("transactions data = %#v", txBody.Data)
	}
	if txBody.Meta.RequestID != "req-transactions" || txBody.Meta.Cursor.HasMore || txBody.Meta.Cursor.PrevCursor == nil || *txBody.Meta.Cursor.PrevCursor != "prev-page" {
		t.Fatalf("transactions meta = %#v", txBody.Meta)
	}

	if server.Count() != 3 {
		t.Fatalf("request count = %d, want 3", server.Count())
	}
	server.AssertPathSegments(t, 0, []string{"v1", "credits"})
	server.AssertPathSegments(t, 1, []string{"v1", "credits", "transactions"})
	server.AssertPathSegments(t, 2, []string{"v1", "credits", "transactions"})
	server.AssertQueryValues(t, 1, "limit", []string{"1"})
	server.AssertQueryValues(t, 1, "cursor", nil)
	server.AssertQueryValues(t, 2, "limit", []string{"1"})
	server.AssertQueryValues(t, 2, "cursor", []string{nextCursor})
	server.AssertNoBody(t, 0)
	server.AssertNoBody(t, 1)
	server.AssertNoBody(t, 2)
	server.AssertBearer(t, apiKey)
}

func TestCreditsTransactionsRejectsInvalidCursorFlagsBeforeRuntime(t *testing.T) {
	state := testutil.NewState(t)
	server := testutil.NewAPIServer(t, testutil.FailOnContact(t))

	result := testutil.RunCommandWith(t, chabCreditsOptions(), state.APIArgs(server, "credits", "transactions", "--page-size", "101")...)
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "--page-size must be between 1 and 100") {
		t.Fatalf("stderr missing validation detail:\n%s", result.Stderr)
	}
	server.AssertNoRequests(t)
}

func TestCreditsTransactionsRejectsIncompleteCursorTraversal(t *testing.T) {
	for _, test := range []struct {
		name     string
		meta     map[string]any
		wantText string
	}{
		{
			name:     "missing next cursor",
			meta:     map[string]any{"limit": 1, "has_more": true, "next_cursor": nil, "prev_cursor": nil},
			wantText: "malformed pagination metadata",
		},
		{
			name:     "repeated cursor",
			meta:     map[string]any{"limit": 1, "has_more": true, "next_cursor": "same-cursor", "prev_cursor": nil},
			wantText: "repeated a cursor",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, _ := configuredChabCreditsState(t)
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != "/v1/credits/transactions" {
					t.Fatalf("unexpected path %s", r.URL.EscapedPath())
				}
				testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
					"data": []map[string]any{{
						"id":                          "ctx-loop",
						"occurred_at":                 "2026-08-16T10:00:00+00:00",
						"kind":                        "usage",
						"amount":                      -10,
						"resulting_spendable_balance": 980,
					}},
					"meta":       test.meta,
					"request_id": "req-cursor",
				})
			})

			result := runChabCredits(t, state, server, "credits", "transactions", "--all", "--page-size", "1", "--json")
			if result.ExitCode != cli.ExitAPI || result.Stdout != "" || result.Err == nil {
				t.Fatalf("result = %#v, want protocol failure", result)
			}
			if !strings.Contains(result.Stderr, test.wantText) {
				t.Fatalf("stderr missing %q:\n%s", test.wantText, result.Stderr)
			}
			if test.name == "missing next cursor" && server.Count() != 1 {
				t.Fatalf("request count = %d, want 1", server.Count())
			}
			if test.name == "repeated cursor" && server.Count() != 2 {
				t.Fatalf("request count = %d, want 2", server.Count())
			}
		})
	}
}

func configuredChabCreditsState(t *testing.T) (testutil.State, string) {
	t.Helper()
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{
		Locale:        "en",
		DefaultOutput: "table",
	}, true)
	key := testutil.FakeKey("credits-chab")
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	return state, key
}

func chabCreditsOptions() testutil.Options {
	return testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}
}

func runChabCredits(t *testing.T, state testutil.State, server *testutil.APIServer, args ...string) testutil.Result {
	t.Helper()
	return testutil.RunCommandWith(t, chabCreditsOptions(), state.APIArgs(server, args...)...)
}
