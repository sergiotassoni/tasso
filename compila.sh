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
# Per rifarlo (anche versione e nome prodotto): ./risorse.sh  (usa ../strumenti/risorse)
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/TassoZip.exe .
ls -l dist/TassoZip.exe
# controllo: l'exe contiene la nuova icona (byte per byte) e il manifest asInvoker
if [ -d ../strumenti/risorse ]; then
  (cd ../strumenti/risorse && go run . controlla -exe "$OLDPWD/dist/TassoZip.exe" -ico "$OLDPWD/assets/tasso.ico" -livello asInvoker)
fi
