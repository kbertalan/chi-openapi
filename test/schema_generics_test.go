package openapi_test

import (
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
)

// schemaRef builds the $ref pointing at a generated component.
func schemaRef(key string) string { return "#/components/schemas/" + key }

// Reusable expected leaf components shared across the generic test cases.
var (
	expectedTestBusinessObject = openapi.Schema{
		Type: "object",
		Properties: map[string]*openapi.Schema{
			"id":   {Type: "integer", Format: "int"},
			"name": {Type: "string"},
		},
		Required: []string{"id", "name"},
	}

	expectedTestPageInfo = openapi.Schema{
		Type: "object",
		Properties: map[string]*openapi.Schema{
			"currentPage": {Type: "integer", Format: "int"},
			"totalPages":  {Type: "integer", Format: "int"},
		},
		Required: []string{"currentPage", "totalPages"},
	}
)

// TestPaginatedResponse[TestBusinessObject] should monomorphize into an envelope
// whose `data` is an array of the concrete type and `page` references the
// (non-generic) metadata struct, alongside the two referenced components.
func TestSchemaGenerator_GenericInstantiation(t *testing.T) {
	t.Parallel()

	const envelopeKey = "openapi.TestPaginatedResponse-openapi.TestBusinessObject"

	sg := NewTestSchemaGenerator()
	ref := sg.GenerateSchema("TestPaginatedResponse[TestBusinessObject]")
	AssertEqual(t, schemaRef(envelopeKey), ref.Ref)

	expected := map[string]openapi.Schema{
		envelopeKey: {
			Type: "object",
			Properties: map[string]*openapi.Schema{
				"data": {
					Type:  "array",
					Items: &openapi.Schema{Ref: schemaRef("openapi.TestBusinessObject")},
				},
				"page": {Ref: schemaRef("openapi.TestPageInfo")},
			},
			Required: []string{"data", "page"},
		},
		"openapi.TestBusinessObject": expectedTestBusinessObject,
		"openapi.TestPageInfo":       expectedTestPageInfo,
	}

	AssertDeepEqual(t, expected, sg.GetSchemas())
}

// TestGenericPair[string,TestBusinessObject] exercises multiple type parameters,
// a primitive argument (K=string) and a pointer parameter (*V).
func TestSchemaGenerator_GenericMultipleParams(t *testing.T) {
	t.Parallel()

	const pairKey = "openapi.TestGenericPair-string-openapi.TestBusinessObject"

	sg := NewTestSchemaGenerator()
	ref := sg.GenerateSchema("TestGenericPair[string,TestBusinessObject]")
	AssertEqual(t, schemaRef(pairKey), ref.Ref)

	expected := map[string]openapi.Schema{
		pairKey: {
			Type: "object",
			Properties: map[string]*openapi.Schema{
				"key": {Type: "string"},
				"value": {
					AnyOf: []*openapi.Schema{
						{Ref: schemaRef("openapi.TestBusinessObject")},
						{Type: "null"},
					},
				},
			},
			Required: []string{"key"},
		},
		"openapi.TestBusinessObject": expectedTestBusinessObject,
	}

	AssertDeepEqual(t, expected, sg.GetSchemas())
}

// A slice type argument should nest: `Data []T` with T=[]TestBusinessObject
// becomes an array of arrays of the concrete type.
func TestSchemaGenerator_GenericSliceArgument(t *testing.T) {
	t.Parallel()

	const envelopeKey = "openapi.TestPaginatedResponse--openapi.TestBusinessObject"

	sg := NewTestSchemaGenerator()
	ref := sg.GenerateSchema("TestPaginatedResponse[[]TestBusinessObject]")
	AssertEqual(t, schemaRef(envelopeKey), ref.Ref)

	expected := map[string]openapi.Schema{
		envelopeKey: {
			Type: "object",
			Properties: map[string]*openapi.Schema{
				"data": {
					Type: "array",
					Items: &openapi.Schema{
						Type:  "array",
						Items: &openapi.Schema{Ref: schemaRef("openapi.TestBusinessObject")},
					},
				},
				"page": {Ref: schemaRef("openapi.TestPageInfo")},
			},
			Required: []string{"data", "page"},
		},
		"openapi.TestBusinessObject": expectedTestBusinessObject,
		"openapi.TestPageInfo":       expectedTestPageInfo,
	}

	AssertDeepEqual(t, expected, sg.GetSchemas())
}

// A struct field whose own type is a generic instantiation (e.g.
// TestPaginatedResponse[TestBusinessObject]) should emit a $ref to the
// monomorphized component and generate it plus all transitive leaves. This
// exercises the *ast.IndexExpr arm in convertFieldType, distinct from a
// top-level instantiation requested directly.
func TestSchemaGenerator_GenericInstantiationAsField(t *testing.T) {
	t.Parallel()

	const (
		holderKey   = "openapi.TestGenericFieldHolder"
		envelopeKey = "openapi.TestPaginatedResponse-openapi.TestBusinessObject"
	)

	sg := NewTestSchemaGenerator()
	ref := sg.GenerateSchema("TestGenericFieldHolder")
	AssertEqual(t, schemaRef(holderKey), ref.Ref)

	expected := map[string]openapi.Schema{
		holderKey: {
			Type: "object",
			Properties: map[string]*openapi.Schema{
				"envelope": {Ref: schemaRef(envelopeKey)},
			},
			Required: []string{"envelope"},
		},
		envelopeKey: {
			Type: "object",
			Properties: map[string]*openapi.Schema{
				"data": {
					Type:  "array",
					Items: &openapi.Schema{Ref: schemaRef("openapi.TestBusinessObject")},
				},
				"page": {Ref: schemaRef("openapi.TestPageInfo")},
			},
			Required: []string{"data", "page"},
		},
		"openapi.TestBusinessObject": expectedTestBusinessObject,
		"openapi.TestPageInfo":       expectedTestPageInfo,
	}

	AssertDeepEqual(t, expected, sg.GetSchemas())
}

// TestSchemaGenerator_GenericIsCachedAndDistinct ensures repeated instantiations
// reuse one component and that different arguments yield different components.
func TestSchemaGenerator_GenericIsCachedAndDistinct(t *testing.T) {
	t.Parallel()

	sg := NewTestSchemaGenerator()
	first := sg.GenerateSchema("TestPaginatedResponse[TestBusinessObject]")
	second := sg.GenerateSchema("TestPaginatedResponse[TestBusinessObject]")
	AssertEqual(t, first.Ref, second.Ref)

	other := sg.GenerateSchema("TestPaginatedResponse[TestSimple]")
	if other.Ref == first.Ref {
		t.Fatalf("expected distinct refs for distinct type arguments, both = %s", first.Ref)
	}
}
