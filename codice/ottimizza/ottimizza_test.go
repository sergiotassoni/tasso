package ottimizza

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestQuanteModifiche(t *testing.T) {
	if n := len(Elenco(ModoGioco)); n != 20 {
		t.Fatalf("Gaming: %d modifiche, ne servono 20", n)
	}
	if n := len(Elenco(ModoUfficio)); n != 15 {
		t.Fatalf("Ufficio: %d modifiche, ne servono 15 (13 impostazioni e 2 pulizie)", n)
	}
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		cats := map[string]bool{}
		for _, c := range Categorie(modo) {
			cats[c.ID] = true
		}
		visti := map[string]bool{}
		for _, m := range Elenco(modo) {
			if visti[m.ID] {
				t.Errorf("%s: %s due volte", modo, m.ID)
			}
			visti[m.ID] = true
			if !cats[m.Categoria] {
				t.Errorf("%s: %s in una categoria che non c'è (%s)", modo, m.ID, m.Categoria)
			}
		}
	}
}

// Niente modifiche vietate: antivirus, aggiornamenti spenti, schede di rete, file di sistema.
func TestNienteChiaviVietate(t *testing.T) {
	vietate := []string{"Defender", "NoAutoUpdate", "AUOptions", "wuauserv", "TcpAckFrequency", "TCPNoDelay", "FlowControl", "InterruptModeration", "*FlowControl", "Tcpip", "NVIDIA", "AMD", "System32"}
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		for _, m := range Elenco(modo) {
			b, _ := json.Marshal(chiaviDi(m.G))
			for _, v := range vietate {
				if strings.Contains(string(b), v) {
					t.Errorf("%s tocca %q", m.ID, v)
				}
			}
		}
	}
}

func chiaviDi(g Gestore) []string {
	var out []string
	switch x := g.(type) {
	case *regGest:
		for _, b := range x.bers {
			out = append(out, string(b.vuole.Radice)+`\`+b.vuole.Chiave+`|`+b.vuole.Nome)
		}
	case *insieme:
		for _, p := range x.parti {
			out = append(out, chiaviDi(p)...)
		}
	case *mouseGest:
		out = append(out, chiaviDi(&x.reg)...)
	case *finestraGest:
		out = append(out, string(HKCU)+`\`+ChiaveGPU+`|`+NomeDXGlobale)
	}
	return out
}

// Il giro completo su un PC finto: applica tutto, verifica, rimette tutto e il PC torna identico.
func TestGiroCompletoTornaComEra(t *testing.T) {
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		a, r, e, s := ambienteFinto()
		if modo == ModoUfficio {
			a.PC.Portatile = true
		}
		primaReg, primaPiano, primaSPI, primaMouse := r.copia(), e.attivo, map[string]bool{}, s.mouse
		for k, v := range s.spi {
			primaSPI[k] = v
		}
		primaEn := map[string][2]uint32{}
		for k, v := range e.val {
			primaEn[k] = v
		}
		st := &Stato{}
		n := 0
		fatte := 0
		for _, m := range Elenco(modo) {
			es := Esegui(m, a, st, salvaFinto(&n))
			if es.Stato == VoceFatta {
				fatte++
			}
			if es.Stato == VoceErrore {
				t.Errorf("%s/%s: errore %s", modo, m.ID, es.Motivo)
			}
		}
		if fatte < 5 { // Ufficio: 8 voci, di cui 2 pulizie ("pulita", non "fatta")
			t.Errorf("%s: solo %d fatte", modo, fatte)
		}
		if n == 0 {
			t.Fatal("il file di stato non è mai stato salvato")
		}
		// il file di stato si legge di nuovo uguale (dopo un riavvio)
		b, err := st.Codifica()
		if err != nil {
			t.Fatal(err)
		}
		st2, scartate, err := Leggi(b, true)
		if err != nil || scartate != 0 {
			t.Fatalf("%s: rilettura: %v, scartate %d", modo, err, scartate)
		}
		if !reflect.DeepEqual(st.Attive(), st2.Attive()) {
			t.Fatalf("%s: voci attive diverse dopo la rilettura: %v / %v", modo, st.Attive(), st2.Attive())
		}
		_, ok := RimettiTutto(a, st2, salvaFinto(&n))
		if !ok {
			t.Fatalf("%s: non ha rimesso tutto", modo)
		}
		if len(st2.Attive()) != 0 {
			t.Errorf("%s: restano attive %v", modo, st2.Attive())
		}
		if !reflect.DeepEqual(primaReg, r.v) {
			t.Errorf("%s: il registro non è tornato com'era\nprima %v\ndopo  %v", modo, primaReg, r.v)
		}
		if e.attivo != primaPiano || !reflect.DeepEqual(primaEn, e.val) {
			t.Errorf("%s: piano energetico non tornato: %s %v", modo, e.attivo, e.val)
		}
		if !reflect.DeepEqual(primaSPI, s.spi) || primaMouse != s.mouse {
			t.Errorf("%s: desktop non tornato: %v %v", modo, s.spi, s.mouse)
		}
	}
}

func TestGiaAPostoLaSalta(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kGameBar, "AutoGameModeEnabled", 1))
	m, _ := Trova("gamemode")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceAPosto || es.Motivo != MsgAPosto {
		t.Fatalf("Modalità Gioco già accesa: %+v", es)
	}
	// manca il valore: Windows 10/11 la tengono accesa di base
	delete(r.v, chiaveFinta(HKCU, kGameBar, "AutoGameModeEnabled"))
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceAPosto {
		t.Fatalf("senza valore: %+v", es)
	}
	r.metti(dw(HKCU, kGameBar, "AutoGameModeEnabled", 0))
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatalf("spenta: %+v", es)
	}
	// seconda volta: già sistemata da Tasso, e il "com'era" resta quello vero
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceAPosto || es.Motivo != MsgGiaFatta {
		t.Fatalf("seconda volta: %+v", es)
	}
	if v := st.Voce("gamemode"); v == nil || v.Prima.Valori[0].Num != 0 {
		t.Fatalf("com'era perso: %+v", v)
	}
}

func TestPrimoPiano(t *testing.T) {
	casi := map[uint32]bool{2: true, 0x26: true, 0x22: true, 0x18: false, 0x1a: false, 0x2a: false, 0x16: false, 0: false, 0x12: false, 0x3e: true, 0x24: false}
	for n, vuole := range casi {
		if g := primoPianoOK(dw(HKLM, kPriorita, "x", n)); g != vuole {
			t.Errorf("Win32PrioritySeparation 0x%x: %v, vuole %v", n, g, vuole)
		}
	}
	if !primoPianoOK(Valore{}) {
		t.Error("senza valore va bene (vale 2)")
	}
}

func TestOrari(t *testing.T) {
	if !copreSera(8, 2) || !copreSera(12, 0) || copreSera(8, 17) || copreSera(15, 2) || !copreSera(14, 6) {
		t.Error("copreSera sbagliata")
	}
	a, r, _, _ := ambienteFinto()
	m, _ := Trova("orari")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatalf("%+v", es)
	}
	if v, _ := r.Leggi(HKLM, kAggUX, "ActiveHoursEnd"); v.Num != OrariFine {
		t.Fatal("fine non scritta")
	}
	// un criterio di gruppo dell'azienda: non si tocca
	a2, r2, _, _ := ambienteFinto()
	r2.metti(dw(HKLM, kAggPol, "SetActiveHours", 1))
	if es := Esegui(m, a2, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata {
		t.Fatalf("con il criterio: %+v", es)
	}
}

// Regola del portatile: niente piano energetico sui portatili, batteria solo sui portatili.
func TestRegolaPortatile(t *testing.T) {
	a, _, e, _ := ambienteFinto()
	n := 0
	piano, _ := Trova("piano")
	batt, _ := Trova("batt_schermo")
	a.PC.Portatile = true
	if es := Esegui(piano, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != msgPianoPortatile {
		t.Fatalf("portatile: %+v", es)
	}
	if e.attivo != PianoBilanciato {
		t.Fatal("sul portatile il piano è cambiato")
	}
	// "Mai" a batteria (0) l'ha scelto l'utente: non si tocca (F05-19)
	e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento] = [2]uint32{600, 0}
	if es := Esegui(batt, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != msgSchermoMai {
		t.Fatalf("schermo a batteria su Mai: %+v", es)
	}
	e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento] = [2]uint32{600, 900}
	if es := Esegui(batt, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatalf("batteria sul portatile: %+v", es)
	}
	if e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento] != [2]uint32{600, 300} {
		t.Fatalf("schermo a batteria: %v (a corrente non va toccato)", e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento])
	}
	a.PC.Portatile = false
	if es := Esegui(batt, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata {
		t.Fatalf("batteria sul fisso: %+v", es)
	}
	if es := Esegui(piano, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceFatta || e.attivo != PianoPrestazioni {
		t.Fatalf("fisso: %+v %s", es, e.attivo)
	}
}

// Senza "Prestazioni elevate" (molti PC nuovi): la modalità "Prestazioni ottimali" di Windows.
func TestPianoSenzaPrestazioni(t *testing.T) {
	a, _, e, _ := ambienteFinto()
	delete(e.piani, PianoPrestazioni)
	m, _ := Trova("piano")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta || es.Motivo != msgPianoOverlay || e.overlay != OverlayMigliori || e.attivo != PianoBilanciato {
		t.Fatalf("%+v %s %s", es, e.attivo, e.overlay)
	}
	if es := Rimetti("piano", a, st, salvaFinto(&n)); es.Stato != VoceRimessa || e.overlay != OverlayNessuno {
		t.Fatalf("rimetti: %+v %s", es, e.overlay)
	}
}

// Rimetti com'era non tocca quello che hai cambiato tu nel frattempo.
func TestRimettiSoloSeEAncoraDiTasso(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	m, _ := Trova("registrazione")
	r.metti(dw(HKCU, kGameDVR, "HistoricalCaptureEnabled", 1))
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	r.metti(dw(HKCU, kGameDVR, "HistoricalCaptureEnabled", 5)) // cambiato da te
	if es := Rimetti("registrazione", a, st, salvaFinto(&n)); es.Stato != VoceLasciata {
		t.Fatalf("%+v", es)
	}
	if v, _ := r.Leggi(HKCU, kGameDVR, "HistoricalCaptureEnabled"); v.Num != 5 {
		t.Fatal("ha sovrascritto il valore dell'utente")
	}
}

// Una modifica che non riesce non ferma le altre e non lascia niente a metà.
func TestErroreNonFermaTutto(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.rotte[chiaveFinta(HKCU, kRicerca, "BackgroundAppGlobalToggle")] = true
	st := &Stato{}
	n := 0
	var esiti []Esito
	for _, m := range Elenco(ModoGioco) {
		esiti = append(esiti, Esegui(m, a, st, salvaFinto(&n)))
	}
	trovato := false
	for _, e := range esiti {
		if e.ID == "bgapps" {
			trovato = true
			if e.Stato != VoceErrore {
				t.Fatalf("bgapps: %+v", e)
			}
		}
	}
	if !trovato {
		t.Fatal("bgapps non c'è")
	}
	// la prima parte (GlobalUserDisabled) era stata scritta: deve essere tornata com'era
	if v, _ := r.Leggi(HKCU, kBackground, "GlobalUserDisabled"); v.Esiste {
		t.Fatalf("pezzo rimasto a metà: %+v", v)
	}
	if v, _ := r.Leggi(HKCU, kGameDVR, "HistoricalCaptureEnabled"); v.Esiste {
		t.Log("registrazione: era già spenta di base, va bene")
	}
	if st.Voce("mouse") == nil || st.Voce("mouse").Stato != VoceFatta {
		t.Fatal("dopo l'errore le altre non sono andate avanti")
	}
}

// Il cambio che non resta (Windows lo rimette da solo): lasciata com'era, niente voce attiva.
func TestVerificaFallita(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.ignora[chiaveFinta(HKCU, kNotifiche, "NOC_GLOBAL_SETTING_ALLOW_NOTIFICATION_SOUND")] = true
	m, _ := Trova("suoni")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceErrore || es.Motivo != MsgNonResta {
		t.Fatalf("%+v", es)
	}
	if len(st.Attive()) != 0 {
		t.Fatal("voce attiva per un cambio che non c'è")
	}
}

// Se il file di stato non si salva, Tasso non cambia niente.
func TestSenzaSalvataggioNonCambia(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	m, _ := Trova("mouse")
	prima := r.copia()
	es := Esegui(m, a, &Stato{}, func(*Stato) error { return errors.New("disco pieno") })
	if es.Stato != VoceSaltata || es.Motivo != MsgNonSalvo {
		t.Fatalf("%+v", es)
	}
	if !reflect.DeepEqual(prima, r.v) {
		t.Fatal("ha cambiato senza aver salvato com'era")
	}
}

func TestSenzaAdmin(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	a.PC.Admin = false
	r.bloccate[HKLM], r.bloccate[HKNS] = true, true
	st := &Stato{}
	n := 0
	for _, id := range []string{"primopiano", "hags", "recapito", "orari"} {
		m, _ := Trova(id)
		if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != msgAdmin {
			t.Errorf("%s senza amministratore: %+v", id, es)
		}
	}
	m, _ := Trova("mouse")
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Errorf("mouse senza amministratore: %+v", es)
	}
}

func TestRecapitoNonAccessibile(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.bloccate[HKNS] = true
	m, _ := Trova("recapito")
	if es := Esegui(m, a, &Stato{}, func(*Stato) error { return nil }); es.Stato != VoceSaltata || es.Motivo != MsgNonLeggo {
		t.Fatalf("%+v", es)
	}
}

func TestSchedaVideoGiochi(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	altro := `C:\Giochi\Altro\altro.exe`
	a.PC.Giochi = append(a.PC.Giochi, altro, `relativo.exe`)
	r.metti(sz(HKCU, ChiaveGPU, altro, "GpuPreference=1;AutoHDREnable=1;"))
	r.metti(sz(HKCU, ChiaveGPU, a.PC.Giochi[0], "SwapEffectUpgradeEnable=1;"))
	m, _ := Trova("gpupref")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatalf("%+v", es)
	}
	if v, _ := r.Leggi(HKCU, ChiaveGPU, a.PC.Giochi[0]); v.Testo != "GpuPreference=2;SwapEffectUpgradeEnable=1;" {
		t.Fatalf("gioco: %q", v.Testo)
	}
	if v, _ := r.Leggi(HKCU, ChiaveGPU, altro); v.Testo != "GpuPreference=1;AutoHDREnable=1;" {
		t.Fatal("ha cambiato una scelta fatta dall'utente")
	}
	if es := Rimetti("gpupref", a, st, salvaFinto(&n)); es.Stato != VoceRimessa {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKCU, ChiaveGPU, a.PC.Giochi[0]); v.Testo != "SwapEffectUpgradeEnable=1;" {
		t.Fatalf("non rimesso: %q", v.Testo)
	}
	a.PC.SchedeVere = 1
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata {
		t.Fatalf("una scheda sola: %+v", es)
	}
}

func TestHAGS(t *testing.T) {
	a, _, _, _ := ambienteFinto()
	m, _ := Trova("hags")
	n := 0
	a.PC.HAGS = false
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata {
		t.Fatalf("non supportata: %+v", es)
	}
	a.PC.HAGS, a.PC.HAGSAcceso = true, true
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceAPosto {
		t.Fatalf("già accesa dal driver: %+v", es)
	}
	a.PC.HAGSAcceso = false
	st := &Stato{}
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta || es.Motivo != MsgFattaRiavvio {
		t.Fatalf("da accendere: %+v", es)
	}
	// F04-14: rimessa, ma Windows la usa solo dal prossimo riavvio: va detto
	if es := Rimetti("hags", a, st, salvaFinto(&n)); es.Stato != VoceRimessa || es.Motivo != MsgRimessaRiavvio {
		t.Fatalf("rimetti: %+v", es)
	}
}

// F05-5: con un piano acceso per un po' (Modalità riunione) le impostazioni della batteria vanno nel
// piano dell'utente; se non si sa qual è, la voce si salta e lo dice.
func TestBatteriaNelPianoUtente(t *testing.T) {
	a, _, e, _ := ambienteFinto()
	a.PC.Portatile = true
	e.attivo = PianoPrestazioni // la riunione ha messo Prestazioni elevate
	e.val[PianoPrestazioni+GruppoSchermo+ImpSchermoSpento] = [2]uint32{600, 900}
	a.PC.PianoUtente = PianoBilanciato
	m, _ := Trova("batt_schermo")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatalf("%+v", es)
	}
	if e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento] != [2]uint32{600, 300} || e.val[PianoPrestazioni+GruppoSchermo+ImpSchermoSpento] != [2]uint32{600, 900} {
		t.Fatalf("scritto nel piano sbagliato: %v", e.val)
	}
	// finita la riunione (piano dell'utente di nuovo acceso) si rimette nel piano giusto
	e.attivo, a.PC.PianoUtente = PianoBilanciato, ""
	if es := Rimetti("batt_schermo", a, st, salvaFinto(&n)); es.Stato != VoceRimessa || e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento] != [2]uint32{600, 900} {
		t.Fatalf("rimetti: %+v %v", es, e.val)
	}
	a.PC.PianoTemporaneo = true
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != MsgPianoTemporaneo {
		t.Fatalf("piano dell'utente sconosciuto: %+v", es)
	}
}

// "Mouse e tastiera non si addormentano" (usb) con una modalità di Tasso accesa: va nel piano
// dell'utente salvato prima della modalità, non in quello della modalità (la stessa regola di F05-5).
func TestUSBNelPianoUtente(t *testing.T) {
	a, _, e, _ := ambienteFinto()
	e.attivo = PianoPrestazioni // la modalità Gaming ha messo Prestazioni elevate
	a.PC.ModoTasso = "gaming"
	a.PC.PianoUtente = PianoBilanciato
	m, _ := Trova("usb")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatalf("%+v", es)
	}
	if e.val[PianoBilanciato+GruppoUSB+ImpUSBSospensione] != [2]uint32{0, 1} || e.val[PianoPrestazioni+GruppoUSB+ImpUSBSospensione] != [2]uint32{1, 1} {
		t.Fatalf("scritto nel piano sbagliato: %v", e.val)
	}
	// tornato in Normale (piano dell'utente di nuovo acceso) si rimette nel piano giusto
	e.attivo, a.PC.ModoTasso, a.PC.PianoUtente = PianoBilanciato, "", ""
	if es := Rimetti("usb", a, st, salvaFinto(&n)); es.Stato != VoceRimessa || e.val[PianoBilanciato+GruppoUSB+ImpUSBSospensione] != [2]uint32{1, 1} {
		t.Fatalf("rimetti: %+v %v", es, e.val)
	}
	// senza modalità né riunione va nel piano acceso, come prima
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceFatta || e.val[PianoBilanciato+GruppoUSB+ImpUSBSospensione] != [2]uint32{0, 1} {
		t.Fatalf("senza modalità: %+v %v", es, e.val)
	}
	// modalità accesa ma il piano dell'utente non si sa: si salta e lo dice
	a.PC.ModoTasso, a.PC.PianoTemporaneo = "gaming", true
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != MsgPianoTemporaneo {
		t.Fatalf("piano dell'utente sconosciuto: %+v", es)
	}
}

func TestPuliziaNonRimette(t *testing.T) {
	a, _, _, _ := ambienteFinto()
	m, _ := Trova("pulizia_temp")
	st := &Stato{}
	n := 0
	es := Esegui(m, a, st, salvaFinto(&n))
	if es.Stato != VocePulita {
		t.Fatalf("%+v", es)
	}
	p := a.Pul.(*pulFinto)
	if len(p.fatti) == 0 || p.fatti[0] != "backup" {
		t.Fatalf("prima il backup (come la pulizia Media): %v", p.fatti)
	}
	if len(st.Attive()) != 0 {
		t.Fatal("la pulizia non è una voce da rimettere")
	}
}

// Il file di stato modificato da qualcun altro: niente chiavi estranee, niente valori strani.
func TestFileDiStatoManomesso(t *testing.T) {
	cattivi := []string{
		`{"voci":[{"id":"mouse","stato":"fatta","prima":{"v":[{"r":"HKLM","k":"SYSTEM\\CurrentControlSet\\Services\\WinDefend","n":"Start","t":"dword","e":true,"d":4}],"m":[{"r":"HKCU","k":"Control Panel\\Mouse","n":"MouseSpeed","t":"sz","e":true,"s":"0"}]}}]}`,
		`{"voci":[{"id":"mouse","stato":"fatta","prima":{"v":[{"r":"HKCU","k":"Control Panel\\Mouse","n":"MouseSpeed","t":"sz","e":true,"s":"0"}],"m":[{"r":"HKCU","k":"Control Panel\\Mouse","n":"MouseSpeed","t":"sz","e":true,"s":"1"}]}}]}`,
		`{"voci":[{"id":"sconosciuta","stato":"fatta","prima":{}}]}`,
		`{"voci":[{"id":"piano","stato":"fatta","prima":{"x":{"modo":"piano","prima":"; rm -rf","messo":"8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"}}}]}`,
		`{"voci":[{"id":"gpupref","stato":"fatta","prima":{"v":[{"r":"HKCU","k":"Software\\Microsoft\\DirectX\\UserGpuPreferences","n":"C:\\x.exe","e":false}],"m":[{"r":"HKCU","k":"Software\\Microsoft\\Windows\\CurrentVersion\\Run","n":"C:\\x.exe","t":"sz","e":true,"s":"GpuPreference=2;"}]}}]}`,
		`{"voci":[{"id":"effetti","stato":"fatta","prima":{"p":[{"z":true},{"z":true},{"x":{"spi":"altro","prima":"1","messo":"0"}},{"z":true}]}}]}`,
	}
	for i, c := range cattivi {
		st, scartate, err := Leggi([]byte(c), true)
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if len(st.Attive()) != 0 || scartate != 1 {
			t.Errorf("caso %d accettato: %+v scartate %d", i, st.Voci, scartate)
		}
	}
	if _, _, err := Leggi([]byte("{rotto"), true); err == nil {
		t.Error("JSON rotto accettato")
	}
	// senza amministratore: niente valori di tutto il PC dal file dell'utente
	buono := `{"voci":[{"id":"orari","stato":"fatta","prima":{"v":[{"r":"HKLM","k":"SOFTWARE\\Microsoft\\WindowsUpdate\\UX\\Settings","n":"ActiveHoursEnd","t":"dword","e":true,"d":17}],"m":[{"r":"HKLM","k":"SOFTWARE\\Microsoft\\WindowsUpdate\\UX\\Settings","n":"ActiveHoursEnd","t":"dword","e":true,"d":2}]}}]}`
	if st, _, _ := Leggi([]byte(buono), true); len(st.Attive()) != 1 {
		t.Error("da amministratore va accettato")
	}
	if st, scartate, _ := Leggi([]byte(buono), false); len(st.Attive()) != 0 || scartate != 1 {
		t.Error("senza amministratore va scartato")
	}
}

func TestStatFotogrammi(t *testing.T) {
	var d []time.Duration
	for i := 0; i < 990; i++ {
		d = append(d, 10*time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		d = append(d, 40*time.Millisecond)
	}
	medi, bassi, ok := StatFotogrammi(d)
	if !ok {
		t.Fatal("non ok")
	}
	// 1000 fotogrammi in 10,3 s = 97,1 FPS; i 10 più lenti a 40 ms = 25 FPS
	if medi < 97 || medi > 97.2 || bassi < 24.99 || bassi > 25.01 {
		t.Fatalf("medi %.2f bassi %.2f", medi, bassi)
	}
	if _, _, ok := StatFotogrammi(d[:50]); ok {
		t.Fatal("troppo pochi fotogrammi accettati")
	}
}

func TestConfronta(t *testing.T) {
	r := Risultato{Modo: ModoGioco, Prima: Misura{FPSOk: true, FPSMedi: 142.4, FPSBassi: 96, Voto: 67}, Dopo: &Misura{FPSOk: true, FPSMedi: 154.2, FPSBassi: 107, Voto: 72}}
	righe, piccolo := Confronta(r)
	if piccolo || len(righe) != 3 || righe[0].Chiave != "fps" || righe[0].Prima != 142 || righe[0].Dopo != 154 || !righe[0].Meglio {
		t.Fatalf("%+v %v", righe, piccolo)
	}
	r.Dopo = &Misura{FPSOk: true, FPSMedi: 143, FPSBassi: 97, Voto: 68}
	if _, piccolo := Confronta(r); !piccolo {
		t.Fatal("+1 FPS e +1 voto: deve dire che era già messo bene")
	}
	// la scena DirectX non è partita: solo il voto
	r = Risultato{Modo: ModoGioco, Prima: Misura{Voto: 60}, Dopo: &Misura{Voto: 64}}
	if righe, piccolo := Confronta(r); len(righe) != 1 || piccolo {
		t.Fatalf("senza FPS: %+v %v", righe, piccolo)
	}
	// Ufficio: accensione "dopo" in attesa della prossima riaccensione
	r = Risultato{Modo: ModoUfficio, Prima: Misura{Accensione: 48000, Apertura: map[string]int{"esplora": 900, "blocconote": 700, "browser": 1600}, Voto: 61},
		Dopo: &Misura{Apertura: map[string]int{"esplora": 600, "blocconote": 500, "browser": 0}, Voto: 61}}
	righe, piccolo = Confronta(r)
	if len(righe) != 3 || righe[0].HaDopo || righe[1].Prima != 1.6 || righe[1].Dopo != 1.1 || !righe[1].Meglio || piccolo {
		t.Fatalf("Ufficio: %+v %v", righe, piccolo)
	}
}

func TestEvento100(t *testing.T) {
	x := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-Diagnostics-Performance'/><EventID>100</EventID><TimeCreated SystemTime='2026-11-27T08:03:10.1234567Z'/></System><EventData><Data Name='BootStartTime'>2026-11-27T08:00:01.5000000Z</Data><Data Name='BootTime'>48123</Data><Data Name='MainPathBootTime'>21000</Data></EventData></Event>`
	a, err := LeggiEvento100(x)
	if err != nil || a.Ms != 48123 || a.Inizio.Format("15:04:05") != "08:00:01" {
		t.Fatalf("%+v %v", a, err)
	}
	for _, rotto := range []string{
		strings.Replace(x, "<EventID>100", "<EventID>101", 1),
		strings.Replace(x, "48123", "abc", 1),
		strings.Replace(x, "48123", "12", 1),
		"<non xml",
	} {
		if _, err := LeggiEvento100(rotto); err == nil {
			t.Errorf("accettato: %s", rotto)
		}
	}
	// senza BootStartTime vale l'ora dell'evento
	b, err := LeggiEvento100(strings.Replace(x, "<Data Name='BootStartTime'>2026-11-27T08:00:01.5000000Z</Data>", "", 1))
	if err != nil || b.Inizio.Format("15:04") != "08:03" {
		t.Fatalf("%+v %v", b, err)
	}
	t0 := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	l := []Accensione{{Ms: 50000, Inizio: t0.Add(-time.Hour)}, {Ms: 31000, Inizio: t0.Add(2 * time.Hour)}, {Ms: 33000, Inizio: t0.Add(5 * time.Hour)}}
	if u, ok := UltimaAccensione(l); !ok || u.Ms != 33000 {
		t.Fatal("ultima")
	}
	if d, ok := AccensioneDopo(l, t0); !ok || d.Ms != 31000 {
		t.Fatal("dopo: la prima dopo Ottimizza tutto")
	}
	if _, ok := AccensioneDopo(l, t0.Add(6*time.Hour)); ok {
		t.Fatal("nessuna dopo")
	}
}

func TestEffettiSoloQuelliCheNonSiNotano(t *testing.T) {
	a, r, _, s := ambienteFinto()
	m, _ := Trova("effetti")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKCU, kEsplora, "TaskbarAnimations"); !v.Esiste || v.Num != 0 {
		t.Fatalf("barra: %+v", v)
	}
	if len(s.avvisi) == 0 {
		t.Fatal("niente avviso a Windows dopo il cambio della barra")
	}
	if !s.spi[SPIAnimazioni] || !s.spi[SPIFinestre] || !s.spi[SPIOmbre] {
		t.Fatalf("toccate animazioni o ombre: %v", s.spi)
	}
	if es := Rimetti("effetti", a, st, salvaFinto(&n)); es.Stato != VoceRimessa {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKCU, kEsplora, "TaskbarAnimations"); v.Esiste {
		t.Fatalf("non rimesso: %+v", v)
	}
}

// Richiesta di Sergio: Windows resta bello in Gaming e in Ufficio. Nessuna modifica tocca gli
// effetti che si vedono, e nessuna usa le impostazioni del desktop (animazioni, ombre).
func TestWindowsRestaBello(t *testing.T) {
	vietate := []string{"VisualFXSetting", "UserPreferencesMask", "FontSmoothing", "DragFullWindows", "IconsOnly",
		"DisableThumbnails", "ExtendedUIHoverTime", "EnableAeroPeek", "ListviewShadow", "ListviewAlphaSelect",
		"MinAnimate", "EnableTransparency", "CursorShadow", "DropShadow", "AlwaysHibernateThumbnails"}
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		for _, m := range Elenco(modo) {
			if usaSPI(m.G) {
				t.Errorf("%s: %s cambia animazioni o ombre del desktop", modo, m.ID)
			}
			b, _ := json.Marshal(chiaviDi(m.G))
			for _, v := range vietate {
				if strings.Contains(strings.ToLower(string(b)), strings.ToLower(v)) {
					t.Errorf("%s: %s tocca %q", modo, m.ID, v)
				}
			}
		}
	}
	// il giro completo non cambia nessuna impostazione del desktop
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		a, _, _, s := ambienteFinto()
		st := &Stato{}
		n := 0
		for _, m := range Elenco(modo) {
			Esegui(m, a, st, salvaFinto(&n))
		}
		if !s.spi[SPIAnimazioni] || !s.spi[SPIFinestre] || !s.spi[SPIOmbre] {
			t.Errorf("%s: desktop cambiato %v", modo, s.spi)
		}
	}
}

func usaSPI(g Gestore) bool {
	switch x := g.(type) {
	case *spiGest:
		return true
	case *insieme:
		for _, p := range x.parti {
			if usaSPI(p) {
				return true
			}
		}
	}
	return false
}

func TestNotificheGioco(t *testing.T) {
	a, _, _, _ := ambienteFinto()
	m, _ := Trova("notifgioco")
	st := &Stato{}
	n := 0
	if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato != VoceFatta || !a.Pref.NotificheGioco() {
		t.Fatal(es)
	}
	if es := Rimetti("notifgioco", a, st, salvaFinto(&n)); es.Stato != VoceRimessa || a.Pref.NotificheGioco() {
		t.Fatal(es)
	}
}
