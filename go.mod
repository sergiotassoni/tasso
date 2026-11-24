module tassosetup

go 1.24

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	golang.org/x/sys v0.30.0
)

require github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect

// Librerie copiate in third_party (con le loro licenze): si compila senza internet.
// go-webview2 è la copia modificata di Tasso (vedi third_party/go-webview2/README.md).
replace (
	github.com/jchv/go-webview2 => ./third_party/go-webview2
	github.com/jchv/go-winloader => ./third_party/go-winloader
	golang.org/x/sys => ./third_party/x-sys
)
