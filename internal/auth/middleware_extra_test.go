package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssignDNValueToField(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		value      string
		checkField func(*CertificateInfo) bool
	}{
		{
			name:  "CN sets CommonName",
			key:   "CN",
			value: "alice",
			checkField: func(cert *CertificateInfo) bool {
				return cert.Subject.CommonName == "alice" && cert.CommonName == "alice"
			},
		},
		{
			name:  "O sets Organization",
			key:   "O",
			value: "ACME Corp",
			checkField: func(cert *CertificateInfo) bool {
				return len(cert.Subject.Organization) == 1 && cert.Subject.Organization[0] == "ACME Corp"
			},
		},
		{
			name:  "OU sets OrganizationalUnit",
			key:   "OU",
			value: "Engineering",
			checkField: func(cert *CertificateInfo) bool {
				return len(cert.Subject.OrganizationalUnit) == 1 && cert.Subject.OrganizationalUnit[0] == "Engineering"
			},
		},
		{
			name:  "C sets Country",
			key:   "C",
			value: "US",
			checkField: func(cert *CertificateInfo) bool {
				return len(cert.Subject.Country) == 1 && cert.Subject.Country[0] == "US"
			},
		},
		{
			name:  "ST sets Province",
			key:   "ST",
			value: "California",
			checkField: func(cert *CertificateInfo) bool {
				return len(cert.Subject.Province) == 1 && cert.Subject.Province[0] == "California"
			},
		},
		{
			name:  "L sets Locality",
			key:   "L",
			value: "San Francisco",
			checkField: func(cert *CertificateInfo) bool {
				return len(cert.Subject.Locality) == 1 && cert.Subject.Locality[0] == "San Francisco"
			},
		},
		{
			name:  "EMAILADDRESS sets Email",
			key:   "EMAILADDRESS",
			value: "alice@example.com",
			checkField: func(cert *CertificateInfo) bool {
				return cert.Email == "alice@example.com"
			},
		},
		{
			name:  "EMAIL sets Email",
			key:   "EMAIL",
			value: "bob@example.com",
			checkField: func(cert *CertificateInfo) bool {
				return cert.Email == "bob@example.com"
			},
		},
		{
			name:  "lowercase cn is case-insensitive",
			key:   "cn",
			value: "charlie",
			checkField: func(cert *CertificateInfo) bool {
				return cert.Subject.CommonName == "charlie"
			},
		},
		{
			name:  "unknown key does nothing",
			key:   "UNKNOWN",
			value: "ignored",
			checkField: func(cert *CertificateInfo) bool {
				return cert.Subject.CommonName == "" && cert.Email == ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &CertificateInfo{
				Subject: CertificateSubject{},
			}
			assignDNValueToField(tt.key, tt.value, cert)
			assert.True(t, tt.checkField(cert), "field check failed for key %s", tt.key)
		})
	}
}

func TestAuthError_Error(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want string
	}{
		{
			name: "user_lookup kind",
			kind: "user_lookup",
			want: "user_lookup",
		},
		{
			name: "user_inactive kind",
			kind: "user_inactive",
			want: "user_inactive",
		},
		{
			name: "role_lookup kind",
			kind: "role_lookup",
			want: "role_lookup",
		},
		{
			name: "tenant_lookup kind",
			kind: "tenant_lookup",
			want: "tenant_lookup",
		},
		{
			name: "tenant_inactive kind",
			kind: "tenant_inactive",
			want: "tenant_inactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &authError{kind: tt.kind}
			assert.Equal(t, tt.want, err.Error())
		})
	}
}

func TestPatternToRegex(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		match   bool
	}{
		{
			name:    "trailing wildcard matches deep path",
			pattern: "/api/public/*",
			path:    "/api/public/health/check",
			match:   true,
		},
		{
			name:    "middle wildcard matches single segment",
			pattern: "/api/*/status",
			path:    "/api/v1/status",
			match:   true,
		},
		{
			name:    "middle wildcard does not match multiple segments",
			pattern: "/api/*/status",
			path:    "/api/v1/v2/status",
			match:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.match, MatchesPathPattern(tt.path, tt.pattern))
		})
	}
}

func TestPermissionsToStrings(t *testing.T) {
	perms := []Permission{PermissionSubscriptionRead, PermissionUserCreate}
	strs := permissionsToStrings(perms)
	assert.Equal(t, []string{"subscriptions:read", "users:create"}, strs)
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercase",
			input: "ALICE@EXAMPLE.COM",
			want:  "alice@example.com",
		},
		{
			name:  "trim whitespace",
			input: "  alice@example.com  ",
			want:  "alice@example.com",
		},
		{
			name:  "already normalized",
			input: "alice@example.com",
			want:  "alice@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEmail(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeSubjectKey(t *testing.T) {
	// Same input should produce same output.
	key1 := sanitizeSubjectKey("CN=alice,O=ACME")
	key2 := sanitizeSubjectKey("CN=alice,O=ACME")
	assert.Equal(t, key1, key2)

	// Different input should produce different output.
	key3 := sanitizeSubjectKey("CN=bob,O=ACME")
	assert.NotEqual(t, key1, key3)

	// Hash should be hex-encoded (64 chars for SHA-256).
	assert.Len(t, key1, 64)
}
