# chi-openapi

chi-openapi is an annotation-driven OpenAPI 3.1 specification generator for Go HTTP services using the Chi router. It automatically generates comprehensive API documentation from your Go code with minimal configuration.

**About This Project**: chi-openapi is forked from ![annot8](https://github.com/AxelTahmid/annot8), which was extracted from a larger application with 340+ models and is focused on generating OpenAPI 3.1 documentation from Go code. The project favors pragmatic, working output over exhaustive edge-case coverage.

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.25-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## Install

```sh
go get github.com/kbertalan/chi-openapi
```

Requires Go ≥ 1.25.

## Quick start

Annotate handlers with doc-comment directives, then generate a spec from the router.

```go
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	openapi "github.com/kbertalan/chi-openapi"
)

// @Summary Greet the world
// @Tags hello
// @Success 200 GreetResponse "a greeting"
func greet(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"message":"hello"}`))
}

type GreetResponse struct {
	Message string `json:"message"`
}

func main() {
	r := chi.NewRouter()
	r.Get("/greet", greet)

	cfg := openapi.Config{Info: openapi.Info{Title: "Hello API", Version: "1.0.0"}}
	spec, _ := openapi.NewGenerator().GenerateSpec(r, cfg)
	_ = spec // marshal to JSON, serve, or write to disk
}
```

> ⚠️ This in-process call parses your `.go` source files at runtime — it only works where the
> source tree is present. For binaries, generate at build time. See
> [Generating the spec](#generating-the-spec).

## Annotating handlers

Directives live in the handler's doc comment.

| Directive | Syntax | Purpose |
| --- | --- | --- |
| `@Summary` | `@Summary <text>` | Short operation title |
| `@Description` | `@Description <text>` | Long description |
| `@Tags` | `@Tags <a> <b>` | Group operations |
| `@Accept` | `@Accept <mime>` | Request content type |
| `@Param` | `@Param <name> <in> <type> <required> "<desc>"` | Path/query/header/body parameter |
| `@Success` | `@Success <code> <Type> "<desc>"` | Success response + body type |
| `@Failure` | `@Failure <code> <Type> "<desc>"` | Error response + body type |
| `@Security` | `@Security <scheme>` | Require a security scheme |
| `@See` | `@See <url> "<desc>"` | Link to external docs |

`<in>` is one of `path`, `query`, `header`, `body`. A `body` param names the request body type.

```go
// @Summary Create a user
// @Tags users
// @Accept application/json
// @Param body body CreateUserRequest true "user to create"
// @Success 201 User "the created user"
// @Failure 400 ErrorResponse "invalid payload"
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) { ... }
```

## Describing models with `openapi` struct tags

Field tags in the `openapi` dialect become JSON-schema constraints.

| Key | Example | Key | Example |
| --- | --- | --- | --- |
| `format` | `format=email` | `pattern` | `pattern=^[A-Z]{3}$` |
| `description` | `description=...` | `default` | `default=1` |
| `title` | `title=...` | `example` | `example=Ada` |
| `minLength` / `maxLength` | `minLength=2` | `minimum` / `maximum` | `minimum=1` |
| `exclusiveMinimum` / `exclusiveMaximum` | `exclusiveMinimum=0` | `minItems` / `maxItems` | `minItems=1` |
| `uniqueItems` | `uniqueItems=true` | `enum` | `` enum=a\|b\|c `` |
| `readOnly` | `readOnly=true` | `writeOnly` | `writeOnly=true` |
| `deprecated` | `deprecated=true` | | |

```go
type User struct {
	ID    string `json:"id" openapi:"readOnly=true,description=Server-assigned identifier"`
	Email string `json:"email" openapi:"format=email"`
	Name  string `json:"name" openapi:"minLength=2,maxLength=64,example=Ada Lovelace"`
}

type OrderItem struct {
	SKU      string `json:"sku" openapi:"pattern=^[A-Z]{3}-[0-9]{4}$,example=ABC-0001"`
	Quantity int    `json:"quantity" openapi:"minimum=1,maximum=999,default=1"`
}
```

## Advanced schemas

**Enums** — a string-based type with a `const` block is detected and emitted as an `enum`.

```go
type OrderStatus string

const (
	OrderStatusPending OrderStatus = "pending"
	OrderStatusPaid    OrderStatus = "paid"
	OrderStatusShipped OrderStatus = "shipped"
)
```

**Cross-package `$ref`** — embedding a type from another package links the schemas with a `$ref`.

```go
type Order struct {
	ID       string     `json:"id"`
	Customer users.User `json:"customer"` // → $ref to users.User
}
```

**Generics** — each instantiation monomorphizes into its own component schema.

```go
type PaginatedResponse[T any] struct {
	Items      []T `json:"items"`
	TotalItems int `json:"totalItems"`
}

// @Success 200 PaginatedResponse[users.User] "page of users"
```

## Security

Declare schemes in `Config`; every scheme referenced by a `@Security` directive must be declared or generation fails.

```go
openapi.Config{
	SecuritySchemes: map[string]openapi.SecurityScheme{
		"ApiKeyAuth": {Type: "apiKey", Description: "API key in X-API-Key header"},
		"BearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	},
}
```

Annotations on a **named middleware function** merge into every operation it guards.

```go
// @Security ApiKeyAuth
// @Param X-API-Key header string true "API key issued to the client"
// @Failure 401 ErrorResponse "missing or invalid API key"
func APIKeyAuth(next http.Handler) http.Handler { ... }
```

## Middleware that can't be parsed

Closures and third-party middleware (e.g. chi's `Recoverer`) have no source comment to read. Document them programmatically — the annotation applies to every route the middleware guards.

```go
g := openapi.NewGenerator()
g.RegisterMiddlewareAnnotation(middleware.Recoverer, &openapi.Annotation{
	Failures: []openapi.ErrorResponse{
		{StatusCode: 500, Type: "ErrorResponse", Produce: []string{"application/json"}, Description: "internal server error"},
	},
})
```

## When routes or handlers aren't discovered

chi-openapi walks the router via chi.Walk, then reads each handler's annotations by
**parsing the `.go` file at the path the runtime reports**. Go reflection does not expose doc
comments — only the source path — so these cases produce a route with no documentation (most are
logged at `warn`; check your logs):

| Case | What happens | Fix |
| --- | --- | --- |
| **Source not at the recorded path at runtime** | Routes are walked, but comment text can't be read | Generate at build time and embed the spec (see below) |
| **Nil router** | `InspectRoutes` returns a `RouteDiscoveryError` | Pass a built router |
| **Inline closure handler** (`r.Get("/", func(w, r){…})`) | Anonymous func can't be resolved to source | Use a named function or method-receiver handler |
| **Handler isn't a func value** | Only `HandlerFunc`-style funcs/methods are supported | Use a named handler |
| **Closure middleware** (constructor-returned) | `@Security`/`@Param` are dropped | `RegisterMiddlewareAnnotation` |
| **Ambiguous receiver+method** | Same `(*T).Method` in several files, route can't disambiguate | Align file/package/route names, or rename |
| **`/openapi`, `/swagger` paths** | Skipped by default to avoid self-reference | Override the skip list |

## Generating the spec

Generation parses your Go sources at runtime, so **how** you generate determines whether it works
in production.

### ✅ Build time — the production-safe path

Generate the spec during your build and ship the JSON, not the generator. This is the **only
approach that works in a compiled binary**, because the sources it parses are no longer present at
runtime.

Add a generator command and a `go:generate` directive:

```go
//go:generate go run ./cmd/openapi-gen -o openapi.json

//go:embed openapi.json
var embeddedSpec []byte
```

```go
// cmd/openapi-gen/main.go
func main() {
	spec, err := api.Spec() // builds the router + calls GenerateSpec
	if err != nil {
		log.Fatal(err)
	}
	out, _ := os.Create("openapi.json")
	defer out.Close()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(spec)
}
```

```sh
go generate ./...   # regenerate openapi.json whenever routes change
```

The committed `openapi.json` is embedded with `//go:embed` and served straight from the binary —
no source tree, no parsing at runtime.

`GenerateOpenAPISpecFile` is a one-call helper that does the build + write:

```go
openapi.GenerateOpenAPISpecFile(&openapi.GenerateParams{
	Router:   api.BuildRouter(),
	Config:   api.Config(),
	FilePath: "openapi.json",
})
```

### ⚠️ Live / on-the-fly — development only

Calling `GenerateSpec` per request scans the source tree on each call. It is convenient with
`go run .`, but **it will not work in a binary** with no sources — the spec comes back without any
annotations. Never rely on it in production.

## Full example

[`examples/complex/`](./examples/complex/) is a runnable users + orders service exercising
everything above: multi-package schemas, cross-package `$ref`, generics, enums, custom security
middleware, and both the build-time and live generation paths.
