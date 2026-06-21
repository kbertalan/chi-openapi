package openapi_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
)

func TestSchemaGenerator_PrimitiveAndCollectionTypes(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	tests := []struct {
		name     string
		typeName string
		assert   func(*testing.T, *openapi.Schema)
	}{
		{
			name:     "int",
			typeName: "int",
			assert: func(t *testing.T, schema *openapi.Schema) {
				AssertEqual(t, "integer", schema.Type)
				AssertEqual(t, "", schema.Ref)
			},
		},
		{
			name:     "string",
			typeName: "string",
			assert: func(t *testing.T, schema *openapi.Schema) {
				AssertEqual(t, "string", schema.Type)
				AssertEqual(t, "", schema.Ref)
			},
		},
		{
			name:     "bool pointer",
			typeName: "*bool",
			assert: func(t *testing.T, schema *openapi.Schema) {
				AssertDeepEqual(t, []string{"boolean", "null"}, schema.Type)
			},
		},
		{
			name:     "slice",
			typeName: "[]string",
			assert: func(t *testing.T, schema *openapi.Schema) {
				AssertEqual(t, "array", schema.Type)
				if schema.Items == nil || schema.Items.Type != "string" {
					t.Fatalf("expected string array items, got %+v", schema.Items)
				}
			},
		},
		{
			name:     "map",
			typeName: "map[string]int",
			assert: func(t *testing.T, schema *openapi.Schema) {
				AssertEqual(t, "object", schema.Type)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, sg.GenerateSchema(tc.typeName))
		})
	}
}

func TestSchemaGenerator_ReferenceEmission(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	schema := sg.GenerateSchema("TestSimple")
	if schema.Ref == "" {
		t.Fatalf("expected reference for struct type, got %+v", schema)
	}

	stored := sg.GetSchemas()
	if _, ok := stored["openapi.TestSimple"]; !ok {
		t.Fatalf("expected stored schema for openapi.TestSimple, got %v", stored)
	}
}

func TestSchemaGenerator_StructShapes(t *testing.T) {
	t.Parallel()

	t.Run("simple struct required fields", func(t *testing.T) {
		sg := NewTestSchemaGenerator()
		_ = sg.GenerateSchema("TestSimple")
		schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestSimple")

		AssertEqual(t, "object", schema.Type)
		AssertDeepEqual(t, []string{"id", "name"}, schema.Required)

		if prop, ok := schema.Properties["id"]; !ok || prop.Type != "integer" {
			t.Fatalf("expected integer property 'id', got %+v", prop)
		}
		if prop, ok := schema.Properties["name"]; !ok || prop.Type != "string" {
			t.Fatalf("expected string property 'name', got %+v", prop)
		}
	})

	t.Run("pointer field omitempty", func(t *testing.T) {
		sg := NewTestSchemaGenerator()
		_ = sg.GenerateSchema("TestWithPointer")
		schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestWithPointer")

		if len(schema.Required) != 0 {
			t.Fatalf("expected no required fields due to omitempty, got %v", schema.Required)
		}
		if prop, ok := schema.Properties["name"]; !ok {
			t.Fatalf("expected property 'name' to exist")
		} else {
			AssertDeepEqual(t, []string{"string", "null"}, prop.Type)
		}
	})

	t.Run("OpenAPI 3.1 compliance features", func(t *testing.T) {
		sg := NewTestSchemaGenerator()
		_ = sg.GenerateSchema("TestCompliance31")
		schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestCompliance31")

		count := schema.Properties["count"]
		if count.ExclusiveMinimum == nil || *count.ExclusiveMinimum != 0 {
			t.Errorf("expected ExclusiveMinimum 0, got %v", count.ExclusiveMinimum)
		}
		if count.ExclusiveMaximum == nil || *count.ExclusiveMaximum != 100 {
			t.Errorf("expected ExclusiveMaximum 100, got %v", count.ExclusiveMaximum)
		}

		rate := schema.Properties["rate"]
		if rate.ExclusiveMinimum == nil || *rate.ExclusiveMinimum != 0.5 {
			t.Errorf("expected ExclusiveMinimum 0.5, got %v", rate.ExclusiveMinimum)
		}
	})

	t.Run("type aliases (maps and slices)", func(t *testing.T) {
		sg := NewTestSchemaGenerator()

		// Test map alias
		_ = sg.GenerateSchema("TestAliasMap")
		mapSchema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestAliasMap")
		if mapSchema.Type != "object" {
			t.Errorf("expected object type for TestAliasMap, got %v", mapSchema.Type)
		}
		if mapSchema.AdditionalProperties == nil {
			t.Errorf("expected additionalProperties for TestAliasMap")
		}

		// Test slice alias
		_ = sg.GenerateSchema("TestAliasSlice")
		sliceSchema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestAliasSlice")
		if sliceSchema.Type != "array" {
			t.Errorf("expected array type for TestAliasSlice, got %v", sliceSchema.Type)
		}
		if sliceSchema.Items == nil {
			t.Errorf("expected items for TestAliasSlice")
		}
	})

	t.Run("array field schema", func(t *testing.T) {
		sg := NewTestSchemaGenerator()
		_ = sg.GenerateSchema("TestWithArray")
		container := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestWithArray")

		prop, ok := container.Properties["tags"]
		if !ok || prop.Type != "array" || prop.Items == nil || prop.Items.Type != "string" {
			t.Fatalf("expected array property 'tags', got %+v", prop)
		}
	})

	t.Run("nested references", func(t *testing.T) {
		sg := NewTestSchemaGenerator()
		ref := sg.GenerateSchema("TestNested")
		if ref.Ref == "" {
			t.Fatalf("expected ref for nested struct, got %+v", ref)
		}

		nested := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestNested")
		child, ok := nested.Properties["simple"]
		if !ok || child.Ref == "" {
			t.Fatalf("expected nested property to reference TestSimple, got %+v", child)
		}
	})

	t.Run("qualified nested type", func(t *testing.T) {
		sg := NewTestSchemaGenerator()
		schema := sg.GenerateSchema("TestWithQualified")
		if schema.Ref == "" {
			t.Fatalf("expected ref for TestWithQualified, got %+v", schema)
		}
		stored := sg.GetSchemas()
		if !HasSchemaWithSuffix(stored, ".TestWithQualified") {
			t.Fatalf("expected schema for TestWithQualified, got %v", stored)
		}
	})
}

func TestSchemaGenerator_TagEnhancements(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	sg.RegisterTagHandler(validateTagHandler, bindingTagHandler)
	_ = sg.GenerateSchema("openapi.TagExample")
	schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TagExample")

	id := schema.Properties["id"]
	AssertEqual(t, "uuid", id.Format)
	if id.Deprecated == nil || *id.Deprecated != true {
		t.Fatalf("expected id to be deprecated, got %v", id.Deprecated)
	}
	AssertEqual(t, "00000000-0000-0000-0000-000000000000", id.Default)

	alias := schema.Properties["alias"]
	AssertEqual(t, "^a.*$", alias.Pattern)
	if alias.MinLength == nil || *alias.MinLength != 2 {
		t.Fatalf("expected alias minLength=2, got %v", alias.MinLength)
	}
	if alias.MaxLength == nil || *alias.MaxLength != 5 {
		t.Fatalf("expected alias maxLength=5, got %v", alias.MaxLength)
	}

	email := schema.Properties["email"]
	AssertEqual(t, "email", email.Format)

	owner := schema.Properties["owner"]
	AssertEqual(t, "uuid", owner.Format)

	// description is applied from the openapi tag
	count := schema.Properties["count"]
	AssertEqual(t, "number of items", count.Description)
	// default/example are coerced to the field's primary type, not left as strings
	AssertEqual[any](t, int64(7), count.Default)
	if len(count.Examples) != 1 || count.Examples[0] != int64(42) {
		t.Fatalf("expected count examples [42], got %v", count.Examples)
	}

	// multiple example= segments accumulate into the Examples slice
	size := schema.Properties["size"]
	if len(size.Examples) != 3 || size.Examples[0] != int64(1) || size.Examples[1] != int64(2) || size.Examples[2] != int64(3) {
		t.Fatalf("expected size examples [1 2 3], got %v", size.Examples)
	}

	rate := schema.Properties["rate"]
	AssertEqual[any](t, 1.5, rate.Default)

	// a comma inside a value (regex quantifier) must not be treated as a
	// key separator, and a following key after the comma still parses
	code := schema.Properties["code"]
	AssertEqual(t, "^a{2,5}$", code.Pattern)
	if code.MinLength == nil || *code.MinLength != 2 {
		t.Fatalf("expected code minLength=2, got %v", code.MinLength)
	}
}

func TestSchemaGenerator_CustomTagHandlerPrecedence(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	sg.RegisterTagHandler(validateTagHandler)
	_ = sg.GenerateSchema("openapi.TagCollision")
	schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TagCollision")

	conflict := schema.Properties["conflict"]
	AssertEqual(t, "uuid", conflict.Format)

	survives := schema.Properties["survives"]
	if survives.MinLength == nil || *survives.MinLength != 3 {
		t.Fatalf("expected survives minLength=3, got %v", survives.MinLength)
	}
}

func TestSchemaGenerator_NoBuiltinValidateSupport(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	_ = sg.GenerateSchema("openapi.TagCollision")
	schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TagCollision")

	if ml := schema.Properties["survives"].MinLength; ml != nil {
		t.Fatalf("expected validate tag to be ignored without a handler, got minLength=%v", *ml)
	}
}

func TestSchemaGenerator_TagHandlerError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	sg := NewTestSchemaGenerator()
	sg.RegisterTagHandler(func(*openapi.Schema, reflect.StructTag) error {
		return sentinel
	})
	_ = sg.GenerateSchema("openapi.TagCollision")

	err := sg.Err()
	if err == nil {
		t.Fatal("expected tag handler error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestSchemaGenerator_OpenAPITagConversionError(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	_ = sg.GenerateSchema("openapi.TagBadNumeric")

	err := sg.Err()
	if err == nil {
		t.Fatal("expected conversion error for openapi:\"minimum=abc\", got nil")
	}
	if !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("expected error to mention the failing keyword, got %v", err)
	}
}
