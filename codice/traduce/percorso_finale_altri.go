//go:build !windows

package traduce

import "os"

// apertoDentro: fuori da Windows (solo test) non c'è GetFinalPathNameByHandle: bastano i
// controlli con Lstat e os.SameFile.
func apertoDentro(f *os.File, radice string) bool { return true }
