// Package api is the composition root: it wires middleware, subrouters and
// security into a single chi.Router, and knows how to generate the spec for it.
// Both the server and the cmd/openapi-gen build-time generator depend on it, so
// the served API and the generated document never drift apart.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	openapi "github.com/kbertalan/chi-openapi"
	"github.com/kbertalan/chi-openapi/examples/complex/auth"
	"github.com/kbertalan/chi-openapi/examples/complex/orders"
	"github.com/kbertalan/chi-openapi/examples/complex/users"
)

// BuildRouter assembles the full API router. It contains only API routes; the
// documentation endpoints are added by the server (see main.go) so they stay
// out of the generated spec.
func BuildRouter() chi.Router {
	r := chi.NewRouter()

	// Stock chi middleware applied to every route. These are closures/third-party
	// functions whose source comments cannot be parsed; their documentation is
	// supplied programmatically in NewGenerator below.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Unauthenticated liveness probe.
	r.Get("/healthz", health)

	// Users require an API key.
	r.Route("/users", func(r chi.Router) {
		r.Use(auth.APIKeyAuth)
		users.NewHandler().Register(r)
	})

	// Orders require a bearer token.
	r.Route("/orders", func(r chi.Router) {
		r.Use(auth.BearerAuth)
		orders.NewHandler().Register(r)
	})

	return r
}

// health is a trivial liveness handler.
//
// @Summary Liveness probe
// @Description Always returns 200 while the service is up.
// @Tags system
// @Success 200 HealthResponse "service is healthy"
func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// HealthResponse is the body returned by the liveness probe.
type HealthResponse struct {
	Status string `json:"status"`
}

// NewGenerator returns a generator pre-loaded with documentation for the
// third-party chi middleware used by BuildRouter. RegisterMiddlewareAnnotation
// matches by code pointer, so the Recoverer annotation applies to every route it
// guards — here, documenting the 500 response every endpoint may return.
func NewGenerator() *openapi.Generator {
	g := openapi.NewGenerator()
	g.RegisterMiddlewareAnnotation(middleware.Recoverer, &openapi.Annotation{
		Failures: []openapi.ErrorResponse{
			{StatusCode: 500, Type: "ErrorResponse", Produce: []string{"application/json"}, Description: "internal server error"},
		},
	})
	return g
}

// Spec generates the OpenAPI spec for the API router by scanning the source
// tree. It is shared by the live endpoint and the build-time generator.
func Spec() (openapi.Spec, error) {
	return NewGenerator().GenerateSpec(BuildRouter(), Config())
}
