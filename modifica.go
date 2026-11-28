// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// modifica.go: gli strumenti della vista documento che cambiano il PDF: organizza le pagine
// (ordine, rotazione, elimina, estrai, inserisci), aggiungi testo, immagini, rettangoli,
// filigrana, intestazione e piè di pagina, collegamenti, oscura (vera), proteggi, comprimi.
// Ogni funzione riceve i byte di un PDF e ne restituisce uno NUOVO: l'originale non si tocca.

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

var bg = context.Background()

// apriChiaro: il contesto del PDF senza protezioni, pronto per essere modificato.
func apriChiaro(data []byte, pw string) (*model.Context, error) {
	plain, err := inChiaro(data, pw)
	if err != nil {
		return nil, err
	}
	ctx, _, err := leggi(plain, "")
	if err != nil {
		return nil, err
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, classifica(err)
	}
	return ctx, nil
}

// paginaGeo: il dizionario della pagina, la parte visibile e la rotazione.
type paginaGeo struct {
	d      types.Dict
	ref    *types.IndirectRef
	vb     *types.Rectangle
	rot    int
	dw, dh float64 // dimensioni della pagina come si vede (già girata)
	attrs  *model.InheritedPageAttrs
}

func geometria(ctx *model.Context, p int) (*paginaGeo, error) {
	d, ref, attrs, err := ctx.PageDict(bg, p, false)
	if err != nil || d == nil || attrs == nil {
		return nil, errc("rovinato", err)
	}
	vb := attrs.MediaBox
	if attrs.CropBox != nil {
		vb = attrs.CropBox
	}
	if vb == nil {
		vb = types.RectForFormat("A4")
	}
	rot := ((attrs.Rotate % 360) + 360) % 360
	rot = int(math.Round(float64(rot)/90)) * 90 % 360
	g := &paginaGeo{d: d, ref: ref, vb: vb, rot: rot, attrs: attrs, dw: vb.Width(), dh: vb.Height()}
	if rot == 90 || rot == 270 {
		g.dw, g.dh = g.dh, g.dw
	}
	return g, nil
}

// versoUtente: dalle coordinate della pagina come si vede (punti, origine in basso a sinistra)
// alle coordinate vere della pagina.
func (g *paginaGeo) versoUtente() mat {
	wu, hu := g.vb.Width(), g.vb.Height()
	var d2u mat
	switch g.rot {
	case 90:
		d2u = mat{0, 1, -1, 0, wu, 0}
	case 180:
		d2u = mat{-1, 0, 0, -1, wu, hu}
	case 270:
		d2u = mat{0, -1, 1, 0, 0, hu}
	default:
		d2u = mat{1, 0, 0, 1, 0, 0}
	}
	return d2u.per(mat{1, 0, 0, 1, g.vb.LL.X, g.vb.LL.Y})
}

func (m mat) applica(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

func (m mat) cm() string {
	return fmt.Sprintf("%s %s %s %s %s %s cm", numPDF(m[0]), numPDF(m[1]), numPDF(m[2]), numPDF(m[3]), numPDF(m[4]), numPDF(m[5]))
}

// risorsePagina: le risorse della pagina in una copia solo sua, con i sottodizionari copiati.
func risorsePagina(ctx *model.Context, g *paginaGeo) types.Dict {
	var res types.Dict
	if o, ok := g.d.Find("Resources"); ok {
		res, _ = ctx.DereferenceDict(o)
	}
	if res == nil && g.attrs.Resources != nil {
		res = g.attrs.Resources
	}
	nuove := types.NewDict()
	for k, v := range res {
		nuove[k] = v
	}
	for _, sub := range []string{"XObject", "Font", "ExtGState"} {
		x := types.NewDict()
		if o, ok := nuove.Find(sub); ok {
			if old, err := ctx.DereferenceDict(o); err == nil {
				for k, v := range old {
					x[k] = v
				}
			}
		}
		nuove[sub] = x
	}
	g.d["Resources"] = nuove
	return nuove
}

// nomeLibero: un nome di risorsa non ancora usato (TP1, TP2...).
func nomeLibero(d types.Dict, pre string) string {
	for n := 1; ; n++ {
		s := pre + strconv.Itoa(n)
		if _, ok := d[s]; !ok {
			return s
		}
	}
}

// aggiungiContenuto: mette cs sopra (o sotto) il contenuto della pagina.
func aggiungiContenuto(ctx *model.Context, g *paginaGeo, cs string, sotto bool) error {
	var vecchio types.Array
	if o, ok := g.d.Find("Contents"); ok && o != nil {
		if o2, err := ctx.Dereference(o); err == nil {
			if a, isArr := o2.(types.Array); isArr {
				vecchio = append(vecchio, a...)
			} else if o2 != nil {
				vecchio = append(vecchio, o)
			}
		}
	}
	nuovo, err := nuovoStream(ctx, []byte("q\n"+cs+"\nQ\n"))
	if err != nil {
		return err
	}
	var arr types.Array
	if sotto {
		arr = append(arr, *nuovo)
		if len(vecchio) > 0 {
			arr = append(arr, vecchio...)
		}
	} else {
		q, err := nuovoStream(ctx, []byte("q\n"))
		if err != nil {
			return err
		}
		qq, err := nuovoStream(ctx, []byte("\nQ\n"))
		if err != nil {
			return err
		}
		arr = append(arr, *q)
		arr = append(arr, vecchio...)
		arr = append(arr, *qq, *nuovo)
	}
	g.d["Contents"] = arr
	return nil
}

var spaziTrattino = regexp.MustCompile(`\s*[-–]\s*`)

// pagineScelte: "" o "tutte" = tutte; altrimenti "1-3, 5, 8-" (le pagine oltre la fine si ignorano).
func pagineScelte(s string, n int) (map[int]bool, error) {
	out := map[int]bool{}
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "tutte" || s == "all" {
		for i := 1; i <= n; i++ {
			out[i] = true
		}
		return out, nil
	}
	// ZP05-2: "2 - 3", "2- 3" e "2 -3" vogliono dire "2-3": prima tolgo gli spazi intorno al trattino.
	s = spaziTrattino.ReplaceAllString(s, "-")
	for _, parte := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		da, a := parte, parte
		if parte == "-" {
			return nil, errc("intervallo", nil) // un trattino da solo non vuol dire "tutte"
		}
		if i := strings.IndexAny(parte, "-–"); i >= 0 {
			da, a = parte[:i], strings.TrimLeft(parte[i:], "-–")
			if da == "" {
				da = "1"
			}
			if a == "" {
				a = strconv.Itoa(n)
			}
		}
		x, err1 := strconv.Atoi(strings.TrimSpace(da))
		y, err2 := strconv.Atoi(strings.TrimSpace(a))
		if err1 != nil || err2 != nil || x < 1 || y < x {
			return nil, errc("intervallo", nil)
		}
		for i := x; i <= y && i <= n; i++ {
			out[i] = true
		}
	}
	if len(out) == 0 {
		return nil, errc("intervallo", nil)
	}
	return out, nil
}

// ---------------------------------------------------------------- testo nei caratteri standard

// winAnsi: il testo nella codifica dei caratteri standard di PDF (come Windows-1252).
// ok = false se ci sono lettere che quei caratteri non hanno (es. ł, ą del polacco).
func winAnsi(s string) ([]byte, bool) {
	speciali := map[rune]byte{
		'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89,
		'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
		'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
	}
	var b []byte
	ok := true
	for _, r := range s {
		switch {
		case r == '\t':
			b = append(b, ' ')
		case r >= 32 && r < 127, r >= 0xA0 && r <= 0xFF:
			b = append(b, byte(r))
		default:
			if c, esiste := speciali[r]; esiste {
				b = append(b, c)
			} else {
				b = append(b, '?')
				ok = false
			}
		}
	}
	return b, ok
}

// TestoAdatto: il testo si può scrivere con i caratteri standard (altrimenti la pagina lo
// mette come immagine).
func TestoAdatto(s string) bool { _, ok := winAnsi(s); return ok }

var nomiFont = map[string]string{
	"helv": "Helvetica", "helvb": "Helvetica-Bold", "times": "Times-Roman", "timesb": "Times-Bold",
	"cour": "Courier", "courb": "Courier-Bold",
}

// baseline: dove cade la prima riga (frazione della dimensione del carattere dall'alto),
// uguale all'anteprima nella pagina (righe alte 1,2 volte il carattere).
var baseline = map[string]float64{"helv": 0.95, "helvb": 0.95, "times": 0.94, "timesb": 0.94, "cour": 0.87, "courb": 0.87}

func coloreRGB(s string) (float64, float64, float64) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255
}

// ---------------------------------------------------------------- elementi da disegnare

// Elemento: testo, immagine o rettangolo da mettere sulle pagine.
// X, Y, L, A: frazioni della pagina come si vede, origine in alto a sinistra.
// Nel testo {p} diventa il numero della pagina e {n} il numero di pagine.
type Elemento struct {
	Tipo    string  `json:"tipo"`   // "testo", "immagine", "rett"
	Pagina  int     `json:"pagina"` // 1 = prima; 0 = vedi Pagine
	Pagine  string  `json:"pagine"` // con Pagina 0: "" = tutte, oppure "1-3,5"
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	L       float64 `json:"l"`
	A       float64 `json:"a"`
	Testo   string  `json:"testo"`
	Font    string  `json:"font"` // helv, helvb, times, timesb, cour, courb
	Dim     float64 `json:"dim"`  // punti
	Colore  string  `json:"colore"`
	Opacita float64 `json:"opacita"` // 0 o 1 = piena
	Rot     float64 `json:"rot"`     // gradi, in senso antiorario, attorno al centro
	Allinea string  `json:"allinea"` // "s", "c", "d"
	Sotto   bool    `json:"sotto"`   // dietro al contenuto della pagina
	Img     []byte  `json:"-"`
	// ZP05-3: per le immagini su pagine di misure diverse. Prop = altezza/larghezza dell'immagine
	// (resta uguale su ogni pagina); LPt = larghezza in punti (un testo diventato immagine resta
	// grande come il testo), messa secondo Allinea. 0 = come prima (frazioni della pagina).
	Prop float64 `json:"prop"`
	LPt  float64 `json:"lpt"`
}

type disegnatore struct {
	ctx      *model.Context
	font     map[string]*types.IndirectRef
	gstate   map[float64]*types.IndirectRef
	immagini map[int]*types.IndirectRef
}

func (dz *disegnatore) fontRef(chiave string) (*types.IndirectRef, error) {
	if r, ok := dz.font[chiave]; ok {
		return r, nil
	}
	d := types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"),
		"BaseFont": types.Name(nomiFont[chiave]), "Encoding": types.Name("WinAnsiEncoding")}
	r, err := dz.ctx.IndRefForNewObject(d)
	if err != nil {
		return nil, err
	}
	dz.font[chiave] = r
	return r, nil
}

func (dz *disegnatore) gsRef(a float64) (*types.IndirectRef, error) {
	a = math.Round(a*1000) / 1000
	if r, ok := dz.gstate[a]; ok {
		return r, nil
	}
	d := types.Dict{"Type": types.Name("ExtGState"), "ca": types.Float(a), "CA": types.Float(a)}
	r, err := dz.ctx.IndRefForNewObject(d)
	if err != nil {
		return nil, err
	}
	dz.gstate[a] = r
	return r, nil
}

// contenuto: le istruzioni per disegnare e sulla pagina p (di n).
func (dz *disegnatore) contenuto(g *paginaGeo, res types.Dict, i int, e Elemento, p, n int) (string, error) {
	x, w, h := e.X*g.dw, e.L*g.dw, e.A*g.dh
	y := g.dh - e.Y*g.dh - h // in basso a sinistra, y verso l'alto
	if e.Tipo == "immagine" && e.LPt > 0 {
		w2 := math.Min(e.LPt, g.dw)
		switch e.Allinea {
		case "c":
			x += (w - w2) / 2
		case "d":
			x += w - w2
		}
		w = w2
	}
	if e.Tipo == "immagine" && e.Prop > 0 {
		h2 := w * e.Prop
		y += (h - h2) / 2 // lo stesso centro
		h = h2
	}
	var cs strings.Builder
	cs.WriteString("q " + g.versoUtente().cm() + "\n")
	if e.Rot != 0 {
		r := e.Rot * math.Pi / 180
		cx, cy := x+w/2, y+h/2
		c, s := math.Cos(r), math.Sin(r)
		m := mat{1, 0, 0, 1, -cx, -cy}.per(mat{c, s, -s, c, 0, 0}).per(mat{1, 0, 0, 1, cx, cy})
		cs.WriteString(m.cm() + "\n")
	}
	if e.Opacita > 0 && e.Opacita < 1 {
		gs := res["ExtGState"].(types.Dict)
		ref, err := dz.gsRef(e.Opacita)
		if err != nil {
			return "", err
		}
		nome := nomeLibero(gs, "TPgs")
		gs[nome] = *ref
		cs.WriteString("/" + nome + " gs\n")
	}
	switch e.Tipo {
	case "rett":
		r, gg, b := coloreRGB(e.Colore)
		fmt.Fprintf(&cs, "%s %s %s rg %s %s %s %s re f\n", numPDF(r), numPDF(gg), numPDF(b), numPDF(x), numPDF(y), numPDF(w), numPDF(h))
	case "immagine":
		ref, ok := dz.immagini[i]
		if !ok {
			var err error
			ref, _, _, err = model.CreateImageResource(dz.ctx.XRefTable, bytes.NewReader(e.Img))
			if err != nil {
				return "", errc("immagine", err)
			}
			dz.immagini[i] = ref
		}
		xo := res["XObject"].(types.Dict)
		nome := nomeLibero(xo, "TPim")
		xo[nome] = *ref
		fmt.Fprintf(&cs, "%s 0 0 %s %s %s cm /%s Do\n", numPDF(w), numPDF(h), numPDF(x), numPDF(y), nome)
	case "testo":
		chiave := e.Font
		if _, ok := nomiFont[chiave]; !ok {
			chiave = "helv"
		}
		dim := e.Dim
		if dim <= 0 {
			dim = 12
		}
		ref, err := dz.fontRef(chiave)
		if err != nil {
			return "", err
		}
		fd := res["Font"].(types.Dict)
		nome := nomeLibero(fd, "TPf")
		fd[nome] = *ref
		r, gg, b := coloreRGB(e.Colore)
		testo := strings.ReplaceAll(strings.ReplaceAll(e.Testo, "{p}", strconv.Itoa(p)), "{n}", strconv.Itoa(n))
		righe := strings.Split(strings.ReplaceAll(testo, "\r", ""), "\n")
		fmt.Fprintf(&cs, "BT /%s %s Tf %s %s %s rg\n", nome, numPDF(dim), numPDF(r), numPDF(gg), numPDF(b))
		base := y + h - dim*baseline[chiave]
		for k, riga := range righe {
			enc, _ := winAnsi(riga)
			lw, _ := font.TextWidthFloat(bg, string(enc), nomiFont[chiave], dim)
			tx := x
			switch e.Allinea {
			case "c":
				tx = x + (w-lw)/2
			case "d":
				tx = x + w - lw
			}
			ty := base - float64(k)*dim*1.2
			fmt.Fprintf(&cs, "1 0 0 1 %s %s Tm <%X> Tj\n", numPDF(tx), numPDF(ty), enc)
		}
		cs.WriteString("ET\n")
	default:
		return "", errc("vuoto", nil)
	}
	cs.WriteString("Q\n")
	return cs.String(), nil
}

// Disegna: mette testi, immagini e rettangoli sulle pagine e restituisce il nuovo PDF.
func Disegna(data []byte, pw string, elementi []Elemento) ([]byte, error) {
	if len(elementi) == 0 {
		return nil, errc("vuoto", nil)
	}
	ctx, err := apriChiaro(data, pw)
	if err != nil {
		return nil, err
	}
	n := ctx.PageCount
	dove := make([]map[int]bool, len(elementi))
	for i, e := range elementi {
		if e.L <= 0 || e.A <= 0 {
			return nil, errc("vuoto", nil)
		}
		if e.Tipo == "immagine" && len(e.Img) == 0 {
			return nil, errc("immagine", nil)
		}
		if e.Pagina > 0 {
			if e.Pagina > n {
				return nil, errc("intervallo", nil)
			}
			dove[i] = map[int]bool{e.Pagina: true}
		} else if dove[i], err = pagineScelte(e.Pagine, n); err != nil {
			return nil, err
		}
	}
	dz := &disegnatore{ctx: ctx, font: map[string]*types.IndirectRef{}, gstate: map[float64]*types.IndirectRef{}, immagini: map[int]*types.IndirectRef{}}
	for p := 1; p <= n; p++ {
		var sopra, sotto strings.Builder
		var g *paginaGeo
		var res types.Dict
		for i, e := range elementi {
			if !dove[i][p] {
				continue
			}
			if g == nil {
				if g, err = geometria(ctx, p); err != nil {
					return nil, err
				}
				res = risorsePagina(ctx, g)
			}
			cs, err := dz.contenuto(g, res, i, e, p, n)
			if err != nil {
				return nil, err
			}
			if e.Sotto {
				sotto.WriteString(cs)
			} else {
				sopra.WriteString(cs)
			}
		}
		if g == nil {
			continue
		}
		if sotto.Len() > 0 {
			if err := aggiungiContenuto(ctx, g, sotto.String(), true); err != nil {
				return nil, err
			}
		}
		if sopra.Len() > 0 {
			if err := aggiungiContenuto(ctx, g, sopra.String(), false); err != nil {
				return nil, err
			}
		}
	}
	return scrivi(ctx)
}

// ---------------------------------------------------------------- collegamenti

// Collegamento: un'area cliccabile verso una pagina del documento o un indirizzo web.
type Collegamento struct {
	Pagina int     `json:"pagina"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	L      float64 `json:"l"`
	A      float64 `json:"a"`
	URL    string  `json:"url"`
	Verso  int     `json:"verso"` // pagina di destinazione (se non c'è URL)
}

// indirizzoValido: solo indirizzi web o di posta (niente file, niente programmi).
func indirizzoValido(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if !strings.Contains(s, ":") {
		if strings.Contains(s, "@") && !strings.Contains(s, "/") {
			s = "mailto:" + s
		} else {
			s = "https://" + s
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
	case "mailto":
	default:
		return "", false
	}
	return u.String(), true
}

// AggiungiCollegamenti: mette nel PDF i collegamenti (annotazioni /Link vere).
func AggiungiCollegamenti(data []byte, pw string, cc []Collegamento) ([]byte, error) {
	if len(cc) == 0 {
		return nil, errc("vuoto", nil)
	}
	ctx, err := apriChiaro(data, pw)
	if err != nil {
		return nil, err
	}
	for _, c := range cc {
		if c.Pagina < 1 || c.Pagina > ctx.PageCount || c.L <= 0 || c.A <= 0 {
			return nil, errc("intervallo", nil)
		}
		g, err := geometria(ctx, c.Pagina)
		if err != nil {
			return nil, err
		}
		m := g.versoUtente()
		x0, y0 := m.applica(c.X*g.dw, g.dh-c.Y*g.dh)
		x1, y1 := m.applica((c.X+c.L)*g.dw, g.dh-(c.Y+c.A)*g.dh)
		rect := types.Array{types.Float(math.Min(x0, x1)), types.Float(math.Min(y0, y1)), types.Float(math.Max(x0, x1)), types.Float(math.Max(y0, y1))}
		ann := types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name("Link"), "Rect": rect,
			"Border": types.Array{types.Integer(0), types.Integer(0), types.Integer(0)}, "F": types.Integer(4)}
		if c.URL != "" {
			u, ok := indirizzoValido(c.URL)
			if !ok {
				return nil, errc("link", nil)
			}
			ann["A"] = types.Dict{"S": types.Name("URI"), "URI": types.StringLiteral(escapeLetterale(u))}
		} else {
			if c.Verso < 1 || c.Verso > ctx.PageCount {
				return nil, errc("intervallo", nil)
			}
			_, dref, _, err := ctx.PageDict(bg, c.Verso, false)
			if err != nil || dref == nil {
				return nil, errc("rovinato", err)
			}
			ann["Dest"] = types.Array{*dref, types.Name("Fit")}
		}
		if g.ref != nil {
			ann["P"] = *g.ref
		}
		ref, err := ctx.IndRefForNewObject(ann)
		if err != nil {
			return nil, err
		}
		var annots types.Array
		if o, ok := g.d.Find("Annots"); ok {
			if a, err := ctx.DereferenceArray(o); err == nil {
				annots = append(annots, a...)
			}
		}
		g.d["Annots"] = append(annots, *ref)
	}
	return scrivi(ctx)
}

// escapeLetterale: una stringa (...) di PDF con le parentesi e le barre protette.
func escapeLetterale(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`, "\r", `\r`, "\n", `\n`)
	return r.Replace(s)
}

// ---------------------------------------------------------------- organizza le pagine

// PaginaSeq: una pagina del nuovo documento: la pagina Pagina del file Fonte (0 = il documento,
// 1.. = gli altri file), girata di Rot gradi in più; Fonte -1 = pagina vuota di W x H punti.
type PaginaSeq struct {
	Fonte  int     `json:"f"`
	Pagina int     `json:"p"`
	Rot    int     `json:"r"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
}

// paginaVuota: un PDF di una pagina bianca.
func paginaVuota(w, h float64) []byte {
	if w < 10 || h < 10 || w > 14400 || h > 14400 {
		w, h = 595.28, 841.89
	}
	var b bytes.Buffer
	var off []int
	obj := func(s string) {
		off = append(off, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(off), s)
	}
	b.WriteString("%PDF-1.7\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << >> /Contents 4 0 R >>", numPDF(w), numPDF(h)))
	obj("<< /Length 0 >>\nstream\n\nendstream")
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", len(off)+1)
	for _, o := range off {
		fmt.Fprintf(&b, "%010d 00000 n\r\n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(off)+1, x)
	return b.Bytes()
}

// Organizza: costruisce un nuovo PDF con le pagine nell'ordine e con le rotazioni di seq.
// fonti[0] è il documento; fonti[i] gli altri file (già senza password).
func Organizza(fonti [][]byte, seq []PaginaSeq) ([]byte, error) {
	if len(seq) == 0 {
		return nil, errc("nessuna_pagina", nil)
	}
	// Le fonti davvero usate, ognuna una volta sola, più una pagina vuota per ogni vuota.
	var parti [][]byte
	inizio := map[int]int{} // fonte -> numero (globale) della sua pagina 1, meno 1
	conte := map[int]int{}
	totale := 0
	var globali []int
	for _, s := range seq {
		if s.Fonte == -1 {
			b := paginaVuota(s.W, s.H)
			parti = append(parti, b)
			totale++
			globali = append(globali, totale)
			continue
		}
		if s.Fonte < 0 || s.Fonte >= len(fonti) {
			return nil, errc("intervallo", nil)
		}
		if _, ok := inizio[s.Fonte]; !ok {
			n, err := contaPagine(fonti[s.Fonte])
			if err != nil {
				return nil, err
			}
			inizio[s.Fonte] = totale
			conte[s.Fonte] = n
			parti = append(parti, fonti[s.Fonte])
			totale += n
		}
		if s.Pagina < 1 || s.Pagina > conte[s.Fonte] {
			return nil, errc("intervallo", nil)
		}
		globali = append(globali, inizio[s.Fonte]+s.Pagina)
	}
	var unito []byte
	if len(parti) == 1 {
		unito = parti[0]
	} else {
		u, err := unisciParti(parti)
		if err != nil {
			return nil, err
		}
		unito = u
	}
	ctx, _, err := leggi(unito, "")
	if err != nil {
		return nil, err
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, classifica(err)
	}
	// ZP05-1: le pagine si riordinano nello STESSO documento, così restano moduli, segnalibri,
	// allegati, lingua, etichette e titolo. Solo con una pagina ripetuta si passa dall'estrazione
	// di pdfcpu (che fa un documento nuovo con le sole pagine).
	ctxN := ctx
	if ripetute(globali) {
		ctxN, err = pdfcpu.ExtractPages(bg, ctx, globali, false)
		if err != nil {
			return nil, classifica(err)
		}
	} else if err := senzaPanico(func() error { return riordinaSulPosto(ctx, globali) }); err != nil {
		return nil, classifica(err)
	}
	if err := ctxN.EnsurePageCount(); err != nil {
		return nil, classifica(err)
	}
	perAngolo := map[int]types.IntSet{}
	for i, s := range seq {
		r := ((s.Rot % 360) + 360) % 360
		if r%90 != 0 {
			return nil, errc("intervallo", nil)
		}
		if r == 0 {
			continue
		}
		if perAngolo[r] == nil {
			perAngolo[r] = types.IntSet{}
		}
		perAngolo[r][i+1] = true
	}
	for r, set := range perAngolo {
		if err := pdfcpu.RotatePages(bg, ctxN, set, r); err != nil {
			return nil, classifica(err)
		}
	}
	return scrivi(ctxN)
}

func ripetute(pp []int) bool {
	visti := map[int]bool{}
	for _, p := range pp {
		if visti[p] {
			return true
		}
		visti[p] = true
	}
	return false
}

// paginaAlbero: una pagina dell'albero /Pages, con gli attributi ereditati dai nodi sopra.
type paginaAlbero struct {
	ref types.IndirectRef
	d   types.Dict
}

var chiaviEreditate = []string{"Resources", "MediaBox", "CropBox", "Rotate"}

// pagineInOrdine: le pagine del documento in ordine. Gli attributi che una pagina eredita dai
// nodi sopra (risorse, misure, rotazione) vengono scritti nella pagina stessa.
func pagineInOrdine(ctx *model.Context) ([]paginaAlbero, error) {
	var out []paginaAlbero
	visti := map[int]bool{}
	var giu func(o types.Object, ered types.Dict, prof int) error
	giu = func(o types.Object, ered types.Dict, prof int) error {
		ir, ok := o.(types.IndirectRef)
		if !ok || prof > 64 || visti[ir.ObjectNumber.Value()] {
			return errc("rovinato", nil)
		}
		visti[ir.ObjectNumber.Value()] = true
		d, err := ctx.DereferenceDict(ir)
		if err != nil || d == nil {
			return errc("rovinato", err)
		}
		if t := d.Type(); t != nil && *t == "Page" {
			for _, k := range chiaviEreditate {
				if _, c := d.Find(k); !c {
					if v, c2 := ered.Find(k); c2 {
						d[k] = v
					}
				}
			}
			out = append(out, paginaAlbero{ref: ir, d: d})
			return nil
		}
		e2 := types.Dict{}
		for k, v := range ered {
			e2[k] = v
		}
		for _, k := range chiaviEreditate {
			if v, c := d.Find(k); c {
				e2[k] = v
			}
		}
		kids, err := ctx.DereferenceArray(d["Kids"])
		if err != nil {
			return errc("rovinato", err)
		}
		for _, k := range kids {
			if err := giu(k, e2, prof+1); err != nil {
				return err
			}
		}
		return nil
	}
	r := ctx.RootDict.IndirectRefEntry("Pages")
	if r == nil {
		return nil, errc("rovinato", nil)
	}
	if err := giu(*r, types.Dict{}, 0); err != nil {
		return nil, err
	}
	return out, nil
}

// riordinaSulPosto: l'albero /Pages diventa la lista delle pagine scelte (numeri da 1, senza
// ripetute), nello stesso documento. I collegamenti alle pagine tolte (segnalibri, link, campi
// dei moduli, destinazioni con nome, struttura) si tolgono: così niente rimanda al loro contenuto
// e il contenuto delle pagine tolte non resta nascosto nella copia.
func riordinaSulPosto(ctx *model.Context, scelte []int) error {
	pp, err := pagineInOrdine(ctx)
	if err != nil {
		return err
	}
	radice := ctx.RootDict.IndirectRefEntry("Pages")
	nodo, err := ctx.DereferenceDict(*radice)
	if err != nil {
		return errc("rovinato", err)
	}
	tenute := map[int]bool{}
	kids := types.Array{}
	for _, n := range scelte {
		if n < 1 || n > len(pp) {
			return errc("intervallo", nil)
		}
		p := pp[n-1]
		p.d["Parent"] = *radice
		kids = append(kids, p.ref)
		tenute[p.ref.ObjectNumber.Value()] = true
	}
	nodo["Kids"] = kids
	nodo["Count"] = types.Integer(len(kids))
	ctx.PageCount = len(kids)
	// Oggetti "morti": le pagine tolte, le loro annotazioni (campi compresi) e le destinazioni
	// scritte come oggetto a sé che puntano a una pagina tolta.
	morti, pagineTolte := map[int]bool{}, map[int]bool{}
	var primaTenuta types.Object = kids[0]
	for _, p := range pp {
		if tenute[p.ref.ObjectNumber.Value()] {
			continue
		}
		morti[p.ref.ObjectNumber.Value()] = true
		pagineTolte[p.ref.ObjectNumber.Value()] = true
		if a, err := ctx.DereferenceArray(p.d["Annots"]); err == nil {
			for _, o := range a {
				if ir, ok := o.(types.IndirectRef); ok && !tenuteAnnot(ctx, ir, tenute) {
					morti[ir.ObjectNumber.Value()] = true
				}
			}
		}
	}
	if len(morti) == 0 {
		return nil
	}
	eMorto := func(o types.Object) bool {
		ir, ok := o.(types.IndirectRef)
		return ok && morti[ir.ObjectNumber.Value()]
	}
	// una destinazione [pagina /XYZ ...] verso una pagina tolta
	destMorta := func(o types.Object) bool {
		a, ok := o.(types.Array)
		if !ok || len(a) < 2 {
			return false
		}
		ir, ok := a[0].(types.IndirectRef)
		if !ok || !pagineTolte[ir.ObjectNumber.Value()] {
			return false
		}
		n, ok := a[1].(types.Name)
		return ok && (strings.HasPrefix(string(n), "Fit") || n == "XYZ")
	}
	for nr, e := range ctx.Table {
		if e == nil || e.Free || e.Object == nil || morti[nr] {
			continue
		}
		if destMorta(e.Object) {
			morti[nr] = true
		}
	}
	var pulisci func(o types.Object, prof int) types.Object
	pulisci = func(o types.Object, prof int) types.Object {
		if prof > 100 {
			return o
		}
		switch x := o.(type) {
		case types.Dict:
			for k, v := range x {
				if eMorto(v) || destMorta(v) {
					delete(x, k)
					continue
				}
				x[k] = pulisci(v, prof+1)
			}
			return x
		case types.StreamDict:
			pulisci(x.Dict, prof+1)
			return x
		case types.Array:
			out := x[:0]
			for _, v := range x {
				if eMorto(v) {
					continue
				}
				if destMorta(v) {
					v = types.Array{primaTenuta, types.Name("Fit")} // nell'elenco dei nomi la coppia resta
				}
				out = append(out, pulisci(v, prof+1))
			}
			return out
		}
		return o
	}
	for nr, e := range ctx.Table {
		if e == nil || e.Free || e.Object == nil || morti[nr] {
			continue
		}
		e.Object = pulisci(e.Object, 0)
	}
	potaSegnalibri(ctx)
	return nil
}

// potaSegnalibri: i segnalibri rimasti senza destinazione (puntavano a pagine tolte) si tolgono.
func potaSegnalibri(ctx *model.Context) {
	rad, err := ctx.DereferenceDict(ctx.RootDict["Outlines"])
	if err != nil || rad == nil {
		return
	}
	vuoto := func(d types.Dict) bool {
		if _, c := d.Find("First"); c {
			return false
		}
		if _, c := d.Find("Dest"); c {
			return false
		}
		a, err := ctx.DereferenceDict(d["A"])
		if err != nil || a == nil {
			return true
		}
		if n, ok := a["S"].(types.Name); ok && (n == "GoTo") {
			_, c := a.Find("D")
			return !c
		}
		return false
	}
	var pota func(padre types.Dict, prof int) int
	pota = func(padre types.Dict, prof int) int {
		if prof > 50 {
			return 0
		}
		var tenuti []types.IndirectRef
		cur := padre["First"]
		for i := 0; cur != nil && i < 100000; i++ {
			ir, ok := cur.(types.IndirectRef)
			if !ok {
				break
			}
			d, err := ctx.DereferenceDict(ir)
			if err != nil || d == nil {
				break
			}
			cur = d["Next"]
			figli := pota(d, prof+1)
			if vuoto(d) {
				continue
			}
			if c, ok := d["Count"].(types.Integer); ok && c < 0 {
				d["Count"] = types.Integer(-figli)
			} else if figli > 0 {
				d["Count"] = types.Integer(figli)
			}
			tenuti = append(tenuti, ir)
		}
		if len(tenuti) == 0 {
			delete(padre, "First")
			delete(padre, "Last")
			delete(padre, "Count")
			return 0
		}
		visibili := 0
		for i, ir := range tenuti {
			d, _ := ctx.DereferenceDict(ir)
			delete(d, "Prev")
			delete(d, "Next")
			if i > 0 {
				d["Prev"] = tenuti[i-1]
			}
			if i < len(tenuti)-1 {
				d["Next"] = tenuti[i+1]
			}
			visibili++
			if c, ok := d["Count"].(types.Integer); ok && c > 0 {
				visibili += int(c)
			}
		}
		padre["First"], padre["Last"] = tenuti[0], tenuti[len(tenuti)-1]
		return visibili
	}
	n := pota(rad, 0)
	if n > 0 {
		rad["Count"] = types.Integer(n)
	}
}

// tenuteAnnot: l'annotazione sta anche su una pagina tenuta (raro: la stessa annotazione su due pagine).
func tenuteAnnot(ctx *model.Context, ir types.IndirectRef, tenute map[int]bool) bool {
	d, err := ctx.DereferenceDict(ir)
	if err != nil || d == nil {
		return false
	}
	p, ok := d["P"].(types.IndirectRef)
	return ok && tenute[p.ObjectNumber.Value()]
}

func contaPagine(b []byte) (int, error) {
	ctx, _, err := leggi(b, "")
	if err != nil {
		return 0, err
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return 0, classifica(err)
	}
	return ctx.PageCount, nil
}

// ---------------------------------------------------------------- proteggi

// OpzProteggi: password di apertura e/o permessi (che chiedono la password dei permessi).
type OpzProteggi struct {
	Apertura string `json:"apertura"`
	Permessi string `json:"permessi"` // password del proprietario: serve per i blocchi
	Blocca   bool   `json:"blocca"`
	Stampa   bool   `json:"stampa"`
	Copia    bool   `json:"copia"`
	Modifica bool   `json:"modifica"`
}

// Proteggi: cifra il PDF con AES a 256 bit.
func Proteggi(data []byte, pw string, o OpzProteggi) ([]byte, error) {
	if o.Apertura == "" && !o.Blocca {
		return nil, errc("vuoto", nil)
	}
	if o.Blocca && o.Permessi == "" {
		return nil, errc("pw_permessi", nil)
	}
	if o.Blocca && o.Permessi == o.Apertura {
		return nil, errc("pw_uguali", nil)
	}
	plain, err := inChiaro(data, pw)
	if err != nil {
		return nil, err
	}
	proprietario := o.Apertura
	perm := model.PermissionsAll
	if o.Blocca {
		proprietario = o.Permessi
		// ZP06-5: la lettura per gli screen reader (bit 10) resta sempre permessa; la copia
		// normale resta vietata dal bit 5 se non si sceglie «Copia».
		perm = model.PermissionsNone | model.PermissionExtractRev3
		if o.Stampa {
			perm |= model.PermissionPrintRev2 | model.PermissionPrintRev3
		}
		if o.Copia {
			perm |= model.PermissionExtract | model.PermissionExtractRev3
		}
		if o.Modifica {
			perm |= model.PermissionModify | model.PermissionModAnnFillForm | model.PermissionFillRev3 | model.PermissionAssembleRev3
		}
	}
	conf := model.NewAESConfiguration(o.Apertura, proprietario, 256)
	conf.Offline = true
	conf.ValidationMode = model.ValidationRelaxed
	conf.Permissions = perm
	var out bytes.Buffer
	if err := api.Encrypt(bg, bytes.NewReader(plain), &out, conf); err != nil {
		return nil, classifica(err)
	}
	return out.Bytes(), nil
}

// ---------------------------------------------------------------- comprimi

// Comprimi: ottimizza il PDF (pdfcpu) e, se immagini è vero, rifà più leggere le foto grandi.
// Se il risultato non è più piccolo, restituisce il PDF di partenza (senza protezioni).
func Comprimi(data []byte, pw string, immagini bool) ([]byte, error) {
	plain, err := inChiaro(data, pw)
	if err != nil {
		return nil, err
	}
	ctx, riparato, err := leggi(plain, "")
	if err != nil {
		return nil, err
	}
	togliFiltriTesto(ctx) // ZP06-2: anche in «Leggera»
	if immagini {
		alleggerisciImmagini(ctx, 1600, 65)
	}
	comprimiFlussiNudi(ctx) // pagine e font salvati senza compressione: Flate, come qpdf
	if err := api.OptimizeContext(bg, ctx); err != nil {
		return nil, classifica(err)
	}
	ctx.WriteObjectStream = true
	ctx.WriteXRefStream = true
	out, err := scrivi(ctx)
	if err != nil {
		return nil, err
	}
	// Se la copia non pesa meno tengo l'originale, ma non se era rovinato (riparato in lettura):
	// in quel caso la copia riscritta è quella buona.
	if len(out) >= len(plain) && !riparato && finisceBene(plain) {
		return plain, nil
	}
	return out, nil
}

// finisceBene: il PDF finisce con l'indice ("startxref" e "%%EOF" nelle ultime righe), come
// un file intero. I file tagliati o senza indice li legge lo stesso, ma vanno riscritti.
func finisceBene(b []byte) bool {
	coda := b
	if len(coda) > 1024 {
		coda = coda[len(coda)-1024:]
	}
	return bytes.Contains(coda, []byte("startxref")) && bytes.Contains(coda, []byte("%%EOF"))
}

// togliFiltriTesto (ZP06-2): i filtri «di solo testo» (ASCII85, ASCIIHex) messi davanti agli
// altri in molti PDF (per esempio quelli fatti con ReportLab) pesano un quarto in più e non
// servono: li tolgo da tutti i flussi. Poi le foto si possono anche ricomprimere.
func togliFiltriTesto(ctx *model.Context) {
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok || len(sd.FilterPipeline) == 0 {
			continue
		}
		k := 0
		for k < len(sd.FilterPipeline) && (sd.FilterPipeline[k].Name == filter.ASCII85 || sd.FilterPipeline[k].Name == filter.ASCIIHex) {
			k++
		}
		if k == 0 {
			continue
		}
		raw := sd.Raw
		bene := true
		for _, f := range sd.FilterPipeline[:k] {
			fl, err := filter.NewFilter(f.Name, nil)
			if err != nil {
				bene = false
				break
			}
			r, err := fl.Decode(bytes.NewReader(raw))
			if err != nil {
				bene = false
				break
			}
			b, err := io.ReadAll(r)
			if err != nil {
				bene = false
				break
			}
			raw = b
		}
		if !bene {
			continue
		}
		resto := sd.FilterPipeline[k:]
		d := sd.Dict
		delete(d, "Filter")
		delete(d, "DecodeParms")
		switch len(resto) {
		case 0:
		case 1:
			d["Filter"] = types.Name(resto[0].Name)
			if resto[0].DecodeParms != nil {
				d["DecodeParms"] = resto[0].DecodeParms
			}
		default:
			ff, pp, conParms := types.Array{}, types.Array{}, false
			for _, f := range resto {
				ff = append(ff, types.Name(f.Name))
				if f.DecodeParms != nil {
					pp = append(pp, f.DecodeParms)
					conParms = true
				} else {
					pp = append(pp, nil)
				}
			}
			d["Filter"] = ff
			if conParms {
				d["DecodeParms"] = pp
			}
		}
		n := int64(len(raw))
		d["Length"] = types.Integer(n)
		sd.Raw = raw
		sd.FilterPipeline = resto
		sd.StreamLength = &n
		sd.StreamLengthObjNr = nil
		sd.Content = nil
		e.Object = sd
	}
}

// comprimiFlussiNudi (PRESTAZIONI-ZIP-PDF, P1): i flussi salvati senza nessun filtro (il testo
// delle pagine, i font, le immagini in chiaro di alcuni programmi) vengono compressi con Flate,
// senza perdita, come fa qpdf --compress-streams. pdfcpu da solo non lo fa. Restano come sono:
// i metadati XMP (/Type /Metadata, che alcuni programmi leggono senza decodificare), i flussi
// con DecodeParms senza filtro (strani: meglio non toccarli) e quelli che non si accorciano.
func comprimiFlussiNudi(ctx *model.Context) int {
	fatti := 0
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok || len(sd.FilterPipeline) > 0 || len(sd.Raw) < 64 {
			continue
		}
		d := sd.Dict
		if _, c := d["Filter"]; c {
			continue
		}
		if _, c := d["DecodeParms"]; c {
			continue
		}
		if t := d.Type(); t != nil && (*t == "Metadata" || *t == "XRef" || *t == "ObjStm") {
			continue
		}
		var z bytes.Buffer
		w, err := zlib.NewWriterLevel(&z, zlib.BestCompression)
		if err != nil {
			continue
		}
		if _, err := w.Write(sd.Raw); err != nil || w.Close() != nil || z.Len() >= len(sd.Raw) {
			continue
		}
		raw := z.Bytes()
		n := int64(len(raw))
		d["Filter"] = types.Name(filter.Flate)
		d["Length"] = types.Integer(n)
		sd.Raw = raw
		sd.FilterPipeline = []types.PDFFilter{{Name: filter.Flate}}
		sd.StreamLength = &n
		sd.StreamLengthObjNr = nil
		sd.Content = nil
		e.Object = sd
		fatti++
	}
	return fatti
}

// alleggerisciImmagini: le foto (RGB o grigi, 8 bit) più grandi di maxLato pixel vengono
// rimpicciolite, e tutte riscritte in JPEG con la qualità data, se così pesano meno.
func alleggerisciImmagini(ctx *model.Context, maxLato, qualita int) int {
	fatte := 0
	var numeri []int
	for n := range ctx.XRefTable.Table {
		numeri = append(numeri, n)
	}
	sort.Ints(numeri)
	for _, n := range numeri {
		e := ctx.XRefTable.Table[n]
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok || sd.Type() == nil && sd.Subtype() == nil {
			continue
		}
		if st := sd.Subtype(); st == nil || *st != "Image" {
			continue
		}
		nuovo, ok := immagineLeggera(ctx, sd, maxLato, qualita)
		if !ok {
			continue
		}
		e.Object = *nuovo
		fatte++
	}
	return fatte
}

func immagineLeggera(ctx *model.Context, sd types.StreamDict, maxLato, qualita int) (*types.StreamDict, bool) {
	d := sd.Dict
	if v, _ := d.Find("ImageMask"); v != nil {
		if b, ok := v.(types.Boolean); ok && bool(b) {
			return nil, false
		}
	}
	for _, k := range []string{"Decode", "Mask", "Alternates", "OPI"} {
		if _, ok := d.Find(k); ok {
			return nil, false
		}
	}
	w, h := d.IntEntry("Width"), d.IntEntry("Height")
	bpc := d.IntEntry("BitsPerComponent")
	if w == nil || h == nil || *w < 16 || *h < 16 || int64(*w)*int64(*h) > 60_000_000 {
		return nil, false
	}
	comp := componentiSpazio(ctx, d)
	if comp != 1 && comp != 3 {
		return nil, false
	}
	filtri := sd.FilterPipeline
	var img image.Image
	switch {
	case len(filtri) == 1 && filtri[0].Name == "DCTDecode":
		i, err := jpeg.Decode(bytes.NewReader(sd.Raw))
		if err != nil {
			return nil, false
		}
		switch i.(type) {
		case *image.CMYK:
			return nil, false
		}
		img = i
	case len(filtri) <= 1 && (len(filtri) == 0 || filtri[0].Name == "FlateDecode"):
		if bpc == nil || *bpc != 8 {
			return nil, false
		}
		if err := sd.Decode(); err != nil || len(sd.Content) < *w**h*comp {
			return nil, false
		}
		px := sd.Content
		if comp == 1 {
			g := image.NewGray(image.Rect(0, 0, *w, *h))
			copy(g.Pix, px[:*w**h])
			img = g
		} else {
			rgba := image.NewRGBA(image.Rect(0, 0, *w, *h))
			for i, j := 0, 0; i < *w**h*3; i, j = i+3, j+4 {
				rgba.Pix[j], rgba.Pix[j+1], rgba.Pix[j+2], rgba.Pix[j+3] = px[i], px[i+1], px[i+2], 255
			}
			img = rgba
		}
	default:
		return nil, false
	}
	b := img.Bounds()
	nw, nh := b.Dx(), b.Dy()
	if lato := max(nw, nh); lato > maxLato {
		f := float64(maxLato) / float64(lato)
		nw, nh = max(1, int(float64(nw)*f+0.5)), max(1, int(float64(nh)*f+0.5))
		img = rimpicciolisci(img, nw, nh, comp == 1)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: qualita}); err != nil {
		return nil, false
	}
	vecchio := len(sd.Raw)
	if sd.StreamLength != nil && *sd.StreamLength > 0 {
		vecchio = int(*sd.StreamLength)
	}
	if buf.Len() >= vecchio*9/10 {
		return nil, false
	}
	nd := types.NewDict()
	for k, v := range d {
		switch k {
		case "Filter", "DecodeParms", "Length", "Width", "Height", "BitsPerComponent":
			continue
		}
		nd[k] = v
	}
	nd["Width"] = types.Integer(nw)
	nd["Height"] = types.Integer(nh)
	nd["BitsPerComponent"] = types.Integer(8)
	nd["Filter"] = types.Name("DCTDecode")
	l := int64(buf.Len())
	nd["Length"] = types.Integer(l)
	nuovo := types.StreamDict{Dict: nd, Raw: buf.Bytes(), StreamLength: &l, FilterPipeline: []types.PDFFilter{{Name: "DCTDecode"}}}
	return &nuovo, true
}

// componentiSpazio: quanti colori ha lo spazio colore dell'immagine (0 = non so / non adatto).
func componentiSpazio(ctx *model.Context, d types.Dict) int {
	o, ok := d.Find("ColorSpace")
	if !ok {
		return 0
	}
	o, err := ctx.Dereference(o)
	if err != nil {
		return 0
	}
	switch cs := o.(type) {
	case types.Name:
		switch cs {
		case "DeviceRGB", "CalRGB":
			return 3
		case "DeviceGray", "CalGray":
			return 1
		}
	case types.Array:
		if len(cs) == 2 {
			if nome, ok := cs[0].(types.Name); ok && nome == "ICCBased" {
				if s, _, err := ctx.DereferenceStreamDict(cs[1]); err == nil && s != nil {
					if n := s.IntEntry("N"); n != nil {
						return *n
					}
				}
			}
			if nome, ok := cs[0].(types.Name); ok && nome == "CalRGB" {
				return 3
			}
		}
	}
	return 0
}

// rimpicciolisci: media dei pixel (area), semplice e senza dipendenze.
func rimpicciolisci(src image.Image, nw, nh int, grigio bool) image.Image {
	b := src.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, src, b.Min, draw.Src)
	sw, sh := b.Dx(), b.Dy()
	var dst draw.Image
	if grigio {
		dst = image.NewGray(image.Rect(0, 0, nw, nh))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, nw, nh))
	}
	for y := 0; y < nh; y++ {
		y0, y1 := y*sh/nh, max((y+1)*sh/nh, y*sh/nh+1)
		for x := 0; x < nw; x++ {
			x0, x1 := x*sw/nw, max((x+1)*sw/nw, x*sw/nw+1)
			var r, g, bb, n int
			for yy := y0; yy < y1; yy++ {
				i := yy*rgba.Stride + x0*4
				for xx := x0; xx < x1; xx++ {
					r += int(rgba.Pix[i])
					g += int(rgba.Pix[i+1])
					bb += int(rgba.Pix[i+2])
					i += 4
					n++
				}
			}
			if gr, ok := dst.(*image.Gray); ok {
				gr.Pix[y*gr.Stride+x] = uint8(r / n)
			} else {
				d := dst.(*image.RGBA)
				j := y*d.Stride + x*4
				d.Pix[j], d.Pix[j+1], d.Pix[j+2], d.Pix[j+3] = uint8(r/n), uint8(g/n), uint8(bb/n), 255
			}
		}
	}
	return dst
}

// ---------------------------------------------------------------- oscura (vera)

// PaginaOscurata: l'immagine di una pagina (già con i rettangoli neri), fatta dalla pagina
// del programma; prende il posto di TUTTO il contenuto di quella pagina.
type PaginaOscurata struct {
	Pagina int    `json:"pagina"`
	Img    []byte `json:"-"`
}

// Oscura: le pagine indicate diventano solo un'immagine: il testo, le immagini, i campi dei
// moduli e le note che c'erano sotto spariscono davvero dal file.
func Oscura(data []byte, pw string, pagine []PaginaOscurata) ([]byte, error) {
	if len(pagine) == 0 {
		return nil, errc("vuoto", nil)
	}
	ctx, err := apriChiaro(data, pw)
	if err != nil {
		return nil, err
	}
	tolte := map[types.IndirectRef]bool{}
	for _, po := range pagine {
		if po.Pagina < 1 || po.Pagina > ctx.PageCount || len(po.Img) == 0 {
			return nil, errc("intervallo", nil)
		}
		g, err := geometria(ctx, po.Pagina)
		if err != nil {
			return nil, err
		}
		ref, _, _, err := model.CreateImageResource(ctx.XRefTable, bytes.NewReader(po.Img))
		if err != nil {
			return nil, errc("immagine", err)
		}
		if o, ok := g.d.Find("Annots"); ok {
			if a, err := ctx.DereferenceArray(o); err == nil {
				for _, x := range a {
					if r, ok := x.(types.IndirectRef); ok {
						tolte[r] = true
					}
				}
			}
		}
		for _, k := range []string{"Annots", "CropBox", "BleedBox", "TrimBox", "ArtBox", "Thumb", "PieceInfo", "B", "StructParents", "AA", "Group"} {
			g.d.Delete(k)
		}
		g.d["MediaBox"] = types.Array{types.Integer(0), types.Integer(0), types.Float(g.dw), types.Float(g.dh)}
		g.d["Rotate"] = types.Integer(0)
		g.d["Resources"] = types.Dict{"XObject": types.Dict{"TPoscurata": *ref}}
		cs := fmt.Sprintf("q %s 0 0 %s 0 0 cm /TPoscurata Do Q\n", numPDF(g.dw), numPDF(g.dh))
		s, err := nuovoStream(ctx, []byte(cs))
		if err != nil {
			return nil, err
		}
		g.d["Contents"] = *s
	}
	togliCampi(ctx, tolte)
	return scrivi(ctx)
}

// togliCampi: toglie dal modulo i campi che stavano sulle pagine oscurate (i loro valori
// resterebbero nel file anche senza la pagina).
func togliCampi(ctx *model.Context, tolte map[types.IndirectRef]bool) {
	if len(tolte) == 0 {
		return
	}
	cat, err := ctx.Catalog()
	if err != nil {
		return
	}
	o, ok := cat.Find("AcroForm")
	if !ok {
		return
	}
	af, err := ctx.DereferenceDict(o)
	if err != nil || af == nil {
		return
	}
	fo, ok := af.Find("Fields")
	if !ok {
		return
	}
	campi, err := ctx.DereferenceArray(fo)
	if err != nil {
		return
	}
	var tutteTolte func(x types.Object) bool
	tutteTolte = func(x types.Object) bool {
		r, ok := x.(types.IndirectRef)
		if ok && tolte[r] {
			return true
		}
		d, err := ctx.DereferenceDict(x)
		if err != nil || d == nil {
			return false
		}
		ko, ok := d.Find("Kids")
		if !ok {
			return false
		}
		kids, err := ctx.DereferenceArray(ko)
		if err != nil || len(kids) == 0 {
			return false
		}
		var restano types.Array
		for _, k := range kids {
			if !tutteTolte(k) {
				restano = append(restano, k)
			}
		}
		if len(restano) == 0 {
			return true
		}
		d["Kids"] = restano
		return false
	}
	var nuovi types.Array
	for _, c := range campi {
		if !tutteTolte(c) {
			nuovi = append(nuovi, c)
		}
	}
	if len(nuovi) == 0 {
		cat.Delete("AcroForm")
		return
	}
	af["Fields"] = nuovi
}
