package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Prova sul PC vero di Tasso 3.2 (PROVA-PC-FINALE.md): i piccoli problemi corretti.

func leggiFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func passoPerID(id string) (Step, bool) {
	for _, s := range allSteps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}

// 1. Un Risolvi in sola lettura non dice più "liberati 20 KB": per i soli passaggi di Risolvi la
// differenza dello spazio libero dei dischi non si usa (conta solo quello che i passaggi tolgono).
func TestRisolviNonLiberaSpazio(t *testing.T) {
	g, ok := passoPerID("fix_game_gpuswitch")
	if !ok {
		t.Fatal("manca fix_game_gpuswitch")
	}
	if !soloPassiRisolvi([]Step{g}) {
		t.Error("un Risolvi da solo deve contare come soli passaggi di Risolvi")
	}
	b, ok := passoPerID("backup")
	if !ok {
		t.Fatal("manca backup")
	}
	if soloPassiRisolvi([]Step{g, b}) || soloPassiRisolvi(nil) {
		t.Error("con una pulizia (o senza passaggi) la differenza dei dischi si usa")
	}
	r := leggiFile(t, "runner.go")
	if !strings.Contains(r, "if len(freePrima) > 0 && !soloRisolvi {") || !strings.Contains(r, `trGo("Fine: durata %s")`) {
		t.Error("runner.go: per i soli Risolvi niente differenza dei dischi e niente \"liberati 0 KB\"")
	}
}

// 2. Il Risolvi del caldo usa la stessa temperatura della scheda video della pagina Hardware
// (anche AMD), passata da writeScript solo agli script che la usano.
func TestRisolviTemperaturaComeHardware(t *testing.T) {
	if !strings.Contains(gpuTempPS, "if ($TassoGpuTemp) { $gt = [int]$TassoGpuTemp }") {
		t.Error("gpuTempPS deve usare $TassoGpuTemp")
	}
	for _, id := range []string{"fix_game_heat", "fix_game_gpucrash"} {
		s, ok := passoPerID(id)
		if !ok {
			t.Fatalf("manca %s", id)
		}
		if !strings.Contains(s.Script, "$TassoGpuTemp") {
			t.Errorf("%s non usa la temperatura della pagina Hardware", id)
		}
	}
	r := leggiFile(t, "runner.go")
	if !strings.Contains(r, `if strings.Contains(script, "$TassoGpuTemp") {`) || !strings.Contains(r, `"\n$TassoGpuTemp = " + gpuTempPerScript()`) {
		t.Error("writeScript deve passare $TassoGpuTemp agli script che lo usano")
	}
}

// 3. Ping: i server Riot non rispondono più al ping. Per ognuno c'è un relay Valve vicino (città
// conosciuta) e il risultato misurato lì è segnato come stima.
func TestPingRiotVicino(t *testing.T) {
	for _, x := range riotTargets {
		c, ok := riotVicino[x.Host]
		if !ok {
			t.Errorf("%s (%s): manca il server vicino", x.Name, x.Host)
			continue
		}
		if _, ok := valvePops[c]; !ok {
			t.Errorf("%s: città %q non è tra i relay Valve conosciuti", x.Name, c)
		}
	}
	h := leggiFile(t, "ui/index.html")
	if !strings.Contains(h, "(x.stima ? _t(' · stima') : '')") {
		t.Error("la pagina deve dire che il valore di Riot è una stima")
	}
}

// 5. Un Risolvi partito dalla pagina Risolvi mostra avanzamento e risultato lì, non in Pulisci.
func TestRisolviRisultatoInRisolvi(t *testing.T) {
	h := leggiFile(t, "ui/index.html")
	for _, q := range []string{
		`<div id="fixRunHost" style="display:contents"></div>`,
		`inFix = CURVIEW === 'fix'; RUN_FIX_NEXT = inFix;`,
		`else show(inFix ? 'fix' : 'clean');`,
		`runHost(RUN_FIX_NEXT && !p.level); RUN_FIX_NEXT = false;`,
	} {
		if !strings.Contains(h, q) {
			t.Errorf("index.html: manca %s", q)
		}
	}
	i, j := strings.Index(h, `<section class="view" id="v-fix">`), strings.Index(h, `id="fixRunHost"`)
	if i < 0 || j < i || strings.Contains(h[i:j], "</section>") {
		t.Error("fixRunHost deve stare nella pagina Risolvi")
	}
}

// 6. Col menu largo niente suggerimento sulle voci (restava vuoto sopra la voce sotto).
func TestMenuSenzaSuggerimentoVuoto(t *testing.T) {
	h := leggiFile(t, "ui/index.html")
	if strings.Contains(h, "if (!b.title) b.title = t;") {
		t.Error("le voci del menu non devono avere sempre il suggerimento")
	}
	if !strings.Contains(h, "if (NAV_STRETTO.matches) b.title = b.dataset.tip; else b.removeAttribute('title');") {
		t.Error("il suggerimento delle voci solo col menu stretto")
	}
}

// 7. "Backup creato" solo se c'è un punto di ripristino nuovo fatto adesso da Tasso.
func TestBackupCreatoVero(t *testing.T) {
	b, ok := passoPerID("backup")
	if !ok {
		t.Fatal("manca backup")
	}
	if strings.Contains(b.Script, "if ($after -gt $before) { Say 'Backup creato.") {
		t.Error("il conto dei punti non basta per dire \"creato\"")
	}
	if !strings.Contains(b.Script, "[long]$_.SequenceNumber -gt $maxSeq -and [string]$_.Description -eq 'Tasso - prima della manutenzione'") {
		t.Error("\"creato\" solo con un punto nuovo di Tasso")
	}
}

// 9. La pulizia programmata di base è spenta: la prima scelta della modalità (anche Ufficio e
// dall'installatore) non la accende e non la spegne (chi l'aveva accesa la tiene).
func TestPuliziaProgrammataSpentaDiBase(t *testing.T) {
	s := leggiFile(t, "settings.go")
	m := regexp.MustCompile(`(?s)func profiloPC\(p string\) string \{.*?\n\}`).FindString(s)
	if m == "" {
		t.Fatal("profiloPC non trovata")
	}
	if strings.Contains(m, "setWeekly") || strings.Contains(m, "Weekly") {
		t.Error("profiloPC non deve toccare la pulizia programmata")
	}
	var zero Settings
	if zero.Weekly {
		t.Error("Weekly deve partire spenta")
	}
	if strings.Contains(leggiFile(t, "main.go"), `emit("onPuliziaAccesa"`) {
		t.Error("main.go non deve più dire \"Ho acceso la Pulizia programmata\"")
	}
	h := leggiFile(t, "ui/index.html")
	if strings.Contains(h, "Con Ufficio accendo la Pulizia programmata") || !strings.Contains(h, "_t('La Pulizia programmata resta spenta: se vuoi, la accendi in Pulisci.')") {
		t.Error("la prima apertura in Ufficio non deve promettere la pulizia programmata accesa")
	}
}

// 11. Profonda ed Eccezionale: "Riavvia" non è già scelto, nemmeno spuntando "Aggiorna anche Windows".
func TestFineNonRiavviaDiBase(t *testing.T) {
	h := leggiFile(t, "ui/index.html")
	f := regexp.MustCompile(`(?s)function fineBox\(wu\)\{.*?\n\}`).FindString(h)
	if f == "" || !strings.Contains(f, "const def = 'acceso';") {
		t.Error("fineBox: di base \"Lascia acceso\"")
	}
	w := regexp.MustCompile(`(?s)function fineWire\(\)\{.*?\n\}`).FindString(h)
	if w == "" || strings.Contains(w, "'riavvia'") {
		t.Error("fineWire non deve scegliere Riavvia da solo")
	}
}
