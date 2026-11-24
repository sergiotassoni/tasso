// Tasso PDF - apri, unisci e firma i tuoi PDF.
// Copyright (C) 2026 Sergio Tassoni
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU General Public License as published by the Free Software Foundation, either version 3
// of the License, or (at your option) any later version. See LICENSE.

package main

// pdfops.go: tutto il lavoro sui PDF (riparare, sbloccare, unire, mettere la firma).
// Non dipende da Windows: i test girano anche su Linux.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func init() {
	// pdfcpu non deve creare cartelle o file di configurazione sul PC di chi usa il programma.
	model.ConfigPath = "disable"
}

// ErrCodice: un errore che la pagina sa spiegare con parole semplici (nella lingua scelta).
type ErrCodice struct {
	Codice string // es. "password", "password_sbagliata", "non_pdf", "rovinato"
	Causa  error
}

func (e *ErrCodice) Error() string {
	if e.Causa != nil {
		return e.Codice + ": " + e.Causa.Error()
	}
	return e.Codice
}

func (e *ErrCodice) Unwrap() error { return e.Causa }

func errc(codice string, causa error) error { return &ErrCodice{Codice: codice, Causa: causa} }

// CodiceErrore: il codice da mandare alla pagina ("" se l'errore non ne ha uno).
func CodiceErrore(err error) string {
	var e *ErrCodice
	if errors.As(err, &e) {
		return e.Codice
	}
	return ""
}

// confBase: configurazione di pdfcpu senza file su disco, senza internet e tollerante con i
// PDF un po' storti (quelli veri, fatti da tanti programmi diversi, spesso lo sono).
func confBase(pw string) *model.Configuration {
	c := model.NewStatelessConfiguration()
	c.Offline = true
	c.ValidateLinks = false
	c.ValidationMode = model.ValidationRelaxed
	c.UserPW = pw
	c.OwnerPW = pw
	return c
}

// classifica: trasforma un errore di pdfcpu in un codice comprensibile.
func classifica(err error) error {
	if err == nil {
		return nil
	}
	if CodiceErrore(err) != "" {
		return err
	}
	if errors.Is(err, pdfcpu.ErrWrongPassword) {
		return errc("password", err)
	}
	if errors.Is(err, pdfcpu.ErrOwnerPasswordRequired) || errors.Is(err, pdfcpu.ErrPermissionDenied) {
		return errc("password_proprietario", err)
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "password"):
		return errc("password", err)
	case strings.Contains(s, "no pdf") || strings.Contains(s, "not a pdf") || strings.Contains(s, "missing header") ||
		strings.Contains(s, "version"):
		return errc("non_pdf", err)
	}
	return errc("rovinato", err)
}

// sembraPDF: c'è l'intestazione %PDF- nei primi 1024 byte (come chiede la specifica).
func sembraPDF(b []byte) bool {
	n := len(b)
	if n > 1024 {
		n = 1024
	}
	return bytes.Contains(b[:n], []byte("%PDF-"))
}

// InfoDoc: le cose che la pagina deve sapere di un PDF prima di lavorarci.
type InfoDoc struct {
	Pagine         int  `json:"pagine"`
	Cifrato        bool `json:"cifrato"`        // ha una protezione (password o blocchi)
	ServePassword  bool `json:"servePassword"`  // per aprirlo serve una password
	Bloccato       bool `json:"bloccato"`       // ha blocchi (stampa, copia, modifica...)
	Riparato       bool `json:"riparato"`       // era rovinato ed è stato letto con la riparazione
	PasswordGiusta bool `json:"passwordGiusta"` // la password data apre il file
}

// leggi: legge il PDF (anche rovinato). Se pdfcpu non ci riesce, o legge un file senza pagine
// utilizzabili, prova a ricostruire l'indice degli oggetti. rip = è servita la riparazione.
func leggi(data []byte, pw string) (*model.Context, bool, error) {
	if !sembraPDF(data) {
		return nil, false, errc("non_pdf", nil)
	}
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(data), confBase(pw))
	if err == nil {
		if errP := controllaPagine(ctx); errP == nil {
			return ctx, ctx.Read != nil && ctx.Read.RepairOffset != 0, nil
		} else {
			err = errP
		}
	}
	if c := classifica(err); CodiceErrore(c) == "password" || CodiceErrore(c) == "password_proprietario" {
		return nil, false, c
	}
	// Seconda prova: ricostruisco l'indice degli oggetti leggendo il file da capo a fondo.
	// Terza prova: ricostruisco anche l'elenco delle pagine, con le sole pagine rimaste.
	for _, nuovoAlbero := range []bool{false, true} {
		ric, errR := ricostruisci(data, nuovoAlbero)
		if errR != nil {
			continue
		}
		ctx2, err2 := api.ReadContext(context.Background(), bytes.NewReader(ric), confBase(pw))
		if err2 != nil {
			if c := classifica(err2); CodiceErrore(c) == "password" {
				return nil, false, c
			}
			continue
		}
		if errP := controllaPagine(ctx2); errP != nil {
			continue
		}
		return ctx2, true, nil
	}
	return nil, false, classifica(err)
}

// controllaPagine: il file ha un catalogo e almeno una pagina; le pagine con contenuti
// mancanti o rovinati (tipico dei file scaricati a metà) diventano pagine con quello che resta.
func controllaPagine(ctx *model.Context) error {
	if ctx.Encrypt != nil && ctx.EncKey == nil {
		return errc("password", nil)
	}
	if _, err := ctx.Catalog(); err != nil {
		return err
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return err
	}
	if ctx.PageCount < 1 {
		return errors.New("nessuna pagina")
	}
	bg := context.Background()
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(bg, p, false)
		if err != nil {
			return err
		}
		if d == nil {
			return errors.New("pagina mancante")
		}
		sistemaContenuti(ctx, d)
	}
	return nil
}

// sistemaContenuti: tiene solo i pezzi di contenuto della pagina che sono davvero stream.
func sistemaContenuti(ctx *model.Context, d types.Dict) {
	o, ok := d.Find("Contents")
	if !ok || o == nil {
		return
	}
	valido := func(x types.Object) bool {
		y, err := ctx.Dereference(x)
		if err != nil || y == nil {
			return false
		}
		_, isStream := y.(types.StreamDict)
		return isStream
	}
	v, err := ctx.Dereference(o)
	if err != nil || v == nil {
		d.Delete("Contents")
		return
	}
	switch t := v.(type) {
	case types.StreamDict:
		return
	case types.Array:
		var buoni types.Array
		for _, x := range t {
			if valido(x) {
				buoni = append(buoni, x)
			}
		}
		if len(buoni) == len(t) {
			return
		}
		if len(buoni) == 0 {
			d.Delete("Contents")
		} else {
			d["Contents"] = buoni
		}
	default:
		d.Delete("Contents")
	}
}

// Info: pagine e protezioni di un PDF.
func Info(data []byte, pw string) (InfoDoc, error) {
	var inf InfoDoc
	ctx, rip, err := leggi(data, pw)
	if err != nil {
		if CodiceErrore(err) == "password" {
			inf.Cifrato, inf.ServePassword = true, true
			return inf, nil
		}
		return inf, err
	}
	inf.Riparato = rip
	inf.Cifrato = ctx.Encrypt != nil
	inf.PasswordGiusta = true
	if ctx.E != nil {
		// Bit 3 stampa, 4 modifica, 5 copia, 6 note, 9 moduli, 10 accessibilità, 11 assemblaggio, 12 stampa buona.
		const tutti = 4 | 8 | 16 | 32 | 256 | 512 | 1024 | 2048
		inf.Bloccato = ctx.E.P&tutti != tutti
	}
	if err := ctx.EnsurePageCount(); err == nil {
		inf.Pagine = ctx.PageCount
	}
	if inf.Pagine == 0 {
		_ = api.ValidateContext(context.Background(), ctx)
		inf.Pagine = ctx.PageCount
	}
	return inf, nil
}

// scrivi: salva il contesto in un nuovo PDF, senza protezioni.
func scrivi(ctx *model.Context) ([]byte, error) {
	ctx.Encrypt = nil
	ctx.E = nil
	ctx.EncKey = nil
	ctx.UserPW, ctx.OwnerPW = "", ""
	ctx.Cmd = model.DECRYPT
	var out bytes.Buffer
	if err := api.WriteContext(context.Background(), ctx, &out); err != nil {
		return nil, classifica(err)
	}
	return out.Bytes(), nil
}

// inChiaro: il PDF senza protezioni (se non ne ha, così com'è). Serve prima di unire o firmare.
func inChiaro(data []byte, pw string) ([]byte, error) {
	ctx, rip, err := leggi(data, pw)
	if err != nil {
		return nil, err
	}
	if ctx.Encrypt == nil && !rip {
		return data, nil
	}
	return scrivi(ctx)
}

// Sblocca: toglie blocchi e password da un PDF dell'utente e restituisce una copia libera.
// La password, se serve, la dà l'utente: il programma non prova mai a indovinarla.
func Sblocca(data []byte, pw string) ([]byte, error) {
	ctx, _, err := leggi(data, pw)
	if err != nil {
		return nil, err
	}
	if ctx.Encrypt == nil {
		return nil, errc("non_protetto", nil)
	}
	return scrivi(ctx)
}

// Ripara: legge il PDF con la massima tolleranza (ricostruendo l'indice se serve) e lo riscrive pulito.
func Ripara(data []byte, pw string) ([]byte, error) {
	ctx, _, err := leggi(data, pw)
	if err != nil {
		return nil, err
	}
	// Il controllo completo può fallire su un file rovinato: provo, ma salvo comunque quello che c'è.
	_ = api.ValidateContext(context.Background(), ctx)
	if err := ctx.EnsurePageCount(); err != nil || ctx.PageCount == 0 {
		return nil, errc("rovinato", err)
	}
	out, err := scrivi(ctx)
	if err != nil {
		return nil, err
	}
	// Controllo finale: il file riscritto si deve poter rileggere.
	if _, err := api.ReadContext(context.Background(), bytes.NewReader(out), confBase("")); err != nil {
		return nil, errc("rovinato", err)
	}
	return out, nil
}

var reObj = regexp.MustCompile(`(?m)(?:^|[\r\n\s])(\d{1,10})\s+(\d{1,5})\s+obj\b`)
var reCatalog = regexp.MustCompile(`/Type\s*/Catalog\b`)
var rePagina = regexp.MustCompile(`/Type\s*/Page\b`)
var rePagesRef = regexp.MustCompile(`/Pages\s+\d+\s+\d+\s+R`)

// ricostruisci: per i PDF scaricati a metà o con l'indice rotto. Cerca tutti gli oggetti
// "N G obj ... endobj" nel file, toglie un eventuale oggetto finale tagliato a metà, e scrive in
// fondo un indice (xref) e un trailer nuovi che puntano al catalogo trovato.
// Con nuovoAlbero scrive anche un nuovo elenco delle pagine con le sole pagine rimaste
// (quando l'elenco originale punta a pagine che nel file tagliato non ci sono più).
func ricostruisci(data []byte, nuovoAlbero bool) ([]byte, error) {
	type pos struct {
		off, gen    int
		inizioCorpo int
		fineCorpo   int
	}
	trovati := map[int]pos{}
	var ordinePagine []int
	root := 0
	taglio := len(data)
	for _, m := range reObj.FindAllSubmatchIndex(data, -1) {
		num, err1 := strconv.Atoi(string(data[m[2]:m[3]]))
		gen, err2 := strconv.Atoi(string(data[m[4]:m[5]]))
		if err1 != nil || err2 != nil || num <= 0 {
			continue
		}
		fine := bytes.Index(data[m[1]:], []byte("endobj"))
		if fine < 0 {
			// oggetto tagliato a metà: il file buono finisce qui
			if m[2] < taglio {
				taglio = m[2]
			}
			break
		}
		corpo := data[m[1] : m[1]+fine]
		trovati[num] = pos{off: m[2], gen: gen, inizioCorpo: m[1], fineCorpo: m[1] + fine}
		if reCatalog.Match(corpo) {
			root = num
		}
		if rePagina.Match(corpo) {
			ordinePagine = append(ordinePagine, num)
		}
	}
	if len(trovati) == 0 || root == 0 {
		return nil, errors.New("nessun catalogo trovato")
	}
	max := 0
	for n, p := range trovati {
		if p.off >= taglio {
			delete(trovati, n)
			continue
		}
		if n > max {
			max = n
		}
	}
	var b bytes.Buffer
	b.Write(data[:taglio])
	if taglio > 0 && data[taglio-1] != '\n' {
		b.WriteByte('\n')
	}
	if nuovoAlbero {
		var kids []string
		visto := map[int]bool{}
		for _, n := range ordinePagine {
			if _, ok := trovati[n]; ok && !visto[n] {
				visto[n] = true
				kids = append(kids, fmt.Sprintf("%d %d R", n, trovati[n].gen))
			}
		}
		if len(kids) == 0 {
			return nil, errors.New("nessuna pagina trovata")
		}
		pagesNum, catNum := max+1, max+2
		trovati[pagesNum] = pos{off: b.Len()}
		fmt.Fprintf(&b, "%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 595.28 841.89] >>\nendobj\n",
			pagesNum, strings.Join(kids, " "), len(kids))
		rc := trovati[root]
		cat := string(data[rc.inizioCorpo:rc.fineCorpo])
		nuovoRif := fmt.Sprintf("/Pages %d 0 R", pagesNum)
		if rePagesRef.MatchString(cat) {
			cat = rePagesRef.ReplaceAllLiteralString(cat, nuovoRif)
		} else {
			cat = "<< /Type /Catalog " + nuovoRif + " >>"
		}
		trovati[catNum] = pos{off: b.Len()}
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", catNum, strings.TrimSpace(cat))
		root, max = catNum, catNum
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", max+1)
	for i := 1; i <= max; i++ {
		if p, ok := trovati[i]; ok {
			fmt.Fprintf(&b, "%010d %05d n\r\n", p.off, p.gen)
		} else {
			b.WriteString("0000000000 65535 f\r\n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", max+1, root, xref)
	return b.Bytes(), nil
}

// ---------------------------------------------------------------- Rotazione

// ruota: gira di gradi (multipli di 90) tutte le pagine.
func ruota(data []byte, gradi int) ([]byte, error) {
	gradi = ((gradi % 360) + 360) % 360
	if gradi == 0 {
		return data, nil
	}
	ctx, _, err := leggi(data, "")
	if err != nil {
		return nil, err
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, classifica(err)
	}
	tutte := types.IntSet{}
	for i := 1; i <= ctx.PageCount; i++ {
		tutte[i] = true
	}
	if err := pdfcpu.RotatePages(context.Background(), ctx, tutte, gradi); err != nil {
		return nil, classifica(err)
	}
	return scrivi(ctx)
}

// ---------------------------------------------------------------- Unisci

// Pezzo: un file da unire (PDF o foto), con la sua rotazione in gradi.
type Pezzo struct {
	Nome      string
	Dati      []byte
	Foto      bool
	Rotazione int
	Password  string
}

// ErrPezzo: errore su uno dei file da unire (Indice parte da 0).
type ErrPezzo struct {
	Indice int
	Err    error
}

func (e *ErrPezzo) Error() string { return fmt.Sprintf("file %d: %v", e.Indice+1, e.Err) }
func (e *ErrPezzo) Unwrap() error { return e.Err }

// Unisci: mette insieme PDF e foto, nell'ordine dato, in un PDF solo.
func Unisci(pezzi []Pezzo) ([]byte, error) {
	if len(pezzi) == 0 {
		return nil, errc("vuoto", nil)
	}
	var parti [][]byte
	for i, p := range pezzi {
		var b []byte
		var err error
		if p.Foto {
			b, err = FotoInPDF(p.Dati)
		} else {
			b, err = inChiaro(p.Dati, p.Password)
		}
		if err == nil {
			b, err = ruota(b, p.Rotazione)
		}
		if err != nil {
			return nil, &ErrPezzo{Indice: i, Err: err}
		}
		parti = append(parti, b)
	}
	if len(parti) == 1 {
		return Ripara(parti[0], "")
	}
	var rss []io.ReadSeeker
	for _, b := range parti {
		rss = append(rss, bytes.NewReader(b))
	}
	var out bytes.Buffer
	conf := confBase("")
	conf.CreateBookmarks = false
	if err := api.MergeRaw(context.Background(), rss, &out, false, conf); err != nil {
		return nil, classifica(err)
	}
	return out.Bytes(), nil
}

// ---------------------------------------------------------------- Firma (timbri sulla pagina)

// Timbro: un'immagine PNG (firma, data, testo, spunta) da mettere su una pagina.
// X, Y, L, A sono frazioni (0..1) della pagina COME SI VEDE (già girata), con l'origine in alto a sinistra.
type Timbro struct {
	Pagina int     `json:"pagina"` // 1 = prima pagina; 0 = tutte le pagine
	PNG    []byte  `json:"-"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	L      float64 `json:"l"`
	A      float64 `json:"a"`
}

// matrice 2D di PDF: [a b c d e f]
type mat [6]float64

func (m mat) per(n mat) mat { // m poi n
	return mat{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

// matriceTimbro: dove disegnare l'immagine (quadrato unitario) nello spazio della pagina.
// vb è la parte visibile della pagina (CropBox o MediaBox), rot la rotazione /Rotate.
func matriceTimbro(vb *types.Rectangle, rot int, t Timbro) mat {
	rot = ((rot % 360) + 360) % 360
	rot = int(math.Round(float64(rot)/90)) * 90 % 360
	wu, hu := vb.Width(), vb.Height()
	dw, dh := wu, hu
	if rot == 90 || rot == 270 {
		dw, dh = hu, wu
	}
	x, y, w, h := t.X*dw, t.Y*dh, t.L*dw, t.A*dh
	// nella pagina come si vede, con y verso l'alto
	img := mat{w, 0, 0, h, x, dh - y - h}
	var d2u mat
	switch rot {
	case 90:
		d2u = mat{0, 1, -1, 0, wu, 0}
	case 180:
		d2u = mat{-1, 0, 0, -1, wu, hu}
	case 270:
		d2u = mat{0, -1, 1, 0, 0, hu}
	default:
		d2u = mat{1, 0, 0, 1, 0, 0}
	}
	sposta := mat{1, 0, 0, 1, vb.LL.X, vb.LL.Y}
	return img.per(d2u).per(sposta)
}

func numPDF(f float64) string {
	if math.Abs(f) < 1e-9 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', 4, 64)
}

// Timbra: mette firme, date, testi e spunte sulle pagine e restituisce il nuovo PDF.
func Timbra(data []byte, pw string, timbri []Timbro) ([]byte, error) {
	if len(timbri) == 0 {
		return nil, errc("vuoto", nil)
	}
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
	bg := context.Background()

	// Un'immagine per timbro, usata su tutte le pagine dove va.
	immagini := make([]*types.IndirectRef, len(timbri))
	for i, t := range timbri {
		if t.L <= 0 || t.A <= 0 || len(t.PNG) == 0 {
			return nil, errc("timbro", nil)
		}
		ir, _, _, err := model.CreateImageResource(ctx.XRefTable, bytes.NewReader(t.PNG))
		if err != nil {
			return nil, errc("timbro", err)
		}
		immagini[i] = ir
	}

	for p := 1; p <= ctx.PageCount; p++ {
		var qui []int
		for i, t := range timbri {
			if t.Pagina == 0 || t.Pagina == p {
				qui = append(qui, i)
			}
		}
		if len(qui) == 0 {
			continue
		}
		d, _, attrs, err := ctx.PageDict(bg, p, false)
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

		// Risorse: una copia solo per questa pagina (le risorse possono essere condivise con altre).
		var res types.Dict
		if o, ok := d.Find("Resources"); ok {
			res, _ = ctx.DereferenceDict(o)
		}
		if res == nil && attrs.Resources != nil {
			res = attrs.Resources
		}
		nuove := types.NewDict()
		for k, v := range res {
			nuove[k] = v
		}
		xo := types.NewDict()
		if o, ok := nuove.Find("XObject"); ok {
			if old, err := ctx.DereferenceDict(o); err == nil {
				for k, v := range old {
					xo[k] = v
				}
			}
		}
		var cs strings.Builder
		cs.WriteString("Q\nq\n")
		for _, i := range qui {
			nome := ""
			for n := 1; ; n++ {
				nome = fmt.Sprintf("TassoPDF%d", n)
				if _, esiste := xo[nome]; !esiste {
					break
				}
			}
			xo[nome] = *immagini[i]
			m := matriceTimbro(vb, attrs.Rotate, timbri[i])
			fmt.Fprintf(&cs, "q %s %s %s %s %s %s cm /%s Do Q\n",
				numPDF(m[0]), numPDF(m[1]), numPDF(m[2]), numPDF(m[3]), numPDF(m[4]), numPDF(m[5]), nome)
		}
		cs.WriteString("Q\n")
		nuove["XObject"] = xo
		d["Resources"] = nuove

		// Contenuto: "q" prima di quello che c'era (così i suoi cambi di stato non toccano la
		// firma) e la firma dopo, in cima a tutto.
		prima, err := nuovoStream(ctx, []byte("q\n"))
		if err != nil {
			return nil, err
		}
		dopo, err := nuovoStream(ctx, []byte(cs.String()))
		if err != nil {
			return nil, err
		}
		arr := types.Array{*prima}
		if o, ok := d.Find("Contents"); ok && o != nil {
			o2, err := ctx.Dereference(o)
			if err == nil {
				if a, isArr := o2.(types.Array); isArr {
					arr = append(arr, a...)
				} else {
					arr = append(arr, o)
				}
			}
		}
		arr = append(arr, *dopo)
		d["Contents"] = arr
	}
	return scrivi(ctx)
}

func nuovoStream(ctx *model.Context, b []byte) (*types.IndirectRef, error) {
	sd, err := ctx.NewStreamDictForBuf(b)
	if err != nil {
		return nil, err
	}
	if err := sd.Encode(); err != nil {
		return nil, err
	}
	return ctx.IndRefForNewObject(*sd)
}
