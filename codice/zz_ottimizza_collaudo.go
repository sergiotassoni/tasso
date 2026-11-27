//go:build collaudo

package main

// Solo nella build di prova (-tags collaudo). Tasso-collaudo.exe --prova-ottimizza fa il giro
// completo di Ottimizza tutto (Gaming e Ufficio) con il "registro di prova": ogni valore si legge
// la prima volta dal registro vero, ma si scrive e si rimette solo sotto
// HKCU\Software\Tasso\ProvaOttimizza (piano energetico, desktop e preferenze restano in memoria;
// la pulizia non cancella niente). Controlla già a posto, applica, verifica, il file di stato
// (scritto e riletto) e rimetti com'era, poi che tutto sia tornato identico. Prova anche la scena
// DirectX (10 secondi), la lettura del tempo di accensione e il backup .reg delle chiavi.
// Con --prova-ottimizza=apertura misura anche l'apertura di Esplora file, Blocco note e browser.
// Scrive tutto in %LOCALAPPDATA%\Tasso\logs\prova-ottimizza.txt ed esce (0 tutto bene, 1 no).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tasso/apertura"
	"tasso/ottimizza"
)

const chiaveProvaOtt = `Software\Tasso\ProvaOttimizza`

type regProva struct {
	vero  regVero
	letti map[string]bool
}

func (p *regProva) sotto(r ottimizza.Radice, chiave string) string {
	return chiaveProvaOtt + `\` + string(r) + `\` + chiave
}

func (p *regProva) semina(r ottimizza.Radice, chiave, nome string) error {
	id := string(r) + `\` + strings.ToLower(chiave) + `|` + strings.ToLower(nome)
	if p.letti[id] {
		return nil
	}
	v, err := p.vero.Leggi(r, chiave, nome)
	if err != nil {
		return err // come sul PC vero (per esempio senza amministratore)
	}
	p.letti[id] = true
	if v.Esiste {
		v.Radice, v.Chiave = ottimizza.HKCU, p.sotto(r, chiave)
		return p.vero.Scrivi(v)
	}
	return p.vero.Cancella(ottimizza.HKCU, p.sotto(r, chiave), nome)
}

func (p *regProva) Leggi(r ottimizza.Radice, chiave, nome string) (ottimizza.Valore, error) {
	if err := p.semina(r, chiave, nome); err != nil {
		return ottimizza.Valore{Radice: r, Chiave: chiave, Nome: nome}, err
	}
	v, err := p.vero.Leggi(ottimizza.HKCU, p.sotto(r, chiave), nome)
	v.Radice, v.Chiave, v.Nome = r, chiave, nome
	return v, err
}

func (p *regProva) Scrivi(v ottimizza.Valore) error {
	if err := p.semina(v.Radice, v.Chiave, v.Nome); err != nil {
		return err
	}
	w := v
	w.Radice, w.Chiave = ottimizza.HKCU, p.sotto(v.Radice, v.Chiave)
	return p.vero.Scrivi(w)
}

func (p *regProva) Cancella(r ottimizza.Radice, chiave, nome string) error {
	if err := p.semina(r, chiave, nome); err != nil {
		return err
	}
	return p.vero.Cancella(ottimizza.HKCU, p.sotto(r, chiave), nome)
}

func (p *regProva) Nomi(r ottimizza.Radice, chiave string) ([]string, error) { return nil, nil }

// enProva: legge il piano energetico vero, scrive solo in memoria.
type enProva struct {
	vero    enVera
	attivo  string
	overlay string
	val     map[string][2]uint32
}

func (e *enProva) PianoAttivo() (string, error) {
	if e.attivo == "" {
		a, err := e.vero.PianoAttivo()
		if err != nil {
			return "", err
		}
		e.attivo = a
	}
	return e.attivo, nil
}
func (e *enProva) PianoEsiste(g string) bool { return e.vero.PianoEsiste(g) }
func (e *enProva) AttivaPiano(g string) error {
	if !e.vero.PianoEsiste(g) {
		return errors.New("piano che non c'è")
	}
	e.attivo = g
	return nil
}
func (e *enProva) Overlay() (string, error) {
	if e.overlay == "" {
		o, err := e.vero.Overlay()
		if err != nil {
			return "", err
		}
		e.overlay = o
	}
	return e.overlay, nil
}
func (e *enProva) ImpostaOverlay(g string) error { e.overlay = g; return nil }
func (e *enProva) Leggi(p, g, i string) (uint32, uint32, error) {
	if x, ok := e.val[p+g+i]; ok {
		return x[0], x[1], nil
	}
	ac, dc, err := e.vero.Leggi(p, g, i)
	if err == nil {
		e.val[p+g+i] = [2]uint32{ac, dc}
	}
	return ac, dc, err
}
func (e *enProva) ScriviAC(p, g, i string, v uint32) error {
	ac, dc, err := e.Leggi(p, g, i)
	_ = ac
	e.val[p+g+i] = [2]uint32{v, dc}
	return err
}
func (e *enProva) ScriviDC(p, g, i string, v uint32) error {
	ac, _, err := e.Leggi(p, g, i)
	e.val[p+g+i] = [2]uint32{ac, v}
	return err
}

type sisProva struct {
	vero  sisVero
	spi   map[string]bool
	mouse []int
}

func (s *sisProva) LeggiSPI(id string) (bool, error) {
	if v, ok := s.spi[id]; ok {
		return v, nil
	}
	v, err := s.vero.LeggiSPI(id)
	if err == nil {
		s.spi[id] = v
	}
	return v, err
}
func (s *sisProva) ScriviSPI(id string, on bool) error { s.spi[id] = on; return nil }
func (s *sisProva) ImpostaMouse(a, b, c int) error     { s.mouse = []int{a, b, c}; return nil }
func (s *sisProva) Avvisa(string)                      {}

type prefProva struct{ on bool }

func (p *prefProva) NotificheGioco() bool                { return p.on }
func (p *prefProva) ImpostaNotificheGioco(on bool) error { p.on = on; return nil }

type pulProva struct{ log func(string) }

func (p pulProva) Pulisci(id string) (int64, error) {
	p.log("   pulizia " + id + ": NON eseguita nella prova (cancellerebbe file veri)")
	return 0, nil
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == apertura.ArgDaAttivita {
		return
	}
	for _, a := range os.Args[1:] {
		if a == "--prova-ottimizza" || a == "--prova-ottimizza=apertura" {
			os.Exit(provaOttimizza(a == "--prova-ottimizza=apertura"))
		}
	}
}

func provaOttimizza(conApertura bool) int {
	appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso")
	dir := filepath.Join(appDir, "logs")
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, "prova-ottimizza.txt"))
	if err != nil {
		return 2
	}
	defer f.Close()
	errori := 0
	log := func(s string) { fmt.Fprintln(f, time.Now().Format("15:04:05")+"  "+s) }
	sbaglio := func(s string) { errori++; log("ERRORE  " + s) }
	log("Prova di Ottimizza tutto - Tasso " + appVersion)
	_, _ = runHidden("reg.exe", "delete", `HKCU\`+chiaveProvaOtt, "/f")
	defer func() { _, _ = runHidden("reg.exe", "delete", `HKCU\`+chiaveProvaOtt, "/f") }()

	pc := pcOtt()
	log(fmt.Sprintf("PC: amministratore=%v portatile=%v caricatore=%v build=%d schede vere=%d HAGS supportata=%v accesa=%v giochi=%d",
		pc.Admin, pc.Portatile, pc.Caricatore, pc.Build, pc.SchedeVere, pc.HAGS, pc.HAGSAcceso, len(pc.Giochi)))
	for _, a := range adattatoriOtt() {
		log(fmt.Sprintf("   scheda: %s (vendor 0x%04x, software %v)", a.nome, a.vendor, a.software))
	}

	for _, modo := range []string{ottimizza.ModoGioco, ottimizza.ModoUfficio} {
		log("")
		log("=== " + modo + " ===")
		rp := &regProva{letti: map[string]bool{}}
		copia := nuovoRegConCopia(rp)
		copia.esporta = func(r ottimizza.Radice, chiave, file string) {
			file = filepath.Join(dir, "prova-ottimizza-"+filepath.Base(file))
			_, err := runHiddenTimeout(20*time.Second, "reg.exe", "export", `HKCU\`+rp.sotto(r, chiave), file, "/y")
			log(fmt.Sprintf("   backup di %s -> %s (errore: %v)", nomeRegCompleto(r, chiave), filepath.Base(file), err))
		}
		en := &enProva{val: map[string][2]uint32{}}
		sis := &sisProva{spi: map[string]bool{}}
		a := &ottimizza.Ambiente{Reg: copia, En: en, Sis: sis, Pref: &prefProva{}, Pul: pulProva{log: log}, PC: pc}
		st := &ottimizza.Stato{Risultati: map[string]*ottimizza.Risultato{}}
		fileStato := filepath.Join(dir, "prova-ottimizza-stato.json")
		salva := func(s *ottimizza.Stato) error {
			b, err := s.Codifica()
			if err != nil {
				return err
			}
			return os.WriteFile(fileStato, b, 0o644)
		}
		// com'era all'inizio (dopo la prima lettura ogni valore è nella copia di prova)
		for _, m := range ottimizza.Elenco(modo) {
			ok, msg := m.G.Disponibile(a)
			gia, errA := false, error(nil)
			if ok && !m.NonRimette {
				gia, errA = m.G.APosto(a)
			}
			log(fmt.Sprintf("%-14s disponibile=%v %s già a posto=%v %v", m.ID, ok, msg, gia, errA))
		}
		primaPiano, primaOverlay := en.attivo, en.overlay
		primaEn := map[string][2]uint32{}
		for k, v := range en.val {
			primaEn[k] = v
		}
		fatte := 0
		for _, m := range ottimizza.Elenco(modo) {
			e := ottimizza.Esegui(m, a, st, salva)
			log(fmt.Sprintf("applica %-14s -> %s: %s", m.ID, e.Stato, e.Motivo))
			if e.Stato == ottimizza.VoceErrore {
				sbaglio(m.ID + ": " + e.Motivo)
			}
			if e.Stato == ottimizza.VoceFatta {
				fatte++
				if v := st.Voce(m.ID); v == nil || !m.G.Verifica(a, v.Prima) {
					sbaglio(m.ID + ": verifica fallita dopo il cambio")
				}
			}
		}
		log(fmt.Sprintf("fatte %d su %d", fatte, len(ottimizza.Elenco(modo))))
		// il file di stato si rilegge (come dopo un riavvio)
		b, err := os.ReadFile(fileStato)
		if err != nil {
			sbaglio("file di stato non scritto: " + err.Error())
			continue
		}
		st2, scartate, err := ottimizza.Leggi(b, pc.Admin)
		if err != nil || scartate > 0 {
			sbaglio(fmt.Sprintf("file di stato riletto: %v, scartate %d", err, scartate))
			continue
		}
		es, ok := ottimizza.RimettiTutto(a, st2, salva)
		for _, e := range es {
			log(fmt.Sprintf("rimetti %-14s -> %s: %s", e.ID, e.Stato, e.Motivo))
		}
		if !ok || len(st2.Attive()) > 0 {
			sbaglio("non ha rimesso tutto")
		}
		// tutto identico al registro vero (che non è mai stato toccato)
		for id := range rp.letti {
			parti := strings.SplitN(id, "|", 2)
			rk := strings.SplitN(parti[0], `\`, 2)
			r, k, n := ottimizza.Radice(rk[0]), rk[1], parti[1]
			vero, e1 := regVero{}.Leggi(r, k, n)
			prova, e2 := rp.Leggi(r, k, n)
			if e1 != nil || e2 != nil || !vero.Uguale(prova) {
				sbaglio(fmt.Sprintf("non tornato com'era: %s\\%s %s (vero %+v, prova %+v)", r, k, n, vero, prova))
			}
		}
		if en.attivo != primaPiano || en.overlay != primaOverlay {
			sbaglio("piano energetico non tornato com'era")
		}
		for k, v := range primaEn {
			if en.val[k] != v {
				sbaglio("impostazione del piano non tornata: " + k)
			}
		}
		for id, v := range sis.spi {
			if vero, err := (sisVero{}).LeggiSPI(id); err == nil && vero != v {
				sbaglio("impostazione del desktop non tornata: " + id)
			}
		}
		_ = os.Remove(fileStato)
	}

	log("")
	log("=== scena DirectX (10 secondi) ===")
	durate, err := scenaFPS(context.Background(), 10*time.Second, nil)
	if medi, bassi, ok := ottimizza.StatFotogrammi(durate); ok {
		log(fmt.Sprintf("FPS medi %.1f, più bassi (1%%) %.1f, fotogrammi %d", medi, bassi, len(durate)))
	} else {
		log(fmt.Sprintf("scena non riuscita: %v (Ottimizza tutto continua senza FPS)", err))
	}
	log("")
	log("=== tempo di accensione ===")
	l, motivo := leggiAccensioniOtt()
	for _, x := range l {
		log(fmt.Sprintf("accensione del %s: %.1f s", x.Inizio.Local().Format("02/01 15:04"), float64(x.Ms)/1000))
	}
	if motivo != "" {
		log(motivo)
	}
	if conApertura {
		log("")
		log("=== apertura dei programmi ===")
		ap := aperturaOtt(context.Background(), func(float64) {}, log)
		log(fmt.Sprintf("%v", ap))
	}
	log("")
	if errori == 0 {
		log("TUTTO BENE")
		return 0
	}
	log(fmt.Sprintf("ERRORI: %d", errori))
	return 1
}
