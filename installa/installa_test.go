package installa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// pacchetto finto: tre programmi + disinstallatore.
func pacchetto(t *testing.T, ver string) (fstest.MapFS, *Elenco) {
	t.Helper()
	m := fstest.MapFS{}
	e := &Elenco{Versione: ver}
	add := func(id Programma, nome, avvio string, file map[string]string) {
		p := ProgElenco{ID: id, Nome: nome, Versione: ver, Avvio: avvio}
		var nomi []string
		for n := range file {
			nomi = append(nomi, n)
		}
		sort.Strings(nomi)
		for _, n := range nomi {
			c := []byte(file[n])
			h := sha256.Sum256(c)
			m[string(id)+"/"+n] = &fstest.MapFile{Data: c}
			p.File = append(p.File, FileElenco{Nome: n, SHA256: hex.EncodeToString(h[:]), Dim: int64(len(c))})
		}
		e.Programmi = append(e.Programmi, p)
	}
	add(ProgTasso, "Tasso", "Tasso.exe", map[string]string{"Tasso.exe": "tasso " + ver, "VERSIONE": ver, "LICENSE": "gpl"})
	add(ProgZip, "Tasso Zip", "TassoZip.exe", map[string]string{"TassoZip.exe": "zip " + ver, "LEGGIMI.txt": "leggi"})
	add(ProgPDF, "Tasso PDF", "TassoPDF.exe", map[string]string{"TassoPDF.exe": "pdf " + ver})
	add(ProgSetup, "Disinstalla Tasso", NomeDisinstalla, map[string]string{NomeDisinstalla: "disinst " + ver})
	b, _ := json.Marshal(e)
	m[NomeElenco] = &fstest.MapFile{Data: b}
	e2, err := LeggiElenco(m)
	if err != nil {
		t.Fatal(err)
	}
	return m, e2
}

type prova struct {
	in         *Installatore
	reg        *RegistroMemoria
	base, menu string
	desk       string
	coll       map[string]Collegamento
}

func nuovaProva(t *testing.T, ver string) *prova {
	dir := t.TempDir()
	src, e := pacchetto(t, ver)
	p := &prova{reg: NuovoRegistroMemoria(), base: filepath.Join(dir, "Programmi", "Tasso"),
		menu: filepath.Join(dir, "Start"), desk: filepath.Join(dir, "Desktop"), coll: map[string]Collegamento{}}
	os.MkdirAll(p.menu, 0o755)
	os.MkdirAll(p.desk, 0o755)
	p.in = &Installatore{Sorgente: src, Elenco: e, Registro: p.reg,
		Adesso: func() time.Time { return time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC) },
		CreaCollegamento: func(c Collegamento) error {
			p.coll[c.Percorso] = c
			return os.WriteFile(c.Percorso, []byte(c.Destinazione), 0o644)
		}}
	return p
}

func (p *prova) opzioni(cc ...Componente) Opzioni {
	return Opzioni{Base: p.base, Componenti: cc, Lingua: "it", CartellaMenu: p.menu, CartellaDesktop: p.desk}
}

func esiste(p string) bool { _, err := os.Stat(p); return err == nil }

func leggi(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestComponentiEModalita(t *testing.T) {
	c, err := Normalizza([]Componente{"pdf", "Gaming", "pdf", "zip"})
	if err != nil || TestoComponenti(c) != "gaming,zip,pdf" {
		t.Fatalf("normalizza: %v %v", c, err)
	}
	if _, err := Normalizza(nil); err == nil {
		t.Fatal("scelta vuota accettata")
	}
	if _, err := Normalizza([]Componente{"giochi"}); err == nil {
		t.Fatal("componente sconosciuto accettato")
	}
	casi := map[string]string{"gaming,ufficio": "entrambi", "gaming": "gioco", "ufficio,zip": "ufficio", "zip,pdf": ""}
	for in, atteso := range casi {
		if m := ModalitaTasso(LeggiComponenti(in)); m != atteso {
			t.Errorf("%s: modalità %q invece di %q", in, m, atteso)
		}
	}
	pp := ProgrammiPer(LeggiComponenti("ufficio,gaming,pdf"))
	if len(pp) != 2 || pp[0] != ProgTasso || pp[1] != ProgPDF {
		t.Fatalf("programmi: %v", pp)
	}
	if got := Meno(LeggiComponenti("gaming,ufficio,zip"), []Componente{Ufficio}); TestoComponenti(got) != "gaming,zip" {
		t.Fatalf("meno: %v", got)
	}
}

func TestCartelle(t *testing.T) {
	b := filepath.Join("x", "Tasso")
	if CartellaDi(b, ProgZip) != filepath.Join(b, "Tasso Zip") || CartellaDi(b, ProgPDF) != filepath.Join(b, "Tasso PDF") || CartellaDi(b, ProgTasso) != b {
		t.Fatal("cartelle dei programmi sbagliate")
	}
	buone := []string{`C:\Program Files\Tasso`, `D:\Programmi\Tasso 3`, `c:\a\b\`}
	for _, p := range buone {
		if err := ControllaCartellaWin(p, `C:\Windows`); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	cattive := map[string]string{
		``: ErrCartVuota, `Tasso`: ErrCartNonAssol, `\\server\x`: ErrCartNonAssol, `C:\`: ErrCartRadice,
		`C:\a|b`: ErrCartCaratteri, `C:\a\..\b`: ErrCartCaratteri, `C:\Windows\Tasso`: ErrCartWindows,
		`C:\windows`: ErrCartWindows, `C:\con\x`: ErrCartNomeRiserv, `C:\a\nul.txt`: ErrCartNomeRiserv,
		`C:\fine.`: ErrCartCaratteri, `C:\` + strings.Repeat("a", 200): ErrCartLunga,
	}
	for p, cod := range cattive {
		err := ControllaCartellaWin(p, `C:\Windows`)
		var ec *ErroreCartella
		if !errors.As(err, &ec) || ec.Codice != cod {
			t.Errorf("%q: errore %v invece di %s", p, err, cod)
		}
	}
	if !DentroProgrammi(filepath.Join("pf", "Tasso"), "pf") || DentroProgrammi("pf2", "pf") {
		t.Fatal("DentroProgrammi")
	}
	for _, n := range []string{"../x", "/etc/x", `C:\x`, `a\..\b`, ""} {
		if nomeSicuro(n) {
			t.Errorf("nome %q accettato", n)
		}
	}
}

func TestInstallaTutto(t *testing.T) {
	p := nuovaProva(t, "3.1")
	o := p.opzioni(TuttiComponenti...)
	o.Desktop = true
	var fasi []string
	p.in.Avanzamento = func(a Avanzamento) {
		if len(fasi) == 0 || fasi[len(fasi)-1] != a.Fase {
			fasi = append(fasi, a.Fase)
		}
	}
	st, err := p.in.Installa(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fasi, ",") != "prepara,copia,sposta,registro,collegamenti,fine" {
		t.Errorf("fasi: %v", fasi)
	}
	for _, f := range []string{"Tasso.exe", "VERSIONE", "LICENSE", NomeDisinstalla, FileStato,
		filepath.Join("Tasso Zip", "TassoZip.exe"), filepath.Join("Tasso PDF", "TassoPDF.exe")} {
		if !esiste(filepath.Join(p.base, f)) {
			t.Errorf("manca %s", f)
		}
	}
	if esiste(filepath.Join(p.base, cartellaNuovi)) || esiste(filepath.Join(p.base, cartellaVecchi)) {
		t.Error("cartelle temporanee rimaste")
	}
	if leggi(t, filepath.Join(p.base, "VERSIONE")) != "3.1" {
		t.Error("VERSIONE sbagliato")
	}
	// registro
	r := p.reg
	chk := func(rad Radice, k, n, atteso string) {
		t.Helper()
		if v, _ := r.LeggiTesto(rad, k, n); v != atteso {
			t.Errorf("%s\\%s = %q invece di %q", k, n, v, atteso)
		}
	}
	chk(HKLM, ChiaveDisinstalla, "DisplayName", "Tasso")
	chk(HKLM, ChiaveDisinstalla, "Publisher", "Sergio Tassoni")
	chk(HKLM, ChiaveDisinstalla, "DisplayIcon", filepath.Join(p.base, "Tasso.exe")+",0")
	chk(HKLM, ChiaveDisinstalla, "UninstallString", `"`+filepath.Join(p.base, NomeDisinstalla)+`"`)
	chk(HKLM, ChiaveDisinstalla, "InstallDate", "20261124")
	chk(HKLM, ChiaveTasso, "Modalita", "entrambi")
	chk(HKLM, ChiaveTasso, "Componenti", "gaming,ufficio,zip,pdf")
	chk(HKCU, ChiaveTasso, "Lingua", "it")
	if n, ok := r.LeggiNumero(HKLM, ChiaveDisinstalla, "EstimatedSize"); !ok || n == 0 {
		t.Error("EstimatedSize")
	}
	// collegamenti: 3 nel menu + 3 sul desktop
	if len(p.coll) != 6 || len(st.Collegamenti) != 6 {
		t.Fatalf("collegamenti: %d %d", len(p.coll), len(st.Collegamenti))
	}
	c := p.coll[filepath.Join(p.menu, "Tasso Zip.lnk")]
	if c.Destinazione != filepath.Join(p.base, "Tasso Zip", "TassoZip.exe") || c.Cartella != filepath.Join(p.base, "Tasso Zip") {
		t.Errorf("collegamento zip: %+v", c)
	}
	// lo stato riletto dal disco
	st2, err := LeggiStato(p.base)
	if err != nil || TestoComponenti(st2.Componenti) != "gaming,ufficio,zip,pdf" || len(st2.Programmi) != 4 {
		t.Fatalf("stato: %+v %v", st2, err)
	}
}

func TestSoloUfficio(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Ufficio)); err != nil {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(p.base, "Tasso.exe")) || esiste(filepath.Join(p.base, "Tasso Zip")) || esiste(filepath.Join(p.base, "Tasso PDF")) {
		t.Fatal("file sbagliati")
	}
	if m, _ := p.reg.LeggiTesto(HKLM, ChiaveTasso, "Modalita"); m != "ufficio" {
		t.Fatalf("modalità %q", m)
	}
	if len(p.coll) != 1 { // niente desktop
		t.Fatalf("collegamenti %v", p.coll)
	}
}

func TestSoloZipIconaDisinstallatore(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayIcon"); v != filepath.Join(p.base, NomeDisinstalla)+",0" {
		t.Fatalf("icona %q", v)
	}
	if _, ok := p.reg.LeggiTesto(HKLM, ChiaveTasso, "Modalita"); ok {
		t.Fatal("Modalita senza Tasso")
	}
	if esiste(filepath.Join(p.base, "Tasso.exe")) {
		t.Fatal("Tasso.exe non scelto ma installato")
	}
}

func TestAggiornamento(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	// file di Tasso che non sono dell'installatore: devono restare
	lavoro := filepath.Join(p.base, "lavoro", "dati.txt")
	os.MkdirAll(filepath.Dir(lavoro), 0o755)
	os.WriteFile(lavoro, []byte("di Tasso"), 0o644)

	// versione nuova: Zip non ha più LEGGIMI.txt; si aggiunge PDF, non si sceglie Zip
	src, e := pacchetto(t, "3.1")
	zp := e.Prog(ProgZip)
	zp.File = zp.File[1:] // toglie LEGGIMI.txt (ordinati: LEGGIMI.txt, TassoZip.exe)
	p.in.Sorgente, p.in.Elenco = src, e
	st, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Ufficio, PDF))
	if err != nil {
		t.Fatal(err)
	}
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.1" || leggi(t, lavoro) != "di Tasso" {
		t.Fatal("aggiornamento di Tasso sbagliato")
	}
	// Zip non scelto: resta la versione vecchia, con il suo LEGGIMI
	if leggi(t, filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) != "zip 3.0" || !esiste(filepath.Join(p.base, "Tasso Zip", "LEGGIMI.txt")) {
		t.Fatal("Zip non doveva cambiare")
	}
	if TestoComponenti(st.Componenti) != "gaming,ufficio,zip,pdf" {
		t.Fatalf("componenti %v", st.Componenti)
	}
	// ora aggiorno anche Zip: il LEGGIMI che non c'è più va via
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.base, "Tasso Zip", "LEGGIMI.txt")) || leggi(t, filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) != "zip 3.1" {
		t.Fatal("file vecchio di Zip non tolto")
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayVersion"); v != "3.1" {
		t.Fatalf("DisplayVersion %q", v)
	}
}

func TestAnnulla(t *testing.T) {
	p := nuovaProva(t, "3.1")
	ctx, annulla := context.WithCancel(context.Background())
	n := 0
	p.in.Avanzamento = func(a Avanzamento) {
		if a.Fase == "copia" {
			n++
			if n == 3 {
				annulla()
			}
		}
	}
	_, err := p.in.Installa(ctx, p.opzioni(TuttiComponenti...))
	if !errors.Is(err, ErrAnnullato) {
		t.Fatalf("errore %v", err)
	}
	if esiste(p.base) {
		t.Fatal("annullando, la cartella nuova doveva sparire")
	}
	if len(p.reg.Chiavi()) != 0 || len(p.coll) != 0 {
		t.Fatal("annullando non si doveva scrivere niente")
	}
}

func TestAnnullaAggiornamentoLasciaVecchio(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	src, e := pacchetto(t, "3.1")
	p.in.Sorgente, p.in.Elenco = src, e
	ctx, annulla := context.WithCancel(context.Background())
	p.in.Avanzamento = func(a Avanzamento) {
		if a.Fase == "copia" && a.Fatto > 0 {
			annulla()
		}
	}
	if _, err := p.in.Installa(ctx, p.opzioni(TuttiComponenti...)); !errors.Is(err, ErrAnnullato) {
		t.Fatalf("errore %v", err)
	}
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.0" || esiste(filepath.Join(p.base, cartellaNuovi)) {
		t.Fatal("annullare l'aggiornamento doveva lasciare la versione vecchia")
	}
}

func TestRinominaSbagliataTornaIndietro(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	src, e := pacchetto(t, "3.1")
	p.in.Sorgente, p.in.Elenco = src, e
	// TassoPDF.exe "aperto": non si può spostare
	p.in.Rinomina = func(a, b string) error {
		if strings.HasSuffix(a, filepath.Join("Tasso PDF", "TassoPDF.exe")) && strings.Contains(b, cartellaVecchi) {
			return errors.New("file in uso")
		}
		return os.Rename(a, b)
	}
	_, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...))
	if err == nil || !strings.Contains(err.Error(), "in uso") {
		t.Fatalf("errore %v", err)
	}
	// tutto come prima: Tasso e Zip (già spostati) tornati alla 3.0
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.0" || leggi(t, filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) != "zip 3.0" {
		t.Fatal("non è tornato indietro")
	}
	if esiste(filepath.Join(p.base, cartellaNuovi)) || esiste(filepath.Join(p.base, cartellaVecchi)) {
		t.Fatal("cartelle temporanee rimaste")
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayVersion"); v != "3.0" {
		t.Fatalf("registro cambiato: %q", v)
	}
}

func TestRegistroSbagliatoTornaIndietro(t *testing.T) {
	p := nuovaProva(t, "3.1")
	p.reg.Sbaglia = func(op, k, n string) error {
		if op == "scrivi" && n == "Componenti" {
			return errors.New("accesso negato")
		}
		return nil
	}
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err == nil {
		t.Fatal("atteso errore")
	}
	p.reg.Sbaglia = nil
	if esiste(filepath.Join(p.base, "Tasso.exe")) || esiste(p.base) {
		t.Fatal("i file dovevano sparire")
	}
	if len(p.reg.Chiavi()) != 0 {
		t.Fatalf("registro non pulito: %v", p.reg.Chiavi())
	}
}

func TestVerificaSHA(t *testing.T) {
	p := nuovaProva(t, "3.1")
	p.in.Elenco.Prog(ProgZip).File[1].SHA256 = strings.Repeat("0", 64)
	_, err := p.in.Installa(context.Background(), p.opzioni(Zip))
	if !errors.Is(err, ErrVerifica) {
		t.Fatalf("errore %v", err)
	}
	if esiste(p.base) {
		t.Fatal("niente doveva restare")
	}
}

func TestRipara(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	// PC spento a metà: Tasso.exe spostato tra i vecchi e il nuovo non ancora al suo posto
	v := filepath.Join(p.base, cartellaVecchi, "tasso", "Tasso.exe")
	os.MkdirAll(filepath.Dir(v), 0o755)
	os.Rename(filepath.Join(p.base, "Tasso.exe"), v)
	os.MkdirAll(filepath.Join(p.base, cartellaNuovi, "tasso"), 0o755)
	p.in.Ripara(p.base)
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.0" || esiste(filepath.Join(p.base, cartellaVecchi)) || esiste(filepath.Join(p.base, cartellaNuovi)) {
		t.Fatal("Ripara non ha rimesso il file")
	}
}

func TestDisinstallaSelettiva(t *testing.T) {
	p := nuovaProva(t, "3.1")
	o := p.opzioni(TuttiComponenti...)
	o.Desktop = true
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	lavoro := filepath.Join(p.base, "lavoro", "dati.txt")
	os.MkdirAll(filepath.Dir(lavoro), 0o755)
	os.WriteFile(lavoro, []byte("di Tasso"), 0o644)
	altro := filepath.Join(p.base, "Tasso Zip", "mio.txt") // file non dell'installatore
	os.WriteFile(altro, []byte("mio"), 0o644)

	// 1. via Gaming: Tasso resta, modalità ufficio
	r, err := p.in.Disinstalla(p.base, []Componente{Gaming})
	if err != nil || r.TuttoTolto {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(p.base, "Tasso.exe")) || !esiste(filepath.Join(p.menu, "Tasso Ufficio.lnk")) || esiste(filepath.Join(p.menu, "Tasso Gaming.lnk")) {
		t.Fatal("Tasso doveva restare (c'è ancora Ufficio) col collegamento \"Tasso Ufficio\"")
	}
	if m, _ := p.reg.LeggiTesto(HKLM, ChiaveTasso, "Modalita"); m != "ufficio" {
		t.Fatalf("modalità %q", m)
	}
	// 2. via Zip: i suoi file e collegamenti, non il file mio.txt
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) || esiste(filepath.Join(p.menu, "Tasso Zip.lnk")) || esiste(filepath.Join(p.desk, "Tasso Zip.lnk")) {
		t.Fatal("Zip non tolto")
	}
	if !esiste(altro) || !esiste(filepath.Join(p.menu, "Tasso PDF.lnk")) {
		t.Fatal("tolto qualcosa di troppo")
	}
	// 3. via Ufficio: ora Tasso.exe va via, la cartella lavoro resta
	if _, err := p.in.Disinstalla(p.base, []Componente{Ufficio}); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.base, "Tasso.exe")) || esiste(filepath.Join(p.base, "VERSIONE")) || !esiste(lavoro) {
		t.Fatal("Tasso tolto male")
	}
	if _, ok := p.reg.LeggiTesto(HKLM, ChiaveTasso, "Modalita"); ok {
		t.Fatal("Modalita rimasta senza Tasso")
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayIcon"); !strings.Contains(v, NomeDisinstalla) {
		t.Fatalf("DisplayIcon %q", v)
	}
	// 4. via l'ultimo: tutto
	p.reg.ScriviTesto(HKCU, ChiaveTasso+`\AudioUSB`, "x", "di Tasso")
	r, err = p.in.Disinstalla(p.base, []Componente{PDF})
	if err != nil || !r.TuttoTolto {
		t.Fatal("doveva togliere tutto", err)
	}
	for _, f := range []string{NomeDisinstalla, FileStato, "Tasso PDF"} {
		if esiste(filepath.Join(p.base, f)) {
			t.Errorf("rimasto %s", f)
		}
	}
	if !esiste(lavoro) || !esiste(altro) {
		t.Fatal("cancellati file non dell'installatore")
	}
	if _, ok := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayName"); ok {
		t.Fatal("voce di Programmi e funzionalità rimasta")
	}
	if _, ok := p.reg.LeggiTesto(HKCU, ChiaveTasso, "Lingua"); ok {
		t.Fatal("Lingua rimasta")
	}
	if v, _ := p.reg.LeggiTesto(HKCU, ChiaveTasso+`\AudioUSB`, "x"); v != "di Tasso" {
		t.Fatal("tolta una chiave di Tasso")
	}
	if len(p.coll) == 0 {
		t.Fatal("prova sbagliata")
	}
	for c := range p.coll {
		if esiste(c) {
			t.Errorf("collegamento rimasto %s", c)
		}
	}
}

func TestDisinstallaTuttoCartellaVuota(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	var tolti []string
	p.in.TogliFile = func(f string) error { tolti = append(tolti, f); return os.Remove(f) }
	r, err := p.in.Disinstalla(p.base, TuttiComponenti)
	if err != nil || !r.TuttoTolto || len(r.NonTolti) != 0 {
		t.Fatal(r, err)
	}
	if esiste(p.base) {
		t.Fatal("la cartella vuota doveva sparire")
	}
	if len(p.reg.Chiavi()) != 0 {
		t.Fatalf("registro: %v", p.reg.Chiavi())
	}
	trovato := false
	for _, f := range tolti {
		trovato = trovato || filepath.Base(f) == NomeDisinstalla
	}
	if !trovato {
		t.Fatal("il disinstallatore doveva passare da TogliFile")
	}
}

func TestStatoNonSiFidaDeiPercorsi(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	// qualcuno cambia tasso-setup.json per far cancellare un file fuori
	fuori := filepath.Join(filepath.Dir(p.base), "importante.txt")
	os.WriteFile(fuori, []byte("x"), 0o644)
	st, _ := LeggiStato(p.base)
	zp := st.Programmi[ProgZip]
	zp.Cartella = filepath.Dir(p.base)
	zp.File = append(zp.File, FileElenco{Nome: "../../importante.txt"}, FileElenco{Nome: "../importante.txt"})
	st.Programmi[ProgZip] = zp
	b, _ := st.dati()
	os.WriteFile(filepath.Join(p.base, FileStato), b, 0o644)
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if !esiste(fuori) {
		t.Fatal("cancellato un file fuori dalla cartella")
	}
}

func TestElencoCattivo(t *testing.T) {
	casi := []string{
		`{"programmi":[{"id":"virus","file":[{"nome":"a","sha256":"` + strings.Repeat("a", 64) + `"}]}]}`,
		`{"programmi":[{"id":"zip","file":[{"nome":"../a","sha256":"` + strings.Repeat("a", 64) + `"}]}]}`,
		`{"programmi":[{"id":"zip","file":[{"nome":"a","sha256":"xyz"}]}]}`,
		`{"programmi":[{"id":"zip","file":[]}]}`,
		`non json`,
	}
	for _, c := range casi {
		if _, err := LeggiElenco(fstest.MapFS{NomeElenco: &fstest.MapFile{Data: []byte(c)}}); err == nil {
			t.Errorf("elenco accettato: %s", c)
		}
	}
	if _, err := LeggiElenco(fstest.MapFS{}); err == nil {
		t.Error("elenco mancante accettato")
	}
}

// Installare (o aggiornare in silenzio) solo Zip non deve togliere il collegamento di Tasso;
// reinstallare solo Ufficio con Gaming già presente lascia "Tasso Gaming".
func TestCollegamentoTassoRestaInstallandoAltro(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	gaming := filepath.Join(p.menu, "Tasso Gaming.lnk")
	if !esiste(gaming) {
		t.Fatal("manca Tasso Gaming")
	}
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	if !esiste(gaming) {
		t.Fatal("collegamento di Tasso tolto installando solo Zip")
	}
	st, _ := LeggiStato(p.base)
	trovato := false
	for _, c := range st.Collegamenti {
		trovato = trovato || strings.EqualFold(c.Percorso, gaming)
	}
	if !trovato {
		t.Fatal("collegamento di Tasso sparito da tasso-setup.json")
	}
	if _, err := p.in.Installa(context.Background(), p.opzioni(Ufficio)); err != nil {
		t.Fatal(err)
	}
	if !esiste(gaming) || esiste(filepath.Join(p.menu, "Tasso Ufficio.lnk")) {
		t.Fatal("con Gaming installato il collegamento deve restare \"Tasso Gaming\"")
	}
}
