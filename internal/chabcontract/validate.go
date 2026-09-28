package chabcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ValidateRequest checks a JSON request body against the pinned request schema
// features used by Chab operation inputs.
func (r Registry) ValidateRequest(operationKey string, raw json.RawMessage) error {
	op, ok := r.Find(operationKey)
	if !ok {
		return fmt.Errorf("unknown operation key %s", operationKey)
	}
	if len(op.RequestSchema) == 0 {
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil
		}
		return fmt.Errorf("%s does not define a JSON request body", operationKey)
	}
	schema, err := decodeSchema(op.RequestSchema)
	if err != nil {
		return fmt.Errorf("%s request schema is not inspectable: %w", operationKey, err)
	}
	value, err := decodeSchema(raw)
	if err != nil {
		return fmt.Errorf("%s request body is not valid JSON: %w", operationKey, err)
	}
	if err := r.validateValue(operationKey, value, schema); err != nil {
		return fmt.Errorf("%s request %s", operationKey, err)
	}
	return nil
}

func decodeSchema(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("contains trailing JSON values")
		}
		return nil, err
	}
	return value, nil
}

func (r Registry) validateValue(path string, value any, schema any) error {
	object, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	if ref, _ := object["$ref"].(string); ref != "" {
		name, ok := strings.CutPrefix(ref, ResultSchemaPrefix)
		if !ok || name == "" {
			return fmt.Errorf("%s references unsupported schema %q", path, ref)
		}
		target, ok := r.schemas[name]
		if !ok {
			return fmt.Errorf("%s references missing schema %q", path, ref)
		}
		decoded, err := decodeSchema(target)
		if err != nil {
			return fmt.Errorf("%s reference %q is not inspectable: %w", path, ref, err)
		}
		return r.validateValue(path, value, decoded)
	}
	if err := r.validateCombinators(path, value, object); err != nil {
		return err
	}
	if err := validateType(path, value, object["type"]); err != nil {
		return err
	}
	if err := validateConst(path, value, object); err != nil {
		return err
	}
	if err := validateEnum(path, value, object); err != nil {
		return err
	}
	if err := validateBounds(path, value, object); err != nil {
		return err
	}
	if err := r.validateObject(path, value, object); err != nil {
		return err
	}
	return r.validateArray(path, value, object)
}

func (r Registry) validateCombinators(path string, value any, schema map[string]any) error {
	if raw, ok := schema["oneOf"].([]any); ok {
		matches := 0
		var lastErr error
		for _, candidate := range raw {
			if err := r.validateValue(path, value, candidate); err != nil {
				lastErr = err
				continue
			}
			matches++
		}
		if matches != 1 {
			if lastErr != nil {
				return fmt.Errorf("at %s must match exactly one allowed shape: %w", path, lastErr)
			}
			return fmt.Errorf("at %s must match exactly one allowed shape", path)
		}
	}
	if raw, ok := schema["anyOf"].([]any); ok {
		var lastErr error
		for _, candidate := range raw {
			if err := r.validateValue(path, value, candidate); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		if lastErr != nil {
			return fmt.Errorf("at %s must match at least one allowed shape: %w", path, lastErr)
		}
		return fmt.Errorf("at %s must match at least one allowed shape", path)
	}
	if raw, ok := schema["allOf"].([]any); ok {
		for _, candidate := range raw {
			if err := r.validateValue(path, value, candidate); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateType(path string, value any, rawType any) error {
	if rawType == nil {
		return nil
	}
	types := schemaTypes(rawType)
	for _, typ := range types {
		if jsonValueMatchesType(value, typ) {
			return nil
		}
	}
	return fmt.Errorf("at %s has invalid type", path)
}

func schemaTypes(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func jsonValueMatchesType(value any, typ string) bool {
	switch typ {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := number.Int64()
		return err == nil
	case "number":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := strconv.ParseFloat(number.String(), 64)
		return err == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func validateConst(path string, value any, schema map[string]any) error {
	constValue, ok := schema["const"]
	if !ok || equalJSONValue(value, constValue) {
		return nil
	}
	return fmt.Errorf("at %s must equal %s", path, describeJSONValue(constValue))
}

func validateEnum(path string, value any, schema map[string]any) error {
	raw, ok := schema["enum"].([]any)
	if !ok {
		return nil
	}
	for _, allowed := range raw {
		if equalJSONValue(value, allowed) {
			return nil
		}
	}
	return fmt.Errorf("at %s is not one of the allowed values", path)
}

func validateBounds(path string, value any, schema map[string]any) error {
	number, ok := value.(json.Number)
	if !ok {
		return nil
	}
	actual, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return nil
	}
	if minimum, ok := numericSchemaValue(schema["minimum"]); ok && actual < minimum {
		return fmt.Errorf("at %s is below the minimum", path)
	}
	if maximum, ok := numericSchemaValue(schema["maximum"]); ok && actual > maximum {
		return fmt.Errorf("at %s is above the maximum", path)
	}
	return nil
}

func (r Registry) validateObject(path string, value any, schema map[string]any) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	properties := schemaProperties(schema["properties"])
	if minProperties, ok := numericSchemaValue(schema["minProperties"]); ok && float64(len(object)) < minProperties {
		return fmt.Errorf("at %s has too few fields", path)
	}
	if maxProperties, ok := numericSchemaValue(schema["maxProperties"]); ok && float64(len(object)) > maxProperties {
		return fmt.Errorf("at %s has too many fields", path)
	}
	if required, ok := schema["required"].([]any); ok {
		for _, rawName := range required {
			name, _ := rawName.(string)
			if name == "" {
				continue
			}
			if _, exists := object[name]; !exists {
				return fmt.Errorf("missing required field %s at %s", name, path)
			}
		}
	}
	switch additional := schema["additionalProperties"].(type) {
	case bool:
		if additional {
			break
		}
		for name := range object {
			if _, exists := properties[name]; !exists {
				return fmt.Errorf("field %s is not allowed at %s", name, path)
			}
		}
	case map[string]any:
		for name, child := range object {
			if _, exists := properties[name]; exists {
				continue
			}
			if err := r.validateValue(path+"."+name, child, additional); err != nil {
				return err
			}
		}
	}
	for name, propertySchema := range properties {
		child, exists := object[name]
		if !exists {
			continue
		}
		if err := r.validateValue(path+"."+name, child, propertySchema); err != nil {
			return err
		}
	}
	return nil
}

func (r Registry) validateArray(path string, value any, schema map[string]any) error {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	if minItems, ok := numericSchemaValue(schema["minItems"]); ok && float64(len(values)) < minItems {
		return fmt.Errorf("at %s has too few items", path)
	}
	if maxItems, ok := numericSchemaValue(schema["maxItems"]); ok && float64(len(values)) > maxItems {
		return fmt.Errorf("at %s has too many items", path)
	}
	items, ok := schema["items"]
	if !ok {
		return nil
	}
	for i, item := range values {
		if err := r.validateValue(fmt.Sprintf("%s[%d]", path, i), item, items); err != nil {
			return err
		}
	}
	return nil
}

func schemaProperties(raw any) map[string]any {
	properties, _ := raw.(map[string]any)
	if properties == nil {
		return map[string]any{}
	}
	return properties
}

func numericSchemaValue(raw any) (float64, bool) {
	switch typed := raw.(type) {
	case json.Number:
		value, err := strconv.ParseFloat(typed.String(), 64)
		return value, err == nil && !math.IsNaN(value)
	case float64:
		return typed, !math.IsNaN(typed)
	default:
		return 0, false
	}
}

func equalJSONValue(a, b any) bool {
	if an, ok := a.(json.Number); ok {
		if bn, ok := b.(json.Number); ok {
			return an.String() == bn.String()
		}
	}
	return reflect.DeepEqual(a, b)
}

func describeJSONValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "the documented value"
	}
	return string(raw)
}
