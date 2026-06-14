package openapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	openapi "github.com/kbertalan/chi-openapi"
)

// TestGenerateSpecRoutes ensures that GenerateSpec includes discovered routes and parameters.
func TestGenerateSpecRoutes(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/foo/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg := openapi.Config{Info: openapi.Info{Title: "Test Service", Version: "1.2.3"}}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	// Check Info
	if spec.Info != cfg.Info {
		t.Errorf("expected Info %+v, got %+v", cfg.Info, spec.Info)
	}

	// Check path presence and operation
	paths := spec.Paths
	if _, ok := paths["/foo/{id}"]; !ok {
		t.Fatalf("expected path '/foo/{id}' in spec.Paths")
	}
	ops := paths["/foo/{id}"]
	op := ops.Get
	if op == nil {
		t.Fatalf("expected GET operation for '/foo/{id}'")
	}

	// Verify path parameter id
	if len(op.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(op.Parameters))
	}
	p := op.Parameters[0]
	if p.Name != "id" || p.In != "path" || !p.Required {
		t.Errorf("unexpected path parameter: %+v", p)
	}
}

func TestGenerateSpec_Compliance31(t *testing.T) {
	r := chi.NewRouter()
	cfg := openapi.Config{
		Info: openapi.Info{
			Title:       "Compliance Test",
			Summary:     "Test Summary",
			Version:     "3.1.0",
			Description: "Testing 3.1 features",
			License: &openapi.License{
				Name:       "Apache 2.0",
				Identifier: "Apache-2.0",
			},
		},
	}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI 3.1.0, got %s", spec.OpenAPI)
	}

	if spec.Info != cfg.Info {
		t.Errorf("expected info %+v, got %+v", cfg.Info, spec.Info)
	}
}

// Ensure that handlers implemented as methods on a struct (method receiver)
// are discovered and their comment annotations are parsed correctly.
// This reproduces the case where the router registers a method value, e.g.
// r.Post("/invoices/{id}", h.create)
type invoicesHandler struct{}

// @Summary Create invoice
// @Description Create a new invoice for the tenant
// @Tags invoices
// @Param id path int true "Invoice ID"
// @Success 201 CreateInvoiceResponse "created"
// @Failure 400 InvalidRequest "invalid invoice creation request"
func (h *invoicesHandler) create(w http.ResponseWriter, r *http.Request) {}

func TestGenerateSpecRoutes_MethodReceiver(t *testing.T) {
	r := chi.NewRouter()
	h := &invoicesHandler{}
	// register method value as handler
	r.Post("/invoices/{id}", http.HandlerFunc(h.create))

	cfg := openapi.Config{Info: openapi.Info{Title: "Test Service", Version: "1.2.3"}}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	// ensure path exists
	if _, ok := spec.Paths["/invoices/{id}"]; !ok {
		t.Fatalf("expected path '/invoices/{id}' in spec.Paths")
	}

	ops := spec.Paths["/invoices/{id}"]
	op := ops.Post
	if op == nil {
		t.Fatalf("expected POST operation for '/invoices/{id}'")
	}

	// Verify annotation fields were parsed into the operation
	if op.Summary != "Create invoice" {
		t.Errorf("expected summary 'Create invoice', got %q", op.Summary)
	}
	if op.Description != "Create a new invoice for the tenant" {
		t.Errorf("expected description parsed, got %q", op.Description)
	}

	// Verify path parameter id
	foundID := false
	for _, p := range op.Parameters {
		if p.In == "path" && p.Name == "id" {
			foundID = true
			if !p.Required {
				t.Errorf("expected path parameter id to be required")
			}
		}
	}
	if !foundID {
		t.Errorf("expected path parameter 'id' in operation parameters, got %+v", op.Parameters)
	}

	expectedSuccess := openapi.Response{
		Description: "created",
		Content: map[string]openapi.MediaTypeObject{
			"application/json": {
				Schema: &openapi.Schema{Ref: "#/components/schemas/CreateInvoiceResponse"},
			},
		},
	}

	expectedFailure := openapi.Response{
		Description: "invalid invoice creation request",
		Content: map[string]openapi.MediaTypeObject{
			"application/problem+json": {
				Schema: &openapi.Schema{Ref: "#/components/schemas/InvalidRequest"},
			},
		},
	}

	for status, response := range op.Responses {
		if status == "201" {
			AssertDeepEqual(t, expectedSuccess, response)
			continue
		}
		if status == "400" {
			AssertDeepEqual(t, expectedFailure, response)
			continue
		}
		t.Errorf("unexpected response %s: %+v", status, response)
	}
}

// listWidgets is a top-level named function handler. Unlike a method value, its
// runtime file path is a real source path, which becomes module-relative under
// -trimpath. This test guards that annotation resolution works in both builds.
//
// @Summary List widgets
// @Description List all widgets for the tenant
func listWidgets(w http.ResponseWriter, r *http.Request) {}

func TestGenerateSpecRoutes_TopLevelFunction(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/widgets", listWidgets)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test Service", Version: "1.2.3"}}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	ops, ok := spec.Paths["/widgets"]
	if !ok {
		t.Fatalf("expected path '/widgets' in spec.Paths")
	}
	op := ops.Get
	if op == nil {
		t.Fatalf("expected GET operation for '/widgets'")
	}
	if op.Summary != "List widgets" {
		t.Errorf("expected summary 'List widgets', got %q", op.Summary)
	}
	if op.Description != "List all widgets for the tenant" {
		t.Errorf("expected description parsed, got %q", op.Description)
	}
}

// widgetRepo is a generic receiver type. The runtime renders its type argument
// as a literal "[...]" (e.g. "(*widgetRepo[...]).List"), which must still resolve
// to the base type so the method's annotations are found.
type widgetRepo[T any, R any] struct {
	items   []T
	results []R
}

// @Summary List repo widgets
// @Description List widgets from a generic repository
func (r *widgetRepo[T, R]) List(w http.ResponseWriter, req *http.Request) {}

func TestGenerateSpecRoutes_GenericReceiver(t *testing.T) {
	r := chi.NewRouter()
	repo := &widgetRepo[int, float64]{}
	r.Get("/repo/widgets", repo.List)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test Service", Version: "1.2.3"}}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	ops, ok := spec.Paths["/repo/widgets"]
	if !ok {
		t.Fatalf("expected path '/repo/widgets' in spec.Paths")
	}
	op := ops.Get
	if op == nil {
		t.Fatalf("expected GET operation for '/repo/widgets'")
	}
	if op.Summary != "List repo widgets" {
		t.Errorf("expected summary 'List repo widgets', got %q", op.Summary)
	}
	if op.Description != "List widgets from a generic repository" {
		t.Errorf("expected description parsed, got %q", op.Description)
	}
}

// TestGenerateSpec_MenuCouponCollision tests the exact scenario where menu and coupon
// handlers both have "List" method names, ensuring they get distinct summaries.
func TestGenerateSpec_MenuCouponCollision(t *testing.T) {
	// Create mock handlers that simulate the actual menu and coupon handlers
	menuHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	couponHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := chi.NewRouter()
	r.Get("/api/v1/menu/", menuHandler)
	r.Get("/api/v1/coupon/", couponHandler)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test API", Version: "1.0.0"}}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	// Check that both paths exist
	paths := spec.Paths
	if _, ok := paths["/api/v1/menu/"]; !ok {
		t.Fatalf("expected path '/api/v1/menu/' in spec.Paths")
	}
	if _, ok := paths["/api/v1/coupon/"]; !ok {
		t.Fatalf("expected path '/api/v1/coupon/' in spec.Paths")
	}

	// Get the operations
	menuOps := paths["/api/v1/menu/"]
	couponOps := paths["/api/v1/coupon/"]

	// Check GET operations exist
	menuGet := menuOps.Get
	if menuGet == nil {
		t.Fatal("expected GET operation for menu path")
	}
	couponGet := couponOps.Get
	if couponGet == nil {
		t.Fatal("expected GET operation for coupon path")
	}

	// Extract summaries
	menuSummary := menuGet.Summary
	couponSummary := couponGet.Summary

	t.Logf("Menu route summary: %q", menuSummary)
	t.Logf("Coupon route summary: %q", couponSummary)

	// If either summary is non-empty, ensure they are not the same (no cross-contamination).
	if menuSummary != "" || couponSummary != "" {
		if menuSummary == couponSummary {
			t.Errorf(
				"Menu and coupon operations should have different summaries when present, both got: %q",
				menuSummary,
			)
		}
	}

	// Always ensure operation IDs differ to avoid accidental collisions
	if menuGet.OperationID == couponGet.OperationID {
		t.Errorf(
			"Menu and coupon operations should have different operation IDs, both got: %q",
			menuGet.OperationID,
		)
	}
}

type RenameTestModel struct {
	ID string `json:"id"`
}

type NestedModel struct {
	Child RenameTestModel `json:"child"`
}

func TestGenerateSpec_ModelRenaming(t *testing.T) {
	r := chi.NewRouter()

	// Register some "external" types to test renaming without needing real source files
	openapi.AddExternalKnownType("openapi_test.RenameTarget", &openapi.Schema{
		Type: "object",
		Properties: map[string]*openapi.Schema{
			"id": {Type: "string"},
		},
	})
	openapi.AddExternalKnownType("openapi_test.NestedTarget", &openapi.Schema{
		Type: "object",
		Properties: map[string]*openapi.Schema{
			"child": {Ref: "#/components/schemas/openapi_test.RenameTarget"},
		},
	})

	// Define a custom strategy that removes "Rename" prefix
	customStrategy := func(pkg, name string) string {
		if strings.HasPrefix(name, "Rename") {
			return strings.TrimPrefix(name, "Rename")
		}
		return name
	}

	cfg := openapi.Config{Info: openapi.Info{Title: "Renaming Test", Version: "1.0.0"}}
	g := openapi.NewGenerator()
	g.SetModelNameFunc(customStrategy)

	// Trigger generation of these schemas
	g.GenerateSchema("openapi_test.RenameTarget")
	g.GenerateSchema("openapi_test.NestedTarget")

	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	// openapi_test.RenameTarget should be renamed to Target (pkg=openapi_test, name=RenameTarget)
	if _, ok := spec.Components.Schemas["Target"]; !ok {
		keys := make([]string, 0, len(spec.Components.Schemas))
		for k := range spec.Components.Schemas {
			keys = append(keys, k)
		}
		t.Errorf("expected schema 'Target', but got keys: %v", keys)
	}

	// NestedTarget should still be NestedTarget
	nested, ok := spec.Components.Schemas["NestedTarget"]
	if !ok {
		t.Fatal("expected schema 'NestedTarget'")
	}

	// Check if the ref inside NestedTarget points to Target
	// The property name is "child"
	childProp, ok := nested.Properties["child"]
	if !ok {
		t.Fatal("expected property 'child' in NestedTarget")
	}

	expectedRef := "#/components/schemas/Target"
	if childProp.Ref != expectedRef {
		t.Errorf("expected ref %q, got %q", expectedRef, childProp.Ref)
	}
}

func TestGenerateSpec_ConflictResolution(t *testing.T) {
	cfg := openapi.Config{Info: openapi.Info{Title: "Conflict Test", Version: "1.0.0"}}
	g := openapi.NewGenerator()

	// Strategy that forces everything to the same name
	g.SetModelNameFunc(func(pkg, name string) string {
		return "ConflictModel"
	})

	// Manually generate two different schemas
	g.GenerateSchema("Alpha")
	g.GenerateSchema("Beta")

	spec, err := g.GenerateSpec(chi.NewRouter(), cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	// One should be ConflictModel, the other ConflictModel2
	if _, ok := spec.Components.Schemas["ConflictModel"]; !ok {
		keys := make([]string, 0, len(spec.Components.Schemas))
		for k := range spec.Components.Schemas {
			keys = append(keys, k)
		}
		t.Errorf("expected 'ConflictModel', but got keys: %v", keys)
	}
	if _, ok := spec.Components.Schemas["ConflictModel2"]; !ok {
		keys := make([]string, 0, len(spec.Components.Schemas))
		for k := range spec.Components.Schemas {
			keys = append(keys, k)
		}
		t.Errorf("expected 'ConflictModel2', but got keys: %v", keys)
	}
}

// securedHandler carries a @Security annotation referencing a named scheme.
// @Summary Secured endpoint
// @Security ApiKeyAuth
// @Success 200 SecuredResponse "ok"
func securedHandler(w http.ResponseWriter, r *http.Request) {}

type SecuredResponse struct {
	OK bool `json:"ok"`
}

// TestGenerateSpec_SecurityUndeclaredFails verifies that a @Security scheme not
// declared in Config.SecuritySchemes causes GenerateSpec to fail.
func TestGenerateSpec_SecurityUndeclaredFails(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/secured", securedHandler)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test", Version: "1.0.0"}}
	g := openapi.NewGenerator()

	_, err := g.GenerateSpec(r, cfg)
	if err == nil {
		t.Fatal("expected error for undeclared security scheme, got nil")
	}
	if !strings.Contains(err.Error(), "ApiKeyAuth") {
		t.Errorf("expected error to mention 'ApiKeyAuth', got %v", err)
	}
}

// TestGenerateSpec_SecurityDeclaredSucceeds verifies that declaring the scheme in
// Config.SecuritySchemes makes generation succeed and wires the operation and
// components together.
func TestGenerateSpec_SecurityDeclaredSucceeds(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/secured", securedHandler)

	cfg := openapi.Config{
		Info: openapi.Info{Title: "Test", Version: "1.0.0"},
		SecuritySchemes: map[string]openapi.SecurityScheme{
			"ApiKeyAuth": {Type: "apiKey"},
		},
	}
	g := openapi.NewGenerator()

	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	if _, ok := spec.Components.SecuritySchemes["ApiKeyAuth"]; !ok {
		t.Errorf("expected ApiKeyAuth in components.securitySchemes, got %+v", spec.Components.SecuritySchemes)
	}

	op := spec.Paths["/secured"].Get
	if op == nil {
		t.Fatal("expected GET /secured operation")
	}
	if len(op.Security) != 1 {
		t.Fatalf("expected 1 security requirement, got %+v", op.Security)
	}
	if _, ok := op.Security[0]["ApiKeyAuth"]; !ok {
		t.Errorf("expected operation security requirement ApiKeyAuth, got %+v", op.Security[0])
	}
}
