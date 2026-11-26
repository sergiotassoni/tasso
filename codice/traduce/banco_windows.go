//go:build windows

package traduce

// Banco di prova su Windows, con l'OCR vero e il disegno vero (banco_prova.go per il resto):
//   - su una cartella di schermate (png/jpg);
//   - sullo schermo: un fotogramma ogni 2 secondi per un tempo dato (anche un video di gameplay
//     a schermo intero nel browser: qui il filtro "solo giochi" non conta).
// Lo usano Tasso-collaudo (--prova-traduzione=…) e TassoProva.exe (cmd/tassoprova).

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// leggiOCRBanco: la lettura vera della schermata (come preparaPagina), aspettando il turno se la
// lettura è occupata.
func leggiOCRBanco(img *image.RGBA) ([]RigaOCR, float64, error) {
	for i := 0; !occupato.CompareAndSwap(false, true); i++ {
		if i > 300 {
			return nil, 0, fmt.Errorf("lettura del testo occupata")
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer occupato.Store(false)
	if err := assicuraOCR(); err != nil {
		return nil, 0, err
	}
	righe, f, err := leggiPaginaOCR(img)
	if err != nil {
		return nil, 0, err
	}
	return rileggiIncerte(img, righe, f), f, nil
}

// dopoIABanco: se il correttore IA è acceso, aspetta che abbia corretto le frasi nuove (al
// massimo 30 s) e ritraduce: la fonte diventa "libreria (IA)" o "memoria" corretta.
func dopoIABanco(da []Blocco, ris []Risultato) []Risultato {
	if !ia.Attivo() {
		return ris
	}
	fine := time.Now().Add(30 * time.Second)
	for time.Now().Before(fine) {
		if _, _, attesa := traduttore.StatoCorrezioni(); attesa == 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	for i, r := range ris {
		if r.Motore != "Mozilla" {
			continue
		}
		n := traduttore.TraduciScritta(conLotto(context.Background()), da[i].Testo)
		if n.Tradotto != "" && n.Tradotto != r.Tradotto {
			n.Motore = "IA (corretta)"
			ris[i] = n
		}
	}
	return ris
}

// statoMotoriBanco: per il report, quali motori c'erano.
func statoMotoriBanco() string {
	var p []string
	p = append(p, "frasi pronte e librerie: sì")
	if mozOK.Load() {
		p = append(p, "traduttore veloce (Mozilla): sì")
	} else {
		p = append(p, "traduttore veloce (Mozilla): NO (non pronto o non scaricato)")
	}
	if ia.Attivo() {
		p = append(p, "correttore IA: sì")
	} else {
		p = append(p, "correttore IA: no (spento, non installato o senza permessi): solo frasi pronte + veloce")
	}
	return strings.Join(p, " · ")
}

func opzioniBancoVere() opzioniBanco {
	return opzioniBanco{leggi: leggiOCRBanco, tr: traduttore, disegna: salvaAnteprima, dopo: dopoIABanco}
}

// EseguiBancoCartella: il banco sulle schermate di una cartella; report nella cartella stessa.
func EseguiBancoCartella(dir string) error {
	defer guard("banco-cartella")
	esiti, err := bancoCartella(dir, opzioniBancoVere())
	if err != nil {
		return err
	}
	return aggiungiMotoriAlReport(dir, esiti)
}

func aggiungiMotoriAlReport(dir string, esiti []esitoImmagine) error {
	p := filepath.Join(dir, "banco-prova.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return os.WriteFile(p, append([]byte("Motori: "+statoMotoriBanco()+"\r\n\r\n"), b...), 0o644)
}

// zonaSchermoPrincipale: tutto il monitor principale.
func zonaSchermoPrincipale() Zona {
	w, _, _ := pGetSysMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := pGetSysMetrics.Call(1) // SM_CYSCREEN
	return Zona{X: 0, Y: 0, W: int(w), H: int(h)}
}

// riduciLarghezza: l'immagine larga al massimo larg pixel (media dei pixel), per i fotogrammi salvati.
func riduciLarghezza(img *image.RGBA, larg int) *image.RGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= larg {
		return img
	}
	nh := h * larg / w
	out := image.NewRGBA(image.Rect(0, 0, larg, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max(y*h/nh+1, (y+1)*h/nh)
		for x := 0; x < larg; x++ {
			x0, x1 := x*w/larg, max(x*w/larg+1, (x+1)*w/larg)
			var r, g, b, n int
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					i := img.PixOffset(xx, yy)
					r, g, b, n = r+int(img.Pix[i]), g+int(img.Pix[i+1]), b+int(img.Pix[i+2]), n+1
				}
			}
			o := out.PixOffset(x, y)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return out
}

func salvaPNGBanco(img image.Image, p string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// EseguiBancoSchermo: un fotogramma dello schermo ogni ogni (di solito 2 s) per durata (o finché
// ferma() dice basta). Salva fotogrammi (larghi al massimo larg px), tradotti e report.
// avanzamento (può essere nil) riceve il numero di fotogrammi fatti.
func EseguiBancoSchermo(dir string, durata, ogni time.Duration, larg int, ferma func() bool, avanzamento func(int)) error {
	defer guard("banco-schermo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	o := opzioniBancoVere()
	o.voti = &votazione{}   // come nel gioco: stabilità tra un fotogramma e l'altro
	o.stato = &statoBanco{} // e le scritte già mostrate restano ferme
	fine := time.Now().Add(durata)
	var esiti []esitoImmagine
	var instab []instabilita
	n := 0
	var ultimaImp Impronta
	for time.Now().Before(fine) && (ferma == nil || !ferma()) {
		inizio := time.Now()
		if !davantiSchermoIntero() {
			// davanti non c'è il gioco o il video a schermo intero (la scrivania, Esplora file, un
			// altro programma): niente fotogramma, così nel report non finiscono dati personali
			if d := ogni - time.Since(inizio); d > 0 {
				time.Sleep(d)
			}
			continue
		}
		img, err := cattura(zonaSchermoPrincipale())
		if err == nil && !catturaNera(img) {
			imp := calcolaImpronta(img)
			if n == 0 || imp.differenza(ultimaImp) > 0.002 { // schermo fermo: niente fotogramma doppio
				ultimaImp = imp
				n++
				nome := fmt.Sprintf("fotogramma-%04d", n)
				_ = salvaPNGBanco(riduciLarghezza(img, larg), filepath.Join(dir, nome+".png"))
				e, pronti := elaboraImmagine(img, nome, o)
				tmp := filepath.Join(dir, nome+"-tradotto-grande.png")
				if err := salvaAnteprima(img, pronti, tmp); err == nil {
					if t, err := apriImmagineRGBA(tmp); err == nil {
						_ = salvaPNGBanco(riduciLarghezza(t, larg), filepath.Join(dir, nome+"-tradotto.png"))
					}
					_ = os.Remove(tmp)
				}
				if len(esiti) > 0 {
					instab = append(instab, confrontaFotogrammi(esiti[len(esiti)-1], e)...)
				}
				esiti = append(esiti, e)
				if avanzamento != nil {
					avanzamento(n)
				}
				if n%10 == 0 { // report intermedio: se il PC si spegne resta quello che c'è
					_ = scriviReportBanco(dir, esiti, instab)
				}
			}
		}
		if d := ogni - time.Since(inizio); d > 0 {
			time.Sleep(d)
		}
	}
	if len(esiti) == 0 {
		return fmt.Errorf("nessun fotogramma catturato")
	}
	if err := scriviReportBanco(dir, esiti, instab); err != nil {
		return err
	}
	return aggiungiMotoriAlReport(dir, esiti)
}
