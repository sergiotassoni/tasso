package traduce

// Numeri letti male dall'OCR di Windows nei menu dei giochi (visto nelle foto del 22/11/2026 su
// una schermata vera di Path of Titans): "IS%" invece di "15%", "1.2Sx" invece di "1.25x",
// "6.796" invece di "6.7%" (il simbolo % letto come "96").

import (
	"regexp"
	"strings"
)

var (
	reTokenNumero = regexp.MustCompile(`^[0-9SOIl.,:/%+\-x]+$`)
	// percentuale letta "96" dopo le parole delle statistiche: "Increased 6.796" -> "Increased 6.7%"
	rePercento96 = regexp.MustCompile(`(?i)\b(increased|decreased|reduced|boosted|resist|bonus|chance|rate|by)\s+([0-9]+(?:[.,][0-9]+)?)96\b`)
)

func correggiNumeriOCR(s string) string {
	parole := strings.Fields(s)
	for i, w := range parole {
		// solo parole fatte di cifre (con qualche lettera simile a una cifra): "1.2Sx", "18S/42S", "IS%"
		if !reTokenNumero.MatchString(w) || !strings.ContainsAny(w, "SOIl") || !(strings.ContainsAny(w, "0123456789") || strings.HasSuffix(w, "%")) {
			continue
		}
		parole[i] = strings.NewReplacer("S", "5", "O", "0", "I", "1", "l", "1").Replace(w)
	}
	s = strings.Join(parole, " ")
	return rePercento96.ReplaceAllStringFunc(s, func(m string) string {
		p := rePercento96.FindStringSubmatch(m)
		parola, num := strings.ToLower(p[1]), p[2]
		// "by 1996", "bonus 396": anni e numeri veri. Lo cambio solo con i decimali ("6.796")
		// o con un numero piccolo dopo le parole delle statistiche ("Increased 1096", non "by 1996")
		if !strings.ContainsAny(num, ".,") && !(parola != "by" && parola != "bonus" && len(num) <= 2) {
			return m
		}
		return p[1] + " " + num + "%"
	})
}
