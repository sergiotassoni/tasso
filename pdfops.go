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
	"image"
	_ "image/png"
	"io"
	"math"
	"regexp"
	"sort"
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
	// Solo i messaggi che dicono davvero "manca l'intestazione %PDF-". Una parola generica come
	// "version" no: "root version: missing root dict" è un PDF scaricato a metà (rovinato).
	case strings.Contains(s, "no pdf") || strings.Contains(s, "not a pdf") || strings.Contains(s, "missing header") ||
		strings.Contains(s, "no header version"):
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
	// FirmatoDigitalmente: il PDF contiene una firma digitale (PAdES, campo /Sig firmato).
	// Ogni copia modificata (firma disegnata, testo, unione...) la rende non più valida.
	FirmatoDigitalmente bool `json:"firmatoDigitalmente"`
	// ConPassword: per aprirlo serve una password (anche se qui è stata data giusta).
	ConPassword bool `json:"conPassword"`
	// Moduli (ZP05-12): il PDF ha campi da compilare (/AcroForm con /Fields non vuoto).
	Moduli bool `json:"moduli"`
	// Attivo (ZP06-8): JavaScript, Launch, invio di moduli, azioni automatiche (non GoTo) o
	// allegati. In Tasso PDF sono spenti, ma le copie li conservano: la pagina lo dice.
	Attivo bool `json:"attivo"`
}

// haModuli: /AcroForm con almeno un campo.
func haModuli(ctx *model.Context) bool {
	af, err := ctx.DereferenceDict(ctx.RootDict["AcroForm"])
	if err != nil || af == nil {
		return false
	}
	f, err := ctx.DereferenceArray(af["Fields"])
	return err == nil && len(f) > 0
}

// azioniAttive: le azioni che un lettore PDF potrebbe eseguire da solo o con un clic.
var azioniAttive = map[string]bool{"JavaScript": true, "Launch": true, "SubmitForm": true, "ImportData": true, "Rendition": true, "GoToE": true}

// haContenutoAttivo: cerca in tutto il PDF JavaScript, Launch, SubmitForm, /AA, /OpenAction
// che non sia una semplice destinazione, e allegati.
func haContenutoAttivo(ctx *model.Context) bool {
	if oa, ok := ctx.RootDict.Find("OpenAction"); ok {
		if d, err := ctx.DereferenceDict(oa); err == nil && d != nil {
			if n, ok := d["S"].(types.Name); ok && n != "GoTo" {
				return true
			}
		}
	}
	if nn, err := ctx.DereferenceDict(ctx.RootDict["Names"]); err == nil && nn != nil {
		if _, ok := nn.Find("JavaScript"); ok {
			return true
		}
		if _, ok := nn.Find("EmbeddedFiles"); ok {
			return true
		}
	}
	for _, e := range ctx.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		var d types.Dict
		switch x := e.Object.(type) {
		case types.Dict:
			d = x
		case types.StreamDict:
			d = x.Dict
		default:
			continue
		}
		if dictAttivo(d, 0) {
			return true
		}
	}
	return false
}

func dictAttivo(d types.Dict, prof int) bool {
	if prof > 8 {
		return false
	}
	if _, ok := d["AA"]; ok {
		return true
	}
	if _, ok := d["JS"]; ok {
		return true
	}
	if n, ok := d["S"].(types.Name); ok && azioniAttive[string(n)] {
		return true
	}
	if n, ok := d["Subtype"].(types.Name); ok && n == "FileAttachment" {
		return true
	}
	if t := d.Type(); t != nil && (*t == "Filespec" || *t == "EmbeddedFile") {
		if _, ok := d["EF"]; ok || *t == "EmbeddedFile" {
			return true
		}
	}
	for _, v := range d {
		if x, ok := v.(types.Dict); ok && dictAttivo(x, prof+1) {
			return true
		}
	}
	return false
}

// haFirmaDigitale: c'è almeno un campo firma (/FT /Sig) con un valore (/V), oppure
// /SigFlags con il bit "SignaturesExist" nel modulo del PDF.
func haFirmaDigitale(ctx *model.Context) bool {
	root, err := ctx.Catalog()
	if err != nil || root == nil {
		return false
	}
	o, ok := root.Find("AcroForm")
	if !ok {
		return false
	}
	af, err := ctx.DereferenceDict(o)
	if err != nil || af == nil {
		return false
	}
	if f, ok := af.Find("SigFlags"); ok {
		if n, err := ctx.DereferenceInteger(f); err == nil && n != nil && n.Value()&1 != 0 {
			return true
		}
	}
	visti := map[types.IndirectRef]bool{}
	var campo func(o types.Object, prof int) bool
	campo = func(o types.Object, prof int) bool {
		if prof > 30 {
			return false
		}
		if ir, ok := o.(types.IndirectRef); ok {
			if visti[ir] {
				return false
			}
			visti[ir] = true
		}
		d, err := ctx.DereferenceDict(o)
		if err != nil || d == nil {
			return false
		}
		if ft := d.NameEntry("FT"); ft != nil && *ft == "Sig" {
			if v, ok := d.Find("V"); ok && v != nil {
				return true
			}
		}
		if k, ok := d.Find("Kids"); ok {
			if arr, err := ctx.DereferenceArray(k); err == nil {
				for _, x := range arr {
					if campo(x, prof+1) {
						return true
					}
				}
			}
		}
		return false
	}
	if f, ok := af.Find("Fields"); ok {
		if arr, err := ctx.DereferenceArray(f); err == nil {
			for _, x := range arr {
				if campo(x, 0) {
					return true
				}
			}
		}
	}
	return false
}

// leggi: legge il PDF (anche rovinato). Se pdfcpu non ci riesce, o legge un file senza pagine
// utilizzabili, prova a ricostruire l'indice degli oggetti. rip = è servita la riparazione.
func leggi(data []byte, pw string) (*model.Context, bool, error) {
	if !sembraPDF(data) {
		return nil, false, errc("non_pdf", nil)
	}
	var ctx *model.Context
	err := senzaPanico(func() (e error) {
		ctx, e = leggiConPassword(data, pw)
		return e
	})
	if err == nil && oltreLimite(ctx) {
		// Un numero di oggetto oltre il limite dei PDF (file rovinato): pdfcpu, scrivendo la copia,
		// passerebbe uno per uno tutti i numeri fino a lì (miliardi: il programma si blocca).
		// La riparazione qui sotto quegli oggetti non li prende.
		err = errors.New("numero di oggetto oltre il limite dei PDF")
	}
	if err == nil {
		if errP := senzaPanico(func() error { return controllaPagine(ctx) }); errP == nil {
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
		var ctx2 *model.Context
		err2 := senzaPanico(func() (e error) {
			ctx2, e = leggiConPassword(ric, pw)
			return e
		})
		if err2 != nil {
			if c := classifica(err2); CodiceErrore(c) == "password" {
				return nil, false, c
			}
			continue
		}
		if oltreLimite(ctx2) {
			continue
		}
		if errP := senzaPanico(func() error { return controllaPagine(ctx2) }); errP != nil {
			continue
		}
		return ctx2, true, nil
	}
	return nil, false, classifica(err)
}

// leggiConPassword: legge il PDF con la password dell'utente. Nei PDF con la protezione più
// vecchia (RC4 o AES a 128 bit, fatti per esempio da Acrobat o qpdf) la password con le
// lettere accentate ("città") è scritta nella codifica PDFDocEncoding, non in UTF-8: se la
// prima prova dice "password sbagliata", riprovo con quella codifica.
func leggiConPassword(data []byte, pw string) (*model.Context, error) {
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(data), confBase(pw))
	if err == nil || CodiceErrore(classifica(err)) != "password" {
		return ctx, err
	}
	if alt, ok := pdfDocEncoding(pw); ok && alt != pw {
		if ctx2, err2 := api.ReadContext(context.Background(), bytes.NewReader(data), confBase(alt)); err2 == nil {
			return ctx2, nil
		}
	}
	return ctx, err
}

// pdfDocEncoding: pw nella codifica PDFDocEncoding (uguale a Latin-1 per le lettere accentate,
// € = 0xA0). ok = false se pw non è ASCII o contiene un carattere che lì non c'è.
func pdfDocEncoding(pw string) (string, bool) {
	var b []byte
	ascii := true
	for _, r := range pw {
		switch {
		case r >= 0x20 && r <= 0x7E:
			b = append(b, byte(r))
		case r == '€':
			b, ascii = append(b, 0xA0), false
		case r >= 0xA1 && r <= 0xFF && r != 0xAD:
			b, ascii = append(b, byte(r)), false
		default:
			return "", false
		}
	}
	return string(b), !ascii
}

// senzaPanico: esegue f; se pdfcpu va in panico su un file rovinato (es. "/MediaBox [ ]"),
// diventa un errore normale: così si prova la riparazione e, se non basta, il messaggio è
// "PDF rovinato" invece di "errore interno".
func senzaPanico(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("file rovinato (%v)", r)
		}
	}()
	return f()
}

// maxOggettiPDF: il numero di oggetto più alto che un PDF può avere (specifica PDF, allegato C:
// 8.388.607 oggetti). Numeri più alti si trovano solo nei file rovinati.
const maxOggettiPDF = 8_388_607

// oltreLimite: il PDF letto dice di avere oggetti oltre il limite (vedi maxOggettiPDF).
func oltreLimite(ctx *model.Context) bool {
	return ctx.XRefTable.Size != nil && *ctx.XRefTable.Size > maxOggettiPDF+1
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
	sistemaMisure(ctx)
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

// boxValida: o è un rettangolo [x1 y1 x2 y2] di 4 numeri con larghezza e altezza non nulle.
func boxValida(ctx *model.Context, o types.Object) bool {
	o, err := ctx.Dereference(o)
	if err != nil {
		return false
	}
	a, ok := o.(types.Array)
	if !ok || len(a) != 4 {
		return false
	}
	var v [4]float64
	for i, x := range a {
		x, err := ctx.Dereference(x)
		if err != nil {
			return false
		}
		switch n := x.(type) {
		case types.Integer:
			v[i] = float64(n)
		case types.Float:
			v[i] = float64(n)
		default:
			return false
		}
		if math.IsNaN(v[i]) || math.IsInf(v[i], 0) {
			return false
		}
	}
	return v[0] != v[2] && v[1] != v[3]
}

// sistemaMisure: una pagina (o un elenco di pagine) con la misura rovinata, per esempio
// "/MediaBox []", diventa A4; un ritaglio (/CropBox) rovinato viene tolto. Senza, tutto il
// file risultava "rovinato" (anche Ripara), pur avendo le pagine intere.
func sistemaMisure(ctx *model.Context) {
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		t := d.Type()
		if t == nil || (*t != "Page" && *t != "Pages") {
			continue
		}
		if o, ok := d.Find("MediaBox"); ok && !boxValida(ctx, o) {
			d.Update("MediaBox", types.Array{types.Integer(0), types.Integer(0), types.Float(595.28), types.Float(841.89)})
		}
		if o, ok := d.Find("CropBox"); ok && !boxValida(ctx, o) {
			d.Delete("CropBox")
		}
	}
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
			inf.Cifrato, inf.ServePassword, inf.ConPassword = true, true, true
			return inf, nil
		}
		return inf, err
	}
	inf.Riparato = rip
	inf.Cifrato = ctx.Encrypt != nil
	inf.FirmatoDigitalmente = haFirmaDigitale(ctx)
	inf.Moduli = haModuli(ctx)
	inf.Attivo = haContenutoAttivo(ctx)
	if ctx.Encrypt != nil && pw != "" {
		// Si apre anche senza password? Se no, la password data era quella di apertura.
		if _, _, e := leggi(data, ""); CodiceErrore(e) == "password" {
			inf.ConPassword = true
		}
	}
	inf.PasswordGiusta = true
	// Bit 3 stampa, 4 modifica, 5 copia, 6 note, 9 moduli, 10 accessibilità, 11 assemblaggio, 12 stampa buona.
	inf.Bloccato = haBlocchi(ctx)
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
		// Una password c'era, ma non apre il PDF: "password_sbagliata", non "serve la password"
		// (PRESTAZIONI-ZIP-PDF, P3).
		if pw != "" && CodiceErrore(err) == "password" {
			return nil, errc("password_sbagliata", err)
		}
		return nil, err
	}
	if ctx.Encrypt == nil && !rip {
		return data, nil
	}
	return scrivi(ctx)
}

// ---------------------------------------------------------------- Sblocca: due modi

// ModoSblocco: come si decide se si possono togliere i blocchi di un PDF.
//
//	SbloccoConConferma (B): l'utente spunta "Confermo che il PDF è mio o che ho il diritto
//	                        di sbloccarlo" (obbligatoria). Per i PDF con password di apertura
//	                        serve comunque quella password.
//	SbloccoConProprietario (A): i blocchi (stampa, copia, modifica) si tolgono solo con la
//	                        password del proprietario (quella dei permessi). Senza blocchi
//	                        basta la password di apertura.
type ModoSblocco string

const (
	SbloccoConProprietario ModoSblocco = "A"
	SbloccoConConferma     ModoSblocco = "B"
)

// modoSblocco: LA scelta del titolare. Per passare all'altro modo basta cambiare questa riga
// (l'interfaccia lo legge da goAvvio e mostra la spunta o il campo della password).
const modoSblocco = SbloccoConConferma

// passwordProprietarioGiusta: pw è la password del proprietario (dei permessi) di questo PDF?
// pdfcpu prova prima la password del proprietario e, se è giusta, si ferma lì; altrimenti
// prova quella di apertura. Come password di apertura gli do una stringa che nessuno può
// avere scritto: così il file si apre solo se pw è proprio quella del proprietario.
func passwordProprietarioGiusta(data []byte, pw string) bool {
	c := confBase("")
	c.OwnerPW, c.UserPW = pw, "\x00tasso-pdf: non una password\x00"
	_, err := api.ReadContext(context.Background(), bytes.NewReader(data), c)
	return err == nil
}

// Sblocca: toglie blocchi e password da un PDF dell'utente e restituisce una copia libera.
// La password, se serve, la dà l'utente: il programma non prova mai a indovinarla.
// Modo B: senza la conferma dell'utente (confermato) non fa niente. Modo A: se il PDF ha
// dei blocchi, pw deve essere la password del proprietario.
func Sblocca(data []byte, pw string, confermato bool) ([]byte, error) {
	return sbloccaCon(modoSblocco, data, pw, confermato)
}

func sbloccaCon(modo ModoSblocco, data []byte, pw string, confermato bool) ([]byte, error) {
	if modo == SbloccoConConferma && !confermato {
		return nil, errc("conferma_diritti", nil)
	}
	ctx, _, err := leggi(data, pw)
	if err != nil {
		return nil, err
	}
	if ctx.Encrypt == nil {
		return nil, errc("non_protetto", nil)
	}
	if modo == SbloccoConProprietario && haBlocchi(ctx) && !passwordProprietarioGiusta(data, pw) {
		return nil, errc("password_proprietario", nil)
	}
	return scrivi(ctx)
}

// haBlocchi: il PDF vieta qualcosa (stampa, modifica, copia, note, moduli, assemblaggio...).
func haBlocchi(ctx *model.Context) bool {
	if ctx.E == nil {
		return false
	}
	const tutti = 4 | 8 | 16 | 32 | 256 | 512 | 1024 | 2048
	return ctx.E.P&tutti != tutti
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
var reXRef = regexp.MustCompile(`/Type\s*/XRef\b`)
var rePagesRef = regexp.MustCompile(`/Pages\s+\d+\s+\d+\s+R`)
var reParentRef = regexp.MustCompile(`/Parent\s+\d+\s+\d+\s+R`)

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
	var xrefStream []int // posizioni degli oggetti "/Type /XRef" (indice compresso dei PDF moderni)
	root := 0
	taglio := len(data)
	for _, m := range reObj.FindAllSubmatchIndex(data, -1) {
		num, err1 := strconv.Atoi(string(data[m[2]:m[3]]))
		gen, err2 := strconv.Atoi(string(data[m[4]:m[5]]))
		if err1 != nil || err2 != nil || num <= 0 || num > maxOggettiPDF {
			continue // numeri oltre il limite dei PDF: pezzi rovinati, non oggetti veri
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
		if reXRef.Match(corpo) {
			xrefStream = append(xrefStream, m[2])
		}
	}
	if root == 0 {
		// PDF moderni (Word, Chrome, scanner): il catalogo sta dentro un "object stream" e
		// l'indice è uno stream /Type /XRef. Non lo riscrivo: faccio puntare startxref
		// all'ultimo indice compresso ancora intero (di solito è solo startxref a essere sbagliato).
		for i := len(xrefStream) - 1; i >= 0; i-- {
			if xrefStream[i] < taglio {
				var b bytes.Buffer
				b.Write(data[:taglio])
				fmt.Fprintf(&b, "\nstartxref\n%d\n%%%%EOF\n", xrefStream[i])
				return b.Bytes(), nil
			}
		}
		return nil, errors.New("nessun catalogo trovato")
	}
	if len(trovati) == 0 {
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
		var tenute []int
		visto := map[int]bool{}
		for _, n := range ordinePagine {
			if _, ok := trovati[n]; ok && !visto[n] {
				visto[n] = true
				tenute = append(tenute, n)
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
		// Ogni pagina deve dire che il suo "genitore" è il nuovo elenco (/Parent): riscrivo una
		// copia di ciascuna in fondo, e l'indice punta alla copia. Senza, i lettori pignoli
		// (e il controllo rigoroso) dicono che il file è rovinato.
		nuovoParent := fmt.Sprintf("/Parent %d 0 R", pagesNum)
		for _, n := range tenute {
			p := trovati[n]
			corpo := string(data[p.inizioCorpo:p.fineCorpo])
			if reParentRef.MatchString(corpo) {
				corpo = reParentRef.ReplaceAllLiteralString(corpo, nuovoParent)
			} else if i := strings.Index(corpo, "<<"); i >= 0 {
				corpo = corpo[:i+2] + " " + nuovoParent + " " + corpo[i+2:]
			} else {
				continue
			}
			trovati[n] = pos{off: b.Len(), gen: p.gen}
			fmt.Fprintf(&b, "%d %d obj\n%s\nendobj\n", n, p.gen, strings.TrimSpace(corpo))
		}
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
	// L'indice elenca solo gli oggetti trovati, a gruppi di numeri consecutivi ("da quanti"):
	// un numero enorme in un file rovinato (es. "4000000000 0 obj") non deve far scrivere
	// miliardi di righe (memoria del PC esaurita: il programma si chiuderebbe di colpo).
	numeri := make([]int, 0, len(trovati))
	for n := range trovati {
		numeri = append(numeri, n)
	}
	sort.Ints(numeri)
	xref := b.Len()
	b.WriteString("xref\n0 1\n0000000000 65535 f\r\n")
	for i := 0; i < len(numeri); {
		j := i
		for j+1 < len(numeri) && numeri[j+1] == numeri[j]+1 {
			j++
		}
		fmt.Fprintf(&b, "%d %d\r\n", numeri[i], j-i+1)
		for _, n := range numeri[i : j+1] {
			fmt.Fprintf(&b, "%010d %05d n\r\n", trovati[n].off, trovati[n].gen)
		}
		i = j + 1
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
	return unisciParti(parti)
}

// unisciParti: mette insieme PDF già in chiaro. pdfcpu rifiuta di aggiungere un PDF 2.0 a un
// PDF più vecchio ("PDF 2.0 unsupported"): se una delle parti è 2.0, la prima parte (quella a
// cui si aggiungono le altre) diventa anche lei 2.0. Il 2.0 legge tutto quello che c'è nei
// PDF più vecchi, quindi non si perde niente.
func unisciParti(parti [][]byte) ([]byte, error) {
	if len(parti) > 1 {
		c0, err := api.ReadContext(context.Background(), bytes.NewReader(parti[0]), confBase(""))
		if err != nil {
			return nil, classifica(err)
		}
		if c0.XRefTable.Version() < model.V20 {
			serve := false
			for _, b := range parti[1:] {
				c, err := api.ReadContext(context.Background(), bytes.NewReader(b), confBase(""))
				if err == nil && c.XRefTable.Version() == model.V20 {
					serve = true
					break
				}
			}
			if serve {
				v := model.V20
				c0.HeaderVersion = &v
				c0.RootVersion = &v
				b0, err := scrivi(c0)
				if err != nil {
					return nil, err
				}
				parti = append([][]byte{b0}, parti[1:]...)
			}
		}
	}
	out, err := uniscieRaw(parti)
	if err != nil && strings.Contains(err.Error(), "unsupported in version") {
		// ZP05-5: un PDF buono con l'intestazione «troppo vecchia» per quello che contiene (per
		// esempio 1.3 con un allegato della 1.4): lo riscrivo come 1.7 e riprovo una volta.
		nuove := make([][]byte, len(parti))
		for i, b := range parti {
			nuove[i] = b
			c, errL := api.ReadContext(context.Background(), bytes.NewReader(b), confBase(""))
			if errL != nil || c.XRefTable.Version() >= model.V17 {
				continue
			}
			v := model.V17
			c.HeaderVersion, c.RootVersion = &v, &v
			if b2, errS := scrivi(c); errS == nil {
				nuove[i] = b2
			}
		}
		out, err = uniscieRaw(nuove)
	}
	if err != nil {
		return nil, classifica(err)
	}
	return out, nil
}

func uniscieRaw(parti [][]byte) ([]byte, error) {
	var rss []io.ReadSeeker
	for _, b := range parti {
		rss = append(rss, bytes.NewReader(b))
	}
	var out bytes.Buffer
	conf := confBase("")
	conf.CreateBookmarks = false
	if err := api.MergeRaw(context.Background(), rss, &out, false, conf); err != nil {
		return nil, err
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
	// ZP06-3: con Pagina 0 (tutte le pagine) la larghezza in punti: su pagine di forma diversa
	// la firma resta grande uguale e con le proporzioni dell'immagine (prop = altezza/larghezza).
	LPt  float64 `json:"lpt"`
	prop float64
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
	if t.Pagina == 0 && t.LPt > 0 && t.prop > 0 {
		cx, cy := x+w/2, y+h/2 // lo stesso centro
		w = math.Min(t.LPt, dw)
		h = w * t.prop
		if h > dh {
			h, w = dh, dh/t.prop
		}
		x = math.Max(0, math.Min(dw-w, cx-w/2))
		y = math.Max(0, math.Min(dh-h, cy-h/2))
	}
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
	timbri = append([]Timbro{}, timbri...)
	for i := range timbri {
		if c, _, err := image.DecodeConfig(bytes.NewReader(timbri[i].PNG)); err == nil && c.Width > 0 {
			timbri[i].prop = float64(c.Height) / float64(c.Width)
		}
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
		// Una pagina che non c'è (oltre la fine): errore, non una firma che sparisce in silenzio.
		if t.Pagina < 0 || t.Pagina > ctx.PageCount {
			return nil, errc("pagina_fuori", nil)
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
