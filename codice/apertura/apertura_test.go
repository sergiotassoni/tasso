package apertura

import (
	"reflect"
	"strings"
	"testing"
)

const (
	exeProva = `C:\Program Files\Tasso\Tasso.exe`
	sidProva = "S-1-5-21-1111111111-2222222222-3333333333-1001"
	segno    = "0123456789abcdef0123456789abcdef"
)

func TestTuttiPermessi(t *testing.T) {
	casi := []struct {
		args []string
		ok   bool
	}{
		{nil, true},
		{[]string{"--controlla-aggiornamenti"}, true},
		{[]string{"--min", "--controlla-aggiornamenti"}, true},
		{[]string{"--cancella-sicura", `C:\Windows`}, false},
		{[]string{"--togli-silenzioso"}, false},
		{[]string{"--auto=5"}, false},
		{[]string{"--profile"}, false},
		{[]string{`C:\Users\me\file.txt`}, false},
		{[]string{"--MIN"}, false},
	}
	for _, c := range casi {
		if got := TuttiPermessi(c.args); got != c.ok {
			t.Errorf("TuttiPermessi(%q) = %v, voglio %v", c.args, got, c.ok)
		}
	}
}

func TestNonceValido(t *testing.T) {
	if !NonceValido(segno) {
		t.Fatal("segno buono rifiutato")
	}
	for _, s := range []string{"", "abc", strings.ToUpper(segno), segno + "0", segno[:31] + "g", "--min"} {
		if NonceValido(s) {
			t.Errorf("NonceValido(%q) = true", s)
		}
	}
}

func TestParametroEAndataRitorno(t *testing.T) {
	p := ParametroAttivita(segno, []string{"--controlla-aggiornamenti", "--cancella-sicura", "--min"})
	if p != segno+" --controlla-aggiornamenti --min" {
		t.Fatalf("parametro: %q", p)
	}
	// Windows mette $(Arg0) dopo il segno fisso: la riga arriva divisa per gli spazi
	args := append([]string{ArgDaAttivita}, strings.Fields(p)...)
	da, n, resto := DaAttivita(args)
	if !da || n != segno || !reflect.DeepEqual(resto, []string{"--controlla-aggiornamenti", "--min"}) {
		t.Fatalf("DaAttivita: %v %q %q", da, n, resto)
	}
}

func TestDaAttivitaButtaIlResto(t *testing.T) {
	// qualcuno avvia l'attività a mano con argomenti pericolosi: passano solo quelli permessi
	args := []string{ArgDaAttivita, "--cancella-sicura", `C:\Windows`, "--togli-silenzioso", "--min", "--min", "$(Arg0)", segno, segno[:31] + "0"}
	da, n, resto := DaAttivita(args)
	if !da {
		t.Fatal("non riconosciuta")
	}
	if n != segno {
		t.Fatalf("segno: %q", n)
	}
	if !reflect.DeepEqual(resto, []string{"--min"}) {
		t.Fatalf("resto: %q", resto)
	}
	// senza parametro (avviata da Utilità di pianificazione): niente segno, niente argomenti
	da, n, resto = DaAttivita([]string{ArgDaAttivita})
	if !da || n != "" || len(resto) != 0 {
		t.Fatalf("senza parametro: %v %q %q", da, n, resto)
	}
	// il segno fisso deve essere il PRIMO argomento
	in := []string{"--min", ArgDaAttivita}
	da, _, resto = DaAttivita(in)
	if da || !reflect.DeepEqual(resto, in) {
		t.Fatalf("segno non primo: %v %q", da, resto)
	}
}

func TestQuotaArg(t *testing.T) {
	casi := map[string]string{
		"--min":            "--min",
		"":                 `""`,
		`C:\Program Files`: `"C:\Program Files"`,
		`a"b`:              `"a\"b"`,
		`C:\cartella x\`:   `"C:\cartella x\\"`,
		`a\\"b`:            `"a\\\\\"b"`,
		`a\b`:              `a\b`,
	}
	for in, want := range casi {
		if got := QuotaArg(in); got != want {
			t.Errorf("QuotaArg(%q) = %q, voglio %q", in, got, want)
		}
	}
	if r := RigaComando([]string{"--x", "a b"}); r != `--x "a b"` {
		t.Errorf("RigaComando: %q", r)
	}
}

func TestXMLAttivita(t *testing.T) {
	x := XMLAttivita(exeProva, sidProva)
	for _, deve := range []string{
		`<Command>"C:\Program Files\Tasso\Tasso.exe"</Command>`,
		`<Arguments>--da-attivita $(Arg0)</Arguments>`,
		`<WorkingDirectory>C:\Program Files\Tasso</WorkingDirectory>`,
		`<UserId>` + sidProva + `</UserId>`,
		`<LogonType>InteractiveToken</LogonType>`,
		`<RunLevel>HighestAvailable</RunLevel>`,
		`<MultipleInstancesPolicy>Parallel</MultipleInstancesPolicy>`,
		`<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>`,
		`<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>`,
		`<AllowStartOnDemand>true</AllowStartOnDemand>`,
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`,
		`<Priority>4</Priority>`,
	} {
		if !strings.Contains(x, deve) {
			t.Errorf("manca %s", deve)
		}
	}
	if strings.Contains(x, "<Triggers") {
		t.Error("l'attività non deve avere orari")
	}
	a, err := LeggiAttivita(x)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Aggiornata(exeProva, sidProva) {
		t.Fatalf("la mia attività non risulta aggiornata: %+v", a)
	}
	if !a.Aggiornata(strings.ToLower(exeProva), strings.ToLower(sidProva)) {
		t.Error("maiuscole e minuscole non contano nei percorsi di Windows")
	}
	if a.Usabile(`C:\Users\me\Downloads\Tasso.exe`, sidProva) || a.Usabile(exeProva, "S-1-5-21-9") {
		t.Error("usabile per un altro exe o un altro utente")
	}
}

func TestXMLAttivitaCaratteriStrani(t *testing.T) {
	exe := `D:\Programmi & Giochi\Tasso <1>\Tasso.exe`
	x := XMLAttivita(exe, sidProva)
	if !strings.Contains(x, `D:\Programmi &amp; Giochi\Tasso &lt;1&gt;\Tasso.exe`) {
		t.Fatal("percorso non protetto nell'XML")
	}
	a, err := LeggiAttivita(x)
	if err != nil {
		t.Fatal(err)
	}
	if a.Comando != exe || !a.Aggiornata(exe, sidProva) {
		t.Fatalf("letta male: %+v", a)
	}
	if !strings.Contains(XMLAttivita(`C:\Tasso.exe`, sidProva), `<WorkingDirectory>C:\</WorkingDirectory>`) {
		t.Error("cartella della radice del disco")
	}
}

// Come la restituisce Windows: senza virgolette nel comando, con altri elementi in mezzo.
func TestLeggiAttivitaDiWindows(t *testing.T) {
	x := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Author>Tasso</Author><Version>tasso-apri-1</Version><URI>\Tasso\Apri Tasso</URI></RegistrationInfo>
  <Triggers />
  <Principals><Principal id="Author"><UserId>` + sidProva + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><IdleSettings><StopOnIdleEnd>true</StopOnIdleEnd></IdleSettings></Settings>
  <Actions Context="Author"><Exec><Command>C:\Program Files\Tasso\Tasso.exe</Command><Arguments>--da-attivita $(Arg0)</Arguments></Exec></Actions>
</Task>`
	a, err := LeggiAttivita(x)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Usabile(exeProva, sidProva) {
		t.Fatalf("non usabile: %+v", a)
	}
	if a.Aggiornata(exeProva, sidProva) {
		t.Fatal("versione vecchia e IgnoreNew: va rifatta")
	}
	// altre attività (per esempio Avvio con Windows) non sono quella giusta
	altra := strings.Replace(x, "--da-attivita $(Arg0)", "--min", 1)
	if a, err := LeggiAttivita(altra); err != nil || a.Usabile(exeProva, sidProva) {
		t.Fatalf("attività senza il segno usabile: %+v %v", a, err)
	}
	due := strings.Replace(x, "</Exec>", "</Exec><Exec><Command>x.exe</Command></Exec>", 1)
	if _, err := LeggiAttivita(due); err == nil {
		t.Fatal("due azioni accettate")
	}
	if _, err := LeggiAttivita("non è xml"); err == nil {
		t.Fatal("testo a caso accettato")
	}
}

func TestAttivitaDisattivata(t *testing.T) {
	x := strings.Replace(XMLAttivita(exeProva, sidProva), "<Enabled>true</Enabled>", "<Enabled>false</Enabled>", 1)
	a, err := LeggiAttivita(x)
	if err != nil {
		t.Fatal(err)
	}
	if a.Attiva || a.Aggiornata(exeProva, sidProva) {
		t.Fatal("disattivata: va rifatta")
	}
	// senza <Enabled> è attiva (valore di base di Windows)
	if a, _ := LeggiAttivita(strings.Replace(XMLAttivita(exeProva, sidProva), "<Enabled>true</Enabled>", "", 1)); !a.Attiva {
		t.Fatal("senza Enabled deve essere attiva")
	}
}

func TestNomeESDDL(t *testing.T) {
	if n := NomeAttivitaPer(sidProva); n != "Apri Tasso "+sidProva {
		t.Fatalf("nome: %q", n)
	}
	if s := SDDLAttivita(sidProva); s != "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FRFX;;;"+sidProva+")" {
		t.Fatalf("sddl: %q", s)
	}
	for _, s := range []string{sidProva, "S-1-5-18"} {
		if !SidValido(s) {
			t.Errorf("SidValido(%q) = false", s)
		}
	}
	for _, s := range []string{"", "S-1-", "S-1-5-21-1)(A;;FA;;;WD", "s-1-5-18", "S-1-5--1", "S-1-5-", `PC\utente`} {
		if SidValido(s) {
			t.Errorf("SidValido(%q) = true", s)
		}
	}
}

func TestSilenziosi(t *testing.T) {
	for _, a := range [][]string{{"--scheduled"}, {"--auto=3", "--min"}, {"--profile"}} {
		if !Silenziosi(a) {
			t.Errorf("Silenziosi(%q) = false", a)
		}
	}
	for _, a := range [][]string{nil, {"--min"}, {"--controlla-aggiornamenti"}, {"--after-update"}} {
		if Silenziosi(a) {
			t.Errorf("Silenziosi(%q) = true", a)
		}
	}
}

func TestDopoUAC(t *testing.T) {
	dopo, resto := TogliDopoUAC([]string{"--min", ArgDopoUAC, "--controlla-aggiornamenti"})
	if !dopo || !reflect.DeepEqual(resto, []string{"--min", "--controlla-aggiornamenti"}) {
		t.Fatalf("TogliDopoUAC: %v %q", dopo, resto)
	}
	if dopo, resto := TogliDopoUAC([]string{"--min"}); dopo || !reflect.DeepEqual(resto, []string{"--min"}) {
		t.Fatalf("senza segno: %v %q", dopo, resto)
	}
	// il segno c'è una volta sola anche se c'era già
	if got := ArgomentiRunas([]string{ArgDopoUAC, "--auto=5"}); !reflect.DeepEqual(got, []string{"--auto=5", ArgDopoUAC}) {
		t.Fatalf("ArgomentiRunas: %q", got)
	}
	if got := ArgomentiRunas(nil); !reflect.DeepEqual(got, []string{ArgDopoUAC}) {
		t.Fatalf("ArgomentiRunas(nil): %q", got)
	}
	// non è permesso: con il segno niente attività, e dall'attività non passa
	if TuttiPermessi([]string{ArgDopoUAC}) || permessi[ArgDopoUAC] {
		t.Fatal("--dopo-uac non deve essere tra gli argomenti permessi")
	}
	if _, _, resto := DaAttivita([]string{ArgDaAttivita, segno, ArgDopoUAC, "--min"}); !reflect.DeepEqual(resto, []string{"--min"}) {
		t.Fatalf("DaAttivita deve buttare --dopo-uac: %q", resto)
	}
	if strings.Contains(ParametroAttivita(segno, []string{ArgDopoUAC}), ArgDopoUAC) {
		t.Fatal("--dopo-uac nel parametro dell'attività")
	}
}

func TestAvviabile(t *testing.T) {
	a, err := LeggiAttivita(XMLAttivita(exeProva, sidProva))
	if err != nil || !a.Avviabile(exeProva, sidProva) {
		t.Fatalf("attività nuova non avviabile: %v %+v", err, a)
	}
	b := a
	b.Attiva = false
	if b.Avviabile(exeProva, sidProva) {
		t.Fatal("attività disattivata avviabile")
	}
	c := a
	c.Livello = "LeastPrivilege"
	if c.Avviabile(exeProva, sidProva) || c.Aggiornata(exeProva, sidProva) {
		t.Fatal("attività senza permessi alti avviabile o aggiornata")
	}
	if a.Avviabile(`C:\Users\me\Tasso.exe`, sidProva) {
		t.Fatal("altro exe avviabile")
	}
}

func TestSDDLCartella(t *testing.T) {
	if !strings.HasPrefix(SDDLCartella, "D:P(") || !strings.Contains(SDDLCartella, "(A;;FR;;;AU)") ||
		strings.Contains(SDDLCartella, "FA;;;AU") || strings.Contains(SDDLCartella, ";;;WD)") {
		t.Fatalf("SDDLCartella: %s", SDDLCartella)
	}
}
