package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOidcUserEmailVerified(t *testing.T) {
	tests := []struct {
		name     string
		jsonBody string
		want     bool
	}{
		{
			name:     "boolean true",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":true}`,
			want:     true,
		},
		{
			name:     "string true",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":"true"}`,
			want:     true,
		},
		{
			name:     "uppercase string true",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":"TRUE"}`,
			want:     true,
		},
		{
			name:     "trimmed string true",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":" True "}`,
			want:     true,
		},
		{
			name:     "boolean false",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":false}`,
			want:     false,
		},
		{
			name:     "string false",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":"false"}`,
			want:     false,
		},
		{
			name:     "null",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com","email_verified":null}`,
			want:     false,
		},
		{
			name:     "missing",
			jsonBody: `{"sub":"oidc-sub","name":"OIDC User","email":"user@example.com"}`,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var user OidcUser
			if err := json.Unmarshal([]byte(tt.jsonBody), &user); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if user.VerifiedEmail != tt.want {
				t.Fatalf("VerifiedEmail = %v, want %v", user.VerifiedEmail, tt.want)
			}

			oauthUser := user.ToOauthUser()
			if oauthUser.VerifiedEmail != tt.want {
				t.Fatalf("ToOauthUser().VerifiedEmail = %v, want %v", oauthUser.VerifiedEmail, tt.want)
			}
		})
	}
}

func TestOauthClientSecretIsNotSerialized(t *testing.T) {
	oauth := Oauth{Op: "oidc", ClientId: "client", ClientSecret: "super-secret"}
	body, err := json.Marshal(oauth)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(body) == "" || !json.Valid(body) {
		t.Fatalf("invalid json body %q", string(body))
	}
	if strings.Contains(string(body), "client_secret") || strings.Contains(string(body), "super-secret") {
		t.Fatalf("serialized oauth leaked client secret: %s", body)
	}
}
