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

const righeChatMostrate = 6

var (
	muChatAltri  sync.Mutex
	chatViste    = map[string]bool{} // righe già tradotte (per non ritradurle a ogni giro)
	chatOrdine   []string            // ordine delle righe viste, per dimenticare le più vecchie
	chatMostrate []string            // ultime righe tradotte, mostrate nel riquadro
)

// scegliZonaChat apre il velo per disegnare il riquadro della chat.
func scegliZonaChat() {
	selPerChat.Store(true)
	ov.DimenticaGioco()
	ov.ApriSelezione()
}

// zonaChatScelta: arriva dalla scelta col mouse quando selPerChat è acceso.
func zonaChatScelta(z Zona) {
	muImp.Lock()
	imp.ZonaChat = z
	imp.ChatAltri = true
	muImp.Unlock()
	salvaImp()
	azzeraChatAltri()
	emetti("aggiorna", stato())
	emetti("info", map[string]string{"testo": "Chat degli altri accesa: i messaggi nuovi compaiono in italiano accanto alla chat."})
}

func impostaChatAltri(on bool) {
	muImp.Lock()
	imp.ChatAltri = on
	muImp.Unlock()
	salvaImp()
	if !on {
		ovChat.Nascondi()
	}
	azzeraChatAltri()
	emetti("aggiorna", stato())
}

func azzeraChatAltri() {
	muChatAltri.Lock()
	chatViste, chatOrdine, chatMostrate = map[string]bool{}, nil, nil
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

// righeNuoveChat: dalle righe lette tiene solo quelle mai viste (e se le ricorda).
func righeNuoveChat(righe []string) []string {
	muChatAltri.Lock()
	defer muChatAltri.Unlock()
	var nuove []string
	for _, r := range righe {
		k := chiaveRiga(r)
		if len(k) < 2 || chatViste[k] {
			continue
		}
		chatViste[k] = true
		chatOrdine = append(chatOrdine, k)
		nuove = append(nuove, r)
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
	var ultima Impronta
	primo := true
	for {
		time.Sleep(time.Second)
		i := leggiImp()
		if !acceso.Load() || !i.ChatAltri || !i.ZonaChat.Valida() || paginaAttiva.Load() {
			if !nascosto {
				ovChat.Nascondi()
				nascosto = true
			}
			primo = true
			continue
		}
		// davanti c'è Tasso o il desktop: nascondo il riquadro e non leggo niente
		if fg, _, _ := pGetForeground.Call(); fg == 0 || eNostro(exeDiFinestra(fg)) {
			if !nascosto {
				ovChat.Nascondi()
				nascosto = true
			}
			continue
		}
		if nascosto {
			nascosto = false
			muChatAltri.Lock()
			testo := strings.Join(chatMostrate, "\n")
			muChatAltri.Unlock()
			if testo != "" {
				ovChat.Mostra(i.ZonaChat, testo, "", "")
			}
		}
		img, err := cattura(i.ZonaChat)
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
			continue
		}
		ultima = imp
		lette := righeChat(grezzo)
		if primo {
			// la prima volta segno come già viste le righe vecchie: traduco solo quelle che arrivano dopo
			righeNuoveChat(lette)
			primo = false
			continue
		}
		nuove := righeNuoveChat(lette)
		if len(nuove) == 0 {
			continue
		}
		for _, r := range nuove {
			ris := traduttore.Traduci(context.Background(), r)
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
			ovChat.Mostra(i.ZonaChat, testo, "", "Chat degli altri · "+ris.Motore)
			muCronologia.Lock()
			righe = append(righe, ris)
			if len(righe) > 1000 {
				righe = righe[len(righe)-1000:]
			}
			muCronologia.Unlock()
			emetti("riga", ris)
		}
	}
}

// leggiChat: OCR della zona della chat (stesso lettore di Windows della traduzione dello schermo).
// Va chiamata con occupato preso.
func leggiChat(img *image.RGBA) (string, bool) {
	if ocr == nil {
		l, err := avviaOCR(filepath.Join(dirApp, "lavoro"))
		if err != nil {
			return "", false
		}
		ocr, ocrErr = l, ""
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
