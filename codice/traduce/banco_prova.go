package traduce

// Banco di prova universale della traduzione dello schermo: tante schermate di giochi diversi
// passano per lo STESSO percorso della traduzione vera (OCR, rilettura delle righe incerte,
// correzione, righe intere, frasi pronte, traduttore veloce, riquadro di copertura) e per ogni
// riga si scrive cosa è successo. Così si misura su tutti i giochi insieme, non su uno solo.
//
// Su Windows (Tasso-collaudo) l'OCR è quello vero: --prova-traduzione=<cartella> e
// --prova-traduzione-schermo=<cartella>[,secondi]. Su Linux (test) l'OCR è finto: per ogni
// immagine "nome.png" un file "nome.ocr.json" con le righe lette ([{"p":[{"t":..,"x":..}]}]).

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// opzioniBanco: le parti che cambiano tra Windows e i test.
type opzioniBanco struct {
	leggi   func(img *image.RGBA) ([]RigaOCR, float64, error) // OCR (con rilettura delle righe incerte)
	tr      *Traduttore
	disegna func(img *image.RGBA, pronti []Blocco, percorso string) error // immagine "come la vede il giocatore"
	// dopo: facoltativa, dopo la prima traduzione (es. aspetta il correttore IA e ritraduce)
	dopo func(da []Blocco, ris []Risultato) []Risultato
	voti *votazione // per i fotogrammi di un video: la stabilità nel tempo come nel gioco
	// stato: per i fotogrammi di un video, le scritte mostrate nel fotogramma prima (come
	// blocchiPagina nel gioco: stessa traduzione tenuta ferma, scritte saltate dalla lettura
	// recuperate). nil = ogni immagine per conto suo.
	stato *statoBanco
}

// statoBanco: quello che il gioco ricorda tra una lettura e l'altra.
type statoBanco struct {
	vecchi []Blocco
	fisse  *traduzioniFisse // traduzioni già mostrate: la stessa scritta esce sempre uguale
	ist    *isteresi        // niente sfarfallio tra un fotogramma e l'altro
	ora    time.Time        // orologio finto: ogni fotogramma è un giro di rilettura del gioco
}

// passoBanco: quanto tempo passa tra due fotogrammi del banco (come un giro di rilettura nel gioco).
const passoBanco = 250 * time.Millisecond

// esitoImmagine: una schermata del banco.
type esitoImmagine struct {
	Nome          string
	Righe         []infoRiga
	MsOCR, MsTot  int64
	Errore        string
	Mostrate      int
	Scartate      int
	Mezze         int // traduzioni mezze in inglese trovate (scartate): il disegno deve averne 0
	MezzeMostrate int // righe mezze tradotte DISEGNATE: deve essere sempre 0
	// Fratelli: scritte pronte tolte perché toccano un pezzo della stessa frase rimasto in
	// inglese (senzaMezzeRighe), anche quelle rimaste dal fotogramma prima. Contano tra le mezze
	// trovate e scartate.
	Fratelli int
}

// elaboraImmagine: il percorso completo su una schermata. Restituisce l'esito e le scritte disegnate.
func elaboraImmagine(img *image.RGBA, nome string, o opzioniBanco) (esitoImmagine, []Blocco) {
	e := esitoImmagine{Nome: nome}
	t0 := time.Now()
	righe, f, err := o.leggi(img)
	e.MsOCR = time.Since(t0).Milliseconds()
	if err != nil {
		e.Errore = err.Error()
		return e, nil
	}
	blocchi := blocchiLetti(img, righe, f, o.voti, time.Now())
	spazioLibero(blocchi, img.Bounds().Dx())
	var da []Blocco
	var inglesi []Blocco // le scritte che restano in inglese con parole vere (per senzaMezzeRighe)
	for _, b := range blocchi {
		info := infoRiga{Grezzo: b.Grezzo, Corretto: b.Testo, Affid: b.Affid, X: b.X, Y: b.Y, W: b.W, H: b.H}
		if b.Grezzo == "" {
			info.Grezzo = b.Testo
		}
		if m := motivoNonTradurre(b); m != "" {
			info.Scartata = m
			e.Righe = append(e.Righe, info)
			if ingleseVisibile(m) {
				inglesi = append(inglesi, b)
			}
			continue
		}
		if m := controllaCompleta(img, &b, blocchi); m != "" {
			info.Scartata = m
			e.Righe = append(e.Righe, info)
			if ingleseVisibile(m) {
				inglesi = append(inglesi, b)
			}
			continue
		}
		da = append(da, b)
	}
	ctx := conLotto(context.Background())
	tTrad := time.Now()
	var fisse *traduzioniFisse
	if o.stato != nil {
		if o.stato.fisse == nil {
			o.stato.fisse, o.stato.ist, o.stato.ora = &traduzioniFisse{}, &isteresi{}, time.Unix(1_000_000, 0)
		}
		o.stato.ora = o.stato.ora.Add(passoBanco)
		fisse = o.stato.fisse
	}
	ris := traduciTutteFisse(ctx, o.tr, da, false, fisse)
	if o.dopo != nil {
		ris = o.dopo(da, ris)
	}
	da, ris = riprovaFile(ctx, o.tr, img, da, ris, blocchi) // file di pulsanti lette come una riga
	var pronti []Blocco
	for i, b := range da {
		r := ris[i]
		info := infoRiga{Grezzo: b.Grezzo, Corretto: b.Testo, Affid: b.Affid, Tradotto: r.Tradotto, Fonte: r.Motore, Ms: r.Millisec,
			X: b.X, Y: b.Y, W: b.W, H: b.H}
		if r.Millisec == 0 {
			info.Ms = time.Since(tTrad).Milliseconds() / int64(max(1, len(da)))
		}
		if m := motivoScartaTraduzione(b, r); m != "" {
			info.Scartata = m
			if strings.HasPrefix(m, "mezza") {
				e.Mezze++
			}
			e.Righe = append(e.Righe, info)
			if ingleseVisibile(m) {
				inglesi = append(inglesi, b)
			}
			continue
		}
		p := preparaScritta(img, img, b, r)
		if m := adattaAlRiquadro(&p); m != "" { // suggerimento di un tasto che non ci sta nel suo riquadro
			info.Scartata = m
			e.Righe = append(e.Righe, info)
			inglesi = append(inglesi, b)
			continue
		}
		fisse.fissa(coppiaDi(o.tr), b.Testo, r.Tradotto)
		if r.Motore != "catalogo" && rigaMezzaTradotta(p.Testo, p.Tradotto) { // stessa regola di motivoScartaTraduzione
			e.MezzeMostrate++
		}
		pronti = append(pronti, p)
		info.Tradotto = p.Tradotto
		e.Righe = append(e.Righe, info)
	}
	// segna come scartate (con il motivo) le righe delle scritte pronte che non sono più in tenute
	segnaTolte := func(tenute []Blocco, motivo string) {
		for i := range e.Righe {
			r := &e.Righe[i]
			if r.Scartata != "" {
				continue
			}
			resta := false
			for _, p := range tenute {
				if p.X == r.X && p.Y == r.Y && p.W == r.W && p.H == r.H {
					resta = true
					break
				}
			}
			if !resta {
				r.Scartata = motivo
			}
		}
	}
	// mai due traduzioni una sopra l'altra (restano in inglese tutte e due)
	if tenute := senzaSovrapposte(pronti); len(tenute) != len(pronti) {
		segnaTolte(tenute, "traduzioni sovrapposte")
		for _, p := range pronti {
			if !contieneBlocco(tenute, p) {
				inglesi = append(inglesi, p)
			}
		}
		pronti = tenute
	}
	// mai una scritta tradotta accanto a un pezzo della stessa frase rimasto in inglese
	if tenute, tolte := senzaMezzeRighe(pronti, inglesi); len(tolte) > 0 {
		segnaTolte(tenute, motivoMezzaRiga)
		e.Fratelli += len(tolte)
		inglesi = append(inglesi, tolte...)
		pronti = tenute
	}
	if o.stato != nil { // come nel gioco: la traduzione già mostrata nello stesso posto resta quella
		stabilizza(pronti, o.stato.vecchi)
		for i := range e.Righe {
			for _, p := range pronti {
				if p.X == e.Righe[i].X && p.Y == e.Righe[i].Y && p.W == e.Righe[i].W && p.H == e.Righe[i].H && e.Righe[i].Scartata == "" {
					e.Righe[i].Tradotto = p.Tradotto
				}
			}
		}
	}
	if len(pronti) > 0 {
		rifinisciPagina(img, pronti, blocchi)
	}
	if o.stato != nil {
		prima := append([]Blocco(nil), pronti...)
		pronti, inglesi = o.stato.ist.applica(pronti, o.stato.vecchi, inglesi, img, o.stato.ora)
		// le righe del report: quelle tenute dall'isteresi dicono la traduzione mostrata davvero
		for _, p := range pronti {
			if contieneBlocco(prima, p) {
				continue
			}
			rp := rettBlocco(p)
			for i := range e.Righe {
				r := &e.Righe[i]
				if sovrapposti(rp, image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)) {
					r.Scartata, r.Tradotto, r.Fonte = "", p.Tradotto, "fotogramma prima"
				}
			}
		}
		n := len(pronti)
		pronti = tieniFerme(pronti, o.stato.vecchi, img)
		pronti = recuperaPerse(pronti, o.stato.vecchi, img, inglesi) // saltate dalla lettura ma ancora lì
		recuperate := append([]Blocco(nil), pronti[n:]...)
		// le traduzioni tenute o recuperate dal fotogramma prima passano lo stesso controllo
		if tenute, tolte := senzaMezzeRighe(pronti, inglesi); len(tolte) > 0 {
			e.Fratelli += len(tolte)
			for _, t := range tolte {
				for i := range e.Righe {
					r := &e.Righe[i]
					if r.Scartata == "" && r.X == t.X && r.Y == t.Y && r.W == t.W && r.H == t.H {
						r.Scartata = motivoMezzaRiga
					}
				}
			}
			pronti = tenute
		}
		for _, p := range recuperate {
			if !contieneBlocco(pronti, p) {
				continue
			}
			rp := image.Rect(p.X, p.Y, p.X+p.W, p.Y+p.H)
			for i := range e.Righe {
				r := &e.Righe[i]
				if r.Scartata != "" && sovrapposti(rp, image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)) {
					r.Scartata, r.Tradotto, r.Fonte = "", p.Tradotto, "fotogramma prima"
				}
			}
		}
		o.stato.vecchi = pronti
	}
	// controllo finale sul disegno: nessuna scritta tradotta accanto a un pezzo della stessa
	// frase rimasto in inglese, nessuna traduzione mezza inglese (devono essere 0)
	e.MezzeMostrate += mezzeDisegnate(pronti, inglesi)
	e.Mezze += e.Fratelli
	for _, r := range e.Righe {
		if r.Scartata == "" {
			e.Mostrate++
		} else {
			e.Scartate++
		}
	}
	e.MsTot = time.Since(t0).Milliseconds()
	sort.SliceStable(e.Righe, func(i, j int) bool {
		if abs(e.Righe[i].Y-e.Righe[j].Y) > 6 {
			return e.Righe[i].Y < e.Righe[j].Y
		}
		return e.Righe[i].X < e.Righe[j].X
	})
	return e, pronti
}

// immaginiCartella: le schermate (png, jpg) della cartella, in ordine di nome. Salta quelle già
// prodotte dal banco ("-tradotto.png").
func immaginiCartella(dir string) []string {
	voci, _ := os.ReadDir(dir)
	var out []string
	for _, v := range voci {
		n := strings.ToLower(v.Name())
		if v.IsDir() || strings.HasSuffix(n, "-tradotto.png") || strings.HasSuffix(n, "-copertura.png") {
			continue
		}
		if strings.HasSuffix(n, ".png") || strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".jpeg") {
			out = append(out, filepath.Join(dir, v.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func apriImmagineRGBA(p string) (*image.RGBA, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
	return out, nil
}

// bancoCartella: tutte le immagini della cartella, con report (banco-prova.html e .txt).
func bancoCartella(dir string, o opzioniBanco) ([]esitoImmagine, error) {
	file := immaginiCartella(dir)
	if len(file) == 0 {
		return nil, fmt.Errorf("nessuna immagine (png/jpg) in %s", dir)
	}
	var esiti []esitoImmagine
	for _, p := range file {
		img, err := apriImmagineRGBA(p)
		nome := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		if err != nil {
			esiti = append(esiti, esitoImmagine{Nome: nome, Errore: err.Error()})
			continue
		}
		e, pronti := elaboraImmagine(img, nome, o)
		if o.disegna != nil {
			if err := o.disegna(img, pronti, filepath.Join(dir, nome+"-tradotto.png")); err != nil && e.Errore == "" {
				e.Errore = "disegno: " + err.Error()
			}
		}
		esiti = append(esiti, e)
	}
	return esiti, scriviReportBanco(dir, esiti, nil)
}

// instabilita: tra due fotogrammi di fila, la stessa scritta (stesso posto, stessa lettura) con
// una traduzione diversa, o mostrata in uno e scartata nell'altro.
type instabilita struct {
	Da, A       string
	Corretto    string
	Prima, Dopo string
}

func confrontaFotogrammi(a, b esitoImmagine) []instabilita {
	var out []instabilita
	for _, ra := range a.Righe {
		for _, rb := range b.Righe {
			r1 := image.Rect(ra.X, ra.Y, ra.X+ra.W, ra.Y+ra.H)
			r2 := image.Rect(rb.X, rb.Y, rb.X+rb.W, rb.Y+rb.H)
			if !sovrapposti(r1, r2) || !stessaScritta(ra.Grezzo, rb.Grezzo) {
				continue
			}
			pa, pb := ra.Tradotto, rb.Tradotto
			if ra.Scartata != "" {
				pa = "(inglese: " + ra.Scartata + ")"
			}
			if rb.Scartata != "" {
				pb = "(inglese: " + rb.Scartata + ")"
			}
			if pa != pb && (ra.Scartata == "" || rb.Scartata == "") { // inglese e inglese: sullo schermo non cambia niente
				out = append(out, instabilita{Da: a.Nome, A: b.Nome, Corretto: rb.Corretto, Prima: pa, Dopo: pb})
			}
			break
		}
	}
	return out
}

// scriviReportBanco: report HTML (con le immagini) e testo, con il riepilogo e i casi peggiori.
func scriviReportBanco(dir string, esiti []esitoImmagine, instab []instabilita) error {
	var tot, mostrate, scartate, mezze, mezzeMostrate, fratelli, catalogo, veloce, ia, correzioni int
	var msTot, msOCR int64
	type caso struct {
		nome string
		r    infoRiga
	}
	var peggiScartate, peggiCorrette, rifiutate []caso
	for _, e := range esiti {
		msTot += e.MsTot
		msOCR += e.MsOCR
		mezze += e.Mezze
		mezzeMostrate += e.MezzeMostrate
		fratelli += e.Fratelli
		for _, r := range e.Righe {
			tot++
			if r.Scartata != "" {
				scartate++
				if strings.TrimSpace(r.Tradotto) != "" {
					rifiutate = append(rifiutate, caso{e.Nome, r}) // c'era una traduzione, ma non si è disegnata
				}
				if r.Scartata != "non è testo da tradurre" && r.Scartata != motivoUgualeIt {
					peggiScartate = append(peggiScartate, caso{e.Nome, r})
				}
				continue
			}
			mostrate++
			switch {
			case r.Fonte == "catalogo":
				catalogo++
			case strings.Contains(r.Fonte, "IA"):
				ia++
			default:
				veloce++
			}
			if r.Grezzo != r.Corretto {
				correzioni++
				peggiCorrette = append(peggiCorrette, caso{e.Nome, r})
			}
		}
	}
	n := max(1, len(esiti))
	var t strings.Builder
	fmt.Fprintf(&t, "BANCO DI PROVA DELLA TRADUZIONE  (%s)\r\n\r\n", time.Now().Format("02/01/2006 15:04"))
	fmt.Fprintf(&t, "Schermate: %d   righe lette: %d   tradotte e mostrate: %d   lasciate in inglese: %d\r\n", len(esiti), tot, mostrate, scartate)
	fmt.Fprintf(&t, "Fonti: frasi pronte %d, traduttore veloce/memoria %d, IA %d   righe corrette dall'OCR: %d\r\n", catalogo, veloce, ia, correzioni)
	fmt.Fprintf(&t, "Righe mezze tradotte DISEGNATE: %d (deve essere 0)   mezze trovate e scartate: %d (di cui accanto a un pezzo inglese della stessa frase, anche dal fotogramma prima: %d)\r\n", mezzeMostrate, mezze, fratelli)
	fmt.Fprintf(&t, "Tempo medio per schermata: %d ms (lettura %d ms)\r\n", msTot/int64(n), msOCR/int64(n))
	if instab != nil {
		fmt.Fprintf(&t, "Instabilità tra fotogrammi di fila: %d\r\n", len(instab))
	}
	t.WriteString("\r\n")
	for _, e := range esiti {
		fmt.Fprintf(&t, "=== %s  (lettura %d ms, totale %d ms, mostrate %d, in inglese %d)%s\r\n", e.Nome, e.MsOCR, e.MsTot, e.Mostrate, e.Scartate, testoErrore(e.Errore))
		for _, r := range e.Righe {
			fmt.Fprintf(&t, "  [%d,%d] letto: %q\r\n", r.X, r.Y, r.Grezzo)
			if r.Corretto != r.Grezzo {
				fmt.Fprintf(&t, "         corretto: %q\r\n", r.Corretto)
			}
			if r.Scartata != "" {
				fmt.Fprintf(&t, "         affidabilità %d  -> resta in inglese: %s\r\n", r.Affid, r.Scartata)
				if strings.HasPrefix(r.Scartata, "mezza") && strings.TrimSpace(r.Tradotto) != "" {
					fmt.Fprintf(&t, "         traduzione scartata: %q\r\n", r.Tradotto) // per capire se si buttano traduzioni buone
				}
			} else {
				fmt.Fprintf(&t, "         affidabilità %d  -> %q  (%s, %d ms)\r\n", r.Affid, r.Tradotto, r.Fonte, r.Ms)
			}
		}
	}
	if len(instab) > 0 {
		t.WriteString("\r\n=== INSTABILITÀ (la stessa scritta tradotta in modo diverso tra due fotogrammi di fila)\r\n")
		for _, x := range instab {
			fmt.Fprintf(&t, "  %s -> %s  %q: %q / %q\r\n", x.Da, x.A, x.Corretto, x.Prima, x.Dopo)
		}
	}
	if len(peggiScartate) > 0 {
		t.WriteString("\r\n=== RIGHE LASCIATE IN INGLESE (da guardare)\r\n")
		for i, c := range peggiScartate {
			if i >= 60 {
				break
			}
			fmt.Fprintf(&t, "  %s: %q (%s)\r\n", c.nome, c.r.Grezzo, c.r.Scartata)
		}
	}
	if len(rifiutate) > 0 {
		fmt.Fprintf(&t, "\r\n=== TRADUZIONI SCARTATE (c'era una traduzione ma è rimasto l'inglese, e perché): %d\r\n", len(rifiutate))
		for i, c := range rifiutate {
			if i >= 200 {
				fmt.Fprintf(&t, "  ... e altre %d\r\n", len(rifiutate)-i)
				break
			}
			fmt.Fprintf(&t, "  %s: %q -> %q  (%s)\r\n         perché: %s\r\n", c.nome, c.r.Corretto, c.r.Tradotto, c.r.Fonte, c.r.Scartata)
		}
	}
	if len(peggiCorrette) > 0 {
		sort.SliceStable(peggiCorrette, func(i, j int) bool {
			return distanzaLettere(peggiCorrette[i].r.Grezzo, peggiCorrette[i].r.Corretto) > distanzaLettere(peggiCorrette[j].r.Grezzo, peggiCorrette[j].r.Corretto)
		})
		t.WriteString("\r\n=== RIGHE CON PIÙ CORREZIONI DELL'OCR\r\n")
		for i, c := range peggiCorrette {
			if i >= 40 {
				break
			}
			fmt.Fprintf(&t, "  %s: %q -> %q\r\n", c.nome, c.r.Grezzo, c.r.Corretto)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "banco-prova.txt"), []byte(t.String()), 0o644); err != nil {
		return err
	}
	// HTML: stesso contenuto, con le immagini tradotte
	var h strings.Builder
	h.WriteString(`<!DOCTYPE html><html lang="it"><head><meta charset="utf-8"><title>Banco di prova della traduzione</title><style>
body{font-family:Segoe UI,Arial,sans-serif;background:#12151c;color:#e8ecf2;margin:24px}h1{color:#3ee6b0}
table{border-collapse:collapse;width:100%;margin:8px 0 28px}td,th{border-bottom:1px solid #2a3140;padding:4px 8px;font-size:13px;text-align:left;vertical-align:top}
th{color:#9aa6b8}.no{color:#ff9a7a}.si{color:#3ee6b0}.r{background:#1b2030;padding:12px;border-radius:10px;margin:12px 0}img{max-width:100%;border-radius:8px}
</style></head><body><h1>Banco di prova della traduzione</h1><div class="r"><pre>`)
	h.WriteString(html.EscapeString(strings.SplitN(t.String(), "\r\n\r\n", 3)[1]))
	h.WriteString("</pre></div>")
	if len(rifiutate) > 0 {
		fmt.Fprintf(&h, "<h2>Traduzioni scartate (%d)</h2><p>C'era una traduzione, ma sullo schermo è rimasto l'inglese. Qui il perché, da ricontrollare.</p>", len(rifiutate))
		h.WriteString("<table><tr><th>schermata</th><th>inglese</th><th>traduzione scartata</th><th>fonte</th><th>perché</th></tr>")
		for _, c := range rifiutate {
			fmt.Fprintf(&h, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class="no">%s</td></tr>`,
				html.EscapeString(c.nome), html.EscapeString(c.r.Corretto), html.EscapeString(c.r.Tradotto), html.EscapeString(c.r.Fonte), html.EscapeString(c.r.Scartata))
		}
		h.WriteString("</table>")
	}
	for _, e := range esiti {
		fmt.Fprintf(&h, "<h2>%s</h2><p>lettura %d ms, totale %d ms, mostrate %d, in inglese %d%s</p>", html.EscapeString(e.Nome), e.MsOCR, e.MsTot, e.Mostrate, e.Scartate, html.EscapeString(testoErrore(e.Errore)))
		if _, err := os.Stat(filepath.Join(dir, e.Nome+"-tradotto.png")); err == nil {
			fmt.Fprintf(&h, `<img src="%s" alt="">`, html.EscapeString(e.Nome+"-tradotto.png"))
		}
		h.WriteString("<table><tr><th>posto</th><th>letto (OCR)</th><th>corretto</th><th>affid.</th><th>traduzione</th><th>fonte</th><th>ms</th></tr>")
		for _, r := range e.Righe {
			tr, cl := html.EscapeString(r.Tradotto), "si"
			if r.Scartata != "" {
				tr, cl = "resta in inglese: "+html.EscapeString(r.Scartata), "no"
			}
			fmt.Fprintf(&h, `<tr><td>%d,%d</td><td>%s</td><td>%s</td><td>%d</td><td class="%s">%s</td><td>%s</td><td>%d</td></tr>`,
				r.X, r.Y, html.EscapeString(r.Grezzo), html.EscapeString(r.Corretto), r.Affid, cl, tr, html.EscapeString(r.Fonte), r.Ms)
		}
		h.WriteString("</table>")
	}
	h.WriteString("</body></html>")
	return os.WriteFile(filepath.Join(dir, "banco-prova.html"), []byte(h.String()), 0o644)
}

func testoErrore(e string) string {
	if e == "" {
		return ""
	}
	return "  ERRORE: " + e
}

// ---------------------------------------------------------------- parti senza Windows (test)

// leggiOCRFinto: l'OCR finto dei test: "nome.ocr.json" accanto all'immagine (pixel dell'immagine, scala 1).
func leggiOCRFinto(percorsoImmagine string) func(img *image.RGBA) ([]RigaOCR, float64, error) {
	return func(img *image.RGBA) ([]RigaOCR, float64, error) {
		base := strings.TrimSuffix(percorsoImmagine, filepath.Ext(percorsoImmagine))
		b, err := os.ReadFile(base + ".ocr.json")
		if err != nil {
			return nil, 0, err
		}
		var righe []RigaOCR
		if err := json.Unmarshal(b, &righe); err != nil {
			return nil, 0, err
		}
		return righe, 1, nil
	}
}

// disegnaCopertura: senza i caratteri di Windows, l'immagine con le toppe (sfondo ricostruito al
// posto dell'inglese) e un riquadro verde dove andrebbe l'italiano: per vedere la copertura.
func disegnaCopertura(img *image.RGBA, pronti []Blocco, percorso string) error {
	out := image.NewRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	if len(pronti) > 0 && pronti[0].Pulita != nil {
		copy(out.Pix, pronti[0].Pulita.Pix) // le lettere inglesi tradotte cancellate
	}
	verde := color.RGBA{62, 230, 176, 255}
	for _, b := range pronti {
		r := calcolaToppa(b, image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H), b.W, max(b.Marg, 2), max(1, b.Righe) == 1)
		for x := r.Min.X; x < r.Max.X; x++ {
			for _, y := range []int{r.Min.Y, r.Max.Y - 1} {
				if (image.Point{x, y}).In(out.Bounds()) {
					out.SetRGBA(x, y, verde)
				}
			}
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for _, x := range []int{r.Min.X, r.Max.X - 1} {
				if (image.Point{x, y}).In(out.Bounds()) {
					out.SetRGBA(x, y, verde)
				}
			}
		}
	}
	f, err := os.Create(percorso)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}

// contieneBlocco: lo stesso riquadro è nella lista.
func contieneBlocco(l []Blocco, b Blocco) bool {
	for _, x := range l {
		if x.X == b.X && x.Y == b.Y && x.W == b.W && x.H == b.H {
			return true
		}
	}
	return false
}
