# Complex example

A small users + orders service that exercises most of chi-openapi in a realistic
layout: multiple subpackages, custom security middleware, stock chi middleware,
cross-package schema references, and a string enum.

It also shows the **two ways to obtain the OpenAPI document**:

| Endpoint              | Source                              | When to use                                   |
| --------------------- | ----------------------------------- | --------------------------------------------- |
| `GET /openapi.json`   | generated **live** per request      | development — needs the `.go` sources present |
| `GET /docs/openapi.json` | served from the **embedded** spec | production — baked into the binary, no source |

chi-openapi derives schemas and annotations by parsing the source tree at
runtime. The live endpoint therefore only works where the sources exist (e.g.
`go run .`). The build-time generator writes [`openapi.json`](./openapi.json),
which is embedded via `//go:embed` and ships in the binary.

## Layout

```
api/         composition root: BuildRouter(), Config(), Spec() — shared by server and generator
auth/        custom security middleware (API key + bearer), annotated so @Security/@Param merge in
apierr/      the one shared ErrorResponse type referenced by every @Failure
pagination/  generic PaginatedResponse[T] envelope used by the list endpoints
users/       users subpackage: annotated method-receiver handlers + tag-annotated models
orders/      orders subpackage; Order references users.User (cross-package $ref) and an enum
cmd/openapi-gen/  build-time generator invoked by `go generate`
main.go      server; mounts the live + embedded doc endpoints; holds the //go:embed
```

`BuildRouter()` is the single source of truth for routes, so the served API and
the generated document never drift. The documentation endpoints are added in
`main.go` (not `BuildRouter`) so they stay out of the spec — and because routes
containing `/openapi` are skipped by default anyway.

## Run

```sh
go run .                       # serves on :8080
curl localhost:8080/openapi.json        # live
curl localhost:8080/docs/openapi.json   # embedded (build-time)

curl localhost:8080/users/                       # 401 — needs an API key
curl -H 'X-API-Key: secret' localhost:8080/users/   # 200
```

## Regenerate the embedded spec

```sh
go generate ./...   # runs cmd/openapi-gen, rewrites openapi.json
```

## What to look at

- **Multi-package schemas** — models are keyed by package name (`users.User`,
  `orders.Order`). `orders.Order` embeds a `users.User`, producing a `$ref`
  across packages.
- **Custom security middleware** (`auth/`) — the `@Security` and `@Param`
  annotations on `APIKeyAuth`/`BearerAuth` merge into every operation they guard.
- **Third-party middleware** — chi's `Recoverer` is a closure that can't be
  source-parsed, so `api.NewGenerator` documents it with
  `RegisterMiddlewareAnnotation` (it contributes the `500` response everywhere).
- **Enum** — `orders.OrderStatus` is a string enum detected from its `const`
  block.
- **Schema tags** — model fields carry `openapi:"…"` tags that
  become JSON-schema constraints: `users.User`/`CreateUserRequest` get `format:
  email`, `readOnly`, `example`, `minLength`/`maxLength`; `orders.OrderItem` gets
  a `pattern` and numeric `minimum`/`maximum`/`default`.
- **Generic paginated responses** — list endpoints return
  `pagination.PaginatedResponse[T]`. The `@Success 200 PaginatedResponse[users.User]`
  annotation monomorphizes into a dedicated component
  (`pagination.PaginatedResponse-users.User`) whose `items` array `$ref`s the
  element type. `orders.List` uses the same generic with `orders.Order`.
- **Request bodies / responses** — `@Accept`, `@Param … body`, `@Success`,
  `@Failure`, and `@See` on the handlers.
