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

// extractTag retrieves the value of a specific key from a struct tag string.
// e.g. tag="validate:\"required\" json:\"foo\"", key="validate" -> "required".
// It uses reflect.StructTag.Get so that values may contain spaces (e.g. a
// description), which a naive space-tokenizing parser would truncate.
func extractTag(tag, key string) string {
	return reflect.StructTag(tag).Get(key)
}

// coerceTagValue converts a raw struct-tag string into a typed value matching
// the schema's primary type, so that e.g. `default=5` on an integer field emits
// the JSON number 5 rather than the string "5". On parse failure (or for string
// and unknown types) it returns the raw string unchanged.
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

// openapiTagKeys is the set of recognized keys in the `openapi` struct tag.
// It is used to find key boundaries when splitting the comma-separated tag, so
// that a comma inside a value (e.g. the regex pattern ^a{2,5}$) is not treated
// as a separator.
var openapiTagKeys = map[string]bool{
	"format": true, "pattern": true, "example": true, "title": true,
	"description": true, "deprecated": true, "readOnly": true, "writeOnly": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMin": true,
	"exclusiveMaximum": true, "exclusiveMax": true, "minLength": true, "maxLength": true,
	"minItems": true, "maxItems": true, "uniqueItems": true, "enum": true, "default": true,
}

// splitOpenAPITagParts splits a comma-separated `openapi` tag into key=value
// segments. A comma only begins a new segment when the text after it starts
// with a recognized key followed by '='; otherwise the comma is part of the
// preceding value (e.g. ^a{2,5}$ stays intact). This is a heuristic: a value
// that literally contains ",<knownKey>=" will still be split there.
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

// applyEnhancedTags applies OpenAPI 3.1 metadata from struct tags to a schema.
func (sg *SchemaGenerator) applyEnhancedTags(schema *Schema, tag string) {
	// Parse openapi tag for enhanced features
	if openapiTag := extractTag(tag, "openapi"); openapiTag != "" {
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

	// Parse validate tag for additional constraints
	if validateTag := extractTag(tag, "validate"); validateTag != "" {
		parts := strings.Split(validateTag, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			switch {
			case part == "email":
				schema.Format = "email"
			case part == "uuid":
				schema.Format = "uuid"
			case part == "uri", part == "url":
				schema.Format = "uri"
			case strings.HasPrefix(part, "min="):
				val := strings.TrimPrefix(part, "min=")
				if hasType(schema, "integer") || hasType(schema, "number") {
					if min, err := strconv.ParseFloat(val, 64); err == nil {
						schema.Minimum = &min
					}
				} else if hasType(schema, "string") {
					if m, err := strconv.Atoi(val); err == nil {
						schema.MinLength = &m
					}
				} else if hasType(schema, "array") {
					if m, err := strconv.Atoi(val); err == nil {
						schema.MinItems = &m
					}
				}
			case strings.HasPrefix(part, "max="):
				val := strings.TrimPrefix(part, "max=")
				if hasType(schema, "integer") || hasType(schema, "number") {
					if max, err := strconv.ParseFloat(val, 64); err == nil {
						schema.Maximum = &max
					}
				} else if hasType(schema, "string") {
					if m, err := strconv.Atoi(val); err == nil {
						schema.MaxLength = &m
					}
				} else if hasType(schema, "array") {
					if m, err := strconv.Atoi(val); err == nil {
						schema.MaxItems = &m
					}
				}
			case strings.HasPrefix(part, "exclusiveMin="):
				val := strings.TrimPrefix(part, "exclusiveMin=")
				if hasType(schema, "integer") || hasType(schema, "number") {
					if min, err := strconv.ParseFloat(val, 64); err == nil {
						schema.ExclusiveMinimum = &min
					}
				}
			case strings.HasPrefix(part, "exclusiveMax="):
				val := strings.TrimPrefix(part, "exclusiveMax=")
				if hasType(schema, "integer") || hasType(schema, "number") {
					if max, err := strconv.ParseFloat(val, 64); err == nil {
						schema.ExclusiveMaximum = &max
					}
				}
			case strings.HasPrefix(part, "len="):
				val := strings.TrimPrefix(part, "len=")
				if hasType(schema, "string") {
					if m, err := strconv.Atoi(val); err == nil {
						schema.MinLength = &m
						schema.MaxLength = &m
					}
				} else if hasType(schema, "array") {
					if m, err := strconv.Atoi(val); err == nil {
						schema.MinItems = &m
						schema.MaxItems = &m
					}
				}
			case strings.HasPrefix(part, "oneof="):
				val := strings.TrimPrefix(part, "oneof=")
				vals := strings.Split(val, " ")
				schema.Enum = make([]interface{}, len(vals))
				for i, v := range vals {
					schema.Enum[i] = v
				}
			}
		}
	}

	// Parse binding tag for additional format hints
	if bindingTag := extractTag(tag, "binding"); bindingTag != "" {
		if strings.Contains(bindingTag, "email") {
			schema.Format = "email"
		}
		if strings.Contains(bindingTag, "uuid") {
			schema.Format = "uuid"
		}
	}
}
