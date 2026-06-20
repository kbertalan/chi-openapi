package openapi_test

import (
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
)

// OpenAPI 3.1 allows tag-derived siblings directly alongside a field's $ref.
func TestSchemaGenerator_RefSiblings(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	_ = sg.GenerateSchema("openapi.TestRefSiblings")
	schema := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestRefSiblings")

	// direct ref: $ref plus siblings on the same object
	decorated := schema.Properties["decorated"]
	AssertEqual(t, schemaRef("openapi.TestSimple"), decorated.Ref)
	AssertEqual(t, "the decorated one", decorated.Description)
	if decorated.Deprecated == nil || !*decorated.Deprecated {
		t.Fatalf("expected decorated deprecated=true, got %v", decorated.Deprecated)
	}
	if decorated.MaxLength == nil || *decorated.MaxLength != 50 {
		t.Fatalf("expected decorated maxLength=50, got %v", decorated.MaxLength)
	}

	// nullable ref: anyOf wrapper, siblings on the wrapper
	nullable := schema.Properties["nullable"]
	AssertEqual(t, "the nullable one", nullable.Description)
	AssertEqual(t, "", nullable.Ref)
	AssertDeepEqual(t, []*openapi.Schema{
		{Ref: schemaRef("openapi.TestSimple")},
		{Type: "null"},
	}, nullable.AnyOf)

	// untagged ref: bare $ref, no siblings
	plain := schema.Properties["plainRef"]
	AssertEqual(t, schemaRef("openapi.TestSimple"), plain.Ref)
	AssertEqual(t, "", plain.Description)
	if plain.Deprecated != nil || plain.MaxLength != nil {
		t.Fatalf("expected plainRef to have no siblings, got %+v", plain)
	}

	// leak guard: shared component must stay clean
	shared := FindSchemaBySuffix(t, sg.GetSchemas(), ".TestSimple")
	if shared.Description != "" || shared.Deprecated != nil || shared.MaxLength != nil {
		t.Fatalf("sibling tags leaked into shared component: %+v", shared)
	}
}
