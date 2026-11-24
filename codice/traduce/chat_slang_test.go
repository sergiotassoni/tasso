package traduce

import "testing"

func TestChatGergo(t *testing.T) {
	if !soloSigle("GG WP!") || !soloSigle("glhf") || soloSigle("gg you are bad") {
		t.Fatal("sigle")
	}
	if g := espandiAbbreviazioni("u r late, pls wait"); g != "you are late, please wait" {
		t.Fatal(g)
	}
	r := uniscriRigheChat([]string{"Marco: we need to go to the", "castle right now", "Anna: ok"})
	if len(r) != 2 || r[0] != "Marco: we need to go to the castle right now" {
		t.Fatal(r)
	}
	if r := uniscriRigheChat([]string{"hello there", "how are you"}); len(r) != 2 {
		t.Fatal("senza nomi non si unisce")
	}
}
