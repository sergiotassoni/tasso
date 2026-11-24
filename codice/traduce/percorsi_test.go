package traduce

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPulisciScriptTT(t *testing.T) {
	d := t.TempDir()
	vecchio := filepath.Join(d, "tt_ocr_1_2.ps1")
	vecchio2 := filepath.Join(d, "tt_9_9.ps1")
	nuovo := filepath.Join(d, "tt_ocr_3_4.ps1")
	altro := filepath.Join(d, "mio.ps1")
	for _, f := range []string{vecchio, vecchio2, nuovo, altro} {
		_ = os.WriteFile(f, []byte("x"), 0o644)
	}
	ieri := time.Now().Add(-30 * time.Hour)
	_ = os.Chtimes(vecchio, ieri, ieri)
	_ = os.Chtimes(vecchio2, ieri, ieri)
	_ = os.Chtimes(altro, ieri, ieri)
	if n := pulisciScriptTT(d, time.Now()); n != 2 {
		t.Fatalf("tolti %d, attesi 2", n)
	}
	for f, deve := range map[string]bool{vecchio: false, vecchio2: false, nuovo: true, altro: true} {
		if esisteFile(f) != deve {
			t.Errorf("%s: esiste=%v, atteso %v", filepath.Base(f), !deve, deve)
		}
	}
	togliScriptTT(nuovo)
	if esisteFile(nuovo) {
		t.Error("script usato non tolto")
	}
}
