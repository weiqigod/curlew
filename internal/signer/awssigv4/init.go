package awssigv4

import "github.com/peterlindqvist/apitest/internal/signer"

func init() {
	signer.MustRegister("aws-sigv4", Factory)
}
