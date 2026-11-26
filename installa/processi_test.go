package installa

import (
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
	got := DaChiudere(lista, CriteriChiusura{Radici: []uint32{100}, Io: 500, Cartelle: []string{base},
		Escluse: []string{base + `\Tasso Zip`, base + `\Tasso PDF`}, CartelleAiutanti: []string{modelli}})
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
	// senza radici (Tasso non era aperto): solo quello che parte dalle cartelle
	got = DaChiudere(lista, CriteriChiusura{Io: 999, Cartelle: []string{base}, CartelleAiutanti: []string{modelli}})
	pid = pid[:0]
	for _, p := range got {
		pid = append(pid, int(p.PID))
	}
	sort.Ints(pid)
	// 100 (Tasso stesso, dentro la cartella), i suoi aiutanti, 500 (non sono io), 600 (Zip non escluso qui)
	if len(pid) != 7 || pid[0] != 100 || pid[len(pid)-1] != 600 {
		t.Fatalf("senza radici: %v", pid)
	}
}

func TestDentro(t *testing.T) {
	if !Dentro(`C:\Program Files\Tasso\a.exe`, `c:\program files\tasso\`) || Dentro(`C:\Program Files\Tasso2\a.exe`, `C:\Program Files\Tasso`) ||
		Dentro("", `C:\x`) || !Dentro(`C:/X/y.exe`, `c:\x`) {
		t.Fatal("Dentro")
	}
}
