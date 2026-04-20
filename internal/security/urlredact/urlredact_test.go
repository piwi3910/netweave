package urlredact_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/security/urlredact"
	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "no secrets preserved intact",
			input: "https://smo.example.com/notify",
			want:  "https://smo.example.com/notify",
		},
		{
			name:  "query string removed",
			input: "https://smo.example.com/notify?token=supersecret&foo=bar",
			want:  "https://smo.example.com/notify",
		},
		{
			name:  "userinfo stripped",
			input: "https://user:pass@smo.example.com/notify",
			want:  "https://smo.example.com/notify",
		},
		{
			name:  "fragment removed",
			input: "https://smo.example.com/notify#session=abc",
			want:  "https://smo.example.com/notify",
		},
		{
			name:  "userinfo and query both removed",
			input: "https://bob:hunter2@smo.example.com:8443/notify?sig=xyz#frag",
			want:  "https://smo.example.com:8443/notify",
		},
		{
			name:  "invalid URL returns sentinel",
			input: "://bad",
			want:  "[invalid-url]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, urlredact.Redact(tt.input))
		})
	}
}
