package traduce

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScarica(t *testing.T) {
	contenuto := bytes.Repeat([]byte("modello finto "), 5000)
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(contenuto)
	w.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".gz") {
			w.Write(gz.Bytes())
			return
		}
		w.Write(contenuto)
	}))
	defer srv.Close()
	vecchio := clientScarica
	clientScarica = srv.Client()
	defer func() { clientScarica = vecchio }()

	h := sha256.Sum256(contenuto)
	giusta := hex.EncodeToString(h[:])
	dir := t.TempDir()
	files := []FileDaScaricare{
		{URL: srv.URL + "/a.gz", Nome: "a.bin", SHA256: giusta, Byte: int64(len(contenuto)), Gzip: true},
		{URL: srv.URL + "/b", Nome: "b.bin", SHA256: giusta, Byte: int64(len(contenuto))},
	}
	var ultimo int64
	if err := scaricaTutti(context.Background(), dir, files, func(f, tot int64) { ultimo = f }); err != nil {
		t.Fatal(err)
	}
	if ultimo != 2*int64(len(contenuto)) || !presente(dir, files[0]) || !presente(dir, files[1]) {
		t.Fatalf("scaricamento incompleto: %d", ultimo)
	}
	// impronta sbagliata: file rifiutato e cancellato
	cattivo := []FileDaScaricare{{URL: srv.URL + "/c", Nome: "c.bin", SHA256: strings.Repeat("0", 64), Byte: int64(len(contenuto))}}
	if err := scaricaTutti(context.Background(), dir, cattivo, func(int64, int64) {}); err == nil {
		t.Fatal("doveva rifiutare il file")
	}
	if _, err := os.Stat(filepath.Join(dir, "c.bin")); err == nil {
		t.Fatal("file sbagliato non cancellato")
	}
	// senza impronta: non si scarica
	if err := scaricaTutti(context.Background(), dir, []FileDaScaricare{{URL: srv.URL + "/d", Nome: "d.bin"}}, func(int64, int64) {}); err == nil {
		t.Fatal("doveva rifiutare un file senza impronta")
	}
	// nome pericoloso
	if err := scaricaUno(context.Background(), dir, FileDaScaricare{URL: srv.URL, Nome: `..\x`, SHA256: giusta}, func(int64) {}); err == nil {
		t.Fatal("doveva rifiutare il nome")
	}
}
