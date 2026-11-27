#!/bin/sh
# Rifà rsrc_windows_amd64.syso (icona + manifest asInvoker + informazioni di versione)
# con lo strumento dell'ecosistema ../strumenti/risorse (scritto da noi, solo Go, niente internet).
# L'icona assets/tasso.ico è la copia di icone/zip/definitiva/tasso-zip.ico.
set -e
cd "$(dirname "$0")"
QUI="$(pwd)"
VERSIONE=$(sed -n 's/^const Versione = "\(.*\)"/\1/p' main_windows.go)
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod
cd ../strumenti/risorse
go run . crea -ico "$QUI/assets/tasso.ico" -manifest "$QUI/tassozip.manifest" \
  -o "$QUI/rsrc_windows_amd64.syso" -versione "$VERSIONE" \
  -prodotto "Tasso Zip" -descrizione "Tasso Zip" -azienda "Sergio Tassoni" -copyright "© 2026 Sergio Tassoni" -file TassoZip.exe
