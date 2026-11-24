#!/bin/sh
# Tasso PDF - compila dist/TassoPDF.exe per Windows a 64 bit (senza internet: dipendenze in third_party).
# Uso: strumenti/compila.sh   (serve Go 1.26 nel PATH)
set -e
cd "$(dirname "$0")/.."
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod
go test -count=1 .
go vet .
GOOS=windows GOARCH=amd64 go vet .
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/TassoPDF.exe .
cp LEGGIMI.txt LICENSE LICENZE.txt dist/ 2>/dev/null || true
ls -la dist/TassoPDF.exe
