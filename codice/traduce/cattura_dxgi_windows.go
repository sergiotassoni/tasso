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
	errDuplicazionePersa  = errors.New("duplicazione persa")
	errAccessoPerso       = errors.New("duplicazione persa (cambio di schermo)")
)

const (
	dxgiErrWaitTimeout = 0x887A0027
	dxgiErrAccessLost  = 0x887A0026 // normale per qualche fotogramma al cambio schermo intero/HDR
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
	dxgiNo       bool           // non riuscita: per dxgiAttesa uso BitBlt, poi riprovo
	dxgiQuando   time.Time
	dxgiAttesa   time.Duration // quanto aspettare prima di riprovare: cresce da 0,5 s a 30 s
	dxgiPron     bool
	erroriDXGI   []time.Time // quando si è persa la duplicazione (solo gli ultimi 2 secondi)
	dxgiPronta   time.Time   // primo preparaDXGI riuscito dopo l'ultimo fotogramma arrivato
	dxgiGen      uint64
	giaPronte    []catturaPronta // ultime immagini già convertite dell'ultimo fotogramma
	pulizia      bool            // c'è già un controllo programmato che toglie le immagini vecchie
)

const (
	finestraErroriDXGI = 2 * time.Second
	maxErroriDXGI      = 5                       // più di 5 duplicazioni perse in 2 s: uso BitBlt per un po'
	graziaDXGI         = 1500 * time.Millisecond // dopo una preparazione riuscita ACCESS_LOST è normale
	vitaPronta         = 2 * time.Second         // un'immagine già pronta non usata da 2 s si butta
)

// catturaPronta: una zona dell'ultimo fotogramma già convertita. Finché Windows non manda un
// fotogramma nuovo si riusa: prima ogni cattura riconvertiva tutta l'immagine (8-33 MB, 25 volte
// al secondo) anche con lo schermo fermo.
type catturaPronta struct {
	d     *duplicatore
	z     Zona
	gen   uint64
	img   *image.RGBA
	usata time.Time // l'ultima volta che qualcuno l'ha chiesta
}

// spegniDXGIPerUnPo: per un po' uso la cattura classica. L'attesa raddoppia a ogni volta (0,5 s,
// 1 s, 2 s… fino a 30 s) e torna a 0,5 s appena arriva un fotogramma buono: un cambio di
// schermo intero o di HDR fa perdere la duplicazione per pochi fotogrammi, e prima bastava a
// spegnerla per 30 s (BitBlt nero e falso avviso di schermo esclusivo).
func spegniDXGIPerUnPo() {
	if dxgiAttesa <= 0 {
		dxgiAttesa = 500 * time.Millisecond
	} else {
		dxgiAttesa = min(dxgiAttesa*2, 30*time.Second)
	}
	dxgiNo, dxgiQuando = true, time.Now()
	erroriDXGI = erroriDXGI[:0]
}

// togliPronteVecchie: butta le immagini già pronte che nessuno chiede da più di vitaPronta (in
// 4K sono 33 MB l'una). Si chiama con muDXGI preso.
func togliPronteVecchie(ora time.Time) {
	tenute := giaPronte[:0]
	for _, c := range giaPronte {
		if ora.Sub(c.usata) <= vitaPronta {
			tenute = append(tenute, c)
		}
	}
	clear(giaPronte[len(tenute):])
	giaPronte = tenute
	if len(giaPronte) > 0 && !pulizia {
		// se nessuno cattura più (traduzione spenta) le butto lo stesso, poco dopo
		pulizia = true
		time.AfterFunc(vitaPronta+500*time.Millisecond, func() {
			muDXGI.Lock()
			defer muDXGI.Unlock()
			pulizia = false
			togliPronteVecchie(time.Now())
		})
	}
}

// liberaCatturePronte: la pagina si ferma, le immagini già pronte non servono più.
func liberaCatturePronte() {
	muDXGI.Lock()
	defer muDXGI.Unlock()
	clear(giaPronte)
	giaPronte = nil
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
		// monitor ruotato (verticale): l'immagine duplicata non è ruotata come le coordinate e la
		// zona si leggerebbe nel posto sbagliato. Lì resta la cattura classica (collaudo 26/11).
		if d.Rotazione > 1 { // 1 = DXGI_MODE_ROTATION_IDENTITY (0 = non specificata)
			lascia(out)
			continue
		}
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
	if r == dxgiErrAccessLost {
		return false, errAccessoPerso
	}
	if r != 0 {
		return false, errDuplicazionePersa
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

// catturaDXGIGen: come cattura(), ma con la duplicazione del desktop, e dice anche il numero del
// fotogramma (gen). Se ultimo è il
// numero della cattura precedente di questa zona e Windows non ha mandato niente di nuovo,
// restituisce errStessoFotogramma senza rifare l'immagine (chi chiama riusa la sua).
func catturaDXGIGen(z Zona, ultimo uint64) (*image.RGBA, uint64, error) {
	muDXGI.Lock()
	defer muDXGI.Unlock()
	if dxgiNo && time.Since(dxgiQuando) < dxgiAttesa {
		return nil, 0, errDXGINonDisponibile
	}
	ora := time.Now()
	togliPronteVecchie(ora)
	if !dxgiPron {
		if err := preparaDXGI(); err != nil {
			// Ctrl+Alt+Canc, blocco schermo, gioco a schermo intero esclusivo: riprovo tra poco,
			// ogni volta un po' più tardi
			chiudiDXGI()
			spegniDXGIPerUnPo()
			return nil, 0, err
		}
		dxgiNo = false
		dxgiPron = true
		if dxgiPronta.IsZero() {
			dxgiPronta = ora
		}
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
	if err == nil {
		// la duplicazione funziona (c'è un fotogramma buono): tutto torna normale
		erroriDXGI = erroriDXGI[:0]
		dxgiAttesa = 0
		dxgiPronta = time.Time{}
	}
	if err != nil {
		if err != errNessunFotogramma {
			// risoluzione cambiata, schermo intero o HDR acceso, Ctrl+Alt+Canc…: ricreo tutto la
			// volta dopo. ACCESS_LOST subito dopo una preparazione riuscita è normale per qualche
			// fotogramma: non conta (al massimo per graziaDXGI dalla prima preparazione).
			// Solo se si perde più di 5 volte in 2 secondi uso per un po' la cattura classica
			chiudiDXGI()
			if !(err == errAccessoPerso && ora.Sub(dxgiPronta) < graziaDXGI) {
				tenuti := erroriDXGI[:0]
				for _, t := range erroriDXGI {
					if ora.Sub(t) <= finestraErroriDXGI {
						tenuti = append(tenuti, t)
					}
				}
				erroriDXGI = append(tenuti, ora)
				if len(erroriDXGI) > maxErroriDXGI {
					spegniDXGIPerUnPo()
				}
			}
		}
		return nil, 0, err
	}
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
		for i, c := range giaPronte {
			if c.d == d && c.z == z {
				giaPronte[i].usata = ora
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
	// una zona sola per volta per ogni zona chiesta: la stessa zona presa di nuovo sostituisce la sua
	tenute := giaPronte[:0]
	for _, c := range giaPronte {
		if c.d != d || c.z != z {
			tenute = append(tenute, c)
		}
	}
	clear(giaPronte[len(tenute):])
	giaPronte = tenute
	if len(giaPronte) >= 3 { // al massimo 3 zone (pagina, dialoghi, chat), solo quelle usate da poco
		giaPronte[0] = catturaPronta{}
		giaPronte = giaPronte[1:]
	}
	giaPronte = append(giaPronte, catturaPronta{d: d, z: z, gen: dxgiGen, img: img, usata: ora})
	togliPronteVecchie(ora) // programma la pulizia se poi nessuno cattura più
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
