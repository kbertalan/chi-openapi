package openapi_test

import (
	"reflect"
	"strconv"
	"strings"

	openapi "github.com/kbertalan/chi-openapi"
)

// Test-only TagHandlers mapping a subset of `validate` and `binding` tags onto
// schema constraints, used to exercise the RegisterTagHandler extension point.

func schemaIsType(s *openapi.Schema, typeName string) bool {
	switch t := s.Type.(type) {
	case string:
		return t == typeName
	case []any:
		for _, v := range t {
			if v == typeName {
				return true
			}
		}
	}
	return false
}

func validateTagHandler(schema *openapi.Schema, tag reflect.StructTag) {
	validateTag := tag.Get("validate")
	if validateTag == "" {
		return
	}
	for _, part := range strings.Split(validateTag, ",") {
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
			switch {
			case schemaIsType(schema, "integer") || schemaIsType(schema, "number"):
				if min, err := strconv.ParseFloat(val, 64); err == nil {
					schema.Minimum = &min
				}
			case schemaIsType(schema, "string"):
				if m, err := strconv.Atoi(val); err == nil {
					schema.MinLength = &m
				}
			case schemaIsType(schema, "array"):
				if m, err := strconv.Atoi(val); err == nil {
					schema.MinItems = &m
				}
			}
		case strings.HasPrefix(part, "max="):
			val := strings.TrimPrefix(part, "max=")
			switch {
			case schemaIsType(schema, "integer") || schemaIsType(schema, "number"):
				if max, err := strconv.ParseFloat(val, 64); err == nil {
					schema.Maximum = &max
				}
			case schemaIsType(schema, "string"):
				if m, err := strconv.Atoi(val); err == nil {
					schema.MaxLength = &m
				}
			case schemaIsType(schema, "array"):
				if m, err := strconv.Atoi(val); err == nil {
					schema.MaxItems = &m
				}
			}
		}
	}
}

func bindingTagHandler(schema *openapi.Schema, tag reflect.StructTag) {
	bindingTag := tag.Get("binding")
	if strings.Contains(bindingTag, "email") {
		schema.Format = "email"
	}
	if strings.Contains(bindingTag, "uuid") {
		schema.Format = "uuid"
	}
}
