// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

// Package testi contiene i testi dell'interfaccia nelle lingue di Tasso Zip.
// Per aggiungere una lingua basta aggiungerla a testi.json (con tutte le chiavi dell'italiano).
package testi

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed testi.json
var grezzo []byte

// Lingue disponibili, nell'ordine in cui compaiono in alto nella finestra.
var Lingue = []string{"it", "en", "es", "fr", "de", "pt", "pl"}

var (
	once sync.Once
	diz  map[string]map[string]string
)

func carica() {
	once.Do(func() {
		diz = map[string]map[string]string{}
		_ = json.Unmarshal(grezzo, &diz)
	})
}

// Tutti restituisce l'intero dizionario (lingua -> chiave -> testo).
func Tutti() map[string]map[string]string {
	carica()
	return diz
}

// Normalizza "it-IT", "IT", "pt_BR"... in un codice di Lingue ("" se non c'è).
func Normalizza(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if len(l) > 2 {
		l = l[:2]
	}
	for _, x := range Lingue {
		if x == l {
			return x
		}
	}
	return ""
}

// Scegli la prima lingua disponibile tra le preferenze (in ordine); se nessuna va bene, inglese.
func Scegli(preferenze ...string) string {
	for _, p := range preferenze {
		if n := Normalizza(p); n != "" {
			return n
		}
	}
	return "en"
}

// T restituisce il testo della chiave nella lingua (se manca: inglese, poi italiano, poi la chiave).
func T(lingua, chiave string) string {
	carica()
	for _, l := range []string{lingua, "en", "it"} {
		if s, ok := diz[l][chiave]; ok && s != "" {
			return s
		}
	}
	return chiave
}
