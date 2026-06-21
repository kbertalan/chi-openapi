package openapi_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	openapi "github.com/kbertalan/chi-openapi"
)

// securedMiddleware is a named middleware whose doc comment carries annotations.
// They should be merged into every operation it guards.
//
// @Summary Middleware summary (should be overridden by the handler)
// @Tags secured
// @Security BearerAuth
// @Param Authorization header string true "Bearer token"
func securedMiddleware(next http.Handler) http.Handler {
	return next
}

// securedListHandler carries its own annotations; its scalar fields win over the
// middleware's because the handler is merged last.
//
// @Summary List secured things
// @Tags listing
func securedListHandler(w http.ResponseWriter, r *http.Request) {}

func TestGenerateSpec_MiddlewareAnnotations(t *testing.T) {
	r := chi.NewRouter()
	r.Use(securedMiddleware)
	r.Get("/secured", securedListHandler)

	cfg := openapi.Config{
		Info: openapi.Info{Title: "Test Service", Version: "1.0.0"},
		SecuritySchemes: map[string]openapi.SecurityScheme{
			"BearerAuth": {Type: "http", Scheme: "bearer"},
		},
	}
	g := openapi.NewGenerator()
	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	op := spec.Paths["/secured"].Get
	if op == nil {
		t.Fatalf("expected GET operation for '/secured'")
	}

	// Handler is merged last, so its summary wins over the middleware's.
	if op.Summary != "List secured things" {
		t.Errorf("expected handler summary to win, got %q", op.Summary)
	}

	// Slice fields from middleware and handler are concatenated (order: middleware first).
	if !containsAll(op.Tags, "secured", "listing") {
		t.Errorf("expected tags from both middleware and handler, got %v", op.Tags)
	}

	// Security requirement contributed by the middleware.
	foundSec := false
	for _, req := range op.Security {
		if _, ok := req["BearerAuth"]; ok {
			foundSec = true
		}
	}
	if !foundSec {
		t.Errorf("expected BearerAuth security from middleware, got %+v", op.Security)
	}

	// Header parameter contributed by the middleware.
	foundParam := false
	for _, p := range op.Parameters {
		if p.In == "header" && p.Name == "Authorization" {
			foundParam = true
		}
	}
	if !foundParam {
		t.Errorf("expected Authorization header parameter from middleware, got %+v", op.Parameters)
	}
}

// TestGenerateSpec_RegisteredMiddlewareAnnotation covers third-party / closure
// middlewares that cannot be resolved from source and are registered explicitly.
func TestGenerateSpec_RegisteredMiddlewareAnnotation(t *testing.T) {
	// An anonymous closure middleware (e.g. a constructor-returned chi middleware)
	// whose doc comment cannot be parsed.
	thirdParty := func(next http.Handler) http.Handler { return next }

	r := chi.NewRouter()
	r.Use(thirdParty)
	r.Get("/registered", securedListHandler)

	cfg := openapi.Config{Info: openapi.Info{Title: "Test Service", Version: "1.0.0"}}
	g := openapi.NewGenerator()
	g.RegisterMiddlewareAnnotation(thirdParty, &openapi.Annotation{
		Tags: []string{"third-party"},
	})

	spec, err := g.GenerateSpec(r, cfg)
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	op := spec.Paths["/registered"].Get
	if op == nil {
		t.Fatalf("expected GET operation for '/registered'")
	}
	if !containsAll(op.Tags, "third-party", "listing") {
		t.Errorf("expected registered middleware tag merged with handler, got %v", op.Tags)
	}
}

func containsAll(haystack []string, wanted ...string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, w := range wanted {
		if !set[w] {
			return false
		}
	}
	return true
}
