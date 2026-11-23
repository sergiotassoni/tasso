package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// Mouse senza accelerazione ("Precisione puntatore migliorata" spenta) solo per sparatutto e
// giochi competitivi, dentro il Boost. Il cambio NON viene scritto nelle impostazioni salvate di
// Windows (niente SPIF_UPDATEINIFILE): vale solo finché non lo rimetto, e in ogni caso torna com'era
// al prossimo accesso a Windows. Lo rimetto quando chiudi il gioco e con "Rimetti com'era".

var pSysParamInfo = user32.NewProc("SystemParametersInfoW")

const (
	spiGetMouse    = 0x0003
	spiSetMouse    = 0x0004
	spifSendChange = 0x0002
)

// Nomi dei programmi (senza .exe) dei giochi competitivi che non somigliano al nome del gioco.
var competitiveExe = map[string]bool{
	"r5apex": true, "r5apex_dx12": true, "valorant-win64-shipping": true, "valorant": true,
	"fortniteclient-win64-shipping": true, "cs2": true, "csgo": true, "cod": true, "cod24-cod": true,
	"modernwarfare": true, "blackops6": true, "rainbowsix": true, "rainbowsix_vulkan": true,
	"rainbowsix_be": true, "tslgame": true, "rocketleague": true, "overwatch": true,
	"marvel-win64-shipping": true, "dota2": true, "project8": true, "deadlock": true,
	"bf2042": true, "bf6": true, "escapefromtarkov": true, "huntgame": true, "deltaforceclient-win64-shipping": true,
	"splitgate": true, "tf_win64": true, "league of legends": true,
	"streetfighter6": true, "tekken8": true, "mk12": true, "rustclient": true, "deadbydaylight-win64-shipping": true,
	"halinfinite": true, "haloinfinite": true, "paladins": true, "brawlhalla": true, "aces": true,
	"worldoftanks": true, "thefinals": true, "pioneergame": true, "pioneergame-win64-shipping": true,
}

// competitiveGame: il gioco (nome del programma senza .exe) è uno sparatutto/competitivo?
func competitiveGame(name string) bool {
	n := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.ToLower(name), ".exe")))
	if n == "" {
		return false
	}
	if v, ok := competitiveExe[n]; ok {
		return v
	}
	// Anche "valorant-win64-shipping" ecc.: confronto con i nomi senza spazi e segni.
	flat := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	fn := flat(n)
	for _, k := range gameKinds {
		if k.kind != "competitivo" {
			continue
		}
		for _, g := range k.names {
			fg := flat(g)
			// Nomi corti ("rust", "dota", "cs2"): solo se il programma comincia così, non se li contiene.
			if (len(fg) >= 5 && strings.Contains(fn, fg)) || (len(fg) >= 3 && strings.HasPrefix(fn, fg)) {
				return true
			}
		}
	}
	return false
}

func mouseGet() ([3]int32, bool) {
	var v [3]int32
	r, _, _ := pSysParamInfo.Call(spiGetMouse, 0, uintptr(unsafe.Pointer(&v[0])), 0)
	return v, r != 0
}

func mouseSet(v [3]int32) bool {
	r, _, _ := pSysParamInfo.Call(spiSetMouse, 0, uintptr(unsafe.Pointer(&v[0])), spifSendChange)
	return r != 0
}

// mouseAccelOff toglie l'accelerazione e si segna com'era. Va chiamata con bMu NON bloccato.
// Restituisce il messaggio per te, o "" se non ha cambiato niente.
func mouseAccelOff(game string) string {
	if !competitiveGame(game) {
		return ""
	}
	bMu.Lock()
	defer bMu.Unlock()
	s := loadBoost()
	if s.Mouse != nil { // già tolta dal Boost
		return ""
	}
	cur, ok := mouseGet()
	if !ok || cur[2] == 0 { // accelerazione già spenta: non tocco niente
		return ""
	}
	prev := cur
	s.Mouse = &prev
	saveBoost() // prima mi segno com'era, poi cambio
	if !mouseSet([3]int32{0, 0, 0}) {
		s.Mouse = nil
		saveBoost()
		return ""
	}
	return "Mouse senza accelerazione per " + game + ": mira più precisa. Torna com'era quando chiudi il gioco."
}

// mouseAccelRestore rimette l'accelerazione com'era, solo se l'aveva tolta il Boost e non l'hai
// rimessa tu nel frattempo. Va chiamata con bMu NON bloccato.
func mouseAccelRestore() string {
	bMu.Lock()
	defer bMu.Unlock()
	s := loadBoost()
	if s.Mouse == nil {
		return ""
	}
	prev := *s.Mouse
	if cur, ok := mouseGet(); ok && cur[2] != 0 { // l'hai già cambiata tu: la lascio come l'hai messa
		s.Mouse = nil
		saveBoost()
		return ""
	}
	// Nel frattempo l'accelerazione è stata spenta per sempre (riparazione "mira non precisa" o
	// modalità FPS competitivo): non la riaccendo.
	if v, ok := regGet(registry.CURRENT_USER, `Control Panel\Mouse`, "MouseSpeed"); ok && strings.TrimSpace(v) == "0" {
		s.Mouse = nil
		saveBoost()
		return ""
	}
	if !mouseSet(prev) {
		return ""
	}
	s.Mouse = nil
	saveBoost()
	return "Mouse rimesso com'era."
}

// mouseAccelActive: il Boost ha tolto l'accelerazione del mouse?
func mouseAccelActive() bool {
	bMu.Lock()
	defer bMu.Unlock()
	return loadBoost().Mouse != nil
}
