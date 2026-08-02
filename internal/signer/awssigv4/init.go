package awssigv4

import "github.com/weiqigod/curlew/internal/signer"

func init() {
	signer.MustRegister("aws-sigv4", Factory)
}
