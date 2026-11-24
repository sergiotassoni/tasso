#!/bin/sh
# Rifà rsrc_windows_amd64.syso (icona + manifest asInvoker + informazioni di versione)
# con lo strumento dell'ecosistema ../strumenti/risorse (scritto da noi, solo Go, niente internet).
# L'icona risorse/TassoPDF.ico è la copia di icone/pdf/definitiva/tasso-pdf.ico.
set -e
cd "$(dirname "$0")/.."
QUI="$(pwd)"
VERSIONE=$(sed -n 's/^const versione = "\(.*\)"/\1/p' app.go)
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod
cd ../strumenti/risorse
go run . crea -ico "$QUI/risorse/TassoPDF.ico" -manifest "$QUI/risorse/TassoPDF.manifest" \
  -o "$QUI/rsrc_windows_amd64.syso" -versione "$VERSIONE" \
  -prodotto "Tasso PDF" -descrizione "Tasso PDF" -azienda "Sergio Tassoni" -file TassoPDF.exe
