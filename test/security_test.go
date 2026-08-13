package openapi_test

import (
	"encoding/json"
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
	"github.com/kbertalan/chi-openapi/security"
)

func TestSecuritySchemeConstructors(t *testing.T) {
	cases := []struct {
		name   string
		scheme openapi.SecurityScheme
		want   string
	}{
		{
			name:   "apiKey",
			scheme: security.APIKey("X-API-Key", security.InHeader, security.WithDescription("key")),
			want:   `{"type":"apiKey","description":"key","name":"X-API-Key","in":"header"}`,
		},
		{
			name:   "bearer",
			scheme: security.Bearer("JWT"),
			want:   `{"type":"http","scheme":"bearer","bearerFormat":"JWT"}`,
		},
		{
			name:   "basic",
			scheme: security.Basic(),
			want:   `{"type":"http","scheme":"basic"}`,
		},
		{
			name:   "mutualTLS",
			scheme: security.MutualTLS(),
			want:   `{"type":"mutualTLS"}`,
		},
		{
			name:   "openIdConnect",
			scheme: security.OpenIDConnect("https://issuer.example/.well-known/openid-configuration"),
			want:   `{"type":"openIdConnect","openIdConnectUrl":"https://issuer.example/.well-known/openid-configuration"}`,
		},
		{
			name: "oauth2 authorizationCode",
			scheme: security.OAuth2(openapi.OAuthFlows{
				AuthorizationCode: security.AuthorizationCodeFlow(
					"https://a.example/authorize", "https://a.example/token",
					map[string]string{"read": "Read access"}),
			}),
			want: `{"type":"oauth2","flows":{"authorizationCode":{"authorizationUrl":"https://a.example/authorize","tokenUrl":"https://a.example/token","scopes":{"read":"Read access"}}}}`,
		},
		{
			name: "oauth2 clientCredentials empty scopes are serialized",
			scheme: security.OAuth2(openapi.OAuthFlows{
				ClientCredentials: security.ClientCredentialsFlow("https://a.example/token", nil),
			}),
			want: `{"type":"oauth2","flows":{"clientCredentials":{"tokenUrl":"https://a.example/token","scopes":{}}}}`,
		},
		{
			name: "oauth2 clientCredentials with extensions",
			scheme: security.OAuth2(openapi.OAuthFlows{
				ClientCredentials: security.ClientCredentialsFlow("https://login.example.com/oauth/token", nil,
					security.WithFlowExtension("x-credentials-location", "body"),
					security.WithFlowExtension("x-security-body", map[string]string{
						"audience": "https://api.example.com/",
					})),
			}),
			want: `{"type":"oauth2","flows":{"clientCredentials":{"tokenUrl":"https://login.example.com/oauth/token","scopes":{},"x-credentials-location":"body","x-security-body":{"audience":"https://api.example.com/"}}}}`,
		},
		{
			name: "oauth2 clientCredentials without extensions is unchanged",
			scheme: security.OAuth2(openapi.OAuthFlows{
				ClientCredentials: security.ClientCredentialsFlow("https://a.example/token",
					map[string]string{"read": "Read access"}),
			}),
			want: `{"type":"oauth2","flows":{"clientCredentials":{"tokenUrl":"https://a.example/token","scopes":{"read":"Read access"}}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.scheme)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestOAuthFlowExtensionNameMustBePrefixed(t *testing.T) {
	scheme := security.OAuth2(openapi.OAuthFlows{
		ClientCredentials: security.ClientCredentialsFlow("https://a.example/token", nil,
			security.WithFlowExtension("credentials-location", "body")),
	})

	if _, err := json.Marshal(scheme); err == nil {
		t.Fatal("expected an error for an extension name without the \"x-\" prefix")
	}
}
