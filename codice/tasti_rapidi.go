package main

// Tasti rapidi di Tasso fuori dalla traduzione: autoclick (di base F6), pannellino in gioco (di
// base Ctrl+Maiusc+F12) e scorciatoia del portachiavi. Codice = modificatori<<16 | tasto di
// Windows (1 Alt, 2 Ctrl, 4 Maiusc, 8 Win), come in traduce/tasti.go. Qui si controlla che due
// funzioni di Tasso non usino mai la stessa combinazione e che nessuna usi quelle di Windows.

import "tasso/traduce"

const (
	ovKeyBase = 6<<16 | 0x7B // Ctrl+Maiusc+F12
	acKeyBase = 0x75         // F6

	nomeTastoAc    = "Autoclick"
	nomeTastoOv    = "Pannellino in gioco"
	nomeTastoVault = "Portachiavi"
)

func ovKeyCode() uint32 {
	setMu.Lock()
	c := settings.OvKey
	setMu.Unlock()
	if c == 0 {
		return ovKeyBase
	}
	return c
}

func acKeyCode() uint32 {
	setMu.Lock()
	vk, mods := settings.AcVK, settings.AcMods
	setMu.Unlock()
	if vk == 0 {
		return acKeyBase
	}
	return (mods&0xF)<<16 | vk
}

// altriTastiTasso: per traduce.AltriTasti (codice -> nome della funzione).
func altriTastiTasso() map[uint32]string {
	setMu.Lock()
	vt := settings.VtKey
	setMu.Unlock()
	m := map[uint32]string{acKeyCode(): nomeTastoAc, ovKeyCode(): nomeTastoOv}
	if vt != 0 {
		m[vt] = nomeTastoVault
	}
	return m
}

// controllaTastoTasso: "" se la combinazione va bene per la funzione "chi" (uno dei nomi qui
// sopra), se no il motivo, in italiano (lo traduce trGo).
func controllaTastoTasso(c uint32, chi string) string {
	if m := traduce.TastoVietato(c); m != "" {
		return trGo(m)
	}
	if f := traduce.TastoGiaUsato(c, traduce.TastiTraduzione(), chi); f != "" {
		return trGo(traduce.EtichettaTasto(c)+" lo usa già «") + trGo(f) + trGo("»: scegline un altro.")
	}
	return ""
}

// setOverlayKey: cambia il tasto che mostra e nasconde il pannellino in gioco (0 = quello di base).
func setOverlayKey(c uint32) string {
	if c == 0 {
		c = ovKeyBase
	}
	if m := controllaTastoTasso(c, nomeTastoOv); m != "" {
		return m
	}
	old := ovKeyCode()
	save := func(v uint32) {
		if v == ovKeyBase {
			v = 0
		}
		updateSettings(func(s *Settings) { s.OvKey = v })
	}
	save(c)
	ovMu.Lock()
	h := ovHwnd
	ovMu.Unlock()
	if h == 0 {
		return "ok"
	}
	if r, _, _ := pSendMessage.Call(h, wmOvUpdate+3, 0, 0); r == 0 {
		save(old)
		pSendMessage.Call(h, wmOvUpdate+3, 0, 0)
		return trGo(traduce.EtichettaTasto(c) + " lo usa già Windows o un altro programma: scegline un altro.")
	}
	return "ok"
}

// ovKeyRegister (sul filo del pannellino): rimette il tasto del pannellino se era registrato,
// altrimenti lo prova un attimo. 1 = libero, 0 = già preso da un altro programma.
func ovKeyRegister(hwnd uintptr) uintptr {
	c := ovKeyCode()
	if ovHotkey {
		pUnregHotKey.Call(hwnd, 1)
		r, _, _ := pRegisterHotKey.Call(hwnd, 1, uintptr(c>>16)|0x4000, uintptr(c&0xFFFF))
		ovHotkey = r != 0
		if r != 0 {
			return 1
		}
		return 0
	}
	r, _, _ := pRegisterHotKey.Call(hwnd, 1, uintptr(c>>16)|0x4000, uintptr(c&0xFFFF))
	if r == 0 {
		return 0
	}
	pUnregHotKey.Call(hwnd, 1)
	return 1
}

// tastiRapidiInfo: i tasti fuori dalla traduzione, per la sezione Tasti rapidi.
func tastiRapidiInfo() map[string]interface{} {
	return map[string]interface{}{
		"ac": acKeyCode(), "acLabel": traduce.EtichettaTasto(acKeyCode()),
		"ov": ovKeyCode(), "ovLabel": traduce.EtichettaTasto(ovKeyCode()),
	}
}
