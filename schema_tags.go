// Package openapi provides JSON-schema tag parsing utilities.
package openapi

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// extractJSONTag returns the JSON key name from a struct tag string.
// e.g. `json:"foo,omitempty" xml:"bar"` -> "foo".
func extractJSONTag(tag string) string {
	for _, part := range strings.Split(tag, " ") {
		if strings.HasPrefix(part, "json:") {
			value := strings.Trim(part[5:], `"`)
			if comma := strings.Index(value, ","); comma != -1 {
				return value[:comma]
			}
			return value
		}
	}
	return ""
}

// coerceTagValue converts a tag value to match the schema's primary type, so
// `default=5` on an integer field emits 5, not "5". Falls back to the raw string.
func coerceTagValue(schema *Schema, value string) any {
	switch primaryType(schema) {
	case "integer":
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	case "number":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	case "boolean":
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return value
}

// openapiTagKeys are the recognized `openapi` tag keys, used to find segment
// boundaries so a comma inside a value (e.g. ^a{2,5}$) isn't a separator.
var openapiTagKeys = map[string]bool{
	"format": true, "pattern": true, "example": true, "examples": true, "title": true,
	"description": true, "deprecated": true, "readOnly": true, "writeOnly": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMin": true,
	"exclusiveMaximum": true, "exclusiveMax": true, "minLength": true, "maxLength": true,
	"minItems": true, "maxItems": true, "uniqueItems": true, "enum": true, "enums": true, "default": true,
}

// splitOpenAPITagParts splits a comma-separated `openapi` tag into key=value
// segments, treating a comma as a separator only when followed by a known key.
// Heuristic: a value literally containing ",<knownKey>=" is still split there.
func splitOpenAPITagParts(tag string) []string {
	var parts []string
	for _, seg := range strings.Split(tag, ",") {
		if key, _, ok := strings.Cut(seg, "="); ok && openapiTagKeys[strings.TrimSpace(key)] {
			parts = append(parts, seg)
		} else if len(parts) > 0 {
			parts[len(parts)-1] += "," + seg
		} else {
			parts = append(parts, seg)
		}
	}
	return parts
}

// TagHandler applies schema metadata derived from a struct field's tag. Handlers
// run before the built-in openapi handler, which wins on any keyword collision.
// A non-nil error aborts spec generation. Register before the first
// GenerateSchema call; not safe to register concurrently with generation.
type TagHandler func(schema *Schema, tag reflect.StructTag) error

// RegisterTagHandler appends custom tag handlers, run in registration order.
func (sg *SchemaGenerator) RegisterTagHandler(handlers ...TagHandler) {
	sg.tagHandlers = append(sg.tagHandlers, handlers...)
}

// applyEnhancedTags runs custom handlers first, then the openapi handler last so
// its keywords override any colliding value a handler produced. It returns the
// first error encountered without applying the openapi tag.
func (sg *SchemaGenerator) applyEnhancedTags(schema *Schema, tag string) error {
	st := reflect.StructTag(tag)
	for _, h := range sg.tagHandlers {
		if err := h(schema, st); err != nil {
			return err
		}
	}
	return applyOpenAPITag(schema, st)
}

// addErr records a tag-processing error for later retrieval via Err.
func (sg *SchemaGenerator) addErr(err error) {
	sg.mutex.Lock()
	sg.errs = append(sg.errs, err)
	sg.mutex.Unlock()
}

// Err returns the combined error from tag processing, or nil if none failed.
func (sg *SchemaGenerator) Err() error {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()
	return errors.Join(sg.errs...)
}

// applyOpenAPITag applies OpenAPI 3.1 metadata from the `openapi` struct tag,
// returning an error if a numeric keyword's value fails to convert.
func applyOpenAPITag(schema *Schema, tag reflect.StructTag) error {
	openapiTag := tag.Get("openapi")
	if openapiTag == "" {
		return nil
	}
	for _, part := range splitOpenAPITagParts(openapiTag) {
		part = strings.TrimSpace(part)
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		parseFloat := func() (*float64, error) {
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, fmt.Errorf("openapi tag %q=%q: %w", key, value, err)
			}
			return &f, nil
		}
		parseInt := func() (*int, error) {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("openapi tag %q=%q: %w", key, value, err)
			}
			return &n, nil
		}

		var err error
		switch key {
		case "format":
			schema.Format = value
		case "pattern":
			schema.Pattern = value
		case "example":
			schema.Examples = append(schema.Examples, coerceTagValue(schema, value))
		case "examples":
			for _, v := range strings.Split(value, ",") {
				schema.Examples = append(schema.Examples, coerceTagValue(schema, strings.TrimSpace(v)))
			}
		case "title":
			schema.Title = value
		case "description":
			schema.Description = value
		case "deprecated":
			if value == "true" {
				dep := true
				schema.Deprecated = &dep
			}
		case "readOnly":
			if value == "true" {
				ro := true
				schema.ReadOnly = &ro
			}
		case "writeOnly":
			if value == "true" {
				wo := true
				schema.WriteOnly = &wo
			}
		case "minimum":
			schema.Minimum, err = parseFloat()
		case "maximum":
			schema.Maximum, err = parseFloat()
		case "exclusiveMinimum", "exclusiveMin":
			schema.ExclusiveMinimum, err = parseFloat()
		case "exclusiveMaximum", "exclusiveMax":
			schema.ExclusiveMaximum, err = parseFloat()
		case "minLength":
			schema.MinLength, err = parseInt()
		case "maxLength":
			schema.MaxLength, err = parseInt()
		case "minItems":
			schema.MinItems, err = parseInt()
		case "maxItems":
			schema.MaxItems, err = parseInt()
		case "uniqueItems":
			if value == "true" {
				ui := true
				schema.UniqueItems = &ui
			}
		case "enum":
			schema.Enum = append(schema.Enum, value)
		case "enums":
			for _, v := range strings.Split(value, ",") {
				schema.Enum = append(schema.Enum, strings.TrimSpace(v))
			}
		case "default":
			schema.Default = coerceTagValue(schema, value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
