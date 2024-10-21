package secret

import (
	"errors"

	"github.com/zalando/go-keyring"
)

type Keyring struct{}

var _ TokenStore = (*Keyring)(nil)

func (*Keyring) Set(token string) error {
	return keyring.Set(service, key, token)
}

func (*Keyring) Get() (string, error) {
	return keyring.Get(service, key)
}

func (*Keyring) Delete() error {
	err := keyring.Delete(service, key)
	if err != nil && errors.Is(err, keyring.ErrNotFound) {
		return nil
	}

	return err
}
