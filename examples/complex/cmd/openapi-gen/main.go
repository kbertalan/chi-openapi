// Command openapi-gen generates the OpenAPI document at build time and writes it
// to disk so it can be embedded into the server binary. It is invoked by the
// `//go:generate` directive in the module's main.go:
//
//	go generate ./...
//
// Because it runs as a normal program in the module root, the chi-openapi source
// scanner sees the full source tree and resolves every annotation and type.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/kbertalan/chi-openapi/examples/complex/api"
)

func main() {
	out := flag.String("o", "openapi.json", "output path for the generated spec")
	flag.Parse()

	spec, err := api.Spec()
	if err != nil {
		log.Fatalf("generate spec: %v", err)
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create %s: %v", *out, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		log.Fatalf("encode spec: %v", err)
	}

	log.Printf("wrote %s", *out)
}
