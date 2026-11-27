package aiutoamico

import "testing"

const qa = "MicrosoftCorporationII.QuickAssist_2.0.31.0_x64__8wekyb3d8bbwe"
const store = "Microsoft.WindowsStore_22410.1401.5.0_x64__8wekyb3d8bbwe"

func TestScegli(t *testing.T) {
	casi := []struct {
		nome      string
		pacchetti []Installato
		modo, url string
	}{
		{"installata, manifest letto", []Installato{{NomeCompleto: store}, {NomeCompleto: qa, AppID: "App", Protocolli: []string{"ms-quick-assist"}}}, ModoApp, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!App`},
		{"installata, Id diverso nel manifest", []Installato{{NomeCompleto: qa, AppID: "QuickAssist"}}, ModoApp, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!QuickAssist`},
		{"installata, manifest non letto", []Installato{{NomeCompleto: qa}}, ModoApp, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!App`},
		{"nome strano, c'è il protocollo", []Installato{{NomeCompleto: "microsoftcorporationii.quickassist", Protocolli: []string{"ms-quick-assist"}}}, ModoApp, Protocollo},
		{"nome strano, niente protocollo", []Installato{{NomeCompleto: "MicrosoftCorporationII.QuickAssist"}}, ModoApp, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!App`},
		{"manca: c'è lo Store", []Installato{{NomeCompleto: store}}, ModoStore, "ms-windows-store://pdp/?ProductId=9P7BP5VNWKX5"},
		{"manca anche lo Store", []Installato{{NomeCompleto: "Microsoft.Photos_2024.1_x64__8wekyb3d8bbwe"}}, ModoWeb, "https://apps.microsoft.com/detail/9P7BP5VNWKX5"},
		{"registro vuoto", nil, ModoWeb, PaginaWeb},
		{"nome simile non basta", []Installato{{NomeCompleto: "MicrosoftCorporationII.QuickAssistBeta_1.0_x64__x"}, {NomeCompleto: "Contoso.QuickAssist_1.0_x64__y"}}, ModoWeb, PaginaWeb},
		{"Id app pericoloso", []Installato{{NomeCompleto: qa, AppID: `App" & calc`}}, ModoApp, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!App`},
	}
	for _, c := range casi {
		s := Scegli(c.pacchetti)
		if s.Modo != c.modo || s.Indirizzo != c.url {
			t.Errorf("%s: %+v, volevo %s %s", c.nome, s, c.modo, c.url)
		}
		if !IndirizzoAmmesso(s.Indirizzo) {
			t.Errorf("%s: %s non ammesso", c.nome, s.Indirizzo)
		}
	}
}

func TestFamiglia(t *testing.T) {
	if f := Famiglia(qa); f != "MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe" {
		t.Errorf("famiglia %q", f)
	}
	for _, x := range []string{"", "Nome", "a_b_c", "N_1_x64__p_q", "N!x_1_x64__p"} {
		if f := Famiglia(x); f != "" {
			t.Errorf("%q: famiglia %q, volevo vuota", x, f)
		}
	}
}

func TestLeggiManifest(t *testing.T) {
	m := `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10" xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10">
 <Applications>
  <Application Id="App" Executable="QuickAssist.exe">
   <Extensions>
    <uap:Extension Category="windows.protocol"><uap:Protocol Name="MS-Quick-Assist"/></uap:Extension>
   </Extensions>
  </Application>
 </Applications>
</Package>`
	id, pr := LeggiManifest([]byte(m))
	if id != "App" || len(pr) != 1 || pr[0] != "ms-quick-assist" {
		t.Errorf("id %q protocolli %v", id, pr)
	}
	if id, pr := LeggiManifest([]byte("non xml")); id != "" || pr != nil {
		t.Errorf("manifest rotto: %q %v", id, pr)
	}
}

func TestIndirizzoAmmesso(t *testing.T) {
	for _, u := range []string{"", "https://example.com", "ms-quick-assist:x", "ms-windows-store://pdp/?ProductId=9NBLGGH4NNS1", "cmd.exe",
		`shell:AppsFolder\Contoso.App_abc!App`, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!App"x`, `shell:AppsFolder\MicrosoftCorporationII.QuickAssist_8wekyb3d8bbwe!App\..\x`} {
		if IndirizzoAmmesso(u) {
			t.Errorf("%q non doveva essere ammesso", u)
		}
	}
}
