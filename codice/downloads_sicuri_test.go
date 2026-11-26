package main

import (
	"strings"
	"testing"
)

// Riordino e "Annulla il riordino" spostano file da amministratore: Download solo nel profilo vero,
// niente collegamenti su file e cartelle, elenco per annullare letto solo se vero e righe solo dentro
// Download.
func TestDownloadsSicuri(t *testing.T) {
	var riordino, annulla string
	for _, s := range allSteps {
		switch s.ID {
		case "downloads":
			riordino = s.Script
		case "downloads_undo":
			annulla = s.Script
		}
	}
	if riordino == "" || annulla == "" {
		t.Fatal("passaggi del riordino non trovati")
	}
	for nome, sc := range map[string]string{"downloads": riordino, "downloads_undo": annulla} {
		if strings.Contains(sc, "Get-DownloadsPath") || !strings.Contains(sc, "$dl = Get-DownloadsSicura") {
			t.Errorf("%s: Download preso senza i controlli (Get-DownloadsSicura)", nome)
		}
		if !strings.Contains(sc, "Test-NoReparse") {
			t.Errorf("%s: niente controllo dei collegamenti", nome)
		}
	}
	if !strings.Contains(riordino, "Test-DownloadsSposta $from $destDir $dl") {
		t.Error("downloads: file e cartella di arrivo non controllati prima di spostare")
	}
	if !strings.Contains(annulla, "ReparsePoint") || !strings.Contains(annulla, "GetDirectoryName($from) -ieq $dl") {
		t.Error("downloads_undo: elenco o righe accettati senza controlli")
	}
	if !strings.Contains(psPrelude, "function Get-DownloadsSicura") || !strings.Contains(psPrelude, "$TassoProfiloVero") {
		t.Error("Get-DownloadsSicura manca o non usa il profilo vero")
	}
}

// Cambiare lingua fra riordino e annullamento cambia i nomi delle cartelle (Immagini -> Pictures):
// il riordino scrive nell'elenco i nomi veri usati e l'annullamento li accetta da lì.
func TestDownloadsAnnullaDopoCambioLingua(t *testing.T) {
	var riordino, annulla string
	for _, s := range allSteps {
		switch s.ID {
		case "downloads":
			riordino = s.Script
		case "downloads_undo":
			annulla = s.Script
		}
	}
	if !strings.Contains(riordino, "'#cartelle' + [string][char]9 + (@($cats.Keys) -join") {
		t.Error("downloads: i nomi veri delle cartelle non finiscono nell'elenco")
	}
	for _, w := range []string{"$p[0] -ne '#cartelle'", "GetInvalidFileNameChars()", "@($nomi | Where-Object { $toDir -ieq (Join-Path $dl $_) })", "foreach ($c in $nomi) {"} {
		if !strings.Contains(annulla, w) {
			t.Errorf("downloads_undo: manca %q", w)
		}
	}
}
