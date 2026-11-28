// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// Aggiornamenti: Tasso Zip non usa internet. Gli aggiornamenti dell'Ecosistema Tasso li gestisce
// Tasso (Impostazioni → Aggiornamenti): "Controlla aggiornamenti" apre Tasso con
// --controlla-aggiornamenti. Se Tasso non c'è, la pagina delle versioni si apre solo dopo che
// l'utente ha detto sì (lo chiede la pagina).

import (
	"os"
	"path/filepath"
	"strings"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

// paginaVersioni: le versioni di Tasso (con l'installatore dell'ecosistema).
const paginaVersioni = "https://github.com/sergiotassoni/tasso/releases/latest"

// trovaTasso: Tasso.exe dell'ecosistema, SOLO dalla cartella scritta dall'installatore
// (HKLM\Software\Tasso\Cartella: solo gli amministratori la cambiano). Niente più "la cartella
// sopra quella di Tasso Zip": con Tasso Zip portatile (per esempio in Download) un Tasso.exe
// qualsiasi scaricato lì sarebbe stato avviato premendo "Controlla aggiornamenti".
func trovaTasso() string {
	var cand []string
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `Software\Tasso`, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
		if v, _, err := k.GetStringValue("Cartella"); err == nil && strings.TrimSpace(v) != "" {
			cand = append(cand, filepath.Join(strings.TrimSpace(v), "Tasso.exe"))
		}
		k.Close()
	}
	for _, c := range cand {
		if !filepath.IsAbs(c) || strings.HasPrefix(c, `\\`) || strings.ContainsAny(c, "\"%&|<>^\r\n") {
			continue
		}
		if fi, err := os.Lstat(c); err == nil && fi.Mode().IsRegular() {
			return filepath.Clean(c)
		}
	}
	return ""
}

func collegaAggiornamenti(w webview2.WebView) {
	// "tasso": Tasso aperto su Aggiornamenti; "nessuno": Tasso non è installato;
	// "errore": Windows non l'ha aperto (per esempio hai detto no alla richiesta di permesso).
	_ = w.Bind("goControllaAggiornamenti", func() string {
		exe := trovaTasso()
		if exe == "" {
			return "nessuno"
		}
		// "open": Tasso chiede l'amministratore (suo manifest), Windows mostra la richiesta
		if shellExecute(exe, "--controlla-aggiornamenti") <= 32 {
			return "errore"
		}
		return "tasso"
	})
	// "errore": il browser non è partito (la pagina lo dice, invece di non fare niente)
	_ = w.Bind("goPaginaVersioni", func() string {
		if shellExecute(paginaVersioni, "") <= 32 {
			return "errore"
		}
		return "ok"
	})
}
