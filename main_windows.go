//go:build windows

// Tasso PDF - apri, unisci e firma i tuoi PDF.
// Copyright (C) 2026 Sergio Tassoni
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU General Public License as published by the Free Software Foundation, either version 3
// of the License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU General Public License for more details (file LICENSE).

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func init() {
	// La finestra e WebView2 vogliono restare sempre nello stesso filo del sistema.
	runtime.LockOSThread()
}

// rendiSicuro: le DLL si cercano solo in System32 (mai nella cartella dell'exe, per esempio
// Download, dove qualcuno potrebbe mettere una DLL falsa) e le variabili WEBVIEW2_* non
// possono cambiare il comportamento della finestra.
func rendiSicuro() {
	_ = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && strings.HasPrefix(strings.ToUpper(kv[:i]), "WEBVIEW2_") {
			_ = os.Unsetenv(kv[:i])
		}
	}
}

func scriviErrore(dove string, r interface{}, stack []byte) {
	d := cartellaDatiWin()
	if d == "" {
		return
	}
	_ = os.MkdirAll(d, 0o700)
	p := filepath.Join(d, "errori.txt")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 256<<10 {
		_ = os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "--- %s, versione %s, in %s: %v\r\n%s\r\n", time.Now().Format("02/01/2006 15:04:05"), versione, dove, r, stack)
}

func nuovaFinestra() webview2.WebView {
	dati := cartellaDatiWin()
	if dati == "" {
		dati = filepath.Join(os.TempDir(), "Tasso PDF")
	}
	return webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:            filepath.Join(dati, "WebView2"),
		AutoFocus:           true,
		Risorse:             risorsa,
		NavigazionePermessa: navigazionePermessa,
		WindowOptions: webview2.WindowOptions{
			Title:  "Tasso PDF",
			Width:  1240,
			Height: 860,
			IconId: 2,
			Center: true,
		},
	})
}

func main() {
	rendiSicuro()
	defer func() {
		if r := recover(); r != nil {
			scriviErrore("main", r, debug.Stack())
		}
	}()
	webview2.PanicHook = func(m string, r interface{}, st []byte) { scriviErrore(m, r, st) }

	// File dalla riga di comando (doppio clic su un PDF, menu del tasto destro). Windows avvia
	// una copia per ogni file scelto: la prima raccoglie quelli delle altre, che si chiudono.
	modo, percorsi := leggiArgomenti(os.Args[1:])
	if modo != "" {
		primo, chiudi := sonoIlPrimo(modo)
		if !primo {
			passaAlPrimo(modo, percorsi)
			return
		}
		defer chiudi()
		percorsi = raccogliCoda(modo, percorsi, 450*time.Millisecond)
		ordinaPerNome(percorsi) // Windows avvia una copia per file, in ordine casuale
		avvio.modo, avvio.percorsi = modo, percorsi
		go ascoltaCoda(modo)
	}
	// Se il menu del tasto destro è attivo ma l'exe è stato spostato, aggiorno i percorsi.
	if menuAttivoWin() {
		if exe, err := os.Executable(); err == nil && !menuPuntaA(exe) {
			_ = attivaMenu(linguaIniziale())
		}
	}

	w := nuovaFinestra()
	if w == nil {
		testo := "Tasso PDF needs Microsoft Edge WebView2 (a free Windows component).\nPress OK to open the official Microsoft page, then start Tasso PDF again."
		if linguaIniziale() == "it" {
			testo = "Per funzionare Tasso PDF ha bisogno di Microsoft Edge WebView2, un componente gratuito di Windows.\nPremi OK: si apre la pagina ufficiale Microsoft per installarlo, poi riapri Tasso PDF."
		}
		messaggio(testo, "Tasso PDF")
		_ = apriLinkWin("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
		return
	}
	defer w.Destroy()
	hwndPrincipale = uintptr(w.Window())
	w.SetSize(900, 620, webview2.HintMin)

	// La X e Alt+F4 con documenti aperti: prima chiedo alla pagina (F16-1, vedi chiusura.go).
	chiudiFinestraVera = w.Destroy
	chiediChiusuraPagina = func() {
		w.Dispatch(func() { w.Eval("window.tassoChiudiFinestra && window.tassoChiudiFinestra()") })
	}
	webview2.ChiusuraHook = suChiusuraX

	for _, n := range legamiSuFinestra {
		webview2.SuFinestra[n] = true
	}
	for _, n := range legamiSubito {
		webview2.Subito[n] = true
	}
	for n, f := range legami() {
		if err := w.Bind(n, f); err != nil {
			scriviErrore("bind "+n, err, nil)
		}
	}
	w.Navigate(indirizzoBase + "index.html")
	w.Run()
}
