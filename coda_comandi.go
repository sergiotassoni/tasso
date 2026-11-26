// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package main

import "strings"

// maxParametri: lunghezza massima dei parametri di un avvio. Windows accetta righe di comando
// fino a 32767 caratteri, ma ShellExecute in certe versioni si ferma molto prima (circa 2048):
// resto largamente sotto.
const maxParametri = 1800

// comandiRiapertura: i parametri per riaprire Tasso Zip con questi file, divisi in più avvii
// se sono tanti (ognuno sotto maxParametri). Gli avvii in più si uniscono da soli al primo con
// la coda del tasto destro, come quando Windows avvia un Tasso Zip per ogni file.
// I percorsi con virgolette o a capo (non validi su Windows) si scartano.
func comandiRiapertura(modo string, percorsi []string, max int) []string {
	testa := "--" + modo
	var out []string
	cur := testa
	for _, p := range percorsi {
		if p == "" || strings.ContainsAny(p, "\"\r\n") {
			continue
		}
		pezzo := ` "` + p + `"`
		if cur != testa && len(cur)+len(pezzo) > max {
			out = append(out, cur)
			cur = testa
		}
		cur += pezzo
	}
	if cur != testa {
		out = append(out, cur)
	}
	return out
}
