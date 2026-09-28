// Package readservice contains reusable read-model operations shared by Cobra
// commands and local integrations.
package readservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/redact"
)

const SecretVariableName = "CHAB_API_KEY"

// OptionalString distinguishes omitted options from explicit empty strings.
type OptionalString struct {
	Value string
	Set   bool
}

// OptionalInt distinguishes omitted options from explicit zeroes.
type OptionalInt struct {
	Value int
	Set   bool
}

// OptionalBool distinguishes omitted options from explicit false values.
type OptionalBool struct {
	Value bool
	Set   bool
}

// PaginationOptions is the flag-independent pagination input shared by read
// tools and commands.
type PaginationOptions struct {
	Limit   OptionalInt
	All     OptionalBool
	Page    OptionalInt
	PerPage OptionalInt
}

// CursorPaginationOptions is the flag-independent form of Chab cursor paging.
type CursorPaginationOptions struct {
	Limit    OptionalInt
	All      OptionalBool
	Cursor   OptionalString
	PageSize OptionalInt
}

// AuthEnvReport is the sanitized local runtime projection for automation.
type AuthEnvReport struct {
	Profile        string `json:"profile"`
	BaseURL        string `json:"base_url"`
	APIBaseURL     string `json:"api_base_url"`
	Locale         string `json:"locale"`
	SecretVariable string `json:"secret_variable"`
}

// AuthEnv reports non-secret runtime state and never reads credentials.
func AuthEnv(rt config.Runtime) AuthEnvReport {
	return AuthEnvReport{
		Profile:        redact.String(rt.Profile),
		BaseURL:        redact.String(rt.BaseURL),
		APIBaseURL:     redact.String(rt.APIBaseURL),
		Locale:         redact.String(rt.Locale),
		SecretVariable: SecretVariableName,
	}
}

// Whoami is the stable CLI-owned identity projection.
type Whoami = api.WhoamiData

// MCPWhoami is the same non-secret token context exposed through local MCP.
type MCPWhoami = api.WhoamiData

// WhoamiProjection returns Chab's documented flat /me token context.
func WhoamiProjection(data api.WhoamiData) Whoami {
	return data
}

// MCPWhoamiProjection returns the same non-secret token context for MCP.
func MCPWhoamiProjection(data api.WhoamiData) MCPWhoami {
	return data
}

// Project is the stable public project read shape.
type Project struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	URL         *string `json:"url"`
	Status      string  `json:"status"`
	Timezone    string  `json:"timezone"`
	Language    string  `json:"language"`
	Limit       int64   `json:"limit"`
	Automate    bool    `json:"automate"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// UnmarshalJSON keeps project ids opaque even when the backend currently
// serializes numeric database ids, and preserves nullable string fields.
func (p *Project) UnmarshalJSON(data []byte) error {
	var payload struct {
		ID          json.RawMessage `json:"id"`
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		URL         *string         `json:"url"`
		Status      string          `json:"status"`
		Timezone    string          `json:"timezone"`
		Language    string          `json:"language"`
		Limit       int64           `json:"limit"`
		Automate    bool            `json:"automate"`
		CreatedAt   string          `json:"created_at"`
		UpdatedAt   string          `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	id, err := opaqueID(payload.ID)
	if err != nil {
		return err
	}
	p.ID = id
	p.Name = payload.Name
	p.Description = payload.Description
	p.URL = payload.URL
	p.Status = payload.Status
	p.Timezone = payload.Timezone
	p.Language = payload.Language
	p.Limit = payload.Limit
	p.Automate = payload.Automate
	p.CreatedAt = payload.CreatedAt
	p.UpdatedAt = payload.UpdatedAt
	return nil
}

func opaqueID(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", fmt.Errorf("project response field %q must be a string or number", "id")
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text, nil
	}
	var number json.Number
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&number); err == nil {
		if _, err := strconv.ParseFloat(number.String(), 64); err == nil {
			return number.String(), nil
		}
	}
	return "", fmt.Errorf("project response field %q must be a string or number", "id")
}

// Balance is the stable credits balance read shape.
type Balance struct {
	SpendableBalance int64        `json:"spendable_balance"`
	FreeCredits      *FreeCredits `json:"free_credits,omitempty"`
	Debt             int64        `json:"debt"`
	Expires          *string      `json:"expires"`
}

type FreeCredits struct {
	Available int64 `json:"available"`
	Held      int64 `json:"held"`
	Remaining int64 `json:"remaining"`
}

// UnmarshalJSON rejects a missing or null balance while preserving nullable
// optional fields.
func (b *Balance) UnmarshalJSON(data []byte) error {
	var payload struct {
		SpendableBalance *int64       `json:"spendable_balance"`
		Debt             *int64       `json:"debt"`
		Expires          *string      `json:"expires"`
		FreeCredits      *FreeCredits `json:"free_credits"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.SpendableBalance == nil {
		return fmt.Errorf("credit balance response field %q must be an integer", "spendable_balance")
	}
	if payload.Debt == nil {
		return fmt.Errorf("credit balance response field %q must be an integer", "debt")
	}
	b.SpendableBalance = *payload.SpendableBalance
	b.Debt = *payload.Debt
	b.Expires = payload.Expires
	b.FreeCredits = payload.FreeCredits
	return nil
}

// Transaction is the stable public credit transaction read shape.
type Transaction struct {
	ID                        string  `json:"id"`
	OccurredAt                string  `json:"occurred_at"`
	Kind                      string  `json:"kind"`
	Amount                    int64   `json:"amount"`
	ResultingSpendableBalance int64   `json:"resulting_spendable_balance"`
	ExpiresAt                 *string `json:"expires_at"`
	OperationID               *string `json:"operation_id"`
	PurchaseID                *string `json:"purchase_id"`
	Description               string  `json:"description"`
}
