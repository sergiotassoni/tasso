package main

// Scrivici (3.0): chi usa Tasso manda un'idea, un problema o un commento, con al massimo 2 foto.
// Il messaggio va alla "cassetta" di Tasso (feedbackURL, un piccolo servizio gratuito online).
// Tasso manda solo quello che la persona vede nel modulo: testo, foto scelte da lei, e i dati
// del PC solo se lascia la spunta. Se la cassetta non risponde, Tasso propone l'email già scritta.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

// cassettaURL: indirizzo della cassetta di Tasso (senza / finale), es. https://tasso-cassetta.NOME.workers.dev
// Vuoto = cassetta non ancora attiva: Scrivici propone l'email e i numeri anonimi non partono.
const cassettaURL = "https://tasso-cassetta.tasso-services.workers.dev"

const feedbackURL = cassettaURL + "/v1/messaggio"

const (
	feedbackMail     = "tasso.services@proton.me"
	feedbackMaxPhoto = 4 << 20 // 4 MB per foto
	feedbackMaxPics  = 2
	feedbackMaxText  = 3000
	feedbackMaxDay   = 5 // messaggi al giorno da questo PC
)

var (
	fbMu     sync.Mutex
	fbPicked = map[string]bool{} // solo le foto scelte con la finestra di Windows si possono mandare
)

var fbKinds = map[string]string{"idea": "Idea", "problema": "Problema", "commento": "Commento"}

// photoType: il tipo vero dell'immagine, letto dai primi byte (non dal nome del file).
func photoType(b []byte) string {
	switch t := http.DetectContentType(b); t {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return t
	}
	return ""
}

// readPhoto legge una foto e controlla che sia davvero un'immagine e non troppo grande.
func readPhoto(p string) ([]byte, string, error) {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return nil, "", fmt.Errorf("Non trovo la foto.")
	}
	if st.Size() > feedbackMaxPhoto {
		return nil, "", fmt.Errorf("La foto è troppo grande: al massimo 4 MB.")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, "", fmt.Errorf("Non riesco a leggere la foto.")
	}
	t := photoType(b)
	if t == "" {
		return nil, "", fmt.Errorf("Questo file non è una foto (PNG, JPG, GIF, WEBP o BMP).")
	}
	return b, t, nil
}

// pickFeedbackPhoto: finestra di Windows per scegliere una foto da allegare.
func pickFeedbackPhoto() map[string]interface{} {
	buf := make([]uint16, 4096)
	filter := utf16.Encode([]rune("Immagini\x00*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp\x00\x00"))
	title, _ := windows.UTF16PtrFromString(trGo("Scegli una foto da allegare"))
	var hwnd uintptr
	if wv != nil {
		hwnd = uintptr(wv.Window())
	}
	ofn := openFileName{
		hwndOwner:   hwnd,
		lpstrFilter: &filter[0],
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  title,
		flags:       0x00001000 | 0x00000800 | 0x00000008 | 0x00080000, // FILEMUSTEXIST|PATHMUSTEXIST|NOCHANGEDIR|EXPLORER
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFile.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return map[string]interface{}{"ok": false}
	}
	p := windows.UTF16ToString(buf)
	b, t, err := readPhoto(p)
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}
	}
	fbMu.Lock()
	fbPicked[p] = true
	fbMu.Unlock()
	return map[string]interface{}{
		"ok": true, "path": p, "name": filepath.Base(p), "size": len(b),
		"thumb": "data:" + t + ";base64," + base64.StdEncoding.EncodeToString(b),
	}
}

// fbCheck controlla tutto quello che arriva dal modulo, così non parte niente di strano.
func fbCheck(kind, text, contact, pc string, photos []string) (string, string, string, string, error) {
	if _, ok := fbKinds[kind]; !ok {
		return "", "", "", "", fmt.Errorf("Scegli se è un'idea, un problema o un commento.")
	}
	text = strings.TrimSpace(strings.ToValidUTF8(text, ""))
	if utf8.RuneCountInString(text) < 3 {
		return "", "", "", "", fmt.Errorf("Scrivi almeno qualche parola.")
	}
	if utf8.RuneCountInString(text) > feedbackMaxText {
		return "", "", "", "", fmt.Errorf("Il messaggio è troppo lungo: al massimo 3000 caratteri.")
	}
	contact = strings.TrimSpace(strings.ToValidUTF8(contact, ""))
	if utf8.RuneCountInString(contact) > 120 || strings.ContainsAny(contact, "\r\n") {
		return "", "", "", "", fmt.Errorf("Il contatto è troppo lungo.")
	}
	pc = strings.TrimSpace(strings.ToValidUTF8(pc, ""))
	if utf8.RuneCountInString(pc) > 400 {
		pc = string([]rune(pc)[:400])
	}
	if len(photos) > feedbackMaxPics {
		return "", "", "", "", fmt.Errorf("Al massimo 2 foto.")
	}
	fbMu.Lock()
	defer fbMu.Unlock()
	for _, p := range photos {
		if !fbPicked[p] {
			return "", "", "", "", fmt.Errorf("Scegli di nuovo la foto con il pulsante Aggiungi una foto.")
		}
	}
	return kind, text, contact, pc, nil
}

// fbReview: le stelline e il permesso di mettere il commento sul sito valgono solo per i commenti.
func fbReview(kind string, stars int, publish bool, name string) (int, bool, string) {
	if kind != "commento" || stars < 1 || stars > 5 {
		return 0, false, ""
	}
	if !publish {
		return stars, false, ""
	}
	name = strings.TrimSpace(strings.ToValidUTF8(name, ""))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '<' || r == '>' {
			return -1
		}
		return r
	}, name)
	if utf8.RuneCountInString(name) > 40 {
		name = string([]rune(name)[:40])
	}
	return stars, true, strings.TrimSpace(name)
}

func fbDayOK() bool {
	today := time.Now().Format("2006-01-02")
	setMu.Lock()
	defer setMu.Unlock()
	return settings.FeedbackDay != today || settings.FeedbackCount < feedbackMaxDay
}

func fbCount() {
	today := time.Now().Format("2006-01-02")
	updateSettings(func(s *Settings) {
		if s.FeedbackDay != today {
			s.FeedbackDay, s.FeedbackCount = today, 0
		}
		s.FeedbackCount++
		s.FeedbackSent++
	})
}

// sendFeedback manda il messaggio alla cassetta. Risponde sempre con onFeedbackDone.
func sendFeedback(kind, text, contact, pc string, photos []string, stars int, publish bool, name string) {
	done := func(ok bool, fallback bool, msg string) {
		emit("onFeedbackDone", map[string]interface{}{"ok": ok, "fallback": fallback, "msg": msg})
	}
	kind, text, contact, pc, err := fbCheck(kind, text, contact, pc, photos)
	if err != nil {
		done(false, false, err.Error())
		return
	}
	if !fbDayOK() {
		done(false, false, "Oggi hai già mandato 5 messaggi: grazie! Riprova domani.")
		return
	}
	if !strings.HasPrefix(cassettaURL, "https://") {
		done(false, true, "La cassetta dei messaggi non è ancora attiva: puoi mandarlo per email.")
		return
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	setMu.Lock()
	mode := settings.Mode
	setMu.Unlock()
	stars, publish, name = fbReview(kind, stars, publish, name)
	fields := map[string]string{"kind": kind, "text": text, "contact": contact, "pc": pc, "version": appVersion, "lang": curLang(), "mode": mode}
	if stars > 0 {
		fields["stelle"] = fmt.Sprint(stars)
	}
	if publish {
		fields["pubblica"] = "1"
		fields["nome"] = name
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for i, p := range photos {
		b, t, err := readPhoto(p)
		if err != nil {
			done(false, false, err.Error())
			return
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="photo%d"; filename="foto%d%s"`, i+1, i+1, strings.ToLower(filepath.Ext(p))))
		h.Set("Content-Type", t)
		w, _ := mw.CreatePart(h)
		_, _ = w.Write(b)
	}
	_ = mw.Close()
	req, err := http.NewRequest("POST", feedbackURL, &body)
	if err != nil {
		done(false, true, "Non riesco a preparare il messaggio: puoi mandarlo per email.")
		return
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", "Tasso/"+appVersion)
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		done(false, true, "Internet non risponde: controlla la connessione, oppure mandalo per email.")
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == 429 {
		done(false, false, "Troppi messaggi in poco tempo: riprova tra qualche minuto.")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		done(false, true, fmt.Sprintf("La cassetta non ha accettato il messaggio (codice %d): puoi mandarlo per email.", resp.StatusCode))
		return
	}
	fbCount()
	done(true, false, "Messaggio mandato. Grazie, lo leggo davvero!")
}

// feedbackMail apre l'email già scritta (le foto si allegano a mano).
func feedbackMailOpen(kind, text, pc string) string {
	kind, text, _, pc, err := fbCheck(kind, text, "", pc, nil)
	if err != nil {
		return err.Error()
	}
	if utf8.RuneCountInString(text) > 1500 {
		text = string([]rune(text)[:1500]) + "…"
	}
	body := text
	if pc != "" {
		body += "\n\n---\n" + pc
	}
	body += "\nTasso " + appVersion
	u := "mailto:" + feedbackMail + "?subject=" + url.PathEscape("Tasso "+appVersion+" · "+trGo(fbKinds[kind])) + "&body=" + url.PathEscape(body)
	openURL(u)
	return "ok"
}

func feedbackAsked() string {
	updateSettings(func(s *Settings) { s.FeedbackAsked = true })
	return "ok"
}
