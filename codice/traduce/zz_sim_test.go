//go:build simula

package traduce

import (
	"bufio"
	"context"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

type mozPonte struct{}

func (mozPonte) Nome() string { return "Mozilla" }
func (mozPonte) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	resp, err := http.Post("http://127.0.0.1:8767/", "text/plain", strings.NewReader(r.Testo))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func TestSimulaPagina(t *testing.T) {
	dir := os.Getenv("SIM")
	nome := os.Getenv("NOME")
	f, _ := os.Open(dir + "/" + nome + ".tsv")
	sc := bufio.NewScanner(f)
	righe := map[string]*RigaOCR{}
	var ordine []string
	for sc.Scan() {
		c := strings.Split(sc.Text(), "\t")
		if len(c) < 12 || c[0] != "5" || strings.TrimSpace(c[11]) == "" {
			continue
		}
		k := c[2] + "/" + c[3] + "/" + c[4]
		x, _ := strconv.Atoi(c[6])
		y, _ := strconv.Atoi(c[7])
		w, _ := strconv.Atoi(c[8])
		h, _ := strconv.Atoi(c[9])
		if righe[k] == nil {
			righe[k] = &RigaOCR{}
			ordine = append(ordine, k)
		}
		righe[k].Parole = append(righe[k].Parole, ParolaOCR{c[11], float64(x), float64(y), float64(w), float64(h)})
	}
	var rr []RigaOCR
	for _, k := range ordine {
		rr = append(rr, *righe[k])
	}
	fi, _ := os.Open(dir + "/" + nome + ".png")
	src, _, _ := image.Decode(fi)
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	bl := unisciParagrafi(segmenti(rr, 1))
	spazioLibero(bl, img.Bounds().Dx())
	trad := NuovoTraduttore(dir+"/cache-"+nome+".json", glossarioBase(), mozPonte{})
	type out struct {
		Testo, Pulito, Catalogo, Bloccato string
		Mappa                             map[string]string
		X, Y, W, H, Righe, LargMax        int
		Em                                float64
		Sf, Ink                           [3]uint8
		Fondo                             string
		Tratto                            float64
	}
	var o []out
	var tenuti []Blocco
	for _, b := range bl {
		if !daTradurre(b.Testo) || !formaPlausibile(b) {
			continue
		}
		coloriBlocco(img, &b)
		tenuti = append(tenuti, b)
	}
	armonizza(tenuti)
	for i, b := range tenuti {
		fp := ""
		if fo := fondoRicostruito(img, b); fo != nil {
			fp = dir + "/fondo-" + nome + "-" + strconv.Itoa(i) + ".png"
			ff, _ := os.Create(fp)
			png.Encode(ff, fo)
			ff.Close()
		}
		p := strings.ReplaceAll(pulisci(b.Testo), "\n", " ")
		r := trad.TraduciScritta(context.Background(), b.Testo)
		o = append(o, out{b.Testo, p, r.Tradotto, r.Motore, nil, b.X, b.Y, b.W, b.H, b.Righe, b.LargMax, b.Em, [3]uint8{b.Sfondo.R, b.Sfondo.G, b.Sfondo.B}, [3]uint8{b.Inchiostro.R, b.Inchiostro.G, b.Inchiostro.B}, fp, b.Tratto})
	}
	j, _ := json.MarshalIndent(o, "", " ")
	os.WriteFile(dir+"/blocchi-"+nome+".json", j, 0o644)
}
