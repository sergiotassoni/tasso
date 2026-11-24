module tassopdf

go 1.26.0

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/pdfcpu/pdfcpu v0.16.4
	golang.org/x/sys v0.49.0
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/hhrutter/tiff v1.0.7 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-runewidth v0.0.31 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// Tutte le dipendenze sono copiate in third_party (solo i pacchetti usati, con le loro licenze):
// il programma si compila senza internet. Gli "stub" sono moduli vuoti richiesti solo sulla carta.
replace (
	github.com/jchv/go-webview2 => ./third_party/go-webview2
	github.com/jchv/go-winloader => ./third_party/go-winloader
	github.com/pdfcpu/pdfcpu => ./third_party/pdfcpu
	github.com/hhrutter/tiff => ./third_party/tiff
	github.com/mattn/go-runewidth => ./third_party/go-runewidth
	github.com/xdg-go/stringprep => ./third_party/stringprep
	github.com/clipperhouse/uax29/v2 => ./third_party/uax29
	go.yaml.in/yaml/v3 => ./third_party/go-yaml
	golang.org/x/crypto => ./third_party/x-crypto
	golang.org/x/image => ./third_party/x-image
	golang.org/x/text => ./third_party/x-text
	golang.org/x/sys => ./third_party/x-sys
	github.com/google/jsonschema-go => ./third_party/stub/jsonschema-go
	github.com/spf13/cobra => ./third_party/stub/cobra
	github.com/spf13/pflag => ./third_party/stub/pflag
	github.com/inconshreveable/mousetrap => ./third_party/stub/mousetrap
	golang.org/x/tools => ./third_party/stub/tools
	golang.org/x/mod => ./third_party/stub/mod
	golang.org/x/sync => ./third_party/stub/sync
	golang.org/x/net => ./third_party/stub/net
	golang.org/x/term => ./third_party/stub/term
)
