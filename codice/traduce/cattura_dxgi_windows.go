//go:build windows

package traduce

// Cattura dello schermo con la "duplicazione del desktop" di Windows (DXGI, la stessa che usano
// OBS e la registrazione di Windows). Sul PC di Sergio la cattura classica (BitBlt) costava circa
// 250 ms a schermata: questa legge la copia che la scheda video ha già pronta, in pochi
// millisecondi. Non tocca il gioco: legge solo l'immagine del monitor. Le finestre di Tasso
// escluse dalle catture restano escluse. Se qualcosa non va, si torna alla cattura classica.

import (
	"errors"
	"image"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	d3d11                 = windows.NewLazySystemDLL("d3d11.dll")
	pD3D11CreateDevice    = d3d11.NewProc("D3D11CreateDevice")
	iidIDXGIDevice        = windows.GUID{Data1: 0x54ec77fa, Data2: 0x1377, Data3: 0x44e6, Data4: [8]byte{0x8c, 0x32, 0x88, 0xfd, 0x5f, 0x44, 0xc8, 0x4c}}
	iidIDXGIOutput1       = windows.GUID{Data1: 0x00cddea8, Data2: 0x939b, Data3: 0x4b83, Data4: [8]byte{0xa3, 0x40, 0xa6, 0x85, 0x22, 0x66, 0x66, 0xcc}}
	iidID3D11Texture2D    = windows.GUID{Data1: 0x6f15aaf2, Data2: 0xd208, Data3: 0x4e89, Data4: [8]byte{0x9a, 0xb4, 0x48, 0x95, 0x35, 0xd3, 0x4f, 0x9c}}
	errDXGINonDisponibile = errors.New("duplicazione del desktop non disponibile")
	errNessunFotogramma   = errors.New("nessun fotogramma ancora")
	errStessoFotogramma   = errors.New("lo schermo non è cambiato dall'ultima cattura")
)

const (
	dxgiErrWaitTimeout = 0x887A0027
)

func chiama(obj unsafe.Pointer, indice int, args ...uintptr) uint32 {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(indice)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return uint32(r)
}

func lascia(obj unsafe.Pointer) {
	if obj != nil {
		chiama(obj, 2)
	}
}

type texture2DDesc struct {
	Width, Height, MipLevels, ArraySize, Format uint32
	SampleCount, SampleQuality                  uint32
	Usage, BindFlags, CPUAccessFlags, MiscFlags uint32
}

type outputDesc struct {
	DeviceName [32]uint16
	Coordinate rect
	Attaccato  int32
	Rotazione  uint32
	Monitor    uintptr
}

type duplicatore struct {
	dev, ctx, dup, staging unsafe.Pointer
	schermo                rect // coordinate del monitor sul desktop
	w, h                   int
	pronta                 bool // nello staging c'è un'immagine valida
}

var (
	dxgiFunziona atomic.Bool // almeno un fotogramma arrivato davvero
	muDXGI       sync.Mutex
	duplic       []*duplicatore // uno per monitor
	dxgiNo       bool           // non riuscita: per 30 s uso BitBlt, poi riprovo
	dxgiQuando   time.Time
	dxgiPron     bool
	erroriDXGI   int // duplicazioni perse di fila
	dxgiGen      uint64
	giaPronte    []catturaPronta // ultime immagini già convertite dell'ultimo fotogramma
)

// catturaPronta: una zona dell'ultimo fotogramma già convertita. Finché Windows non manda un
// fotogramma nuovo si riusa: prima ogni cattura riconvertiva tutta l'immagine (8-33 MB, 25 volte
// al secondo) anche con lo schermo fermo.
type catturaPronta struct {
	d   *duplicatore
	z   Zona
	gen uint64
	img *image.RGBA
}

func preparaDXGI() error {
	var dev, ctx unsafe.Pointer
	var livello uint32
	r, _, _ := pD3D11CreateDevice.Call(0, 1 /*HARDWARE*/, 0, 0, 0, 0, 7 /*SDK*/, uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(&livello)), uintptr(unsafe.Pointer(&ctx)))
	if uint32(r) != 0 || dev == nil {
		lascia(ctx)
		return errDXGINonDisponibile
	}
	// ogni duplicatore tiene il suo riferimento: questi si lasciano sempre, anche se qualcosa va storto
	defer lascia(dev)
	defer lascia(ctx)
	var dxdev, adattatore unsafe.Pointer
	if chiama(dev, 0, uintptr(unsafe.Pointer(&iidIDXGIDevice)), uintptr(unsafe.Pointer(&dxdev))) != 0 {
		return errDXGINonDisponibile
	}
	defer lascia(dxdev)
	if chiama(dxdev, 7, uintptr(unsafe.Pointer(&adattatore))) != 0 {
		return errDXGINonDisponibile
	}
	defer lascia(adattatore)
	for i := 0; i < 8; i++ {
		var out, out1, dup unsafe.Pointer
		if chiama(adattatore, 7, uintptr(i), uintptr(unsafe.Pointer(&out))) != 0 {
			break
		}
		var d outputDesc
		chiama(out, 7, uintptr(unsafe.Pointer(&d)))
		if chiama(out, 0, uintptr(unsafe.Pointer(&iidIDXGIOutput1)), uintptr(unsafe.Pointer(&out1))) == 0 {
			if chiama(out1, 22, uintptr(dev), uintptr(unsafe.Pointer(&dup))) == 0 && dup != nil {
				chiama(dev, 1) // AddRef: ogni duplicatore tiene un riferimento al device
				chiama(ctx, 1)
				duplic = append(duplic, &duplicatore{dev: dev, ctx: ctx, dup: dup, schermo: d.Coordinate,
					w: int(d.Coordinate.Right - d.Coordinate.Left), h: int(d.Coordinate.Bottom - d.Coordinate.Top)})
			}
			lascia(out1)
		}
		lascia(out)
	}
	if len(duplic) == 0 {
		return errDXGINonDisponibile
	}
	return nil
}

func chiudiDXGI() {
	for _, d := range duplic {
		lascia(d.staging)
		lascia(d.dup)
		lascia(d.ctx)
		lascia(d.dev)
	}
	duplic = nil
	dxgiPron = false
	giaPronte = nil
}

// aggiorna prende l'ultima immagine del monitor (se è cambiata) nello staging. nuovo dice se
// c'è davvero un fotogramma nuovo (se no nello staging resta quello di prima).
func (d *duplicatore) aggiorna() (nuovo bool, err error) {
	var info [64]byte
	var risorsa unsafe.Pointer
	r := chiama(d.dup, 8, 0 /*niente attesa*/, uintptr(unsafe.Pointer(&info[0])), uintptr(unsafe.Pointer(&risorsa)))
	if r == dxgiErrWaitTimeout {
		if d.pronta {
			return false, nil // niente di nuovo sullo schermo: vale l'immagine di prima
		}
		// non ho ancora nessuna immagine e lo schermo è fermo (Windows manda un fotogramma solo
		// quando qualcosa cambia): chi chiama usa la cattura classica, e finché lo schermo resta
		// fermo può riusare quella
		return false, errNessunFotogramma
	}
	if r != 0 {
		return false, errors.New("duplicazione persa")
	}
	defer chiama(d.dup, 14) // ReleaseFrame
	defer lascia(risorsa)
	ultimaPresentazione := *(*int64)(unsafe.Pointer(&info[0]))
	if ultimaPresentazione == 0 {
		// è cambiato solo il mouse: l'immagine del desktop in questo fotogramma non c'è
		if d.pronta {
			return false, nil
		}
		return false, errNessunFotogramma
	}
	var tex unsafe.Pointer
	if chiama(risorsa, 0, uintptr(unsafe.Pointer(&iidID3D11Texture2D)), uintptr(unsafe.Pointer(&tex))) != 0 {
		return false, errDXGINonDisponibile
	}
	defer lascia(tex)
	if d.staging == nil {
		var desc texture2DDesc
		chiama(tex, 10, uintptr(unsafe.Pointer(&desc)))
		if desc.Format != 87 && desc.Format != 91 { // B8G8R8A8 (anche _SRGB)
			return false, errors.New("formato dello schermo non gestito")
		}
		desc.MipLevels, desc.ArraySize = 1, 1
		desc.Usage, desc.BindFlags, desc.CPUAccessFlags, desc.MiscFlags = 3 /*STAGING*/, 0, 0x20000 /*READ*/, 0
		desc.SampleCount, desc.SampleQuality = 1, 0
		if chiama(d.dev, 5, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&d.staging))) != 0 {
			d.staging = nil
			return false, errDXGINonDisponibile
		}
		d.w, d.h = int(desc.Width), int(desc.Height)
	}
	chiama(d.ctx, 47, uintptr(tex), uintptr(d.staging)) // CopyResource
	d.pronta = true
	dxgiFunziona.Store(true)
	return true, nil
}

type mappata struct {
	dati               unsafe.Pointer
	rowPitch, depthPit uint32
}

// catturaDXGI: come cattura(), ma con la duplicazione del desktop.
func catturaDXGI(z Zona) (*image.RGBA, error) {
	img, _, err := catturaDXGIGen(z, 0)
	return img, err
}

// catturaDXGIGen: come catturaDXGI, e dice anche il numero del fotogramma (gen). Se ultimo è il
// numero della cattura precedente di questa zona e Windows non ha mandato niente di nuovo,
// restituisce errStessoFotogramma senza rifare l'immagine (chi chiama riusa la sua).
func catturaDXGIGen(z Zona, ultimo uint64) (*image.RGBA, uint64, error) {
	muDXGI.Lock()
	defer muDXGI.Unlock()
	if dxgiNo && time.Since(dxgiQuando) < 30*time.Second {
		return nil, 0, errDXGINonDisponibile
	}
	if !dxgiPron {
		if err := preparaDXGI(); err != nil {
			// Ctrl+Alt+Canc, blocco schermo, gioco a schermo intero esclusivo: riprovo tra 30 s
			chiudiDXGI()
			dxgiNo, dxgiQuando = true, time.Now()
			return nil, 0, err
		}
		dxgiNo = false
		dxgiPron = true
	}
	var d *duplicatore
	for _, c := range duplic {
		if z.X >= int(c.schermo.Left) && z.Y >= int(c.schermo.Top) && z.X+z.W <= int(c.schermo.Right) && z.Y+z.H <= int(c.schermo.Bottom) {
			d = c
			break
		}
	}
	if d == nil {
		return nil, 0, errors.New("la zona sta su più monitor")
	}
	nuovo, err := d.aggiorna()
	if err != nil {
		if err != errNessunFotogramma {
			// risoluzione cambiata, gioco a schermo intero esclusivo, Ctrl+Alt+Canc…: ricreo tutto
			// la volta dopo. Ma se succede tre volte di fila, per 30 s uso la cattura classica
			// invece di ricreare tutto a ogni cattura
			chiudiDXGI()
			erroriDXGI++
			if erroriDXGI >= 3 {
				dxgiNo, dxgiQuando, erroriDXGI = true, time.Now(), 0
			}
		}
		return nil, 0, err
	}
	erroriDXGI = 0
	if nuovo {
		// fotogramma nuovo: le immagini già pronte di questo monitor non valgono più
		tenute := giaPronte[:0]
		for _, c := range giaPronte {
			if c.d != d {
				tenute = append(tenute, c)
			}
		}
		clear(giaPronte[len(tenute):]) // niente immagini vecchie tenute in vita dal fondo della lista
		giaPronte = tenute
	} else {
		for _, c := range giaPronte {
			if c.d == d && c.z == z {
				if ultimo != 0 && ultimo == c.gen {
					return nil, c.gen, errStessoFotogramma
				}
				out := image.NewRGBA(c.img.Bounds()) // una copia: chi chiama può tenersela
				copy(out.Pix, c.img.Pix)
				return out, c.gen, nil
			}
		}
	}
	img, err := d.converti(z)
	if err != nil {
		return nil, 0, err
	}
	dxgiGen++
	if len(giaPronte) >= 4 { // al massimo 4 zone (pagina, dialoghi, chat…)
		giaPronte[0] = catturaPronta{}
		giaPronte = giaPronte[1:]
	}
	giaPronte = append(giaPronte, catturaPronta{d: d, z: z, gen: dxgiGen, img: img})
	return img, dxgiGen, nil
}

// converti: la zona z dello staging (BGRA della scheda video) in un'immagine RGBA nuova.
func (d *duplicatore) converti(z Zona) (*image.RGBA, error) {
	var m mappata
	if chiama(d.ctx, 14, uintptr(d.staging), 0, 1 /*READ*/, 0, uintptr(unsafe.Pointer(&m))) != 0 || m.dati == nil {
		return nil, errDXGINonDisponibile
	}
	defer chiama(d.ctx, 15, uintptr(d.staging), 0)
	x0, y0 := z.X-int(d.schermo.Left), z.Y-int(d.schermo.Top)
	if x0 < 0 || y0 < 0 || x0+z.W > d.w || y0+z.H > d.h || z.W <= 0 || z.H <= 0 {
		return nil, errors.New("zona fuori dal monitor")
	}
	img := image.NewRGBA(image.Rect(0, 0, z.W, z.H))
	for y := 0; y < z.H; y++ {
		riga := unsafe.Slice((*byte)(unsafe.Add(m.dati, uintptr((y0+y)*int(m.rowPitch)+x0*4))), z.W*4)
		dst := img.Pix[y*img.Stride : y*img.Stride+z.W*4]
		for i := 0; i < len(riga); i += 4 { // BGRA -> RGBA
			dst[i], dst[i+1], dst[i+2], dst[i+3] = riga[i+2], riga[i+1], riga[i], 255
		}
	}
	return img, nil
}
