module tasso

go 1.24.7

// Compilare sempre con questo Go (correzioni di sicurezza della libreria standard).
toolchain go1.26.10

replace golang.org/x/sys => github.com/golang/sys v0.30.0

// Copia locale con il caricamento di WebView2Loader.dll solo da System32 (vedi third_party/go-webview2/README.md).
replace github.com/jchv/go-webview2 => ./third_party/go-webview2

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	golang.org/x/sys v0.0.0-20210218145245-beda7e5e158e
)

require github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
