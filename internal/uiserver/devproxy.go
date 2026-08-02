package uiserver

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// newDevProxy reverse-proxies non-API paths to the Vite dev server
// (CURLEW_UI_DEV_PROXY, spec §3.4). Chosen over a Vite-side proxy so the
// token flow, Host checks, and WS path are identical to production.
func newDevProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "dev proxy: "+err.Error()+" — is the Vite dev server running?", http.StatusBadGateway)
	}
	return proxy
}
