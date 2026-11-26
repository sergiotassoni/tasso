//go:build windows && collaudo

package traduce

// Collaudo dal vivo: Tasso-collaudo.exe --collaudo "C:\cartella" [--anche-meta 200]
//
// Apre il programma vero e aspetta che Mozilla sia pronto dentro WebView2. Poi, senza toccare
// la memoria delle traduzioni né le librerie dei giochi dell'utente:
//  1. traduce le 1000 frasi di prova con la stessa strada del programma (pulizia, glossario,
//     Mozilla vero dentro WebView2) e misura i tempi;
//  2. sceglie da solo il modo dell'IA con le regole di Sergio (memoria video libera, RAM, core);
//  3. accende il correttore in sottofondo vero (coda di 300, pausa se il processore è pieno)
//     e aspetta che abbia ricontrollato tutte le frasi; misura memoria video e tempi;
//  4. con --anche-meta N rifà le prime N frasi con il modello diviso tra scheda e processore;
//  5. prova i casi noti (termini come "Elmo di ferro") e il grassetto con l'OCR vero di Windows.
//
// Scrive collaudo.md e collaudo.json nella cartella e chiude il programma.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	// prima di questi init: se Tasso è partito dall'attività Apri Tasso, le prove non vedono gli
	// argomenti di chi l'ha avviata (apertura/collaudo.go)
	_ "tasso/apertura"
)

var (
	cartellaCollaudo  string
	frasiMetaCollaudo int
	collaudoAvviato   atomic.Bool
)

func leggiArgomentiCollaudo(args []string) {
	leggiArgomentiNuove(args)
	for i, a := range args {
		if a == "--collaudo" && i+1 < len(args) {
			cartellaCollaudo = args[i+1]
		}
		if a == "--anche-meta" && i+1 < len(args) {
			fmt.Sscan(args[i+1], &frasiMetaCollaudo)
		}
	}
}

func init() {
	// Senza questa chiamata --collaudo, --anche-meta, --solo-nuove e --voci-prova non
	// facevano niente: la funzione c'era ma nessuno la chiamava.
	leggiArgomentiCollaudo(os.Args)
	salvaAnteprime = true
	cartellaWebView = "WebView2-prova" // il motore delle prove usa una cartella dati sua (Prepara)
}

func avviaCollaudoSeServe() {
	// --prova1000 "cartella" [--correttore] [--ngl N]: confronto dei modelli IA sulle frasi di
	// prova. Parte qui, quando il traduttore veloce (Mozilla) è pronto: serve per le bozze.
	for _, a := range os.Args {
		if (a == "--prova100" || a == "--prova1000") && collaudoAvviato.CompareAndSwap(false, true) {
			go func() {
				defer guard("prova-modelli")
				modalitaProva(os.Args)
			}()
			return
		}
	}
	if provaFileSeServe() {
		return
	}
	if provaLingueSeServe() {
		return
	}
	if provaSottotitoliSeServe() {
		return
	}
	if cartellaFoto != "" && collaudoAvviato.CompareAndSwap(false, true) {
		go eseguiFoto()
		return
	}
	if cartellaCollaudo != "" && collaudoAvviato.CompareAndSwap(false, true) {
		go eseguiCollaudo()
	}
}

type motoreCronometrato struct {
	m     Motore
	mu    sync.Mutex
	tempi []float64
}

func (c *motoreCronometrato) Nome() string { return c.m.Nome() }
func (c *motoreCronometrato) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	t0 := time.Now()
	s, err := c.m.Traduci(ctx, r)
	c.mu.Lock()
	c.tempi = append(c.tempi, float64(time.Since(t0).Microseconds())/1000)
	c.mu.Unlock()
	return s, err
}

func (c *motoreCronometrato) statistiche() (mediana, p90, massimo float64, n int) {
	c.mu.Lock()
	t := append([]float64(nil), c.tempi...)
	c.mu.Unlock()
	sort.Float64s(t)
	if n = len(t); n > 0 {
		mediana, p90, massimo = t[n/2], t[n*9/10], t[n-1]
	}
	return
}

type EsitoCorrettore struct {
	Modo          string  `json:"modo"`
	Strati        string  `json:"strati"`
	Frasi         int     `json:"frasi"`
	ChrFMozilla   float64 `json:"chrf_mozilla"`
	ChrFCorretto  float64 `json:"chrf_corretto"`
	Ricontrollate int64   `json:"ricontrollate"`
	Cambiate      int64   `json:"cambiate"`
	Migliori      int     `json:"migliori"`
	Peggiori      int     `json:"peggiori"`
	MedianaMs     float64 `json:"mediana_ms"`
	P90Ms         float64 `json:"p90_ms"`
	MassimoMs     float64 `json:"max_ms"`
	VRAMGB        float64 `json:"vram_gb"`
	Pause         int64   `json:"pause_cpu"`
	CPUMax        float64 `json:"cpu_gioco_max"`
	Durata        string  `json:"durata"`
	Note          string  `json:"note"`
	Esempi        [][3]string
	Uscite        []string `json:"uscite"`
}

type CasoNoto struct {
	Inglese string `json:"inglese"`
	Vuole   string `json:"vuole"`
	Mozilla string `json:"mozilla"`
	ConIA   string `json:"con_ia"`
	OK      bool   `json:"ok"`
}

type EsitoGrassetto struct {
	Testo     string  `json:"testo"`
	Grassetto bool    `json:"grassetto_vero"`
	Visto     bool    `json:"grassetto_visto"`
	Tratto    float64 `json:"tratto"`
	Letto     string  `json:"letto"`
}

type Collaudo struct {
	Data       string            `json:"data"`
	PC         map[string]string `json:"pc"`
	Mozilla    EsitoProva        `json:"mozilla"`
	Regola     string            `json:"regola"`
	Correttori []EsitoCorrettore `json:"correttori"`
	Casi       []CasoNoto        `json:"casi"`
	Grassetto  []EsitoGrassetto  `json:"grassetto"`
	Errori     []string          `json:"errori"`
}

func eseguiCollaudo() {
	defer guard("collaudo")
	defer func() {
		if wv != nil {
			wv.Dispatch(func() { wv.Terminate() })
		}
	}()
	inizio := time.Now()
	_ = os.MkdirAll(cartellaCollaudo, 0o755)
	var c Collaudo
	c.Data = time.Now().Format("02/01/2006 15:04")
	scrivi := func() {
		b, _ := json.MarshalIndent(c, "", "  ")
		_ = os.WriteFile(filepath.Join(cartellaCollaudo, "collaudo.json"), b, 0o644)
		_ = os.WriteFile(filepath.Join(cartellaCollaudo, "collaudo.md"), []byte(rapportoCollaudo(c, time.Since(inizio))), 0o644)
	}
	defer scrivi()
	if soloNuove {
		collaudoNuove(cartellaCollaudo)
		return
	}

	go sorvegliaCPU()
	g0 := controllaGPU()
	ram, core := memoriaPC(), coreFisici()
	c.PC = map[string]string{
		"scheda":         fmt.Sprintf("%s, %.1f GB (usati adesso %.1f GB)", g0.Nome, g0.MemoriaGB, g0.UsataGB),
		"ram":            fmt.Sprintf("%.0f GB installati, %.1f GB liberi", ram, ramLibera()),
		"core":           fmt.Sprintf("%d core, %d processori logici", core, runtime.NumCPU()),
		"ia_installata":  fmt.Sprint(iaPresente()),
		"glossario_voci": fmt.Sprint(len(glossarioBase().conGiochi().Termini)),
	}

	frasi, err := caricaFrasiProva()
	if err != nil {
		c.Errori = append(c.Errori, "frasi: "+err.Error())
		return
	}

	// 1. Mozilla dal vivo con la strada del programma
	cm := &motoreCronometrato{m: moz}
	tm := NuovoTraduttore("", glossarioBase(), cm)
	es := EsitoProva{Motore: "Mozilla dal vivo (WebView2 su questo PC, strada completa del programma)", Data: c.Data}
	genere := ""
	for _, f := range frasi {
		if f.Genere != genere {
			tm.mu.Lock()
			tm.contesto = nil
			tm.mu.Unlock()
			genere = f.Genere
		}
		t0 := time.Now()
		r := tm.Traduci(context.Background(), f.EN)
		ms := float64(time.Since(t0).Microseconds()) / 1000
		out := r.Tradotto
		if r.Errore != "" {
			out = "[errore: " + r.Errore + "]"
			c.Errori = append(c.Errori, fmt.Sprintf("Mozilla frase %d: %s", f.ID, r.Errore))
		}
		if r.Parlante != "" {
			out = r.Parlante + ": " + out
		}
		es.Frasi = append(es.Frasi, EsitoFrase{FraseProva: f, Uscita: out, Ms: ms})
	}
	calcolaPunteggi(&es)
	c.Mozilla = es
	scrivi()

	// 2. regola automatica
	p := sceltaCorrettore(g0, ram, core)
	c.Regola = fmt.Sprintf("Con %.1f GB liberi sulla scheda, %.0f GB di RAM e %d core la regola sceglie: %q. %s", g0.MemoriaGB-g0.UsataGB, ram, core, nomeModo(p.Nome), p.Motivo)
	if !iaPresente() {
		c.Errori = append(c.Errori, "IA non installata in "+dirIA()+": correttore non provato")
	} else {
		modi := []ProfiloCorrettore{}
		if p.Nome != "" {
			modi = append(modi, p)
		}
		if frasiMetaCollaudo > 0 && p.Nome != "meta" {
			modi = append(modi, ProfiloCorrettore{Nome: "meta", Strati: stratiParziali})
		}
		for _, m := range modi {
			n := len(frasi)
			if m.Nome == "meta" && frasiMetaCollaudo > 0 && m.Nome != p.Nome {
				n = min(n, frasiMetaCollaudo)
			}
			e := collaudaCorrettore(m, frasi[:n], es.Frasi[:n])
			if len(c.Correttori) > 0 && len(c.Correttori[0].Uscite) >= n && n < len(frasi) {
				var ref []string
				for _, f := range frasi[:n] {
					ref = append(ref, f.IT)
				}
				e.Note += fmt.Sprintf("Sulle stesse %d frasi l'IA tutta sulla scheda fa %.1f.", n, chrFpp(c.Correttori[0].Uscite[:n], ref))
			}
			c.Correttori = append(c.Correttori, e)
			scrivi()
		}
	}

	// 4. casi noti
	c.Casi = casiNoti(tm)
	scrivi()
	// 5. grassetto con l'OCR vero
	c.Grassetto, err = collaudoGrassetto()
	if err != nil {
		c.Errori = append(c.Errori, "grassetto: "+err.Error())
	}
}

func nomeModo(n string) string {
	switch n {
	case "scheda":
		return "IA tutta sulla scheda video"
	case "meta":
		return "IA divisa tra scheda video e processore"
	}
	return "solo Mozilla"
}

func collaudaCorrettore(p ProfiloCorrettore, frasi []FraseProva, base []EsitoFrase) EsitoCorrettore {
	t0 := time.Now()
	e := EsitoCorrettore{Modo: nomeModo(p.Nome), Frasi: len(frasi)}
	if p.Strati > 0 {
		e.Strati = fmt.Sprintf("%d su %d", p.Strati, stratiModello)
	} else {
		e.Strati = "tutti"
	}
	ia.Ferma()
	prima := controllaGPU()
	ia.impostaProfilo(p)
	if err := ia.Avvia(dirMotoreIA(), fileModelloIA()); err != nil {
		e.Note = "IA non avviata: " + err.Error()
		return e
	}
	defer ia.Ferma()
	_, _ = motoreIA{ia}.Traduci(context.Background(), RichiestaTraduzione{Testo: "Hello.", Bozza: "Ciao."})
	dopo := controllaGPU()
	e.VRAMGB = dopo.UsataGB - prima.UsataGB

	cm := &motoreCronometrato{m: moz}
	t := NuovoTraduttore("", glossarioBase(), cm)
	ci := &motoreCronometrato{m: motoreIA{ia}}
	var pause atomic.Int64
	var cpuMax atomic.Int64
	t.ImpostaCorrettore(ci, func() bool {
		if v := cpuGioco.Load(); v > cpuMax.Load() {
			cpuMax.Store(v)
		}
		if cpuOccupata.Load() {
			pause.Add(1)
			return true
		}
		return false
	})
	// come nel gioco: le frasi arrivano una dopo l'altra; se la coda è quasi piena aspetto
	// (in un gioco vero le frasi arrivano molto più piano di così)
	for _, f := range frasi {
		for {
			_, _, attesa := t.StatoCorrezioni()
			if attesa < maxCodaCorrezioni-10 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Traduci(context.Background(), f.EN)
	}
	for {
		_, _, attesa := t.StatoCorrezioni()
		if attesa == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.ImpostaCorrettore(nil, nil)
	fatte, cambiate, _ := t.StatoCorrezioni()
	e.Ricontrollate, e.Cambiate = fatte, cambiate

	var hypM, hypC, ref []string
	for i, f := range frasi {
		parlante, battuta := separaParlante(pulisci(f.EN))
		t.mu.Lock()
		corr, inCache := t.cache[parlante+"\x00"+battuta]
		t.mu.Unlock()
		if !inCache { // frase uscita dal catalogo o dal glossario: non passa dal correttore, resta quella di Mozilla
			corr = strings.TrimSpace(strings.TrimPrefix(base[i].Uscita, parlante+":"))
		}
		if parlante != "" {
			corr = parlante + ": " + corr
		}
		hypM = append(hypM, base[i].Uscita)
		hypC = append(hypC, corr)
		ref = append(ref, f.IT)
		if corr != base[i].Uscita {
			a := chrFpp([]string{base[i].Uscita}, []string{f.IT})
			b := chrFpp([]string{corr}, []string{f.IT})
			switch {
			case b > a:
				e.Migliori++
			case b < a:
				e.Peggiori++
			}
			if len(e.Esempi) < 25 {
				e.Esempi = append(e.Esempi, [3]string{f.EN, base[i].Uscita, corr})
			}
		}
	}
	e.ChrFMozilla, e.ChrFCorretto = chrFpp(hypM, ref), chrFpp(hypC, ref)
	e.Uscite = hypC
	e.MedianaMs, e.P90Ms, e.MassimoMs, _ = ci.statistiche()
	e.Pause = pause.Load()
	e.CPUMax = float64(cpuMax.Load()) / 10
	e.Durata = time.Since(t0).Round(time.Second).String()
	return e
}

// casiNoti: errori visti nelle prove (es. "Iron Helmet" diventava "Casco di ferro").
var elencoCasiNoti = [][2]string{
	{"Iron Helmet", "Elmo di ferro"},
	{"IRON HELMET", "ELMO DI FERRO"},
	{"Steel Sword", "Spada d'acciaio"},
	{"Leather Boots", "Stivali di cuoio"},
	{"Health Potion x3", "Pozione di salute x3"},
	{"You found an Iron Helmet.", "elmo di ferro"},
	{"Equip the Iron Helmet to increase your defense.", "elmo di ferro"},
	{"The blacksmith sells helmets and shields.", "elmi"},
	{"Your helmet is broken.", "elmo"},
	{"Press E to open the inventory.", "inventario"},
	{"Quest completed!", "Missione completata"},
	{"Achievement unlocked", "Obiettivo sbloccato"},
	{"Autosave in progress. Do not turn off your console.", "Salvataggio automatico"},
	{"Hold Shift to sprint.", "Tieni premuto"},
	{"Crouch", "Accovacciati"},
	{"Skill tree", "Albero delle abilità"},
}

func casiNoti(t *Traduttore) []CasoNoto {
	var out []CasoNoto
	for _, k := range elencoCasiNoti {
		var r Risultato
		if len(strings.Fields(k[0])) <= 3 && !strings.ContainsAny(k[0], ".!?") {
			r = t.TraduciScritta(context.Background(), k[0])
		} else {
			r = t.Traduci(context.Background(), k[0])
		}
		ok := strings.Contains(strings.ToLower(r.Tradotto), strings.ToLower(k[1]))
		if k[1] == strings.ToUpper(k[1]) || k[0] == "Iron Helmet" {
			ok = strings.Contains(r.Tradotto, k[1])
		}
		caso := CasoNoto{Inglese: k[0], Vuole: k[1], Mozilla: r.Tradotto + "  [" + r.Motore + "]", OK: ok}
		if ia.Attivo() || iaPresente() {
			if !ia.Attivo() {
				ia.impostaProfilo(ProfiloCorrettore{Nome: "scheda", Strati: -1})
				_ = ia.Avvia(dirMotoreIA(), fileModelloIA())
			}
			if r.Motore == "Mozilla" {
				_, battuta := separaParlante(pulisci(k[0]))
				g := glossarioBase().conGiochi().trova(battuta)
				if corr, err := (motoreIA{ia}).Traduci(context.Background(), RichiestaTraduzione{Testo: battuta, Glossario: g, Bozza: r.Tradotto}); err == nil {
					corr = sigleItaliane(corr, g)
					if correzioneAccettabile(battuta, r.Tradotto, corr) {
						caso.ConIA = corr
					} else {
						caso.ConIA = "(scartata) " + corr
					}
				}
			}
		}
		out = append(out, caso)
	}
	ia.Ferma()
	return out
}

// collaudoGrassetto: scrive con Windows (GDI) scritte normali e in grassetto, le fa leggere
// all'OCR vero e controlla che il programma riconosca quali sono in grassetto.
func collaudoGrassetto() ([]EsitoGrassetto, error) {
	casi := []struct {
		testo string
		bold  bool
		px    int32
	}{
		{"New Game", true, 40}, {"Continue", false, 40}, {"Options", true, 32}, {"Credits", false, 32},
		{"Inventory", true, 28}, {"Quit to desktop", false, 28}, {"Your journey begins", true, 36}, {"Load saved game", false, 36},
	}
	const W, H = 1280, 720
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{24, 28, 40, 255}}, image.Point{}, draw.Src)
	type pos struct{ x, y int }
	var posti []pos
	for i, k := range casi {
		x, y := 80+(i%2)*600, 60+(i/2)*150
		posti = append(posti, pos{x, y})
		if err := scriviTestoGDI(img, k.testo, x, y, k.px, k.bold); err != nil {
			return nil, err
		}
	}
	if ocr == nil {
		l, err := avviaOCR(filepath.Join(dirApp, "lavoro"))
		if err != nil {
			return nil, err
		}
		ocr = l
	}
	var b bytes.Buffer
	f, err := preparaPaginaPerOCR(img, &b)
	if err != nil {
		return nil, err
	}
	righe, err := ocr.LeggiPosizioni(b.Bytes())
	if err != nil {
		return nil, err
	}
	blocchi := unisciParagrafi(segmenti(righe, f))
	var out []EsitoGrassetto
	for i, k := range casi {
		e := EsitoGrassetto{Testo: k.testo, Grassetto: k.bold}
		for _, bl := range blocchi {
			if bl.X <= posti[i].x+40 && bl.X+bl.W >= posti[i].x && bl.Y <= posti[i].y+int(k.px) && bl.Y+bl.H >= posti[i].y {
				coloriBlocco(img, &bl)
				e.Letto, e.Tratto, e.Visto = bl.Testo, bl.Tratto, bl.grassetto()
				break
			}
		}
		out = append(out, e)
	}
	_ = salvaPNG(img, filepath.Join(cartellaCollaudo, "grassetto-prova.png"))
	return out, nil
}

var (
	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pTextOutW           = gdi32.NewProc("TextOutW")
)

// scriviTestoGDI disegna un testo bianco con Segoe UI (normale o grassetto) dentro img.
func scriviTestoGDI(img *image.RGBA, testo string, x, y int, px int32, bold bool) error {
	w, h := 560, int(px)*2
	dc, _, _ := pCreateCompatibleDC.Call(0)
	if dc == 0 {
		return fmt.Errorf("CreateCompatibleDC")
	}
	defer pDeleteDC.Call(dc)
	type bih struct {
		Size                                uint32
		Width, Height                       int32
		Planes, BitCount                    uint16
		Compression, SizeImage              uint32
		XPels, YPels, ClrUsed, ClrImportant int32
	}
	hdr := bih{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&hdr)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return fmt.Errorf("CreateDIBSection")
	}
	defer pDeleteObject.Call(bmp)
	pSelectObject.Call(dc, bmp)
	peso := uintptr(400)
	if bold {
		peso = 700
	}
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	font, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, peso, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	defer pDeleteObject.Call(font)
	pSelectObject.Call(dc, font)
	pSetBkMode.Call(dc, 1)
	pSetTextColor.Call(dc, 0x00FFFFFF)
	s, _ := windows.UTF16PtrFromString(testo)
	pTextOutW.Call(dc, 0, 0, uintptr(unsafe.Pointer(s)), uintptr(len([]rune(testo))))
	pix := unsafe.Slice((*byte)(bits), w*h*4)
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			i := (yy*w + xx) * 4
			v := max(pix[i], pix[i+1], pix[i+2]) // bianco su nero: luminosità = copertura
			if v == 0 || x+xx >= img.Bounds().Dx() || y+yy >= img.Bounds().Dy() {
				continue
			}
			sf := img.RGBAAt(x+xx, y+yy)
			a := float64(v) / 255
			mix := func(b uint8) uint8 { return uint8(float64(b)*(1-a) + 255*a) }
			img.SetRGBA(x+xx, y+yy, color.RGBA{mix(sf.R), mix(sf.G), mix(sf.B), 255})
		}
	}
	return nil
}

func rapportoCollaudo(c Collaudo, durata time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Collaudo dal vivo di Tasso Traduce (%s, durata %s)\n\n", c.Data, durata.Round(time.Second))
	b.WriteString("## PC\n\n")
	for _, k := range []string{"scheda", "ram", "core", "ia_installata", "glossario_voci"} {
		fmt.Fprintf(&b, "- %s: %s\n", k, c.PC[k])
	}
	fmt.Fprintf(&b, "\n## 1000 frasi: Mozilla dal vivo\n\nchrF++ %.1f · mediana %.0f ms · 90%% sotto %.0f ms · massimo %.0f ms\n\n", c.Mozilla.ChrF, c.Mozilla.Mediana, c.Mozilla.P90, c.Mozilla.Massimo)
	fmt.Fprintf(&b, "## Regola automatica\n\n%s\n\n", c.Regola)
	for _, e := range c.Correttori {
		fmt.Fprintf(&b, "## Correttore IA: %s (strati sulla scheda: %s)\n\n", e.Modo, e.Strati)
		if e.Note != "" {
			fmt.Fprintf(&b, "%s\n\n", e.Note)
		}
		fmt.Fprintf(&b, "- frasi: %d · ricontrollate %d · cambiate %d (meglio %d, peggio %d)\n", e.Frasi, e.Ricontrollate, e.Cambiate, e.Migliori, e.Peggiori)
		fmt.Fprintf(&b, "- chrF++: Mozilla %.1f → con correttore %.1f\n", e.ChrFMozilla, e.ChrFCorretto)
		fmt.Fprintf(&b, "- tempo per correzione: mediana %.0f ms, 90%% sotto %.0f ms, massimo %.0f ms\n", e.MedianaMs, e.P90Ms, e.MassimoMs)
		fmt.Fprintf(&b, "- memoria video del correttore: circa %.1f GB · pause per processore pieno: %d · processore senza IA al massimo %.0f%% · durata %s\n\n", e.VRAMGB, e.Pause, e.CPUMax, e.Durata)
		if len(e.Esempi) > 0 {
			b.WriteString("| Inglese | Mozilla | Con correttore |\n|---|---|---|\n")
			for _, x := range e.Esempi {
				fmt.Fprintf(&b, "| %s | %s | %s |\n", x[0], x[1], x[2])
			}
			b.WriteString("\n")
		}
	}
	if len(c.Casi) > 0 {
		b.WriteString("## Casi noti (termini)\n\n| Inglese | Deve contenere | Programma | Con correttore | OK |\n|---|---|---|---|---|\n")
		for _, k := range c.Casi {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %v |\n", k.Inglese, k.Vuole, k.Mozilla, k.ConIA, k.OK)
		}
		b.WriteString("\n")
	}
	if len(c.Grassetto) > 0 {
		b.WriteString("## Grassetto (OCR vero di Windows)\n\n| Scritta | Grassetto vero | Riconosciuto | Spessore | Letto |\n|---|---|---|---|---|\n")
		for _, g := range c.Grassetto {
			fmt.Fprintf(&b, "| %s | %v | %v | %.3f | %s |\n", g.Testo, g.Grassetto, g.Visto, g.Tratto, g.Letto)
		}
		b.WriteString("\n")
	}
	if len(c.Errori) > 0 {
		b.WriteString("## Errori\n\n")
		for _, e := range c.Errori {
			b.WriteString("- " + e + "\n")
		}
	}
	return b.String()
}

func salvaPNG(img image.Image, percorso string) error {
	f, err := os.Create(percorso)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// finestraErrore / finestraInfo: messaggi delle prove (solo Tasso-collaudo).
func finestraErrore(testo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	c, _ := windows.UTF16PtrFromString("Tasso Traduce")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafePtr(t)), uintptr(unsafePtr(c)), 0x10)
}

func finestraInfo(testo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	c, _ := windows.UTF16PtrFromString("Tasso Traduce")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafePtr(t)), uintptr(unsafePtr(c)), 0x40)
}

func unsafePtr(p *uint16) unsafe.Pointer { return unsafe.Pointer(p) }
