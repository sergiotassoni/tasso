//go:build windows

package traduce

// 3.1 "Chat degli altri": scegli una volta dove sta la chat del gioco; Tasso la legge ogni
// secondo e mostra in italiano, accanto, i messaggi nuovi degli altri giocatori.
// Legge solo lo schermo (come la traduzione dello schermo): niente file del gioco, niente
// anti-imbroglio toccato.

import (
	"bytes"
	"context"
	"image"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ovChat: il riquadro con i messaggi tradotti, accanto alla chat del gioco.
var ovChat = &finestrella{dimFont: 18, posizione: "lato"}

// selPerChat: la prossima zona scelta col mouse è quella della chat (non quella dei dialoghi).
var selPerChat atomic.Bool

// chatFinestra: la finestra del gioco per cui si sta scegliendo la zona della chat.
var chatFinestra atomic.Uintptr

// chatGen cambia quando si sceglie una zona nuova o si spegne la chat: il ciclo ricomincia
// e segna come già viste le righe che ci sono (traduce solo quelle che arrivano dopo).
var chatGen atomic.Int64

const righeChatMostrate = 6

var (
	muChatAltri  sync.Mutex
	chatViste    = map[string]bool{} // righe già tradotte (per non ritradurle a ogni giro)
	chatOrdine   []string            // ordine delle righe viste, per dimenticare le più vecchie
	chatMostrate []string            // ultime righe tradotte, mostrate nel riquadro
	mieiMessaggi []string            // frasi copiate con la Chat tradotta: sono tue, non degli altri
)

// zonaChatDi: la zona della chat salvata per questo gioco (per programma, come la zona dei dialoghi).
func zonaChatDi(exe string) (Zona, bool) {
	muImp.Lock()
	defer muImp.Unlock()
	z := imp.Profili[exe].ZonaChat
	return z, z.Valida()
}

// giochiConChat: programma e cartella dei giochi con la zona della chat scelta (per la pagina del gioco).
func giochiConChat() []map[string]string {
	muImp.Lock()
	defer muImp.Unlock()
	out := []map[string]string{}
	for exe, p := range imp.Profili {
		if p.ZonaChat.Valida() {
			out = append(out, map[string]string{"exe": exe, "dove": p.ChatDove})
		}
	}
	return out
}

// zonaChatPresente: almeno un gioco ha la zona della chat scelta.
func zonaChatPresente(i Impostazioni) bool {
	muImp.Lock()
	defer muImp.Unlock()
	for _, p := range imp.Profili {
		if p.ZonaChat.Valida() {
			return true
		}
	}
	return false
}

// scegliZonaChat: Tasso va a icona, così davanti torna il gioco; dopo 3 secondi si apre il velo
// per disegnare il riquadro della chat sopra il gioco vero (non alla cieca).
func scegliZonaChat() {
	defer guard("zona-chat")
	var h uintptr
	if wv != nil {
		h = uintptr(wv.Window())
		pShowWindow.Call(h, 6) // SW_MINIMIZE
	}
	time.Sleep(3 * time.Second)
	fg, _, _ := pGetForeground.Call()
	if fg == 0 || eNostro(exeDiFinestra(fg)) {
		if h != 0 {
			pShowWindow.Call(h, 9) // SW_RESTORE
			pSetForeground.Call(h)
		}
		emetti("info", map[string]string{"testo": "Apri prima il gioco con la chat visibile, poi torna qui e premi di nuovo «Scegli dove sta la chat»."})
		return
	}
	if sel.hwnd.Load() != 0 {
		return // la scelta è già aperta (per esempio F9 nei 3 secondi): non la trasformo in quella della chat
	}
	chatFinestra.Store(fg)
	selPerChat.Store(true)
	ov.ApriSelezione()
}

// scegliZonaGioco: "Traduci solo una zona" da Tasso. Come per la chat: Tasso si toglie di mezzo,
// così il riquadro si disegna sopra il gioco e non sopra Tasso.
func scegliZonaGioco() {
	defer guard("zona-gioco")
	var h uintptr
	if wv != nil {
		h = uintptr(wv.Window())
		pShowWindow.Call(h, 6) // SW_MINIMIZE
	}
	time.Sleep(3 * time.Second)
	fg, _, _ := pGetForeground.Call()
	if fg == 0 || eNostro(exeDiFinestra(fg)) {
		if h != 0 {
			pShowWindow.Call(h, 9) // SW_RESTORE
			pSetForeground.Call(h)
		}
		emetti("info", map[string]string{"testo": "Apri prima il gioco, poi torna qui e premi di nuovo «Traduci solo una zona» (oppure premi " + nomeTasto("zona") + " nel gioco)."})
		return
	}
	if sel.hwnd.Load() != 0 {
		return // la scelta è già aperta (per esempio F9 nei 3 secondi)
	}
	selPerChat.Store(false)
	ov.RicordaGioco()
	ov.ApriSelezione()
}

// zonaChatScelta: arriva dalla scelta col mouse quando selPerChat è acceso.
func zonaChatScelta(z Zona) {
	f := chatFinestra.Load()
	exe := exeDiFinestra(f)
	if exe == "" || eNostro(exe) {
		avvisoBreve(testoTasso("Chat degli altri: non riconosco il programma del gioco, riprova con il gioco davanti.", "Other players' chat: I can't recognise the game program, try again with the game in front."))
		return
	}
	dove := strings.ToLower(filepath.Dir(percorsoDiFinestra(f)))
	muImp.Lock()
	if imp.Profili == nil {
		imp.Profili = map[string]Profilo{}
	}
	pr, ok := imp.Profili[exe]
	if !ok {
		// profilo nuovo: solo la chat. La zona dei dialoghi di adesso può essere di un altro gioco
		pr = Profilo{Posizione: imp.Posizione, DimFont: imp.DimFont}
	}
	pr.ZonaChat, pr.ChatDove = z, dove
	imp.Profili[exe] = pr
	imp.ZonaChat = z
	imp.ChatAltri = true
	muImp.Unlock()
	salvaImp()
	accendiTraduzione() // la chat degli altri legge lo schermo solo con la traduzione accesa
	chatGen.Add(1)
	azzeraChatAltri()
	emetti("aggiorna", stato())
	avvisoBreve(testoTasso("Chat degli altri accesa: i messaggi nuovi compaiono tradotti accanto alla chat.", "Other players' chat on: new messages appear translated next to the chat."))
}

func impostaChatAltri(on bool) {
	muImp.Lock()
	imp.ChatAltri = on
	muImp.Unlock()
	salvaImp()
	if !on {
		ovChat.Nascondi()
	}
	chatGen.Add(1)
	azzeraChatAltri()
	emetti("aggiorna", stato())
}

func azzeraChatAltri() {
	muChatAltri.Lock()
	chatViste, chatOrdine, chatMostrate = map[string]bool{}, nil, nil
	muChatAltri.Unlock()
}

// ricordaMioMessaggio: la frase inglese che hai copiato con la Chat tradotta, quando la rileggo
// nella chat non la traduco come se fosse di un altro.
func ricordaMioMessaggio(en string) {
	k := chiaveRiga(en)
	if len(k) < 4 {
		return
	}
	muChatAltri.Lock()
	mieiMessaggi = append(mieiMessaggi, k)
	if len(mieiMessaggi) > 20 {
		mieiMessaggi = mieiMessaggi[len(mieiMessaggi)-20:]
	}
	muChatAltri.Unlock()
}

// chiaveRiga: la stessa riga letta due volte può cambiare di poco (OCR): confronto senza
// spazi, punteggiatura e maiuscole.
func chiaveRiga(r string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(r) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c > 127 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// distanzaMax: true se a e b differiscono al massimo di max lettere (cambiate, tolte o aggiunte).
func distanzaMax(a, b string, max int) bool {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > max || -d > max {
		return false
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		minRiga := cur[0]
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
			minRiga = min(minRiga, cur[j])
		}
		if minRiga > max {
			return false
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)] <= max
}

// righeNuoveChat: dalle righe lette tiene solo quelle mai viste (e se le ricorda). Una riga
// quasi uguale a una delle ultime viste (una lettera letta diversa) conta come già vista;
// anche le tue frasi copiate con la Chat tradotta.
func righeNuoveChat(righe []string) []string {
	muChatAltri.Lock()
	defer muChatAltri.Unlock()
	var nuove []string
	for _, r := range righe {
		k := chiaveRiga(r)
		if len(k) < 2 || chatViste[k] {
			continue
		}
		gia := false
		if len(k) >= 10 {
			chi, _ := separaParlante(r)
			kc := chiaveRiga(chi)
			for i := len(chatOrdine) - 1; i >= 0 && i >= len(chatOrdine)-50; i-- {
				// solo righe dello stesso giocatore: "A: gg wp" e "B: gg wp" sono due messaggi
				if strings.HasPrefix(chatOrdine[i], kc) && distanzaMax(k, chatOrdine[i], 2) {
					gia = true
					break
				}
			}
		}
		for _, m := range mieiMessaggi {
			if strings.Contains(k, m) {
				gia = true
				break
			}
		}
		chatViste[k] = true
		chatOrdine = append(chatOrdine, k)
		if !gia {
			nuove = append(nuove, r)
		}
	}
	for len(chatOrdine) > 400 {
		delete(chatViste, chatOrdine[0])
		chatOrdine = chatOrdine[1:]
	}
	return nuove
}

// righeChat: le righe della chat lette dall'OCR (una riga = un messaggio).
func righeChat(grezzo string) []string {
	var out []string
	for _, r := range strings.Split(strings.ReplaceAll(grezzo, "\r", ""), "\n") {
		r = strings.TrimSpace(r)
		if r != "" && sembraTesto(r) {
			out = append(out, r)
		}
	}
	return out
}

func cicloChatAltri() {
	defer guard("chat-altri")
	nascosto := false
	nascondi := func() {
		if !nascosto {
			ovChat.Nascondi()
			nascosto = true
		}
	}
	var ultima Impronta
	primo := true
	ultimoExe, ultimaGen := "", int64(-1)
	var pausaFino time.Time // la lettura di Windows non è partita: riprovo più tardi
	avvisato := false
	for {
		// ogni 0,6 secondi: un messaggio nuovo compare tradotto quasi subito. Se la chat non è
		// cambiata il giro costa solo una piccola cattura (niente lettura del testo)
		time.Sleep(600 * time.Millisecond)
		i := leggiImp()
		if !acceso.Load() || !i.ChatAltri || paginaAttiva.Load() {
			nascondi()
			primo = true
			continue
		}
		fg, _, _ := pGetForeground.Call()
		exe := exeDiFinestra(fg)
		if fg == 0 || eNostro(exe) {
			nascondi() // davanti c'è Tasso o il desktop
			continue
		}
		zona, ok := zonaChatDi(exe)
		if !ok {
			nascondi() // davanti c'è un altro programma o un gioco senza zona della chat
			continue
		}
		gen := chatGen.Load()
		if exe != ultimoExe || gen != ultimaGen {
			// altro gioco o zona nuova: ricomincio, traduco solo i messaggi che arrivano da ora
			ultimoExe, ultimaGen, primo = exe, gen, true
			azzeraChatAltri()
			nascondi()
		}
		if nascosto {
			muChatAltri.Lock()
			testo := strings.Join(chatMostrate, "\n")
			muChatAltri.Unlock()
			if testo != "" {
				nascosto = false
				ovChat.Mostra(zona, testo, "", "")
			}
		}
		if time.Now().Before(pausaFino) {
			continue
		}
		img, err := cattura(zona)
		if err != nil || catturaNera(img) {
			continue
		}
		imp := calcolaImpronta(img)
		if !primo && imp.differenza(ultima) < sogliaFermo {
			continue // la chat non è cambiata
		}
		// lettura condivisa con la traduzione dello schermo: se è occupata riprovo al giro dopo
		if !occupato.CompareAndSwap(false, true) {
			continue
		}
		grezzo, ok := leggiChat(img)
		occupato.Store(false)
		if !ok {
			pausaFino = time.Now().Add(60 * time.Second)
			if !avvisato {
				avvisato = true
				emetti("info", map[string]string{"testo": "Chat degli altri: Windows non riesce a leggere il testo della chat. Riprovo tra un minuto."})
			}
			continue
		}
		avvisato = false
		ultima = imp
		lette := uniscriRigheChat(righeChat(grezzo))
		if primo {
			// la prima volta segno come già viste le righe vecchie: traduco solo quelle che arrivano dopo
			righeNuoveChat(lette)
			primo = false
			continue
		}
		// ancora valido: mentre traduco (con l'IA anche secondi) puoi spegnere, cambiare gioco o tornare in Tasso
		ancora := func() bool {
			f, _, _ := pGetForeground.Call()
			return f == fg && acceso.Load() && leggiImp().ChatAltri && !paginaAttiva.Load() && chatGen.Load() == gen
		}
		for _, r := range righeNuoveChat(lette) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			ris := traduttore.TraduciMessaggio(ctx, r)
			cancel()
			if !ancora() {
				break
			}
			if ris.Errore != "" || strings.TrimSpace(ris.Tradotto) == "" {
				continue
			}
			t := ris.Tradotto
			if ris.Parlante != "" {
				t = ris.Parlante + ": " + t
			}
			muChatAltri.Lock()
			chatMostrate = append(chatMostrate, t)
			if len(chatMostrate) > righeChatMostrate {
				chatMostrate = chatMostrate[len(chatMostrate)-righeChatMostrate:]
			}
			testo := strings.Join(chatMostrate, "\n")
			muChatAltri.Unlock()
			nascosto = false
			ovChat.Mostra(zona, testo, "", testoTasso("Chat degli altri · ", "Other players' chat · ")+nomeMotore(ris.Motore))
			aggiungiCronologia(ris, false)
			emetti("riga", ris)
		}
	}
}

// leggiChat: OCR della zona della chat (stesso lettore di Windows della traduzione dello schermo).
// Va chiamata con occupato preso.
func leggiChat(img *image.RGBA) (string, bool) {
	if assicuraOCR() != nil {
		return "", false
	}
	var buf bytes.Buffer
	if err := preparaPerOCR(img, &buf); err != nil {
		return "", false
	}
	g, err := ocr.Leggi(buf.Bytes())
	if err != nil {
		return "", false
	}
	return g, true
}
