//go:build provaui && !windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// prova_ui.go: SOLO PER LE PROVE (go run -tags provaui .). Serve l'interfaccia su
// http://127.0.0.1:PORTA in un browser normale, con le stesse funzioni del programma vero
// raggiungibili via /rpc/<nome>. Le finestre di Windows sono simulate:
//   TASSOPDF_SCEGLI = file da "scegliere" (separati da ;), TASSOPDF_SALVA = cartella dove salvare.
// Non fa parte di TassoPDF.exe.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func main() {
	porta := os.Getenv("TASSOPDF_PORTA")
	if porta == "" {
		porta = "8765"
	}
	dati := filepath.Join(os.TempDir(), "tassopdf-prova-dati")
	piattaforma.cartellaDati = func() string { return dati }
	piattaforma.linguaSistema = func() string { return os.Getenv("TASSOPDF_LINGUA") }
	piattaforma.scegliFile = func(multi, soloPDF bool, titolo string) ([]string, error) {
		s := os.Getenv("TASSOPDF_SCEGLI")
		if s == "" {
			return nil, nil
		}
		pp := strings.Split(s, ";")
		if !multi {
			pp = pp[:1]
		}
		return pp, nil
	}
	piattaforma.salvaFile = func(nome, cartella, titolo string) (string, error) {
		d := os.Getenv("TASSOPDF_SALVA")
		if d == "" {
			d = filepath.Join(os.TempDir(), "tassopdf-salvati")
		}
		_ = os.MkdirAll(d, 0o755)
		log.Printf("salva: nome=%q cartella=%q titolo=%q", nome, cartella, titolo)
		return filepath.Join(d, nome), nil
	}
	piattaforma.apriLink = func(u string) error { log.Printf("APRI LINK: %s", u); return nil }
	piattaforma.proteggi = func(b []byte) ([]byte, error) { return append([]byte("PROVA"), b...), nil }
	piattaforma.svela = func(b []byte) ([]byte, error) {
		if len(b) < 5 {
			return nil, errors.New("corto")
		}
		return b[5:], nil
	}

	funzioni := legami()
	var nomi []string
	for n := range funzioni {
		nomi = append(nomi, n)
	}
	rpcJS := "window._rpcProva=function(n){return function(){var a=[].slice.call(arguments);return fetch('/rpc/'+n,{method:'POST',body:JSON.stringify(a)}).then(function(r){return r.json()}).then(function(j){if(j.errore!==undefined)throw new Error(j.errore);return j.valore})}};" +
		"[" + `"` + strings.Join(nomi, `","`) + `"` + "].forEach(function(n){window[n]=window._rpcProva(n)});"

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rpc/") {
			nome := strings.TrimPrefix(r.URL.Path, "/rpc/")
			f, ok := funzioni[nome]
			if !ok {
				http.Error(w, "?", 404)
				return
			}
			var args []json.RawMessage
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &args)
			val, err := chiamaProva(f, args)
			out := map[string]interface{}{"valore": val}
			if err != nil {
				out = map[string]interface{}{"errore": err.Error()}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		if r.URL.Path == "/prova-rpc.js" {
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(w, rpcJS)
			return
		}
		body, ct, ok := risorsa("https://" + hostInterno + r.URL.RequestURI())
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s := strings.Replace(string(body), `<script type="module" src="app.js">`, `<script src="/prova-rpc.js"></script><script type="module" src="app.js">`, 1)
			body = []byte(s)
		}
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write(body)
	})
	log.Printf("Tasso PDF (prova) su http://127.0.0.1:%s/", porta)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+porta, nil))
}

func chiamaProva(f interface{}, params []json.RawMessage) (interface{}, error) {
	v := reflect.ValueOf(f)
	tipo := v.Type()
	if len(params) != tipo.NumIn() {
		return nil, fmt.Errorf("argomenti: %d invece di %d", len(params), tipo.NumIn())
	}
	var args []reflect.Value
	for i, p := range params {
		a := reflect.New(tipo.In(i))
		if err := json.Unmarshal(p, a.Interface()); err != nil {
			return nil, err
		}
		args = append(args, a.Elem())
	}
	res := v.Call(args)
	errT := reflect.TypeOf((*error)(nil)).Elem()
	switch len(res) {
	case 0:
		return nil, nil
	case 1:
		if res[0].Type().Implements(errT) {
			if !res[0].IsNil() {
				return nil, res[0].Interface().(error)
			}
			return nil, nil
		}
		return res[0].Interface(), nil
	default:
		if !res[1].IsNil() {
			return nil, res[1].Interface().(error)
		}
		return res[0].Interface(), nil
	}
}
