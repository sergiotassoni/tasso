// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// app.go: i file aperti, le funzioni chiamate dalla pagina e le pagine interne del programma.
// Non dipende da Windows (le parti di Windows sono in "piattaforma", vedi sistema_windows.go).

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const versione = "1.1.1"

// Indirizzo delle pagine interne: non esiste su internet, le risponde il programma stesso.
const hostInterno = "tassopdf.localhost"
const indirizzoBase = "https://" + hostInterno + "/"

//go:embed ui
var uiFS embed.FS

// Massimo per un file: oltre, è quasi certamente un errore (e la memoria del PC non basta).
const maxFile = 1 << 30 // 1 GB

// piattaforma: le cose che dipendono dal sistema (finestre di Windows, DPAPI, registro...).
var piattaforma struct {
	scegliFile    func(multi bool, soloPDF bool, titolo string) ([]string, error)
	salvaFile     func(nomeSuggerito, cartella, titolo, ext string) (string, error)
	apriLink      func(u string) error
	mostraFile    func(p string) error
	proteggi      func([]byte) ([]byte, error)
	svela         func([]byte) ([]byte, error)
	cartellaDati  func() string
	linguaTasso   func() string // HKCU\Software\Tasso\Lingua (scritta dall'installatore dell'ecosistema)
	linguaSistema func() string
	menuAttivo    func() bool
	impostaMenu   func(on bool, lingua string) error
	apriTassoAgg  func() string // apre Tasso su Impostazioni → Aggiornamenti (aggiornamenti.go)
	// Marchio del web (Mark-of-the-Web): il flusso NTFS ":Zone.Identifier" che Windows mette
	// sui file scaricati. Lo copiamo sulle copie salvate, così Windows, SmartScreen e i lettori
	// PDF (Protected View) sanno ancora che il contenuto viene da internet.
	leggiZona  func(p string) []byte
	scriviZona func(p string, zona []byte) error
}

// ---------------------------------------------------------------- Archivio dei file

// Voce: un file aperto dall'utente (dal disco o trascinato) o un risultato non ancora salvato.
type Voce struct {
	ID       string
	Nome     string // nome del file, es. "Contratto.pdf"
	Percorso string // percorso sul disco, se il file viene da lì
	Origine  string // per i risultati: percorso del file da cui vengono (per proporre la cartella)
	Dati     []byte // contenuto, se non è (solo) sul disco
	Tipo     string // pdf, jpg, png, heic
	jpgHEIC  []byte // anteprima JPG di una foto HEIC
	Zona     []byte // marchio del web del file d'origine (nil = file locale o sconosciuto)
}

// idZona: il numero ZoneId del marchio del web (3 = internet, 4 = siti non attendibili); 0 se manca.
func idZona(z []byte) int {
	for _, r := range strings.Split(string(z), "\n") {
		r = strings.TrimSpace(r)
		if strings.HasPrefix(strings.ToLower(r), "zoneid=") {
			n, _ := strconv.Atoi(strings.TrimSpace(r[len("zoneid="):]))
			return n
		}
	}
	return 0
}

// zonaPiuSevera: fra più file uniti, il marchio che protegge di più (il numero di zona più alto).
func zonaPiuSevera(zz ...[]byte) []byte {
	var scelta []byte
	for _, z := range zz {
		if len(z) > 0 && (scelta == nil || idZona(z) > idZona(scelta)) {
			scelta = z
		}
	}
	return scelta
}

// mettiZona: dopo il salvataggio, copia il marchio del web sul file nuovo (se l'origine lo aveva).
func mettiZona(p string, zona []byte) {
	if len(zona) > 0 && piattaforma.scriviZona != nil {
		_ = piattaforma.scriviZona(p, zona)
	}
}

// VoceInfo: quello che la pagina sa di un file.
type VoceInfo struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
	Tipo string `json:"tipo"`
	Dim  int64  `json:"dim"`
}

var archivio = struct {
	sync.Mutex
	m map[string]*Voce
	n int
}{m: map[string]*Voce{}}

func aggiungiVoce(v *Voce) VoceInfo {
	archivio.Lock()
	archivio.n++
	v.ID = "f" + strconv.Itoa(archivio.n)
	archivio.m[v.ID] = v
	archivio.Unlock()
	return v.info()
}

func (v *Voce) info() VoceInfo {
	dim := int64(len(v.Dati))
	if v.Dati == nil && v.Percorso != "" {
		if fi, err := os.Stat(v.Percorso); err == nil {
			dim = fi.Size()
		}
	}
	return VoceInfo{ID: v.ID, Nome: v.Nome, Tipo: v.Tipo, Dim: dim}
}

func prendiVoce(id string) (*Voce, error) {
	archivio.Lock()
	defer archivio.Unlock()
	v, ok := archivio.m[id]
	if !ok {
		return nil, errc("perso", nil)
	}
	return v, nil
}

// contenuto: i byte del file (letti dal disco ogni volta: l'originale non si tocca mai).
func (v *Voce) contenuto() ([]byte, error) {
	if v.Dati != nil {
		return v.Dati, nil
	}
	fi, err := os.Stat(v.Percorso)
	if err != nil {
		return nil, errc("lettura", err)
	}
	if fi.Size() > maxFile {
		return nil, errc("troppo_grande", nil)
	}
	b, err := os.ReadFile(v.Percorso)
	if err != nil {
		return nil, errc("lettura", err)
	}
	return b, nil
}

// cartella: dove proporre di salvare (quella del file originale, se si sa).
func (v *Voce) cartella() string {
	for _, p := range []string{v.Origine, v.Percorso} {
		if p != "" {
			return filepath.Dir(p)
		}
	}
	return ""
}

func apriPercorso(p string) (VoceInfo, error) {
	f, err := os.Open(p)
	if err != nil {
		return VoceInfo{}, errc("lettura", err)
	}
	testa := make([]byte, 1024)
	n, _ := f.Read(testa)
	f.Close()
	tipo := TipoFile(testa[:n])
	if tipo == "" {
		tipo = tipoDaNome(p)
	}
	v := &Voce{Nome: filepath.Base(p), Percorso: p, Tipo: tipo}
	if piattaforma.leggiZona != nil {
		v.Zona = piattaforma.leggiZona(p)
	}
	return aggiungiVoce(v), nil
}

func tipoDaNome(n string) string {
	switch strings.ToLower(filepath.Ext(n)) {
	case ".pdf":
		return "pdf"
	case ".jpg", ".jpeg":
		return "jpg"
	case ".png":
		return "png"
	case ".heic", ".heif":
		return "heic"
	}
	return ""
}

// ---------------------------------------------------------------- Errori per la pagina

// perPagina: l'errore come lo legge la pagina: "codice|dettaglio" oppure "pezzo:N|codice|dettaglio".
func perPagina(err error) error {
	if err == nil {
		return nil
	}
	pre := ""
	var ep *ErrPezzo
	if errors.As(err, &ep) {
		pre = "pezzo:" + strconv.Itoa(ep.Indice) + "|"
	}
	c := CodiceErrore(err)
	if c == "" {
		c = "generico"
	}
	return errors.New(pre + c + "|" + err.Error())
}

// ---------------------------------------------------------------- Funzioni per la pagina

// Avvio: le informazioni che servono alla pagina appena aperta.
type Avvio struct {
	Lingua   string `json:"lingua"`
	Versione string `json:"versione"`
	Firma    bool   `json:"firma"` // c'è una firma salvata
	Menu     bool   `json:"menu"`  // le voci nel menu del tasto destro di Windows sono attive
	Windows  bool   `json:"windows"`
	Sblocco  string `json:"sblocco"` // "A" (password del proprietario) o "B" (spunta di conferma)
	Nome     string `json:"nome"`    // "Il tuo nome" (nome_utente.go), "" = senza nome
}

type impostazioni struct {
	Lingua string `json:"lingua"`
}

func fileImpostazioni() string {
	if piattaforma.cartellaDati == nil {
		return ""
	}
	d := piattaforma.cartellaDati()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "impostazioni.json")
}

func leggiImpostazioni() impostazioni {
	var imp impostazioni
	if p := fileImpostazioni(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &imp)
		}
	}
	return imp
}

var lingueNote = map[string]bool{"it": true, "en": true, "es": true, "fr": true, "de": true, "pt": true, "pl": true}

// linguaIniziale: la scelta fatta in alto nel programma, poi quella dell'installatore di
// Tasso (HKCU\Software\Tasso\Lingua), poi la lingua di Windows; se nessuna è nota, inglese.
func linguaIniziale() string {
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if len(s) > 2 {
			s = s[:2]
		}
		return s
	}
	cand := []string{leggiImpostazioni().Lingua}
	if piattaforma.linguaTasso != nil {
		cand = append(cand, piattaforma.linguaTasso())
	}
	if piattaforma.linguaSistema != nil {
		cand = append(cand, piattaforma.linguaSistema())
	}
	for _, c := range cand {
		if l := norm(c); lingueNote[l] {
			return l
		}
	}
	return "en"
}

func goAvvio() Avvio {
	return Avvio{Lingua: linguaIniziale(), Versione: versione, Firma: fileFirmaEsiste(), Menu: goMenu(), Windows: piattaforma.impostaMenu != nil, Sblocco: string(modoSblocco), Nome: leggiNome()}
}

func goLingua(l string) error {
	if !lingueNote[l] {
		return perPagina(errc("lingua", nil))
	}
	p := fileImpostazioni()
	if p == "" {
		return nil
	}
	// Le voci del menu del tasto destro seguono la lingua scelta.
	if goMenu() && piattaforma.impostaMenu != nil {
		_ = piattaforma.impostaMenu(true, l)
	}
	imp := leggiImpostazioni()
	imp.Lingua = l
	b, _ := json.Marshal(imp)
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	return os.WriteFile(p, b, 0o600)
}

// goScegli: la finestra di Windows "Apri" (soloPDF: solo PDF; altrimenti anche foto).
func goScegli(multi, soloPDF bool, titolo string) ([]VoceInfo, error) {
	if piattaforma.scegliFile == nil {
		return nil, nil
	}
	pp, err := piattaforma.scegliFile(multi, soloPDF, titolo)
	if err != nil {
		return nil, perPagina(errc("dialogo", err))
	}
	var out []VoceInfo
	for _, p := range pp {
		vi, err := apriPercorso(p)
		if err != nil {
			return nil, perPagina(err)
		}
		out = append(out, vi)
	}
	return out, nil
}

// File trascinati nella finestra: la pagina li manda a pezzi (base64), così anche i file grandi passano.
func goCaricaInizio(nome string) string {
	vi := aggiungiVoce(&Voce{Nome: filepath.Base(strings.ReplaceAll(nome, "\\", "/")), Dati: []byte{}})
	return vi.ID
}

func goCaricaPezzo(id, b64 string) error {
	v, err := prendiVoce(id)
	if err != nil {
		return perPagina(err)
	}
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return perPagina(errc("lettura", err))
	}
	archivio.Lock()
	defer archivio.Unlock()
	if len(v.Dati)+len(b) > maxFile {
		return perPagina(errc("troppo_grande", nil))
	}
	v.Dati = append(v.Dati, b...)
	return nil
}

func goCaricaFine(id string) (VoceInfo, error) {
	v, err := prendiVoce(id)
	if err != nil {
		return VoceInfo{}, perPagina(err)
	}
	archivio.Lock()
	v.Tipo = TipoFile(v.Dati)
	if v.Tipo == "" {
		v.Tipo = tipoDaNome(v.Nome)
	}
	archivio.Unlock()
	return v.info(), nil
}

// goDimentica: il file non serve più (libera la memoria).
func goDimentica(id string) {
	archivio.Lock()
	delete(archivio.m, id)
	archivio.Unlock()
}

func goInfo(id, pw string) (InfoDoc, error) {
	v, err := prendiVoce(id)
	if err != nil {
		return InfoDoc{}, perPagina(err)
	}
	b, err := v.contenuto()
	if err != nil {
		return InfoDoc{}, perPagina(err)
	}
	inf, err := Info(b, pw)
	return inf, perPagina(err)
}

// risultato: un nuovo file (non ancora salvato) nato da v.
func risultato(v *Voce, b []byte, nome string) VoceInfo {
	orig := v.Percorso
	if orig == "" {
		orig = v.Origine
	}
	return aggiungiVoce(&Voce{Nome: nome, Origine: orig, Dati: b, Tipo: "pdf", Zona: v.Zona})
}

type lavoro func([]byte, string) ([]byte, error)

func esegui(id, pw string, f lavoro) (VoceInfo, error) {
	v, err := prendiVoce(id)
	if err != nil {
		return VoceInfo{}, perPagina(err)
	}
	b, err := v.contenuto()
	if err != nil {
		return VoceInfo{}, perPagina(err)
	}
	out, err := f(b, pw)
	if err != nil {
		return VoceInfo{}, perPagina(err)
	}
	return risultato(v, out, v.Nome), nil
}

func goRipara(id, pw string) (VoceInfo, error) { return esegui(id, pw, Ripara) }

// goSblocca: confermato = l'utente ha spuntato "il PDF è mio o ho il diritto di sbloccarlo"
// (modo B); nel modo A pw deve essere la password del proprietario se il PDF ha blocchi.
func goSblocca(id, pw string, confermato bool) (VoceInfo, error) {
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return Sblocca(b, pw, confermato) })
}

// PezzoPagina: un file da unire, come lo manda la pagina.
type PezzoPagina struct {
	ID       string `json:"id"`
	Rot      int    `json:"rot"`
	Password string `json:"pw"`
}

func goUnisci(pezzi []PezzoPagina) (VoceInfo, error) {
	var pp []Pezzo
	var primo *Voce
	var zone [][]byte
	for i, p := range pezzi {
		v, err := prendiVoce(p.ID)
		if err != nil {
			return VoceInfo{}, perPagina(&ErrPezzo{Indice: i, Err: err})
		}
		if primo == nil {
			primo = v
		}
		b, err := v.contenuto()
		if err != nil {
			return VoceInfo{}, perPagina(&ErrPezzo{Indice: i, Err: err})
		}
		zone = append(zone, v.Zona)
		pp = append(pp, Pezzo{Nome: v.Nome, Dati: b, Foto: v.Tipo != "pdf", Rotazione: p.Rot, Password: p.Password})
	}
	out, err := Unisci(pp)
	if err != nil {
		return VoceInfo{}, perPagina(err)
	}
	if primo == nil {
		return VoceInfo{}, perPagina(errc("vuoto", nil))
	}
	vi := risultato(primo, out, primo.Nome)
	if nv, err := prendiVoce(vi.ID); err == nil {
		archivio.Lock()
		nv.Zona = zonaPiuSevera(zone...)
		archivio.Unlock()
	}
	return vi, nil
}

// TimbroPagina: firma, data, testo o spunta come li manda la pagina (PNG in base64).
type TimbroPagina struct {
	Timbro
	PNG64 string `json:"png"`
}

func goFirma(id, pw string, tt []TimbroPagina) (VoceInfo, error) {
	var timbri []Timbro
	for _, t := range tt {
		s := t.PNG64
		if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
			s = s[i+1:]
		}
		png, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return VoceInfo{}, perPagina(errc("timbro", err))
		}
		tb := t.Timbro
		tb.PNG = png
		timbri = append(timbri, tb)
	}
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return Timbra(b, pw, timbri) })
}

// stessoFile: a e b sono lo stesso file sul disco (anche scritti in modo diverso).
func stessoFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) {
		return true
	}
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

// eUnOriginale: p è uno dei file aperti dall'utente (che non vanno mai sovrascritti).
func eUnOriginale(p string) bool {
	archivio.Lock()
	defer archivio.Unlock()
	for _, v := range archivio.m {
		if stessoFile(v.Percorso, p) || stessoFile(v.Origine, p) {
			return true
		}
	}
	return false
}

// goSalva: la finestra di Windows "Salva con nome" (che chiede prima di sostituire un file
// esistente) e poi la scrittura. Restituisce il percorso, o "" se l'utente annulla.
func goSalva(id, nomeSuggerito, titolo string) (string, error) {
	return goSalvaTipo(id, nomeSuggerito, titolo, "pdf")
}

// estensioniSalvabili: i tipi di file che il programma scrive.
var estensioniSalvabili = map[string]bool{"pdf": true, "txt": true, "png": true, "jpg": true}

// chiediDove: la finestra "Salva con nome" per un file di tipo ext; mai sopra un originale.
func chiediDove(nomeSuggerito, cartella, titolo, ext string) (string, error) {
	if !estensioniSalvabili[ext] {
		return "", errc("generico", nil)
	}
	if piattaforma.salvaFile == nil {
		return "", errc("dialogo", nil)
	}
	p, err := piattaforma.salvaFile(nomeValido(nomeSuggerito), cartella, titolo, ext)
	if err != nil {
		return "", errc("dialogo", err)
	}
	if p == "" {
		return "", nil
	}
	if !strings.EqualFold(filepath.Ext(p), "."+ext) {
		// Es. "Contratto.txt" -> "Contratto.txt.pdf". Windows ha chiesto conferma solo per il
		// nome scritto, non per questo: se "Contratto.txt.pdf" esiste già, non lo sostituisco
		// in silenzio ma uso "Contratto.txt (2).pdf".
		p = percorsoLibero(p + "." + ext)
	}
	if eUnOriginale(p) {
		return "", errc("originale", nil)
	}
	return p, nil
}

func goSalvaTipo(id, nomeSuggerito, titolo, ext string) (string, error) {
	v, err := prendiVoce(id)
	if err != nil {
		return "", perPagina(err)
	}
	p, err := chiediDove(nomeSuggerito, v.cartella(), titolo, ext)
	if err != nil || p == "" {
		return "", perPagina(err)
	}
	b, err := v.contenuto()
	if err != nil {
		return "", perPagina(err)
	}
	if err := scriviSicuro(p, b); err != nil {
		return "", perPagina(errc("scrittura", err))
	}
	archivio.Lock()
	zona := v.Zona
	archivio.Unlock()
	mettiZona(p, zona)
	return p, nil
}

// scriviSicuro: scrive in un file temporaneo accanto e poi lo rinomina: se qualcosa va storto
// a metà, non resta un PDF mezzo scritto.
func scriviSicuro(p string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tassopdf-*.tmp")
	if err != nil {
		return err
	}
	nome := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(nome)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(nome)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(nome)
		return err
	}
	if err := os.Rename(nome, p); err != nil {
		os.Remove(nome)
		return err
	}
	return nil
}

// scriviNuovo: crea p solo se non esiste ancora (mai sovrascrivere). Se la scrittura si ferma
// a metà, il file incompleto viene tolto. Se p esiste già: errore fs.ErrExist.
func scriviNuovo(p string, b []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(p)
	}
	return err
}

// percorsoLibero: p, oppure "nome (2).ext", "nome (3).ext"... il primo che non esiste.
func percorsoLibero(p string) string {
	if _, err := os.Lstat(p); os.IsNotExist(err) && !eUnOriginale(p) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for k := 2; k < 1000; k++ {
		q := fmt.Sprintf("%s (%d)%s", base, k, ext)
		if _, err := os.Lstat(q); os.IsNotExist(err) && !eUnOriginale(q) {
			return q
		}
	}
	return p
}

// nomeValido: toglie dal nome i caratteri che Windows non accetta.
func nomeValido(n string) string {
	ext := strings.ToLower(filepath.Ext(n))
	if !estensioniSalvabili[strings.TrimPrefix(ext, ".")] {
		ext = ".pdf"
	}
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, n)
	n = strings.TrimSpace(strings.TrimRight(n, ". "))
	if n == "" {
		n = "documento" + ext
	}
	if len([]rune(n)) > 180 {
		n = string([]rune(n)[:176]) + ext
	}
	return n
}

// goApriLink: apre un collegamento nel browser, solo se l'utente ha detto sì nella pagina e
// solo per indirizzi web o di posta.
func goApriLink(u string) error {
	pu, err := url.Parse(strings.TrimSpace(u))
	if err != nil {
		return perPagina(errc("link", err))
	}
	switch strings.ToLower(pu.Scheme) {
	case "http", "https":
		if pu.Host == "" {
			return perPagina(errc("link", nil))
		}
	case "mailto":
	default:
		return perPagina(errc("link", nil))
	}
	if piattaforma.apriLink == nil {
		return nil
	}
	return perPagina(piattaforma.apriLink(pu.String()))
}

func goMostra(p string) error {
	if piattaforma.mostraFile == nil || p == "" {
		return nil
	}
	return piattaforma.mostraFile(p)
}

// ---------------------------------------------------------------- Firma salvata (DPAPI)

func fileFirma() string {
	if piattaforma.cartellaDati == nil || piattaforma.cartellaDati() == "" {
		return ""
	}
	return filepath.Join(piattaforma.cartellaDati(), "firma.dat")
}

func fileFirmaEsiste() bool {
	p := fileFirma()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// goSalvaFirma: la firma (PNG in base64) viene protetta con DPAPI: solo questo utente di
// Windows, su questo PC, può rileggerla.
func goSalvaFirma(b64 string) error {
	p := fileFirma()
	if p == "" || piattaforma.proteggi == nil {
		return perPagina(errc("firma_salvata", nil))
	}
	png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(b64, "data:image/png;base64,"))
	if err != nil || len(png) > 8<<20 {
		return perPagina(errc("firma_salvata", err))
	}
	enc, err := piattaforma.proteggi(png)
	if err != nil {
		return perPagina(errc("firma_salvata", err))
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	return perPagina(scriviSicuro(p, enc))
}

func goCaricaFirma() (string, error) {
	p := fileFirma()
	if p == "" || piattaforma.svela == nil {
		return "", nil
	}
	enc, err := os.ReadFile(p)
	if err != nil {
		return "", nil
	}
	png, err := piattaforma.svela(enc)
	if err != nil {
		return "", perPagina(errc("firma_salvata", err))
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

func goCancellaFirma() error {
	if p := fileFirma(); p != "" {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return perPagina(errc("firma_salvata", err))
		}
	}
	return nil
}

// ---------------------------------------------------------------- Pagine interne

// risorsa: risponde alle richieste della pagina. Solo l'indirizzo interno: tutto il resto
// (internet compreso) riceve "non trovato".
func risorsa(indirizzo string) ([]byte, string, bool) {
	u, err := url.Parse(indirizzo)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, hostInterno) {
		return nil, "", false
	}
	p := path.Clean("/" + u.Path)
	if strings.HasPrefix(p, "/file/") {
		return fileInterno(strings.TrimPrefix(p, "/file/"))
	}
	if p == "/" {
		p = "/index.html"
	}
	b, err := fs.ReadFile(uiFS, "ui"+p)
	if err != nil {
		return nil, "", false
	}
	return b, tipoContenuto(p), true
}

func tipoContenuto(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".mjs", ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".ftl", ".txt":
		return "text/plain; charset=utf-8"
	case ".wasm":
		return "application/wasm"
	case ".svg":
		return "image/svg+xml"
	case ".bcmap", ".pfb", ".icc":
		return "application/octet-stream"
	}
	if t := mime.TypeByExtension(path.Ext(p)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// fileInterno: il contenuto di un file aperto, per il visualizzatore e le anteprime.
func fileInterno(id string) ([]byte, string, bool) {
	v, err := prendiVoce(id)
	if err != nil {
		return nil, "", false
	}
	b, err := v.contenuto()
	if err != nil {
		return nil, "", false
	}
	switch v.Tipo {
	case "pdf":
		return b, "application/pdf", true
	case "jpg":
		return b, "image/jpeg", true
	case "png":
		return b, "image/png", true
	case "heic":
		archivio.Lock()
		j := v.jpgHEIC
		archivio.Unlock()
		if j == nil && decodificaHEIC != nil {
			img, err := decodificaHEIC(b)
			if err != nil {
				return nil, "", false
			}
			var buf bytes.Buffer
			if jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}) != nil {
				return nil, "", false
			}
			j = buf.Bytes()
			archivio.Lock()
			v.jpgHEIC = j
			archivio.Unlock()
		}
		if j == nil {
			return nil, "", false
		}
		return j, "image/jpeg", true
	}
	return nil, "", false
}

// navigazionePermessa: la pagina può andare solo alle sue pagine interne.
func navigazionePermessa(u string) bool {
	return strings.HasPrefix(strings.ToLower(u), indirizzoBase) || u == "about:blank"
}

// legami: le funzioni che la pagina può chiamare.
func legami() map[string]interface{} {
	return map[string]interface{}{
		"goAvvio":         goAvvio,
		"goLingua":        goLingua,
		"goScegli":        goScegli,
		"goCaricaInizio":  goCaricaInizio,
		"goCaricaPezzo":   goCaricaPezzo,
		"goCaricaFine":    goCaricaFine,
		"goDimentica":     goDimentica,
		"goInfo":          goInfo,
		"goRipara":        goRipara,
		"goSblocca":       goSblocca,
		"goUnisci":        goUnisci,
		"goFirma":         goFirma,
		"goSalva":         goSalva,
		"goApriLink":      goApriLink,
		"goMostra":        goMostra,
		"goSalvaFirma":    goSalvaFirma,
		"goCaricaFirma":   goCaricaFirma,
		"goCancellaFirma": goCancellaFirma,
		"goSalvaTipo":     goSalvaTipo,
		"goOrganizza":     goOrganizza,
		"goDisegna":       goDisegna,
		"goCollegamenti":  goCollegamenti,
		"goOscura":        goOscura,
		"goProteggi":      goProteggi,
		"goComprimi":      goComprimi,
		"goTestoAdatto":   goTestoAdatto,
		"goCaricaTesto":   goCaricaTesto,
		"goSerieInizio":   goSerieInizio,
		"goSerieScrivi":   goSerieScrivi,
		"goSerieFine":     goSerieFine,
		"goFileAvvio":     goFileAvvio,
		"goMenu":          goMenu,
		"goImpostaMenu":   goImpostaMenu,
		// nome_utente.go: "Il tuo nome", lo stesso di Tasso e Tasso Zip
		"goNome":      goNome,
		"goNomeSalva": goNomeSalva,
		// aggiornamenti.go
		"goControllaAggiornamenti": goControllaAggiornamenti,
		"goPaginaVersioni":         goPaginaVersioni,
	}
}

// Funzioni che aprono finestre di Windows: devono girare nel filo della finestra.
var legamiSuFinestra = []string{"goScegli", "goSalva", "goSalvaTipo", "goSerieInizio"}

// Funzioni veloci che non devono aspettare in coda dietro un lavoro lungo.
var legamiSubito = []string{"goCaricaPezzo", "goDimentica", "goFileAvvio", "goTestoAdatto"}
