package ottimizza

import (
	"reflect"
	"testing"
)

// Prove delle voci aggiunte (RICERCA-OTTIMIZZA-PIU): per ognuna già a posto, applica, rimetti e
// valore che prima non c'era; poi le regole "chi l'ha messo?" con il Boost e con il passo Privacy.

func esegui(t *testing.T, id string, a *Ambiente, st *Stato) Esito {
	t.Helper()
	m, ok := Trova(id)
	if !ok {
		t.Fatalf("%s non trovata", id)
	}
	n := 0
	return Esegui(m, a, st, salvaFinto(&n))
}

func rimetti(t *testing.T, id string, a *Ambiente, st *Stato) Esito {
	t.Helper()
	n := 0
	return Rimetti(id, a, st, salvaFinto(&n))
}

// Le voci fatte solo di valori del registro: con i valori che non c'erano (si cancellano) e con
// valori di prima presenti (si rimettono uguali), e "già a posto".
func TestVociNuoveRegistro(t *testing.T) {
	casi := []struct {
		id   string
		vals []Valore // i valori che la voce mette
	}{
		{"feedback", []Valore{dw(HKCU, kSiuf, "NumberOfSIUFInPeriod", 0), dw(HKCU, kSiuf, "PeriodInNanoSeconds", 0)}},
		{"ricerca", []Valore{dw(HKCU, kCercaImp, "IsDynamicSearchBoxEnabled", 0)}},
		{"esplora", []Valore{dw(HKCU, kEsplora, "ShowSyncProviderNotifications", 0)}},
		{"menustart", []Valore{dw(HKCU, kEsplora, "Start_IrisRecommendations", 0), dw(HKCU, kEsplora, "Start_AccountNotifications", 0)}},
		{"pubblicita", []Valore{dw(HKCU, kPubblicita, "Enabled", 0), dw(HKCU, kPrivacyUt, "TailoredExperiencesWithDiagnosticDataEnabled", 0)}},
		{"consigli", nil}, // sotto, a parte
	}
	for _, c := range casi {
		if c.vals == nil {
			continue
		}
		for _, cera := range []bool{false, true} {
			a, r, _, _ := ambienteFinto()
			if cera {
				for _, v := range c.vals {
					v.Num = 1
					r.metti(v)
				}
			}
			prima := r.copia()
			st := &Stato{}
			if es := esegui(t, c.id, a, st); es.Stato != VoceFatta {
				t.Fatalf("%s (c'era %v): %+v", c.id, cera, es)
			}
			for _, v := range c.vals {
				if x, _ := r.Leggi(v.Radice, v.Chiave, v.Nome); !x.Uguale(v) {
					t.Errorf("%s: %s non messo: %+v", c.id, v.Nome, x)
				}
			}
			// rifatta subito: già sistemata da Tasso, il com'era resta quello vero
			if es := esegui(t, c.id, a, st); es.Stato != VoceAPosto {
				t.Errorf("%s: secondo giro %+v", c.id, es)
			}
			if es := rimetti(t, c.id, a, st); es.Stato != VoceRimessa {
				t.Fatalf("%s: rimetti %+v", c.id, es)
			}
			if !reflect.DeepEqual(prima, r.v) {
				t.Errorf("%s (c'era %v): non tornato com'era\nprima %v\ndopo  %v", c.id, cera, prima, r.v)
			}
			// già a posto
			for _, v := range c.vals {
				r.metti(v)
			}
			st = &Stato{}
			if es := esegui(t, c.id, a, st); es.Stato != VoceAPosto {
				t.Errorf("%s: già a posto non vista: %+v", c.id, es)
			}
		}
	}
}

func TestConsigliAllargati(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kContenuti, "SubscribedContent-338389Enabled", 1))
	prima := r.copia()
	st := &Stato{}
	if es := esegui(t, "consigli", a, st); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	for _, n := range []string{"SubscribedContent-338393Enabled", "SubscribedContent-353694Enabled", "SubscribedContent-353696Enabled", "RotatingLockScreenOverlayEnabled", "SubscribedContent-338387Enabled"} {
		if v, _ := r.Leggi(HKCU, kContenuti, n); !dwordE(v, 0) {
			t.Errorf("%s non spento: %+v", n, v)
		}
	}
	if es := rimetti(t, "consigli", a, st); es.Stato != VoceRimessa || !reflect.DeepEqual(prima, r.v) {
		t.Fatalf("%+v\nprima %v\ndopo  %v", es, prima, r.v)
	}
	// con "Contenuti in evidenza di Windows" nella schermata di blocco le curiosità non si toccano
	a, r, _, _ = ambienteFinto()
	r.metti(dw(HKCU, kContenuti, "RotatingLockScreenEnabled", 1))
	st = &Stato{}
	if es := esegui(t, "consigli", a, st); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	for _, n := range []string{"RotatingLockScreenOverlayEnabled", "SubscribedContent-338387Enabled"} {
		if v, _ := r.Leggi(HKCU, kContenuti, n); v.Esiste {
			t.Errorf("con Contenuti in evidenza %s non va toccato: %+v", n, v)
		}
	}
	if v, _ := r.Leggi(HKCU, kContenuti, "RotatingLockScreenEnabled"); !dwordE(v, 1) {
		t.Error("lo sfondo della schermata di blocco non si cambia")
	}
}

func TestFeedbackTipoDiverso(t *testing.T) {
	a, _, _, _ := ambienteFinto()
	a.Reg = &regTipoN{Registro: a.Reg, nome: "PeriodInNanoSeconds"}
	if es := esegui(t, "feedback", a, &Stato{}); es.Stato != VoceSaltata {
		t.Fatalf("un QWORD scelto in Impostazioni va lasciato: %+v", es)
	}
}

// regTipo: un valore di un tipo che Tasso non scrive (come un QWORD).
type regTipoN struct {
	Registro
	nome string
}

func (r *regTipoN) Leggi(x Radice, k, n string) (Valore, error) {
	if n == r.nome {
		return Valore{Radice: x, Chiave: k, Nome: n}, ErrTipoDiverso
	}
	return r.Registro.Leggi(x, k, n)
}

func TestVociSoloWindows11(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	a.PC.Build = 19045
	prima := r.copia()
	for _, id := range []string{"finestra", "menustart"} {
		if es := esegui(t, id, a, &Stato{}); es.Stato != VoceSaltata {
			t.Errorf("%s su Windows 10: %+v", id, es)
		}
	}
	a.PC.Portatile = true
	if es := esegui(t, "modoac", a, &Stato{}); es.Stato != VoceSaltata {
		t.Errorf("modoac su Windows 10: %+v", es)
	}
	if !reflect.DeepEqual(prima, r.v) {
		t.Error("su Windows 10 non deve cambiare niente")
	}
	// 22621 (22H2): consigli sì, avvisi dell'account (23H2) no
	a.PC.Build = 22621
	st := &Stato{}
	if es := esegui(t, "menustart", a, st); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKCU, kEsplora, "Start_AccountNotifications"); v.Esiste {
		t.Error("su 22H2 Start_AccountNotifications non serve")
	}
}

func TestPiuPrestazioniACorrente(t *testing.T) {
	a, _, e, _ := ambienteFinto()
	// sui fissi no
	if es := esegui(t, "modoac", a, &Stato{}); es.Stato != VoceSaltata {
		t.Fatalf("fisso: %+v", es)
	}
	a.PC.Portatile = true
	// Windows senza la funzione: saltata e detto
	e.senzaModoAC = true
	if es := esegui(t, "modoac", a, &Stato{}); es.Stato != VoceSaltata || es.Motivo != msgModoACWindows {
		t.Fatalf("senza funzione: %+v", es)
	}
	e.senzaModoAC = false
	for _, prima := range []string{OverlayNessuno, ModoEfficienza} {
		e.modoAC = prima
		st := &Stato{}
		if es := esegui(t, "modoac", a, st); es.Stato != VoceFatta || e.modoAC != OverlayMigliori {
			t.Fatalf("%+v %s", es, e.modoAC)
		}
		if e.overlay != OverlayNessuno {
			t.Error("la modalità a batteria (e l'overlay del piano) non si tocca")
		}
		if es := rimetti(t, "modoac", a, st); es.Stato != VoceRimessa || e.modoAC != prima {
			t.Fatalf("rimetti: %+v %s", es, e.modoAC)
		}
	}
	// già a posto
	e.modoAC = OverlayMigliori
	if es := esegui(t, "modoac", a, &Stato{}); es.Stato != VoceAPosto {
		t.Fatal(es)
	}
	// con una modalità di Tasso accesa non si tocca e non si rimette (segno Modo, regola D3)
	e.modoAC = OverlayNessuno
	st := &Stato{}
	esegui(t, "modoac", a, st)
	a.PC.ModoTasso = "gaming"
	if es := rimetti(t, "modoac", a, st); es.Stato != VoceErrore || e.modoAC != OverlayMigliori {
		t.Fatalf("con la modalità Gaming: %+v", es)
	}
	if es := esegui(t, "modoac", a, &Stato{}); es.Stato != VoceSaltata {
		t.Fatalf("con la modalità Gaming: %+v", es)
	}
	a.PC.ModoTasso = ""
	// l'utente l'ha cambiata: resta com'è
	e.modoAC = ModoEfficienza
	if es := rimetti(t, "modoac", a, st); es.Stato != VoceLasciata || e.modoAC != ModoEfficienza {
		t.Fatalf("%+v", es)
	}
	// Windows non la tiene: rimessa subito com'era, non riuscita
	e.modoAC, e.modoACIgnora = OverlayNessuno, true
	if es := esegui(t, "modoac", a, &Stato{}); es.Stato == VoceFatta {
		t.Fatalf("%+v", es)
	}
	// file di stato manomesso: solo i tre GUID di Windows
	m, _ := Trova("modoac")
	if _, ok := m.G.Valida(Precedente{Extra: map[string]string{"modoac": "12345678-1234-1234-1234-123456789012", "messo": OverlayMigliori}}, true); ok {
		t.Error("GUID sconosciuto accettato")
	}
}

func TestScorciatoieAccessibilita(t *testing.T) {
	a, _, _, s := ambienteFinto()
	prima := map[string]uint32{}
	for k, v := range s.tasti {
		prima[k] = v
	}
	st := &Stato{}
	if es := esegui(t, "scorciatoie", a, st); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	for k, v := range s.tasti {
		if v&TastoScorciatoia != 0 || v&TastoConferma != 0 || v != prima[k]&^0xC {
			t.Errorf("%s: %d", k, v)
		}
	}
	if es := esegui(t, "scorciatoie", a, st); es.Stato != VoceAPosto {
		t.Errorf("secondo giro: %+v", es)
	}
	if es := rimetti(t, "scorciatoie", a, st); es.Stato != VoceRimessa || !reflect.DeepEqual(prima, s.tasti) {
		t.Fatalf("%+v %v", es, s.tasti)
	}
	// una funzione accesa (la usa qualcuno) non si tocca mai
	s.tasti[TastiPermanenti] = 511 // Tasti permanenti accesi
	st = &Stato{}
	if es := esegui(t, "scorciatoie", a, st); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	if s.tasti[TastiPermanenti] != 511 {
		t.Errorf("Tasti permanenti accesi toccati: %d", s.tasti[TastiPermanenti])
	}
	// accesa dopo il cambio: rimettendo resta accesa
	s.tasti[TastiFiltro] |= TastoAcceso
	if es := rimetti(t, "scorciatoie", a, st); es.Stato != VoceLasciata || s.tasti[TastiFiltro]&TastoAcceso == 0 || s.tasti[TastiAvviso] != prima[TastiAvviso] {
		t.Fatalf("%+v %v", es, s.tasti)
	}
	// tutte già spente o accese: già a posto
	s.tasti = map[string]uint32{TastiPermanenti: 511, TastiFiltro: 122, TastiAvviso: 58}
	if es := esegui(t, "scorciatoie", a, &Stato{}); es.Stato != VoceAPosto {
		t.Fatal(es)
	}
	// file manomesso: niente valori con la funzione accesa o con altri bit cambiati
	g := &tastoGest{id: TastiPermanenti}
	for _, x := range []map[string]string{{"tasti": TastiPermanenti, "prima": "511", "messo": "499"}, {"tasti": TastiPermanenti, "prima": "510", "messo": "0"}, {"tasti": "altro", "prima": "510", "messo": "498"}} {
		if _, ok := g.Valida(Precedente{Extra: x}, true); ok {
			t.Errorf("accettato %v", x)
		}
	}
	if _, ok := g.Valida(Precedente{Extra: map[string]string{"tasti": TastiPermanenti, "prima": "510", "messo": "498"}}, true); !ok {
		t.Error("valido scartato")
	}
}

func TestStringaDirectX(t *testing.T) {
	s := "VRROptimizeEnable=0;AutoHDREnable=1;"
	if g := DXConParte(s, DXFinestra, "1"); g != "VRROptimizeEnable=0;AutoHDREnable=1;SwapEffectUpgradeEnable=1;" {
		t.Error(g)
	}
	if g := DXSenzaParte("SwapEffectUpgradeEnable=1;VRROptimizeEnable=0;", DXFinestra); g != "VRROptimizeEnable=0;" {
		t.Error(g)
	}
	if !DXSceltoSpento("AutoHDREnable=1;SwapEffectUpgradeEnable=0;") || DXSceltoSpento("") {
		t.Error("scelta spenta")
	}
}

func TestGiochiInFinestra(t *testing.T) {
	for _, prima := range []string{"", "VRROptimizeEnable=0;AutoHDREnable=1;"} {
		a, r, _, _ := ambienteFinto()
		if prima != "" {
			r.metti(sz(HKCU, ChiaveGPU, NomeDXGlobale, prima))
		}
		vecchio := r.copia()
		st := &Stato{}
		if es := esegui(t, "finestra", a, st); es.Stato != VoceFatta {
			t.Fatal(es)
		}
		v, _ := r.Leggi(HKCU, ChiaveGPU, NomeDXGlobale)
		if x, _ := DXParte(v.Testo, DXFinestra); x != "1" {
			t.Fatalf("%+v", v)
		}
		if prima != "" && v.Testo != prima+"SwapEffectUpgradeEnable=1;" {
			t.Errorf("le altre parti devono restare: %q", v.Testo)
		}
		// nel frattempo Windows ha aggiunto una parte: rimettendo resta, si toglie solo quella di Tasso
		if prima != "" {
			r.metti(sz(HKCU, ChiaveGPU, NomeDXGlobale, v.Testo+"Nuova=1;"))
			if es := rimetti(t, "finestra", a, st); es.Stato != VoceRimessa {
				t.Fatal(es)
			}
			if v, _ := r.Leggi(HKCU, ChiaveGPU, NomeDXGlobale); v.Testo != prima+"Nuova=1;" {
				t.Errorf("rimessa male: %q", v.Testo)
			}
			continue
		}
		if es := rimetti(t, "finestra", a, st); es.Stato != VoceRimessa || !reflect.DeepEqual(vecchio, r.v) {
			t.Fatalf("%+v %v", es, r.v)
		}
	}
	// spenta da te in Impostazioni: lasciata
	a, r, _, _ := ambienteFinto()
	r.metti(sz(HKCU, ChiaveGPU, NomeDXGlobale, "SwapEffectUpgradeEnable=0;"))
	if es := esegui(t, "finestra", a, &Stato{}); es.Stato != VoceSaltata {
		t.Fatal(es)
	}
	// già accesa
	r.metti(sz(HKCU, ChiaveGPU, NomeDXGlobale, "SwapEffectUpgradeEnable=1;"))
	if es := esegui(t, "finestra", a, &Stato{}); es.Stato != VoceAPosto {
		t.Fatal(es)
	}
}

// D3 con il Boost: se il Boost l'ha accesa la lascia a lui; mentre il Boost la tiene non si rimette.
func TestGiochiInFinestraEBoost(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	a.PC.BoostFinestra = true
	if es := esegui(t, "finestra", a, &Stato{}); es.Stato != VoceSaltata || es.Motivo != MsgBoostAcceso {
		t.Fatal(es)
	}
	a.PC.BoostFinestra = false
	st := &Stato{}
	if es := esegui(t, "finestra", a, st); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	a.PC.BoostFinestra = true // poi l'utente accende il Boost
	if es := rimetti(t, "finestra", a, st); es.Stato != VoceErrore || es.Motivo != MsgBoostRimetti {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKCU, ChiaveGPU, NomeDXGlobale); v.Testo != "SwapEffectUpgradeEnable=1;" {
		t.Fatalf("toccata mentre il Boost la tiene: %+v", v)
	}
	if st.Voce("finestra") == nil || !st.Voce("finestra").Attiva() {
		t.Fatal("il com'era non si perde")
	}
	a.PC.BoostFinestra = false // Boost spento (rimette la sua parte, cioè "1"): adesso si rimette
	if es := rimetti(t, "finestra", a, st); es.Stato != VoceRimessa {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKCU, ChiaveGPU, NomeDXGlobale); v.Esiste {
		t.Fatalf("non c'era: %+v", v)
	}
}

// Livello B: Windows non tiene il valore → saltata con il motivo, niente lasciato cambiato.
func TestLivelloBNonAccettato(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.ignora[chiaveFinta(HKCU, kCercaImp, "IsDynamicSearchBoxEnabled")] = true
	st := &Stato{}
	es := esegui(t, "ricerca", a, st)
	if es.Stato != VoceSaltata || es.Motivo != MsgNonAccettato {
		t.Fatalf("%+v", es)
	}
	if v := st.Voce("ricerca"); v == nil || v.Attiva() {
		t.Fatal("niente da rimettere")
	}
}

// Il passo Privacy: "Privacy: ripristina" rimette i valori di prima (non 1 o cancellato).
func TestPrivacyRipristinaComEra(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kPubblicita, "Enabled", 1))
	r.metti(dw(HKCU, kContenuti, "SoftLandingEnabled", 0))            // già spento: non si tocca
	r.metti(dw(HKCU, kContenuti, "SystemPaneSuggestionsEnabled", 7))  // un valore strano: torna 7
	r.metti(dw(HKLM, kTelemetria, "AllowTelemetry", 3))               // di base sui PC di casa
	r.metti(dw(HKCU, kExplorerPol, "DisableSearchBoxSuggestions", 0)) // c'era a 0: torna 0
	prima := r.copia()
	p := Privacy()
	st := &Stato{}
	n := 0
	if es := Esegui(p, a, st, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKLM, kTelemetria, "AllowTelemetry"); !dwordE(v, 1) {
		t.Fatal(v)
	}
	if v, _ := r.Leggi(HKCU, kExplorerPol, "DisableSearchBoxSuggestions"); !dwordE(v, 1) {
		t.Fatal(v)
	}
	// rilettura del file di stato (dopo un riavvio)
	b, _ := st.Codifica()
	st2, sc, err := Leggi(b, true)
	if err != nil || sc != 0 {
		t.Fatal(err, sc)
	}
	// l'utente riaccende a mano l'ID pubblicitario: quello resta
	r.metti(dw(HKCU, kPubblicita, "Enabled", 1))
	if es := RimettiM(p, a, st2, salvaFinto(&n)); es.Stato != VoceLasciata {
		t.Fatal(es)
	}
	if !reflect.DeepEqual(prima, r.v) {
		t.Errorf("non tornato com'era\nprima %v\ndopo  %v", prima, r.v)
	}
	// telemetria già a 0 (decisa da un'azienda): non si alza
	a, r, _, _ = ambienteFinto()
	r.metti(dw(HKLM, kTelemetria, "AllowTelemetry", 0))
	Esegui(p, a, &Stato{}, salvaFinto(&n))
	if v, _ := r.Leggi(HKLM, kTelemetria, "AllowTelemetry"); !dwordE(v, 0) {
		t.Fatal("telemetria alzata", v)
	}
	// senza amministratore: il criterio di tutto il PC si salta, il resto si fa
	a, r, _, _ = ambienteFinto()
	a.PC.Admin = false
	if es := Esegui(p, a, &Stato{}, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	if v, _ := r.Leggi(HKLM, kTelemetria, "AllowTelemetry"); v.Esiste {
		t.Fatal("senza permessi non si scrive HKLM")
	}
}

// "Chi l'ha messo?" tra Ottimizza tutto e Privacy: Ottimizza prima, poi Privacy, poi "Rimetti com'era"
// di Ottimizza: i valori di Privacy restano; "Privacy: ripristina" rimette quelli veri di prima.
func TestOttimizzaPoiPrivacy(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kContenuti, "SubscribedContent-338389Enabled", 1))
	r.metti(dw(HKCU, kPubblicita, "Enabled", 1))
	prima := r.copia()
	stO := &Stato{}
	for _, id := range []string{"consigli", "pubblicita"} {
		if es := esegui(t, id, a, stO); es.Stato != VoceFatta {
			t.Fatal(id, es)
		}
	}
	// Privacy dopo: eredita il com'era vero dei valori messi da Ottimizza
	ap := *a
	ap.Eredita = func(v Valore) (Valore, bool) { return PrimaVera(stO, v) }
	stP := &Stato{}
	n := 0
	if es := Esegui(Privacy(), &ap, stP, salvaFinto(&n)); es.Stato != VoceFatta {
		t.Fatal(es)
	}
	// Rimetti com'era di Ottimizza: non disfa la Privacy
	ao := *a
	ao.Tenuti = ValoriTenuti(stP)
	RimettiTutto(&ao, stO, salvaFinto(&n))
	for _, x := range [][2]string{{kContenuti, "SubscribedContent-338389Enabled"}, {kPubblicita, "Enabled"}} {
		if v, _ := r.Leggi(HKCU, x[0], x[1]); !dwordE(v, 0) {
			t.Errorf("Ottimizza ha disfatto la Privacy: %s %+v", x[1], v)
		}
	}
	// valori solo di Ottimizza (non della Privacy): rimessi
	if v, _ := r.Leggi(HKCU, kEngagement, "ScoobeSystemSettingEnabled"); v.Esiste {
		t.Errorf("Scoobe non rimesso: %+v", v)
	}
	// Privacy: ripristina → tutto com'era prima di Tasso
	if es := RimettiM(Privacy(), a, stP, salvaFinto(&n)); es.Stato != VoceRimessa {
		t.Fatal(es)
	}
	if !reflect.DeepEqual(prima, r.v) {
		t.Errorf("non tornato com'era\nprima %v\ndopo  %v", prima, r.v)
	}
}

// Privacy prima, poi Ottimizza: Ottimizza trova i valori già a posto e non se li prende.
func TestPrivacyPoiOttimizza(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kPubblicita, "Enabled", 1))
	prima := r.copia()
	stP := &Stato{}
	n := 0
	Esegui(Privacy(), a, stP, salvaFinto(&n))
	stO := &Stato{}
	if es := esegui(t, "pubblicita", a, stO); es.Stato != VoceAPosto {
		t.Fatal(es)
	}
	RimettiTutto(a, stO, salvaFinto(&n))
	if v, _ := r.Leggi(HKCU, kPubblicita, "Enabled"); !dwordE(v, 0) {
		t.Fatal("Ottimizza ha disfatto la Privacy")
	}
	RimettiM(Privacy(), a, stP, salvaFinto(&n))
	if !reflect.DeepEqual(prima, r.v) {
		t.Errorf("non tornato com'era\nprima %v\ndopo  %v", prima, r.v)
	}
}

// Giro completo in Gaming su un portatile Windows 11 (anche la modalità a corrente) e ritorno identico.
func TestGiroPortatileTornaComEra(t *testing.T) {
	a, r, e, s := ambienteFinto()
	a.PC.Portatile = true
	primaReg, primaTasti, primaModo := r.copia(), map[string]uint32{}, e.modoAC
	for k, v := range s.tasti {
		primaTasti[k] = v
	}
	st := &Stato{}
	n := 0
	fatte := map[string]bool{}
	for _, m := range Elenco(ModoGioco) {
		if es := Esegui(m, a, st, salvaFinto(&n)); es.Stato == VoceFatta {
			fatte[m.ID] = true
		}
	}
	for _, id := range []string{"modoac", "scorciatoie", "finestra", "consigli", "feedback", "ricerca"} {
		if !fatte[id] {
			t.Errorf("%s non fatta", id)
		}
	}
	if _, ok := RimettiTutto(a, st, salvaFinto(&n)); !ok {
		t.Fatal("non tutto rimesso")
	}
	if !reflect.DeepEqual(primaReg, r.v) || !reflect.DeepEqual(primaTasti, s.tasti) || e.modoAC != primaModo {
		t.Errorf("non tornato com'era: %v %v %s", r.v, s.tasti, e.modoAC)
	}
}

func TestPrimaVera(t *testing.T) {
	a, r, _, _ := ambienteFinto()
	r.metti(dw(HKCU, kPubblicita, "Enabled", 1))
	st := &Stato{}
	esegui(t, "pubblicita", a, st)
	v, ok := PrimaVera(st, dw(HKCU, kPubblicita, "Enabled", 0))
	if !ok || !dwordE(v, 1) {
		t.Fatal(v, ok)
	}
	if _, ok := PrimaVera(st, dw(HKCU, kPubblicita, "Enabled", 1)); ok {
		t.Fatal("cambiato dall'utente: non è più di Tasso")
	}
	if v, ok := PrimaVera(st, dw(HKCU, kPrivacyUt, "TailoredExperiencesWithDiagnosticDataEnabled", 0)); !ok || v.Esiste {
		t.Fatal("non c'era", v, ok)
	}
}
