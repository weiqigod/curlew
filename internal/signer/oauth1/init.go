package oauth1

import "github.com/peterlindqvist/apitest/internal/signer"

func init() {
	signer.MustRegister("oauth1", Factory)
}
