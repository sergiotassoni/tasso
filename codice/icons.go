package main

// Icone vere dei programmi (avvio e installati): Tasso prende l'icona dal programma stesso
// (.exe, .dll, .ico, collegamento) o dal logo dell'app dello Store, e la mostra accanto al nome.
// Prova più fonti, una dopo l'altra; se proprio non ne trova, l'interfaccia mostra l'iniziale.

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32ic      = windows.NewLazySystemDLL("shell32.dll")
	pExtractIconEx = shell32ic.NewProc("ExtractIconExW")
	pSHGetFileInfo = shell32ic.NewProc("SHGetFileInfoW")
	pDestroyIcon   = user32.NewProc("DestroyIcon")
	pGetIconInfo   = user32.NewProc("GetIconInfo")
	pGetDC         = user32.NewProc("GetDC")
	pReleaseDC     = user32.NewProc("ReleaseDC")
	pGetDIBits     = gdi32.NewProc("GetDIBits")
	pDeleteObject  = gdi32.NewProc("DeleteObject")
	pGetObject     = gdi32.NewProc("GetObjectW")

	iconMu    sync.Mutex
	iconCache = map[string]string{}
	envRe     = regexp.MustCompile(`%([^%]+)%`)
)

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  windows.Handle
	hbmColor windows.Handle
}

type bitmapObj struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bmiHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type shFileInfo struct {
	hIcon         windows.Handle
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

// readBits legge i pixel di una bitmap (32 bit, dall'alto in basso).
func readBits(hdc uintptr, bmp windows.Handle) ([]byte, int, int) {
	var bo bitmapObj
	if r, _, _ := pGetObject.Call(uintptr(bmp), unsafe.Sizeof(bo), uintptr(unsafe.Pointer(&bo))); r == 0 || bo.bmWidth <= 0 || bo.bmHeight <= 0 || bo.bmWidth > 256 || bo.bmHeight > 512 {
		return nil, 0, 0
	}
	w, h := int(bo.bmWidth), int(bo.bmHeight)
	bi := bmiHeader{biSize: uint32(unsafe.Sizeof(bmiHeader{})), biWidth: int32(w), biHeight: -int32(h), biPlanes: 1, biBitCount: 32}
	buf := make([]byte, w*h*4)
	// BITMAPINFO = header + 3 colori: lascio spazio in più per sicurezza.
	var info [unsafe.Sizeof(bmiHeader{}) + 16]byte
	*(*bmiHeader)(unsafe.Pointer(&info[0])) = bi
	if r, _, _ := pGetDIBits.Call(hdc, uintptr(bmp), 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&info[0])), 0); r == 0 {
		return nil, 0, 0
	}
	return buf, w, h
}

// iconPNG trasforma un'icona di Windows in un'immagine PNG (data: URL).
func iconPNG(hicon uintptr) string {
	var ii iconInfo
	if r, _, _ := pGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return ""
	}
	defer func() {
		if ii.hbmColor != 0 {
			pDeleteObject.Call(uintptr(ii.hbmColor))
		}
		if ii.hbmMask != 0 {
			pDeleteObject.Call(uintptr(ii.hbmMask))
		}
	}()
	if ii.hbmColor == 0 {
		return "" // icona in bianco e nero: rara, uso l'iniziale
	}
	hdc, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, hdc)
	px, w, h := readBits(hdc, ii.hbmColor)
	if px == nil {
		return ""
	}
	hasAlpha := false
	for i := 3; i < len(px); i += 4 {
		if px[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha && ii.hbmMask != 0 {
		mask, _, _ = readBits(hdc, ii.hbmMask)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			a := px[i+3]
			if !hasAlpha {
				a = 255
				if mask != nil && i < len(mask) && mask[i] != 0 {
					a = 0
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{px[i+2], px[i+1], px[i], a})
		}
	}
	var b bytes.Buffer
	if png.Encode(&b, img) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func expandEnv(p string) string {
	return envRe.ReplaceAllStringFunc(p, func(m string) string {
		if v := os.Getenv(strings.Trim(m, "%")); v != "" {
			return v
		}
		return m
	})
}

// exeFromCmd: dal comando di avvio ("C:\x\a.exe" --min, rundll32 ...) tiro fuori il file del programma.
func exeFromCmd(cmd string) string {
	c := strings.TrimSpace(expandEnv(cmd))
	if c == "" {
		return ""
	}
	if strings.HasPrefix(c, "\"") {
		if i := strings.Index(c[1:], "\""); i > 0 {
			return c[1 : i+1]
		}
		return strings.Trim(c, "\"")
	}
	l := strings.ToLower(c)
	for _, ext := range []string{".exe", ".lnk", ".bat", ".cmd", ".ico", ".dll"} {
		if i := strings.Index(l, ext); i > 0 {
			return c[:i+len(ext)]
		}
	}
	return c
}

// dirExe: il programma principale in una cartella (il più grande, escluse le disinstallazioni).
func dirExe(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type fe struct {
		p string
		n int64
	}
	var list []fe
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if e.IsDir() || !strings.HasSuffix(n, ".exe") || strings.Contains(n, "unins") || strings.Contains(n, "uninst") || strings.Contains(n, "update") || strings.Contains(n, "crash") || strings.Contains(n, "helper") || strings.Contains(n, "setup") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			list = append(list, fe{filepath.Join(dir, e.Name()), fi.Size()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	if len(list) > 0 {
		return list[0].p
	}
	return ""
}

// iconFrom prova una fonte: "file.exe,3", "file.exe", "logo.png", "dir:C:\cartella".
func iconFrom(src string) string {
	src = strings.TrimSpace(expandEnv(src))
	if src == "" {
		return ""
	}
	if strings.HasPrefix(src, "cmd:") {
		return iconFrom(exeFromCmd(strings.TrimPrefix(src, "cmd:")))
	}
	if strings.HasPrefix(src, "dir:") {
		if e := dirExe(strings.TrimPrefix(src, "dir:")); e != "" {
			return iconFrom(e)
		}
		return ""
	}
	idx := 0
	path := strings.Trim(src, "\"")
	if i := strings.LastIndex(path, ","); i > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(path[i+1:])); err == nil {
			idx, path = n, strings.Trim(strings.TrimSpace(path[:i]), "\"")
		}
	}
	low := strings.ToLower(path)
	if strings.HasSuffix(low, "msiexec.exe") || strings.HasSuffix(low, "rundll32.exe") || strings.HasSuffix(low, "explorer.exe") && idx == 0 {
		return ""
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return ""
	}
	if strings.HasSuffix(low, ".png") || strings.HasSuffix(low, ".jpg") {
		if st.Size() > 300<<10 {
			return ""
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		mime := "image/png"
		if strings.HasSuffix(low, ".jpg") {
			mime = "image/jpeg"
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	}
	p16, _ := windows.UTF16PtrFromString(path)
	if strings.HasSuffix(low, ".exe") || strings.HasSuffix(low, ".dll") || strings.HasSuffix(low, ".ico") {
		var large uintptr
		if r, _, _ := pExtractIconEx.Call(uintptr(unsafe.Pointer(p16)), uintptr(int32(idx)), uintptr(unsafe.Pointer(&large)), 0, 1); r != 0 && r != 0xFFFFFFFF && large != 0 {
			defer pDestroyIcon.Call(large)
			if s := iconPNG(large); s != "" {
				return s
			}
		}
	}
	// Qualsiasi file (anche i collegamenti): l'icona che mostra Esplora file.
	var fi shFileInfo
	const shgfiIcon, shgfiLarge = 0x100, 0x0
	if r, _, _ := pSHGetFileInfo.Call(uintptr(unsafe.Pointer(p16)), 0, uintptr(unsafe.Pointer(&fi)), unsafe.Sizeof(fi), shgfiIcon|shgfiLarge); r != 0 && fi.hIcon != 0 {
		defer pDestroyIcon.Call(uintptr(fi.hIcon))
		return iconPNG(uintptr(fi.hIcon))
	}
	return ""
}

// queueIcons: una sola estrazione alla volta; le richieste arrivate nel frattempo si mettono in fila.
var (
	iconQMu   sync.Mutex
	iconQ     = map[string][]string{}
	iconQBusy bool
)

func queueIcons(req map[string][]string) string {
	iconQMu.Lock()
	for k, v := range req {
		iconQ[k] = v
	}
	if iconQBusy {
		iconQMu.Unlock()
		return "ok"
	}
	iconQBusy = true
	iconQMu.Unlock()
	go func() {
		defer guard("icons.go:295")
		for {
			iconQMu.Lock()
			if len(iconQ) == 0 {
				iconQBusy = false
				iconQMu.Unlock()
				return
			}
			batch := iconQ
			iconQ = map[string][]string{}
			iconQMu.Unlock()
			iconsFor(batch)
		}
	}()
	return "ok"
}

// iconsFor: per ogni elemento (chiave -> fonti in ordine) la prima icona trovata.
func iconsFor(req map[string][]string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() { _ = recover() }()
	// SHGetFileInfo vuole COM inizializzato sul thread.
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	defer windows.CoUninitialize()
	out := map[string]string{}
	n := 0
	flush := func() {
		if len(out) > 0 {
			emit("onIcons", out)
			out = map[string]string{}
		}
	}
	for key, srcs := range req {
		got := ""
		for _, s := range srcs {
			iconMu.Lock()
			c, ok := iconCache[s]
			iconMu.Unlock()
			if !ok {
				c = func() (r string) {
					defer func() {
						if recover() != nil {
							r = ""
						}
					}()
					return iconFrom(s)
				}()
				iconMu.Lock()
				if len(iconCache) > 3000 {
					iconCache = map[string]string{}
				}
				iconCache[s] = c
				iconMu.Unlock()
			}
			if c != "" {
				got = c
				break
			}
		}
		out[key] = got
		if n++; n%25 == 0 {
			flush()
		}
	}
	flush()
}
