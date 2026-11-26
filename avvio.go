// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// avvio.go: i file con cui il programma è stato avviato (doppio clic su un PDF, menu del tasto
// destro di Windows) e quelli che arrivano dopo da altre copie avviate insieme (vedi coda_windows.go).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// modiRiga: le voci del menu del tasto destro (--apri, --unisci, --firma, --sblocca, --comprimi).
var modiRiga = map[string]bool{"apri": true, "unisci": true, "firma": true, "sblocca": true, "comprimi": true}

// leggiArgomenti: il modo e i file dalla riga di comando. Un file senza modo = "apri".
func leggiArgomenti(args []string) (string, []string) {
	modo := ""
	var percorsi []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--"):
			if m := strings.TrimPrefix(a, "--"); modiRiga[m] {
				modo = m
			}
		default:
			if p, err := filepath.Abs(a); err == nil {
				if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
					percorsi = append(percorsi, p)
				}
			}
		}
	}
	if modo == "" && len(percorsi) > 0 {
		modo = "apri"
	}
	if len(percorsi) == 0 {
		modo = ""
	}
	return modo, percorsi
}

var avvio = struct {
	sync.Mutex
	modo     string
	percorsi []string // file dell'avvio (consegnati una volta sola)
	arrivati []string // file arrivati dopo, da altre copie del programma
}{}

// FileAvvio: cosa la pagina deve aprire subito, o appena arriva.
type FileAvvio struct {
	Modo string     `json:"modo"`
	File []VoceInfo `json:"file"`
}

func apriTutti(pp []string) []VoceInfo {
	var out []VoceInfo
	for _, p := range pp {
		if vi, err := apriPercorso(p); err == nil {
			out = append(out, vi)
		}
	}
	return out
}

// goFileAvvio: i file dell'avvio (la prima volta) e poi quelli arrivati nel frattempo.
func goFileAvvio() FileAvvio {
	avvio.Lock()
	pp := append(avvio.percorsi, avvio.arrivati...)
	avvio.percorsi, avvio.arrivati = nil, nil
	modo := avvio.modo
	avvio.Unlock()
	if len(pp) == 0 {
		return FileAvvio{Modo: modo}
	}
	return FileAvvio{Modo: modo, File: apriTutti(pp)}
}

func arrivano(pp []string) {
	avvio.Lock()
	avvio.arrivati = append(avvio.arrivati, pp...)
	avvio.Unlock()
}

// Il menu del tasto destro di Windows (vedi menu_windows.go).
func goMenu() bool {
	if piattaforma.menuAttivo == nil {
		return false
	}
	return piattaforma.menuAttivo()
}

func goImpostaMenu(on bool) (bool, error) {
	if piattaforma.impostaMenu == nil {
		return false, perPagina(errc("menu", nil))
	}
	if err := piattaforma.impostaMenu(on, linguaIniziale()); err != nil {
		return goMenu(), perPagina(errc("menu", err))
	}
	return goMenu(), nil
}

// testiMenu: le voci del sottomenu "Tasso PDF", in tutte le lingue.
var testiMenu = map[string]map[string]string{
	"it": {"apri": "Apri con Tasso PDF", "unisci": "Unisci con Tasso PDF", "firma": "Firma con Tasso PDF", "sblocca": "Sblocca con Tasso PDF", "comprimi": "Comprimi con Tasso PDF"},
	"en": {"apri": "Open with Tasso PDF", "unisci": "Merge with Tasso PDF", "firma": "Sign with Tasso PDF", "sblocca": "Unlock with Tasso PDF", "comprimi": "Compress with Tasso PDF"},
	"es": {"apri": "Abrir con Tasso PDF", "unisci": "Unir con Tasso PDF", "firma": "Firmar con Tasso PDF", "sblocca": "Desbloquear con Tasso PDF", "comprimi": "Comprimir con Tasso PDF"},
	"fr": {"apri": "Ouvrir avec Tasso PDF", "unisci": "Fusionner avec Tasso PDF", "firma": "Signer avec Tasso PDF", "sblocca": "Déverrouiller avec Tasso PDF", "comprimi": "Compresser avec Tasso PDF"},
	"de": {"apri": "Mit Tasso PDF öffnen", "unisci": "Mit Tasso PDF zusammenfügen", "firma": "Mit Tasso PDF unterschreiben", "sblocca": "Mit Tasso PDF entsperren", "comprimi": "Mit Tasso PDF komprimieren"},
	"pt": {"apri": "Abrir com o Tasso PDF", "unisci": "Juntar com o Tasso PDF", "firma": "Assinar com o Tasso PDF", "sblocca": "Desbloquear com o Tasso PDF", "comprimi": "Comprimir com o Tasso PDF"},
	"pl": {"apri": "Otwórz w Tasso PDF", "unisci": "Połącz w Tasso PDF", "firma": "Podpisz w Tasso PDF", "sblocca": "Odblokuj w Tasso PDF", "comprimi": "Kompresuj w Tasso PDF"},
}

func testoMenu(l, k string) string {
	if m, ok := testiMenu[l]; ok && m[k] != "" {
		return m[k]
	}
	return testiMenu["en"][k]
}

// ordinaPerNome: i file arrivati insieme dal tasto destro, in ordine di nome (come in Esplora
// file: "2.jpg" prima di "10.jpg").
func ordinaPerNome(pp []string) {
	sort.SliceStable(pp, func(i, j int) bool { return menoNaturale(filepath.Base(pp[i]), filepath.Base(pp[j])) })
}

// menoNaturale: confronto "naturale" senza maiuscole: i numeri dentro i nomi valgono come numeri.
func menoNaturale(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		ca, cb := a[0], b[0]
		if ca >= '0' && ca <= '9' && cb >= '0' && cb <= '9' {
			i, j := 0, 0
			for i < len(a) && a[i] >= '0' && a[i] <= '9' {
				i++
			}
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			na, nb := strings.TrimLeft(a[:i], "0"), strings.TrimLeft(b[:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			a, b = a[i:], b[j:]
			continue
		}
		if ca != cb {
			return ca < cb
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}
