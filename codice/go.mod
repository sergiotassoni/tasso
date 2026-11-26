module tasso

go 1.24.7

// Compilare sempre con questo Go (correzioni di sicurezza della libreria standard).
toolchain go1.26.10

// Copie locali (con le loro licenze) per compilare senza internet e senza percorsi del PC dentro
// l'exe: x/sys v0.30.0, go-winloader 2025-04-06, wazero v1.8.2. Vedi third_party/LEGGIMI.txt.
replace golang.org/x/sys => ./third_party/x-sys

replace github.com/jchv/go-winloader => ./third_party/go-winloader

replace github.com/tetratelabs/wazero => ./third_party/wazero

// Copia locale con il caricamento di WebView2Loader.dll solo da System32 (vedi third_party/go-webview2/README.md).
replace github.com/jchv/go-webview2 => ./third_party/go-webview2

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	golang.org/x/sys v0.30.0
)

require (
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/tetratelabs/wazero v1.8.2
)
