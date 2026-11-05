package main

// Protezione dei dati segreti (token del bot Discord, portachiavi) con DPAPI di Windows:
// il file si può aprire solo dal tuo account Windows su questo PC. Copiato altrove non si legge.

import (
	"encoding/base64"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var dpapiEntropy = []byte("Tasso by SerginoLimoncino")

func blobOf(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func blobBytes(d *windows.DataBlob) []byte {
	if d.Data == nil || d.Size == 0 {
		return nil
	}
	out := make([]byte, d.Size)
	copy(out, unsafe.Slice(d.Data, d.Size))
	windows.LocalFree(windows.Handle(unsafe.Pointer(d.Data)))
	return out
}

func protect(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	// 0x1 = CRYPTPROTECT_UI_FORBIDDEN: mai finestre di Windows.
	if err := windows.CryptProtectData(blobOf(plain), nil, blobOf(dpapiEntropy), 0, nil, 0x1, &out); err != nil {
		return nil, err
	}
	b := blobBytes(&out)
	if b == nil {
		return nil, errors.New("protezione non riuscita")
	}
	return b, nil
}

func unprotect(enc []byte) ([]byte, error) {
	if len(enc) == 0 {
		return nil, errors.New("vuoto")
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blobOf(enc), nil, blobOf(dpapiEntropy), 0, nil, 0x1, &out); err != nil {
		return nil, err
	}
	return blobBytes(&out), nil
}

func protectString(s string) string {
	if s == "" {
		return ""
	}
	b, err := protect([]byte(s))
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func unprotectString(s string) string {
	if s == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	b, err := unprotect(raw)
	if err != nil {
		return ""
	}
	return string(b)
}
