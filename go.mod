module tassozip

go 1.25.0

require (
	github.com/bodgit/sevenzip v1.6.5
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/klauspost/compress v1.19.0
	github.com/nwaples/rardecode/v2 v2.4.6
	github.com/ulikunitz/xz v0.5.15
	golang.org/x/sys v0.30.0
	golang.org/x/text v0.40.0
)

require (
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/stangelandcl/ppmd v0.1.1 // indirect
	go4.org v0.0.0-20260112195520-a5071408f32f // indirect
	golang.org/x/sync v0.22.0 // indirect
)

// Tutte le librerie sono copie locali in third_party/ (stesse versioni dei tag ufficiali
// su GitHub), così il programma si compila anche senza internet. Vedi third_party/LEGGIMI.txt.
replace (
	github.com/andybalholm/brotli => ./third_party/brotli
	github.com/bodgit/plumbing => ./third_party/plumbing
	github.com/bodgit/sevenzip => ./third_party/sevenzip
	github.com/bodgit/windows => ./third_party/bodgit-windows
	github.com/hashicorp/golang-lru/v2 => ./third_party/golang-lru
	github.com/jchv/go-webview2 => ./third_party/go-webview2
	github.com/jchv/go-winloader => ./third_party/go-winloader
	github.com/klauspost/compress => ./third_party/compress
	github.com/nwaples/rardecode/v2 => ./third_party/rardecode
	github.com/pierrec/lz4/v4 => ./third_party/lz4
	github.com/spf13/afero => ./third_party/afero
	github.com/stangelandcl/ppmd => ./third_party/ppmd
	github.com/ulikunitz/xz => ./third_party/xz
	go4.org => ./third_party/go4
	golang.org/x/sync => ./third_party/x-sync
	golang.org/x/sys => ./third_party/x-sys
	golang.org/x/text => ./third_party/x-text
)
