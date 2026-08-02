package oauth1

import "github.com/weiqigod/curlew/internal/signer"

func init() {
	signer.MustRegister("oauth1", Factory)
}
