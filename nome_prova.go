//go:build provaui && !windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// SOLO PER LE PROVE (go run -tags provaui .): "Il tuo nome" in memoria al posto del registro.
// TASSOPDF_NOME = il nome di partenza ("" = senza nome). Non fa parte di TassoPDF.exe.

import "os"

type archivioNomeProva struct {
	v  string
	ok bool
}

func (a *archivioNomeProva) Leggi() (string, bool) { return a.v, a.ok }
func (a *archivioNomeProva) Scrivi(n string) error { a.v, a.ok = n, true; return nil }
func (a *archivioNomeProva) Togli() error          { a.v, a.ok = "", false; return nil }

func init() {
	n, ok := os.LookupEnv("TASSOPDF_NOME")
	archivioNom = &archivioNomeProva{v: n, ok: ok && n != ""}
}
