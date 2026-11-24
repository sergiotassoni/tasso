#!/bin/sh
# Compila dist/TassoSetup.exe (Windows 64 bit) anche da Linux, senza internet.
# Serve Go 1.24 o più nuovo. Prima vanno compilati Tasso Zip e Tasso PDF (i loro dist/).
#
# TASSO_EXE: il Tasso.exe da mettere dentro. Ora è un segnaposto (/tmp/tb7/Tasso.exe):
# per l'uscita vera usa la build FIRMATA, per esempio
#     TASSO_EXE=/percorso/Tasso-firmato.exe ./compila.sh
set -e
cd "$(dirname "$0")"
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod
TASSO_EXE=${TASSO_EXE:-/tmp/tb7/Tasso.exe}
TASSO_VER=${TASSO_VER:-$(cat ../../tasso/VERSION 2>/dev/null || echo 3.1)}
ECO_VER=${ECO_VER:-$TASSO_VER}
ZIP_VER=$(sed -n 's/^const Versione = "\(.*\)"/\1/p' ../tassozip/main_windows.go)
PDF_VER=$(sed -n 's/^const versione = "\(.*\)"/\1/p' ../tassopdf/app.go)

go vet ./...
GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go vet -tags disinstallatore .
go test -count=1 ./...

# icona (Tasso Gaming, con gli occhi) + manifest requireAdministrator + versione
QUI="$(pwd)"
(cd ../strumenti/risorse && go run . crea -ico "$QUI/../icone/gaming/definitiva/tasso-gaming.ico" \
  -manifest "$QUI/setup.manifest" -o "$QUI/rsrc_windows_amd64.syso" -versione "$ECO_VER" \
  -prodotto "Tasso Setup" -descrizione "Tasso Setup" -azienda "Sergio Tassoni" -file TassoSetup.exe)

# 1. il disinstallatore (stesso codice, senza i programmi dentro)
mkdir -p build dist
GOOS=windows GOARCH=amd64 go build -tags disinstallatore -trimpath -ldflags "-H windowsgui -s -w" -o "build/Disinstalla Tasso.exe" .
# 2. i programmi da incorporare, con le impronte SHA-256
go run ./strumenti/prepara -versione "$ECO_VER" \
  -tasso "$TASSO_EXE" -tasso-versione "$TASSO_VER" -tasso-licenza ../../tasso/LICENSE \
  -zip-dir ../tassozip -zip-versione "$ZIP_VER" -pdf-dir ../tassopdf -pdf-versione "$PDF_VER" \
  -disinstalla "build/Disinstalla Tasso.exe"
# 3. l'installatore
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/TassoSetup.exe .
(cd ../strumenti/risorse && go run . controlla -exe "$QUI/dist/TassoSetup.exe" -ico "$QUI/../icone/gaming/definitiva/tasso-gaming.ico" -livello requireAdministrator)
ls -l dist/TassoSetup.exe
