// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"

	"tassozip/archivio"
	"tassozip/testi"
)

// impostazioni salvate in %LOCALAPPDATA%\Tasso Zip\impostazioni.json.
type impostazioni struct {
	Lingua  string `json:"lingua,omitempty"`  // scelta dall'utente in alto nella finestra ("" = automatica)
	Formato string `json:"formato,omitempty"` // "7z", "zip", "tarzst" (o il vecchio "tarxz")
	Livello string `json:"livello,omitempty"` // 7z: "veloce", "normale", "massima", "ultra"
	// Esplora: l'ultima cartella aperta nella vista "Esplora i file"; Nascosti: mostra i file nascosti.
	Esplora  string `json:"esplora,omitempty"`
	Nascosti bool   `json:"nascosti,omitempty"`
}

var (
	impMu      sync.Mutex
	imp        impostazioni
	linguaRiga string // --lingua=xx dalla riga di comando (vale solo per questa volta)
)

func fileImpostazioni() string { return filepath.Join(cartella, "impostazioni.json") }

func caricaImpostazioni() {
	impMu.Lock()
	defer impMu.Unlock()
	imp, _ = leggiImpostazioni()
}

// leggiImpostazioni: il file come è adesso sul disco (anche se lo ha scritto un'altra finestra).
// letto=false: il file c'è ma non si legge in questo momento (per esempio un'altra finestra lo
// sta sostituendo): chi salva usa allora quello che ha in memoria.
func leggiImpostazioni() (s impostazioni, letto bool) {
	b, err := os.ReadFile(fileImpostazioni())
	letto = err == nil || os.IsNotExist(err)
	if err == nil && len(b) < 64<<10 {
		letto = json.Unmarshal(b, &s) == nil
	}
	// Il vecchio "tarxz" (Tasso Zip 1.0) diventa la super compressione .7z.
	// Se non c'è niente (o è strano) si parte da .zip, il più compatibile.
	if s.Formato == "tarxz" {
		s.Formato = "7z"
	} else if !archivio.FormatoValido(s.Formato) {
		s.Formato = "zip"
	}
	if !livelloValido(s.Livello) {
		s.Livello = "massima"
	}
	s.Lingua = testi.Normalizza(s.Lingua)
	return s, letto
}

func impostazioniCopia() impostazioni {
	impMu.Lock()
	defer impMu.Unlock()
	return imp
}

// modificaImpostazioni cambia solo quello che fa f. Prima rilegge il file: con due finestre
// aperte, quello che ha cambiato l'altra (la lingua, il formato) non torna com'era.
// Tutto dentro il lucchetto, con un file di lavoro dal nome sempre nuovo nella stessa cartella,
// poi rinominato: mai un file di impostazioni a metà, e mai due salvataggi sullo stesso file.
func modificaImpostazioni(f func(*impostazioni)) {
	impMu.Lock()
	defer impMu.Unlock()
	s, letto := leggiImpostazioni()
	if !letto {
		s = imp
	}
	f(&s)
	imp = s
	b, _ := json.MarshalIndent(s, "", "  ")
	t, err := os.CreateTemp(cartella, "impostazioni-*.nuovo")
	if err != nil {
		registraErrore("impostazioni: %v", err)
		return
	}
	_, err = t.Write(b)
	if e := t.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(t.Name(), fileImpostazioni())
	}
	if err != nil {
		_ = os.Remove(t.Name())
		registraErrore("impostazioni: %v", err)
	}
}

// lingua: in ordine
//  1. --lingua=xx sulla riga di comando (per l'installatore dell'ecosistema);
//  2. la lingua scelta in alto nella finestra;
//  3. HKCU\Software\Tasso, valore "Lingua" (la lingua dell'Ecosistema Tasso, scritta dall'installatore);
//  4. la lingua di Windows;
//  5. inglese.
func lingua() string {
	if linguaRiga != "" {
		return linguaRiga
	}
	if l := impostazioniCopia().Lingua; l != "" {
		return l
	}
	return testi.Scegli(linguaEcosistema(), linguaWindows())
}

func linguaEcosistema() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Tasso`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, err := k.GetStringValue("Lingua")
	if err != nil {
		return ""
	}
	return s
}

var procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")

func linguaWindows() string {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	switch uint16(r) & 0x3FF { // lingua principale
	case 0x10:
		return "it"
	case 0x09:
		return "en"
	case 0x0A:
		return "es"
	case 0x0C:
		return "fr"
	case 0x07:
		return "de"
	case 0x16:
		return "pt"
	case 0x15:
		return "pl"
	}
	return ""
}

// registraErrore scrive una riga in %LOCALAPPDATA%\Tasso Zip\errori.txt (il file resta piccolo).
func registraErrore(formato string, args ...interface{}) {
	defer func() { _ = recover() }()
	p := filepath.Join(cartella, "errori.txt")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 256<<10 {
		_ = os.Rename(p, filepath.Join(cartella, "errori-vecchi.txt"))
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s  v%s  %s\r\n", time.Now().Format("2006-01-02 15:04:05"), Versione, fmt.Sprintf(formato, args...))
}

func livelloValido(l string) bool {
	switch archivio.Livello(l) {
	case archivio.LivelloVeloce, archivio.LivelloNormale, archivio.LivelloMassima, archivio.LivelloUltra:
		return true
	}
	return false
}
