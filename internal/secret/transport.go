package secret

import (
	"net/http"
)

type Transport struct {
	RoundTripper http.RoundTripper
	token        string
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.RoundTripper.RoundTrip(req)
}

func NewTransportWithToken(token string) *Transport {
	return &Transport{
		RoundTripper: http.DefaultTransport,
		token:        token,
	}
}
