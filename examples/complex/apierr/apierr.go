// Package apierr defines the single error body returned across the whole API.
//
// Keeping one shared type means every @Failure annotation can reference the
// unqualified name "ErrorResponse" and resolve to this one definition.
package apierr

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the JSON body returned for any non-2xx response.
type ErrorResponse struct {
	Code    int    `json:"code"`    // mirrors the HTTP status code
	Message string `json:"message"` // human-readable explanation
}

// Write encodes an ErrorResponse with the given status code.
func Write(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Code: status, Message: message})
}
