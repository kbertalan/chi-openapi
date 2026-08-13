// Package security builds OpenAPI security schemes with the required fields set
// per scheme type. Register the results in openapi.Config.SecuritySchemes:
//
//	SecuritySchemes: map[string]openapi.SecurityScheme{
//		"ApiKeyAuth": security.APIKey("X-API-Key", security.InHeader),
//		"BearerAuth": security.Bearer("JWT"),
//	}
package security

import openapi "github.com/kbertalan/chi-openapi"

// apiKey locations.
const (
	InQuery  = "query"
	InHeader = "header"
	InCookie = "cookie"
)

// Option customizes a SecurityScheme.
type Option func(*openapi.SecurityScheme)

// WithDescription sets the scheme description.
func WithDescription(description string) Option {
	return func(s *openapi.SecurityScheme) { s.Description = description }
}

func build(s openapi.SecurityScheme, opts ...Option) openapi.SecurityScheme {
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// APIKey builds an apiKey scheme. in is InQuery, InHeader or InCookie.
func APIKey(name, in string, opts ...Option) openapi.SecurityScheme {
	return build(openapi.SecurityScheme{Type: "apiKey", Name: name, In: in}, opts...)
}

// HTTP builds an http scheme with the given RFC 7235 auth scheme.
func HTTP(scheme string, opts ...Option) openapi.SecurityScheme {
	return build(openapi.SecurityScheme{Type: "http", Scheme: scheme}, opts...)
}

// Basic builds an http basic scheme.
func Basic(opts ...Option) openapi.SecurityScheme {
	return HTTP("basic", opts...)
}

// Bearer builds an http bearer scheme. bearerFormat (e.g. "JWT") may be empty.
func Bearer(bearerFormat string, opts ...Option) openapi.SecurityScheme {
	return build(openapi.SecurityScheme{Type: "http", Scheme: "bearer", BearerFormat: bearerFormat}, opts...)
}

// MutualTLS builds a mutualTLS scheme.
func MutualTLS(opts ...Option) openapi.SecurityScheme {
	return build(openapi.SecurityScheme{Type: "mutualTLS"}, opts...)
}

// OpenIDConnect builds an openIdConnect scheme from its discovery URL.
func OpenIDConnect(url string, opts ...Option) openapi.SecurityScheme {
	return build(openapi.SecurityScheme{Type: "openIdConnect", OpenIdConnectURL: url}, opts...)
}

// OAuth2 builds an oauth2 scheme from the given flows.
func OAuth2(flows openapi.OAuthFlows, opts ...Option) openapi.SecurityScheme {
	f := flows
	return build(openapi.SecurityScheme{Type: "oauth2", Flows: &f}, opts...)
}

// FlowOption customizes an OAuthFlow.
type FlowOption func(*openapi.OAuthFlow)

// WithFlowExtension sets a specification extension on the flow. name must be
// prefixed with "x-"; otherwise marshalling the spec fails.
func WithFlowExtension(name string, value any) FlowOption {
	return func(f *openapi.OAuthFlow) {
		if f.Extensions == nil {
			f.Extensions = make(map[string]any)
		}
		f.Extensions[name] = value
	}
}

func buildFlow(f openapi.OAuthFlow, opts ...FlowOption) *openapi.OAuthFlow {
	for _, opt := range opts {
		opt(&f)
	}
	return &f
}

func scopesOrEmpty(scopes map[string]string) map[string]string {
	if scopes == nil {
		return map[string]string{}
	}
	return scopes
}

// ImplicitFlow builds an OAuth2 implicit flow.
func ImplicitFlow(authorizationURL string, scopes map[string]string, opts ...FlowOption) *openapi.OAuthFlow {
	return buildFlow(openapi.OAuthFlow{AuthorizationURL: authorizationURL, Scopes: scopesOrEmpty(scopes)}, opts...)
}

// PasswordFlow builds an OAuth2 password flow.
func PasswordFlow(tokenURL string, scopes map[string]string, opts ...FlowOption) *openapi.OAuthFlow {
	return buildFlow(openapi.OAuthFlow{TokenURL: tokenURL, Scopes: scopesOrEmpty(scopes)}, opts...)
}

// ClientCredentialsFlow builds an OAuth2 client-credentials flow.
func ClientCredentialsFlow(tokenURL string, scopes map[string]string, opts ...FlowOption) *openapi.OAuthFlow {
	return buildFlow(openapi.OAuthFlow{TokenURL: tokenURL, Scopes: scopesOrEmpty(scopes)}, opts...)
}

// AuthorizationCodeFlow builds an OAuth2 authorization-code flow.
func AuthorizationCodeFlow(authorizationURL, tokenURL string, scopes map[string]string, opts ...FlowOption) *openapi.OAuthFlow {
	return buildFlow(openapi.OAuthFlow{AuthorizationURL: authorizationURL, TokenURL: tokenURL, Scopes: scopesOrEmpty(scopes)}, opts...)
}
