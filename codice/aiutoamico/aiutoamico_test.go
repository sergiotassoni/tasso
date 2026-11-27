package aiutoamico

import "testing"

func TestScegli(t *testing.T) {
	casi := []struct {
		nome      string
		pacchetti []string
		modo, url string
	}{
		{"installata (nome completo)", []string{"Microsoft.WindowsStore_22410.1401.5.0_x64__8wekyb3d8bbwe", "MicrosoftCorporationII.QuickAssist_2.0.31.0_x64__8wekyb3d8bbwe"}, ModoApp, "ms-quick-assist:"},
		{"installata (solo nome, maiuscole diverse)", []string{"microsoftcorporationii.quickassist"}, ModoApp, Protocollo},
		{"manca: c'è lo Store", []string{"Microsoft.WindowsStore_22410.1401.5.0_x64__8wekyb3d8bbwe"}, ModoStore, "ms-windows-store://pdp/?ProductId=9P7BP5VNWKX5"},
		{"manca anche lo Store", []string{"Microsoft.Photos_2024.1_x64__8wekyb3d8bbwe"}, ModoWeb, "https://apps.microsoft.com/detail/9P7BP5VNWKX5"},
		{"registro vuoto", nil, ModoWeb, PaginaWeb},
		{"nome simile non basta", []string{"MicrosoftCorporationII.QuickAssistBeta_1.0_x64__x", "Contoso.QuickAssist_1.0_x64__y"}, ModoWeb, PaginaWeb},
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

func TestIndirizzoAmmesso(t *testing.T) {
	for _, u := range []string{"", "https://example.com", "ms-quick-assist:x", "ms-windows-store://pdp/?ProductId=9NBLGGH4NNS1", "cmd.exe"} {
		if IndirizzoAmmesso(u) {
			t.Errorf("%q non doveva essere ammesso", u)
		}
	}
}
