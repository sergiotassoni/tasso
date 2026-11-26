package installa

import (
	"path/filepath"
	"sort"
	"testing"
)

func TestDaChiudere(t *testing.T) {
	base := `C:\Program Files\Tasso`
	wv := `C:\Program Files (x86)\Microsoft\EdgeWebView\Application\131.0\msedgewebview2.exe`
	modelli := `C:\Users\Anna\AppData\Local\TassoTraduce`
	lista := []Processo{
		{PID: 1, Padre: 0, Nome: "explorer.exe", Percorso: `C:\Windows\explorer.exe`, Nato: 1},
		{PID: 100, Padre: 1, Nome: "Tasso.exe", Percorso: base + `\Tasso.exe`, Nato: 10},
		{PID: 101, Padre: 100, Nome: "msedgewebview2.exe", Percorso: wv, Nato: 11},
		{PID: 102, Padre: 101, Nome: "msedgewebview2.exe", Percorso: wv, Nato: 12},
		{PID: 103, Padre: 100, Nome: "llama-server.exe", Percorso: base + `\lavoro\ia\llama-server.exe`, Nato: 13},
		{PID: 104, Padre: 100, Nome: "whisper-server.exe", Percorso: modelli + `\modelli\whisper\whisper-server.exe`, Nato: 14},
		{PID: 105, Padre: 100, Nome: "PresentMon.exe", Percorso: base + `\strumenti\PresentMon.exe`, Nato: 15},
		{PID: 106, Padre: 100, Nome: "gioco.exe", Percorso: `D:\Giochi\gioco.exe`, Nato: 16}, // un gioco avviato da Tasso: resta
		// Teams con la sua WebView2: non c'entra
		{PID: 199, Padre: 1, Nome: "ms-teams.exe", Percorso: `C:\Teams\ms-teams.exe`, Nato: 5},
		{PID: 200, Padre: 199, Nome: "msedgewebview2.exe", Percorso: wv, Nato: 6},
		// llama-server rimasto da un Tasso che si è chiuso male (padre che non c'è più)
		{PID: 300, Padre: 9999, Nome: "llama-server.exe", Percorso: modelli + `\modelli\llama-server.exe`, Nato: 3},
		{PID: 301, Padre: 1, Nome: "strano.exe", Percorso: modelli + `\strano.exe`, Nato: 3},
		// questo installatore, avviato da Tasso, con la sua WebView2
		{PID: 500, Padre: 100, Nome: "tasso-verifica-1.exe", Percorso: base + `\lavoro\tasso-verifica-1.exe`, Nato: 20},
		{PID: 501, Padre: 500, Nome: "msedgewebview2.exe", Percorso: wv, Nato: 21},
		// Tasso Zip aperto, ma aggiorno solo Tasso
		{PID: 600, Padre: 1, Nome: "TassoZip.exe", Percorso: base + `\Tasso Zip\TassoZip.exe`, Nato: 30},
		// PID 100 riusato: questo "figlio" è nato prima di Tasso, quindi il suo padre era un altro
		{PID: 700, Padre: 100, Nome: "msedgewebview2.exe", Percorso: wv, Nato: 2},
	}
	cr := CriteriChiusura{Radici: []uint32{100}, Io: 500, File: []string{base + `\Tasso.exe`},
		Cartelle: []string{base + `\lavoro`, base + `\strumenti`, base + `\driver`},
		Escluse:  []string{base + `\Tasso Zip`, base + `\Tasso PDF`}, CartelleAiutanti: []string{modelli}}
	got := DaChiudere(lista, cr)
	var pid []int
	for _, p := range got {
		pid = append(pid, int(p.PID))
	}
	sort.Ints(pid)
	want := []int{101, 102, 103, 104, 105, 300}
	if len(pid) != len(want) {
		t.Fatalf("da chiudere %v invece di %v", pid, want)
	}
	for i := range want {
		if pid[i] != want[i] {
			t.Fatalf("da chiudere %v invece di %v", pid, want)
		}
	}
	// senza radici (Tasso non era aperto): solo gli exe di Tasso, le sue cartelle e gli aiutanti
	cr.Radici, cr.Io, cr.Escluse = nil, 999, nil
	cr.File = append(cr.File, base+`\Tasso Zip\TassoZip.exe`)
	got = DaChiudere(lista, cr)
	pid = pid[:0]
	for _, p := range got {
		pid = append(pid, int(p.PID))
	}
	sort.Ints(pid)
	// 100 (Tasso.exe), 103 e 105 (lavoro, strumenti), 104 e 300 (aiutanti), 500 (in lavoro, non sono io), 600 (TassoZip.exe)
	want = []int{100, 103, 104, 105, 300, 500, 600}
	if len(pid) != len(want) {
		t.Fatalf("senza radici: %v invece di %v", pid, want)
	}
	for i := range want {
		if pid[i] != want[i] {
			t.Fatalf("senza radici: %v invece di %v", pid, want)
		}
	}
}

// Cartella condivisa (installazioni vecchie in C:\Program Files o D:\Giochi): si chiudono solo
// gli exe di Tasso e le sue cartelle, mai gli altri programmi partiti da lì.
func TestDaChiudereCartellaCondivisa(t *testing.T) {
	for _, base := range []string{`C:\Program Files`, `D:\Giochi`} {
		st := &Stato{Programmi: map[Programma]StatoProg{
			ProgTasso: {File: []FileElenco{{Nome: "Tasso.exe"}, {Nome: "VERSIONE"}}},
			ProgSetup: {File: []FileElenco{{Nome: "Disinstalla Tasso.exe"}}},
		}}
		cr := CriteriPer(base, []Componente{Gaming}, st, nil, nil, 999, `C:\Users\A\AppData\Local\TassoTraduce`)
		lista := []Processo{
			{PID: 10, Nome: "WINWORD.EXE", Percorso: base + `\Microsoft Office\root\Office16\WINWORD.EXE`},
			{PID: 11, Nome: "game.exe", Percorso: base + `\game.exe`},
			{PID: 12, Nome: "Tasso.exe", Percorso: base + `\Tasso.exe`},
			{PID: 13, Nome: "llama-server.exe", Percorso: base + `\lavoro\ia\llama-server.exe`},
			{PID: 14, Nome: "Disinstalla Tasso.exe", Percorso: base + `\Disinstalla Tasso.exe`},
			{PID: 15, Nome: "TassoZip.exe", Percorso: base + `\Tasso Zip\TassoZip.exe`}, // Zip non si aggiorna
			{PID: 16, Nome: "tassotraduce.exe", Percorso: `C:\Users\A\AppData\Local\TassoTraduce\tassotraduce.exe`},
		}
		var pid []int
		for _, p := range DaChiudere(lista, cr) {
			pid = append(pid, int(p.PID))
		}
		sort.Ints(pid)
		if len(pid) != 3 || pid[0] != 12 || pid[1] != 13 || pid[2] != 14 {
			t.Fatalf("%s: da chiudere %v, voglio solo [12 13 14]", base, pid)
		}
	}
}

func TestCriteriPerElencoNuovo(t *testing.T) {
	e := &Elenco{Programmi: []ProgElenco{{ID: ProgZip, File: []FileElenco{{Nome: "TassoZip.exe"}}}}}
	cr := CriteriPer(`C:\T`, []Componente{Zip}, nil, e, nil, 1, "aiuto")
	if len(cr.File) != 1 || cr.File[0] != filepathJoinTest(`C:\T`, "Tasso Zip", "TassoZip.exe") {
		t.Fatalf("file: %v", cr.File)
	}
	if len(cr.Cartelle) != 0 || len(cr.CartelleAiutanti) != 0 {
		t.Fatalf("solo Zip: niente cartelle di Tasso né aiutanti (%+v)", cr)
	}
}

func TestDentro(t *testing.T) {
	if !Dentro(`C:\Program Files\Tasso\a.exe`, `c:\program files\tasso\`) || Dentro(`C:\Program Files\Tasso2\a.exe`, `C:\Program Files\Tasso`) ||
		Dentro("", `C:\x`) || !Dentro(`C:/X/y.exe`, `c:\x`) {
		t.Fatal("Dentro")
	}
}

func filepathJoinTest(parti ...string) string { return filepath.Join(parti...) }
