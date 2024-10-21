package secret

const (
	service = "tempest_cli"
	key     = "api_token"
)

type TokenStore interface {
	// Set a secret in the store.
	Set(secret string) error
	// Get a secret from the store.
	Get() (string, error)
	// Delete a secret from the store.
	Delete() error
}
