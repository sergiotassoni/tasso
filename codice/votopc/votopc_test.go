package votopc

import "testing"

func TestVotoGPUTabella(t *testing.T) {
	casi := []struct {
		nome string
		vram uint64
		min  int
		max  int
	}{
		{"NVIDIA GeForce GTX 1660 Ti", 6144, 58, 62},
		{"NVIDIA GeForce GTX 1660 Ti with Max-Q Design", 6144, 48, 53},
		{"NVIDIA GeForce RTX 3060", 12288, 70, 74},
		{"NVIDIA GeForce RTX 3060 Laptop GPU", 6144, 63, 67},
		{"NVIDIA GeForce RTX 4070", 12282, 84, 88},
		{"NVIDIA GeForce RTX 4070 Ti SUPER", 16376, 89, 93},
		{"NVIDIA GeForce RTX 4090", 24564, 100, 100},
		{"AMD Radeon RX 6600", 8176, 64, 68},
		{"AMD Radeon RX 6600M", 8176, 58, 62},
		{"AMD Radeon RX 7900 XTX", 24560, 93, 97},
		{"AMD Radeon RX 580 2048SP", 4096, 46, 50},
		{"AMD Radeon(TM) Vega 10 Graphics", 512, 16, 20},
		{"AMD Radeon(TM) Graphics", 512, 20, 24},
		{"AMD Radeon 780M Graphics", 512, 36, 40},
		{"Intel(R) UHD Graphics 620", 128, 10, 14},
		{"Intel(R) Iris(R) Xe Graphics", 128, 24, 28},
		{"Intel(R) Arc(TM) A770 Graphics", 16256, 68, 72},
		{"Intel(R) Arc(TM) 140V GPU (16GB)", 128, 38, 42},
		{"Intel(R) HD Graphics 4000", 64, 7, 11},
		{"NVIDIA GeForce MX450", 2048, 22, 26},
		{"NVIDIA GeForce GTX 980M", 4096, 38, 42},
		{"NVIDIA GeForce GTX 960M", 2048, 20, 24},
		{"NVIDIA GeForce RTX 2050", 4096, 38, 42},
		{"NVIDIA GeForce RTX 4090 Laptop GPU", 16376, 86, 90},
		{"NVIDIA GeForce RTX 3080 Laptop GPU", 16384, 75, 79},
		{"Intel(R) Arc(TM) A730M Graphics", 12288, 58, 62},
		{"Intel(R) Arc(TM) A550M Graphics", 8192, 46, 50},
		{"Intel(R) Arc(TM) A370M Graphics", 4096, 32, 36},
		{"AMD Radeon RX 6500M", 4096, 36, 40},
		{"AMD Radeon RX 6550M", 4096, 38, 42},
		{"AMD Radeon RX 6850M XT", 12288, 76, 76},
		{"AMD Radeon RX 6650M XT", 8192, 64, 64},
		{"AMD Radeon RX 6650M", 8192, 60, 60},
		{"AMD Radeon RX 7600M XT", 8192, 66, 66},
		{"AMD Radeon RX 7900 XT", 20464, 91, 91},
		{"AMD Radeon RX 7900M", 16384, 78, 82},
	}
	for _, c := range casi {
		v, noto := VotoGPU(c.nome, c.vram)
		if !noto || v < c.min || v > c.max {
			t.Errorf("%q: voto %d (noto %v), atteso %d-%d", c.nome, v, noto, c.min, c.max)
		}
	}
}

func TestVotoGPUSconosciutaDallaMemoria(t *testing.T) {
	v, noto := VotoGPU("NVIDIA RTX A4000", 16384)
	if noto || v < 70 || v > 78 {
		t.Errorf("scheda sconosciuta da 16 GB: %d (noto %v)", v, noto)
	}
	if v, _ := VotoGPU("Scheda strana", 0); v < 1 || v > 15 {
		t.Errorf("senza memoria video deve avere un voto basso, non %d", v)
	}
	if v, _ := VotoGPU("", 8192); v != 0 {
		t.Errorf("senza nome niente voto, non %d", v)
	}
}

func TestSceltaDedicataSullIntegrata(t *testing.T) {
	schede := []Scheda{
		{Nome: "Intel(R) UHD Graphics 630", VRAMMB: 128, Vendor: 0x8086},
		{Nome: "NVIDIA GeForce GTX 1660 Ti", VRAMMB: 6144, Vendor: 0x10DE},
		{Nome: "Microsoft Basic Render Driver", Vendor: 0x1414, Software: true},
	}
	s, v, _, ok := SceltaGPU(schede, false)
	if !ok || s.Vendor != 0x10DE || v != 60 {
		t.Fatalf("doveva scegliere la GTX 1660 Ti, ha scelto %q (%d)", s.Nome, v)
	}
	// Ordine diverso, stesso risultato.
	s, _, _, _ = SceltaGPU([]Scheda{schede[1], schede[0]}, false)
	if s.Vendor != 0x10DE {
		t.Fatalf("l'ordine non deve contare: %q", s.Nome)
	}
	// Solo adattatori finti: nessuna scheda.
	if _, _, _, ok := SceltaGPU([]Scheda{schede[2], {Nome: "Parsec Virtual Display Adapter"}}, false); ok {
		t.Fatal("gli adattatori finti non sono schede video")
	}
	// AMD integrata + AMD dedicata.
	s, _, _, _ = SceltaGPU([]Scheda{{Nome: "AMD Radeon(TM) Graphics", VRAMMB: 512, Vendor: 0x1002}, {Nome: "AMD Radeon RX 6600M", VRAMMB: 8176, Vendor: 0x1002}}, false)
	if s.Nome != "AMD Radeon RX 6600M" {
		t.Fatalf("doveva scegliere la RX 6600M, ha scelto %q", s.Nome)
	}
}

func TestPesoNelTotale(t *testing.T) {
	// PC da gioco medio: tutto 70, leggerezza 80 → come con la formula 1 (0.6*70+0.4*80 = 74).
	medio := Parametri{Processore: 70, SchedaVideo: 70, RAM: 70, Disco: 70, Leggerezza: 80}
	if got := Totale(medio); got != 74 {
		t.Errorf("PC medio: %d, atteso 74", got)
	}
	// Senza scheda video (non letta) il peso va sugli altri: stesso voto.
	senza := medio
	senza.SchedaVideo = 0
	if Totale(senza) != 74 {
		t.Errorf("senza scheda video il totale non deve cambiare: %d", Totale(senza))
	}
	// Una scheda video forte alza il totale, una integrata lo abbassa, ma con misura:
	// la scheda pesa il 25% della potenza, cioè il 15% del totale.
	forte, debole := medio, medio
	forte.SchedaVideo, debole.SchedaVideo = 100, 12
	tf, td := Totale(forte), Totale(debole)
	if tf <= 74 || tf > 79 {
		t.Errorf("scheda forte: %d", tf)
	}
	if td >= 74 || td < 64 {
		t.Errorf("scheda integrata: %d", td)
	}
	// PC da ufficio normale (integrata Intel UHD 620): resta nella sua fascia.
	ufficio := Parametri{Processore: 50, SchedaVideo: 12, RAM: 52, Disco: 60, Leggerezza: 70}
	if c := Classifica(Totale(ufficio)); c != "ufficio" {
		t.Errorf("PC da ufficio: %d %s", Totale(ufficio), c)
	}
	// Niente misurato: niente voto.
	if Totale(Parametri{Leggerezza: 90}) != 0 {
		t.Error("senza componenti misurati non c'è voto")
	}
}

func TestPunti100(t *testing.T) {
	for p, v := range map[int]int{1000: 70, 2000: 95, 500: 45, 0: 1, 100000: 100} {
		if got := Punti100(p); got != v {
			t.Errorf("Punti100(%d) = %d, atteso %d", p, got, v)
		}
	}
	if PuntiProcessore(1000, 4000) != 2000 {
		t.Error("media geometrica del processore sbagliata")
	}
	if TipoDisco(1200) != "nvme" || TipoDisco(450) != "ssd" || TipoDisco(40) != "hdd" || TipoDisco(0) != "" {
		t.Error("tipo di disco sbagliato")
	}
}

func TestVotoRAM(t *testing.T) {
	if VotoCapacita(8) != 40 || VotoCapacita(16) != 65 || VotoCapacita(32) != 85 || VotoCapacita(64) != 100 || VotoCapacita(0) != 0 {
		t.Errorf("capacità: %d %d %d %d", VotoCapacita(8), VotoCapacita(16), VotoCapacita(32), VotoCapacita(64))
	}
	if v := VotoConfigurazione(3200, 2); v != 70 {
		t.Errorf("DDR4-3200 doppio canale: %d", v)
	}
	if a, b := VotoConfigurazione(6000, 2), VotoConfigurazione(2666, 1); a < 90 || b > 40 {
		t.Errorf("DDR5-6000 x2 = %d, DDR4-2666 x1 = %d", a, b)
	}
	if Canali(1) != 1 || Canali(2) != 2 || Canali(4) != 2 || Canali(0) != 0 {
		t.Error("canali sbagliati")
	}
	// Senza dati di Windows conta solo la misura.
	if VotoRAM(66, 0, 0) != 66 {
		t.Error("solo misura")
	}
	if v := VotoRAM(70, 65, 70); v < 68 || v > 70 {
		t.Errorf("RAM media: %d", v)
	}
}

func TestVotoRaffreddamento(t *testing.T) {
	for c, v := range map[float64]int{0: 0, 55: 100, 70: 88, 80: 70, 85: 58, 100: 15, 110: 10, 150: 0} {
		if got := VotoRaffreddamento(c); got != v {
			t.Errorf("%v °C: %d, atteso %d", c, got, v)
		}
	}
	// Il raffreddamento non entra nel totale.
	p := Parametri{Processore: 70, SchedaVideo: 70, RAM: 70, Disco: 70, Leggerezza: 80}
	q := p
	q.Raffreddamento = 15
	if Totale(p) != Totale(q) {
		t.Error("il raffreddamento non deve cambiare il voto totale")
	}
}

func TestPortatileConNomeDaDesktop(t *testing.T) {
	// Stesso nome della desktop: sul portatile 4 punti in meno.
	if v, _ := VotoGPUPC("NVIDIA GeForce GTX 1660 Ti", 6144, true); v != 56 {
		t.Errorf("GTX 1660 Ti su portatile: %d, atteso 56", v)
	}
	if v, _ := VotoGPUPC("NVIDIA GeForce RTX 2070", 8192, true); v != 68 {
		t.Errorf("RTX 2070 su portatile: %d, atteso 68", v)
	}
	// Le RTX 30/40 hanno già "Laptop GPU": niente doppia riduzione.
	a, _ := VotoGPUPC("NVIDIA GeForce RTX 3060 Laptop GPU", 6144, true)
	b, _ := VotoGPUPC("NVIDIA GeForce RTX 3060 Laptop GPU", 6144, false)
	if a != b || a != 66 {
		t.Errorf("RTX 3060 Laptop: %d / %d", a, b)
	}
	// Una desktop RTX 4070 su un PC fisso resta 86; le integrate non cambiano.
	if v, _ := VotoGPUPC("NVIDIA GeForce RTX 4070", 12282, false); v != 86 {
		t.Errorf("RTX 4070 fissa: %d", v)
	}
	if v, _ := VotoGPUPC("Intel(R) UHD Graphics 620", 128, true); v != 12 {
		t.Errorf("UHD 620 su portatile: %d", v)
	}
	s, v, _, _ := SceltaGPU([]Scheda{{Nome: "Intel(R) UHD Graphics 630", VRAMMB: 128}, {Nome: "NVIDIA GeForce GTX 1660 Ti", VRAMMB: 6144}}, true)
	if s.Nome != "NVIDIA GeForce GTX 1660 Ti" || v != 56 {
		t.Errorf("portatile con Optimus: %q %d", s.Nome, v)
	}
}

func TestTipoRAM(t *testing.T) {
	casi := []struct {
		cod, mts int
		tipo     string
		saldata  bool
	}{{26, 3200, "DDR4", false}, {34, 6000, "DDR5", false}, {24, 1600, "DDR3", false}, {29, 2133, "LPDDR3", true},
		{30, 3200, "LPDDR4", true}, {30, 4267, "LPDDR4X", true}, {35, 6400, "LPDDR5", true}, {35, 7467, "LPDDR5X", true}, {0, 0, "", false}}
	for _, c := range casi {
		if tp, sd := TipoRAM(c.cod, c.mts); tp != c.tipo || sd != c.saldata {
			t.Errorf("TipoRAM(%d, %d) = %q %v", c.cod, c.mts, tp, sd)
		}
	}
}

func TestNienteDoppiaRiduzione(t *testing.T) {
	casi := []struct {
		nome string
		v    int
	}{
		{"NVIDIA GeForce RTX 2050", 40},       // solo da portatile: il 40 è già il valore da portatile
		{"NVIDIA GeForce GTX 1650 Ti", 50},    // solo da portatile
		{"NVIDIA GeForce GTX 1660 SUPER", 60}, // solo da PC fisso: su un portatile è una scheda esterna
		{"NVIDIA GeForce RTX 2060 SUPER", 70}, // solo da PC fisso
		{"NVIDIA GeForce GTX 1080 Ti", 70},    // solo da PC fisso
		{"NVIDIA GeForce GTX 1070 Ti", 62},    // solo da PC fisso
		{"NVIDIA GeForce GTX 1060", 46},       // nome uguale alla desktop: ridotta
		{"NVIDIA GeForce RTX 2080", 73},       // nome uguale alla desktop: ridotta
	}
	for _, c := range casi {
		if v, _ := VotoGPUPC(c.nome, 4096, true); v != c.v {
			t.Errorf("%q su portatile: %d, atteso %d", c.nome, v, c.v)
		}
	}
}

func TestPortatileOFisso(t *testing.T) {
	casi := []struct {
		nome     string
		tipo     int
		telai    []int
		batterie []string
		atteso   bool
	}{
		{"PC fisso con UPS in USB", 1, []int{3}, []string{"Back-UPS ES 700 FW:871.O4 USB"}, false},
		{"PC fisso con UPS, BIOS senza dati", 0, []int{2}, []string{"UPS"}, false},
		{"PC fisso con telaio 18 o 21", 1, []int{3, 21}, nil, false},
		{"PC fisso con batteria strana ma telaio da fisso", 1, []int{3}, []string{"Batteria"}, false},
		{"portatile dal tipo di PC", 2, []int{3}, nil, true},
		{"portatile dal telaio", 0, []int{10}, nil, true},
		{"convertibile", 1, []int{31}, nil, true},
		{"portatile senza dati se non la batteria", 0, []int{2}, []string{"DELL 7FHHV"}, true},
		{"nessun dato", 0, nil, nil, false},
	}
	for _, c := range casi {
		if got := Portatile(c.tipo, c.telai, c.batterie); got != c.atteso {
			t.Errorf("%s: %v, atteso %v", c.nome, got, c.atteso)
		}
	}
}

func TestTipoDisco(t *testing.T) {
	for _, c := range []struct{ bus, media, atteso string }{
		{"17", "4", "nvme"}, {"NVMe", "SSD", "nvme"}, {"11", "4", "ssd"}, {"SATA", "HDD", "hdd"}, {"7", "3", "hdd"},
		{"8", "4", ""}, {"RAID", "SSD", ""}, {"0", "4", ""}, {"11", "0", ""}, {"", "", ""},
	} {
		if got := TipoDiscoWindows(c.bus, c.media); got != c.atteso {
			t.Errorf("TipoDiscoWindows(%q, %q) = %q, atteso %q", c.bus, c.media, got, c.atteso)
		}
	}
	// Ripiego: se Windows non è sicuro (RAID/RST/VMD) vale la stima, segnata come stimata.
	if tp, st := DiscoFinale("", 1500); tp != "nvme" || !st {
		t.Errorf("ripiego NVMe: %q %v", tp, st)
	}
	if tp, st := DiscoFinale("", 0); tp != "" || st {
		t.Errorf("disco saltato: %q %v", tp, st)
	}
	if tp, st := DiscoFinale("ssd", 1500); tp != "ssd" || st {
		t.Errorf("detto da Windows: %q %v", tp, st)
	}
}

func TestPulisciNome(t *testing.T) {
	for in, out := range map[string]string{
		"ASUSTeK COMPUTER INC. ROG STRIX B550-F":        "ASUSTeK COMPUTER INC. ROG STRIX B550-F",
		"ASUSTeK Default string":                        "ASUSTeK",
		"To be filled by O.E.M. To be filled by O.E.M.": "",
		"System manufacturer System Product Name":       "",
		"Gigabyte Technology Co., Ltd. B450M DS3H":      "Gigabyte Technology Co., Ltd. B450M DS3H",
		"OEM":                           "",
		"Default string Default string": "",
		"  ":                            "",
	} {
		if got := PulisciNome(in); got != out {
			t.Errorf("PulisciNome(%q) = %q, atteso %q", in, got, out)
		}
	}
}

func TestLPDDR5XDalProcessore(t *testing.T) {
	for _, c := range []struct {
		mts  int
		cpu  string
		tipo string
	}{
		{6400, "Intel(R) Core(TM) Ultra 7 155H", "LPDDR5X"},
		{6400, "AMD Ryzen 7 7840U w/ Radeon 780M Graphics", "LPDDR5X"},
		{6400, "AMD Ryzen AI 9 HX 370 w/ Radeon 890M", "LPDDR5X"},
		{6400, "AMD Ryzen 7 6800U with Radeon Graphics", "LPDDR5"},
		{6400, "", "LPDDR5"},
		{5500, "Intel(R) Core(TM) Ultra 7 155H", "LPDDR5"},
		{7467, "", "LPDDR5X"},
	} {
		if tp, _ := TipoRAMCPU(35, c.mts, c.cpu); tp != c.tipo {
			t.Errorf("%d MT/s con %q: %q, atteso %q", c.mts, c.cpu, tp, c.tipo)
		}
	}
	if tp, _ := TipoRAMCPU(26, 3200, "Intel(R) Core(TM) Ultra 7 155H"); tp != "DDR4" {
		t.Error("la DDR4 non cambia")
	}
}
