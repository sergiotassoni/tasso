// Package icotema: quale icona usa Tasso con ogni tema (finestra, barra delle applicazioni,
// icona vicino all'orologio, collegamenti sul Desktop e nel menu Start, logo dentro Tasso).
//
// Le icone dei temi stanno DENTRO Tasso.exe come risorse di Windows (rsrc_windows_amd64.syso,
// preparato da strumenti/icone-temi): così i collegamenti puntano sempre a un file che esiste
// (Tasso.exe, con il numero della risorsa) e togliendo Tasso non resta nessun file.
//
// assets/temi/elenco.txt dice quale numero (ID della risorsa) ha l'icona di ogni tema:
//
//	# commento
//	201 oro gaming
//	202 oro ufficio
//
// I numeri non cambiano mai una volta dati (strumenti/icone-temi li conserva): un collegamento
// che punta a "Tasso.exe,-201" resta giusto anche dopo un aggiornamento.
// Un tema che non è nell'elenco (per esempio "tasso") usa l'icona normale di Tasso (ID 2).
package icotema

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// IDNormale: l'icona di sempre di Tasso (assets/tasso.ico), risorsa 2.
const IDNormale = 2

// Primo e ultimo numero per le icone dei temi (gruppi RT_GROUP_ICON). Le immagini (RT_ICON)
// delle icone dei temi partono da PrimaImmagine, lontane da quelle dell'icona normale (3, 4, ...).
const (
	PrimoID       = 201
	UltimoID      = 999
	PrimaImmagine = 1000
)

// Varianti: l'icona di Tasso in modalità Gaming e in modalità Ufficio.
const (
	Gaming  = "gaming"
	Ufficio = "ufficio"
)

// Voce: una riga dell'elenco.
type Voce struct {
	ID       uint16
	Tema     string
	Variante string
}

// Elenco: le icone dei temi che ci sono dentro Tasso.exe.
type Elenco struct {
	Voci []Voce
}

// TemaValido: solo lettere minuscole e cifre (gli id dei temi di Tasso), mai "tasso".
func TemaValido(t string) bool {
	if t == "" || len(t) > 32 || t == "tasso" {
		return false
	}
	for _, c := range t {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Leggi legge elenco.txt. Righe vuote e quelle con # sono saltate; una riga sbagliata è un errore
// (meglio accorgersene quando si prepara Tasso che avere un'icona sbagliata).
func Leggi(b []byte) (Elenco, error) {
	var e Elenco
	visti := map[uint16]bool{}
	coppie := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	n := 0
	for sc.Scan() {
		n++
		r := strings.TrimSpace(sc.Text())
		if r == "" || strings.HasPrefix(r, "#") {
			continue
		}
		f := strings.Fields(r)
		if len(f) != 3 {
			return Elenco{}, fmt.Errorf("elenco.txt riga %d: servono 3 parole (numero tema variante)", n)
		}
		id, err := strconv.Atoi(f[0])
		if err != nil || id < PrimoID || id > UltimoID {
			return Elenco{}, fmt.Errorf("elenco.txt riga %d: numero %q fuori da %d-%d", n, f[0], PrimoID, UltimoID)
		}
		if !TemaValido(f[1]) {
			return Elenco{}, fmt.Errorf("elenco.txt riga %d: tema %q non valido", n, f[1])
		}
		if f[2] != Gaming && f[2] != Ufficio {
			return Elenco{}, fmt.Errorf("elenco.txt riga %d: variante %q (gaming o ufficio)", n, f[2])
		}
		k := f[1] + " " + f[2]
		if visti[uint16(id)] || coppie[k] {
			return Elenco{}, fmt.Errorf("elenco.txt riga %d: numero o tema ripetuto", n)
		}
		visti[uint16(id)], coppie[k] = true, true
		e.Voci = append(e.Voci, Voce{uint16(id), f[1], f[2]})
	}
	if err := sc.Err(); err != nil {
		return Elenco{}, err
	}
	sort.Slice(e.Voci, func(i, j int) bool { return e.Voci[i].ID < e.Voci[j].ID })
	return e, nil
}

// Scrivi: il testo di elenco.txt.
func (e Elenco) Scrivi() []byte {
	var sb strings.Builder
	sb.WriteString("# Icone dei temi dentro Tasso.exe (risorse di Windows). Scritto da strumenti/icone-temi:\n")
	sb.WriteString("# i numeri non si cambiano mai (i collegamenti sul Desktop puntano a Tasso.exe,-numero).\n")
	vv := append([]Voce(nil), e.Voci...)
	sort.Slice(vv, func(i, j int) bool { return vv[i].ID < vv[j].ID })
	for _, v := range vv {
		fmt.Fprintf(&sb, "%d %s %s\n", v.ID, v.Tema, v.Variante)
	}
	return []byte(sb.String())
}

// Trova: il numero dell'icona di un tema in una variante, 0 se il tema non ha un'icona sua.
func (e Elenco) Trova(tema, variante string) uint16 {
	for _, v := range e.Voci {
		if v.Tema == tema && v.Variante == variante {
			return v.ID
		}
	}
	return 0
}

// IDPer: il numero della risorsa da usare. Con un tema senza icona sua, l'icona normale di Tasso.
// In Ufficio, se il tema ha solo l'icona Gaming, si usa quella (stesso tema, stessi colori).
func (e Elenco) IDPer(tema, variante string) uint16 {
	if id := e.Trova(tema, variante); id != 0 {
		return id
	}
	if variante == Ufficio {
		if id := e.Trova(tema, Gaming); id != 0 {
			return id
		}
	}
	return IDNormale
}

// Aggiungi dà un numero a tema+variante: quello che aveva già, o il primo libero (mai riusato
// uno di un'altra voce). Serve a strumenti/icone-temi.
func (e *Elenco) Aggiungi(tema, variante string) (uint16, error) {
	if id := e.Trova(tema, variante); id != 0 {
		return id, nil
	}
	if !TemaValido(tema) || (variante != Gaming && variante != Ufficio) {
		return 0, fmt.Errorf("tema %q o variante %q non validi", tema, variante)
	}
	usati := map[uint16]bool{}
	for _, v := range e.Voci {
		usati[v.ID] = true
	}
	for id := uint16(PrimoID); id <= UltimoID; id++ {
		if !usati[id] {
			e.Voci = append(e.Voci, Voce{id, tema, variante})
			return id, nil
		}
	}
	return 0, fmt.Errorf("numeri finiti")
}

// Variante: l'icona per la modalità di Tasso (profilo "ufficio" = Ufficio, il resto Gaming).
func Variante(profilo string) string {
	if profilo == "ufficio" {
		return Ufficio
	}
	return Gaming
}

// TemaInUso: il tema che si vede adesso, con le stesse regole dell'interfaccia (curTheme in
// ui/index.html): in Ufficio il tema dell'Ufficio (di base "ufficio"), altrimenti quello Gaming
// (di base "tasso").
func TemaInUso(profilo, tema, temaUfficio string) string {
	if profilo == "ufficio" {
		if temaUfficio == "" {
			return "ufficio"
		}
		return temaUfficio
	}
	if tema == "" {
		return "tasso"
	}
	return tema
}

// PosizioneIcona: il testo IconLocation di un collegamento per questa icona. Per l'icona normale
// (indice 0, come fa l'installatore); per le altre "-numero" = la risorsa con quel numero.
func PosizioneIcona(id uint16) int32 {
	if id == 0 || id == IDNormale {
		return 0
	}
	return -int32(id)
}

var (
	reSVGTitolo = regexp.MustCompile(`(?s)<title>.*?</title>`)
	reSVGId     = regexp.MustCompile(`\bid="([A-Za-z][A-Za-z0-9_-]*)"`)
	reSVGOcchio = regexp.MustCompile(`<ellipse cx="(74|126)(\.0+)?" cy="98"`)
)

// PreparaSVG: il disegno del tema pronto per la pagina di Tasso. Gli id interni (sfumature,
// maschere) prendono il prefisso pref, così due disegni nella pagina non si pestano; gli occhi
// prendono la classe "pup" (si muovono come nel logo di sempre); il titolo va via (il logo è
// decorativo, accanto c'è già la scritta TASSO).
func PreparaSVG(s, pref string) string {
	s = reSVGTitolo.ReplaceAllString(s, "")
	for _, m := range reSVGId.FindAllStringSubmatch(s, -1) {
		id := m[1]
		s = strings.ReplaceAll(s, `id="`+id+`"`, `id="`+pref+id+`"`)
		s = strings.ReplaceAll(s, `url(#`+id+`)`, `url(#`+pref+id+`)`)
	}
	s = reSVGOcchio.ReplaceAllStringFunc(s, func(x string) string { return strings.Replace(x, "<ellipse ", `<ellipse class="pup" `, 1) })
	s = strings.Replace(s, "<svg ", `<svg aria-hidden="true" focusable="false" `, 1)
	return strings.TrimSpace(s)
}
