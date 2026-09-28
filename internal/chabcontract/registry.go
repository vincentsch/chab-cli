package chabcontract

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed testdata/chab-v1.yaml testdata/provenance.json
var fixtureFS embed.FS

const (
	v1FixturePath        = "testdata/chab-v1.yaml"
	provenancePath       = "testdata/provenance.json"
	ResultSchemaPrefix   = "#/components/schemas/"
	embeddedSchemaFormat = "json-schema-draft-openapi-fragment"
)

// Operation describes one public Chab operation from the pinned OpenAPI fixture.
type Operation struct {
	ID                  string          `json:"id"`
	Method              string          `json:"method"`
	Path                string          `json:"path"`
	Parameters          []Parameter     `json:"parameters,omitempty"`
	Availability        string          `json:"availability"`
	Behavior            string          `json:"behavior"`
	OutputKind          string          `json:"output_kind"`
	IdempotencyRequired bool            `json:"idempotency_required"`
	DryRunSupported     bool            `json:"dry_run_supported"`
	RequiresPaidPlan    bool            `json:"requires_paid_plan"`
	NotBillable         bool            `json:"not_billable"`
	RequiredScope       string          `json:"required_scope,omitempty"`
	ResultSchemaRef     string          `json:"result_schema_ref,omitempty"`
	RequestSchema       json.RawMessage `json:"request_schema,omitempty"`
	ResultSchema        json.RawMessage `json:"result_schema,omitempty"`
	DryRunResultSchema  json.RawMessage `json:"dry_run_result_schema,omitempty"`
}

// Parameter describes a public path or query parameter from the pinned
// OpenAPI fixture. Header parameters remain transport/local controls.
type Parameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Required    bool            `json:"required"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// Registry is the parsed public operation inventory.
type Registry struct {
	operations []Operation
	byID       map[string]Operation
	schemas    map[string]json.RawMessage
	pathCount  int
	provenance Provenance
}

// Provenance is the checked source metadata for embedded contract fixtures.
type Provenance struct {
	BackendRevision string              `json:"backend_revision"`
	Fixtures        []ProvenanceFixture `json:"fixtures"`
}

// ProvenanceFixture describes one pinned source fixture.
type ProvenanceFixture struct {
	File       string `json:"file"`
	Source     string `json:"source"`
	SHA256     string `json:"sha256"`
	Operations int    `json:"operations,omitempty"`
	Paths      int    `json:"paths,omitempty"`
}

var (
	registryOnce sync.Once
	registryVal  Registry
	registryErr  error
)

// Load parses and validates the embedded pinned public API contract.
func Load() (Registry, error) {
	registryOnce.Do(func() {
		registryVal, registryErr = load()
	})
	return registryVal, registryErr
}

// MustLoad returns Load or panics. It is intended only for static command setup.
func MustLoad() Registry {
	registry, err := Load()
	if err != nil {
		panic(err)
	}
	return registry
}

// Operations returns a sorted copy of every operation.
func (r Registry) Operations() []Operation {
	out := append([]Operation(nil), r.operations...)
	return out
}

// Find returns one operation by operation key.
func (r Registry) Find(id string) (Operation, bool) {
	op, ok := r.byID[id]
	return op, ok
}

// PathCount returns the number of OpenAPI paths in the pinned fixture.
func (r Registry) PathCount() int {
	return r.pathCount
}

// Provenance returns checked fixture provenance.
func (r Registry) Provenance() Provenance {
	p := r.provenance
	p.Fixtures = append([]ProvenanceFixture(nil), p.Fixtures...)
	return p
}

// SchemaDocument is the offline shape shown by command-line schema inspection.
type SchemaDocument struct {
	OperationID         string                     `json:"operation_id"`
	Method              string                     `json:"method"`
	Path                string                     `json:"path"`
	Parameters          []Parameter                `json:"parameters,omitempty"`
	Availability        string                     `json:"availability"`
	Behavior            string                     `json:"behavior"`
	OutputKind          string                     `json:"output_kind"`
	IdempotencyRequired bool                       `json:"idempotency_required"`
	DryRunSupported     bool                       `json:"dry_run_supported"`
	RequiredScope       string                     `json:"required_scope,omitempty"`
	RequestSchemaFormat string                     `json:"request_schema_format"`
	RequestSchema       json.RawMessage            `json:"request_schema,omitempty"`
	ResultSchemaRef     string                     `json:"result_schema_ref,omitempty"`
	ResultSchema        json.RawMessage            `json:"result_schema,omitempty"`
	DryRunResultSchema  json.RawMessage            `json:"dry_run_result_schema,omitempty"`
	ReferencedSchemas   map[string]json.RawMessage `json:"referenced_schemas,omitempty"`
}

// SchemaDocument returns the embedded request and result schema fragments for
// one operation. The fragments are copied so callers can render them safely.
func (r Registry) SchemaDocument(id string) (SchemaDocument, bool) {
	op, ok := r.Find(id)
	if !ok {
		return SchemaDocument{}, false
	}
	return SchemaDocument{
		OperationID:         op.ID,
		Method:              op.Method,
		Path:                op.Path,
		Parameters:          append([]Parameter(nil), op.Parameters...),
		Availability:        op.Availability,
		Behavior:            op.Behavior,
		OutputKind:          op.OutputKind,
		IdempotencyRequired: op.IdempotencyRequired,
		DryRunSupported:     op.DryRunSupported,
		RequiredScope:       op.RequiredScope,
		RequestSchemaFormat: embeddedSchemaFormat,
		RequestSchema:       append(json.RawMessage(nil), op.RequestSchema...),
		ResultSchemaRef:     op.ResultSchemaRef,
		ResultSchema:        append(json.RawMessage(nil), op.ResultSchema...),
		DryRunResultSchema:  append(json.RawMessage(nil), op.DryRunResultSchema...),
		ReferencedSchemas:   r.referencedSchemas(op.RequestSchema, op.ResultSchema, op.DryRunResultSchema),
	}, true
}

func load() (Registry, error) {
	provenance, err := loadProvenance()
	if err != nil {
		return Registry{}, err
	}
	fixture, err := fixtureBytes(v1FixturePath)
	if err != nil {
		return Registry{}, err
	}
	if err := verifyFixture(provenance, v1FixturePath, fixture); err != nil {
		return Registry{}, err
	}

	var doc openAPIDocument
	if err := yaml.Unmarshal(fixture, &doc); err != nil {
		return Registry{}, fmt.Errorf("decode pinned Chab contract: %w", err)
	}
	if len(doc.Paths) == 0 {
		return Registry{}, fmt.Errorf("pinned Chab contract has no paths")
	}
	componentSchemas, err := componentSchemaJSON(doc.Components.Schemas)
	if err != nil {
		return Registry{}, err
	}

	var operations []Operation
	seen := map[string]bool{}
	for path, methods := range doc.Paths {
		for method, raw := range methods {
			if !isHTTPMethod(method) || raw.OperationID == "" {
				continue
			}
			op, err := operationFromDoc(strings.ToUpper(method), path, raw, doc.Components.Schemas)
			if err != nil {
				return Registry{}, err
			}
			if seen[op.ID] {
				return Registry{}, fmt.Errorf("duplicate operation %q in pinned Chab contract", op.ID)
			}
			seen[op.ID] = true
			operations = append(operations, op)
		}
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].ID < operations[j].ID })

	byID := make(map[string]Operation, len(operations))
	for _, op := range operations {
		byID[op.ID] = op
	}
	registry := Registry{
		operations: operations,
		byID:       byID,
		schemas:    componentSchemas,
		pathCount:  len(doc.Paths),
		provenance: provenance,
	}
	if err := verifyCounts(registry); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func componentSchemaJSON(schemas map[string]yaml.Node) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(schemas))
	for name, node := range schemas {
		data, err := schemaJSON(&node)
		if err != nil {
			return nil, fmt.Errorf("component schema %s: %w", name, err)
		}
		out[name] = data
	}
	return out, nil
}

type openAPIDocument struct {
	Paths      map[string]map[string]openAPIOperation `yaml:"paths"`
	Components struct {
		Schemas map[string]yaml.Node `yaml:"schemas"`
	} `yaml:"components"`
}

type openAPIOperation struct {
	OperationID         string      `yaml:"operationId"`
	Availability        string      `yaml:"x-chab-availability"`
	Behavior            string      `yaml:"x-chab-behavior"`
	OutputKind          string      `yaml:"x-chab-output-kind"`
	IdempotencyRequired bool        `yaml:"x-chab-idempotency-required"`
	DryRunSupported     bool        `yaml:"x-chab-dry-run-supported"`
	RequiresPaidPlan    bool        `yaml:"x-chab-requires-paid-plan"`
	NotBillable         bool        `yaml:"x-chab-not-billable"`
	RequiredScope       string      `yaml:"x-chab-required-scope"`
	ResultSchemaRef     string      `yaml:"x-chab-result-schema"`
	Parameters          []parameter `yaml:"parameters"`
	RequestBody         requestBody `yaml:"requestBody"`
	Responses           responses   `yaml:"responses"`
}

type parameter struct {
	Name        string    `yaml:"name"`
	In          string    `yaml:"in"`
	Required    bool      `yaml:"required"`
	Description string    `yaml:"description"`
	Schema      yaml.Node `yaml:"schema"`
}

type requestBody struct {
	Content map[string]mediaType `yaml:"content"`
}

type responses map[string]response

type response struct {
	Content map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema yaml.Node `yaml:"schema"`
}

func operationFromDoc(method, path string, raw openAPIOperation, schemas map[string]yaml.Node) (Operation, error) {
	if raw.Availability == "" || raw.Behavior == "" || raw.OutputKind == "" {
		return Operation{}, fmt.Errorf("%s %s (%s): missing Chab operation metadata", method, path, raw.OperationID)
	}
	parameters, err := parametersFromDoc(raw.Parameters)
	if err != nil {
		return Operation{}, fmt.Errorf("%s %s (%s): parameters: %w", method, path, raw.OperationID, err)
	}
	requestSchema, err := schemaJSON(firstMediaSchema(raw.RequestBody.Content, "application/json", "multipart/form-data"))
	if err != nil {
		return Operation{}, fmt.Errorf("%s %s (%s): request schema: %w", method, path, raw.OperationID, err)
	}
	resultSchema, err := resultSchemaJSON(raw.ResultSchemaRef, schemas)
	if err != nil {
		return Operation{}, fmt.Errorf("%s %s (%s): result schema: %w", method, path, raw.OperationID, err)
	}
	dryRunResultSchema, err := dryRunResultSchemaJSON(raw.DryRunSupported, raw.Responses, schemas)
	if err != nil {
		return Operation{}, fmt.Errorf("%s %s (%s): dry-run result schema: %w", method, path, raw.OperationID, err)
	}
	return Operation{
		ID:                  raw.OperationID,
		Method:              method,
		Path:                path,
		Parameters:          parameters,
		Availability:        raw.Availability,
		Behavior:            raw.Behavior,
		OutputKind:          raw.OutputKind,
		IdempotencyRequired: raw.IdempotencyRequired,
		DryRunSupported:     raw.DryRunSupported,
		RequiresPaidPlan:    raw.RequiresPaidPlan,
		NotBillable:         raw.NotBillable,
		RequiredScope:       raw.RequiredScope,
		ResultSchemaRef:     raw.ResultSchemaRef,
		RequestSchema:       requestSchema,
		ResultSchema:        resultSchema,
		DryRunResultSchema:  dryRunResultSchema,
	}, nil
}

func parametersFromDoc(raw []parameter) ([]Parameter, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]Parameter, 0, len(raw))
	for _, param := range raw {
		if param.In != "path" && param.In != "query" {
			continue
		}
		schema, err := schemaJSON(&param.Schema)
		if err != nil {
			return nil, fmt.Errorf("%s parameter %s: %w", param.In, param.Name, err)
		}
		out = append(out, Parameter{
			Name:        param.Name,
			In:          param.In,
			Required:    param.Required,
			Description: param.Description,
			Schema:      schema,
		})
	}
	return out, nil
}

func firstMediaSchema(content map[string]mediaType, names ...string) *yaml.Node {
	if len(content) == 0 {
		return nil
	}
	for _, name := range names {
		if media, ok := content[name]; ok {
			return &media.Schema
		}
	}
	return nil
}

func mediaSchema(content map[string]mediaType, name string) *yaml.Node {
	if len(content) == 0 {
		return nil
	}
	media, ok := content[name]
	if !ok {
		return nil
	}
	return &media.Schema
}

func schemaJSON(node *yaml.Node) (json.RawMessage, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	value, err := yamlNodeValue(node)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func resultSchemaJSON(ref string, schemas map[string]yaml.Node) (json.RawMessage, error) {
	if ref == "" {
		return nil, nil
	}
	name, ok := strings.CutPrefix(ref, ResultSchemaPrefix)
	if !ok || name == "" {
		return nil, fmt.Errorf("unsupported result schema ref %q", ref)
	}
	node, ok := schemas[name]
	if !ok {
		return nil, nil
	}
	return schemaJSON(&node)
}

func dryRunResultSchemaJSON(supported bool, responses responses, schemas map[string]yaml.Node) (json.RawMessage, error) {
	if !supported {
		return nil, nil
	}
	response, ok := responses["200"]
	if !ok {
		return nil, nil
	}
	schema := firstMediaSchema(response.Content, "application/json")
	dataSchema := mappingValue(mappingValue(schema, "properties"), "data")
	if dataSchema == nil || dataSchema.Kind == 0 {
		return nil, nil
	}
	if ref := schemaRef(dataSchema); ref != "" {
		return resultSchemaJSON(ref, schemas)
	}
	return schemaJSON(dataSchema)
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func schemaRef(node *yaml.Node) string {
	ref := mappingValue(node, "$ref")
	if ref == nil || ref.Kind != yaml.ScalarNode {
		return ""
	}
	return ref.Value
}

func (r Registry) referencedSchemas(roots ...json.RawMessage) map[string]json.RawMessage {
	seen := map[string]bool{}
	for _, root := range roots {
		collectReferences(root, seen)
	}
	if len(seen) == 0 {
		return nil
	}
	out := map[string]json.RawMessage{}
	queue := make([]string, 0, len(seen))
	for name := range seen {
		queue = append(queue, name)
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, exists := out[name]; exists {
			continue
		}
		schema := r.schemas[name]
		if len(schema) == 0 {
			continue
		}
		out[name] = append(json.RawMessage(nil), schema...)
		before := len(seen)
		collectReferences(schema, seen)
		if len(seen) == before {
			continue
		}
		for next := range seen {
			if _, exists := out[next]; !exists {
				queue = append(queue, next)
			}
		}
		sort.Strings(queue)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectReferences(raw json.RawMessage, seen map[string]bool) {
	if len(raw) == 0 {
		return
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	walkReferences(value, seen)
}

func walkReferences(value any, seen map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			if name, ok := strings.CutPrefix(ref, ResultSchemaPrefix); ok && name != "" {
				seen[name] = true
			}
		}
		for _, child := range typed {
			walkReferences(child, seen)
		}
	case []any:
		for _, child := range typed {
			walkReferences(child, seen)
		}
	}
}

func yamlNodeValue(node *yaml.Node) (any, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return yamlNodeValue(node.Content[0])
	case yaml.MappingNode:
		out := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("schema map key is not scalar")
			}
			value, err := yamlNodeValue(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			out[key.Value] = value
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := yamlNodeValue(child)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case yaml.ScalarNode:
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	case yaml.AliasNode:
		return yamlNodeValue(node.Alias)
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
}

func loadProvenance() (Provenance, error) {
	raw, err := fixtureBytes(provenancePath)
	if err != nil {
		return Provenance{}, err
	}
	var provenance Provenance
	if err := json.Unmarshal(raw, &provenance); err != nil {
		return Provenance{}, fmt.Errorf("decode Chab contract provenance: %w", err)
	}
	if provenance.BackendRevision == "" {
		return Provenance{}, fmt.Errorf("Chab contract provenance missing backend revision")
	}
	return provenance, nil
}

func fixtureBytes(path string) ([]byte, error) {
	data, err := fixtureFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded %s: %w", path, err)
	}
	return data, nil
}

func verifyFixture(provenance Provenance, path string, data []byte) error {
	name := strings.TrimPrefix(path, "testdata/")
	for _, fixture := range provenance.Fixtures {
		if fixture.File != name {
			continue
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if !strings.EqualFold(got, fixture.SHA256) {
			return fmt.Errorf("%s SHA-256 = %s, want %s", name, got, fixture.SHA256)
		}
		return nil
	}
	return fmt.Errorf("provenance missing fixture %s", name)
}

func verifyCounts(registry Registry) error {
	for _, fixture := range registry.provenance.Fixtures {
		if fixture.File != "chab-v1.yaml" {
			continue
		}
		if fixture.Operations > 0 && fixture.Operations != len(registry.operations) {
			return fmt.Errorf("operation count = %d, want %d", len(registry.operations), fixture.Operations)
		}
		if fixture.Paths > 0 && fixture.Paths != registry.pathCount {
			return fmt.Errorf("path count = %d, want %d", registry.pathCount, fixture.Paths)
		}
		return nil
	}
	return fmt.Errorf("provenance missing chab-v1.yaml fixture")
}

func isHTTPMethod(method string) bool {
	switch strings.ToLower(method) {
	case "get", "post", "put", "patch", "delete", "head", "options", "trace":
		return true
	default:
		return false
	}
}

// StableJSON compacts a raw JSON fragment using ordinary encoding/json rules.
func StableJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return json.RawMessage(buf.Bytes())
}
