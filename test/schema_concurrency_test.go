// Concurrency regression tests for the schema generator. Run with -race for
// full coverage; even without -race they guard against the concurrent map
// read/write that would otherwise fatal-panic.
package openapi_test

import (
	"sync"
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
)

var raceTypes = []string{
	"TestPaginatedResponse[TestBusinessObject]",
	"TestGenericPair[string,TestBusinessObject]",
	"TestNested", "TestWithEnumField",
	"TestPaginatedResponse[TestSimple]", "TestBusinessObject",
}

// One shared generator, many goroutines, interleaving GenerateSchema + GetSchemas.
func TestRace_SharedGenerator(t *testing.T) {
	sg := NewTestSchemaGenerator()
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		tn := raceTypes[i%len(raceTypes)]
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_ = sg.GenerateSchema(name)
			_ = sg.GetSchemas()
		}(tn)
	}
	wg.Wait()
}

// Separate generators per goroutine sharing ONE TypeIndex (the externalKnownTypes case).
func TestRace_SharedTypeIndex(t *testing.T) {
	idx := openapi.BuildTypeIndex()
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		tn := raceTypes[i%len(raceTypes)]
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			g := openapi.NewSchemaGenerator(idx)
			_ = g.GenerateSchema(name)
			_ = g.GetSchemas()
		}(tn)
	}
	wg.Wait()
}
