package openapi_test

import (
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
	"github.com/kbertalan/chi-openapi/types"
)

// Third-party types are not resolved out of the box: a fresh generator falls
// back to an unknown-type reference for pgtype.Text.
func TestExternalTypes_NotKnownByDefault(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	schema := sg.GenerateSchema("pgtype.Text")
	// Unknown types are emitted as a $ref to a generated fallback object schema,
	// not as the inline string schema the types package would provide.
	if schema.Ref == "" {
		t.Fatalf("expected unknown pgtype.Text to resolve to a $ref, got %+v", schema)
	}
	if hasType(t, schema) == "string" {
		t.Fatalf("did not expect pgtype.Text to be a known string schema by default")
	}
}

// After bulk-registering types.PgType, the schema is resolved inline.
func TestExternalTypes_BulkRegister(t *testing.T) {
	openapi.AddExternalKnownTypes(types.PgType)

	// Uses the global type index, which AddExternalKnownTypes populates.
	sg := openapi.NewSchemaGenerator()
	schema := sg.GenerateSchema("pgtype.Text")
	if schema.Ref != "" {
		t.Fatalf("expected inline schema for registered pgtype.Text, got $ref %q", schema.Ref)
	}
	AssertEqual(t, "string", hasType(t, schema))
}

// Guard against an empty/typoed map in the types package.
func TestExternalTypes_MapsPopulated(t *testing.T) {
	t.Parallel()

	for name, m := range map[string]map[string]*openapi.Schema{
		"PgType":  types.PgType,
		"UUID":    types.UUID,
		"Decimal": types.Decimal,
	} {
		if len(m) == 0 {
			t.Errorf("types.%s is empty", name)
		}
	}
}

// hasType returns the schema's single string type, or "" if it is not a plain
// string-typed schema.
func hasType(t *testing.T, s *openapi.Schema) string {
	t.Helper()
	if str, ok := s.Type.(string); ok {
		return str
	}
	return ""
}
