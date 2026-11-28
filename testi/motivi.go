// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package testi

import "strings"

// Motivi traduce le voci "nome (codice)" degli elenchi di file saltati o rovinati: il codice
// (es. "collegamento", "percorso_non_sicuro") diventa il testo "motivo_<codice>" nella lingua.
// Le voci senza un codice conosciuto restano come sono.
func Motivi(lingua string, voci []string) []string {
	if len(voci) == 0 {
		return voci
	}
	out := make([]string, len(voci))
	for i, v := range voci {
		out[i] = v
		j := strings.LastIndex(v, " (")
		if j < 0 || !strings.HasSuffix(v, ")") {
			continue
		}
		codice := v[j+2 : len(v)-1]
		if k := "motivo_" + codice; T(lingua, k) != k {
			out[i] = v[:j] + " (" + T(lingua, k) + ")"
		}
	}
	return out
}

// ConMotivo conta le voci "nome (codice)" con uno dei codici dati (prima di Motivi, che li
// traduce): per esempio i file non estratti perché hanno una password ("con_password").
func ConMotivo(voci []string, codici ...string) int {
	n := 0
	for _, v := range voci {
		for _, c := range codici {
			if strings.HasSuffix(v, " ("+c+")") {
				n++
				break
			}
		}
	}
	return n
}

// frasiErrori: i messaggi degli errori di Tasso Zip (in italiano nel programma) e il testo
// che li spiega nella lingua scelta.
var frasiErrori = []struct{ frase, chiave string }{
	{"serve troppa memoria per aprire questo archivio", "err_memoria"},
	{"l'archivio si espanderebbe troppo (possibile zip bomb)", "err_bomba"},
	{"zip diviso in parti .z01: non supportato", "err_zip_diviso"},
	{"zip con password: non supportato", "err_zip_cifrato"},
	{"archivio rovinato o incompleto", "err_rovinato"},
	{"manca una parte dell'archivio", "err_parte_mancante"},
	{"l'archivio contiene troppi file", "err_troppe_voci"},
	{"spazio su disco insufficiente", "err_spazio"},
	{"formato non riconosciuto", "err_formato"},
	{"nessun file da comprimere", "err_niente"},
	{"non posso scrivere nella cartella", "err_permessi"},
	{"password sbagliata", "err_password_sbagliata"},
	{"serve la password", "err_password"},
	{"dati non corretti", "motivo_rovinato"},
}

// Dettagli traduce le parti in italiano dei dettagli di un errore (quello che resta, scritto
// dalle librerie, è tecnico e resta com'è).
func Dettagli(lingua, d string) string {
	if lingua == "it" || d == "" {
		return d
	}
	for _, f := range frasiErrori {
		if strings.Contains(d, f.frase) {
			d = strings.ReplaceAll(d, f.frase, strings.TrimRight(T(lingua, f.chiave), "."))
		}
	}
	return d
}
