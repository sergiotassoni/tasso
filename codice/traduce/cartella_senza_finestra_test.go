package traduce

import (
	"path/filepath"
	"testing"
)

func TestPreparaCartella(t *testing.T) {
	vecchia := dirApp
	defer func() { dirApp = vecchia }()
	dirApp = ""
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // Linux: UserCacheDir; su Windows è %LOCALAPPDATA%
	t.Setenv("LOCALAPPDATA", t.TempDir())
	PreparaCartella()
	if filepath.Base(dirApp) != "TassoTraduce" || !filepath.IsAbs(dirApp) {
		t.Fatalf("dirApp = %q", dirApp)
	}
	dirApp = "/gia/impostata"
	PreparaCartella()
	if dirApp != "/gia/impostata" {
		t.Fatal("PreparaCartella ha cambiato una cartella già impostata da Prepara")
	}
}
