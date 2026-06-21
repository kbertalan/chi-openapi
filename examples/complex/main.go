// Command complex is the example server. It demonstrates two ways to expose the
// OpenAPI document:
//
//   - GET /openapi.json      generated live from the source tree (development only;
//     see the WARNING in main where the route is served).
//   - GET /docs/openapi.json served from the spec embedded at build time. It
//     needs no source at runtime and is safe for a production binary.
//
// Regenerate the embedded document with `go generate ./...`.
package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"

	"github.com/kbertalan/chi-openapi/examples/complex/api"
)

//go:generate go run ./cmd/openapi-gen -o openapi.json

// embeddedSpec is the build-time generated document. It lives in the server
// (not the api package) so that cmd/openapi-gen, which imports api, still builds
// when openapi.json is absent — avoiding an embed bootstrap cycle.
//
//go:embed openapi.json
var embeddedSpec []byte

func main() {
	r := api.BuildRouter()

	// Live spec: regenerated per request by scanning source. Its own path
	// contains "/openapi", so chi-openapi skips it from the document by default.
	//
	// WARNING: development only. This parses the .go sources at request time, so it
	// returns an unannotated (effectively empty) spec from a compiled binary where
	// the sources are absent. Production must serve the embedded spec below.
	r.Get("/openapi.json", liveSpec)

	// Embedded spec: served straight from the binary.
	r.Get("/docs/openapi.json", embeddedSpecHandler)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func liveSpec(w http.ResponseWriter, r *http.Request) {
	spec, err := api.Spec()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(spec)
}

func embeddedSpecHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(embeddedSpec)
}
