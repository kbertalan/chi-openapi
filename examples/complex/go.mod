module github.com/kbertalan/chi-openapi/examples/complex

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.2.4
	github.com/kbertalan/chi-openapi v0.0.0
)

require golang.org/x/mod v0.37.0 // indirect

// Use the in-repo library rather than a published version.
replace github.com/kbertalan/chi-openapi => ../..
