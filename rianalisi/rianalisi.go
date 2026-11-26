// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

// Package rianalisi distingue i collegamenti veri (symlink, giunzioni) dagli altri "punti di
// rianalisi" di Windows, che sono file e cartelle normali: OneDrive "File su richiesta"
// (anche quelli solo online), file compressi da Windows (WOF), deduplicati. Da Go 1.23
// os.Lstat li segna tutti come "irregolari": Tasso Zip li saltava come "collegamento" e
// comprimendo Documenti o Desktop sincronizzati con OneDrive uscivano archivi vuoti.
package rianalisi

import "io/fs"

// Collegamento: il percorso p (con informazioni fi da Lstat) è un collegamento da non seguire
// né mettere nell'archivio. false per i file e le cartelle normali, anche se Windows li tiene
// come punti di rianalisi (OneDrive, WOF, deduplicazione).
func Collegamento(p string, fi fs.FileInfo) bool {
	if fi == nil {
		return true
	}
	m := fi.Mode()
	if m&fs.ModeSymlink != 0 {
		return true
	}
	if m&fs.ModeIrregular == 0 {
		return false
	}
	return collegamentoSistema(p)
}

// collegamentoEtichetta: dall'etichetta del punto di rianalisi. Le etichette "surrogato di
// nome" (bit 29: symlink, giunzione, montaggio) portano altrove; AF_UNIX è un socket.
func collegamentoEtichetta(tag uint32) bool {
	const surrogatoNome = 0x20000000
	const afUnix = 0x80000023
	return tag&surrogatoNome != 0 || tag == afUnix
}

// PortaAltrove: il punto di rianalisi p è un collegamento (symlink, giunzione), secondo la sua
// etichetta. Nel dubbio sì.
func PortaAltrove(p string) bool { return collegamentoSistema(p) }
