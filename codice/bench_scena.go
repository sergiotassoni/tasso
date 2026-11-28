package main

// Scena di prova DirectX 11 di Ottimizza tutto (3.2): Tasso disegna una sua scena (personaggi,
// luci, ombre e particelle calcolati dalla scheda video, 1920×1080, fuori dallo schermo: niente finestra, niente giochi
// toccati) per circa 30 secondi e misura quanto ci mette ogni fotogramma. Ne escono gli FPS medi e
// gli FPS più bassi (1%). Usa la scheda video più potente (quella che userebbero i giochi).
// Tutto con le DLL di Windows (d3d11, dxgi, d3dcompiler_47), senza scaricare niente.
// Se qualcosa non parte (Windows senza DirectX 11, desktop remoto, driver di base) torna un
// errore e Ottimizza tutto continua senza FPS.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	d3d11DLL         = windows.NewLazySystemDLL("d3d11.dll")
	pD3D11CreateDev  = d3d11DLL.NewProc("D3D11CreateDevice")
	d3dCompilerDLL   = windows.NewLazySystemDLL("d3dcompiler_47.dll")
	pD3DCompile      = d3dCompilerDLL.NewProc("D3DCompile")
	iidDXGIFactory6  = windows.GUID{Data1: 0xc1b6694f, Data2: 0xff09, Data3: 0x44a9, Data4: [8]byte{0xb0, 0x3c, 0x77, 0x90, 0x0a, 0x0a, 0x1d, 0x17}}
	iidDXGIAdapter1  = windows.GUID{Data1: 0x29038f61, Data2: 0x3839, Data3: 0x4626, Data4: [8]byte{0x91, 0xfd, 0x08, 0x68, 0x79, 0x01, 0x1a, 0x05}}
	errScenaNonParte = errors.New("scena di prova non partita")
)

// Il disegno (3.2, più pesante): un campo aperto con edifici, alberi semplici e tanti personaggi
// (testa e corpo) che si muovono, il pavimento con una trama calcolata, 16 luci colorate che
// seguono i gruppi di personaggi (mossi dal processore a ogni fotogramma), un'ombra morbida e 64 particelle
// trasparenti. Tutto con il raymarching, nella scheda video. Pesa come un gioco di media
// pesantezza, non come una scena vuota. _p: centro e scala della tela (calibrazione).
const scenaHLSL = `
cbuffer C : register(b0) { float T; float3 _p; float4 L[16]; };
float4 VS(uint id : SV_VertexID) : SV_Position {
  float2 p = float2((id << 1) & 2, id & 2);
  return float4(p * float2(2, -2) + float2(-1, 1), 0, 1);
}
float mappa(float3 p) {
  float3 q = p; q.xz = fmod(abs(q.xz) + 100.0, 2.0) - 1.0;
  float2 cella = floor(p.xz);
  float h = 0.35 + 0.15 * sin(T * 2.0 + cella.x * 1.7 + cella.y * 0.9);
  float s = length(q - float3(0.2 * sin(T + cella.y), h, 0)) - 0.26;
  float3 b = abs(q - float3(0.2 * sin(T + cella.y), h - 0.42, 0)) - float3(0.16, 0.24, 0.16);
  float corpo = length(max(b, 0.0)) + min(max(b.x, max(b.y, b.z)), 0.0);
  // gli edifici (uno ogni 10 metri, altezze diverse, lontani dalla telecamera) e gli alberi
  // semplici (tronco e chioma)
  float2 ce = floor(p.xz / 10.0); float3 e = p; e.xz = fmod(abs(e.xz) + 1000.0, 10.0) - 5.0;
  float alto = 1.0 + 2.5 * frac(sin(dot(ce, float2(12.9, 78.2))) * 437.5);
  float3 be = abs(e - float3(0, alto * 0.5 - 0.5, 0)) - float3(1.2, alto * 0.5, 1.2);
  float casa = length(max(be, 0.0)) + min(max(be.x, max(be.y, be.z)), 0.0);
  casa = max(casa, -(length(e.xy - float2(0, alto - 0.6)) - 0.25)); // le finestre
  float3 a = p; a.xz = fmod(abs(a.xz + 2.0) + 600.0, 6.0) - 3.0;
  float tronco = max(length(a.xz) - 0.08, abs(a.y + 0.1) - 0.4);
  float chioma = length(a - float3(0, 0.6, 0)) - 0.45 - 0.05 * sin(a.x * 9.0 + T) * sin(a.z * 9.0);
  return min(min(min(s, corpo), min(casa, min(tronco, chioma))), p.y + 0.5);
}
float3 normale(float3 p) {
  float2 e = float2(0.002, 0);
  return normalize(float3(mappa(p + e.xyy) - mappa(p - e.xyy), mappa(p + e.yxy) - mappa(p - e.yxy), mappa(p + e.yyx) - mappa(p - e.yyx)));
}
float rumore(float2 x) {
  float2 i = floor(x), f = frac(x); f = f * f * (3.0 - 2.0 * f);
  float a = frac(sin(dot(i, float2(127.1, 311.7))) * 43758.5);
  float b = frac(sin(dot(i + float2(1, 0), float2(127.1, 311.7))) * 43758.5);
  float c = frac(sin(dot(i + float2(0, 1), float2(127.1, 311.7))) * 43758.5);
  float d = frac(sin(dot(i + float2(1, 1), float2(127.1, 311.7))) * 43758.5);
  return lerp(lerp(a, b, f.x), lerp(c, d, f.x), f.y);
}
float trama(float2 x) {
  float v = 0.0, a = 0.5;
  [loop] for (int o = 0; o < 5; o++) { v += a * rumore(x); x *= 2.03; a *= 0.5; }
  return v;
}
float ombra(float3 p, float3 l) {
  float r = 1.0, t = 0.05;
  [loop] for (int i = 0; i < 32; i++) {
    float d = mappa(p + l * t);
    r = min(r, 8.0 * d / t);
    t += clamp(d, 0.03, 0.4);
    if (r < 0.01 || t > 8.0) break;
  }
  return saturate(r);
}
float4 PS(float4 pos : SV_Position) : SV_Target {
  float2 uv = (pos.xy - _p.xy) / _p.z;
  float3 ro = float3(sin(T * 0.3) * 3.0, 1.2, T * 0.8), rd = normalize(float3(uv.x, -uv.y - 0.25, 1.4));
  float t = 0.0; float3 c = float3(0.02, 0.03, 0.05);
  [loop] for (int i = 0; i < 128; i++) {
    float d = mappa(ro + rd * t);
    if (d < 0.001 || t > 40.0) break;
    t += d;
  }
  if (t < 40.0) {
    float3 p = ro + rd * t, n = normale(p), l = normalize(float3(0.6, 0.8, -0.4));
    float dif = saturate(dot(n, l)) * ombra(p + n * 0.01, l), amb = 0.25 + 0.25 * n.y;
    float3 base = 0.5 + 0.5 * cos(float3(0, 2, 4) + floor(p.x) + floor(p.z));
    if (p.y < -0.49) base = float3(0.25, 0.3, 0.22) * (0.6 + 0.8 * trama(p.xz * 3.0));
    float sp = pow(saturate(dot(reflect(-l, n), -rd)), 32.0);
    float3 luci = float3(0, 0, 0);
    [loop] for (int k = 0; k < 16; k++) {
      float3 v = L[k].xyz - p; float d2 = dot(v, v);
      luci += (0.5 + 0.5 * cos(float3(0, 2, 4) + k)) * L[k].w * saturate(dot(n, v * rsqrt(d2 + 0.0001))) / (1.0 + d2);
    }
    c = base * (dif + amb + luci) + sp;
    c = lerp(c, float3(0.02, 0.03, 0.05), saturate(t / 40.0));
  }
  float3 part = float3(0, 0, 0);
  [loop] for (int j = 0; j < 64; j++) {
    float fj = j;
    float3 pp = float3(sin(fj * 12.9 + T) * 5.0, 0.2 + 1.2 * frac(fj * 0.37 + T * 0.2), ro.z + 2.0 + fmod(fj * 1.37 + T, 18.0));
    float3 w = pp - ro; float tt = max(dot(w, rd), 0.0);
    float3 dv = w - rd * tt; float dd = dot(dv, dv);
    if (tt < t) part += (0.5 + 0.5 * cos(float3(0, 2, 4) + fj)) * 0.0015 / (dd + 0.0015);
  }
  c = c * (1.0 - saturate(dot(part, float3(0.1, 0.1, 0.1)))) + part * 0.6;
  return float4(pow(saturate(c), 0.4545), 1);
}
`

// scenaPassate: quante volte si disegna la scena in ogni fotogramma (il numero da cambiare per
// calibrare il peso: 2 = circa il doppio del lavoro per la scheda video).
const scenaPassate = 1

// scenaFolla: quanti personaggi muove il processore a ogni fotogramma.
const scenaFolla = 3000

var (
	scenaScelta  atomic.Bool  // la risoluzione è già stata scelta (nella prima misura)
	scenaLivello atomic.Int32 // 0 = 1920×1080, 1 = 1280×720, 2 = 960×540 (schede video deboli)
	scenaTele    = [3][2]float32{{1920, 1080}, {1280, 720}, {960, 540}}
)

// scenaCB: i dati per la scheda video (T, centro e scala della tela, 16 luci: posizione e forza).
type scenaCB struct{ v [4 + 16*4]float32 }

type follaScena struct{ x, z, vx, vz []float32 }

func nuovaFolla() *follaScena {
	f := &follaScena{make([]float32, scenaFolla), make([]float32, scenaFolla), make([]float32, scenaFolla), make([]float32, scenaFolla)}
	for i := range f.x {
		a := float64(i) * 2.399963 // sempre gli stessi punti di partenza: prima e dopo uguali
		f.x[i], f.z[i] = float32(math.Cos(a)*float64(i%40)*0.5), float32(math.Sin(a)*float64(i%40)*0.5)
	}
	return f
}

// passo: muove i personaggi (si avvicinano alla loro meta e si scansano dai vicini) e mette le luci
// nel centro dei 16 gruppi, davanti alla telecamera.
func (f *follaScena) passo(cb *scenaCB, t, w, h float32) {
	n := len(f.x)
	for i := 0; i < n; i++ {
		g := float64(i%16) + float64(t)*0.3
		mx, mz := float32(math.Cos(g)*8), float32(math.Sin(g*1.3)*8)
		ax, az := (mx-f.x[i])*0.002, (mz-f.z[i])*0.002
		for k := 1; k <= 4; k++ {
			j := (i + k*37) % n
			dx, dz := f.x[i]-f.x[j], f.z[i]-f.z[j]
			if d2 := dx*dx + dz*dz; d2 < 0.25 && d2 > 1e-6 {
				ax, az = ax+dx*0.01/d2, az+dz*0.01/d2
			}
		}
		f.vx[i], f.vz[i] = (f.vx[i]+ax)*0.95, (f.vz[i]+az)*0.95
		f.x[i], f.z[i] = f.x[i]+f.vx[i], f.z[i]+f.vz[i]
	}
	cb.v[0], cb.v[1], cb.v[2], cb.v[3] = t, w/2, h/2, h/2
	for g := 0; g < 16; g++ {
		var sx, sz float32
		c := 0
		for i := g; i < n; i += 16 {
			sx, sz, c = sx+f.x[i], sz+f.z[i], c+1
		}
		o := 4 + g*4
		cb.v[o], cb.v[o+1], cb.v[o+2], cb.v[o+3] = sx/float32(c)*0.4, 0.3, t*0.8+3+float32(g%8)*1.5+sz/float32(c)*0.2, 0.6
	}
}

// adattatorePotente: la scheda video ad alte prestazioni (IDXGIFactory6, Windows 10 1803+), se no
// quella con più memoria video. 0 = quella di base di Windows.
func adattatorePotente() uintptr {
	if pCreateDXGIFactory.Find() != nil {
		return 0
	}
	var fac uintptr
	if hr, _, _ := pCreateDXGIFactory.Call(uintptr(unsafe.Pointer(&iidDXGIFactory1)), uintptr(unsafe.Pointer(&fac))); !hrOK(hr) || fac == 0 {
		return 0
	}
	defer comCall(fac, 2)
	var f6 uintptr
	if hr := comCall(fac, 0, uintptr(unsafe.Pointer(&iidDXGIFactory6)), uintptr(unsafe.Pointer(&f6))); hrOK(hr) && f6 != 0 {
		defer comCall(f6, 2)
		var ad uintptr
		// EnumAdapterByGpuPreference(0, DXGI_GPU_PREFERENCE_HIGH_PERFORMANCE)
		if hr := comCall(f6, 29, 0, 2, uintptr(unsafe.Pointer(&iidDXGIAdapter1)), uintptr(unsafe.Pointer(&ad))); hrOK(hr) && ad != 0 {
			var d dxgiAdapterDesc1
			if hrOK(comCall(ad, 10, uintptr(unsafe.Pointer(&d)))) && d.Flags&2 == 0 {
				return ad
			}
			comCall(ad, 2)
		}
	}
	var best uintptr
	var bestMem uintptr
	for i := uintptr(0); i < 16; i++ {
		var ad uintptr
		if hr := comCall(fac, 12, i, uintptr(unsafe.Pointer(&ad))); !hrOK(hr) || ad == 0 {
			break
		}
		var d dxgiAdapterDesc1
		if hrOK(comCall(ad, 10, uintptr(unsafe.Pointer(&d)))) && d.Flags&2 == 0 && d.VendorID != 0x1414 && (best == 0 || d.DedicatedVideoMemory > bestMem) {
			if best != 0 {
				comCall(best, 2)
			}
			best, bestMem = ad, d.DedicatedVideoMemory
			continue
		}
		comCall(ad, 2)
	}
	return best
}

// compilaShader: HLSL → codice della scheda video (ID3DBlob). Restituisce il blob (da liberare).
func compilaShader(entry, target string) (uintptr, error) {
	src := []byte(scenaHLSL)
	e, _ := windows.BytePtrFromString(entry)
	t, _ := windows.BytePtrFromString(target)
	var code, errs uintptr
	hr, _, _ := pD3DCompile.Call(uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)), 0, 0, 0,
		uintptr(unsafe.Pointer(e)), uintptr(unsafe.Pointer(t)), 1<<15 /* OPTIMIZATION_LEVEL3 */, 0,
		uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&errs)))
	if errs != 0 {
		comCall(errs, 2)
	}
	if !hrOK(hr) || code == 0 {
		return 0, fmt.Errorf("shader %s: 0x%x", entry, uint32(hr))
	}
	return code, nil
}

// scenaFPS disegna la scena per d (dopo 3 secondi di riscaldamento) e restituisce la durata di
// ogni fotogramma. onProg riceve l'avanzamento da 0 a 1.
func scenaFPS(ctx context.Context, d time.Duration, onProg func(float64)) (durate []time.Duration, err error) {
	defer func() {
		if r := recover(); r != nil {
			durate, err = nil, fmt.Errorf("%w: %v", errScenaNonParte, r)
		}
	}()
	if pD3D11CreateDev.Find() != nil || pD3DCompile.Find() != nil {
		return nil, errScenaNonParte
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ad := adattatorePotente()
	if ad == 0 {
		// solo la scheda di base di Windows (desktop remoto, driver mancante): lavorerebbe il
		// processore e gli FPS non vorrebbero dire niente
		return nil, fmt.Errorf("%w: nessuna scheda video vera", errScenaNonParte)
	}
	defer comCall(ad, 2)
	driver := uintptr(0)                         // con una scheda scelta: D3D_DRIVER_TYPE_UNKNOWN
	livelli := [3]uint32{0xb000, 0xa100, 0xa000} // 11_0, 10_1, 10_0
	var dev, ctxD uintptr
	var livello uint32
	hr, _, _ := pD3D11CreateDev.Call(ad, driver, 0, 0, uintptr(unsafe.Pointer(&livelli[0])), 3, 7,
		uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(&livello)), uintptr(unsafe.Pointer(&ctxD)))
	if !hrOK(hr) || dev == 0 || ctxD == 0 {
		return nil, fmt.Errorf("%w: D3D11CreateDevice 0x%x", errScenaNonParte, uint32(hr))
	}
	defer comCall(dev, 2)
	defer comCall(ctxD, 2)

	target := "ps_5_0"
	vtarget := "vs_5_0"
	if livello < 0xb000 {
		target, vtarget = "ps_4_0", "vs_4_0"
	}
	vsB, err := compilaShader("VS", vtarget)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errScenaNonParte, err)
	}
	defer comCall(vsB, 2)
	psB, err := compilaShader("PS", target)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errScenaNonParte, err)
	}
	defer comCall(psB, 2)
	var vs, ps uintptr
	if !hrOK(comCall(dev, 12, comCall(vsB, 3), comCall(vsB, 4), 0, uintptr(unsafe.Pointer(&vs)))) || vs == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(vs, 2)
	if !hrOK(comCall(dev, 15, comCall(psB, 3), comCall(psB, 4), 0, uintptr(unsafe.Pointer(&ps)))) || ps == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(ps, 2)

	// la tela: 1920×1080, fuori dallo schermo
	tex := struct {
		W, H, Mip, Array, Format, SCount, SQuality, Usage, Bind, CPU, Misc uint32
	}{1920, 1080, 1, 1, 28 /* R8G8B8A8_UNORM */, 1, 0, 0, 0x20 /* RENDER_TARGET */, 0, 0}
	var tela, rtv uintptr
	if !hrOK(comCall(dev, 5, uintptr(unsafe.Pointer(&tex)), 0, uintptr(unsafe.Pointer(&tela)))) || tela == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(tela, 2)
	if !hrOK(comCall(dev, 9, tela, 0, uintptr(unsafe.Pointer(&rtv)))) || rtv == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(rtv, 2)
	// il tempo della scena (per farla muovere)
	cbd := struct{ Size, Usage, Bind, CPU, Misc, Stride uint32 }{uint32(4 * len(scenaCB{}.v)), 0, 4 /* CONSTANT_BUFFER */, 0, 0, 0}
	var cb uintptr
	if !hrOK(comCall(dev, 3, uintptr(unsafe.Pointer(&cbd)), 0, uintptr(unsafe.Pointer(&cb)))) || cb == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(cb, 2)
	qd := struct{ Query, Misc uint32 }{0 /* EVENT */, 0}
	var q uintptr
	if !hrOK(comCall(dev, 24, uintptr(unsafe.Pointer(&qd)), uintptr(unsafe.Pointer(&q)))) || q == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(q, 2)

	// calibrazione: 1920×1080; su una scheda video debole (meno di 40 FPS nel riscaldamento della
	// prima volta) 1280×720, e se non basta 960×540. Da lì in poi sempre così: prima e dopo uguali.
	grado := int(scenaLivello.Load())
	vp := struct{ X, Y, W, H, Min, Max float32 }{0, 0, 1920, 1080, 0, 1}
	imposta := func() {
		vp.W, vp.H = scenaTele[grado][0], scenaTele[grado][1]
		comCall(ctxD, 44, 1, uintptr(unsafe.Pointer(&vp))) // RSSetViewports
	}
	comCall(ctxD, 33, 1, uintptr(unsafe.Pointer(&rtv)), 0) // OMSetRenderTargets
	imposta()
	comCall(ctxD, 24, 4)                                  // IASetPrimitiveTopology(TRIANGLELIST)
	comCall(ctxD, 11, vs, 0, 0)                           // VSSetShader
	comCall(ctxD, 9, ps, 0, 0)                            // PSSetShader
	comCall(ctxD, 16, 0, 1, uintptr(unsafe.Pointer(&cb))) // PSSetConstantBuffers

	var cbv scenaCB
	folla := nuovaFolla()
	fotogramma := func(n int) (time.Duration, error) {
		t0 := time.Now()
		folla.passo(&cbv, float32(n)/60, vp.W, vp.H)                          // il lavoro del processore: i personaggi e le luci
		comCall(ctxD, 48, cb, 0, 0, uintptr(unsafe.Pointer(&cbv.v[0])), 0, 0) // UpdateSubresource
		for i := 0; i < scenaPassate; i++ {
			comCall(ctxD, 13, 3, 0) // Draw(3, 0)
		}
		comCall(ctxD, 28, q) // End(query)
		for {
			r := comCall(ctxD, 29, q, 0, 0, 0) // GetData: S_OK quando la scheda ha finito
			if uint32(r) == 0 {
				break
			}
			if !hrOK(r) {
				return 0, fmt.Errorf("%w: scheda video persa (0x%x)", errScenaNonParte, uint32(r))
			}
			if time.Since(t0) > 3*time.Second {
				return 0, fmt.Errorf("%w: fotogramma troppo lento", errScenaNonParte)
			}
		}
		return time.Since(t0), nil
	}
	// riscaldamento: la scheda video sale di frequenza
	inizio := time.Now()
	n := 0
	// (la scelta allunga il riscaldamento di al massimo 2 secondi, solo la prima volta)
	scelta := !scenaScelta.Load()
	var somma time.Duration
	conta := 0
	for time.Since(inizio) < 3*time.Second || scelta && time.Since(inizio) < 5*time.Second {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		dt, err := fotogramma(n)
		if err != nil {
			return nil, err
		}
		n++
		if scelta && time.Since(inizio) > 500*time.Millisecond {
			somma += dt
			conta++
			if somma > time.Second {
				if float64(conta)/somma.Seconds() < 40 && grado < len(scenaTele)-1 {
					grado++
					imposta()
					somma, conta = 0, 0
				} else {
					scelta = false
				}
			}
		}
	}
	if !scenaScelta.Load() {
		scenaLivello.Store(int32(grado))
		scenaScelta.Store(true)
	}
	inizio = time.Now()
	ultimo := time.Now()
	for time.Since(inizio) < d {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		dt, err := fotogramma(n)
		if err != nil {
			return nil, err
		}
		durate = append(durate, dt)
		n++
		if onProg != nil && time.Since(ultimo) > 500*time.Millisecond {
			ultimo = time.Now()
			onProg(time.Since(inizio).Seconds() / d.Seconds())
		}
	}
	comCall(ctxD, 110) // ClearState
	return durate, nil
}
