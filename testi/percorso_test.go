package testi

import (
	"os"
	"strings"
	"testing"
)

// Prova sul PC vero di Tasso 3.2: nella schermata "Fatto" il percorso dell'archivio andava a capo a
// metà parola ("Archivio.zi / p"). Adesso va a capo solo dopo le barre (word-break:break-all tolto) e
// un pezzo troppo lungo si accorcia con i puntini.
func TestPercorsoACapoSulleBarre(t *testing.T) {
	b, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("index.html non trovato")
	}
	p := string(b)
	i := strings.Index(p, ".percorso{")
	if i < 0 {
		t.Fatal("manca lo stile .percorso")
	}
	stile := p[i : i+strings.Index(p[i:], "}")]
	if strings.Contains(stile, "break-all") || !strings.Contains(stile, "overflow-wrap:anywhere") {
		t.Errorf(".percorso deve andare a capo sulle barre, non a metà parola: %s", stile)
	}
	if !strings.Contains(p, ".percorso .pz{display:inline-block;max-width:100%;white-space:pre;overflow:hidden;text-overflow:ellipsis;") {
		t.Error("ogni pezzo del percorso resta intero e, se è più lungo della riga, finisce con i puntini")
	}
	if !strings.Contains(p, `percorsoACapo($("f-percorso"), r.percorso);`) || strings.Contains(p, `$("f-percorso").textContent = r.percorso;`) {
		t.Error("il percorso della schermata Fatto deve passare da percorsoACapo")
	}
}
