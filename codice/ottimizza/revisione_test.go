package ottimizza

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// C1: 5 valori a 1, Tasso li porta a 0, l'utente ne rimette uno a 1, secondo giro, Rimetti tutto:
// tutti e 5 tornano a 1 (prima 3 restavano a 0 per sempre).
func TestCinqueValoriCambiatiInParte(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	nomi := []string{"SubscribedContent-338389Enabled", "SoftLandingEnabled", "SystemPaneSuggestionsEnabled", "SubscribedContent-310093Enabled"}
	for _, n := range nomi {
		r.metti(dw(HKCU, kContenuti, n, 1))
	}
	r.metti(dw(HKCU, kEngagement, "ScoobeSystemSettingEnabled", 1))
	prima := r.copia()
	m, _ := Trova("consigli")
	st := &Stato{}
	n := 0
	if e := Esegui(m, a, st, salvaFinto(&n)); e.Stato != VoceFatta {
		t.Fatal(e)
	}
	r.metti(dw(HKCU, kContenuti, nomi[0], 1)) // l'utente ne riaccende uno
	if e := Esegui(m, a, st, salvaFinto(&n)); e.Stato != VoceFatta {
		t.Fatalf("secondo giro: %+v", e)
	}
	if _, ok := RimettiTutto(a, st, salvaFinto(&n)); !ok {
		t.Fatal("rimetti tutto")
	}
	if !reflect.DeepEqual(prima, r.v) {
		t.Fatalf("valori non tornati com'erano:\nprima %v\ndopo  %v", prima, r.v)
	}
}

// C1: un Applica interrotto a metà (voce "incorso" con solo una parte scritta) non perde niente.
func TestApplicaInterrottoAMeta(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	m, _ := Trova("bgapps")
	prima := r.copia()
	p, _ := m.G.Prepara(a)
	st := &Stato{}
	st.Metti(Voce{ID: "bgapps", Stato: VoceInCorso, Prima: p})
	_ = r.Scrivi(p.Messi[0]) // il PC si spegne dopo il primo valore
	n := 0
	Esegui(m, a, st, salvaFinto(&n)) // giro successivo
	RimettiTutto(a, st, salvaFinto(&n))
	if !reflect.DeepEqual(prima, r.v) {
		t.Fatalf("non tornato com'era: %v", r.v)
	}
}

// C8: se Applica fallisce e anche il rimetti fallisce, il com'era resta salvato.
func TestApplicaERimettiFalliscono(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	m, _ := Trova("bgapps")
	// il secondo valore non si scrive; poi tutta la parte dell'utente diventa illeggibile
	r.rotte[chiaveFinta(HKCU, kRicerca, "BackgroundAppGlobalToggle")] = true
	st := &Stato{}
	n := 0
	salva := func(s *Stato) error {
		n++
		if v := s.Voce("bgapps"); v != nil && v.Stato == VoceInCorso {
			r.bloccate[HKCU] = true // da qui il rimetti non riesce
		}
		return nil
	}
	e := Esegui(m, a, st, salva)
	if e.Stato != VoceErrore {
		t.Fatalf("%+v", e)
	}
	v := st.Voce("bgapps")
	if v == nil || !v.Attiva() || len(v.Prima.Messi) == 0 {
		t.Fatalf("com'era perso: %+v", v)
	}
}

// C4: un valore cambiato in un altro tipo vale come "l'hai cambiato tu", non blocca per sempre.
type regTipo struct{ *regFinto }

func (r regTipo) Leggi(rad Radice, k, n string) (Valore, error) {
	if n == "HistoricalCaptureEnabled" {
		return Valore{}, ErrTipoDiverso
	}
	return r.regFinto.Leggi(rad, k, n)
}

func TestTipoDiversoNonBlocca(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kGameDVR, "HistoricalCaptureEnabled", 1))
	m, _ := Trova("registrazione")
	st := &Stato{}
	n := 0
	Esegui(m, a, st, salvaFinto(&n))
	a.Reg = regTipo{r}
	if e := Rimetti("registrazione", a, st, salvaFinto(&n)); e.Stato != VoceLasciata {
		t.Fatalf("%+v", e)
	}
	if len(st.Attive()) != 0 {
		t.Fatal("voce ancora attiva")
	}
}

// C5: senza i dati del PC, nel dubbio è un portatile e il piano non si cambia.
func TestPortatileNelDubbio(t *testing.T) {
	casi := []struct {
		letto, regola bool
		batt          uint8
		port, dubbio  bool
	}{
		{true, true, 1, true, false}, {true, false, 1, false, false}, {false, false, 255, true, true},
		{false, false, 1, true, true}, {false, false, 128, false, false}, {true, false, 128, false, false},
	}
	for _, c := range casi {
		p, d := DecidiPortatile(c.letto, c.regola, c.batt)
		if p != c.port || d != c.dubbio {
			t.Errorf("%+v: %v %v", c, p, d)
		}
	}
	a, _, e, _ := ambienteFinto()
	a.PC.Portatile, a.PC.PortatileDubbio = DecidiPortatile(false, false, 255)
	m, _ := Trova("piano")
	n := 0
	if es := Esegui(m, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != msgPianoDubbio || e.attivo != PianoBilanciato {
		t.Fatalf("%+v %s", es, e.attivo)
	}
}

// C6: le notifiche si riaccendono solo se sono ancora spente come le ha messe Tasso, mai spente per sempre.
func TestNotificheDaRimettere(t *testing.T) {
	casi := []struct {
		prima string
		v     uint32
		ok    bool
		vuole string
	}{
		{"none", 0, true, "cancella"}, {"1", 0, true, "accendi"}, {"rotto", 0, true, "accendi"},
		{"1", 1, true, "lascia"}, {"none", 0, false, "lascia"},
	}
	for _, c := range casi {
		if g := NotificheDaRimettere(c.prima, c.v, c.ok); g != c.vuole {
			t.Errorf("%+v: %s", c, g)
		}
	}
}

// C3: giro senza permessi (file dell'utente), poi Tasso da amministratore unisce e rimette tutto.
func TestGiroSenzaPermessiPoiAdmin(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	prima := r.copia()
	a.PC.Admin = false
	r.bloccate[HKLM], r.bloccate[HKNS] = true, true
	stU := &Stato{}
	n := 0
	for _, m := range Elenco(ModoGioco) {
		Esegui(m, a, stU, salvaFinto(&n))
	}
	if len(stU.Attive()) == 0 {
		t.Fatal("niente fatto senza permessi")
	}
	b, _ := stU.Codifica()
	letto, _, err := Leggi(b, false)
	if err != nil {
		t.Fatal(err)
	}
	// da amministratore: file protetto vuoto + file dell'utente
	r.bloccate = map[Radice]bool{}
	a.PC.Admin = true
	stP := &Stato{}
	if k := stP.Unisci(letto); k != len(stU.Attive()) {
		t.Fatalf("unite %d su %d", k, len(stU.Attive()))
	}
	if _, ok := RimettiTutto(a, stP, salvaFinto(&n)); !ok || len(stP.Attive()) != 0 {
		t.Fatal("non tutto rimesso")
	}
	if !reflect.DeepEqual(prima, r.v) {
		t.Fatalf("registro non tornato: %v", r.v)
	}
}

// C2: con un altro account amministratore le impostazioni dell'utente si saltano.
func TestAltroUtente(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	a.PC.AltroUtente = true
	prima := r.copia()
	st := &Stato{}
	n := 0
	for _, m := range Elenco(ModoGioco) {
		e := Esegui(m, a, st, salvaFinto(&n))
		if m.Utente && (e.Stato != VoceSaltata || e.Motivo != MsgAltroUtente) {
			t.Errorf("%s: %+v", m.ID, e)
		}
	}
	for k, v := range r.v {
		if v.Radice == HKCU && !reflect.DeepEqual(prima[k], v) {
			t.Errorf("toccato HKCU: %s", k)
		}
	}
}

// D3: con una modalità di Tasso accesa piano, mouse e Modalità Gioco si lasciano a lei, anche nel rimetti.
func TestModalitaTassoAccesa(t *testing.T) {
	a, _, e, _ := ambienteFinto()
	st := &Stato{}
	n := 0
	piano, _ := Trova("piano")
	Esegui(piano, a, st, salvaFinto(&n))
	a.PC.ModoTasso = "gaming"
	if es := Rimetti("piano", a, st, salvaFinto(&n)); es.Stato != VoceErrore || es.Motivo != MsgModoRimetti || len(st.Attive()) != 1 {
		t.Fatalf("rimetti con la modalità accesa: %+v", es)
	}
	mouse, _ := Trova("mouse")
	if es := Esegui(mouse, a, st, salvaFinto(&n)); es.Stato != VoceSaltata || es.Motivo != MsgModoAcceso {
		t.Fatalf("%+v", es)
	}
	a.PC.ModoTasso = ""
	if es := Rimetti("piano", a, st, salvaFinto(&n)); es.Stato != VoceRimessa || e.attivo != PianoBilanciato {
		t.Fatalf("%+v", es)
	}
}

// D8 e D4: "Lascia decidere a Windows" e la pianificazione GPU spenta dall'utente si rispettano.
func TestSceltaUtenteRispettata(t *testing.T) {
	if !GPUScelta("GpuPreference=0;") || GPUScelta("AutoHDREnable=1;") {
		t.Fatal("GPUScelta")
	}
	a, r, _, _ := ambienteFinto()
	r.metti(sz(HKCU, ChiaveGPU, a.PC.Giochi[0], "GpuPreference=0;"))
	g, _ := Trova("gpupref")
	n := 0
	if e := Esegui(g, a, &Stato{}, salvaFinto(&n)); e.Stato != VoceAPosto {
		t.Fatalf("Lascia decidere a Windows: %+v", e)
	}
	r.metti(dw(HKLM, kGrafica, "HwSchMode", 1))
	h, _ := Trova("hags")
	if e := Esegui(h, a, &Stato{}, salvaFinto(&n)); e.Stato != VoceSaltata {
		t.Fatalf("HAGS spenta dall'utente: %+v", e)
	}
}

// D2: gli orari si ricontrollano dopo un'attesa; se Windows li rimette, la voce si salta con errore.
func TestOrariVerificaLenta(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	attese := 0
	a.Attendi = func(d time.Duration) { attese++; r.metti(dw(HKLM, kAggUX, "ActiveHoursEnd", 17)) }
	m, _ := Trova("orari")
	n := 0
	e := Esegui(m, a, &Stato{}, salvaFinto(&n))
	if attese != 1 || e.Stato != VoceErrore || e.Motivo != MsgNonResta {
		t.Fatalf("%d %+v", attese, e)
	}
}

// D5: nel .reg solo i valori toccati, con "nome"=- per quelli che non c'erano.
func TestTestoReg(t *testing.T) {
	got := TestoReg([]Valore{dw(HKLM, kGrafica, "HwSchMode", 1), {Radice: HKCU, Chiave: chiaveMouse, Nome: "MouseSpeed"}, sz(HKCU, chiaveMouse, "MouseThreshold1", `6"\`), dw(HKNS, kRecapito, "DownloadMode", 3)})
	vuole := "Windows Registry Editor Version 5.00\r\n\r\n[HKEY_LOCAL_MACHINE\\SYSTEM\\CurrentControlSet\\Control\\GraphicsDrivers]\r\n\"HwSchMode\"=dword:00000001\r\n" +
		"\r\n[HKEY_CURRENT_USER\\Control Panel\\Mouse]\r\n\"MouseSpeed\"=-\r\n\"MouseThreshold1\"=\"6\\\"\\\\\"\r\n" +
		"\r\n[HKEY_USERS\\S-1-5-20\\Software\\Microsoft\\Windows\\CurrentVersion\\DeliveryOptimization\\Settings]\r\n\"DownloadMode\"=dword:00000003\r\n"
	if got != vuole {
		t.Fatalf("%q\n%q", got, vuole)
	}
}

// R2 (verifica a0059f5): dopo Togli Tasso, per un altro account resta un .reg con i soli valori
// HKCU di prima delle modifiche ancora attive.
func TestValoriUtente(t *testing.T) {
	s := &Stato{Voci: []Voce{
		{ID: "mouse", Stato: VoceFatta, Prima: Precedente{Valori: []Valore{sz(HKCU, chiaveMouse, "MouseSpeed", "1")}}},
		{ID: "hags", Stato: VoceFatta, Prima: Precedente{Valori: []Valore{dw(HKLM, kGrafica, "HwSchMode", 1)}}},
		{ID: "effetti", Stato: VoceInCorso, Prima: Precedente{Parti: []Precedente{{Valori: []Valore{{Radice: HKCU, Chiave: chiaveMouse, Nome: "Altro"}, sz(HKCU, chiaveMouse, "mousespeed", "9")}}}}},
		{ID: "suoni", Stato: VoceRimessa, Prima: Precedente{Valori: []Valore{dw(HKCU, `Software\X`, "Y", 1)}}},
	}}
	v := ValoriUtente(s)
	if len(v) != 2 || v[0].Nome != "MouseSpeed" || v[0].Testo != "1" || v[1].Nome != "Altro" {
		t.Fatalf("valori: %+v", v)
	}
	if r := TestoReg(v); !strings.Contains(r, `"MouseSpeed"="1"`) || !strings.Contains(r, `"Altro"=-`) || strings.Contains(r, "HKEY_LOCAL_MACHINE") {
		t.Fatalf("reg: %q", r)
	}
}
