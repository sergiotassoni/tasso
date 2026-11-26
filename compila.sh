#!/bin/sh
# Compila dist/TassoSetup.exe (Windows 64 bit) anche da Linux, senza internet, con lo STESSO Go
# di Tasso, Tasso Zip e Tasso PDF (1.26, in ../toolchain/go1.26). Prima vanno compilati Tasso Zip
# e Tasso PDF (i loro dist/).
#
# Prova (niente firme):
#     TASSO_EXE=/percorso/Tasso.exe ./compila.sh
# Uscita vera (vedi /home/claude/collaudo-finale/firma-ordine.md):
#     ./compila.sh disinstallatore                 -> build/Disinstalla Tasso.exe, da firmare
#     FIRMA=si TASSO_EXE=Tasso-firmato.exe DISINSTALLA_EXE="build/Disinstalla Tasso.exe" ./compila.sh
#     (firmare dist/TassoSetup.exe, poi: go run ./strumenti/impronta)
# Con FIRMA=si si ferma se Tasso.exe, TassoZip.exe, TassoPDF.exe o il disinstallatore non sono firmati.
set -e
cd "$(dirname "$0")"
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod
GO=${GO:-../toolchain/go1.26/bin/go}
if [ ! -x "$GO" ]; then
  echo "Manca Go 1.26 in $GO (lo stesso di Tasso, Zip e PDF): impostalo con GO=/percorso/bin/go" >&2
  exit 1
fi
GO="$(cd "$(dirname "$GO")" && pwd)/$(basename "$GO")"   # assoluto: sotto si cambia cartella
case "$($GO version)" in
  *go1.26*) ;;
  *) echo "Serve Go 1.26 (come Tasso, Zip e PDF), non: $($GO version)" >&2; exit 1 ;;
esac
export PATH="$(dirname "$GO"):$PATH"   # anche "go run" dentro strumenti usa lo stesso Go

LDF="-H windowsgui -s -w"
mkdir -p build dist
TASSO_VER=${TASSO_VER:-$(cat ../../tasso/VERSION 2>/dev/null || echo 3.1)}
ECO_VER=${ECO_VER:-$TASSO_VER}

# icona (Tasso Gaming, con gli occhi) + manifest requireAdministrator + versione. Lo stesso codice
# fa due exe: ognuno con il suo nome nelle Proprietà (OriginalFilename), quindi la risorsa si rifà
# prima di ogni compilazione. risorse <descrizione> <nome-file>
QUI="$(pwd)"
risorse() {
  (cd ../strumenti/risorse && $GO run . crea -ico "$QUI/../icone/gaming/definitiva/tasso-gaming.ico" \
    -manifest "$QUI/setup.manifest" -o "$QUI/rsrc_windows_amd64.syso" -versione "$ECO_VER" \
    -prodotto "Tasso Setup" -descrizione "$1" -azienda "Sergio Tassoni" -file "$2")
}

# solo il disinstallatore (da firmare prima di metterlo dentro l'installatore)
if [ "$1" = "disinstallatore" ]; then
  GOOS=windows GOARCH=amd64 $GO vet -tags disinstallatore .
  risorse "Disinstalla Tasso" "Disinstalla Tasso.exe"
  GOOS=windows GOARCH=amd64 $GO build -tags disinstallatore -trimpath -ldflags "$LDF" -o "build/Disinstalla Tasso.exe" .
  risorse "Tasso Setup" TassoSetup.exe   # rimetto quella dell'installatore
  ls -l "build/Disinstalla Tasso.exe"
  echo "Adesso firma build/Disinstalla Tasso.exe, poi rilancia con DISINSTALLA_EXE=\"build/Disinstalla Tasso.exe\"."
  exit 0
fi

if [ -z "$TASSO_EXE" ] || [ ! -f "$TASSO_EXE" ]; then
  echo "TASSO_EXE: indica il Tasso.exe da mettere dentro (per l'uscita vera quello FIRMATO)" >&2
  exit 1
fi
ZIP_VER=$(sed -n 's/^const Versione = "\(.*\)"/\1/p' ../tassozip/main_windows.go)
PDF_VER=$(sed -n 's/^const versione = "\(.*\)"/\1/p' ../tassopdf/app.go)

$GO vet ./...
GOOS=windows GOARCH=amd64 $GO vet ./...
GOOS=windows GOARCH=amd64 $GO vet -tags disinstallatore .
$GO test -count=1 ./...

# 1. il disinstallatore (stesso codice, senza i programmi dentro): già firmato, o lo compilo adesso
if [ -n "$DISINSTALLA_EXE" ]; then
  DISINST="$DISINSTALLA_EXE"
else
  risorse "Disinstalla Tasso" "Disinstalla Tasso.exe"
  GOOS=windows GOARCH=amd64 $GO build -tags disinstallatore -trimpath -ldflags "$LDF" -o "build/Disinstalla Tasso.exe" .
  DISINST="build/Disinstalla Tasso.exe"
fi
if [ "$FIRMA" = "si" ]; then
  $GO run ./strumenti/firmato "$TASSO_EXE" "$DISINST" ../tassozip/dist/TassoZip.exe ../tassopdf/dist/TassoPDF.exe
else
  echo "ATTENZIONE: installatore di PROVA (FIRMA=si non impostato): non pubblicarlo." >&2
fi
# 2. i programmi da incorporare, con le impronte SHA-256 (dei file già firmati)
$GO run ./strumenti/prepara -versione "$ECO_VER" \
  -tasso "$TASSO_EXE" -tasso-versione "$TASSO_VER" -tasso-licenza ../../tasso/LICENSE -tasso-avvisi ../../tasso/codice/dist \
  -zip-dir ../tassozip -zip-versione "$ZIP_VER" -pdf-dir ../tassopdf -pdf-versione "$PDF_VER" \
  -disinstalla "$DISINST" $( [ "$FIRMA" = "si" ] && echo -rilascio )
# 3. l'installatore (con la sua risorsa: nome TassoSetup.exe)
risorse "Tasso Setup" TassoSetup.exe
GOOS=windows GOARCH=amd64 $GO build -trimpath -ldflags "$LDF" -o dist/TassoSetup.exe .
(cd ../strumenti/risorse && $GO run . controlla -exe "$QUI/dist/TassoSetup.exe" -ico "$QUI/../icone/gaming/definitiva/tasso-gaming.ico" -livello requireAdministrator)
# 4. TassoSetup.sha256 per l'aggiornamento da Tasso (DOPO la firma di TassoSetup.exe va rifatto:
#    go run ./strumenti/impronta)
$GO run ./strumenti/impronta
$GO version -m dist/TassoSetup.exe | head -1
ls -l dist/TassoSetup.exe dist/TassoSetup.sha256
