package openapi_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	openapi "github.com/kbertalan/chi-openapi"
)

// createTagsHandler exercises @Schema on a body parameter (array constraints)
// and on a query parameter (string constraints).
//
//	@Summary Create tags
//	@Param tags body []string true "tags to create"
//		@Schema
//			@MinItems 1
//			@MaxItems 5
//			@UniqueItems true
//	@Param q query string false "search filter"
//		@Schema
//			@Pattern ^[a-z]+$
//			@MaxLength 32
func createTagsHandler(w http.ResponseWriter, r *http.Request) {}

func TestParamSchema_ParsesNestedBlock(t *testing.T) {
	ann, err := openapi.ParseAnnotations("param_schema_test.go", "createTagsHandler")
	if err != nil {
		t.Fatalf("ParseAnnotations error: %v", err)
	}
	if ann == nil || len(ann.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %+v", ann)
	}

	body := ann.Parameters[0]
	if body.In != "body" {
		t.Fatalf("expected first param in=body, got %q", body.In)
	}
	if body.Schema.MinItems == nil || *body.Schema.MinItems != 1 {
		t.Errorf("expected MinItems=1, got %v", body.Schema.MinItems)
	}
	if body.Schema.MaxItems == nil || *body.Schema.MaxItems != 5 {
		t.Errorf("expected MaxItems=5, got %v", body.Schema.MaxItems)
	}
	if body.Schema.UniqueItems == nil || !*body.Schema.UniqueItems {
		t.Errorf("expected UniqueItems=true, got %v", body.Schema.UniqueItems)
	}

	q := ann.Parameters[1]
	if q.Schema.Pattern != "^[a-z]+$" {
		t.Errorf("expected Pattern, got %q", q.Schema.Pattern)
	}
	if q.Schema.MaxLength == nil || *q.Schema.MaxLength != 32 {
		t.Errorf("expected MaxLength=32, got %v", q.Schema.MaxLength)
	}
}

func TestParamSchema_AppliedToSpec(t *testing.T) {
	r := chi.NewRouter()
	r.Post("/tags", createTagsHandler)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test", Version: "1.0.0"}}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	op := spec.Paths["/tags"].Post
	if op == nil {
		t.Fatalf("expected POST operation for '/tags'")
	}

	// Body array constraints land on the request body schema.
	if op.RequestBody == nil {
		t.Fatalf("expected a request body")
	}
	bodySchema := op.RequestBody.Content["application/json"].Schema
	if bodySchema == nil {
		t.Fatalf("expected a request body schema")
	}
	if bodySchema.MinItems == nil || *bodySchema.MinItems != 1 {
		t.Errorf("expected body minItems=1, got %v", bodySchema.MinItems)
	}
	if bodySchema.MaxItems == nil || *bodySchema.MaxItems != 5 {
		t.Errorf("expected body maxItems=5, got %v", bodySchema.MaxItems)
	}
	if bodySchema.UniqueItems == nil || !*bodySchema.UniqueItems {
		t.Errorf("expected body uniqueItems=true, got %v", bodySchema.UniqueItems)
	}

	// String constraints land on the query parameter schema.
	var q *openapi.Parameter
	for i := range op.Parameters {
		if op.Parameters[i].In == "query" && op.Parameters[i].Name == "q" {
			q = &op.Parameters[i]
		}
	}
	if q == nil {
		t.Fatalf("expected query parameter 'q', got %+v", op.Parameters)
	}
	if q.Schema.Pattern != "^[a-z]+$" {
		t.Errorf("expected query pattern, got %q", q.Schema.Pattern)
	}
	if q.Schema.MaxLength == nil || *q.Schema.MaxLength != 32 {
		t.Errorf("expected query maxLength=32, got %v", q.Schema.MaxLength)
	}
}

// badConstraintHandler applies an array-only constraint to a string parameter,
// which must fail spec generation.
//
//	@Summary Bad constraint
//	@Param q query string false "filter"
//		@Schema
//			@MinItems 2
func badConstraintHandler(w http.ResponseWriter, r *http.Request) {}

func TestParamSchema_InvalidConstraintForTypeFails(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/bad", badConstraintHandler)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test", Version: "1.0.0"}}
	g := openapi.NewGenerator()
	if _, err := g.GenerateSpec(r, cfg); err == nil {
		t.Fatal("expected GenerateSpec to fail for minItems on a string param")
	}
}
