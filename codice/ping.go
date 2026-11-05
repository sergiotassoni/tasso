package main

// Ping ai server dei giochi: misura ritardo, stabilità (jitter) e pacchetti persi verso i server
// veri dove girano i giochi più usati. Onestà: i giochi non pubblicano l'indirizzo di ogni partita,
// quindi Tasso misura i data center (Riot Direct, relay di Valve, Amazon AWS): il ping in partita
// è di solito uguale o di pochi ms diverso.
//   - Riot (League of Legends, TFT, Valorant): indirizzi pubblici di Riot Direct, rispondono al ping.
//   - Valve (CS2, Dota 2): l'elenco ufficiale dei relay arriva da Steam (GetSDRConfig), sempre aggiornato.
//   - Amazon AWS (Fortnite e tanti altri giochi online): tempo di connessione al data center della regione.
//   - Cloudflare e Google: per capire se il ritardo è già nella tua rete o nel tuo operatore.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type pingTarget struct {
	Group  string // riot, valve, aws, base
	Region string // eu, na, sa, asia
	Name   string // nome per le persone
	Host   string // IP o nome
	TCP    bool   // misura con connessione TCP 443 invece del ping ICMP
}

var pingGroups = []map[string]string{
	{"id": "riot", "name": "League of Legends · TFT · Valorant", "note": "Server Riot Direct. Valorant usa la stessa rete: per Valorant il valore è indicativo."},
	{"id": "valve", "name": "Counter-Strike 2 · Dota 2", "note": "Relay ufficiali di Valve, elenco preso da Steam. In CS2 puoi limitare il ping massimo in Impostazioni > Gioco."},
	{"id": "aws", "name": "Fortnite e altri giochi online", "note": "Data center Amazon AWS, dove girano Fortnite e molti altri giochi. Misurato come tempo di connessione."},
	{"id": "base", "name": "La tua connessione", "note": "Server vicinissimi a tutti: se qui il ping è alto, il problema è la tua rete o il tuo operatore, non il gioco."},
}

var riotTargets = []pingTarget{
	{"riot", "eu", "EU West (Amsterdam)", "104.160.141.3", false},
	{"riot", "eu", "EU Nordic & East", "104.160.142.3", false},
	{"riot", "na", "Nord America (Chicago)", "104.160.131.3", false},
	{"riot", "na", "America Latina Nord (Miami)", "104.160.136.3", false},
	{"riot", "sa", "Brasile (San Paolo)", "104.160.152.3", false},
	{"riot", "asia", "Oceania (Sydney)", "104.160.156.1", false},
}

var awsRegions = []struct{ region, code, name string }{
	{"eu", "eu-south-1", "Milano"}, {"eu", "eu-central-1", "Francoforte"}, {"eu", "eu-west-3", "Parigi"},
	{"eu", "eu-west-2", "Londra"}, {"eu", "eu-west-1", "Irlanda"}, {"eu", "eu-north-1", "Stoccolma"},
	{"na", "us-east-1", "Virginia"}, {"na", "us-east-2", "Ohio"}, {"na", "us-west-2", "Oregon"},
	{"sa", "sa-east-1", "San Paolo"},
	{"asia", "ap-northeast-1", "Tokyo"}, {"asia", "ap-southeast-1", "Singapore"}, {"asia", "ap-southeast-2", "Sydney"},
}

// Relay Valve: codice della città -> regione e nome in italiano.
var valvePops = map[string][2]string{
	"fra": {"eu", "Francoforte"}, "ams": {"eu", "Amsterdam"}, "vie": {"eu", "Vienna"}, "waw": {"eu", "Varsavia"},
	"sto": {"eu", "Stoccolma"}, "mad": {"eu", "Madrid"}, "par": {"eu", "Parigi"}, "lhr": {"eu", "Londra"},
	"hel": {"eu", "Helsinki"},
	"iad": {"na", "Washington"}, "ord": {"na", "Chicago"}, "atl": {"na", "Atlanta"}, "lax": {"na", "Los Angeles"},
	"sea": {"na", "Seattle"}, "dfw": {"na", "Dallas"},
	"gru": {"sa", "San Paolo"}, "scl": {"sa", "Santiago"}, "lim": {"sa", "Lima"},
	"sgp": {"asia", "Singapore"}, "tyo": {"asia", "Tokyo"}, "hkg": {"asia", "Hong Kong"}, "syd": {"asia", "Sydney"},
	"bom": {"asia", "Mumbai"},
}

var pingRegions = map[string]bool{"eu": true, "na": true, "sa": true, "asia": true}

// valveTargets legge da Steam l'elenco ufficiale dei relay (cambia nel tempo, quindi niente indirizzi fissi).
func valveTargets(region string) ([]pingTarget, error) {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get("https://api.steampowered.com/ISteamApps/GetSDRConfig/v1/?appid=730")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Steam ha risposto %d", resp.StatusCode)
	}
	var cfg struct {
		Pops map[string]struct {
			Relays []struct {
				IPv4 string `json:"ipv4"`
			} `json:"relays"`
		} `json:"pops"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&cfg); err != nil {
		return nil, err
	}
	var out []pingTarget
	for code, info := range valvePops {
		if info[0] != region {
			continue
		}
		p, ok := cfg.Pops[code]
		if !ok || len(p.Relays) == 0 || net.ParseIP(p.Relays[0].IPv4) == nil {
			continue
		}
		out = append(out, pingTarget{"valve", region, info[1], p.Relays[0].IPv4, false})
	}
	return out, nil
}

func pingTargetsFor(region string) ([]pingTarget, string) {
	var t []pingTarget
	for _, x := range riotTargets {
		if x.Region == region {
			t = append(t, x)
		}
	}
	valveNote := ""
	if v, err := valveTargets(region); err == nil {
		t = append(t, v...)
	} else {
		valveNote = "Non riesco a leggere da Steam l'elenco dei server Valve: CS2 e Dota 2 saltati."
	}
	for _, r := range awsRegions {
		if r.region == region {
			t = append(t, pingTarget{"aws", region, r.name, "dynamodb." + r.code + ".amazonaws.com", true})
		}
	}
	t = append(t,
		pingTarget{"base", region, "Cloudflare", "1.1.1.1", false},
		pingTarget{"base", region, "Google", "8.8.8.8", false})
	return t, valveNote
}

// ---------------- Misura ----------------

var (
	iphlpapi        = windows.NewLazySystemDLL("iphlpapi.dll")
	pIcmpCreateFile = iphlpapi.NewProc("IcmpCreateFile")
	pIcmpSendEcho   = iphlpapi.NewProc("IcmpSendEcho")
	pIcmpClose      = iphlpapi.NewProc("IcmpCloseHandle")
)

// icmpEcho: un ping vero con l'API di Windows (non serve essere amministratore). -1 = nessuna risposta.
func icmpEcho(ip net.IP, timeout time.Duration) float64 {
	ip4 := ip.To4()
	if ip4 == nil {
		return -1
	}
	h, _, _ := pIcmpCreateFile.Call()
	if h == 0 || h == uintptr(windows.InvalidHandle) {
		return -1
	}
	defer pIcmpClose.Call(h)
	addr := *(*uint32)(unsafe.Pointer(&ip4[0])) // già nell'ordine di rete
	data := []byte("Tasso-ping-32-byte-di-prova-ok!!")
	reply := make([]byte, 256)
	start := time.Now()
	n, _, _ := pIcmpSendEcho.Call(h, uintptr(addr), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	if n == 0 {
		return -1
	}
	// ICMP_ECHO_REPLY: Address, Status, RoundTripTime (uint32 ciascuno).
	status := *(*uint32)(unsafe.Pointer(&reply[4]))
	if status != 0 {
		return -1
	}
	rtt := float64(*(*uint32)(unsafe.Pointer(&reply[8])))
	if rtt == 0 { // Windows arrotonda al millisecondo: sotto 1 ms uso il tempo misurato
		rtt = math.Min(float64(time.Since(start).Microseconds())/1000, 1)
	}
	return rtt
}

func tcpConnect(addr string, timeout time.Duration) float64 {
	start := time.Now()
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return -1
	}
	ms := float64(time.Since(start).Microseconds()) / 1000
	c.Close()
	return ms
}

type pingResult struct {
	Group  string  `json:"group"`
	Name   string  `json:"name"`
	Host   string  `json:"host"`
	Avg    float64 `json:"avg"`
	Min    float64 `json:"min"`
	Jitter float64 `json:"jitter"`
	Loss   int     `json:"loss"` // percentuale
	OK     bool    `json:"ok"`
	Color  string  `json:"color"` // green, yellow, red
	TCP    bool    `json:"tcp"`
}

func measure(t pingTarget) pingResult {
	r := pingResult{Group: t.Group, Name: t.Name, Host: t.Host, TCP: t.TCP}
	ips, err := net.LookupIP(t.Host) // il nome si risolve prima: il tempo del DNS non conta
	var ip net.IP
	for _, x := range ips {
		if x.To4() != nil {
			ip = x
			break
		}
	}
	if err != nil || ip == nil {
		r.Loss, r.Color = 100, "red"
		return r
	}
	const n = 8
	var got []float64
	for i := 0; i < n; i++ {
		var ms float64
		if t.TCP {
			ms = tcpConnect(net.JoinHostPort(ip.String(), "443"), 2*time.Second)
		} else {
			ms = icmpEcho(ip, 2*time.Second)
		}
		if ms >= 0 {
			got = append(got, ms)
		}
		time.Sleep(120 * time.Millisecond)
	}
	r.Loss = (n - len(got)) * 100 / n
	if len(got) == 0 {
		r.Color = "red"
		return r
	}
	r.OK = true
	sum, mn := 0.0, got[0]
	for _, v := range got {
		sum += v
		mn = math.Min(mn, v)
	}
	r.Avg, r.Min = math.Round(sum/float64(len(got))), math.Round(mn)
	j := 0.0
	for i := 1; i < len(got); i++ {
		j += math.Abs(got[i] - got[i-1])
	}
	if len(got) > 1 {
		r.Jitter = math.Round(j / float64(len(got)-1))
	}
	switch {
	case r.Avg <= 45 && r.Loss == 0 && r.Jitter <= 10:
		r.Color = "green"
	case r.Avg <= 90 && r.Loss <= 13 && r.Jitter <= 25:
		r.Color = "yellow"
	default:
		r.Color = "red"
	}
	return r
}

// gamePing misura tutto in parallelo (al massimo 6 alla volta, per non falsare i risultati).
func gamePing(region string) {
	if !pingRegions[region] {
		region = "eu"
	}
	updateSettings(func(s *Settings) { s.PingRegion = region })
	targets, valveNote := pingTargetsFor(region)
	emit("onPingProgress", fmt.Sprintf("Misuro %d server…", len(targets)))
	res := make([]pingResult, len(targets))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t pingTarget) {
			defer guard("ping.go:268")
			defer wg.Done()
			sem <- struct{}{}
			res[i] = measure(t)
			<-sem
		}(i, t)
	}
	wg.Wait()
	rank := map[string]int{"riot": 0, "valve": 1, "aws": 2, "base": 3}
	sort.SliceStable(res, func(a, b int) bool {
		if res[a].Group != res[b].Group {
			return rank[res[a].Group] < rank[res[b].Group]
		}
		if res[a].OK != res[b].OK {
			return res[a].OK
		}
		return res[a].Avg < res[b].Avg
	})
	best := map[string]pingResult{}
	for _, r := range res {
		if !r.OK || r.Loss > 13 || r.Group == "base" {
			continue
		}
		if b, ok := best[r.Group]; !ok || r.Avg+r.Jitter < b.Avg+b.Jitter {
			best[r.Group] = r
		}
	}
	tips := pingTips(res)
	if valveNote != "" {
		tips = append([]string{valveNote}, tips...)
	}
	emit("onPing", map[string]interface{}{"region": region, "groups": pingGroups, "results": res, "best": best, "tips": tips})
}

func pingTips(res []pingResult) []string {
	var tips []string
	baseBad, jitterBad, lossBad, anyOK := false, false, false, false
	for _, r := range res {
		if r.OK {
			anyOK = true
		}
		if r.Group == "base" && r.OK && r.Avg > 35 {
			baseBad = true
		}
		if r.OK && r.Jitter > 15 {
			jitterBad = true
		}
		if r.OK && r.Loss > 0 {
			lossBad = true
		}
	}
	if !anyOK {
		return []string{"Nessun server ha risposto: controlla di essere connesso a internet. Alcuni firewall o antivirus bloccano il ping."}
	}
	if baseBad {
		tips = append(tips, "Il ritardo è alto già verso Cloudflare e Google: il problema è nella tua rete o nel tuo operatore. Prova col cavo, riavvia il modem o chiudi download e streaming sugli altri dispositivi.")
	}
	if jitterBad {
		tips = append(tips, "Il ping balla parecchio (jitter alto): succede spesso col Wi-Fi o quando qualcuno in casa scarica. Il cavo di rete è la soluzione migliore.")
	}
	if lossBad {
		tips = append(tips, "Si perde qualche pacchetto: nei giochi si sente come teletrasporti o colpi che non contano. Prova col cavo e, se resta, segnalalo al tuo operatore.")
	}
	if len(tips) == 0 {
		tips = append(tips, "Connessione stabile: nessun pacchetto perso e ping regolare.")
	}
	return tips
}
