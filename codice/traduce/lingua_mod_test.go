package traduce

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRilevaLingua(t *testing.T) {
	casi := map[string][]string{
		"de": {"Spiel fortsetzen", "Einstellungen", "Zum Hauptmenü", "Bist du sicher, dass du das Spiel beenden willst?", "Die Tür ist verschlossen und du hast keinen Schlüssel"},
		"en": {"Continue", "Settings", "Are you sure you want to quit the game?", "The door is locked and you need a key", "Press E to open the door"},
		"it": {"Continua", "Impostazioni", "Sei sicuro di voler uscire dal gioco?", "La porta è chiusa e non hai la chiave per aprirla"},
	}
	for atteso, testi := range casi {
		if l := RilevaLingua(testi); l != atteso {
			t.Errorf("%v: %q invece di %q", testi, l, atteso)
		}
	}
	altri := map[string][]string{
		"fr": {"Continuer la partie", "Paramètres", "Êtes-vous sûr de vouloir quitter le jeu ?", "La porte est fermée et vous avez besoin de la clé"},
		"es": {"Continuar", "Ajustes", "¿Estás seguro de que quieres salir del juego?", "La puerta está cerrada y necesitas una llave para el cofre"},
		"pt": {"Continuar", "Configurações", "Você tem certeza que quer sair do jogo?", "A porta está fechada e você não tem a chave"},
		"ru": {"Продолжить", "Настройки", "Вы уверены, что хотите выйти из игры?"},
		"ja": {"続ける", "設定", "ゲームを終了しますか？"},
		"ko": {"계속하기", "설정", "게임을 종료하시겠습니까?"},
		"zh": {"继续游戏", "设置", "你确定要退出游戏吗？这个选项会关闭"},
		"zt": {"繼續遊戲", "設定", "你確定要退出遊戲嗎？這個選項會關閉"},
		"pl": {"Kontynuuj", "Ustawienia", "Czy na pewno chcesz wyjść z gry? To nie jest dobry pomysł", "Naciśnij przycisk, aby się zalogować w grze"},
		"tr": {"Devam et", "Ayarlar", "Oyundan çıkmak istediğinize emin misiniz? Bu bir oyun ve çok güzel"},
		"sv": {"Fortsätt", "Inställningar", "Är du säker på att du vill avsluta spelet? Det är inte bra och jag vet att du som spelar"},
		"fi": {"Jatka peliä", "Asetukset", "Haluatko varmasti lopettaa pelin? Se on hyvä ja ei ole paha, paina jatka"},
	}
	for atteso, testi := range altri {
		if l := RilevaLingua(testi); l != atteso {
			t.Errorf("%v: %q invece di %q", testi, l, atteso)
		}
	}
	if l := RilevaLingua([]string{"OK", "HP 100"}); l != "" {
		t.Errorf("troppo poco testo: %q", l)
	}
}

func TestModPossibile(t *testing.T) {
	scrivi := func(ver byte) string {
		d := t.TempDir()
		os.MkdirAll(filepath.Join(d, "G_Data", "il2cpp_data", "Metadata"), 0o755)
		os.WriteFile(filepath.Join(d, "G.exe"), peFinto(0x8664), 0o644)
		os.WriteFile(filepath.Join(d, "GameAssembly.dll"), []byte("x"), 0o644)
		os.WriteFile(filepath.Join(d, "G_Data", "il2cpp_data", "Metadata", "global-metadata.dat"), []byte{0xAF, 0x1B, 0xB1, 0xFA, ver, 0, 0, 0}, 0o644)
		return d
	}
	if ok, _ := ModPossibile(scrivi(31)); !ok {
		t.Error("versione 31 deve andare")
	}
	if ok, motivo := ModPossibile(scrivi(108)); ok || motivo == "" {
		t.Error("versione 108 (Unity 6.6) non deve andare")
	}
	if ok, _ := ModPossibile(t.TempDir()); ok {
		t.Error("cartella vuota")
	}
	d32 := scrivi(31)
	os.WriteFile(filepath.Join(d32, "G.exe"), peFinto(0x14c), 0o644)
	if ok, _ := ModPossibile(d32); ok {
		t.Error("gioco a 32 bit: gli strumenti sono a 64 bit")
	}
	// gioco vero, se c'è su questo PC
	if d := `C:\Program Files (x86)\Steam\steamapps\common\Moon Sweepers`; esisteVoce(d) {
		ok, motivo := ModPossibile(d)
		t.Logf("Moon Sweepers: %v %s", ok, motivo)
	}
	if d := `C:\Program Files (x86)\Steam\steamapps\common\Liar's Bar`; esisteVoce(d) {
		ok, motivo := ModPossibile(d)
		t.Logf("Liar's Bar: %v %s", ok, motivo)
	}
}

// Con Tasso in inglese ogni lingua dell'elenco ha il suo nome inglese nei messaggi.
func TestNomiLingueInglesi(t *testing.T) {
	vecchio := InIngleseTasso
	defer func() { InIngleseTasso = vecchio }()
	InIngleseTasso = func() bool { return true }
	for _, l := range lingueGioco {
		if _, ok := nomiInglesi[l.Codice]; !ok {
			t.Errorf("manca il nome inglese di %s", l.Codice)
		}
	}
	if nomeLingua("ja") != "Japanese" || arrivoDiBase() != "en" {
		t.Errorf("nomi o arrivo sbagliati: %q %q", nomeLingua("ja"), arrivoDiBase())
	}
	InIngleseTasso = func() bool { return false }
	if nomeLingua("ja") != "giapponese" || arrivoDiBase() != "it" {
		t.Errorf("in italiano: %q %q", nomeLingua("ja"), arrivoDiBase())
	}
	if etichettaTasto(4<<16|0x2E) != "Maiusc+Canc" {
		t.Errorf("tasto in italiano: %q", etichettaTasto(4<<16|0x2E))
	}
	InIngleseTasso = func() bool { return true }
	if etichettaTasto(4<<16|0x2E) != "Shift+Del" {
		t.Errorf("tasto in inglese: %q", etichettaTasto(4<<16|0x2E))
	}
}

// peFinto: l'intestazione minima di un programma di Windows con il tipo di processore dato.
func peFinto(macchina uint16) []byte {
	b := make([]byte, 0x40+24)
	b[0], b[1] = 'M', 'Z'
	b[0x3c] = 0x40
	copy(b[0x40:], "PE\x00\x00")
	b[0x44], b[0x45] = byte(macchina), byte(macchina>>8)
	return b
}
