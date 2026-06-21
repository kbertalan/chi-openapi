package api

import openapi "github.com/kbertalan/chi-openapi"

// Config returns the static OpenAPI metadata for the service. Every security
// scheme referenced by a @Security annotation must be declared here, or
// generation fails.
func Config() openapi.Config {
	return openapi.Config{
		Info: openapi.Info{
			Title:       "Complex Example API",
			Version:     "1.0.0",
			Description: "Users and orders service demonstrating chi-openapi.",
			License:     &openapi.License{Name: "MIT", Identifier: "MIT"},
		},
		Servers: []openapi.Server{
			{URL: "http://localhost:8080", Description: "Local development"},
		},
		SecuritySchemes: map[string]openapi.SecurityScheme{
			"ApiKeyAuth": {Type: "apiKey", Description: "API key sent in the X-API-Key header"},
			"BearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
		},
	}
}
