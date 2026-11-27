#!/usr/bin/env python3
# Prova dell'interfaccia di Tasso Zip in Chrome headless, senza Windows.
# Crea prove/_pagina.html: la pagina vera (ui/index.html) con i caratteri, i testi e delle
# funzioni Go finte (dati di prova inventati). Lo scenario si sceglie con #nome nell'indirizzo.
# Uso:  python3 prove/simula.py && google-chrome --headless=new --window-size=1000,720 \
#         --screenshot=prove/x.png --virtual-time-budget=5000 "file://$PWD/prove/_pagina.html#esplora"
import json, os
qui = os.path.dirname(os.path.abspath(__file__))
radice = os.path.dirname(qui)
pagina = open(os.path.join(radice, "ui", "index.html"), encoding="utf-8").read()
caratteri = open(os.path.join(radice, "ui", "caratteri.css"), encoding="utf-8").read()
testi = json.load(open(os.path.join(radice, "testi", "testi.json"), encoding="utf-8"))

finto = r"""
<style>/* Chrome headless fotografa prima che finiscano le transizioni: per le prove le tolgo */
*,*::before,*::after{transition:none!important}</style>
<script>
const TESTI = __TESTI__;
const SCENA = (location.hash || "#home").slice(1).split("-");
window.__TZ_INIZIO__ = { modo: "", percorsi: [], mini: false };
const D = "C:\\Users\\Utente\\Downloads";
const ora = Date.parse("2026-11-24T18:30:00Z");
const giorni = g => new Date(ora - g * 86400000).toISOString();
const voce = (nome, dim, g, cat, ext, cartella) => ({ nome, percorso: D + "\\" + nome, cartella: !!cartella, dim, quando: giorni(g), cat: cartella ? "cartella" : cat, ext: ext || "", nascosto: false, archivio: cat === "archivio" });
const VOCI = [
  voce("Foto vacanze 2026", 0, 3, "", "", true), voce("Lavoro", 0, 12, "", "", true), voce("Progetti", 0, 40, "", "", true),
  voce("Foto vacanze 2026.zip", 248512000, 2, "archivio", "zip"), voce("Backup documenti.7z", 98304000, 5, "archivio", "7z"),
  voce("driver-stampante.rar", 15466496, 9, "archivio", "rar"), voce("sorgenti-sito.tar.gz", 4718592, 14, "archivio", "tar.gz"),
  voce("Relazione finale.pdf", 1887436, 1, "documento", "pdf"), voce("Preventivo cucina.docx", 356352, 6, "documento", "docx"),
  voce("Spese 2026.xlsx", 98304, 7, "documento", "xlsx"), voce("tramonto.jpg", 5452595, 2, "immagine", "jpg"),
  voce("compleanno nonna.mp4", 734003200, 20, "video", "mp4"), voce("podcast puntata 12.mp3", 52428800, 4, "audio", "mp3"),
  voce("installa-programma.exe", 73400320, 11, "eseguibile", "exe"), voce("note.txt", 2048, 0.2, "documento", "txt"),
  voce("licenza.key", 512, 30, "file", "key"),
];
const LUOGHI = [
  { chiave: "desktop", percorso: "C:\\Users\\Utente\\Desktop", tipo: "speciale" },
  { chiave: "download", percorso: D, tipo: "speciale" },
  { chiave: "documenti", percorso: "C:\\Users\\Utente\\Documents", tipo: "speciale" },
  { chiave: "immagini", percorso: "C:\\Users\\Utente\\Pictures", tipo: "speciale" },
  { chiave: "home", nome: "Utente", percorso: "C:\\Users\\Utente", tipo: "speciale" },
  { nome: "Windows", percorso: "C:\\", tipo: "fisso" }, { nome: "Dati", percorso: "D:\\", tipo: "fisso" },
  { nome: "", percorso: "E:\\", tipo: "rimovibile" },
];
const ELENCO = { tipo: "zip", dim: 248512000, metodo: "Deflate, Store", voci: [
  { nome: "Foto vacanze 2026/Sardegna/spiaggia 01.jpg", dim: 6291456, compresso: 6200000, quando: giorni(30) },
  { nome: "Foto vacanze 2026/Sardegna/spiaggia 02.jpg", dim: 5767168, compresso: 5700000, quando: giorni(30) },
  { nome: "Foto vacanze 2026/Sardegna", cartella: true, dim: 0, compresso: 0, quando: giorni(30) },
  { nome: "Foto vacanze 2026/Dolomiti/rifugio.jpg", dim: 4194304, compresso: 4100000, quando: giorni(25) },
  { nome: "Foto vacanze 2026/Dolomiti/video lago.mp4", dim: 157286400, compresso: 156000000, quando: giorni(25) },
  { nome: "Foto vacanze 2026/diario di viaggio.docx", dim: 1048576, compresso: 640000, quando: giorni(22) },
  { nome: "Foto vacanze 2026/itinerario.pdf", dim: 734003, compresso: 610000, quando: giorni(40) },
  { nome: "Foto vacanze 2026/spese.xlsx", dim: 45056, compresso: 30100, quando: giorni(21) },
  { nome: "Foto vacanze 2026/leggimi.txt", dim: 2048, compresso: 900, quando: giorni(21) },
]};
const pausa = ms => new Promise(r => setTimeout(r, ms));
window.goAvvio = async () => ({ lingua: SCENA[1] || "it", lingue: ["it","en","es","fr","de","pt","pl"], testi: TESTI, versione: "1.3.1", menu: true, assoc: false, formato: "zip", livello: "massima", desktop: "C:\\Users\\Utente\\Desktop", formati: ["zip","7z","tar","targz","tarxz","tarbz2","tarzst"], nome: SCENA[2] === "senza" ? "" : (SCENA[2] ? decodeURIComponent(SCENA[2]) : "Sergio") });
// "Il tuo nome": #home-it (Sergio), #home-it-senza (senza nome), #nome-it, #nomeerr-it
window.goNome = async () => S.nome || "";
window.goNomeSalva = async (n) => ({ ok: true, nome: n });
window.goLuoghi = async () => ({ luoghi: LUOGHI, iniziale: D, nascosti: false });
window.goEsplora = async (id, dir) => { setTimeout(() => tz.cartella({ id, codice: "", el: { percorso: dir || D, padre: "C:\\Users\\Utente", voci: VOCI, nascosti: 2, troppi: false } }), 20); return "ok"; };
window.goElenca = async (p) => { setTimeout(() => tz.elenco({ percorso: p, elenco: ELENCO }), 30); return "ok"; };
window.goInfo = async ps => ps.map(p => ({ percorso: p, nome: p.split("\\").pop(), cartella: !/\.[a-z0-9]+$/i.test(p), dim: 123456789, archivio: /\.(zip|7z|rar|gz)$/i.test(p), dove: D }));
window.goInfoPercorsi = async (id, ps) => { setTimeout(() => tz.info({ id, misura: { file: 1, cartelle: 0, byte: 248512000 }, archivio: { tipo: "zip", metodo: "Deflate, Store", solido: false, file: 8, cartelle: 3, orig: 175363063, comp: 173280000, dim: 248512000 } }), 20); return "ok"; };
for (const n of ["goScegliFile","goScegliCartella","goScegliArchivio","goScegliDestinazione"]) window[n] = async () => [];
for (const n of ["goLingua","goFormato","goLivello","goApri","goAnnulla","goChiudi","goFermaTrova","goTrova","goVerifica","goRipara","goApriFile","goApriVoce","goComprimi","goEstrai","goEstraiIn","goEstraiSelezione"]) window[n] = async () => "ok";
window.goCestino = async () => ({ ok: true, n: 1 });
window.goMenu = async v => ({ ok: true, on: v }); window.goAssocia = async v => ({ ok: true, on: v });

function trascina(sel, tipo) {
  const el = document.querySelector(sel); const b = el.getBoundingClientRect();
  const dt = new DataTransfer(); dt.items.add(new File(["x"], "foto.jpg"));
  const o = { bubbles: true, cancelable: true, dataTransfer: dt, clientX: b.left + b.width / 2, clientY: b.top + b.height / 2 };
  document.dispatchEvent(new DragEvent("dragenter", o));
  document.elementFromPoint(o.clientX, o.clientY).dispatchEvent(new DragEvent("dragover", o));
}
function clic(sel, opz) { const e = typeof sel === "string" ? document.querySelector(sel) : sel; const b = e.getBoundingClientRect();
  const o = Object.assign({ bubbles: true, button: 0, clientX: b.left + 20, clientY: b.top + b.height / 2 }, opz || {});
  e.dispatchEvent(new MouseEvent("mousedown", o)); e.dispatchEvent(new MouseEvent("mouseup", o)); e.dispatchEvent(new MouseEvent("click", o)); }
window.addEventListener("load", async () => {
  await pausa(150);
  const s = SCENA[0];
  if (s === "trascinaestrai") trascina('#v-home .card.e');
  if (s === "trascinacomprimi") trascina('#v-home .card.c');
  if (s === "esplora" || s === "ctx" || s === "info" || s === "trova") {
    document.getElementById("b-esplora").click(); await pausa(200);
    const righe = document.querySelectorAll("#x-corpo .r");
    clic(righe[3]); clic(righe[7], { ctrlKey: true });
    if (s === "ctx") { clic(righe[3]); const b = righe[3].getBoundingClientRect(); righe[3].dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, clientX: b.left + 160, clientY: b.top + 14 })); }
    if (s === "info") { clic(righe[3]); azione("info"); }
    if (s === "trova") { tz.trovati && (X.id++, X.trova = { voci: [] }, document.getElementById("x-banner").hidden = false, tz.trovati({ id: X.id, ris: { cerca: "foto", voci: VOCI.filter(v => /foto/i.test(v.nome)).map(v => Object.assign({}, v, { dove: "Lavoro\\2026" })), troppi: false } })); }
  }
  if (s === "archivio") { apriArchivio(D + "\\Foto vacanze 2026.zip"); await pausa(150); document.querySelectorAll("#a-corpo .r")[0].dispatchEvent(new MouseEvent("dblclick", { bubbles: true })); }
  if (s === "guida") { document.getElementById("b-guida").click(); await pausa(50); document.querySelector('.opz[data-g="comprimi"]').click(); await guidaAggiungi([D + "\\Foto vacanze 2026", D + "\\Relazione finale.pdf"]); }
  if (s === "guida3") { document.getElementById("b-guida").click(); await pausa(50); document.querySelector('.opz[data-g="comprimi"]').click(); await guidaAggiungi([D + "\\Foto vacanze 2026"]); document.getElementById("g-avanti").click(); G.formato = "7z"; disegnaGuida(); }
  if (s === "guida3e") { document.getElementById("b-guida").click(); await pausa(50); document.querySelector('.opz[data-g="estrai"]').click(); await guidaAggiungi([D + "\\Backup documenti.7z"]); document.getElementById("g-avanti").click(); }
  if (s === "guida1") { document.getElementById("b-guida").click(); }
  if (s === "comprimi") { await aggiungi([D + "\\Foto vacanze 2026", D + "\\Relazione finale.pdf", D + "\\Spese 2026.xlsx"]); }
  if (s === "lavoro") { S.tipo = "comprimi"; inizioLavoro(t("lavoro_comprimo") || "", "Foto vacanze 2026\\Dolomiti\\video lago.mp4"); tz.avanza({ fatto: 62, totale: 100, nome: "Foto vacanze 2026\\Dolomiti\\video lago.mp4" }); }
  if (s === "fatto") { mostraFatto({ tipo: "comprimi", file: 8, originale: 175363063, compresso: 98304000, percorso: D + "\\Foto vacanze 2026.7z", saltati: [] }, 0); }
  if (s === "errore") { mostraErrore({ codice: "generico", percorso: D + "\\driver-stampante.rar", dettagli: "unexpected EOF" }); }
  if (s === "password") { S.corrente = D + "\\Backup documenti.7z"; chiediPassword(true); }
  if (s === "nome" || s === "nomeerr") { document.getElementById("nomeBtn").click(); await pausa(80); if (s === "nomeerr") { const i = document.getElementById("nomeIn"); i.value = "Sergio★"; i.dispatchEvent(new Event("input")); } }
  if (s === "avviso") { document.querySelector('#v-home .card.e').dataset.zona; S.dropDove = "estrai"; tz.trascinati([D + "\\note.txt"]); }
});
</script>
"""
finto = finto.replace("__TESTI__", json.dumps(testi, ensure_ascii=False))
out = pagina.replace("/*CARATTERI*/", caratteri, 1).replace("<script>", finto + "<script>", 1)
open(os.path.join(qui, "_pagina.html"), "w", encoding="utf-8").write(out)
print("ok", os.path.join(qui, "_pagina.html"))
