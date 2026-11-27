package ottimizza

import (
	"errors"
	"strconv"
	"strings"
)

// Le voci aggiunte dopo la ricerca RICERCA-OTTIMIZZA-PIU (fonti e livelli nel rapporto
// F32-OTTIMIZZA.md, "Voci aggiunte"). Le regole sono quelle di sempre: già a posto, com'era per
// singolo valore, verifica, rimetti solo se c'è ancora il valore di Tasso, cancella se prima non c'era.

// ---------------- Più prestazioni a corrente (portatili, Windows 11) ----------------

// Le tre modalità di risparmio energia di Windows (PowerSetUserConfiguredACPowerMode su learn: "the
// only supported values for the user configured power mode").
const (
	ModoEfficienza = "961cc777-2547-4f9d-8174-7d86181b8a7a" // GUID_POWER_MODE_BEST_EFFICIENCY
	// OverlayNessuno (Bilanciato) e OverlayMigliori (GUID_POWER_MODE_BEST_PERFORMANCE) sono in gestori.go.
)

func modoACNoto(g string) bool {
	return g == ModoEfficienza || g == OverlayNessuno || g == OverlayMigliori
}

// modoACGest: "Prestazioni migliori" solo quando il portatile è attaccato alla corrente. A batteria
// (la modalità DC) Windows usa la sua scelta, che Tasso non tocca mai.
type modoACGest struct{}

const (
	msgModoACFisso    = "Solo sui portatili: sui PC fissi c'è già il piano Prestazioni elevate."
	msgModoACWindows  = "Questa versione di Windows non ha la modalità di risparmio energia per la corrente (serve Windows 11): la salto."
	msgModoACPianoAlt = "Il piano energetico è già Prestazioni elevate: la modalità non serve."
)

func (g *modoACGest) Disponibile(a *Ambiente) (bool, string) {
	if !a.PC.Portatile {
		return false, msgModoACFisso
	}
	if a.PC.Build > 0 && a.PC.Build < 22000 || !a.En.HaModoAC() {
		return false, msgModoACWindows
	}
	if att, err := a.En.PianoAttivo(); err == nil && pianoGiaAlto(att) {
		return false, msgModoACPianoAlt
	}
	return true, ""
}

func (g *modoACGest) APosto(a *Ambiente) (bool, error) {
	v, err := a.En.ModoAC()
	return err == nil && v == OverlayMigliori, err
}

func (g *modoACGest) Prepara(a *Ambiente) (Precedente, error) {
	v, err := a.En.ModoAC()
	if err != nil {
		return Precedente{}, err
	}
	if !modoACNoto(v) {
		return Precedente{}, ErrNonDisponibile
	}
	return Precedente{Extra: map[string]string{"modoac": v, "messo": OverlayMigliori}}, nil
}

func (g *modoACGest) Applica(a *Ambiente, p Precedente) error {
	return a.En.ImpostaModoAC(p.Extra["messo"])
}

func (g *modoACGest) Verifica(a *Ambiente, p Precedente) bool {
	v, err := a.En.ModoAC()
	return err == nil && v == p.Extra["messo"]
}

func (g *modoACGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	v, err := a.En.ModoAC()
	if err != nil {
		return RimessoTutto, err
	}
	if v != p.Extra["messo"] {
		return RimessoLascia, nil
	}
	if err := a.En.ImpostaModoAC(p.Extra["modoac"]); err != nil {
		return RimessoTutto, err
	}
	if v, err := a.En.ModoAC(); err != nil || v != p.Extra["modoac"] {
		return RimessoTutto, errors.New("modalità non rimessa")
	}
	return RimessoTutto, nil
}

func (g *modoACGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	x := p.Extra
	if len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Parti) > 0 || p.Vuoto || len(x) != 2 ||
		!modoACNoto(x["modoac"]) || x["messo"] != OverlayMigliori {
		return Precedente{}, false
	}
	return Precedente{Extra: map[string]string{"modoac": x["modoac"], "messo": x["messo"]}}, true
}

// ---------------- Niente scorciatoie di accessibilità durante il gioco ----------------

// tastoGest: la scorciatoia di una funzione di accessibilità della tastiera, spenta come consiglia
// Microsoft ("Disabling Shortcut Keys in Games"): solo se la funzione è spenta. Se la usi (è accesa),
// non si tocca niente. Si tolgono la scorciatoia e la sua finestra di conferma; la funzione resta in
// Impostazioni > Accessibilità > Tastiera.
type tastoGest struct{ id string }

func spegniScorciatoia(f uint32) uint32 { return f &^ (TastoScorciatoia | TastoConferma) }

func (g *tastoGest) Disponibile(a *Ambiente) (bool, string) { return a.Sis != nil, "" }

func (g *tastoGest) APosto(a *Ambiente) (bool, error) {
	f, err := a.Sis.LeggiTasti(g.id)
	if err != nil {
		return false, err
	}
	// accesa = la usi: va bene così; spenta: va bene se la scorciatoia è già spenta
	return f&TastoAcceso != 0 || f&TastoScorciatoia == 0, nil
}

func (g *tastoGest) Prepara(a *Ambiente) (Precedente, error) {
	f, err := a.Sis.LeggiTasti(g.id)
	if err != nil {
		return Precedente{}, err
	}
	if f&TastoAcceso != 0 || f&TastoScorciatoia == 0 {
		return Precedente{Vuoto: true}, nil
	}
	return Precedente{Extra: map[string]string{"tasti": g.id, "prima": strconv.FormatUint(uint64(f), 10), "messo": strconv.FormatUint(uint64(spegniScorciatoia(f)), 10)}}, nil
}

func (g *tastoGest) Applica(a *Ambiente, p Precedente) error {
	if p.Vuoto {
		return nil
	}
	m, ok := numExtra(p, "messo")
	if !ok {
		return errors.New("valore non valido")
	}
	return a.Sis.ScriviTasti(g.id, m)
}

func (g *tastoGest) Verifica(a *Ambiente, p Precedente) bool {
	if p.Vuoto {
		return true
	}
	m, ok := numExtra(p, "messo")
	f, err := a.Sis.LeggiTasti(g.id)
	return ok && err == nil && f == m
}

func (g *tastoGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	if p.Vuoto {
		return RimessoTutto, nil
	}
	m, ok1 := numExtra(p, "messo")
	pr, ok2 := numExtra(p, "prima")
	if !ok1 || !ok2 {
		return RimessoTutto, errors.New("valore non valido")
	}
	f, err := a.Sis.LeggiTasti(g.id)
	if err != nil {
		return RimessoTutto, err
	}
	if f != m {
		return RimessoLascia, nil // l'hai cambiata tu (o l'hai accesa): resta com'è
	}
	return RimessoTutto, a.Sis.ScriviTasti(g.id, pr)
}

func (g *tastoGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	if p.Vuoto {
		return Precedente{Vuoto: true}, len(p.Extra) == 0
	}
	x := p.Extra
	pr, ok1 := numExtra(p, "prima")
	m, ok2 := numExtra(p, "messo")
	if len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Parti) > 0 || len(x) != 3 || x["tasti"] != g.id || !TastiNoto(x["tasti"]) ||
		!ok1 || !ok2 || pr&TastoAcceso != 0 || m != spegniScorciatoia(pr) {
		return Precedente{}, false
	}
	return Precedente{Extra: map[string]string{"tasti": x["tasti"], "prima": x["prima"], "messo": x["messo"]}}, true
}

func nuoveScorciatoie() Gestore {
	return &insieme{parti: []Gestore{&tastoGest{id: TastiPermanenti}, &tastoGest{id: TastiFiltro}, &tastoGest{id: TastiAvviso}}}
}

// ---------------- Ottimizzazioni per i giochi in finestra (Windows 11) ----------------

// La stringa delle opzioni DirectX di Windows ("Chiave=valore;Chiave=valore;"), la stessa del Boost
// (comando winopt in gamingcmd.go, che usa queste funzioni). Le parti che non sono di Tasso (HDR
// automatico, frequenza variabile...) restano com'erano e nello stesso ordine.
const (
	NomeDXGlobale = "DirectXUserGlobalSettings"
	DXFinestra    = "SwapEffectUpgradeEnable"
)

// DXParti: le parti della stringa, in ordine.
func DXParti(s string) (chiavi []string, m map[string]string) {
	m = map[string]string{}
	for _, part := range strings.Split(s, ";") {
		if i := strings.Index(part, "="); i > 0 {
			k := strings.TrimSpace(part[:i])
			if _, c := m[k]; !c {
				chiavi = append(chiavi, k)
			}
			m[k] = strings.TrimSpace(part[i+1:])
		}
	}
	return chiavi, m
}

// DXParte: il valore di una parte ("" e false se non c'è).
func DXParte(s, k string) (string, bool) {
	_, m := DXParti(s)
	v, ok := m[k]
	return v, ok
}

func dxScrivi(chiavi []string, m map[string]string) string {
	var b strings.Builder
	for _, k := range chiavi {
		if v, ok := m[k]; ok {
			b.WriteString(k + "=" + v + ";")
		}
	}
	return b.String()
}

// DXConParte: la stringa con k=v (al suo posto, o in fondo se non c'era).
func DXConParte(s, k, v string) string {
	chiavi, m := DXParti(s)
	if _, ok := m[k]; !ok {
		chiavi = append(chiavi, k)
	}
	m[k] = v
	return dxScrivi(chiavi, m)
}

// DXSenzaParte: la stringa senza la parte k.
func DXSenzaParte(s, k string) string {
	chiavi, m := DXParti(s)
	delete(m, k)
	return dxScrivi(chiavi, m)
}

// DXSceltoSpento: "Ottimizzazioni per i giochi in finestra" spenta apposta in Impostazioni >
// Schermo > Grafica (scrive SwapEffectUpgradeEnable=0; se non l'hai mai toccata il valore non c'è).
// Né il Boost né Ottimizza tutto vanno contro questa scelta.
func DXSceltoSpento(s string) bool {
	v, ok := DXParte(s, DXFinestra)
	return ok && v == "0"
}

// finestraGest: SwapEffectUpgradeEnable=1 nella stringa DirectX dell'utente, lasciando com'erano le
// altre parti. Rimettendo si toglie (o si rimette) solo quella parte, e solo se c'è ancora "1".
type finestraGest struct{}

func (g *finestraGest) leggi(a *Ambiente) (Valore, error) {
	v, err := a.Reg.Leggi(HKCU, ChiaveGPU, NomeDXGlobale)
	if err == nil && v.Esiste && v.Tipo != TipoSz {
		return v, ErrTipoDiverso
	}
	return v, err
}

func (g *finestraGest) Disponibile(a *Ambiente) (bool, string) {
	if a.PC.Build < 22000 {
		return false, "Solo su Windows 11: la salto."
	}
	if v, err := g.leggi(a); err == nil && v.Esiste && DXSceltoSpento(v.Testo) {
		return false, "L'avevi spenta tu in Impostazioni: la lascio così."
	}
	return true, ""
}

func (g *finestraGest) APosto(a *Ambiente) (bool, error) {
	v, err := g.leggi(a)
	if err != nil {
		return false, err
	}
	x, ok := DXParte(v.Testo, DXFinestra)
	return v.Esiste && ok && x == "1", nil
}

func (g *finestraGest) Prepara(a *Ambiente) (Precedente, error) {
	v, err := g.leggi(a)
	if err != nil {
		return Precedente{}, err
	}
	v.Radice, v.Chiave, v.Nome = HKCU, ChiaveGPU, NomeDXGlobale
	vecchia := ""
	if v.Esiste {
		vecchia = v.Testo
	}
	return Precedente{Valori: []Valore{v}, Messi: []Valore{sz(HKCU, ChiaveGPU, NomeDXGlobale, DXConParte(vecchia, DXFinestra, "1"))}}, nil
}

func (g *finestraGest) Applica(a *Ambiente, p Precedente) error {
	if len(p.Messi) != 1 {
		return errors.New("valore non valido")
	}
	return a.Reg.Scrivi(p.Messi[0])
}

func (g *finestraGest) Verifica(a *Ambiente, p Precedente) bool {
	ok, err := g.APosto(a)
	return err == nil && ok
}

func (g *finestraGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	if len(p.Valori) != 1 {
		return RimessoTutto, errors.New("valore non valido")
	}
	cur, err := g.leggi(a)
	if errors.Is(err, ErrTipoDiverso) {
		return RimessoLascia, nil
	}
	if err != nil {
		return RimessoTutto, err
	}
	if x, ok := DXParte(cur.Testo, DXFinestra); !cur.Esiste || !ok || x != "1" {
		return RimessoLascia, nil // l'hai cambiata tu in Impostazioni: resta com'è
	}
	prima := p.Valori[0]
	vecchio, cera := DXParte(prima.Testo, DXFinestra)
	nuova := DXSenzaParte(cur.Testo, DXFinestra)
	if prima.Esiste && cera {
		nuova = DXConParte(cur.Testo, DXFinestra, vecchio)
	}
	if nuova == "" && !prima.Esiste {
		return RimessoTutto, a.Reg.Cancella(HKCU, ChiaveGPU, NomeDXGlobale)
	}
	return RimessoTutto, a.Reg.Scrivi(sz(HKCU, ChiaveGPU, NomeDXGlobale, nuova))
}

func (g *finestraGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	if len(p.Valori) != 1 || len(p.Messi) != 1 || len(p.Parti) > 0 || len(p.Extra) > 0 {
		return Precedente{}, false
	}
	m, v := p.Messi[0], p.Valori[0]
	if x, ok := DXParte(m.Testo, DXFinestra); m.Radice != HKCU || m.Chiave != ChiaveGPU || m.Nome != NomeDXGlobale || m.Tipo != TipoSz ||
		!m.Esiste || !ok || x != "1" || !testoSicuro(m.Testo, 260) || !v.Stesso(m) || !valoreSicuro(v) || v.Esiste && v.Tipo != TipoSz {
		return Precedente{}, false
	}
	return Precedente{Valori: []Valore{v}, Messi: []Valore{m}}, true
}

// ---------------- Voci del registro ----------------

const (
	kSiuf        = `Software\Microsoft\Siuf\Rules`
	kCercaImp    = `Software\Microsoft\Windows\CurrentVersion\SearchSettings`
	kCercaPol    = `SOFTWARE\Policies\Microsoft\Windows\Windows Search`
	kPubblicita  = `Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`
	kPrivacyUt   = `Software\Microsoft\Windows\CurrentVersion\Privacy`
	kExplorerPol = `Software\Policies\Microsoft\Windows\Explorer`
	kTelemetria  = `SOFTWARE\Policies\Microsoft\Windows\DataCollection`
	kInput       = `Software\Microsoft\Input\TIPC`
)

// consigli (allargata): i consigli e le proposte di Windows. In più rispetto alla prima versione: i
// "contenuti suggeriti nell'app Impostazioni" e le curiosità della schermata di blocco. Queste ultime
// solo se la schermata di blocco non usa "Contenuti in evidenza di Windows": lì Windows non mostra
// quell'interruttore, e Tasso non cambia lo sfondo.
func nuoviConsigli() Gestore {
	var b []bersaglio
	for _, n := range []string{"SubscribedContent-338389Enabled", "SoftLandingEnabled", "SystemPaneSuggestionsEnabled", "SubscribedContent-310093Enabled",
		"SubscribedContent-338393Enabled", "SubscribedContent-353694Enabled", "SubscribedContent-353696Enabled",
		"RotatingLockScreenOverlayEnabled", "SubscribedContent-338387Enabled"} {
		b = append(b, bersaglio{vuole: dw(HKCU, kContenuti, n, 0), ok: soloQuesti(0)})
	}
	b = append(b, bersaglio{vuole: dw(HKCU, kEngagement, "ScoobeSystemSettingEnabled", 0), ok: soloQuesti(0)})
	return &regGest{bers: b, filtro: func(a *Ambiente, x bersaglio) bool {
		if x.vuole.Nome == "RotatingLockScreenOverlayEnabled" || x.vuole.Nome == "SubscribedContent-338387Enabled" {
			v, err := a.Reg.Leggi(HKCU, kContenuti, "RotatingLockScreenEnabled")
			return err == nil && !dwordE(v, 1)
		}
		return true
	}}
}

// feedback: "Frequenza dei feedback" = Mai (tabella "Never: 0 / 0" della pagina Microsoft "Manage
// connections from Windows operating system components", 18.16). Il livello dei dati di
// diagnostica non si tocca.
func nuovoFeedback() Gestore {
	return &regGest{
		bers: []bersaglio{
			{vuole: dw(HKCU, kSiuf, "NumberOfSIUFInPeriod", 0), ok: soloQuesti(0)},
			{vuole: dw(HKCU, kSiuf, "PeriodInNanoSeconds", 0), ok: soloQuesti(0)},
		},
		cond: func(a *Ambiente) (bool, string) {
			for _, n := range []string{"NumberOfSIUFInPeriod", "PeriodInNanoSeconds"} {
				if _, err := a.Reg.Leggi(HKCU, kSiuf, n); errors.Is(err, ErrTipoDiverso) {
					// "una volta al giorno" non sta in un numero piccolo: è una scelta fatta in Impostazioni
					return false, "La frequenza dei feedback l'hai già scelta tu in Impostazioni: la lascio così."
				}
			}
			return true, ""
		},
	}
}

// ricerca: "Mostra evidenziazioni ricerca" spento (l'interruttore dell'utente; il criterio
// EnableDynamicContentInWSB, se c'è, decide l'azienda).
func nuovaRicerca() Gestore {
	return &regGest{
		bers: []bersaglio{{vuole: dw(HKCU, kCercaImp, "IsDynamicSearchBoxEnabled", 0), ok: soloQuesti(0)}},
		cond: func(a *Ambiente) (bool, string) {
			if a.PC.Build > 0 && a.PC.Build < 19042 {
				return false, "Questa versione di Windows non ha le evidenziazioni nella ricerca: niente da fare."
			}
			if policyPresente(a, kCercaPol, "EnableDynamicContentInWSB") {
				return false, "Su questo PC lo decide chi lo gestisce (azienda o scuola): non lo cambio."
			}
			return true, ""
		},
		dopo: avvisa("TraySettings"),
	}
}

// esplora: "Mostra notifiche del provider di sincronizzazione" spento (Opzioni cartella).
func nuovaEsplora() Gestore {
	return &regGest{bers: []bersaglio{{vuole: dw(HKCU, kEsplora, "ShowSyncProviderNotifications", 0), ok: soloQuesti(0)}}, dopo: avvisa("ShellState")}
}

// start: consigli e avvisi dell'account nel menu Start di Windows 11 (Impostazioni >
// Personalizzazione > Start). I file recenti (Start_TrackDocs) restano.
func nuovoStart() Gestore {
	return &regGest{
		bers: []bersaglio{
			{vuole: dw(HKCU, kEsplora, "Start_IrisRecommendations", 0), ok: soloQuesti(0)},
			{vuole: dw(HKCU, kEsplora, "Start_AccountNotifications", 0), ok: soloQuesti(0)},
		},
		cond: func(a *Ambiente) (bool, string) {
			if a.PC.Build < 22621 {
				return false, "Solo su Windows 11 versione 22H2 o più nuova: la salto."
			}
			return true, ""
		},
		filtro: func(a *Ambiente, b bersaglio) bool {
			return b.vuole.Nome != "Start_AccountNotifications" || a.PC.Build >= 22631
		},
		dopo: avvisa("TraySettings"),
	}
}

// pubblicita: ID pubblicitario e "Esperienze personalizzate" spenti (Impostazioni > Privacy e
// sicurezza > Generale; Diagnostica e feedback).
func nuovaPubblicita() Gestore {
	return &regGest{bers: []bersaglio{
		{vuole: dw(HKCU, kPubblicita, "Enabled", 0), ok: soloQuesti(0)},
		{vuole: dw(HKCU, kPrivacyUt, "TailoredExperiencesWithDiagnosticDataEnabled", 0), ok: soloQuesti(0)},
	}}
}

// ---------------- Il passo "Privacy" (Pulisci > Sicurezza e privacy) ----------------

// Privacy: il passo "Privacy: stop pubblicità e tracciamento", con lo stesso motore di Ottimizza
// tutto (com'era per singolo valore, verifica, "Privacy: ripristina" rimette i valori di prima solo
// dove c'è ancora quello di Tasso). Ha un suo file di stato, separato da quello di Ottimizza tutto.
// I valori che ha in comune con Ottimizza tutto seguono la regola "chi l'ha messo?": se li ha messi
// Ottimizza, Privacy prende il loro com'era vero (Ambiente.Eredita) e Ottimizza non li rimette più
// (Ambiente.Tenuti); se c'erano già, Ottimizza li trova già a posto.
func Privacy() Modifica {
	var b []bersaglio
	b = append(b, bersaglio{vuole: dw(HKCU, kPubblicita, "Enabled", 0), ok: soloQuesti(0)})
	for _, n := range []string{"SilentInstalledAppsEnabled", "SystemPaneSuggestionsEnabled", "SoftLandingEnabled", "SubscribedContent-338388Enabled",
		"SubscribedContent-338389Enabled", "SubscribedContent-338393Enabled", "SubscribedContent-353694Enabled", "SubscribedContent-353696Enabled",
		"SubscribedContent-310093Enabled"} {
		b = append(b, bersaglio{vuole: dw(HKCU, kContenuti, n, 0), ok: soloQuesti(0)})
	}
	b = append(b,
		bersaglio{vuole: dw(HKCU, kPrivacyUt, "TailoredExperiencesWithDiagnosticDataEnabled", 0), ok: soloQuesti(0)},
		bersaglio{vuole: dw(HKCU, kEsplora, "ShowSyncProviderNotifications", 0), ok: soloQuesti(0)},
		bersaglio{vuole: dw(HKCU, kEsplora, "Start_IrisRecommendations", 0), ok: soloQuesti(0)},
		bersaglio{vuole: dw(HKCU, kExplorerPol, "DisableSearchBoxSuggestions", 1), ok: soloQuesti(1)},
		// livello 1 = "dati di diagnostica obbligatori" (quelli della sicurezza restano). Se è già 0
		// (più basso, deciso da un'azienda) non lo alzo.
		bersaglio{vuole: dw(HKLM, kTelemetria, "AllowTelemetry", 1), ok: soloQuesti(0, 1)},
		bersaglio{vuole: dw(HKCU, kInput, "Enabled", 0), ok: soloQuesti(0)},
	)
	return Modifica{ID: IDPrivacy, Categoria: "privacy", Utente: true, G: &regGest{bers: b,
		// senza amministratore il criterio di tutto il PC si salta, il resto si fa
		filtro: func(a *Ambiente, x bersaglio) bool { return !serveAdmin(x.vuole.Radice) || a.PC.Admin },
		dopo:   avvisa("TraySettings")}}
}

// ValoriPrivacy: i valori che il passo Privacy mette (per leggere il com'era dalla foto di una
// versione vecchia di Tasso).
func ValoriPrivacy() []Valore {
	var out []Valore
	for _, b := range Privacy().G.(*regGest).bers {
		out = append(out, b.vuole)
	}
	return out
}

// IDPrivacy: l'id del passo Privacy nel suo file di stato.
const IDPrivacy = "privacy"

// ValoriTenuti: i valori che il passo Privacy tiene adesso (voce attiva), per Ambiente.Tenuti di
// Ottimizza tutto.
func ValoriTenuti(st *Stato) func(v Valore) bool {
	if st == nil {
		return nil
	}
	v := st.Voce(IDPrivacy)
	if v == nil || !v.Attiva() {
		return nil
	}
	messi := append([]Valore(nil), v.Prima.Messi...)
	return func(x Valore) bool {
		for _, m := range messi {
			if m.Stesso(x) {
				return true
			}
		}
		return false
	}
}

// valoriAttivi: coppie (com'era, messo) di tutte le voci attive, anche dentro le parti.
func valoriAttivi(p Precedente, f func(prima, messo Valore)) {
	for i, m := range p.Messi {
		if i < len(p.Valori) {
			f(p.Valori[i], m)
		}
	}
	for _, x := range p.Parti {
		valoriAttivi(x, f)
	}
}

// PrimaVera: se il valore di adesso è quello messo da una voce attiva di st, il suo com'era vero
// (di prima di Tasso). Serve al passo Privacy (Eredita) e alla foto di "Rimetti tutto com'era"
// (restore.go), così nessuna delle due salva come "com'era" un valore di Ottimizza tutto.
func PrimaVera(st *Stato, adesso Valore) (Valore, bool) {
	if st == nil {
		return Valore{}, false
	}
	var out Valore
	trovato := false
	for _, id := range st.Attive() {
		v := st.Voce(id)
		valoriAttivi(v.Prima, func(prima, messo Valore) {
			if !trovato && messo.Stesso(adesso) && messo.Uguale(adesso) {
				out, trovato = prima, true
			}
		})
	}
	return out, trovato
}
