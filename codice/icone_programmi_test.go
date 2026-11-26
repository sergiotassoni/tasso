package main

import (
	"reflect"
	"testing"
)

func TestTrovaVoceWinget(t *testing.T) {
	voci := []voceDisinst{
		{chiave: `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Git_is1`, nome: "Git", icona: `C:\Program Files\Git\mingw64\share\git\git-for-windows.ico`},
		{chiave: `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\BleachBit`, nome: "BleachBit 6.0.4.310", icona: `C:\Program Files (x86)\BleachBit\bleachbit.exe`},
		{chiave: `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Uninstall\Bionic`, nome: "Bionic", loc: `C:\Users\a\AppData\Local\Bionic`},
		{chiave: `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{AAAA-1111}`, nome: "Corsair iCUE5 Software", icona: `C:\Program Files\Corsair\iCUE.exe,0`},
		{chiave: `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{BBBB-2222}`, nome: "Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.40.33810"},
		{chiave: `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\7-Zip`, nome: "7-Zip 24.08 (x64)", icona: `C:\Program Files\7-Zip\7zFM.exe`},
	}
	casi := []struct{ nome, id, vuole string }{
		{"Git", "Git.Git", "Git"},
		{"BleachBit", "BleachBit.BleachBit", "BleachBit 6.0.4.310"},
		{"Bionic 1.2.0+2", "Bionic.Bionic", "Bionic"},
		{"Corsair iCUE5 Software", "Corsair.iCUE.5", "Corsair iCUE5 Software"},
		{"Programma strano", `ARP\Machine\X64\{AAAA-1111}`, "Corsair iCUE5 Software"},
		{"Microsoft Visual C++ 2015-2…", "Microsoft.VCRedist.2015+.x64", "Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.40.33810"},
		{"7-Zip", "7zip.7zip", "7-Zip 24.08 (x64)"},
		{"cloudflared", "Cloudflare.cloudflared", ""},
	}
	for _, c := range casi {
		v, ok := trovaVoceWinget(c.nome, c.id, voci)
		if c.vuole == "" {
			if ok {
				t.Errorf("%q: non doveva trovare niente, trovato %q", c.nome, v.nome)
			}
			continue
		}
		if !ok || v.nome != c.vuole {
			t.Errorf("%q (%s): voluto %q, trovato %q (%v)", c.nome, c.id, c.vuole, v.nome, ok)
		}
	}
}

func TestTrovaVoceWingetPreferisceConIcona(t *testing.T) {
	voci := []voceDisinst{{chiave: `a\X`, nome: "Steam"}, {chiave: `a\Y`, nome: "Steam", icona: `C:\Steam\steam.exe`}}
	if v, ok := trovaVoceWinget("Steam", "Valve.Steam", voci); !ok || v.icona == "" {
		t.Fatalf("doveva scegliere la voce con l'icona: %+v", v)
	}
}

func TestPercorsoDiscoLocale(t *testing.T) {
	si := []string{`C:\Program Files\Git\git.exe`, `d:\Giochi\x.ico`, `C:/Programmi/a.exe`}
	no := []string{``, `\\server\share\a.exe`, `\\?\C:\a.exe`, `C:a.exe`, `C:\a.exe:flusso`, `C:\x\..\Windows\a.exe`, `1:\a.exe`, `relativo\a.exe`}
	for _, p := range si {
		if !percorsoDiscoLocale(p) {
			t.Errorf("%q doveva andare bene", p)
		}
	}
	for _, p := range no {
		if percorsoDiscoLocale(p) {
			t.Errorf("%q non doveva andare bene", p)
		}
	}
}

func TestDividiIconaReg(t *testing.T) {
	casi := []struct {
		in, p string
		i     int
		ok    bool
	}{
		{`"C:\Program Files\A\a.exe",-101`, `C:\Program Files\A\a.exe`, -101, true},
		{`C:\Program Files\A\a.exe,2`, `C:\Program Files\A\a.exe`, 2, true},
		{`"C:\x\a.ico"`, `C:\x\a.ico`, 0, true},
		{`C:\Cartella, con virgola\a.exe`, `C:\Cartella, con virgola\a.exe`, 0, true},
		{``, ``, 0, false},
	}
	for _, c := range casi {
		p, i, ok := dividiIconaReg(c.in)
		if p != c.p || i != c.i || ok != c.ok {
			t.Errorf("%q -> %q %d %v", c.in, p, i, ok)
		}
	}
}

func TestPacchettiPerID(t *testing.T) {
	tutti := []pacchettoAppx{}
	for _, full := range []string{
		"Microsoft.BingWeather_4.53.1.0_x64__8wekyb3d8bbwe",
		"Microsoft.BingWeather_4.54.2.0_x64__8wekyb3d8bbwe",
		"Microsoft.BingWeather_4.54.2.0_neutral_split.scale-125_8wekyb3d8bbwe",
		"king.com.CandyCrushSaga_1.2.3.0_x86__kgqvnymyfvs32",
		"Microsoft.BingNews_1.0.0.0_x64__8wekyb3d8bbwe",
		"rotto",
	} {
		n, v, ris, ok := dividiNomeCompleto(full)
		if !ok || ris {
			continue
		}
		tutti = append(tutti, pacchettoAppx{nomeCompleto: full, nome: n, versione: v})
	}
	got := pacchettiPerID("Microsoft.BingWeather", tutti)
	if len(got) != 2 || got[0].nomeCompleto != "Microsoft.BingWeather_4.54.2.0_x64__8wekyb3d8bbwe" {
		t.Fatalf("BingWeather: %+v", got)
	}
	if got := pacchettiPerID("*CandyCrush*", tutti); len(got) != 1 {
		t.Fatalf("CandyCrush: %+v", got)
	}
	if got := pacchettiPerID("*TikTok*", tutti); len(got) != 0 {
		t.Fatalf("TikTok: %+v", got)
	}
}

func TestLogoDaManifest(t *testing.T) {
	xml := `<Package><Properties><Logo>Assets\StoreLogo.png</Logo></Properties>
<uap:VisualElements DisplayName="Meteo" Square150x150Logo="Assets/Square150x150Logo.png" Square44x44Logo="Assets\Square44x44Logo.png" />
<uap:VisualElements Square44x44Logo="..\..\Windows\evil.png" Square150x150Logo="C:\evil.png"/>
<uap:VisualElements Square44x44Logo="Assets\Square44x44Logo.png"/></Package>`
	want := []string{`Assets\Square44x44Logo.png`, `Assets\Square150x150Logo.png`, `Assets\StoreLogo.png`}
	if got := logoDaManifest(xml); !reflect.DeepEqual(got, want) {
		t.Fatalf("voluto %v, trovato %v", want, got)
	}
}

func TestScegliLogo(t *testing.T) {
	nomi := []string{
		"Square44x44Logo.scale-200.png",
		"Square44x44Logo.targetsize-32.png",
		"Square44x44Logo.targetsize-32_altform-unplated.png",
		"Square44x44Logo.targetsize-32_altform-lightunplated.png",
		"Square44x44Logo.targetsize-64_altform-unplated.png",
		"Square44x44Logo.scale-100_contrast-black.png",
		"Square44x44LogoAltro.scale-100.png",
		"Square150x150Logo.scale-100.png",
	}
	got := scegliLogo(nomi, "Square44x44Logo", ".png", false)
	if len(got) == 0 || got[0] != "Square44x44Logo.targetsize-32_altform-unplated.png" {
		t.Fatalf("32: %v", got)
	}
	got = scegliLogo(nomi, "Square44x44Logo", ".png", true)
	if len(got) == 0 || got[0] != "Square44x44Logo.targetsize-64_altform-unplated.png" {
		t.Fatalf("64: %v", got)
	}
	for _, n := range scegliLogo(nomi, "Square44x44Logo", ".png", false) {
		if n == "Square44x44LogoAltro.scale-100.png" || n == "Square150x150Logo.scale-100.png" {
			t.Fatalf("file di un altro logo: %s", n)
		}
	}
	// Solo alto contrasto: lo prendo lo stesso (meglio dell'iniziale), ma per ultimo.
	if got := scegliLogo([]string{"L.scale-100_contrast-white.png"}, "L", ".png", false); len(got) != 1 {
		t.Fatalf("contrasto: %v", got)
	}
	if got := scegliLogo([]string{"StoreLogo.png"}, "StoreLogo", ".png", false); len(got) != 1 {
		t.Fatalf("senza qualificatori: %v", got)
	}
}

func TestNomeSenzaVersione(t *testing.T) {
	casi := map[string]string{"Bionic 1.2.0+2": "bionic", "7-Zip 24.08 (x64)": "7zip", "Git": "git", "Notepad++ v8.6.4": "notepad", "Corsair iCUE5 Software": "corsairicue5software"}
	for in, w := range casi {
		if g := nomeSenzaVersione(in); g != w {
			t.Errorf("%q -> %q, voluto %q", in, g, w)
		}
	}
}

func TestChiaveAggiornamento(t *testing.T) {
	if chiaveAggiornamento("Git.Git", "Git") != "u:Git.Git" || chiaveAggiornamento("", "Abc") != "u:n:Abc" {
		t.Fatal("chiavi diverse da quelle dell'interfaccia")
	}
}
