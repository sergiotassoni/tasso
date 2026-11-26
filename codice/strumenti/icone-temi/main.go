// icone-temi: mette dentro Tasso.exe le icone dei temi (finestra, barra delle applicazioni,
// icona vicino all'orologio, collegamenti) e i PNG per il logo dentro Tasso.
//
// Uso (dalla cartella codice/):
//
//	go run ./strumenti/icone-temi -da /home/claude/ecosistema/icone/temi
//
// Per ogni cartella <tema> in -da (id del tema di Tasso: oro, argento, neonrgb, ...) prende
//
//	tasso-gaming.ico   tasso-gaming.svg
//	tasso-ufficio.ico  tasso-ufficio.svg      (facoltative)
//
// e scrive:
//   - assets/temi/<tema>/tasso-<variante>.ico e .svg (l'.svg è il logo dentro Tasso);
//   - assets/temi/elenco.txt: il numero della risorsa di ogni icona (i numeri già dati restano);
//   - rsrc_windows_amd64.syso: le risorse di prima (manifest, versione, icona normale 2) uguali
//     byte per byte, più le icone dei temi (gruppi 201..., immagini 1000...).
//
// Con -controlla non scrive niente: dice solo se il .syso e l'elenco vanno d'accordo con assets/temi.
// Codice COFF/ICO copiato da ecosistema/strumenti/risorse (stesso autore, GPL-3.0).
package main

import (
	"bytes"
	"debug/pe"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tasso/icotema"
)

func main() {
	if err := esegui(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "icone-temi:", err)
		os.Exit(1)
	}
}

func esegui(arg []string, out io.Writer) error {
	fs := flag.NewFlagSet("icone-temi", flag.ContinueOnError)
	da := fs.String("da", "", "cartella con una sottocartella per tema (vuoto = usa solo assets/temi)")
	codice := fs.String("codice", ".", "cartella codice/ di Tasso")
	controlla := fs.Bool("controlla", false, "solo controllo, non scrive")
	if err := fs.Parse(arg); err != nil {
		return err
	}
	assets := filepath.Join(*codice, "assets", "temi")
	sysoP := filepath.Join(*codice, "rsrc_windows_amd64.syso")
	var el icotema.Elenco
	if b, err := os.ReadFile(filepath.Join(assets, "elenco.txt")); err == nil {
		if el, err = icotema.Leggi(b); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if *da != "" && !*controlla {
		if err := copiaDa(*da, assets, &el, out); err != nil {
			return err
		}
	}
	// le icone che ci sono davvero in assets/temi (l'elenco tiene anche i numeri dei temi tolti,
	// così non vengono mai dati a un altro tema)
	icone := map[uint16][]byte{}
	for _, v := range el.Voci {
		b, err := os.ReadFile(filepath.Join(assets, v.Tema, "tasso-"+v.Variante+".ico"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := LeggiICO(b); err != nil {
			return fmt.Errorf("%s %s: %w", v.Tema, v.Variante, err)
		}
		icone[v.ID] = b
	}
	vecchio, err := os.ReadFile(sysoP)
	if err != nil {
		return err
	}
	nuovo, err := ComponiSyso(vecchio, icone)
	if err != nil {
		return err
	}
	if *controlla {
		if !bytes.Equal(vecchio, nuovo) {
			return errors.New("rsrc_windows_amd64.syso non è aggiornato: rilancia senza -controlla")
		}
		fmt.Fprintf(out, "ok: %d icone dei temi dentro il .syso\n", len(icone))
		return nil
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(assets, "elenco.txt"), el.Scrivi(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(sysoP, nuovo, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "scritto %s (%d byte, %d icone dei temi)\n", sysoP, len(nuovo), len(icone))
	return nil
}

// copiaDa copia .ico e .png dei temi da T1 in assets/temi e dà i numeri nell'elenco.
func copiaDa(da, assets string, el *icotema.Elenco, out io.Writer) error {
	voci, err := os.ReadDir(da)
	if err != nil {
		return err
	}
	for _, d := range voci {
		tema := d.Name()
		if !d.IsDir() || !icotema.TemaValido(tema) {
			if d.IsDir() && tema == "tasso" {
				fmt.Fprintln(out, "tasso: è l'icona normale di Tasso (risorsa 2), già dentro")
			} else if d.IsDir() {
				fmt.Fprintf(out, "salto la cartella %q (non è un id di tema valido)\n", tema)
			}
			continue
		}
		for _, v := range []string{icotema.Gaming, icotema.Ufficio} {
			ico, err := os.ReadFile(filepath.Join(da, tema, "tasso-"+v+".ico"))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if _, err := LeggiICO(ico); err != nil {
				return fmt.Errorf("%s/tasso-%s.ico: %w", tema, v, err)
			}
			svg, err := os.ReadFile(filepath.Join(da, tema, "tasso-"+v+".svg"))
			if err != nil || !SVGSicuro(svg) {
				svg = nil
			}
			id, err := el.Aggiungi(tema, v)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(assets, tema), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(assets, tema, "tasso-"+v+".ico"), ico, 0o644); err != nil {
				return err
			}
			if svg != nil {
				if err := os.WriteFile(filepath.Join(assets, tema, "tasso-"+v+".svg"), svg, 0o644); err != nil {
					return err
				}
			}
			fmt.Fprintf(out, "%s %s: risorsa %d%s\n", tema, v, id, map[bool]string{true: "", false: " (senza SVG: dentro Tasso resta il logo di sempre)"}[svg != nil])
		}
	}
	return nil
}

// ComponiSyso: le risorse del .syso di prima senza le vecchie icone dei temi, più quelle nuove.
// Le risorse di Tasso (manifest, versione, icona 2 e le sue immagini) restano identiche.
func ComponiSyso(vecchio []byte, icone map[uint16][]byte) ([]byte, error) {
	ris, err := LeggiRisorse(vecchio)
	if err != nil {
		return nil, err
	}
	var tenute []Risorsa
	haNormale := false
	for _, r := range ris {
		if r.Tipo == RT_GROUP_ICON && r.ID >= icotema.PrimoID || r.Tipo == RT_ICON && r.ID >= icotema.PrimaImmagine {
			continue // vecchie icone dei temi: le rimetto da capo
		}
		if r.Tipo == RT_ICON && r.ID >= icotema.PrimoID {
			return nil, fmt.Errorf("immagine %d in mezzo ai numeri dei temi", r.ID)
		}
		if r.Tipo == RT_GROUP_ICON && r.ID == icotema.IDNormale {
			haNormale = true
		}
		tenute = append(tenute, r)
	}
	if !haNormale {
		return nil, errors.New("il .syso non ha l'icona normale di Tasso (2)")
	}
	ids := make([]int, 0, len(icone))
	for id := range icone {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	prossima := uint32(icotema.PrimaImmagine)
	for _, id := range ids {
		imm, err := LeggiICO(icone[uint16(id)])
		if err != nil {
			return nil, err
		}
		if prossima+uint32(len(imm)) > 0xFFFF {
			return nil, errors.New("troppe immagini")
		}
		for i, im := range imm {
			tenute = append(tenute, Risorsa{RT_ICON, prossima + uint32(i), im.Dati})
		}
		tenute = append(tenute, Risorsa{RT_GROUP_ICON, uint32(id), gruppoIcone(imm, uint16(prossima))})
		prossima += uint32(len(imm))
	}
	return ScriviSyso(tenute, pe.IMAGE_FILE_MACHINE_AMD64)
}

// SVGSicuro: il disegno va dentro la pagina di Tasso, quindi solo forme: niente script, eventi,
// collegamenti o immagini esterne.
func SVGSicuro(b []byte) bool {
	s := strings.ToLower(string(b))
	if !strings.Contains(s, "<svg") || len(b) > 64<<10 {
		return false
	}
	for _, no := range []string{"<script", "javascript:", "<foreignobject", "href", "<image", "<iframe", "<!entity", "<!doctype", "@import", "url(http", "url(data", "url('", "url(\""} {
		if strings.Contains(s, no) {
			return false
		}
	}
	// attributi on...= (onload, onclick, ...)
	for i := 0; i+3 < len(s); i++ {
		if (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '/') && s[i+1] == 'o' && s[i+2] == 'n' {
			j := i + 3
			for j < len(s) && s[j] >= 'a' && s[j] <= 'z' {
				j++
			}
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			if j > i+3 && j < len(s) && s[j] == '=' {
				return false
			}
		}
	}
	return true
}
