package main

// Blocco al primo avvio (FIX-BLOCCO-AVVIO): changesList (Impostazioni → Torna indietro) chiamava
// trGo con setMu già preso; trGo prende di nuovo setMu (curLang) e i lucchetti di Go non rientrano:
// Tasso restava fermo per sempre (coda dei pulsanti, Ottimizza, X "Non risponde"). Questo test legge
// il codice e cerca, dentro un tratto con setMu preso (o dentro updateSettings), una chiamata a una
// funzione che prende setMu, anche passando per altre funzioni.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestNienteSetMuDentroSetMu(t *testing.T) {
	dir := os.Getenv("TASSO_CODICE") // per provarlo anche fuori da Windows; di base la cartella del pacchetto
	if dir == "" {
		dir = "."
	}
	trovati := rientriLucchetto(t, dir, "setMu", "updateSettings")
	if len(trovati) > 0 {
		t.Fatalf("setMu preso due volte nello stesso filo (Tasso si blocca per sempre):\n%s", strings.Join(trovati, "\n"))
	}
}

func nomeChiamata(c *ast.CallExpr, mu string) string {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok && x.Name == mu {
			return mu + "." + f.Sel.Name
		}
		return f.Sel.Name
	}
	return ""
}

// rientriLucchetto: le chiamate a funzioni che prendono mu fatte mentre mu è già preso.
// Una "go f()" non conta (gira in un altro filo). conChiusura: funzione che chiama la sua
// funzione argomento con mu preso (updateSettings).
func rientriLucchetto(t *testing.T, dir, mu, conChiusura string) []string {
	fs := token.NewFileSet()
	file, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	chiama := map[string]map[string]bool{}
	type corpo struct {
		nome string
		b    *ast.BlockStmt
	}
	var corpi []corpo
	prende := map[string]bool{conChiusura: true}
	for _, f := range file {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fs, f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			n := fd.Name.Name
			corpi = append(corpi, corpo{n, fd.Body})
			if chiama[n] == nil {
				chiama[n] = map[string]bool{}
			}
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				switch v := x.(type) {
				case *ast.GoStmt:
					return false
				case *ast.FuncLit:
					return false // le funzioni dentro (time.AfterFunc, callback) girano di solito altrove
				case *ast.CallExpr:
					cn := nomeChiamata(v, mu)
					if cn == mu+".Lock" {
						prende[n] = true
					}
					if cn != "" {
						chiama[n][cn] = true
					}
					// updateSettings(func…): il corpo gira subito, nello stesso filo
					if cn == conChiusura {
						for _, a := range v.Args {
							if fl, ok := a.(*ast.FuncLit); ok {
								ast.Inspect(fl.Body, func(y ast.Node) bool {
									if c2, ok := y.(*ast.CallExpr); ok {
										if n2 := nomeChiamata(c2, mu); n2 != "" {
											chiama[n][n2] = true
										}
									}
									return true
								})
							}
						}
					}
				}
				return true
			})
		}
	}
	for cambiato := true; cambiato; {
		cambiato = false
		for f, cs := range chiama {
			if prende[f] {
				continue
			}
			for c := range cs {
				if prende[c] {
					prende[f], cambiato = true, true
					break
				}
			}
		}
	}
	var out []string
	segnala := func(nome string, n ast.Node, perche string) {
		ast.Inspect(n, func(x ast.Node) bool {
			if _, ok := x.(*ast.GoStmt); ok {
				return false
			}
			if c, ok := x.(*ast.CallExpr); ok {
				cn := nomeChiamata(c, mu)
				if cn == mu+".Lock" || (prende[cn] && cn != mu+".Unlock") {
					out = append(out, fmt.Sprintf("%s: %s chiama %s (%s)", nome, perche, cn, fs.Position(c.Pos())))
				}
			}
			return true
		})
	}
	var scorri func(nome string, list []ast.Stmt)
	scorri = func(nome string, list []ast.Stmt) {
		preso := false
		for _, s := range list {
			if es, ok := s.(*ast.ExprStmt); ok {
				if c, ok := es.X.(*ast.CallExpr); ok {
					switch nomeChiamata(c, mu) {
					case mu + ".Lock":
						preso = true
						continue
					case mu + ".Unlock":
						preso = false
						continue
					}
				}
			}
			if ds, ok := s.(*ast.DeferStmt); ok && nomeChiamata(ds.Call, mu) == mu+".Unlock" {
				continue
			}
			if preso {
				segnala(nome, s, "con "+mu+" preso")
				continue
			}
			ast.Inspect(s, func(x ast.Node) bool {
				switch v := x.(type) {
				case *ast.FuncLit:
					scorri(nome, v.Body.List)
					return false
				case *ast.BlockStmt:
					scorri(nome, v.List)
					return false
				}
				return true
			})
		}
	}
	for _, c := range corpi {
		scorri(c.nome, c.b.List)
		ast.Inspect(c.b, func(x ast.Node) bool {
			if call, ok := x.(*ast.CallExpr); ok && nomeChiamata(call, mu) == conChiusura {
				for _, a := range call.Args {
					if fl, ok := a.(*ast.FuncLit); ok {
						segnala(c.nome, fl.Body, "dentro "+conChiusura)
					}
				}
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}
