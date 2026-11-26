package rianalisi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEtichette(t *testing.T) {
	casi := map[uint32]bool{
		0xA000000C: true,  // IO_REPARSE_TAG_SYMLINK
		0xA0000003: true,  // IO_REPARSE_TAG_MOUNT_POINT (giunzione)
		0x80000023: true,  // AF_UNIX
		0x9000001A: false, // IO_REPARSE_TAG_CLOUD (OneDrive)
		0x9000601A: false, // IO_REPARSE_TAG_CLOUD_6
		0x9000F01A: false, // IO_REPARSE_TAG_CLOUD_F
		0x80000017: false, // IO_REPARSE_TAG_WOF (compresso da Windows)
		0x80000013: false, // IO_REPARSE_TAG_DEDUP
	}
	for tag, atteso := range casi {
		if collegamentoEtichetta(tag) != atteso {
			t.Errorf("etichetta %#x: atteso collegamento=%v", tag, atteso)
		}
	}
}

func TestCollegamento(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(f)
	if Collegamento(f, fi) {
		t.Fatal("un file normale non è un collegamento")
	}
	l := filepath.Join(dir, "l")
	if err := os.Symlink(f, l); err == nil {
		fi, _ := os.Lstat(l)
		if !Collegamento(l, fi) {
			t.Fatal("il symlink è un collegamento")
		}
	}
	if !Collegamento(f, nil) {
		t.Fatal("senza informazioni: collegamento (prudenza)")
	}
}
