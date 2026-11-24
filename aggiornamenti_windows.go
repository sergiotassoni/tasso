//go:build windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// trovaTasso: Tasso.exe dell'ecosistema. Prima la cartella scritta dall'installatore
// (HKLM\Software\Tasso\Cartella, solo gli amministratori la cambiano), poi la cartella sopra
// quella di Tasso PDF (<cartella>\Tasso PDF\TassoPDF.exe → <cartella>\Tasso.exe).
func trovaTasso() string {
	var cand []string
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `Software\Tasso`, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
		if v, _, err := k.GetStringValue("Cartella"); err == nil && strings.TrimSpace(v) != "" {
			cand = append(cand, filepath.Join(strings.TrimSpace(v), "Tasso.exe"))
		}
		k.Close()
	}
	if me, err := os.Executable(); err == nil {
		cand = append(cand, filepath.Join(filepath.Dir(filepath.Dir(me)), "Tasso.exe"))
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

// apriTassoAggWin: "open" su Tasso.exe (Tasso chiede l'amministratore: Windows mostra la richiesta).
func apriTassoAggWin() string {
	exe := trovaTasso()
	if exe == "" {
		return "nessuno"
	}
	if shellApri("open", exe, "--controlla-aggiornamenti") != nil {
		return "errore"
	}
	return "tasso"
}

func init() { piattaforma.apriTassoAgg = apriTassoAggWin }
