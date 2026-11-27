// Package votopc: i conti del "Voto al PC" che non dipendono da Windows, così si provano anche su
// Linux (go test ./votopc/). Qui ci sono il voto della scheda video (dal modello e dalla memoria
// video, con una tabella scritta a mano) e la formula del voto totale, uguale su tutti i PC.
package votopc

import (
	"regexp"
	"strconv"
	"strings"
)

// Scheda: una scheda video come la vede Windows (DXGI).
type Scheda struct {
	Nome     string `json:"nome"`
	VRAMMB   uint64 `json:"vramMB"` // memoria video dedicata
	Vendor   uint32 `json:"vendor"` // 0x10DE NVIDIA, 0x1002 AMD, 0x8086 Intel, 0x1414 Microsoft
	Software bool   `json:"software,omitempty"`
}

// Voto della scheda video da 1 a 100, sulla stessa scala degli altri voti: 70 = PC da gioco medio
// di oggi, 100 = il meglio in commercio, le schede integrate (dentro il processore) tra 8 e 45.
// I numeri vengono dalle prestazioni medie nei giochi a 1080p/1440p (ogni volta che la scheda va il
// doppio, il voto sale di circa 20-25), arrotondate a fasce.
var nvidia = map[string]int{
	// RTX 50
	"rtx 5090": 100, "rtx 5080": 97, "rtx 5070 ti": 93, "rtx 5070": 88, "rtx 5060 ti": 80, "rtx 5060": 76, "rtx 5050": 66,
	// RTX 40
	"rtx 4090": 100, "rtx 4080 super": 96, "rtx 4080": 95, "rtx 4070 ti super": 91, "rtx 4070 ti": 89,
	"rtx 4070 super": 88, "rtx 4070": 86, "rtx 4060 ti": 78, "rtx 4060": 74, "rtx 4050": 66,
	// RTX 30
	"rtx 3090 ti": 94, "rtx 3090": 92, "rtx 3080 ti": 91, "rtx 3080": 89, "rtx 3070 ti": 84, "rtx 3070": 82,
	"rtx 3060 ti": 78, "rtx 3060": 72, "rtx 3050 ti": 58, "rtx 3050": 60,
	// RTX 20
	"rtx 2080 ti": 82, "rtx 2080 super": 79, "rtx 2080": 77, "rtx 2070 super": 75, "rtx 2070": 72,
	"rtx 2060 super": 70, "rtx 2060": 66,
	// GTX 16
	"gtx 1660 ti": 60, "gtx 1660 super": 60, "gtx 1660": 57, "gtx 1650 ti": 50, "gtx 1650 super": 52, "gtx 1650": 48, "gtx 1630": 36,
	// GTX 10
	"gtx 1080 ti": 70, "gtx 1080": 64, "gtx 1070 ti": 62, "gtx 1070": 58, "gtx 1060": 50, "gtx 1050 ti": 38, "gtx 1050": 33,
	"gt 1030": 18, "gt 1010": 12,
	// GTX 900 e più vecchie
	"gtx 980 ti": 54, "gtx 980": 47, "gtx 970": 43, "gtx 960": 33, "gtx 950": 29, "gtx 750 ti": 20, "gtx 750": 17,
	"gt 730": 7, "gt 710": 5,
	// MX dei portatili (piccole: poco più di una integrata)
	"mx 570": 30, "mx 550": 26, "mx 450": 24, "mx 350": 18, "mx 330": 17, "mx 250": 16, "mx 230": 14, "mx 150": 15, "mx 130": 12, "mx 110": 9,
}

var amd = map[string]int{
	// RX 9000
	"rx 9070 xt": 92, "rx 9070": 88, "rx 9060 xt": 78, "rx 9060": 72,
	// RX 7000
	"rx 7900 xtx": 95, "rx 7900 xt": 91, "rx 7900 gre": 86, "rx 7800 xt": 84, "rx 7700 xt": 80, "rx 7700": 77,
	"rx 7650 gre": 72, "rx 7600 xt": 72, "rx 7600": 70,
	// RX 6000
	"rx 6950 xt": 89, "rx 6900 xt": 88, "rx 6800 xt": 86, "rx 6800": 83, "rx 6750 xt": 78, "rx 6750 gre": 76,
	"rx 6700 xt": 77, "rx 6700": 74, "rx 6650 xt": 70, "rx 6600 xt": 68, "rx 6600": 66, "rx 6500 xt": 46, "rx 6400": 40,
	// RX 5000
	"rx 5700 xt": 68, "rx 5700": 64, "rx 5600 xt": 61, "rx 5600": 58, "rx 5500 xt": 50, "rx 5500": 47, "rx 5300": 42,
	// Polaris e Vega
	"rx 590": 50, "rx 580": 48, "rx 570": 44, "rx 480": 46, "rx 470": 42, "rx 560": 32, "rx 550": 26, "rx 460": 28,
	"rx vega 64": 62, "rx vega 56": 58, "radeon vii": 66,
}

var (
	reNvidia = regexp.MustCompile(`\b(rtx|gtx|gt|mx)\s?(\d{3,4})(?:\s?(ti|super))?(?:\s?(super))?\b`)
	reAMD    = regexp.MustCompile(`\brx\s?(\d{3,4})\s?(xtx|xt|gre)?\s?(m|s)?\b`)
	reArc    = regexp.MustCompile(`\barc\s?(?:pro\s?)?([ab])(\d{3})(m)?\b`)
	reArcInt = regexp.MustCompile(`\barc\s?(\d{3})([vt])\b`)
	reRadM   = regexp.MustCompile(`\bradeon\s?(\d{3,4})([ms])\b`)
	reVega   = regexp.MustCompile(`\bvega\s?(\d{1,2})\b`)
	reUHD    = regexp.MustCompile(`\buhd\s?(?:graphics\s?)?(\d{3})\b`)
	reSpazi  = regexp.MustCompile(`\s+`)
)

var arc = map[string]int{"b580": 72, "b570": 68, "a770": 70, "a750": 66, "a580": 60, "a380": 40, "a310": 32}

// APU AMD: Radeon 890M, 780M...
var radeonM = map[string]int{"8060s": 72, "8050s": 64, "8040s": 56, "890m": 44, "880m": 40, "860m": 32, "840m": 24,
	"780m": 38, "760m": 34, "740m": 24, "680m": 34, "660m": 28, "610m": 14}

// normalizza: minuscolo, senza (TM) e (R), spazi singoli.
func normalizza(s string) string {
	s = strings.ToLower(s)
	for _, x := range []string{"(tm)", "(r)", "™", "®"} {
		s = strings.ReplaceAll(s, x, " ")
	}
	return strings.TrimSpace(reSpazi.ReplaceAllString(s, " "))
}

// VotoGPU: voto 1-100 dal nome e dalla memoria video. noto=false vuol dire stimato solo dalla
// memoria video (modello che la tabella non conosce).
func VotoGPU(nome string, vramMB uint64) (voto int, noto bool) {
	n := normalizza(nome)
	if n == "" {
		return 0, false
	}
	portatile := strings.Contains(n, "laptop") || strings.Contains(n, "mobile")
	maxq := strings.Contains(n, "max-q") || strings.Contains(n, "max q")
	meno := func(v int) int {
		switch {
		case maxq:
			v -= 9
		case portatile:
			v -= 7
		}
		return limita(v)
	}
	// NVIDIA
	if m := reNvidia.FindStringSubmatch(n); m != nil {
		k := m[1] + " " + m[2]
		if m[3] != "" {
			k += " " + m[3]
		}
		if m[4] != "" {
			k += " " + m[4]
		}
		if v, ok := nvidia[k]; ok {
			return meno(v), true
		}
		if v, ok := nvidia[m[1]+" "+m[2]]; ok { // variante non in tabella: uso il modello base
			return meno(v), true
		}
	}
	// Intel Arc dedicate (A770, B580...) e integrate (Arc 140V, Arc Graphics dei Core Ultra)
	if m := reArc.FindStringSubmatch(n); m != nil {
		if v, ok := arc[m[1]+m[2]]; ok {
			if m[3] == "m" {
				v -= 8
			}
			return limita(v), true
		}
	}
	if m := reArcInt.FindStringSubmatch(n); m != nil {
		switch m[1] {
		case "140":
			return 40, true
		case "130":
			return 36, true
		}
		return 34, true
	}
	if strings.Contains(n, "intel") && strings.Contains(n, "arc") && strings.Contains(n, "graphics") {
		return 34, true
	}
	// AMD dedicate
	if m := reAMD.FindStringSubmatch(n); m != nil {
		k := "rx " + m[1]
		if m[2] != "" {
			k += " " + m[2]
		}
		v, ok := amd[k]
		if !ok {
			v, ok = amd["rx "+m[1]]
		}
		if ok {
			if m[3] == "m" || m[3] == "s" { // portatili: RX 6600M, RX 7600S
				v -= 6
			}
			return meno(v), true
		}
	}
	for _, k := range []string{"rx vega 64", "rx vega 56", "radeon vii"} {
		if strings.Contains(n, k) {
			return amd[k], true
		}
	}
	// AMD integrate
	if m := reRadM.FindStringSubmatch(n); m != nil {
		if v, ok := radeonM[m[1]+m[2]]; ok {
			return v, true
		}
	}
	if m := reVega.FindStringSubmatch(n); m != nil {
		x, _ := strconv.Atoi(m[1])
		switch {
		case x >= 11:
			return 20, true
		case x >= 8:
			return 18, true
		default:
			return 13, true
		}
	}
	if strings.Contains(n, "radeon") && strings.Contains(n, "graphics") && !strings.Contains(n, "rx") && !strings.Contains(n, "pro") {
		return 22, true // "AMD Radeon(TM) Graphics" dei Ryzen 4000-7000
	}
	// Intel integrate
	switch {
	case strings.Contains(n, "iris xe"):
		return 26, true
	case strings.Contains(n, "iris plus"):
		return 18, true
	case strings.Contains(n, "iris"):
		return 16, true
	}
	if m := reUHD.FindStringSubmatch(n); m != nil {
		x, _ := strconv.Atoi(m[1])
		if x >= 700 {
			return 18, true
		}
		return 12, true
	}
	if strings.Contains(n, "uhd graphics") {
		return 14, true
	}
	if strings.Contains(n, "hd graphics") {
		return 9, true
	}
	return stimaVRAM(n, vramMB), false
}

// stimaVRAM: per le schede che la tabella non conosce (professionali, nuove, rare) uso la memoria
// video dedicata, con voti prudenti.
func stimaVRAM(n string, mb uint64) int {
	gb := float64(mb) / 1024
	v := 10
	switch {
	case gb >= 20:
		v = 80
	case gb >= 15:
		v = 74
	case gb >= 11:
		v = 68
	case gb >= 7.5:
		v = 60
	case gb >= 5.5:
		v = 52
	case gb >= 3.5:
		v = 42
	case gb >= 1.8:
		v = 28
	case gb >= 0.9:
		v = 18
	}
	if strings.Contains(n, "laptop") || strings.Contains(n, "max-q") {
		v -= 7
	}
	return limita(v)
}

func limita(v int) int {
	if v < 1 {
		return 1
	}
	if v > 100 {
		return 100
	}
	return v
}

// escludi: adattatori che non sono schede vere (driver base di Windows, desktop remoto, virtuali).
func escludi(s Scheda) bool {
	if s.Software || s.Vendor == 0x1414 {
		return true
	}
	n := normalizza(s.Nome)
	for _, x := range []string{"basic render", "basic display", "remote", "virtual", "parsec", "meta ", "citrix", "vmware", "hyper-v"} {
		if strings.Contains(n, x) {
			return true
		}
	}
	return n == ""
}

// SceltaGPU: con più schede (integrata + dedicata) vale quella con il voto più alto, cioè la
// dedicata: è quella che usano i giochi. ok=false se non c'è nessuna scheda vera.
func SceltaGPU(schede []Scheda) (best Scheda, voto int, noto bool, ok bool) {
	for _, s := range schede {
		if escludi(s) {
			continue
		}
		v, k := VotoGPU(s.Nome, s.VRAMMB)
		if !ok || v > voto || (v == voto && s.VRAMMB > best.VRAMMB) {
			best, voto, noto, ok = s, v, k, true
		}
	}
	return
}
