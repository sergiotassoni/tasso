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

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tassozip/testi"
)

// impostazioni salvate in %LOCALAPPDATA%\Tasso Zip\impostazioni.json.
type impostazioni struct {
	Lingua  string `json:"lingua,omitempty"`  // scelta dall'utente in alto nella finestra ("" = automatica)
	Formato string `json:"formato,omitempty"` // "tarxz" o "zip"
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
	b, err := os.ReadFile(fileImpostazioni())
	if err == nil && len(b) < 64<<10 {
		_ = json.Unmarshal(b, &imp)
	}
	if imp.Formato != "zip" && imp.Formato != "tarxz" {
		imp.Formato = "zip"
	}
	imp.Lingua = testi.Normalizza(imp.Lingua)
}

func impostazioniCopia() impostazioni {
	impMu.Lock()
	defer impMu.Unlock()
	return imp
}

func modificaImpostazioni(f func(*impostazioni)) {
	impMu.Lock()
	f(&imp)
	b, _ := json.MarshalIndent(imp, "", "  ")
	impMu.Unlock()
	// Scrivo in un file nuovo e poi lo rinomino: mai un file di impostazioni a metà.
	tmp := fileImpostazioni() + ".nuovo"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		registraErrore("impostazioni: %v", err)
		return
	}
	if err := os.Rename(tmp, fileImpostazioni()); err != nil {
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

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

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
