package ottimizza

// Le modifiche di Ottimizza tutto: 20 in Gaming, 15 in Ufficio (13 impostazioni che si rimettono
// com'erano e 2 pulizie dei file inutili, che non si rimettono). Sette voci sono uguali nelle due
// modalità (stesso id: effetti, riapertura, consigli, feedback, ricerca, orari, recapito). Su un PC
// vero alcune si saltano da sole (per esempio "Più prestazioni a corrente" sui fissi e le voci solo
// di Windows 11 su Windows 10). Titoli e frasi stanno nella
// finestra (index.html, con le traduzioni); qui ci sono le chiavi esatte, quando una modifica va
// già bene e su quali PC si fa.
//
// VIETATO (scelta di Sergio, 27/11): spegnere antivirus o aggiornamenti; toccare il controllo di
// flusso, la moderazione degli interrupt o il TCP delayed ACK delle schede di rete; cambiare le
// impostazioni dentro i giochi o nel pannello della scheda video; modificare file di sistema.

// Modalità di Tasso.
const (
	ModoGioco   = "gioco"
	ModoUfficio = "ufficio"
)

// Modifica: una delle cose che Ottimizza tutto cambia.
type Modifica struct {
	ID         string
	Categoria  string // per la colonna a sinistra della finestra
	Riavvio    bool   // vale dal prossimo riavvio del PC (va detto)
	AlGioco    bool   // vale dal prossimo avvio del gioco (va detto anche rimettendola com'era)
	NonRimette bool   // non si rimette com'era (pulizia dei file inutili)
	// Utente: tocca le impostazioni o i file dell'utente (HKCU, desktop, temporanei): con Tasso
	// aperto da un altro account amministratore si salta.
	Utente bool
	// Modo: la gestiscono anche le modalità di Tasso (Gaming, FPS...): con una modalità accesa si lascia a lei.
	Modo bool
	// Boost: la gestisce anche il Boost di Tasso (Ottimizzazioni per i giochi in finestra): se il Boost
	// l'ha accesa si lascia a lui, e non si rimette finché il Boost la tiene.
	Boost bool
	// VerificaLenta: si ricontrolla dopo qualche secondo (Windows potrebbe rimettere il suo valore).
	VerificaLenta bool
	// LivelloB: Microsoft descrive l'interruttore ma non pubblica il nome del valore. Se dopo la
	// scrittura Windows non lo tiene, la voce si salta e si dice perché.
	LivelloB bool
	G        Gestore
}

// Categoria: una riga a sinistra nella finestra. Chiedi = l'utente decide (programmi all'avvio):
// non conta tra le modifiche.
type Categoria struct {
	ID     string
	Chiedi bool
}

// Chiavi del registro usate da più modifiche.
const (
	kGameBar     = `Software\Microsoft\GameBar`
	kPriorita    = `SYSTEM\CurrentControlSet\Control\PriorityControl`
	kBackground  = `Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications`
	kRicerca     = `Software\Microsoft\Windows\CurrentVersion\Search`
	kWinlogon    = `Software\Microsoft\Windows NT\CurrentVersion\Winlogon`
	kGrafica     = `SYSTEM\CurrentControlSet\Control\GraphicsDrivers`
	kRecapito    = `Software\Microsoft\Windows\CurrentVersion\DeliveryOptimization\Settings`
	kRecapitoPol = `SOFTWARE\Policies\Microsoft\Windows\DeliveryOptimization`
	kAggUX       = `SOFTWARE\Microsoft\WindowsUpdate\UX\Settings`
	kAggPol      = `SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate`
	kMDMAgg      = `SOFTWARE\Microsoft\PolicyManager\current\device\Update`
	kMDMRecapito = `SOFTWARE\Microsoft\PolicyManager\current\device\DeliveryOptimization`
	kTemi        = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	kEsplora     = `Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`
	kDWM         = `Software\Microsoft\Windows\DWM`
	kGameDVR     = `Software\Microsoft\Windows\CurrentVersion\GameDVR`
	kNotifiche   = `Software\Microsoft\Windows\CurrentVersion\Notifications\Settings`
	kContenuti   = `Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager`
	kEngagement  = `Software\Microsoft\Windows\CurrentVersion\UserProfileEngagement`
)

// OrariInizio, OrariFine: gli orari attivi di Windows Update messi da Ottimizza tutto (dalle 8
// alle 2 di notte: Windows non riavvia il PC per gli aggiornamenti in quelle ore; il massimo che
// Windows accetta è 18 ore).
const (
	OrariInizio = 8
	OrariFine   = 2
)

// primoPianoOK: Win32PrioritySeparation (6 bit AABBCC) con la precedenza ai programmi in primo
// piano: CC (bit 0-1) = 2, come "Regola per prestazioni migliori di: Programmi"; AA (bit 4-5,
// lunghezza dei quanti di tempo) non lunghi (≠ 1); BB (bit 2-3) non fissi (≠ 2). 0x26 e il 2 di
// Windows vanno bene. Il valore che manca vale come quello di Windows (2): va bene.
func primoPianoOK(v Valore) bool {
	if !v.Esiste {
		return true
	}
	if v.Tipo != TipoDword {
		return false
	}
	n := v.Num
	return n&3 == 2 && (n>>4)&3 != 1 && (n>>2)&3 != 2
}

// copreSera: gli orari attivi [inizio, fine) coprono dalle 14 a mezzanotte (quando si gioca di più).
func copreSera(inizio, fine uint32) bool {
	if inizio > 23 || fine > 23 {
		return false
	}
	for h := uint32(14); h <= 23; h++ {
		dentro := false
		if inizio < fine {
			dentro = h >= inizio && h < fine
		} else if inizio > fine {
			dentro = h >= inizio || h < fine
		}
		if !dentro {
			return false
		}
	}
	return true
}

// soloPortatile: le impostazioni della batteria.
func soloPortatile(a *Ambiente) (bool, string) {
	if !a.PC.Portatile {
		return false, "Solo sui portatili: su un PC fisso non serve."
	}
	return true, ""
}

// msgSchermoMai: lo schermo a batteria su "Mai" (valore 0) è una scelta dell'utente.
const msgSchermoMai = "A batteria lo schermo non si spegne mai: l'hai scelto tu, lo lascio così."

// schermoNonMai: solo sui portatili, e non se l'utente ha scelto "Mai" (regola "chi l'ha messo?").
func schermoNonMai(a *Ambiente) (bool, string) {
	if ok, motivo := soloPortatile(a); !ok {
		return false, motivo
	}
	if piano, err := pianoUtente(a); err == nil {
		if _, dc, err := a.En.Leggi(piano, GruppoSchermo, ImpSchermoSpento); err == nil && dc == 0 {
			return false, msgSchermoMai
		}
	}
	return true, ""
}

func avvisa(area string) func(a *Ambiente) {
	return func(a *Ambiente) {
		if a.Sis != nil {
			a.Sis.Avvisa(area)
		}
	}
}

// policyPresente: un criterio di gruppo (azienda o scuola) decide già questa impostazione.
func policyPresente(a *Ambiente, chiave, nome string) bool {
	v, err := a.Reg.Leggi(HKLM, chiave, nome)
	return err == nil && v.Esiste
}

func nuovaGamemode() Gestore {
	return &regGest{bers: []bersaglio{{vuole: dw(HKCU, kGameBar, "AutoGameModeEnabled", 1), ok: assenteO(1)}}}
}

func nuovaPrimoPiano() Gestore {
	return &regGest{bers: []bersaglio{{vuole: dw(HKLM, kPriorita, "Win32PrioritySeparation", 2), ok: primoPianoOK}}}
}

func nuoveApp() Gestore {
	return &regGest{bers: []bersaglio{
		{vuole: dw(HKCU, kBackground, "GlobalUserDisabled", 1), ok: soloQuesti(1)},
		{vuole: dw(HKCU, kRicerca, "BackgroundAppGlobalToggle", 0), ok: soloQuesti(0)},
	}}
}

func nuovaRiapertura() Gestore {
	return &regGest{
		bers: []bersaglio{{vuole: dw(HKCU, kWinlogon, "RestartApps", 0), ok: assenteO(0)}},
		cond: func(a *Ambiente) (bool, string) {
			if a.PC.Build > 0 && a.PC.Build < 16299 {
				return false, "Questa versione di Windows non riapre le app da sola: niente da fare."
			}
			return true, ""
		},
	}
}

func nuovaHAGS() Gestore {
	return &regGest{
		bers: []bersaglio{{vuole: dw(HKLM, kGrafica, "HwSchMode", 2), ok: soloQuesti(2)}},
		cond: func(a *Ambiente) (bool, string) {
			if a.PC.Build > 0 && a.PC.Build < 19041 {
				return false, "Serve Windows 10 versione 2004 o più nuovo: la salto."
			}
			if !a.PC.HAGS {
				return false, "La scheda video o il suo driver non la permettono: la salto."
			}
			if v, err := a.Reg.Leggi(HKLM, kGrafica, "HwSchMode"); err == nil && dwordE(v, 1) && !a.PC.HAGSAcceso {
				// spenta apposta (VR, registrazione, scatti): è una scelta dell'utente
				return false, "L'avevi spenta tu: la lascio così."
			}
			return true, ""
		},
		aPosto: func(a *Ambiente) (bool, error) { return a.PC.HAGSAcceso, nil },
	}
}

func nuovoRecapito() Gestore {
	return &regGest{
		bers: []bersaglio{{vuole: dw(HKNS, kRecapito, "DownloadMode", 0), ok: soloQuesti(0, 99, 100)}},
		cond: func(a *Ambiente) (bool, string) {
			if policyPresente(a, kRecapitoPol, "DODownloadMode") || policyPresente(a, kMDMRecapito, "DODownloadMode") {
				return false, "Su questo PC lo decide chi lo gestisce (azienda o scuola): non lo cambio."
			}
			return true, ""
		},
	}
}

// orariGest: orari attivi di Windows Update. Gli aggiornamenti restano accesi: Windows non riavvia
// il PC da solo in quelle ore.
func nuoviOrari() Gestore {
	g := &regGest{
		bers: []bersaglio{
			{vuole: dw(HKLM, kAggUX, "ActiveHoursStart", OrariInizio)},
			{vuole: dw(HKLM, kAggUX, "ActiveHoursEnd", OrariFine)},
			{vuole: dw(HKLM, kAggUX, "SmartActiveHoursState", 0), ok: assenteO(0)},
		},
		cond: func(a *Ambiente) (bool, string) {
			if policyPresente(a, kAggPol, "SetActiveHours") || policyPresente(a, kAggPol, "ActiveHoursStart") || policyPresente(a, kAggPol, "ActiveHoursMaxRange") ||
				policyPresente(a, kMDMAgg, "ActiveHoursStart") || policyPresente(a, kMDMAgg, "ActiveHoursEnd") || policyPresente(a, kMDMAgg, "ActiveHoursMaxRange") {
				return false, "Su questo PC gli orari degli aggiornamenti li decide chi lo gestisce (azienda o scuola): non li cambio."
			}
			return true, ""
		},
	}
	g.aPosto = func(a *Ambiente) (bool, error) {
		s, err1 := a.Reg.Leggi(HKLM, kAggUX, "ActiveHoursStart")
		e, err2 := a.Reg.Leggi(HKLM, kAggUX, "ActiveHoursEnd")
		sm, err3 := a.Reg.Leggi(HKLM, kAggUX, "SmartActiveHoursState")
		for _, err := range []error{err1, err2, err3} {
			if err != nil {
				return false, err
			}
		}
		manuale := !sm.Esiste || dwordE(sm, 0)
		return manuale && dwordE(s, s.Num) && dwordE(e, e.Num) && copreSera(s.Num, e.Num), nil
	}
	return g
}

// effetti: solo gli effetti che non si notano (Gaming e Ufficio, la stessa modifica).
// Richiesta di Sergio: Windows deve restare bello. Mai il profilo "prestazioni migliori" degli
// effetti visivi, e mai toccare: caratteri arrotondati (ClearType/FontSmoothing), contenuto della
// finestra durante il trascinamento, miniature al posto delle icone, anteprime della barra delle
// applicazioni (e Aero Peek), ombre sotto il puntatore, le finestre e i nomi delle icone,
// rettangolo di selezione trasparente, animazione di apertura e chiusura delle finestre
// (SPI_SETANIMATION), animazioni dentro le finestre (SPI_SETCLIENTAREAANIMATION) e trasparenze
// di Start e barra (EnableTransparency). Resta solo l'animazione dei pulsanti della barra delle
// applicazioni ("Animazioni nella barra delle applicazioni" in Opzioni prestazioni): un valore
// HKCU documentato, che si rimette com'era e torna anche con il .reg.
func nuoviEffetti() Gestore {
	return &regGest{bers: []bersaglio{{vuole: dw(HKCU, kEsplora, "TaskbarAnimations", 0), ok: soloQuesti(0)}}, dopo: avvisa("TraySettings")}
}

func nuovaRegistrazione() Gestore {
	return &regGest{bers: []bersaglio{{vuole: dw(HKCU, kGameDVR, "HistoricalCaptureEnabled", 0), ok: assenteO(0)}}}
}

func nuoviSuoni() Gestore {
	return &regGest{bers: []bersaglio{{vuole: dw(HKCU, kNotifiche, "NOC_GLOBAL_SETTING_ALLOW_NOTIFICATION_SOUND", 0), ok: soloQuesti(0)}}}
}

// Elenco: le modifiche di una modalità, nell'ordine in cui le fa (e le mostra la finestra).
func Elenco(modo string) []Modifica {
	if modo == ModoUfficio {
		// "App in background ferme" non c'è in Ufficio (decisione di Sergio, 28/11): le app dello
		// Store chiuse non manderebbero più notifiche e promemoria (Posta, Calendario, Telefono).
		return []Modifica{
			{ID: "effetti", Categoria: "win", Utente: true, G: nuoviEffetti()},
			{ID: "ricerca", Categoria: "win", Utente: true, LivelloB: true, G: nuovaRicerca()},
			{ID: "menustart", Categoria: "win", Utente: true, LivelloB: true, G: nuovoStart()},
			{ID: "pulizia_temp", Categoria: "clean", Utente: true, NonRimette: true, G: &puliziaGest{passi: []string{"backup", "temp_user", "temp_win", "thumbs"}}},
			{ID: "pulizia_resti", Categoria: "clean", NonRimette: true, G: &puliziaGest{passi: []string{"wu_cache", "cleanmgr"}}},
			{ID: "riapertura", Categoria: "ram", Utente: true, G: nuovaRiapertura()},
			{ID: "suoni", Categoria: "bell", Utente: true, G: nuoviSuoni()},
			{ID: "consigli", Categoria: "ads", Utente: true, LivelloB: true, G: nuoviConsigli()},
			{ID: "feedback", Categoria: "ads", Utente: true, G: nuovoFeedback()},
			{ID: "esplora", Categoria: "ads", Utente: true, LivelloB: true, G: nuovaEsplora()},
			{ID: "pubblicita", Categoria: "ads", Utente: true, LivelloB: true, G: nuovaPubblicita()},
			{ID: "orari", Categoria: "net", VerificaLenta: true, G: nuoviOrari()},
			{ID: "recapito", Categoria: "net", G: nuovoRecapito()},
			{ID: "batt_risparmio", Categoria: "batt", G: &enGest{gruppo: GruppoRisparmio, imp: ImpRisparmioSoglia, dc: true, vuole: 20, ok: func(v uint32) bool { return v >= 20 }, cond: soloPortatile}},
			{ID: "batt_schermo", Categoria: "batt", G: &enGest{gruppo: GruppoSchermo, imp: ImpSchermoSpento, dc: true, vuole: 300, ok: func(v uint32) bool { return v > 0 && v <= 300 }, cond: schermoNonMai}},
		}
	}
	return []Modifica{
		{ID: "gamemode", Categoria: "cpu", Utente: true, Modo: true, G: nuovaGamemode()},
		{ID: "primopiano", Categoria: "cpu", G: nuovaPrimoPiano()},
		{ID: "bgapps", Categoria: "ram", Utente: true, G: nuoveApp()},
		{ID: "riapertura", Categoria: "ram", Utente: true, G: nuovaRiapertura()},
		{ID: "hags", Categoria: "gpu", Riavvio: true, G: nuovaHAGS()},
		{ID: "gpupref", Categoria: "gpu", Utente: true, G: &gpuGest{}},
		{ID: "finestra", Categoria: "gpu", Utente: true, Boost: true, LivelloB: true, AlGioco: true, G: &finestraGest{}},
		{ID: "recapito", Categoria: "net", G: nuovoRecapito()},
		{ID: "orari", Categoria: "net", VerificaLenta: true, G: nuoviOrari()},
		{ID: "effetti", Categoria: "win", Utente: true, G: nuoviEffetti()},
		{ID: "notifgioco", Categoria: "win", Utente: true, G: &notifGest{}},
		{ID: "registrazione", Categoria: "win", Utente: true, G: nuovaRegistrazione()},
		{ID: "ricerca", Categoria: "win", Utente: true, LivelloB: true, G: nuovaRicerca()},
		{ID: "consigli", Categoria: "ads", Utente: true, LivelloB: true, G: nuoviConsigli()},
		{ID: "feedback", Categoria: "ads", Utente: true, G: nuovoFeedback()},
		{ID: "mouse", Categoria: "mouse", Utente: true, Modo: true, G: nuovoMouse()},
		{ID: "scorciatoie", Categoria: "mouse", Utente: true, G: nuoveScorciatoie()},
		{ID: "piano", Categoria: "pow", Modo: true, G: &pianoGest{}},
		{ID: "modoac", Categoria: "pow", Modo: true, G: &modoACGest{}},
		{ID: "usb", Categoria: "pow", G: &enGest{gruppo: GruppoUSB, imp: ImpUSBSospensione, utente: true, vuole: 0, ok: func(v uint32) bool { return v == 0 }}},
	}
}

// Categorie: le righe a sinistra, nell'ordine della finestra (quella "start" è chiedi).
func Categorie(modo string) []Categoria {
	if modo == ModoUfficio {
		return []Categoria{{ID: "start", Chiedi: true}, {ID: "win"}, {ID: "clean"}, {ID: "ram"}, {ID: "bell"}, {ID: "ads"}, {ID: "net"}, {ID: "batt"}}
	}
	return []Categoria{{ID: "cpu"}, {ID: "ram"}, {ID: "gpu"}, {ID: "net"}, {ID: "win"}, {ID: "ads"}, {ID: "start", Chiedi: true}, {ID: "mouse"}, {ID: "pow"}}
}

// Trova: la modifica con questo id (di Gaming o di Ufficio: alcune sono le stesse).
func Trova(id string) (Modifica, bool) {
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		for _, m := range Elenco(modo) {
			if m.ID == id {
				return m, true
			}
		}
	}
	if id == IDPrivacy {
		return Privacy(), true // il passo Privacy (ha un suo file di stato)
	}
	return Modifica{}, false
}
