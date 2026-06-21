// Package auth provides custom security middleware. Each middleware is a named
// function whose doc-comment annotations chi-openapi merges into every operation
// it guards: the @Security directive wires the scheme, and @Param documents the
// header the client must send.
package auth

import (
	"net/http"

	"github.com/kbertalan/chi-openapi/examples/complex/apierr"
)

// APIKeyHeader is the header carrying the API key.
const APIKeyHeader = "X-API-Key"

// APIKeyAuth rejects requests without a valid API key header.
//
// @Security ApiKeyAuth
// @Param X-API-Key header string true "API key issued to the client"
// @Failure 401 ErrorResponse "missing or invalid API key"
func APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(APIKeyHeader) == "" {
			apierr.Write(w, http.StatusUnauthorized, "missing API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BearerAuth rejects requests without a bearer token.
//
// @Security BearerAuth
// @Param Authorization header string true "Bearer access token"
// @Failure 401 ErrorResponse "missing or invalid bearer token"
func BearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		if h := r.Header.Get("Authorization"); len(h) <= len(prefix) || h[:len(prefix)] != prefix {
			apierr.Write(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
