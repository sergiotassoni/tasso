#!/bin/sh
# Compila Tasso Zip per Windows 64 bit (anche da Linux), senza internet.
# Le librerie sono in third_party/ (vedi go.mod). Serve Go 1.25 o più nuovo.
set -e
cd "$(dirname "$0")"
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod
go vet ./...
GOOS=windows GOARCH=amd64 go vet ./...
go test ./...
mkdir -p dist
# rsrc_windows_amd64.syso contiene icona e manifest (asInvoker: niente amministratore).
# Per rifarlo: rsrc -manifest tassozip.manifest -ico assets/tasso.ico -arch amd64 -o rsrc_windows_amd64.syso
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/TassoZip.exe .
ls -l dist/TassoZip.exe
