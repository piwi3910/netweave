package keycloak

import "errors"

// Sentinel errors returned by the Keycloak client.
// Callers MUST use errors.Is to detect these conditions rather than matching
// on error message strings — the wording is not part of the public contract.
var (
	// ErrUserNotFound indicates the requested user does not exist in Keycloak.
	ErrUserNotFound = errors.New("keycloak: user not found")

	// ErrUserExists indicates a user with the given identifier already exists.
	ErrUserExists = errors.New("keycloak: user already exists")

	// ErrRoleNotFound indicates the requested role does not exist.
	ErrRoleNotFound = errors.New("keycloak: role not found")

	// ErrRoleExists indicates a role with the given name already exists.
	ErrRoleExists = errors.New("keycloak: role already exists")

	// ErrClientNotFound indicates the requested client does not exist.
	ErrClientNotFound = errors.New("keycloak: client not found")

	// ErrClientRoleNotFound indicates the requested client role does not exist.
	ErrClientRoleNotFound = errors.New("keycloak: client role not found")

	// ErrClientRoleExists indicates a client role with the given name already exists.
	ErrClientRoleExists = errors.New("keycloak: client role already exists")

	// ErrAttributeNotFound indicates the requested user attribute does not exist.
	ErrAttributeNotFound = errors.New("keycloak: attribute not found")
)
