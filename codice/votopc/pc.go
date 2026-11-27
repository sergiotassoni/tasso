package votopc

import (
	"regexp"
	"strings"
)

// ---------------- Portatile o PC fisso ----------------

// Telai da portatile (SMBIOS): 8 portatile, 9 laptop, 10 notebook, 11 palmare, 12 docking,
// 14 sub-notebook, 30 tablet, 31 convertibile, 32 staccabile. Non 18 e 21 (contenitori di
// espansione e periferiche: alcuni BIOS economici li scrivono a caso). Stessa regola di perf.go.
var telaiPortatili = map[int]bool{8: true, 9: true, 10: true, 11: true, 12: true, 14: true, 30: true, 31: true, 32: true}

// Portatile: tipo di PC 2 (mobile) oppure telaio da portatile. La batteria conta solo se Windows
// non dice niente né del tipo (0) né del telaio (1 Altro / 2 Sconosciuto), e mai se è un UPS:
// un gruppo di continuità collegato in USB compare in Windows come batteria. Nel dubbio: fisso
// (al massimo un portatile prende il voto della desktop, mai il contrario).
func Portatile(pcSystemType int, telai []int, batterie []string) bool {
	if pcSystemType == 2 {
		return true
	}
	ignoti := true
	for _, c := range telai {
		if telaiPortatili[c] {
			return true
		}
		if c != 1 && c != 2 {
			ignoti = false
		}
	}
	if pcSystemType != 0 || !ignoti {
		return false
	}
	for _, b := range batterie {
		n := strings.ToLower(b)
		if strings.Contains(n, "ups") || strings.Contains(n, "uninterrupt") || strings.Contains(n, "back-ups") {
			continue
		}
		return true
	}
	return false
}

// ---------------- Disco ----------------

// TipoDiscoWindows: il tipo che dice Windows (MSFT_PhysicalDisk). BusType 17 = NVMe; MediaType
// 4 = SSD, 3 = HDD. Con i controller RAID / Intel RST / VMD (BusType 8) o sconosciuti un NVMe
// può sembrare un SSD qualsiasi: lì il tipo non è sicuro e torna "" (si usa la stima).
func TipoDiscoWindows(bus, media string) string {
	b, m := strings.ToLower(strings.TrimSpace(bus)), strings.ToLower(strings.TrimSpace(media))
	switch b {
	case "17", "nvme":
		return "nvme"
	case "8", "raid", "0", "unknown", "16", "spaces", "":
		return ""
	}
	switch m {
	case "4", "ssd":
		return "ssd"
	case "3", "hdd":
		return "hdd"
	}
	return ""
}

// DiscoFinale: il tipo da mostrare. Se Windows l'ha detto con sicurezza vale quello; altrimenti la
// stima dalla velocità misurata, segnata come stimata ("probabilmente").
func DiscoFinale(windows string, punti int) (tipo string, stimato bool) {
	if windows != "" {
		return windows, false
	}
	t := TipoDisco(punti)
	return t, t != ""
}

// ---------------- Nomi della scheda madre ----------------

var (
	reFraseVuota = regexp.MustCompile(`(?i)to be filled by o\.?e\.?m\.?|default string|system product name|system manufacturer|not applicable|\bn/a\b`)
	reNomeVuoto  = regexp.MustCompile(`(?i)^(none|unknown|o\.?e\.?m\.?|[-_.\s]*)$`)
)

// PulisciNome toglie solo le frasi che il BIOS mette quando il costruttore non ha scritto niente
// e tiene il resto ("ASUSTeK Default string" → "ASUSTeK", "To be filled by O.E.M." → "").
func PulisciNome(s string) string {
	s = strings.Join(strings.Fields(reFraseVuota.ReplaceAllString(s, " ")), " ")
	if reNomeVuoto.MatchString(s) {
		return ""
	}
	return s
}
