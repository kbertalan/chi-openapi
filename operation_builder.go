package openapi

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// buildOperation turns a Chi route into an OpenAPI operation.
func (g *Generator) buildOperation(
	handler http.Handler,
	route, method string,
) Operation {
	slog.Debug("[openapi] buildOperation: called", "route", route, "method", method)

	handlerInfo := g.extractHandlerInfo(handler, route)

	var annotations *Annotation
	if handlerInfo != nil && handlerInfo.File != "" {
		var err error
		annotations, err = ParseAnnotations(handlerInfo.File, handlerInfo.FunctionName)
		if err != nil {
			slog.Warn("[openapi] buildOperation: annotations parse error", "error", err)
		}
	}

	op := Operation{
		OperationID: generateOperationID(method, route),
		Responses:   g.buildResponses(annotations),
	}

	// Merge path parameters derived from the route itself.
	op.Parameters = append(op.Parameters, extractPathParameters(route)...)

	// Apply annotation-derived metadata.
	if annotations != nil {
		op.Summary = annotations.Summary
		op.Description = annotations.Description
		op.Tags = append(op.Tags, annotations.Tags...)

		if annotations.See != nil {
			op.ExternalDocs = &ExternalDocumentation{
				URL:         annotations.See.URL,
				Description: annotations.See.Description,
			}
		}

		for _, sec := range annotations.Security {
			scopes := sec.Scopes
			if scopes == nil {
				scopes = []string{}
			}
			op.Security = append(op.Security, SecurityRequirement{sec.Scheme: scopes})
		}

		for _, param := range annotations.Parameters {
			if param.In == "body" {
				continue
			}
			op.Parameters = upsertParameter(op.Parameters, Parameter{
				Name:        param.Name,
				In:          param.In,
				Description: param.Description,
				Required:    param.Required,
				Schema:      g.schemaGen.GenerateSchema(param.Type),
			})
		}

		if annotations.Success != nil && annotations.Success.Description != "" {
			if success := op.Responses[strconv.Itoa(annotations.Success.StatusCode)]; success.Description == "" {
				success.Description = annotations.Success.Description
				op.Responses[strconv.Itoa(annotations.Success.StatusCode)] = success
			}
		}
	}

	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		op.RequestBody = g.buildRequestBody(annotations)
	}

	slog.Debug("[openapi] buildOperation: completed", "operationId", op.OperationID)
	return op
}

// buildResponses assembles HTTP responses using annotations as hints.
func (g *Generator) buildResponses(annotations *Annotation) map[string]Response {
	slog.Debug("[openapi] buildResponses: called")

	responses := make(map[string]Response)

	if annotations != nil && annotations.Success != nil {
		statusCode := strconv.Itoa(annotations.Success.StatusCode)

		schema := g.schemaGen.GenerateSchema(annotations.Success.DataType)

		responses[statusCode] = Response{
			Description: annotations.Success.Description,
			Content:     contentMap(annotations.Success.Produce, "application/json", schema),
		}
	} else {
		responses["200"] = Response{
			Description: "Successful response",
			Content: map[string]MediaTypeObject{
				"application/json": {Schema: &Schema{Type: "object"}},
			},
		}
	}

	if annotations != nil {
		for _, failure := range annotations.Failures {
			statusCode := strconv.Itoa(failure.StatusCode)
			schema := g.schemaGen.GenerateSchema(failure.Type)
			responses[statusCode] = Response{
				Description: failure.Description,
				Content:     contentMap(failure.Produce, "application/problem+json", schema),
			}
		}
	}

	slog.Debug("[openapi] buildResponses: completed", "response_count", len(responses))
	return responses
}

// contentMap builds a content map keyed by the given media types, all sharing
// schema. When contentTypes is empty, defaultType is used as the single key.
func contentMap(contentTypes []string, defaultType string, schema *Schema) map[string]MediaTypeObject {
	if len(contentTypes) == 0 {
		contentTypes = []string{defaultType}
	}
	content := make(map[string]MediaTypeObject, len(contentTypes))
	for _, ct := range contentTypes {
		content[ct] = MediaTypeObject{Schema: schema}
	}
	return content
}

// buildRequestBody constructs a request body definition.
func (g *Generator) buildRequestBody(annotations *Annotation) *RequestBody {
	slog.Debug("[openapi] buildRequestBody: called")

	var (
		schema      *Schema
		description = "Request body"
		// Required defaults to true; a pointer-typed body parameter marks the
		// body as optional.
		required = true
	)

	if annotations != nil {
		for _, param := range annotations.Parameters {
			if param.In != "body" {
				continue
			}
			slog.Debug("[openapi] buildRequestBody: found body parameter", "type", param.Type)

			// A pointer data type (e.g. "*CreateRequest") denotes an optional body.
			dataType := param.Type
			if strings.HasPrefix(dataType, "*") {
				required = false
				dataType = strings.TrimPrefix(dataType, "*")
			}

			schema = g.schemaGen.GenerateSchema(dataType)
			if param.Description != "" {
				description = param.Description
			}
			break
		}
	}

	if schema == nil {
		slog.Debug("[openapi] buildRequestBody: no body parameter found, using default object schema")
		schema = &Schema{Type: "object"}
	}

	// Derive the accepted content types from @Accept, defaulting to JSON.
	var accept []string
	if annotations != nil {
		accept = annotations.Accept
	}

	return &RequestBody{
		Description: description,
		Required:    required,
		Content:     contentMap(accept, "application/json", schema),
	}
}

// buildTags produces tag entries sorted for determinism.
func (g *Generator) buildTags(tagNames map[string]bool) []Tag {
	slog.Debug("[openapi] buildTags: called", "tag_count", len(tagNames))

	var tags []Tag
	for name := range tagNames {
		tags = append(tags, Tag{
			Name:        name,
			Description: capitalize(name) + " related operations",
		})
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags
}

// convertRouteToOpenAPIPath removes regex constraints from Chi-style parameters.
func convertRouteToOpenAPIPath(route string) string {
	parts := strings.Split(route, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			inner := strings.Trim(part, "{}")
			if colon := strings.Index(inner, ":"); colon != -1 {
				parts[i] = "{" + inner[:colon] + "}"
			}
		}
	}
	return strings.Join(parts, "/")
}

// extractPathParameters converts route parameters into OpenAPI parameters.
func extractPathParameters(route string) []Parameter {
	var params []Parameter

	for _, part := range strings.Split(route, "/") {
		if !strings.HasPrefix(part, "{") || !strings.HasSuffix(part, "}") {
			continue
		}

		paramName := strings.Trim(part, "{}")
		if colon := strings.Index(paramName, ":"); colon != -1 {
			paramName = paramName[:colon]
		}

		params = append(params, Parameter{
			Name:     paramName,
			In:       "path",
			Required: true,
			Schema:   &Schema{Type: "string"},
		})
	}

	return params
}

// generateOperationID creates a stable operation ID based on method and route.
func generateOperationID(method, route string) string {
	var cleanParts []string
	for _, part := range strings.Split(strings.Trim(route, "/"), "/") {
		if part == "" || strings.Contains(part, "{") {
			continue
		}
		cleanParts = append(cleanParts, capitalize(part))
	}
	return strings.ToLower(method) + strings.Join(cleanParts, "")
}

// capitalize upper-cases the first rune of s.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// upsertParameter merges a parameter into an existing slice.
func upsertParameter(params []Parameter, p Parameter) []Parameter {
	for i, existing := range params {
		if existing.Name != p.Name || existing.In != p.In {
			continue
		}

		if p.Description != "" {
			existing.Description = p.Description
		}
		if p.Required {
			existing.Required = true
		}
		if p.Schema != nil {
			existing.Schema = p.Schema
		}
		params[i] = existing
		return params
	}

	return append(params, p)
}
