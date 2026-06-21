// Package openapi provides JSON-schema tag parsing utilities.
package openapi

import (
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
	"format": true, "pattern": true, "example": true, "title": true,
	"description": true, "deprecated": true, "readOnly": true, "writeOnly": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMin": true,
	"exclusiveMaximum": true, "exclusiveMax": true, "minLength": true, "maxLength": true,
	"minItems": true, "maxItems": true, "uniqueItems": true, "enum": true, "default": true,
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
// Register before the first GenerateSchema call; not safe to register concurrently
// with generation.
type TagHandler func(schema *Schema, tag reflect.StructTag)

// RegisterTagHandler appends custom tag handlers, run in registration order.
func (sg *SchemaGenerator) RegisterTagHandler(handlers ...TagHandler) {
	sg.tagHandlers = append(sg.tagHandlers, handlers...)
}

// applyEnhancedTags runs custom handlers first, then the openapi handler last so
// its keywords override any colliding value a handler produced.
func (sg *SchemaGenerator) applyEnhancedTags(schema *Schema, tag string) {
	st := reflect.StructTag(tag)
	for _, h := range sg.tagHandlers {
		h(schema, st)
	}
	applyOpenAPITag(schema, st)
}

// applyOpenAPITag applies OpenAPI 3.1 metadata from the `openapi` struct tag.
func applyOpenAPITag(schema *Schema, tag reflect.StructTag) {
	if openapiTag := tag.Get("openapi"); openapiTag != "" {
		parts := splitOpenAPITagParts(openapiTag)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "=") {
				kv := strings.SplitN(part, "=", 2)
				key, value := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
				switch key {
				case "format":
					schema.Format = value
				case "pattern":
					schema.Pattern = value
				case "example":
					schema.Example = coerceTagValue(schema, value)
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
					if min, err := strconv.ParseFloat(value, 64); err == nil {
						schema.Minimum = &min
					}
				case "maximum":
					if max, err := strconv.ParseFloat(value, 64); err == nil {
						schema.Maximum = &max
					}
				case "exclusiveMinimum", "exclusiveMin":
					if min, err := strconv.ParseFloat(value, 64); err == nil {
						schema.ExclusiveMinimum = &min
					}
				case "exclusiveMaximum", "exclusiveMax":
					if max, err := strconv.ParseFloat(value, 64); err == nil {
						schema.ExclusiveMaximum = &max
					}
				case "minLength":
					if m, err := strconv.Atoi(value); err == nil {
						schema.MinLength = &m
					}
				case "maxLength":
					if m, err := strconv.Atoi(value); err == nil {
						schema.MaxLength = &m
					}
				case "minItems":
					if m, err := strconv.Atoi(value); err == nil {
						schema.MinItems = &m
					}
				case "maxItems":
					if m, err := strconv.Atoi(value); err == nil {
						schema.MaxItems = &m
					}
				case "uniqueItems":
					if value == "true" {
						ui := true
						schema.UniqueItems = &ui
					}
				case "enum":
					vals := strings.Split(value, "|")
					schema.Enum = make([]interface{}, len(vals))
					for i, v := range vals {
						schema.Enum[i] = strings.TrimSpace(v)
					}
				case "default":
					schema.Default = coerceTagValue(schema, value)
				}
			}
		}
	}
}
