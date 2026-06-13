package openapi_test

import (
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// genericNamingProbe and genericNamingProbeMulti are generic types whose method
// symbols we inspect to verify the Go runtime's naming convention for generic
// type parameters, with one and with multiple type parameters respectively.
type genericNamingProbe[T any] struct{}

func (genericNamingProbe[T]) handle(w http.ResponseWriter, r *http.Request) {}

type genericNamingProbeMulti[K comparable, V any] struct{}

func (genericNamingProbeMulti[K, V]) handle(w http.ResponseWriter, r *http.Request) {}

// TestRuntimeGenericTypeParamRepresentation guards the assumption that the Go
// runtime renders a generic receiver's type arguments as the literal "[...]" in
// runtime.Func.Name() (e.g. ".../test.genericNamingProbe[...].handle-fm"),
// regardless of how many type parameters the type declares.
//
// handler_resolution.go depends on this: extractReceiverAndMethod strips "[...]"
// to recover the base receiver type from the runtime symbol. If a future Go
// release changes the representation (for example to "[int]", "[go.shape.int]",
// or "[..., ...]" for multiple parameters), this test fails to signal that the
// stripping logic in handler_resolution.go must be updated.
func TestRuntimeGenericTypeParamRepresentation(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name:    "single type parameter",
			handler: genericNamingProbe[int]{}.handle,
			want:    "genericNamingProbe[...]",
		},
		{
			name:    "multiple type parameters",
			handler: genericNamingProbeMulti[string, int]{}.handle,
			want:    "genericNamingProbeMulti[...]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			symbol := runtime.FuncForPC(reflect.ValueOf(tc.handler).Pointer()).Name()
			if !strings.Contains(symbol, tc.want) {
				t.Fatalf("Go runtime no longer renders generic type parameters as %q: got symbol %q.\n"+
					"The %q stripping in handler_resolution.go (extractReceiverAndMethod) must be updated to match.",
					"[...]", symbol, "[...]")
			}
		})
	}
}
