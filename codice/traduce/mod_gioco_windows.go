//go:build windows

package traduce

// Parte Windows della traduzione dentro il gioco: il piccolo server sul PC a cui il gioco chiede
// le traduzioni, l'installazione dalla procedura "Traduzione gioco" e le correzioni dell'IA.

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	postiServer   = make(chan struct{}, 4) // al massimo 4 traduzioni insieme per il gioco
	muModRecenti  sync.Mutex
	modRecenti    = map[string]string{}               // inglese → chiave del gioco che l'ha chiesto
	modCorrezioni = map[string]map[string][2]string{} // chiave → inglese → [bozza, corretta]
	modRichieste  atomic.Int64
	modAvvio      sync.Once
)

// giocoInFile: il gioco è tradotto dai file (non serve la sovrimpressione).
func giocoInFile(cartella string) bool {
	if cartella == "" {
		return false
	}
	_, ok := leggiManifesto(dirApp, chiaveGioco(cartella))
	return ok
}

// avviaServerModGioco: solo su 127.0.0.1, nessuno da fuori dal PC può collegarsi.
func avviaServerModGioco() {
	modAvvio.Do(func() {
		go func() {
			defer guard("server-gioco")
			mux := http.NewServeMux()
			mux.HandleFunc("/gioco/", rispondiAlGioco)
			l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", portaModGioco))
			if err != nil {
				logErrore("server-gioco", err, nil)
				return
			}
			srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
			_ = srv.Serve(l)
		}()
		go cicloCorrezioniFile()
	})
}

func rispondiAlGioco(w http.ResponseWriter, r *http.Request) {
	defer guard("risposta-gioco")
	// solo XUnity dentro un gioco di questo PC: niente richieste da pagine web (Origin, Referer,
	// Sec-Fetch-Site) né da indirizzi diversi, e solo con il segreto scritto nella sua configurazione
	if r.Header.Get("Origin") != "" || r.Header.Get("Referer") != "" || r.Header.Get("Sec-Fetch-Site") != "" ||
		r.Host != fmt.Sprintf("127.0.0.1:%d", portaModGioco) {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	if InUfficio.Load() { // modalità Ufficio: niente traduzione dei giochi
		http.Error(w, "ufficio", http.StatusServiceUnavailable)
		return
	}
	parti := strings.Split(strings.TrimPrefix(r.URL.Path, "/gioco/"), "/")
	if len(parti) != 2 {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	chiave := parti[0]
	m, ok := leggiManifesto(dirApp, chiave)
	if !ok || m.Token == "" || subtle.ConstantTimeCompare([]byte(parti[1]), []byte(m.Token)) != 1 {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	testo := r.URL.Query().Get("text")
	if testo == "" || len([]rune(testo)) > 1000 {
		http.Error(w, "vuoto o troppo lungo", http.StatusBadRequest)
		return
	}
	postiServer <- struct{}{}
	defer func() { <-postiServer }()
	if traduttore == nil || !mozOK.Load() {
		http.Error(w, "traduttore non pronto", http.StatusServiceUnavailable)
		return
	}
	// sempre la libreria di questo gioco (anche se la sovrimpressione ne ha aperta un'altra)
	traduttore.ApriLibreria(fileLibreriaGioco(m.Cartella, m.Gioco, nomeRiconosciuto(m.Cartella)))
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	// le frasi lunghe sono dialoghi (contesto, nomi dei personaggi), le corte scritte di menu
	dialogo := len([]rune(testo)) >= 40 || strings.Contains(testo, ". ")
	// le lingue scelte quando hai installato la traduzione in questo gioco, non quelle di adesso
	da, a := lingueMod(m)
	ris := traduttore.TraduciPer(ctx, testo, dialogo, da, a)
	if ris.Errore != "" || ris.Tradotto == "" {
		http.Error(w, "non tradotto", http.StatusInternalServerError)
		return
	}
	modRichieste.Add(1)
	contaRichiestaGioco(chiave)
	muModRecenti.Lock()
	if len(modRecenti) > 20000 {
		modRecenti = map[string]string{}
	}
	modRecenti[strings.TrimSpace(testo)] = chiave
	modRecenti[strings.Join(strings.Fields(testo), " ")] = chiave // come la chiama il correttore
	muModRecenti.Unlock()
	emetti("riga", map[string]string{"originale": testo, "tradotto": ris.Tradotto, "motore": "nel gioco"})
	registraRigaGioco(chiave, testo, ris.Tradotto)
	ris.Motore = testoTasso("nel gioco", "in the game")
	aggiungiCronologia(ris, true)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(ris.Tradotto))
}

// modCorretta: l'IA ha migliorato una frase. Se l'aveva chiesta un gioco, la cambio nel suo file.
func modCorretta(inglese, bozza, corretta string) {
	muModRecenti.Lock()
	defer muModRecenti.Unlock()
	chiave, ok := modRecenti[strings.TrimSpace(inglese)]
	if !ok {
		return
	}
	if modCorrezioni[chiave] == nil {
		modCorrezioni[chiave] = map[string][2]string{}
	}
	modCorrezioni[chiave][inglese] = [2]string{bozza, corretta}
}

// cicloCorrezioniFile: scrivo le correzioni quando il gioco è chiuso (mentre gira, XUnity tiene
// il file aperto e lo riscrive lui).
func cicloCorrezioniFile() {
	defer guard("correzioni-file")
	controllaModInstallate()
	for {
		time.Sleep(20 * time.Second)
		controllaModInstallate()
		muModRecenti.Lock()
		lavoro := modCorrezioni
		modCorrezioni = map[string]map[string][2]string{}
		muModRecenti.Unlock()
		for chiave, corr := range lavoro {
			m, ok := leggiManifesto(dirApp, chiave)
			if !ok {
				continue
			}
			if processoAperto(m.Exe) {
				rimettiCorrezioni(chiave, corr)
				continue
			}
			_, a := lingueMod(m)
			file := filepath.Join(m.Cartella, "BepInEx", "Translation", a, "Text", "_AutoGeneratedTranslations.txt")
			if _, err := CorreggiFileXUnity(file, corr); err != nil && !os.IsNotExist(err) {
				rimettiCorrezioni(chiave, corr)
			}
		}
	}
}

// lingueMod: le lingue della traduzione dentro il gioco (quelle dell'installazione; i manifesti
// delle versioni di prima non le hanno: allora le lingue scelte adesso).
func lingueMod(m ManifestoMod) (string, string) {
	da, a := m.Lingue[0], m.Lingue[1]
	if da == "" || da == "auto" || a == "" {
		da, a = coppiaAttiva()
	}
	return da, a
}

func rimettiCorrezioni(chiave string, corr map[string][2]string) {
	muModRecenti.Lock()
	defer muModRecenti.Unlock()
	if modCorrezioni[chiave] == nil {
		modCorrezioni[chiave] = map[string][2]string{}
	}
	for k, v := range corr {
		if _, gia := modCorrezioni[chiave][k]; !gia {
			modCorrezioni[chiave][k] = v
		}
	}
}

// processoAperto: c'è un processo con questo nome di exe.
func processoAperto(exe string) bool {
	if exe == "" {
		return false
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), exe) {
			return true
		}
	}
	return false
}

// applicaModGioco: passi veri della procedura "Traduzione gioco" quando si sceglie "dentro il gioco".
var muInstalla sync.Mutex

// rischioOK: per i giochi con anti-cheat l'utente ha letto l'avviso sul rischio di ban e ha
// scelto lui di tradurre dentro il gioco (sempre spento se non lo sceglie, gioco per gioco).
func applicaModGioco(nome, cartella string, rischioOK bool) {
	defer func() {
		if r := recover(); r != nil {
			logErrore("applica-mod", r, debug.Stack())
			emetti("guidata", map[string]any{"nome": nome, "errore": "Errore inatteso: riprova (dettagli in logs\\crash.txt)."})
		}
	}()
	if !muInstalla.TryLock() {
		emetti("guidata", map[string]any{"nome": nome, "errore": "Sto già installando la traduzione in un gioco: aspetta che finisca."})
		return
	}
	defer muInstalla.Unlock()
	passo := func(p int, testo string) {
		emetti("guidata", map[string]any{"nome": nome, "percento": p, "testo": testo})
	}
	fallito := func(testo string) {
		emetti("guidata", map[string]any{"nome": nome, "errore": testo})
	}
	passo(3, "Controllo il gioco…")
	if tipo, _ := TipoUnity(cartella); tipo == "" && CartellaPaksUnreal(cartella) != "" {
		ac := haAntiCheat(cartella)
		if ac && !rischioOK {
			applicaTraduzione(nome, cartella)
			return
		}
		if ok, motivo := UnrealPossibile(cartella); !ok {
			emetti("guidata", map[string]any{"nome": nome, "percento": 4, "testo": "Dentro il gioco non si può (" + motivo + "): lo traduco sullo schermo."})
			applicaTraduzione(nome, cartella)
			return
		}
		if _, a := coppiaAttiva(); GiocoHaLingua(CartellaPaksUnreal(cartella), a) && os.Getenv("TASSO_FORZA_FILE") == "" {
			// Il controllo può sbagliare (file della lingua quasi vuoti): lo dico, ma la traduzione
			// dello schermo parte lo stesso, così non resti mai senza niente.
			emetti("guidata", map[string]any{"nome": nome, "percento": 4, "testo": nome + " sembra avere già la sua traduzione in " + strings.ToLower(nomeLingua(a)) + ": prova a sceglierla nelle impostazioni del gioco (di solito Opzioni → Lingua). Intanto lo traduco sullo schermo."})
			applicaTraduzione(nome, cartella)
			return
		}
		if os.Getenv("TASSO_FILE_UNREAL") == "" {
			// 24/11: provato dal vivo su PC Cafe Simulator (Unreal 5 con IoStore): il gioco non
			// carica il file aggiunto in Paks\~mods, nemmeno con i suoi testi ufficiali dentro.
			// Finché non funziona in un gioco vero resta spento: traduco sullo schermo
			applicaTraduzione(nome, cartella)
			return
		}
		applicaModUnreal(nome, cartella, ac, passo, fallito)
		return
	}
	if haAntiCheat(cartella) {
		// mai file aggiunti a un gioco con l'anti-imbroglio: passo alla traduzione dello schermo
		applicaTraduzione(nome, cartella)
		return
	}
	if ok, motivo := ModPossibile(cartella); !ok {
		emetti("guidata", map[string]any{"nome": nome, "percento": 4, "testo": "Dentro il gioco non si può (" + motivo + "): lo traduco sullo schermo."})
		applicaTraduzione(nome, cartella)
		return
	}
	tipo, exe := TipoUnity(cartella)
	if tipo == "" {
		fallito(nome + " non è fatto con Unity: per ora so tradurre dai file solo i giochi Unity. Usa la traduzione dello schermo, funziona lo stesso.")
		return
	}
	if processoAperto(exe) {
		fallito("Chiudi " + nome + " e riprova: con il gioco aperto non posso aggiungere i file.")
		return
	}
	if g := ModGiaPresenti(cartella); g != "" {
		fallito(nome + " ha già delle mod installate (" + g + "): per sicurezza non lo tocco. Usa la traduzione dello schermo.")
		return
	}
	if e := aspettaMotore(); e != "" {
		fallito(e + ", poi riprova.")
		return
	}
	zips := []FileDaScaricare{zipBepInExMono, zipXUnityMono}
	if tipo == "il2cpp" {
		zips = []FileDaScaricare{zipBepInExIL2CPP, zipXUnityIL2CPP}
	}
	dir := filepath.Join(dirApp, "mod")
	var tot int64
	for _, z := range zips {
		tot += z.Byte
	}
	passo(8, fmt.Sprintf("Scarico gli strumenti liberi BepInEx e XUnity AutoTranslator (%.0f MB, una volta sola)…", float64(tot)/1048576))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := scaricaTutti(ctx, dir, zips, func(fatti, totale int64) {
		if totale > 0 {
			passo(8+int(50*fatti/totale), fmt.Sprintf("Scarico BepInEx e XUnity AutoTranslator… %d%%", 100*fatti/totale))
		}
	}); err != nil {
		fallito("Scaricamento non riuscito: " + err.Error())
		return
	}
	passo(62, "Faccio la copia di sicurezza e aggiungo i file accanto al gioco…")
	var percorsi []string
	for _, z := range zips {
		percorsi = append(percorsi, filepath.Join(dir, z.Nome))
	}
	chiave := chiaveGioco(cartella)
	da, a := coppiaAttiva()
	if da == "auto" || da == "" {
		da = "en"
	}
	if da == a {
		// "Riconosci da solo" verso la stessa lingua del gioco (o non ancora riconosciuta)
		fallito("Le scritte del gioco e la lingua che vuoi sono uguali (" + nomeLingua(a) + "): scegli la lingua del gioco al passo 1 e riprova.")
		return
	}
	if _, err := InstallaModGioco(dirApp, nome, cartella, chiave, percorsi, da, a); err != nil {
		fallito("Non sono riuscito: " + err.Error() + ". Il gioco è rimasto com'era.")
		return
	}
	passo(85, "Collego il gioco al traduttore di Tasso sul PC…")
	avviaServerModGioco()
	impostaGiocoTradotto(cartella, true)
	time.Sleep(300 * time.Millisecond)
	esito := "File aggiunti, copia di sicurezza fatta. Ora premi Apri il gioco: ti dico io se la traduzione dentro parte davvero. "
	if tipo == "il2cpp" {
		esito += "La prima volta il gioco ci mette qualche minuto in più a partire (prepara i file una volta sola). "
	}
	if iaPresenteSvelto() && leggiImp().Motore != "mozilla" && iaAdattaPC() {
		esito += "Le frasi migliorate dal correttore IA si vedono dalla volta dopo. "
	}
	esito += "Se il gioco si aggiorna, rifai Applica."
	emetti("guidata", map[string]any{"nome": nome, "percento": 100, "fatto": true, "modo": "file", "testo": esito})
	emetti("aggiorna", stato())
}

// fermaModGioco: toglie tutto e rimette il gioco com'era.
func fermaModGioco(cartella string) string {
	m, ok := leggiManifesto(dirApp, chiaveGioco(cartella))
	if !ok {
		return ""
	}
	if !strings.EqualFold(filepath.Clean(m.Cartella), filepath.Clean(cartella)) {
		return "L'elenco dei file aggiunti non corrisponde a questa cartella: non tocco niente."
	}
	if processoAperto(m.Exe) {
		return "Chiudi " + m.Gioco + " e riprova: con il gioco aperto non posso togliere i file."
	}
	if err := TogliModGioco(dirApp, chiaveGioco(cartella)); err != nil {
		return err.Error()
	}
	return ""
}

var muRigheGioco sync.Mutex

// registraRigaGioco: elenco sul PC di quello che il gioco ha chiesto (per controllare la qualità).
func registraRigaGioco(chiave, en, it string) {
	muRigheGioco.Lock()
	defer muRigheGioco.Unlock()
	p := filepath.Join(dirApp, "logs", "traduzioni-gioco-"+chiave+".txt")
	if st, err := os.Stat(p); err == nil && st.Size() > 4<<20 {
		return
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(strings.ReplaceAll(en, "\n", " / ") + "\t" + strings.ReplaceAll(it, "\n", " / ") + "\r\n")
}

var (
	muRichiestePerGioco sync.Mutex
	richiestePerGioco   = map[string]int{}
	modNonFunziona      = map[string]bool{} // in questa sessione: torno alla sovrimpressione
)

func contaRichiestaGioco(chiave string) {
	muRichiestePerGioco.Lock()
	richiestePerGioco[chiave]++
	muRichiestePerGioco.Unlock()
}

func richiesteDelGioco(chiave string) int {
	muRichiestePerGioco.Lock()
	defer muRichiestePerGioco.Unlock()
	return richiestePerGioco[chiave]
}

// modDaUsare: il gioco è tradotto dentro e (per quanto ne so) funziona. Se il gioco gira da 4
// minuti in primo piano e non mi ha chiesto neanche una scritta, la traduzione dentro non parte
// (anti-imbroglio, aggiornamento del gioco…): per questa sessione uso la sovrimpressione.
func modDaUsare(cartella string, inPrimoPianoDa time.Duration, nome string) bool {
	if !giocoInFile(cartella) {
		return false
	}
	chiave := chiaveGioco(cartella)
	if m, _ := leggiManifesto(dirApp, chiave); m.Tipo == "unreal" {
		// Unreal: le scritte tradotte sono nel file aggiunto, il gioco non chiede niente a Tasso
		return len(m.Aggiunti) > 0 && esisteVoce(filepath.Join(m.Cartella, m.Aggiunti[0]))
	}
	muRichiestePerGioco.Lock()
	no := modNonFunziona[chiave]
	muRichiestePerGioco.Unlock()
	if no {
		return false
	}
	if inPrimoPianoDa > 4*time.Minute && richiesteDelGioco(chiave) == 0 {
		muRichiestePerGioco.Lock()
		modNonFunziona[chiave] = true
		muRichiestePerGioco.Unlock()
		emetti("info", map[string]string{"testo": nome + ": la traduzione dentro il gioco non è partita (forse per un aggiornamento del gioco). Passo alla traduzione dello schermo. Con Ferma traduzione puoi togliere i file aggiunti."})
		return false
	}
	return true
}

var (
	muLingue    sync.Mutex
	lingueViste = map[string]string{} // chiave del gioco → lingua delle scritte lette
)

func linguaVista(chiave string) string {
	muLingue.Lock()
	defer muLingue.Unlock()
	return lingueViste[chiave]
}

func messaggioLingua(nome, lingua string) string {
	_, a := coppiaAttiva()
	if lingua == a {
		return nome + " è già in " + nomeLingua(a) + ": non serve tradurlo."
	}
	return nome + " · lingua delle scritte: " + nomeLingua(lingua) + ". Per tradurle scarica la lingua nella procedura a passi del gioco."
}

// segnaLingua: ricordo la lingua delle scritte lette sullo schermo di un gioco.
func segnaLingua(cartella, nome, lingua string) {
	if lingua == "" {
		return
	}
	muLingue.Lock()
	lingueViste[chiaveGioco(cartella)] = lingua
	muLingue.Unlock()
}

// segnaModNonPartita: la traduzione dentro il gioco non è partita. Uso subito lo schermo e, a
// gioco chiuso, tolgo i file aggiunti (il gioco torna com'era).
func segnaModNonPartita(chiave, nome string) {
	muRichiestePerGioco.Lock()
	gia := modNonFunziona[chiave]
	modNonFunziona[chiave] = true
	muRichiestePerGioco.Unlock()
	if !gia {
		emetti("info", map[string]string{"testo": "La traduzione dentro " + nome + " non è partita (gioco troppo nuovo per gli strumenti liberi). Lo traduco sullo schermo; a gioco chiuso tolgo i file aggiunti."})
	}
}

// controllaModInstallate: giochi con i file aggiunti in cui la traduzione dentro non è partita:
// a gioco chiuso li rimetto com'erano e resta la traduzione dello schermo.
func controllaModInstallate() {
	voci, _ := os.ReadDir(filepath.Join(dirApp, "copie-giochi"))
	for _, v := range voci {
		m, ok := leggiManifesto(dirApp, v.Name())
		if !ok || !ModNonPartita(m.Cartella) {
			continue
		}
		segnaModNonPartita(v.Name(), m.Gioco)
		if processoAperto(m.Exe) {
			continue
		}
		if err := TogliModGioco(dirApp, v.Name()); err == nil {
			impostaGiocoTradotto(m.Cartella, true) // resta tradotto, sullo schermo
			emetti("info", map[string]string{"testo": "Ho tolto da " + m.Gioco + " i file della traduzione dentro il gioco, che non partiva: il gioco è com'era. Lo traduco sullo schermo."})
			emetti("aggiorna", stato())
		}
	}
}

// TogliTutteLeTraduzioni (3.1, "Togli Tasso dal PC"): toglie le traduzioni messe dentro i giochi.
// Restituisce i giochi sistemati e quelli dove non è riuscita (di solito perché il gioco è aperto).
func TogliTutteLeTraduzioni() (tolti, falliti []string) {
	voci, _ := os.ReadDir(filepath.Join(dirApp, "copie-giochi"))
	for _, v := range voci {
		m, ok := leggiManifesto(dirApp, v.Name())
		if !ok {
			continue
		}
		nome := m.Gioco
		if nome == "" {
			nome = v.Name()
		}
		if err := TogliModGioco(dirApp, v.Name()); err != nil {
			falliti = append(falliti, nome)
			continue
		}
		impostaGiocoTradotto(m.Cartella, false)
		tolti = append(tolti, nome)
	}
	return tolti, falliti
}

// CartellaDati: la cartella dei dati di Tasso Traduce (modelli, memoria, copie dei giochi).
func CartellaDati() string { return dirApp }
