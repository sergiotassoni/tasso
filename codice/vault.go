package main

// Portachiavi: password salvate sul PC, chiuse con un PIN.
// Doppia protezione: cifratura AES-256-GCM con una chiave ricavata dal PIN (PBKDF2, 310.000 passaggi)
// e sopra la protezione di Windows (DPAPI), così il file copiato su un altro PC o account non si apre.
// Niente internet, niente cloud: il file resta in %LOCALAPPDATA%\Tasso. Se dimentichi il PIN le password
// non si possono recuperare (è il prezzo della sicurezza vera).

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

type vaultEntry struct {
	ID      string `json:"id"`
	Site    string `json:"site"`
	User    string `json:"user"`
	Pw      string `json:"pw"`
	Note    string `json:"note"`
	Changed string `json:"changed"`
}

type vaultFile struct {
	V     int    `json:"v"`
	Salt  []byte `json:"salt"`
	Iter  int    `json:"iter"`
	Nonce []byte `json:"nonce"`
	Data  []byte `json:"data"`
}

const vaultIter = 310000

var (
	vaultMu      sync.Mutex
	vaultKey     []byte
	vaultSalt    []byte
	vaultItems   []vaultEntry
	vaultTouched time.Time
	vaultTimer   *time.Timer
)

const vaultIdle = 5 * time.Minute

func vaultPath() string { return filepath.Join(appDir, "portachiavi.dat") }

// vaultExists: se Tasso si è chiuso a metà salvataggio, riprende la copia di sicurezza.
func vaultExists() bool {
	if fileExists(vaultPath()) {
		return true
	}
	if fileExists(vaultPath() + ".bak") {
		return os.Rename(vaultPath()+".bak", vaultPath()) == nil
	}
	return false
}

// I tentativi sbagliati restano salvati: chiudere e riaprire Tasso non dà tentativi in più.
type vaultGuard struct {
	Fails int       `json:"fails"`
	Until time.Time `json:"until"`
}

func guardPath() string { return filepath.Join(appDir, "portachiavi.tentativi") }

func loadGuard() vaultGuard {
	var g vaultGuard
	if b, err := os.ReadFile(guardPath()); err == nil {
		if raw, err := unprotect(b); err == nil {
			_ = json.Unmarshal(raw, &g)
		}
	}
	return g
}

func saveGuard(g vaultGuard) {
	if g.Fails == 0 {
		_ = os.Remove(guardPath())
		return
	}
	raw, _ := json.Marshal(g)
	if enc, err := protect(raw); err == nil {
		_ = os.WriteFile(guardPath(), enc, 0o600)
	}
}

// checkPIN prova il PIN contando i tentativi sbagliati (va chiamata con vaultMu preso).
func checkPIN(pin string) ([]vaultEntry, []byte, []byte, string) {
	g := loadGuard()
	if time.Now().Before(g.Until) {
		return nil, nil, nil, fmt.Sprintf("Troppi tentativi: aspetta %d secondi.", int(time.Until(g.Until).Seconds())+1)
	}
	items, key, salt, err := vaultRead(pin)
	if err != nil {
		if err == errWrongPIN {
			g.Fails++
			msg := err.Error()
			if g.Fails >= 5 {
				wait := 30 * (g.Fails - 4)
				if wait > 3600 {
					wait = 3600
				}
				g.Until = time.Now().Add(time.Duration(wait) * time.Second)
				msg = fmt.Sprintf("PIN sbagliato %d volte: aspetta %d secondi.", g.Fails, wait)
			}
			saveGuard(g)
			return nil, nil, nil, msg
		}
		return nil, nil, nil, err.Error()
	}
	saveGuard(vaultGuard{})
	return items, key, salt, ""
}

// Il PIN può essere da 4 a 12 numeri, oppure vuoto (portachiavi senza PIN, protetto solo
// dall'account di Windows: comodo, ma chi usa il tuo account lo apre).
func validPIN(pin string) bool {
	if pin == "" {
		return true
	}
	if len(pin) < 4 || len(pin) > 12 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func deriveKey(pin string, salt []byte, iter int) []byte {
	k, err := pbkdf2.Key(sha256.New, pin, salt, iter, 32)
	if err != nil {
		return nil
	}
	return k
}

// vaultWrite cifra e salva lo stato passato (va chiamata con vaultMu preso).
// Chi la chiama aggiorna la memoria solo se il salvataggio riesce, così file e schermo restano uguali.
func vaultWrite(items []vaultEntry, salt, key []byte) error {
	if items == nil {
		items = []vaultEntry{}
	}
	plain, _ := json.Marshal(items)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	vf := vaultFile{V: 1, Salt: salt, Iter: vaultIter, Nonce: nonce, Data: gcm.Seal(nil, nonce, plain, []byte("tasso-portachiavi"))}
	raw, _ := json.Marshal(vf)
	enc, err := protect(raw)
	if err != nil {
		return err
	}
	tmp := vaultPath() + ".tmp"
	if err := os.WriteFile(tmp, enc, 0o600); err != nil {
		return err
	}
	// Su Windows os.Rename sostituisce il file in un colpo solo (MoveFileEx): niente file a metà.
	if err := os.Rename(tmp, vaultPath()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func vaultRead(pin string) ([]vaultEntry, []byte, []byte, error) {
	enc, err := os.ReadFile(vaultPath())
	if err != nil {
		return nil, nil, nil, errors.New("Portachiavi non trovato.")
	}
	raw, err := unprotect(enc)
	if err != nil {
		return nil, nil, nil, errors.New("Questo portachiavi è stato creato con un altro account Windows o su un altro PC: da qui non si apre.")
	}
	var vf vaultFile
	if json.Unmarshal(raw, &vf) != nil || len(vf.Salt) < 16 || vf.Iter < 100000 || vf.Iter > 5000000 || len(vf.Nonce) != 12 {
		return nil, nil, nil, errors.New("Il file del portachiavi è rovinato.")
	}
	key := deriveKey(pin, vf.Salt, vf.Iter)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}
	plain, err := gcm.Open(nil, vf.Nonce, vf.Data, []byte("tasso-portachiavi"))
	if err != nil {
		return nil, nil, nil, errWrongPIN
	}
	var items []vaultEntry
	if json.Unmarshal(plain, &items) != nil {
		return nil, nil, nil, errors.New("Il file del portachiavi è rovinato.")
	}
	return items, key, vf.Salt, nil
}

var errWrongPIN = errors.New("PIN sbagliato.")

func vaultLocked() bool {
	if vaultKey == nil {
		return true
	}
	if time.Since(vaultTouched) > vaultIdle {
		vaultLockNow()
		return true
	}
	vaultTouch()
	return false
}

// vaultTouch fa ripartire i 5 minuti: allo scadere il portachiavi si chiude da solo,
// anche se la pagina resta aperta (va chiamata con vaultMu preso).
func vaultTouch() {
	vaultTouched = time.Now()
	if vaultTimer != nil {
		vaultTimer.Stop()
	}
	vaultTimer = time.AfterFunc(vaultIdle+time.Second, func() {
		vaultMu.Lock()
		closed := vaultKey != nil && time.Since(vaultTouched) > vaultIdle
		if closed {
			vaultLockNow()
		}
		vaultMu.Unlock()
		if closed {
			emit("onVaultLocked", nil)
		}
	})
}

func vaultLockNow() {
	for i := range vaultKey {
		vaultKey[i] = 0
	}
	vaultKey, vaultItems = nil, nil
	if vaultTimer != nil {
		vaultTimer.Stop()
		vaultTimer = nil
	}
}

func noPinPath() string { return filepath.Join(appDir, "portachiavi.senzapin") }

func vaultNoPin() bool {
	_, err := os.Stat(noPinPath())
	return err == nil
}

func setNoPin(on bool) {
	if on {
		_ = os.WriteFile(noPinPath(), []byte("Portachiavi senza PIN: si apre da solo con il tuo account di Windows.\r\n"), 0o600)
	} else {
		_ = os.Remove(noPinPath())
	}
}

func vaultStatus() map[string]interface{} {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	return map[string]interface{}{"exists": vaultExists(), "unlocked": !vaultLocked(), "noPin": vaultExists() && vaultNoPin()}
}

func vaultCreate(pin string) string {
	if !validPIN(pin) {
		return "Il PIN deve avere da 4 a 12 numeri (oppure scegli \"Senza PIN\")."
	}
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultExists() {
		return "Il portachiavi esiste già."
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "Errore interno."
	}
	key := deriveKey(pin, salt, vaultIter)
	if err := vaultWrite(nil, salt, key); err != nil {
		return "Non riesco a salvare il portachiavi."
	}
	saveGuard(vaultGuard{})
	setNoPin(pin == "")
	vaultSalt, vaultKey, vaultItems = salt, key, []vaultEntry{}
	vaultTouch()
	return "ok"
}

func vaultUnlock(pin string) string {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	items, key, salt, msg := checkPIN(pin)
	if msg != "" {
		if pin == "" && vaultNoPin() {
			// Il segno "senza PIN" è rimasto ma il portachiavi ha un PIN: lo tolgo e chiedo il PIN.
			setNoPin(false)
		}
		return msg
	}
	vaultItems, vaultKey, vaultSalt = items, key, salt
	vaultTouch()
	return "ok"
}

func vaultLock() string {
	vaultMu.Lock()
	vaultLockNow()
	vaultMu.Unlock()
	return "ok"
}

// pwStrength: 0 debole, 1 media, 2 forte.
func pwStrength(pw string) int {
	var lo, up, dg, sy bool
	for _, c := range pw {
		switch {
		case unicode.IsLower(c):
			lo = true
		case unicode.IsUpper(c):
			up = true
		case unicode.IsDigit(c):
			dg = true
		default:
			sy = true
		}
	}
	kinds := 0
	for _, b := range []bool{lo, up, dg, sy} {
		if b {
			kinds++
		}
	}
	n := len([]rune(pw))
	switch {
	case n >= 14 && kinds >= 3, n >= 12 && kinds == 4:
		return 2
	case n >= 10 && kinds >= 2:
		return 1
	}
	return 0
}

func vaultList() map[string]interface{} {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultLocked() {
		return map[string]interface{}{"locked": true}
	}
	seen := map[string]int{}
	for _, e := range vaultItems {
		if e.Pw != "" {
			seen[e.Pw]++
		}
	}
	list := []map[string]interface{}{}
	for _, e := range vaultItems {
		list = append(list, map[string]interface{}{
			"id": e.ID, "site": e.Site, "user": e.User, "note": e.Note, "changed": e.Changed,
			"strength": pwStrength(e.Pw), "reused": seen[e.Pw] > 1, "len": len([]rune(e.Pw)),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(fmt.Sprint(list[i]["site"])) < strings.ToLower(fmt.Sprint(list[j]["site"]))
	})
	return map[string]interface{}{"locked": false, "items": list}
}

func vaultFind(id string) int {
	for i, e := range vaultItems {
		if e.ID == id {
			return i
		}
	}
	return -1
}

func vaultSave(e vaultEntry) string {
	e.Site, e.User, e.Note = strings.TrimSpace(e.Site), strings.TrimSpace(e.User), strings.TrimSpace(e.Note)
	if e.Site == "" {
		return "Scrivi almeno il nome del sito o del programma."
	}
	if len(e.Site) > 200 || len(e.User) > 200 || len(e.Pw) > 500 || len(e.Note) > 2000 {
		return "Testo troppo lungo."
	}
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultLocked() {
		return "locked"
	}
	e.Changed = time.Now().Format("2006-01-02")
	items := append([]vaultEntry(nil), vaultItems...)
	if e.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		e.ID = hex.EncodeToString(b)
		items = append(items, e)
	} else if i := vaultFind(e.ID); i >= 0 {
		if e.Pw == "" {
			e.Pw = items[i].Pw // campo lasciato vuoto in modifica = password invariata
		}
		items[i] = e
	} else {
		return "Voce non trovata."
	}
	if err := vaultWrite(items, vaultSalt, vaultKey); err != nil {
		return "Non riesco a salvare."
	}
	vaultItems = items
	return "ok"
}

func vaultDelete(id string) string {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultLocked() {
		return "locked"
	}
	i := vaultFind(id)
	if i < 0 {
		return "Voce non trovata."
	}
	items := append(append([]vaultEntry(nil), vaultItems[:i]...), vaultItems[i+1:]...)
	if err := vaultWrite(items, vaultSalt, vaultKey); err != nil {
		return "Non riesco a salvare."
	}
	vaultItems = items
	return "ok"
}

func vaultReveal(id string) string {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultLocked() {
		return ""
	}
	if i := vaultFind(id); i >= 0 {
		return vaultItems[i].Pw
	}
	return ""
}

// vaultCopy copia utente o password negli appunti, fuori dalla cronologia di Win+V,
// e la password si cancella da sola dopo 30 secondi (se nel frattempo non hai copiato altro).
func vaultCopy(id, what string) string {
	vaultMu.Lock()
	if vaultLocked() {
		vaultMu.Unlock()
		return "locked"
	}
	i := vaultFind(id)
	if i < 0 {
		vaultMu.Unlock()
		return "Voce non trovata."
	}
	txt := vaultItems[i].User
	if what == "pw" {
		txt = vaultItems[i].Pw
	}
	vaultMu.Unlock()
	if txt == "" {
		return "Vuoto."
	}
	return copySecret(txt, what == "pw")
}

// copySecret copia fuori dalla cronologia di Win+V; con autoClear si cancella dopo 30 secondi
// (se nel frattempo non hai copiato altro). Usata anche per le password generate e quelle del Wi-Fi.
func copySecret(txt string, autoClear bool) string {
	if txt == "" || len(txt) > 4096 {
		return "Vuoto."
	}
	seq, ok := setClipboardSecret(txt)
	if !ok {
		return "Non riesco a usare gli appunti."
	}
	if autoClear {
		go func() {
			defer guard("vault.go:509")
			time.Sleep(30 * time.Second)
			if r, _, _ := pClipSeq.Call(); uint32(r) == seq {
				clearClipboard()
			}
		}()
	}
	return "ok"
}

func vaultChangePIN(oldPin, newPin string) string {
	if !validPIN(newPin) {
		return "Il nuovo PIN deve avere da 4 a 12 numeri (oppure lascialo vuoto per non usare il PIN)."
	}
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultLocked() {
		return "locked"
	}
	items, _, _, msg := checkPIN(oldPin)
	if msg != "" {
		return msg
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "Errore interno."
	}
	key := deriveKey(newPin, salt, vaultIter)
	if err := vaultWrite(items, salt, key); err != nil {
		return "Non riesco a salvare: il PIN resta quello di prima."
	}
	setNoPin(newPin == "")
	vaultLockNow()
	vaultItems, vaultSalt, vaultKey = items, salt, key
	vaultTouch()
	return "ok"
}

// vaultReset cancella il portachiavi (PIN dimenticato). Le password dentro sono perse.
func vaultReset() string {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	vaultLockNow()
	if err := os.Remove(vaultPath()); err != nil && !os.IsNotExist(err) {
		return "Non riesco a cancellarlo."
	}
	_ = os.Remove(vaultPath() + ".bak")
	_ = os.Remove(vaultPath() + ".tmp")
	_ = os.Remove(guardPath())
	setNoPin(false)
	return "ok"
}

// genPassword crea una password casuale vera (crypto/rand), senza caratteri che si confondono.
func genPassword(n int, symbols bool) string {
	if n < 8 {
		n = 8
	}
	if n > 64 {
		n = 64
	}
	sets := []string{"abcdefghijkmnopqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789"}
	if symbols {
		sets = append(sets, "!@#$%&*?-_+=")
	}
	all := strings.Join(sets, "")
	pick := func(s string) byte {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(s))))
		return s[x.Int64()]
	}
	for {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick(all)
		}
		ok := true
		for _, s := range sets {
			if !strings.ContainsAny(string(b), s) {
				ok = false
			}
		}
		if ok {
			return string(b)
		}
	}
}

// vaultPwned controlla se la password compare tra quelle rubate (Have I Been Pwned).
// Al sito arrivano solo i primi 5 caratteri dell'impronta SHA-1: la password non esce mai dal PC.
func vaultPwned(id string) map[string]interface{} {
	pw := vaultReveal(id)
	if pw == "" {
		return map[string]interface{}{"ok": false, "error": "Portachiavi chiuso o voce senza password."}
	}
	return pwnedCount(pw)
}

func pwnedCount(pw string) map[string]interface{} {
	sum := sha1.Sum([]byte(pw))
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	req, _ := http.NewRequest("GET", "https://api.pwnedpasswords.com/range/"+h[:5], nil)
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "Tasso")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]interface{}{"ok": false, "error": "Il servizio di controllo non risponde: controlla la connessione."}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return map[string]interface{}{"ok": false, "error": fmt.Sprintf("Il servizio di controllo ha risposto con un errore (%d).", resp.StatusCode)}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	count := 0
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 && parts[0] == h[5:] {
			fmt.Sscan(parts[1], &count)
		}
	}
	return map[string]interface{}{"ok": true, "count": count}
}

// ---------------- Appunti di Windows ----------------

var (
	pOpenClip     = user32.NewProc("OpenClipboard")
	pCloseClip    = user32.NewProc("CloseClipboard")
	pEmptyClip    = user32.NewProc("EmptyClipboard")
	pSetClipData  = user32.NewProc("SetClipboardData")
	pRegClipFmt   = user32.NewProc("RegisterClipboardFormatW")
	pClipSeq      = user32.NewProc("GetClipboardSequenceNumber")
	pGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	pGlobalLock   = kernel32.NewProc("GlobalLock")
	pGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	pGlobalFree   = kernel32.NewProc("GlobalFree")
)

func openClip() bool {
	for i := 0; i < 10; i++ {
		if r, _, _ := pOpenClip.Call(0); r != 0 {
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	return false
}

func globalBytes(b []byte) uintptr {
	h, _, _ := pGlobalAlloc.Call(0x0002, uintptr(len(b))) // GMEM_MOVEABLE
	if h == 0 {
		return 0
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return 0
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b)), b)
	pGlobalUnlock.Call(h)
	return h
}

// Gli appunti aperti appartengono al thread che li apre: tengo fermo il thread fino alla chiusura.
func setClipboardSecret(s string) (uint32, bool) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b := unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
	h := globalBytes(b)
	if h == 0 {
		return 0, false
	}
	if !openClip() {
		pGlobalFree.Call(h)
		return 0, false
	}
	pEmptyClip.Call()
	if r, _, _ := pSetClipData.Call(13, h); r == 0 { // CF_UNICODETEXT
		pGlobalFree.Call(h)
		pCloseClip.Call()
		return 0, false
	}
	// Fuori dalla cronologia degli appunti (Win+V) e dalla sincronizzazione cloud di Windows.
	for _, name := range []string{"ExcludeClipboardContentFromMonitorProcessing", "CanIncludeInClipboardHistory", "CanUploadToCloudClipboard"} {
		n, _ := windows.UTF16PtrFromString(name)
		f, _, _ := pRegClipFmt.Call(uintptr(unsafe.Pointer(n)))
		if f == 0 {
			continue
		}
		if hz := globalBytes([]byte{0, 0, 0, 0}); hz != 0 {
			if r, _, _ := pSetClipData.Call(f, hz); r == 0 {
				pGlobalFree.Call(hz)
			}
		}
	}
	pCloseClip.Call()
	r, _, _ := pClipSeq.Call()
	return uint32(r), true
}

func clearClipboard() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if openClip() {
		pEmptyClip.Call()
		pCloseClip.Call()
	}
}
