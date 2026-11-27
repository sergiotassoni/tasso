package nomeutente

// Controllo sulla finestra di Tasso (ui/index.html): il nome entra sempre con esc() o textContent.

import (
	"strings"
	"testing"
)

// Ogni riga che mette il nome dentro dell'HTML lo passa da esc().
func TestPaginaNomeSempreConEsc(t *testing.T) {
	s := pagina(t)
	usi := 0
	for i, riga := range strings.Split(s, "\n") {
		if !strings.Contains(riga, "nomeOra()") && !strings.Contains(riga, "nomeSulBiglietto()") {
			continue
		}
		usi++
		html := strings.Contains(riga, "innerHTML") || strings.Contains(riga, "insertAdjacentHTML") || strings.Contains(riga, "modal(") || strings.Contains(riga, "'<")
		if html && !strings.Contains(riga, "esc(") {
			t.Errorf("riga %d: il nome entra nell'HTML senza esc(): %.160s", i+1, strings.TrimSpace(riga))
		}
	}
	if usi < 8 {
		t.Fatalf("il nome si usa in %d righe: manca qualcosa?", usi)
	}
	for _, f := range []string{"if (e && n) e.textContent = (", `pill.textContent = n ||`} {
		if !strings.Contains(s, f) {
			t.Errorf("non trovo %q", f)
		}
	}
	// f32-ottimizza aveva un secondo posto per il nome (in fondo al biglietto): non deve tornare
	if strings.Contains(s, "drawCardNome") || strings.Contains(s, "S.nomeUtente") {
		t.Error("c'è ancora drawCardNome / S.nomeUtente: il nome comparirebbe due volte")
	}
}
