package traduce

import (
	"os"
	"path/filepath"
)

// ControllaPrimaDiTogliere: i controlli di TogliTutteLeTraduzioni che fermano "Togli Tasso dal PC",
// fatti PRIMA che Tasso cambi qualcosa (CTRL-FIN-14 F14-9: prima Tasso rimetteva Ottimizza tutto e
// le impostazioni di Windows e solo dopo si fermava perché un gioco tradotto era aperto).
// Non tocca niente. motivo: "INSTALLA" (una traduzione si sta installando adesso), "ADMIN" (la
// chiave delle firme non si legge senza i permessi da amministratore) o ""; aperti: i giochi
// tradotti che sono aperti adesso.
func ControllaPrimaDiTogliere() (motivo string, aperti []string) {
	if StaInstallando() {
		return "INSTALLA", nil
	}
	if chiaveNonLeggibile() {
		return "ADMIN", nil
	}
	for _, base := range basiCopie() {
		voci, _ := os.ReadDir(filepath.Join(base, "copie-giochi"))
		for _, v := range voci {
			if doppione(base, v.Name()) {
				continue
			}
			if m, ok := leggiManifesto(base, v.Name()); ok {
				if processoAperto(m.Exe) {
					aperti = append(aperti, nomeNellElenco(base, v.Name()))
				}
				continue
			}
			if mv, vecchio := leggiManifestoVecchio(base, v.Name()); vecchio && chiaveDellaCartella(v.Name(), mv.Cartella) && processoAperto(mv.Exe) {
				nome := mv.Gioco
				if nome == "" {
					nome = v.Name()
				}
				aperti = append(aperti, nome)
			}
		}
	}
	return "", aperti
}
