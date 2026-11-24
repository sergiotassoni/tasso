package installa

import (
	"fmt"
	"strings"
)

// Pubblicazione per l'aggiornamento da Tasso (Impostazioni → Aggiornamenti). Accanto a Tasso.zip,
// Tasso.sha256 e VERSION, nel progetto su GitHub (ramo main) vanno:
//
//	TassoSetup.exe     questo installatore (firmato)
//	TassoSetup.sha256  il file scritto da Manifesto (strumenti/impronta, lo chiama compila.sh)
//
// TassoSetup.sha256, righe di testo:
//
//	<sha256 di TassoSetup.exe>  TassoSetup.exe  <versione dell'ecosistema>
//	tasso <versione>
//	zip <versione>
//	pdf <versione>
//
// La prima riga ha lo stesso formato di Tasso.sha256 (Tasso la legge allo stesso modo); le altre
// dicono a Tasso quale versione di ogni programma c'è dentro, per mostrarle e per sapere se
// conviene aggiornare.
const (
	NomeSetupPubblicato = "TassoSetup.exe"
	NomeManifesto       = "TassoSetup.sha256"
)

// Manifesto scrive il contenuto di TassoSetup.sha256.
func Manifesto(sha256 string, e *Elenco) (string, error) {
	sha256 = strings.ToLower(strings.TrimSpace(sha256))
	if len(sha256) != 64 || strings.Trim(sha256, "0123456789abcdef") != "" {
		return "", fmt.Errorf("impronta SHA-256 non valida")
	}
	if _, ok := numeriVersione(e.Versione); !ok {
		return "", fmt.Errorf("versione dell'ecosistema non valida: %q", e.Versione)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s\n", sha256, NomeSetupPubblicato, e.Versione)
	for _, id := range []Programma{ProgTasso, ProgZip, ProgPDF} {
		p := e.Prog(id)
		if p == nil {
			continue
		}
		if _, ok := numeriVersione(p.Versione); !ok {
			return "", fmt.Errorf("versione di %s non valida: %q", id, p.Versione)
		}
		fmt.Fprintf(&b, "%s %s\n", id, p.Versione)
	}
	return b.String(), nil
}
