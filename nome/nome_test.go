package nome

import (
	"strings"
	"testing"
)

func TestPulisci(t *testing.T) {
	casi := map[string]string{
		"Sergio":              "Sergio",
		"  Sergio  ":          "Sergio",
		"Gian   Luca":         "Gian Luca",
		"\tAnna\u00a0Maria\n": "Anna Maria",
		"\ufeffLuca":          "Luca",
		"Zoë\u2003Kraus":      "Zoë Kraus",
		"   ":                 "",
		"":                    "",
	}
	for in, want := range casi {
		if got := Pulisci(in); got != want {
			t.Errorf("Pulisci(%q) = %q, voglio %q", in, got, want)
		}
	}
}

func TestControlla(t *testing.T) {
	buoni := []string{"Sergio", "Jo", "Gian Luca", "Niccolò", "José-María", "Zoë_92", "Ž. Šimić", "Łukasz",
		"Ἀθηνᾶ", "Анна", "李小龍", "R2-D2", "a.b", "e\u0301va", "e\u0301\u0302\u0303", strings.Repeat("a", 20),
		"ÀÈÌÒÙàèìòù ÇçÑñ ß", "שרה", "محمد", "र\u093eज", "สมชาย",
		"გიო", "Nguyễn", "さくら", "サクラ", "민수", "Արա",
		// 3.2: anche le altre scritture delle lingue vive
		"தமிழ்செல்வி", "সৌমিত্র", "ਗੁਰਪ੍ਰੀਤ", "ಕನ್ನಡ", "മലയാളം", "ලංකා", "ኃይሌ", "አበበ", "佐々木", "々子", "ကြွေး",
		"ᱥᱟᱱᱛᱟᱲᱤ", "\U0001e900\U0001e923\U0001e924", "ລາວ", "ཀྲ", "ខ្មែរ", "ᏣᎳᎩ", "ꦗꦮ", "\U00020bb7野", "Kɔfi", "ⵜⴰⵎⴰⵣⵉⵖⵜ", "ꆈꌠ"}
	for _, s := range buoni {
		if e := Controlla(s); e.Codice != OK {
			t.Errorf("Controlla(%q) = %+v, doveva andare bene", s, e)
		}
	}
	cattivi := []struct {
		s, codice, simbolo string
	}{
		{"", Corto, ""},
		{"S", Corto, ""},
		{"é", Corto, ""},
		{strings.Repeat("a", 21), Lungo, ""},
		{strings.Repeat("è", 21), Lungo, ""},
		{"Sergio★", Simbolo, "★"},
		{"<b>Ciao</b>", Simbolo, "<"},
		{"Ser\"gio", Simbolo, "\""},
		{"Ser'gio", Simbolo, "'"},
		{"a&b", Simbolo, "&"},
		{"Sergio\U0001f600", Simbolo, "\U0001f600"},
		{"Ser/gio", Simbolo, "/"},
		{"××", Simbolo, "×"},
		{"ＷＷ", Simbolo, "Ｗ"},
		{"\U0001d412\U0001d41e", Simbolo, "\U0001d412"},
		{"Ser\u200bgio", Invisibile, ""},
		{"Ser\u202egio", Invisibile, ""},
		{"Ser\x00gio", Invisibile, ""},
		{"ㅤㅤ", Invisibile, ""},
		{"ᅟᅠ", Invisibile, ""},
		{"ﾠﾠ", Invisibile, ""},
		{"Sergio\ufe0f", Invisibile, ""},
		{"Se\u034frgio", Invisibile, ""},
		{"ab\u180b", Invisibile, ""},
		{"a\U000e0100b", Invisibile, ""},
		{"a\u17b4b", Invisibile, ""},
		{"ক্\u200dষ", Invisibile, ""},
		{"ܐ\u070fܒ", Invisibile, ""},
		{"a\u08e2b", Invisibile, ""},
		{"a\u0c5cb", Simbolo, "\u0c5c"},
		{"a\U0002b73ab", Simbolo, "\U0002b73a"},
		{"a〆", Simbolo, "〆"},
		{"a\u0bcd\u0bcd\u0bcd\u0bcd", Accenti, ""},
		{"\u0bbeab", Accenti, ""},
		{"\u0301ab", Accenti, ""},
		{"ab \u0301", Accenti, ""},
		{"a.\u0301b", Accenti, ""},
		{"a\u0301\u0302\u0303\u0304", Accenti, ""},
		{"a" + strings.Repeat("\u0301", 19), Accenti, ""},
		{"..", Senza, ""},
		{"-_ .", Senza, ""},
		{"ab\xff", Invisibile, ""},
	}
	for _, c := range cattivi {
		e := Controlla(c.s)
		if e.Codice != c.codice || e.Simbolo != c.simbolo {
			t.Errorf("Controlla(%q) = %+v, voglio {%s %q}", c.s, e, c.codice, c.simbolo)
		}
		if Valido(c.s) {
			t.Errorf("Valido(%q) = true", c.s)
		}
	}
}

func TestValidoSoloPulito(t *testing.T) {
	if Valido(" Sergio") || Valido("Gian  Luca") {
		t.Fatal("un nome non pulito non è valido così com'è")
	}
	n, e := DaSalvare("  Gian   Luca ")
	if n != "Gian Luca" || e.Codice != OK {
		t.Fatalf("DaSalvare = %q %+v", n, e)
	}
	if Letto("Sergio") != "Sergio" || Letto("<script>") != "" || Letto(" Sergio") != "" || Letto("ㅤㅤ") != "" {
		t.Fatal("Letto sbagliato")
	}
}
