package main

// UI Automation di Windows (la stessa che usano i lettori di schermo), chiamata a mano senza cgo:
// serve al Portachiavi per capire su che campo hai cliccato (utente o password), dove sta sullo
// schermo, qual è l'indirizzo della pagina nel browser e per riempire il campo senza tastiera.
// Si usa solo dal filo di vtLavoro in vault_compila.go (COM inizializzato lì, modello MTA).

import (
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/autotipo"
)

var (
	ole32               = windows.NewLazySystemDLL("ole32.dll")
	oleaut32            = windows.NewLazySystemDLL("oleaut32.dll")
	pCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	pCoCreateInstance   = ole32.NewProc("CoCreateInstance")
	pSysAllocString     = oleaut32.NewProc("SysAllocString")
	pSysFreeString      = oleaut32.NewProc("SysFreeString")
	pSysStringLen       = oleaut32.NewProc("SysStringLen")
	clsidCUIAutomation  = windows.GUID{Data1: 0xFF48DBA4, Data2: 0x60EF, Data3: 0x4201, Data4: [8]byte{0xAA, 0x87, 0x54, 0x10, 0x3E, 0xEF, 0x59, 0x4E}}
	iidIUIAutomation    = windows.GUID{Data1: 0x30CBE57D, Data2: 0xD9D0, Data3: 0x452A, Data4: [8]byte{0xAB, 0x13, 0x7A, 0xC5, 0xAC, 0x48, 0x25, 0xEE}}
	iidIUIValuePattern  = windows.GUID{Data1: 0xA94CD8B1, Data2: 0x0844, Data3: 0x4CD6, Data4: [8]byte{0x9D, 0x2D, 0x64, 0x05, 0x37, 0xAB, 0x39, 0xE9}}
	uiaValuePatternID   = uintptr(10002)
	uiaIsPasswordProp   = uintptr(30019)
	uiaControlEdit      = int32(50004)
	uiaControlDocument  = int32(50030)
	uiaTreeDescendants  = uintptr(4)
	uiaBrowserAddrNames = []string{"indirizz", "address", "url", "adresse", "dirección", "adresse und suchleiste"}
)

// comObj è un puntatore a un oggetto COM: la prima cosa dentro è la tabella delle funzioni.
type comObj uintptr

// puntatore: un indirizzo di memoria di Windows (fuori dal Go) come unsafe.Pointer.
// Passa dalla variabile perché go vet non lo segnali come uso sbagliato di unsafe.Pointer.
func puntatore(u uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&u)) }

func (o comObj) call(idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(puntatore(uintptr(o)))
	fn := *(*uintptr)(puntatore(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(o)}, args...)...)
	return r
}

func (o comObj) release() {
	if o != 0 {
		o.call(2)
	}
}

func (o comObj) addRef() {
	if o != 0 {
		o.call(1)
	}
}

func hrOK(r uintptr) bool { return int32(r) >= 0 }

// variant: VARIANT di 24 byte (x64).
type variant struct {
	vt  uint16
	_   [3]uint16
	val uintptr
	_   uintptr
}

func bstrToString(b uintptr) string {
	if b == 0 {
		return ""
	}
	n, _, _ := pSysStringLen.Call(b)
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(puntatore(b)), int(n)))
}

// uia è l'oggetto IUIAutomation del filo di vtLavoro.
type uia struct{ a comObj }

func newUIA() (*uia, bool) {
	pCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
	var a comObj
	r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0, 1|4, // INPROC_SERVER | LOCAL_SERVER
		uintptr(unsafe.Pointer(&iidIUIAutomation)), uintptr(unsafe.Pointer(&a)))
	if !hrOK(r) || a == 0 {
		return nil, false
	}
	return &uia{a: a}, true
}

// focused: l'elemento che ha la tastiera adesso (da liberare con release).
func (u *uia) focused() comObj {
	var e comObj
	if !hrOK(u.a.call(8, uintptr(unsafe.Pointer(&e)))) {
		return 0
	}
	return e
}

func (u *uia) fromHandle(h uintptr) comObj {
	var e comObj
	if !hrOK(u.a.call(6, h, uintptr(unsafe.Pointer(&e)))) {
		return 0
	}
	return e
}

func (u *uia) same(a, b comObj) bool {
	if a == 0 || b == 0 {
		return false
	}
	var same int32
	if !hrOK(u.a.call(3, uintptr(a), uintptr(b), uintptr(unsafe.Pointer(&same)))) {
		return false
	}
	return same != 0
}

// condProp: condizione "proprietà = valore" (valore intero o booleano).
func (u *uia) condProp(prop uintptr, vt uint16, val uintptr) comObj {
	v := variant{vt: vt, val: val}
	var c comObj
	// Il VARIANT (16+ byte) su x64 si passa come puntatore a una copia.
	if !hrOK(u.a.call(23, prop, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&c)))) {
		return 0
	}
	return c
}

func (u *uia) rawWalker() comObj {
	var w comObj
	if !hrOK(u.a.call(16, uintptr(unsafe.Pointer(&w)))) {
		return 0
	}
	return w
}

// --- IUIAutomationElement ---

func elFindFirst(e comObj, scope uintptr, cond comObj) comObj {
	if e == 0 || cond == 0 {
		return 0
	}
	var f comObj
	if !hrOK(e.call(5, scope, uintptr(cond), uintptr(unsafe.Pointer(&f)))) {
		return 0
	}
	return f
}

func elInt(e comObj, idx int) int32 {
	var v int32
	if e == 0 || !hrOK(e.call(idx, uintptr(unsafe.Pointer(&v)))) {
		return 0
	}
	return v
}

func elStr(e comObj, idx int) string {
	var b uintptr
	if e == 0 || !hrOK(e.call(idx, uintptr(unsafe.Pointer(&b)))) {
		return ""
	}
	defer pSysFreeString.Call(b)
	return bstrToString(b)
}

func elPid(e comObj) uint32      { return uint32(elInt(e, 20)) }
func elType(e comObj) int32      { return elInt(e, 21) }
func elName(e comObj) string     { return elStr(e, 23) }
func elHasFocus(e comObj) bool   { return elInt(e, 26) != 0 }
func elEnabled(e comObj) bool    { return elInt(e, 28) != 0 }
func elAutoID(e comObj) string   { return elStr(e, 29) }
func elIsPassword(e comObj) bool { return elInt(e, 35) != 0 }
func elOffscreen(e comObj) bool  { return elInt(e, 38) != 0 }

func elRect(e comObj) (rect, bool) {
	var r rect // RECT in pixel veri dello schermo
	if e == 0 || !hrOK(e.call(43, uintptr(unsafe.Pointer(&r)))) || r.Right <= r.Left || r.Bottom <= r.Top {
		return rect{}, false
	}
	return r, true
}

// elValuePattern: il "campo con un valore" dell'elemento (0 se non c'è).
func elValuePattern(e comObj) comObj {
	var p comObj
	if e == 0 || !hrOK(e.call(14, uiaValuePatternID, uintptr(unsafe.Pointer(&iidIUIValuePattern)), uintptr(unsafe.Pointer(&p)))) {
		return 0
	}
	return p
}

// elValue legge il testo del campo (le password Windows non le dà: torna vuoto).
func elValue(e comObj) string {
	p := elValuePattern(e)
	if p == 0 {
		return ""
	}
	defer p.release()
	return elStr(p, 4)
}

func elReadOnly(e comObj) bool {
	p := elValuePattern(e)
	if p == 0 {
		return true
	}
	defer p.release()
	return elInt(p, 5) != 0
}

// elSetValue riempie il campo. Il testo passa in un BSTR che cancello subito dopo.
func elSetValue(e comObj, u16 []uint16) bool {
	p := elValuePattern(e)
	if p == 0 {
		return false
	}
	defer p.release()
	if len(u16) == 0 || u16[len(u16)-1] != 0 {
		return false
	}
	b, _, _ := pSysAllocString.Call(uintptr(unsafe.Pointer(&u16[0])))
	if b == 0 {
		return false
	}
	ok := hrOK(p.call(3, b))
	// azzero la copia prima di liberarla (la password non resta nella memoria liberata)
	n, _, _ := pSysStringLen.Call(b)
	clear(unsafe.Slice((*uint16)(puntatore(b)), int(n)))
	pSysFreeString.Call(b)
	return ok
}

// --- ricerche ---

// insideDocument: il campo sta dentro la pagina (Document) e non nella barra del browser.
func (u *uia) insideDocument(e comObj) bool {
	w := u.rawWalker()
	if w == 0 {
		return false
	}
	defer w.release()
	cur := e
	cur.addRef()
	for i := 0; i < 60 && cur != 0; i++ {
		if elType(cur) == uiaControlDocument {
			cur.release()
			return true
		}
		var p comObj
		ok := hrOK(w.call(3, uintptr(cur), uintptr(unsafe.Pointer(&p))))
		cur.release()
		if !ok {
			return false
		}
		cur = p
	}
	cur.release()
	return false
}

// findPassword: il primo campo password visibile dentro la finestra (0 se non c'è).
func (u *uia) findPassword(root comObj) comObj {
	c := u.condProp(uiaIsPasswordProp, 11, ^uintptr(0)) // VT_BOOL, VARIANT_TRUE (-1)
	if c == 0 {
		return 0
	}
	defer c.release()
	return elFindFirst(root, uiaTreeDescendants, c)
}

// browserURL: l'indirizzo scritto nella barra del browser. Giro l'albero della finestra (al massimo
// 600 elementi) senza entrare nella pagina (Document): così una pagina non può fingersi la barra.
// Prendo il campo di testo che si chiama "barra degli indirizzi" (o "urlbar" in Firefox); se fuori
// dalla pagina c'è un solo campo di testo, è per forza quello.
func (u *uia) browserURL(top uintptr) string {
	root := u.fromHandle(top)
	if root == 0 {
		return ""
	}
	w := u.rawWalker()
	if w == 0 {
		root.release()
		return ""
	}
	defer w.release()
	coda := []comObj{root}
	defer func() {
		for _, e := range coda {
			e.release()
		}
	}()
	best, fuori, visti := "", 0, 0
	for len(coda) > 0 && visti < 600 {
		e := coda[0]
		coda = coda[1:]
		visti++
		t := elType(e)
		if t == uiaControlEdit {
			fuori++
			v := strings.TrimSpace(elValue(e))
			name := strings.ToLower(elName(e) + " " + elAutoID(e))
			if v != "" && !strings.ContainsAny(v, " \t") {
				isAddr := strings.Contains(name, "urlbar")
				for _, k := range uiaBrowserAddrNames {
					if strings.Contains(name, k) {
						isAddr = true
					}
				}
				if isAddr {
					e.release()
					return v
				}
				best = v
			}
		}
		if t != uiaControlDocument {
			var c comObj
			if hrOK(w.call(4, uintptr(e), uintptr(unsafe.Pointer(&c)))) && c != 0 {
				for c != 0 && len(coda) < 600 {
					coda = append(coda, c)
					var n comObj
					if !hrOK(w.call(6, uintptr(c), uintptr(unsafe.Pointer(&n)))) {
						break
					}
					c = n
				}
			}
		}
		e.release()
	}
	if fuori == 1 {
		return best
	}
	return ""
}

// --- Salva questo account / Crea password sicura ---

const (
	uiaControlTypeProp = uintptr(30003)
	uiaControlList     = int32(50008)
	uiaControlListItem = int32(50007)
	uiaControlMenu     = int32(50009)
	uiaControlMenuItem = int32(50011)
)

// elFindAll: tutti gli elementi che rispettano la condizione (da rilasciare con releaseAll).
func elFindAll(e comObj, scope uintptr, cond comObj, max int) []comObj {
	if e == 0 || cond == 0 {
		return nil
	}
	var arr comObj
	if !hrOK(e.call(6, scope, uintptr(cond), uintptr(unsafe.Pointer(&arr)))) || arr == 0 {
		return nil
	}
	defer arr.release()
	var n int32
	if !hrOK(arr.call(3, uintptr(unsafe.Pointer(&n)))) {
		return nil
	}
	if int(n) > max {
		n = int32(max)
	}
	out := make([]comObj, 0, n)
	for i := int32(0); i < n; i++ {
		var x comObj
		if hrOK(arr.call(4, uintptr(i), uintptr(unsafe.Pointer(&x)))) && x != 0 {
			out = append(out, x)
		}
	}
	return out
}

func releaseAll(l []comObj) {
	for _, e := range l {
		e.release()
	}
}

// passwords: i campi password della finestra (al massimo 8).
func (u *uia) passwords(top uintptr) []comObj {
	root := u.fromHandle(top)
	if root == 0 {
		return nil
	}
	defer root.release()
	c := u.condProp(uiaIsPasswordProp, 11, ^uintptr(0))
	if c == 0 {
		return nil
	}
	defer c.release()
	return elFindAll(root, uiaTreeDescendants, c, 8)
}

// findUser: l'email o il nome utente già scritto nella pagina (campo di testo di accesso, non
// password). Windows non dà mai il testo dei campi password: qui si legge solo l'utente.
func (u *uia) findUser(top uintptr) string {
	root := u.fromHandle(top)
	if root == 0 {
		return ""
	}
	defer root.release()
	c := u.condProp(uiaControlTypeProp, 3, uintptr(uiaControlEdit)) // VT_I4
	if c == 0 {
		return ""
	}
	defer c.release()
	campi := elFindAll(root, uiaTreeDescendants, c, 60)
	defer releaseAll(campi)
	primo := ""
	for _, e := range campi {
		if elIsPassword(e) || !u.insideDocument(e) {
			continue
		}
		v := strings.TrimSpace(elValue(e))
		if v == "" || len(v) > 200 || !autotipo.CampoAccesso(elName(e), elAutoID(e)) {
			continue
		}
		if strings.Contains(v, "@") {
			return v
		}
		if primo == "" {
			primo = v
		}
	}
	return primo
}

// fromPoint: l'elemento sotto il punto dello schermo.
func (u *uia) fromPoint(x, y int32) comObj {
	var e comObj
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32 // POINT passato per valore (x64)
	if !hrOK(u.a.call(7, pt, uintptr(unsafe.Pointer(&e)))) {
		return 0
	}
	return e
}

// occupatoSotto: subito sotto il campo c'è già la tendina di un altro programma o di un altro
// gestore di password (finestra di un altro processo, elenco o menu, o una pagina dentro la pagina
// come fanno le estensioni)? Allora la finestrella va sopra il campo.
func (u *uia) occupatoSotto(campo rect, pid uint32) bool {
	w := u.rawWalker()
	if w == 0 {
		return false
	}
	defer w.release()
	larg := campo.Right - campo.Left
	mio := uint32(os.Getpid())
	for _, f := range []int32{30, 97} {
		x := campo.Left + larg*f/100
		y := campo.Bottom + (campo.Bottom-campo.Top)/2 + 4
		e := u.fromPoint(x, y)
		if e == 0 {
			continue
		}
		if elPid(e) == mio { // c'è già la finestrella di Tasso: sotto non vedo
			e.release()
			continue
		}
		occ := elPid(e) != pid
		docs, elenco := 0, false
		for i := 0; e != 0 && i < 40 && !occ; i++ {
			switch elType(e) {
			case uiaControlList, uiaControlListItem, uiaControlMenu, uiaControlMenuItem:
				elenco = true
			case uiaControlDocument:
				docs++
				occ = docs >= 2
			}
			var p comObj
			if !hrOK(w.call(3, uintptr(e), uintptr(unsafe.Pointer(&p)))) {
				p = 0
			}
			e.release()
			e = p
		}
		if e != 0 {
			e.release()
		}
		// un elenco fuori dalla pagina è la tendina del browser (password, suggerimenti)
		if occ || (elenco && docs == 0) {
			return true
		}
	}
	return false
}
