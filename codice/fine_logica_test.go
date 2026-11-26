package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSceltaFine(t *testing.T) {
	casi := []struct {
		in   interface{}
		want string
	}{
		{true, fineSpegni}, {false, fineAcceso}, // come la 3.1 (spegni sì/no)
		{"riavvia", fineRiavvia}, {"Spegni", fineSpegni}, {"acceso", fineAcceso},
		{"", fineAcceso}, {"boh", fineAcceso}, {nil, fineAcceso}, {3.0, fineAcceso},
	}
	for _, c := range casi {
		if g := sceltaFine(c.in); g != c.want {
			t.Errorf("sceltaFine(%#v) = %q, voglio %q", c.in, g, c.want)
		}
	}
}

func ids(sel []Step) []string {
	out := []string{}
	for _, s := range sel {
		out = append(out, s.ID)
	}
	return out
}

func TestWinupdatePerUltimo(t *testing.T) {
	sel := []Step{{ID: "backup"}, {ID: "drivers"}, {ID: "winupdate"}, {ID: "startup"}, {ID: "defender"}}
	if g, w := ids(winupdatePerUltimo(sel)), []string{"backup", "drivers", "startup", "defender", "winupdate"}; !reflect.DeepEqual(g, w) {
		t.Errorf("ordine %v, voglio %v", g, w)
	}
	if g, w := ids(winupdatePerUltimo([]Step{{ID: "a"}, {ID: "b"}})), []string{"a", "b"}; !reflect.DeepEqual(g, w) {
		t.Errorf("senza winupdate l'ordine non cambia: %v", g)
	}
	if g, w := ids(senzaPasso(sel, "winupdate")), []string{"backup", "drivers", "startup", "defender"}; !reflect.DeepEqual(g, w) {
		t.Errorf("senzaPasso: %v", g)
	}
	// Nei livelli veri (4 e 5) winupdate oggi sta in mezzo: dopo l'ordinamento è l'ultimo.
	for lv := 1; lv <= 5; lv++ {
		var sel []Step
		for _, s := range allSteps {
			if s.Level > 0 && s.Level <= lv {
				sel = append(sel, s)
			}
		}
		o := winupdatePerUltimo(sel)
		for i, s := range o {
			if s.ID == "winupdate" && i != len(o)-1 {
				t.Errorf("livello %d: winupdate al posto %d di %d", lv, i+1, len(o))
			}
		}
		if len(o) != len(sel) {
			t.Errorf("livello %d: passaggi persi", lv)
		}
	}
}

// Riavvio (bug del 26/11): codice di uscita di shutdown.exe controllato, 1190 = annullo e riprovo una volta.
func TestShutdownConRiprova(t *testing.T) {
	finta := func(codici ...int) (func(args ...string) int, *[]string) {
		var chiamate []string
		i := 0
		return func(args ...string) int {
			chiamate = append(chiamate, strings.Join(args, " "))
			if args[0] == "/a" {
				return 0
			}
			c := codici[i]
			if i < len(codici)-1 {
				i++
			}
			return c
		}, &chiamate
	}
	args := []string{"/r", "/t", "60"}

	run, ch := finta(0)
	if e := shutdownConRiprova(run, args); !e.OK || e.Riprovato || len(*ch) != 1 {
		t.Errorf("accettato subito: %+v %v", e, *ch)
	}
	run, ch = finta(1190, 0)
	if e := shutdownConRiprova(run, args); !e.OK || !e.Riprovato || !reflect.DeepEqual(*ch, []string{"/r /t 60", "/a", "/r /t 60"}) {
		t.Errorf("1190 poi ok: %+v %v", e, *ch)
	}
	run, ch = finta(1190, 1190)
	if e := shutdownConRiprova(run, args); e.OK || e.Codice != 1190 || len(*ch) != 3 {
		t.Errorf("1190 due volte: deve fallire dopo una sola riprova: %+v %v", e, *ch)
	}
	run, ch = finta(1115)
	if e := shutdownConRiprova(run, args); e.OK || e.Codice != 1115 || len(*ch) != 1 {
		t.Errorf("1115: niente riprova con shutdown.exe (ci pensa ExitWindowsEx): %+v %v", e, *ch)
	}
	run, _ = finta(-1)
	if e := shutdownConRiprova(run, args); e.OK {
		t.Errorf("shutdown.exe non partito: %+v", e)
	}
}

func TestPassiInVista(t *testing.T) {
	p := []passoFatto{{"a", 10, false}, {"b", 500, false}, {"c", 0, true}, {"d", 300, false}, {"e", 0, false},
		{"f", 7, false}, {"g", 1, false}, {"h", 2, false}, {"i", 0, true}}
	v, altri, salt := passiInVista(p, 6)
	if g := []string{v[0].Nome, v[1].Nome, v[2].Nome}; !reflect.DeepEqual(g, []string{"b", "d", "a"}) || len(v) != 6 || altri != 1 {
		t.Errorf("vista %v altri %d", v, altri)
	}
	if !reflect.DeepEqual(salt, []string{"c", "i"}) {
		t.Errorf("saltati %v", salt)
	}
}

func TestBentornatoDaMostrare(t *testing.T) {
	r := riepilogoPulizia{Fine: fineRiavvia, Quando: 1000}
	if !bentornatoDaMostrare(r, 1500, 2000) {
		t.Error("riavviato dopo la pulizia: va mostrato")
	}
	if bentornatoDaMostrare(r, 900, 2000) {
		t.Error("Windows acceso prima della fine della pulizia (riavvio annullato): non va mostrato")
	}
	if bentornatoDaMostrare(riepilogoPulizia{Fine: fineAcceso, Quando: 1000}, 1500, 2000) {
		t.Error("Lascia acceso: niente Bentornato")
	}
	if bentornatoDaMostrare(r, 1500, 1000+8*24*3600) {
		t.Error("più vecchio di 7 giorni: non va mostrato")
	}
}

// I testi nuovi di Go e degli script della 3.1.1 sono tradotti in tutte le lingue.
func TestTestiFineTradotti(t *testing.T) {
	goKeys := []string{
		"Quando ha finito: riavvio. Il PC si riavvia tra 60 secondi (puoi annullarlo da Tasso).",
		"Quando ha finito: spengo. Il PC si spegne tra 60 secondi (puoi annullarlo da Tasso).",
		"Tasso ha finito la manutenzione: il PC si spegne tra 60 secondi.",
		"Tasso ha finito la manutenzione: il PC si riavvia tra 60 secondi.",
		"Windows non ha accettato il riavvio o lo spegnimento: lo puoi chiedere da Tasso con il pulsante.",
	}
	psKeys := []string{
		"Windows aspetta già un riavvio per finire un aggiornamento: salto questo passaggio. Dopo il riavvio Windows lo finisce da solo.",
		"Windows aspetta un riavvio per finire un aggiornamento: il backup può metterci di più o non riuscire. Ci provo per al massimo 5 minuti, poi vado avanti.",
		"Cerco gli aggiornamenti...",
		"Scarico gli aggiornamenti: {0} di {1}",
		"Installo {0} aggiornamenti: {1} di {2} fatti",
		"Salto un aggiornamento che potrebbe farti delle domande: ",
		" (lo puoi fare da Impostazioni > Windows Update).",
		"Download non riuscito: ",
		"Non sono riuscito a scaricare gli aggiornamenti: Windows ci riprova da solo più tardi.",
	}
	for _, l := range []string{"en", "es", "fr", "de"} {
		b, err := os.ReadFile("i18n/" + l + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var d map[string]map[string]string
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		for _, k := range goKeys {
			if strings.TrimSpace(d["go"][k]) == "" {
				t.Errorf("%s.json/go: manca %q", l, k)
			}
		}
		for _, k := range psKeys {
			if strings.TrimSpace(d["ps"][k]) == "" {
				t.Errorf("%s.json/ps: manca %q", l, k)
			}
		}
	}
}

// Riavvio che "parte" ma non parte (portatile di Sergio): anche con shutdown.exe che risponde ok
// Tasso arma la sua riserva (ExitWindowsEx dopo il tempo + riservaSecondi); se shutdown.exe non
// accetta, il conto alla rovescia lo tiene Tasso; senza privilegio la finestra lo dice.
func TestPianificaFineCon(t *testing.T) {
	type giro struct {
		codici []int
		puo    bool
		ok     bool
		arma   []time.Duration
	}
	casi := []giro{
		{[]int{0}, true, true, []time.Duration{80 * time.Second}},          // accettato: riserva a 60+20 s
		{[]int{1190, 0}, true, true, []time.Duration{80 * time.Second}},    // ce n'era già uno: /a e riprova
		{[]int{1115}, true, true, []time.Duration{60 * time.Second}},       // non accettato: conto di Tasso
		{[]int{1190, 1190}, true, true, []time.Duration{60 * time.Second}}, // riprova fallita: conto di Tasso
		{[]int{-1}, false, false, nil},                                     // niente privilegio: "no"
	}
	for i, c := range casi {
		var armati []time.Duration
		var args [][]string
		n := 0
		d := fineDeps{
			shutdown: func(a ...string) int {
				args = append(args, a)
				if a[0] == "/a" {
					return 0
				}
				x := c.codici[n]
				if n < len(c.codici)-1 {
					n++
				}
				return x
			},
			puoSpegnere: func() bool { return c.puo },
			arma:        func(dopo time.Duration) { armati = append(armati, dopo) },
			esci:        func() error { t.Errorf("caso %d: ExitWindowsEx subito non va chiamato qui", i); return nil },
		}
		if g := pianificaFineCon(d, true, 60, "msg"); g != c.ok {
			t.Errorf("caso %d: ok=%v, voglio %v", i, g, c.ok)
		}
		if !reflect.DeepEqual(armati, c.arma) {
			t.Errorf("caso %d: armati %v, voglio %v", i, armati, c.arma)
		}
		if w := []string{"/r", "/t", "60", "/c", "msg"}; !reflect.DeepEqual(args[0], w) {
			t.Errorf("caso %d: argomenti %v", i, args[0])
		}
	}
	// Spegni: /s
	var primo []string
	d := fineDeps{shutdown: func(a ...string) int {
		if primo == nil {
			primo = a
		}
		return 0
	}, puoSpegnere: func() bool { return true }, arma: func(time.Duration) {}, esci: func() error { return nil }}
	if !pianificaFineCon(d, false, 60, "x") || primo[0] != "/s" {
		t.Errorf("spegni: %v", primo)
	}
}

func TestAdessoFineCon(t *testing.T) {
	var armati []time.Duration
	usciti := 0
	d := fineDeps{
		shutdown:    func(a ...string) int { return 0 },
		puoSpegnere: func() bool { return true },
		arma:        func(dopo time.Duration) { armati = append(armati, dopo) },
		esci:        func() error { usciti++; return nil },
	}
	if !adessoFineCon(d, true) || usciti != 0 || !reflect.DeepEqual(armati, []time.Duration{riservaSecondi * time.Second}) {
		t.Errorf("accettato: usciti %d armati %v", usciti, armati)
	}
	d.shutdown = func(a ...string) int { return 1115 }
	if !adessoFineCon(d, true) || usciti != 1 {
		t.Errorf("non accettato: ExitWindowsEx subito (usciti %d)", usciti)
	}
	d.esci = func() error { return os.ErrPermission }
	if adessoFineCon(d, true) {
		t.Error("ExitWindowsEx fallito: deve rispondere no")
	}
}

// 3.1.1: dopo il riavvio scelto nella pulizia Tasso partito con Windows apre da solo la finestra.
func TestBentornatoFinestra(t *testing.T) {
	casi := []struct {
		pronto, nascosto, auto, schermo, gaming, gioco bool
		vuole                                          int
	}{
		{true, true, false, false, false, false, bentFinestraApri},
		{false, true, false, false, false, false, bentFinestraNo},    // avvio normale con Windows: resta nella tray
		{true, false, false, false, false, false, bentFinestraNo},    // aperto a mano: la finestra c'è già
		{true, true, true, false, false, false, bentFinestraNo},      // pulizia automatica (--auto)
		{true, true, false, true, false, false, bentFinestraAspetta}, // schermo intero davanti
		{true, true, false, false, true, true, bentFinestraAspetta},  // Gaming con un gioco aperto
		{true, true, false, false, false, true, bentFinestraApri},    // gioco aperto ma non Gaming e non a schermo intero
		{true, true, false, false, true, false, bentFinestraApri},    // Gaming senza giochi aperti
	}
	for i, c := range casi {
		if g := bentornatoFinestra(c.pronto, c.nascosto, c.auto, c.schermo, c.gaming, c.gioco); g != c.vuole {
			t.Errorf("caso %d: %d, voleva %d", i, g, c.vuole)
		}
	}
}
