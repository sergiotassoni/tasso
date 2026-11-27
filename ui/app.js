// Tasso PDF - l'interfaccia.
// Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later.

import { LINGUE, TESTI } from "./testi.js";
import * as pdfjsLib from "./pdfjs/build/pdf.mjs";
import { apriDocumento, initDocumento, linguaDocumento, documentiAperti, __doc } from "./documento.js";

pdfjsLib.GlobalWorkerOptions.workerSrc = "/pdfjs/build/pdf.worker.mjs";

export const $ = (s, r = document) => r.querySelector(s);
export const $$ = (s, r = document) => [...r.querySelectorAll(s)];

// Opzioni comuni di PDF.js per anteprime e firma: niente JavaScript dei PDF, tutto in locale.
export const OPZ_PDFJS = {
  cMapUrl: "/pdfjs/web/cmaps/", cMapPacked: true,
  standardFontDataUrl: "/pdfjs/web/standard_fonts/", wasmUrl: "/pdfjs/web/wasm/", iccUrl: "/pdfjs/web/iccs/",
  enableXfa: false, isEvalSupported: false, disableAutoFetch: false,
  // CMap, caratteri e wasm li scarica la pagina (non il worker): così passano sempre dal programma.
  useWorkerFetch: false,
};

// ---------------------------------------------------------------- lingua

export let lingua = "it";
let versione = "";
let firmaSalvataEsiste = false;
// modoSblocco: "B" = spunta "il PDF è mio" obbligatoria; "A" = per i blocchi serve la password
// del proprietario. Lo decide il programma (pdfops.go, costante modoSblocco).
let modoSblocco = "B";

export function t(k, ...a) {
  let s = (TESTI[lingua] && TESTI[lingua][k]) ?? TESTI.en[k] ?? TESTI.it[k] ?? k;
  a.forEach((v, i) => { s = s.split("{" + i + "}").join(String(v)); });
  return s;
}

function applicaLingua() {
  document.documentElement.lang = lingua;
  $$("[data-t]").forEach(e => { e.textContent = t(e.dataset.t); });
  $$("[data-tt]").forEach(e => { e.title = t(e.dataset.tt); e.setAttribute("aria-label", t(e.dataset.tt)); });
  $$("#lingue button").forEach(b => b.classList.toggle("on", b.dataset.l === lingua));
  $("#tNome").placeholder = t("scriviEsempio");
  $("#dPwIn").placeholder = t("password");
  $("#sPw").placeholder = t("password");
  if (stato.vista === "unisci") disegnaLista();
  if (stato.vista === "firma" && firma.doc) aggiornaNavPagina();
  if (stato.vista === "sblocca") aggiornaSblocca();
  if (stato.vista === "firma") $("#fSalva").textContent = t(firma.alFine ? "firmaApplica" : "firmaSalva");
  linguaDocumento();
  $("#nomeIn").placeholder = t("nomeCampo");
  nomeDisegna();
}

export function localePdfjs() {
  return { it: "it", en: "en-US", es: "es-ES", fr: "fr", de: "de", pt: "pt-PT", pl: "pl" }[lingua] || "en-US";
}

// ---------------------------------------------------------------- chiamate al programma

class ErrGo extends Error {
  constructor(testo) {
    super(testo);
    let s = String(testo || "");
    this.pezzo = -1;
    const m = /^pezzo:(\d+)\|/.exec(s);
    if (m) { this.pezzo = +m[1]; s = s.slice(m[0].length); }
    const i = s.indexOf("|");
    this.codice = i > 0 ? s.slice(0, i) : "generico";
    this.dettaglio = i > 0 ? s.slice(i + 1) : s;
  }
}

// chiudiDocPdf: libera un documento di PDF.js. Da PDF.js 6 la chiusura sta nel "loadingTask"
// (doc.destroy() non c'è più: aprire una seconda firma dava errore).
export function chiudiDocPdf(d) {
  try { const p = (d && d.loadingTask) || d; if (p && p.destroy) p.destroy(); } catch { /* */ }
}

export async function go(nome, ...args) {
  const f = window[nome];
  if (typeof f !== "function") throw new ErrGo("generico|" + nome + " non disponibile");
  try {
    return await f(...args);
  } catch (e) {
    throw new ErrGo(e && e.message ? e.message : e);
  }
}

export function testoErrore(e, nomi) {
  if (!(e instanceof ErrGo)) return t("e_generico", e && e.message ? e.message : String(e));
  let base = TESTI.it["e_" + e.codice] ? t("e_" + e.codice, e.dettaglio) : t("e_generico", e.dettaglio);
  if (e.pezzo >= 0) base = t("e_pezzo", e.pezzo + 1, (nomi && nomi[e.pezzo]) || "", base);
  return base;
}

// ---------------------------------------------------------------- finestre, attesa, avvisi

export function finestra(id, on) { $("#" + id).classList.toggle("on", on); }

let contaAttesa = 0;
export function attesa(on, testo) {
  contaAttesa = Math.max(0, contaAttesa + (on ? 1 : -1));
  if (on) $("#dAttesaT").textContent = testo || t("lavoro");
  finestra("dAttesa", contaAttesa > 0);
}

export function chiedi(titolo, testo, tasti) {
  return new Promise(ok => {
    $("#dMsgT").textContent = titolo;
    $("#dMsgP").textContent = testo;
    const box = $("#dMsgTasti");
    box.innerHTML = "";
    tasti.forEach(([etich, valore, prim]) => {
      const b = document.createElement("button");
      b.className = "btn" + (prim ? " prim" : "");
      b.textContent = etich;
      b.onclick = () => { finestra("dMsg", false); ok(valore); };
      box.appendChild(b);
    });
    finestra("dMsg", true);
    setTimeout(() => box.lastChild && box.lastChild.focus(), 30);
  });
}

export const avvisa = (testo, titolo = "Tasso PDF") => chiedi(titolo, testo, [[t("ok"), true, true]]);
export const avvisaErrore = (e, nomi) => avvisa(testoErrore(e, nomi));

export function chiediPassword(nome, sbagliata) {
  return new Promise(ok => {
    $("#dPwP").textContent = t("pwTesto", nome);
    $("#dPwErr").textContent = t("pwSbagliata");
    $("#dPwErr").classList.toggle("hidden", !sbagliata);
    const inp = $("#dPwIn");
    inp.value = "";
    const fine = v => { finestra("dPw", false); inp.onkeydown = null; ok(v); };
    $("#dPwOk").onclick = () => fine(inp.value);
    $("#dPwNo").onclick = () => fine(null);
    inp.onkeydown = e => { if (e.key === "Enter") fine(inp.value); if (e.key === "Escape") fine(null); };
    finestra("dPw", true);
    setTimeout(() => inp.focus(), 30);
  });
}

function chiediTesto() {
  return new Promise(ok => {
    const inp = $("#dTestoIn");
    inp.value = "";
    const fine = v => { finestra("dTesto", false); inp.onkeydown = null; ok(v); };
    $("#dTestoOk").onclick = () => fine(inp.value.trim() || null);
    $("#dTestoNo").onclick = () => fine(null);
    inp.onkeydown = e => { if (e.key === "Enter") fine(inp.value.trim() || null); if (e.key === "Escape") fine(null); };
    finestra("dTesto", true);
    setTimeout(() => inp.focus(), 30);
  });
}

let ultimoSalvato = "";
let timerToast = 0;
export function toast(percorso) {
  ultimoSalvato = percorso;
  $("#toastT").textContent = t("salvato", percorso);
  $("#toastT").title = percorso;
  $("#toast").classList.add("on");
  clearTimeout(timerToast);
  timerToast = setTimeout(() => $("#toast").classList.remove("on"), 12000);
}

// ---------------------------------------------------------------- file

export const stato = { vista: "home" };

// nomeCon("Contratto.pdf", "firmato") = "Contratto - firmato.pdf"
export function nomeCon(nome, suffisso) {
  const base = String(nome || "documento").replace(/\.(pdf|jpe?g|png|heic|heif)$/i, "");
  return base + " - " + suffisso + ".pdf";
}

export function base64DaBytes(u8) {
  let s = "";
  const passo = 0x8000;
  for (let i = 0; i < u8.length; i += passo) s += String.fromCharCode.apply(null, u8.subarray(i, i + passo));
  return btoa(s);
}

// caricaBytes: manda al programma dei byte (file trascinato o PDF compilato) a pezzi da 4 MB.
async function caricaBytes(leggiPezzo, dim, nome) {
  const id = await go("goCaricaInizio", nome);
  const passo = 4 << 20;
  for (let i = 0; i < dim; i += passo) {
    const u8 = await leggiPezzo(i, Math.min(dim, i + passo));
    await go("goCaricaPezzo", id, base64DaBytes(u8));
  }
  return await go("goCaricaFine", id);
}

export const caricaFile = f => caricaBytes(async (a, b) => new Uint8Array(await f.slice(a, b).arrayBuffer()), f.size, f.name);
export const caricaArray = (u8, nome) => caricaBytes(async (a, b) => u8.subarray(a, b), u8.length, nome);

export async function scegli(multi, soloPDF, titolo) {
  try {
    return (await go("goScegli", multi, soloPDF, titolo)) || [];
  } catch (e) {
    avvisaErrore(e);
    return [];
  }
}

export async function salva(id, nomeSuggerito) {
  try {
    const p = await go("goSalva", id, nomeSuggerito, t("titoloSalva"));
    if (p) toast(p);
    return p;
  } catch (e) {
    avvisaErrore(e);
    return "";
  }
}

// proteggiCopia: la copia nuova (firmata, unita) di un PDF protetto. Se l'originale si apriva
// con una password, propone di rimetterla uguale; se aveva solo blocchi, lo dice.
// Restituisce la voce da salvare (quella protetta, se l'utente ha detto sì).
export async function proteggiCopia(r, nome, pwApertura, soloBlocchi) {
  if (pwApertura) {
    const si = await chiedi(t("protettoTit"), t("protettoTesto", nome), [[t("protettoNo"), false], [t("protettoSi"), true, true]]);
    if (!si) return r;
    attesa(true);
    try {
      const p = await go("goProteggi", r.id, "", { apertura: pwApertura, permessi: "", blocca: false, stampa: false, copia: false, modifica: false });
      go("goDimentica", r.id).catch(() => {});
      return p;
    } catch (e) { avvisaErrore(e); return r; }
    finally { attesa(false); }
  }
  if (soloBlocchi) await avvisa(t("protettoSoloBlocchi", nome), t("protettoTit"));
  return r;
}

// ---------------------------------------------------------------- viste

export function vista(nome) {
  if (nome === "home" || nome === "doc") stato.ritorno = nome;
  stato.vista = nome;
  $$(".vista").forEach(v => v.classList.toggle("on", v.id === "v-" + nome));
}

// Indietro: al documento, se si è arrivati da lì; altrimenti alla schermata iniziale.
$$("[data-indietro]").forEach(b => b.addEventListener("click", () => {
  if (stato.ritorno === "doc" && documentiAperti()) vista("doc"); else vista("home");
}));

// ---------------------------------------------------------------- collegamenti e apertura

export async function chiediLink(url) {
  const si = await chiedi(t("linkT"), t("linkD", url), [[t("annulla"), false], [t("linkApri"), true, true]]);
  if (si) {
    try { await go("goApriLink", url); } catch (e) { avvisaErrore(e); }
  }
}

async function apriDaDialogo() {
  const vv = await scegli(true, true, t("titoloApri"));
  for (const v of vv) await apriDocumento(v);
}

// ---------------------------------------------------------------- UNISCI

const unisci = { voci: [] }; // { voce, rot, pw, pagine, protetto }

async function aggiungiAUnisci(voci, pwNoto) {
  for (const v of voci) {
    if (!["pdf", "jpg", "png", "heic"].includes(v.tipo)) {
      await avvisa(t("e_formato") + "\n" + v.nome);
      continue;
    }
    const it = { voce: v, rot: 0, pw: pwNoto || "", pagine: 1, protetto: false };
    if (v.tipo === "pdf") {
      try {
        let inf = await go("goInfo", v.id, it.pw);
        it.protetto = inf.cifrato;
        let sbagliata = false;
        while (inf.servePassword && !inf.passwordGiusta) {
          const p = await chiediPassword(v.nome, sbagliata);
          if (p === null) { it.saltato = true; break; }
          inf = await go("goInfo", v.id, p);
          if (inf.passwordGiusta) it.pw = p;
          sbagliata = true;
        }
        if (it.saltato) continue;
        it.pagine = inf.pagine || 1;
        it.conPassword = !!inf.conPassword; // si apriva solo con la password (it.pw)
      } catch (e) {
        await avvisa(testoErrore(e) + "\n" + v.nome);
        continue;
      }
    }
    unisci.voci.push(it);
  }
  disegnaLista();
}

function icona(nome) {
  const p = {
    sx: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/>',
    dx: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
    x: '<path d="M18 6 6 18M6 6l12 12"/>',
    su: '<path d="m18 15-6-6-6 6"/>', giu: '<path d="m6 9 6 6 6-6"/>',
    man: '<circle cx="9" cy="6" r="1"/><circle cx="15" cy="6" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="18" r="1"/><circle cx="15" cy="18" r="1"/>',
  }[nome];
  return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${p}</svg>`;
}

const miniature = new Map(); // id -> elemento (canvas o img), per non ridisegnarle

function miniatura(it) {
  const id = it.voce.id;
  if (miniature.has(id)) return miniature.get(id);
  let el;
  if (it.voce.tipo === "pdf") {
    el = document.createElement("canvas");
    el.width = 1; el.height = 1;
    (async () => {
      try {
        const doc = await pdfjsLib.getDocument({ url: "/file/" + id, password: it.pw, ...OPZ_PDFJS }).promise;
        const pag = await doc.getPage(1);
        const v1 = pag.getViewport({ scale: 1 });
        const sc = 128 / Math.max(v1.width, v1.height);
        const vp = pag.getViewport({ scale: sc });
        el.width = Math.ceil(vp.width); el.height = Math.ceil(vp.height);
        await pag.render({ canvasContext: el.getContext("2d"), viewport: vp }).promise;
        chiudiDocPdf(doc);
      } catch { /* miniatura non disponibile */ }
    })();
  } else {
    el = document.createElement("img");
    el.alt = "";
    el.src = "/file/" + id;
    el.onerror = () => { el.replaceWith(Object.assign(document.createElement("b"), { textContent: it.voce.tipo.toUpperCase(), style: "color:#333" })); };
  }
  miniature.set(id, el);
  return el;
}

let trascinato = -1;

function disegnaLista() {
  const lista = $("#uLista");
  lista.innerHTML = "";
  unisci.voci.forEach((it, i) => {
    const r = document.createElement("div");
    r.className = "voce";
    r.draggable = true;
    const quante = it.voce.tipo === "pdf" ? (it.pagine === 1 ? t("pagine1") : t("pagineN", it.pagine)) : t("foto");
    r.innerHTML = `<span class="maniglia">${icona("man")}</span><div class="min"></div>
      <div class="testo"><b></b><small>${quante}${it.protetto ? `<span class="etic rossa">${t("protetto")}</span>` : ""}${it.rot ? `<span class="etic">${it.rot}°</span>` : ""}</small></div>
      <div class="azioni">
        <button class="icobtn" data-a="su" title="${t("su")}">${icona("su")}</button>
        <button class="icobtn" data-a="giu" title="${t("giu")}">${icona("giu")}</button>
        <button class="icobtn" data-a="sx" title="${t("girSx")}">${icona("sx")}</button>
        <button class="icobtn" data-a="dx" title="${t("girDx")}">${icona("dx")}</button>
        <button class="icobtn" data-a="x" title="${t("togli")}">${icona("x")}</button>
      </div>`;
    $("b", r).textContent = it.voce.nome;
    const m = miniatura(it);
    m.style.transform = `rotate(${it.rot}deg)`;
    $(".min", r).appendChild(m);
    r.querySelectorAll("[data-a]").forEach(b => b.onclick = () => azioneVoce(i, b.dataset.a));
    r.addEventListener("dragstart", e => {
      trascinato = i;
      e.dataTransfer.setData("text/x-tasso", String(i));
      e.dataTransfer.effectAllowed = "move";
      r.classList.add("trascina");
    });
    r.addEventListener("dragend", () => { trascinato = -1; r.classList.remove("trascina"); });
    r.addEventListener("dragover", e => {
      if (trascinato < 0) return;
      e.preventDefault();
      r.classList.add("sopra");
    });
    r.addEventListener("dragleave", () => r.classList.remove("sopra"));
    r.addEventListener("drop", e => {
      if (trascinato < 0) return;
      e.preventDefault();
      e.stopPropagation();
      r.classList.remove("sopra");
      const [x] = unisci.voci.splice(trascinato, 1);
      unisci.voci.splice(i, 0, x);
      trascinato = -1;
      disegnaLista();
    });
    lista.appendChild(r);
  });
  $("#uZona").textContent = unisci.voci.length === 1 ? t("unisciPochi") : t("unisciVuoto");
  $("#uZona").classList.toggle("hidden", unisci.voci.length > 1);
  $("#uSalva").disabled = unisci.voci.length === 0;
}

function azioneVoce(i, a) {
  const v = unisci.voci;
  if (a === "x") { miniature.delete(v[i].voce.id); v.splice(i, 1); }
  if (a === "su" && i > 0) [v[i - 1], v[i]] = [v[i], v[i - 1]];
  if (a === "giu" && i < v.length - 1) [v[i + 1], v[i]] = [v[i], v[i + 1]];
  if (a === "sx") v[i].rot = (v[i].rot + 270) % 360;
  if (a === "dx") v[i].rot = (v[i].rot + 90) % 360;
  disegnaLista();
}

export async function apriUnisci(voci, daDoc, pw) {
  if (!daDoc) stato.ritorno = "home";
  unisci.voci = [];
  miniature.clear();
  vista("unisci");
  if (voci && voci.length) await aggiungiAUnisci(voci, pw);
  else disegnaLista();
}

$("#uAggiungi").onclick = async () => aggiungiAUnisci(await scegli(true, false, t("titoloApriMolti")));
$("#uSalva").onclick = async () => {
  if (!unisci.voci.length) return avvisa(t("unisciVuoto"));
  attesa(true);
  let r;
  try {
    r = await go("goUnisci", unisci.voci.map(it => ({ id: it.voce.id, rot: it.rot, pw: it.pw })));
  } catch (e) {
    attesa(false);
    return avvisaErrore(e, unisci.voci.map(it => it.voce.nome));
  }
  attesa(false);
  // Un file con password: la copia unita si aprirebbe senza. Propongo la stessa password.
  const conPw = unisci.voci.find(it => it.protetto && it.conPassword && it.pw);
  const conBlocchi = unisci.voci.find(it => it.protetto && !it.conPassword);
  if (conPw) r = await proteggiCopia(r, conPw.voce.nome, conPw.pw, false);
  else if (conBlocchi) r = await proteggiCopia(r, conBlocchi.voce.nome, "", true);
  await salva(r.id, nomeCon(unisci.voci[0].voce.nome, t("suffUnito")));
  go("goDimentica", r.id).catch(() => {});
};

// ---------------------------------------------------------------- FIRMA

const firma = { alFine: null, voce: null, pw: "", doc: null, n: 0, pag: 1, elementi: [], sel: null, colore: "#1a3ea8", png: "", dimPag: null, conta: 0 };

// apriFirmaDaDocumento: la firma va sul documento aperto; alla fine il risultato torna lì.
export function apriFirmaDaDocumento(voce, pw, alFine) { return apriFirma(voce, pw, alFine); }

async function apriFirma(voce, pwNoto, alFine) {
  if (voce.tipo !== "pdf") return avvisa(t("e_non_pdf"));
  if (!alFine) stato.ritorno = "home";
  let pw = pwNoto || "";
  let inf;
  try {
    inf = await go("goInfo", voce.id, pw);
    let sbagliata = false;
    while (inf.servePassword && !inf.passwordGiusta) {
      const p = await chiediPassword(voce.nome, sbagliata);
      if (p === null) return;
      inf = await go("goInfo", voce.id, p);
      if (inf.passwordGiusta) pw = p;
      sbagliata = true;
    }
  } catch (e) { return avvisaErrore(e); }
  let doc;
  attesa(true, t("caricamento"));
  try {
    doc = await pdfjsLib.getDocument({ url: "/file/" + voce.id, password: pw, ...OPZ_PDFJS }).promise;
  } catch {
    // PDF rovinato: lo riparo prima (la firma andrà sulla copia riparata).
    try {
      const r = await go("goRipara", voce.id, pw);
      r.nome = voce.nome;
      voce = r;
      pw = "";
      doc = await pdfjsLib.getDocument({ url: "/file/" + voce.id, ...OPZ_PDFJS }).promise;
    } catch (e2) {
      attesa(false);
      return avvisa(t("nonRiparabile"));
    }
  } finally { attesa(false); }
  if (firma.doc) chiudiDocPdf(firma.doc);
  Object.assign(firma, { voce, pw, doc, n: doc.numPages, pag: 1, elementi: [], sel: null, alFine: alFine || null,
    protetto: !!(inf && inf.cifrato), pwApertura: inf && inf.conPassword ? pw : "", nomeOrig: voce.nome });
  // Firma digitale (PAdES) già presente: con la firma disegnata la copia non la avrà più valida.
  $("#fAvvisoDig").classList.toggle("hidden", !(inf && inf.firmatoDigitalmente));
  $("#fSalva").textContent = t(alFine ? "firmaApplica" : "firmaSalva");
  $("#fCambia").classList.toggle("hidden", !!alFine);
  $("#fNome").textContent = voce.nome;
  vista("firma");
  if (!firma.png && firmaSalvataEsiste) {
    try { const p = await go("goCaricaFirma"); if (p) impostaFirma(p); } catch { /* */ }
  }
  aggiornaAnteprimaFirma();
  await disegnaPagina();
}

function aggiornaNavPagina() {
  $("#fPagNum").textContent = t("pagina", firma.pag, firma.n);
  $("#fPrec").disabled = firma.pag <= 1;
  $("#fSucc").disabled = firma.pag >= firma.n;
}

let rendering = null;
async function disegnaPagina() {
  aggiornaNavPagina();
  const pag = await firma.doc.getPage(firma.pag);
  const v1 = pag.getViewport({ scale: 1 }); // in punti, già girata come si vede
  firma.dimPag = { w: v1.width, h: v1.height };
  const scena = $("#fScena");
  const maxW = Math.max(300, scena.clientWidth - 60), maxH = Math.max(300, scena.clientHeight - 90);
  const sc = Math.min(maxW / v1.width, maxH / v1.height, 2.2);
  const dpr = window.devicePixelRatio || 1;
  const vp = pag.getViewport({ scale: sc * dpr });
  const c = $("#fCanvas");
  c.width = Math.floor(vp.width); c.height = Math.floor(vp.height);
  c.style.width = Math.floor(v1.width * sc) + "px";
  c.style.height = Math.floor(v1.height * sc) + "px";
  if (rendering) { try { rendering.cancel(); } catch { /* */ } }
  rendering = pag.render({ canvasContext: c.getContext("2d"), viewport: vp });
  try { await rendering.promise; } catch { /* annullato */ }
  rendering = null;
  disegnaElementi();
}

$("#fPrec").onclick = () => { if (firma.pag > 1) { firma.pag--; disegnaPagina(); } };
$("#fSucc").onclick = () => { if (firma.pag < firma.n) { firma.pag++; disegnaPagina(); } };
window.addEventListener("resize", () => { if (stato.vista === "firma" && firma.doc) disegnaPagina(); });

function disegnaElementi() {
  const fo = $("#fFoglio");
  $$(".el", fo).forEach(e => e.remove());
  for (const el of firma.elementi) {
    if (!el.tutte && el.pagina !== firma.pag) continue;
    const d = document.createElement("div");
    d.className = "el" + (firma.sel === el ? " sel" : "");
    Object.assign(d.style, { left: el.x * 100 + "%", top: el.y * 100 + "%", width: el.l * 100 + "%", height: el.a * 100 + "%" });
    const img = document.createElement("img");
    img.src = el.png;
    img.alt = "";
    d.appendChild(img);
    const ang = document.createElement("div");
    ang.className = "ang";
    d.appendChild(ang);
    const x = document.createElement("button");
    x.className = "x";
    x.textContent = "✕";
    x.title = t("elimina");
    x.onpointerdown = e => e.stopPropagation();
    x.onclick = () => eliminaElemento(el);
    d.appendChild(x);
    trascinabile(d, el, ang);
    fo.appendChild(d);
  }
  const sel = firma.sel;
  $("#fTutteRiga").classList.toggle("hidden", !sel || firma.n < 2);
  if (sel) $("#fTutte").checked = !!sel.tutte;
}

function seleziona(el) {
  firma.sel = el;
  disegnaElementi();
}

function eliminaElemento(el) {
  firma.elementi = firma.elementi.filter(e => e !== el);
  if (firma.sel === el) firma.sel = null;
  disegnaElementi();
}

function trascinabile(div, el, ang) {
  div.addEventListener("pointerdown", e => {
    if (e.button !== 0) return;
    e.preventDefault();
    const fo = $("#fFoglio").getBoundingClientRect();
    const ridimensiona = e.target === ang;
    const x0 = e.clientX, y0 = e.clientY;
    const s = { x: el.x, y: el.y, l: el.l, a: el.a };
    const rapporto = s.a / s.l;
    if (firma.sel !== el) { firma.sel = el; $$(".el").forEach(d => d.classList.remove("sel")); div.classList.add("sel"); $("#fTutteRiga").classList.toggle("hidden", firma.n < 2); $("#fTutte").checked = !!el.tutte; }
    div.setPointerCapture(e.pointerId);
    const muovi = ev => {
      const dx = (ev.clientX - x0) / fo.width, dy = (ev.clientY - y0) / fo.height;
      if (ridimensiona) {
        let l = Math.max(0.02, Math.min(1 - s.x, s.l + dx));
        let a = l * rapporto;
        if (s.y + a > 1) { a = 1 - s.y; l = a / rapporto; }
        el.l = l; el.a = a;
      } else {
        el.x = Math.max(0, Math.min(1 - el.l, s.x + dx));
        el.y = Math.max(0, Math.min(1 - el.a, s.y + dy));
      }
      Object.assign(div.style, { left: el.x * 100 + "%", top: el.y * 100 + "%", width: el.l * 100 + "%", height: el.a * 100 + "%" });
    };
    const fine = () => { div.removeEventListener("pointermove", muovi); div.removeEventListener("pointerup", fine); div.removeEventListener("pointercancel", fine); };
    div.addEventListener("pointermove", muovi);
    div.addEventListener("pointerup", fine);
    div.addEventListener("pointercancel", fine);
  });
}

$("#fFoglio").addEventListener("pointerdown", e => { if (e.target.id === "fCanvas" && firma.sel) seleziona(null); });
document.addEventListener("keydown", e => {
  if (stato.vista !== "firma" || !firma.sel) return;
  if (document.activeElement && /INPUT|TEXTAREA/.test(document.activeElement.tagName)) return;
  if ($(".velo.on")) return;
  if (e.key === "Delete" || e.key === "Backspace") { e.preventDefault(); eliminaElemento(firma.sel); }
});
$("#fTutte").onchange = () => { if (firma.sel) { firma.sel.tutte = $("#fTutte").checked; if (!firma.sel.tutte) firma.sel.pagina = firma.pag; disegnaElementi(); } };

// dimensioni dell'immagine PNG
const misuraImg = src => new Promise((ok, ko) => { const i = new Image(); i.onload = () => ok({ w: i.naturalWidth, h: i.naturalHeight }); i.onerror = ko; i.src = src; });

// aggiungiElemento: larghezza in punti della pagina; l'altezza segue le proporzioni dell'immagine.
async function aggiungiElemento(tipo, png, larghezzaPt, extra = {}) {
  const { w, h } = await misuraImg(png);
  const P = firma.dimPag;
  let l = Math.min(0.9, larghezzaPt / P.w);
  let a = l * (h / w) * (P.w / P.h);
  if (a > 0.5) { a = 0.5; l = a / ((h / w) * (P.w / P.h)); }
  // Gli elementi nuovi si mettono un po' più in basso uno dell'altro, per non coprirsi.
  const k = firma.elementi.filter(e => e.tutte || e.pagina === firma.pag).length % 6;
  const x = Math.max(0, Math.min(1 - l, 0.5 - l / 2 + k * 0.02));
  const y = Math.max(0, Math.min(1 - a, 0.35 - a / 2 + k * 0.09));
  const el = { id: ++firma.conta, tipo, png, pagina: firma.pag, tutte: false, x, y, l, a, ...extra };
  firma.elementi.push(el);
  seleziona(el);
}

// PNG di un testo (data o testo libero), disegnato in alta risoluzione con il carattere del programma.
export async function pngTesto(testo, colore) {
  await document.fonts.load("64px TassoOnest");
  const fs = 96;
  const c = document.createElement("canvas");
  const x = c.getContext("2d");
  x.font = `600 ${fs}px TassoOnest, "Segoe UI", sans-serif`;
  const w = Math.ceil(x.measureText(testo).width) + 16;
  c.width = Math.max(8, w); c.height = Math.ceil(fs * 1.3);
  x.font = `600 ${fs}px TassoOnest, "Segoe UI", sans-serif`;
  x.fillStyle = colore;
  x.textBaseline = "middle";
  x.fillText(testo, 8, c.height / 2);
  return c.toDataURL("image/png");
}

function pngSpunta(colore) {
  const c = document.createElement("canvas");
  c.width = c.height = 160;
  const x = c.getContext("2d");
  x.strokeStyle = colore;
  x.lineWidth = 22;
  x.lineCap = "round";
  x.lineJoin = "round";
  x.beginPath();
  x.moveTo(26, 86); x.lineTo(64, 124); x.lineTo(136, 36);
  x.stroke();
  return c.toDataURL("image/png");
}

export function oggi() {
  try { return new Intl.DateTimeFormat(lingua === "en" ? "en-GB" : lingua, { day: "2-digit", month: "2-digit", year: "numeric" }).format(new Date()); }
  catch { return new Date().toLocaleDateString(); }
}

$("#fData").onclick = async () => { const s = oggi(); await aggiungiElemento("data", await pngTesto(s, firma.colore), 80, { testo: s }); };
$("#fTesto").onclick = async () => {
  const s = await chiediTesto();
  if (s) await aggiungiElemento("testo", await pngTesto(s, firma.colore), Math.min(400, 7 * s.length + 20), { testo: s });
};
$("#fSpunta").onclick = async () => aggiungiElemento("spunta", pngSpunta(firma.colore), 22);
$("#fUsa").onclick = async () => {
  if (!firma.png) return creaFirma(true);
  await aggiungiElemento("firma", firma.png, 170);
};
$("#fCrea").onclick = () => creaFirma(false);
$("#fCambia").onclick = async () => { const vv = await scegli(false, true, t("titoloApri")); if (vv.length) apriFirma(vv[0]); };
$$("#fColori button").forEach(b => b.onclick = async () => {
  firma.colore = b.dataset.c;
  $$("#fColori button").forEach(x => x.classList.toggle("on", x === b));
  const el = firma.sel;
  if (el && (el.tipo === "data" || el.tipo === "testo")) { el.png = await pngTesto(el.testo, firma.colore); disegnaElementi(); }
  if (el && el.tipo === "spunta") { el.png = pngSpunta(firma.colore); disegnaElementi(); }
});

$("#fSalva").onclick = async () => {
  if (!firma.elementi.length) return avvisa(t("nessunElemento"));
  const timbri = firma.elementi.map(e => ({ pagina: e.tutte ? 0 : e.pagina, x: e.x, y: e.y, l: e.l, a: e.a, png: e.png }));
  attesa(true);
  let r;
  try { r = await go("goFirma", firma.voce.id, firma.pw, timbri); }
  catch (e) { attesa(false); return avvisaErrore(e); }
  attesa(false);
  if (firma.alFine) { const f = firma.alFine; firma.alFine = null; return f(r); }
  if (firma.protetto) r = await proteggiCopia(r, firma.nomeOrig || firma.voce.nome, firma.pwApertura, !firma.pwApertura);
  await salva(r.id, nomeCon(firma.voce.nome, t("suffFirmato")));
  go("goDimentica", r.id).catch(() => {});
};

function impostaFirma(png) {
  firma.png = png;
  aggiornaAnteprimaFirma();
}

function aggiornaAnteprimaFirma() {
  const a = $("#fAnteprima");
  a.innerHTML = "";
  if (firma.png) { const i = new Image(); i.src = firma.png; i.alt = ""; a.appendChild(i); }
  a.classList.toggle("hidden", !firma.png);
}

// ---- creazione della firma (disegnata, scritta, da foto)

let schedaFirma = "disegna";
let tratti = [];
let fotoSorgente = null;

function mostraScheda(n) {
  schedaFirma = n;
  $$("[data-scheda]").forEach(b => b.classList.toggle("on", b.dataset.scheda === n));
  $$("[data-pannello]").forEach(p => p.classList.toggle("hidden", p.dataset.pannello !== n));
  if (n === "disegna") preparaTavola();
}
$$("[data-scheda]").forEach(b => b.onclick = () => mostraScheda(b.dataset.scheda));

function preparaTavola() {
  const c = $("#tavola");
  const r = c.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  c.width = Math.max(10, Math.round(r.width * dpr));
  c.height = Math.max(10, Math.round(r.height * dpr));
  ridisegnaTavola();
}

function ridisegnaTavola() {
  const c = $("#tavola"), x = c.getContext("2d");
  x.clearRect(0, 0, c.width, c.height);
  const dpr = window.devicePixelRatio || 1;
  x.strokeStyle = firma.colore;
  x.lineCap = "round"; x.lineJoin = "round";
  for (const tr of tratti) {
    if (!tr.length) continue;
    x.beginPath();
    x.moveTo(tr[0].x * c.width, tr[0].y * c.height);
    for (let i = 1; i < tr.length; i++) {
      const p = tr[i - 1], q = tr[i];
      x.lineWidth = Math.max(1.2, 3.2 * (q.p || 0.5) * 1.6) * dpr;
      x.quadraticCurveTo(p.x * c.width, p.y * c.height, (p.x + q.x) / 2 * c.width, (p.y + q.y) / 2 * c.height);
    }
    x.stroke();
    if (tr.length === 1) { x.beginPath(); x.arc(tr[0].x * c.width, tr[0].y * c.height, 2 * dpr, 0, 7); x.fillStyle = firma.colore; x.fill(); }
  }
}

(() => {
  const c = $("#tavola");
  let attivo = null;
  c.addEventListener("pointerdown", e => {
    e.preventDefault();
    c.setPointerCapture(e.pointerId);
    const r = c.getBoundingClientRect();
    attivo = [{ x: (e.clientX - r.left) / r.width, y: (e.clientY - r.top) / r.height, p: e.pressure || 0.5 }];
    tratti.push(attivo);
    ridisegnaTavola();
  });
  c.addEventListener("pointermove", e => {
    if (!attivo) return;
    const r = c.getBoundingClientRect();
    attivo.push({ x: (e.clientX - r.left) / r.width, y: (e.clientY - r.top) / r.height, p: e.pressure || 0.5 });
    ridisegnaTavola();
  });
  const fine = () => { attivo = null; };
  c.addEventListener("pointerup", fine);
  c.addEventListener("pointercancel", fine);
})();

$("#tCancella").onclick = () => { tratti = []; ridisegnaTavola(); };

// ritaglia: toglie i bordi trasparenti attorno alla firma (con un piccolo margine).
function ritaglia(c) {
  const x = c.getContext("2d");
  const { data, width: w, height: h } = x.getImageData(0, 0, c.width, c.height);
  let x0 = w, y0 = h, x1 = -1, y1 = -1;
  for (let y = 0; y < h; y++) for (let i = 0; i < w; i++) {
    if (data[(y * w + i) * 4 + 3] > 8) { if (i < x0) x0 = i; if (i > x1) x1 = i; if (y < y0) y0 = y; if (y > y1) y1 = y; }
  }
  if (x1 < 0) return null;
  const m = Math.round(Math.max(w, h) * 0.02);
  x0 = Math.max(0, x0 - m); y0 = Math.max(0, y0 - m); x1 = Math.min(w - 1, x1 + m); y1 = Math.min(h - 1, y1 + m);
  const o = document.createElement("canvas");
  o.width = x1 - x0 + 1; o.height = y1 - y0 + 1;
  o.getContext("2d").drawImage(c, x0, y0, o.width, o.height, 0, 0, o.width, o.height);
  return o.toDataURL("image/png");
}

function firmaScritta() {
  const s = $("#tNome").value.trim();
  if (!s) return null;
  const c = document.createElement("canvas");
  const x = c.getContext("2d");
  const fs = 140;
  x.font = `600 ${fs}px TassoMano`;
  c.width = Math.ceil(x.measureText(s).width + fs);
  c.height = Math.ceil(fs * 1.8);
  x.font = `600 ${fs}px TassoMano`;
  x.fillStyle = firma.colore;
  x.textBaseline = "middle";
  x.fillText(s, fs / 2, c.height / 2);
  return ritaglia(c);
}

$("#tNome").addEventListener("input", () => { $("#scrittaNome").textContent = $("#tNome").value; });

function fotoSenzaSfondo() {
  if (!fotoSorgente) return null;
  const img = fotoSorgente;
  const sc = Math.min(1, 1600 / Math.max(img.naturalWidth, img.naturalHeight));
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(img.naturalWidth * sc)); c.height = Math.max(1, Math.round(img.naturalHeight * sc));
  const x = c.getContext("2d");
  x.drawImage(img, 0, 0, c.width, c.height);
  const d = x.getImageData(0, 0, c.width, c.height);
  const soglia = +$("#tSoglia").value;
  const morbido = Math.max(10, soglia * 0.35);
  const p = d.data;
  for (let i = 0; i < p.length; i += 4) {
    const lum = 0.299 * p[i] + 0.587 * p[i + 1] + 0.114 * p[i + 2];
    // più chiaro della soglia = sfondo (trasparente); vicino alla soglia = bordo morbido
    const alfa = Math.max(0, Math.min(1, (soglia - lum) / morbido));
    p[i + 3] = Math.round(p[i + 3] * alfa);
  }
  x.putImageData(d, 0, 0);
  return ritaglia(c);
}

function aggiornaFotoAnteprima() {
  const a = $("#fotoAnteprima");
  a.innerHTML = "";
  const png = fotoSenzaSfondo();
  if (png) { const i = new Image(); i.src = png; a.appendChild(i); }
}

$("#tFotoScegli").onclick = () => $("#tFotoFile").click();
$("#tFotoFile").onchange = () => {
  const f = $("#tFotoFile").files[0];
  if (!f) return;
  const url = URL.createObjectURL(f);
  const img = new Image();
  img.onload = () => { fotoSorgente = img; aggiornaFotoAnteprima(); };
  img.onerror = () => avvisa(t("e_foto"));
  img.src = url;
};
$("#tSoglia").oninput = () => aggiornaFotoAnteprima();

let poiMetti = false;
function creaFirma(metti) {
  poiMetti = metti;
  tratti = [];
  $("#tRicorda").checked = firmaSalvataEsiste;
  $("#tDimentica").classList.toggle("hidden", !firmaSalvataEsiste);
  finestra("dFirma", true);
  mostraScheda(schedaFirma);
}

$("#tNo").onclick = () => finestra("dFirma", false);
$("#tDimentica").onclick = async () => {
  try { await go("goCancellaFirma"); firmaSalvataEsiste = false; $("#tDimentica").classList.add("hidden"); $("#tRicorda").checked = false; }
  catch (e) { avvisaErrore(e); }
};
$("#tOk").onclick = async () => {
  let png = null;
  if (schedaFirma === "disegna") png = ritaglia($("#tavola"));
  if (schedaFirma === "scrivi") { await document.fonts.load("64px TassoMano"); png = firmaScritta(); }
  if (schedaFirma === "foto") png = fotoSenzaSfondo();
  if (!png) return avvisa(t("firmaVuota"));
  finestra("dFirma", false);
  impostaFirma(png);
  try {
    if ($("#tRicorda").checked) { await go("goSalvaFirma", png); firmaSalvataEsiste = true; }
    else if (firmaSalvataEsiste) { await go("goCancellaFirma"); firmaSalvataEsiste = false; }
  } catch (e) { avvisaErrore(e); }
  if (poiMetti && firma.doc) await aggiungiElemento("firma", png, 170);
};

// ---------------------------------------------------------------- SBLOCCA

const sblocca = { voce: null, info: null };

export async function apriSblocca(voce, daDoc) {
  if (voce.tipo !== "pdf") return avvisa(t("e_non_pdf"));
  if (!daDoc) stato.ritorno = "home";
  try {
    attesa(true);
    sblocca.info = await go("goInfo", voce.id, "");
  } catch (e) {
    return avvisaErrore(e);
  } finally { attesa(false); }
  sblocca.voce = voce;
  $("#sNome").textContent = voce.nome;
  $("#sPw").value = "";
  $("#sConferma").checked = false;
  $("#sErrore").classList.add("hidden");
  vista("sblocca");
  aggiornaSblocca();
  if (sblocca.info.servePassword) setTimeout(() => $("#sPw").focus(), 50);
}

function aggiornaSblocca() {
  const inf = sblocca.info;
  if (!inf) return;
  let s = t("haBlocchi");
  if (!inf.cifrato) s = t("nonProtetto");
  else if (inf.servePassword) s = t("serveLaPassword");
  // Modo A: per togliere i blocchi serve la password del proprietario (il campo compare anche
  // se il PDF si apre senza password). Modo B: spunta obbligatoria.
  const serveCampo = inf.servePassword || (modoSblocco === "A" && inf.bloccato);
  if (modoSblocco === "A" && inf.bloccato && !inf.servePassword) s += " " + t("serveProprietario");
  $("#sStato").textContent = s;
  $("#sCampo").classList.toggle("hidden", !serveCampo);
  $("#sConfermaRiga").classList.toggle("hidden", modoSblocco !== "B" || !inf.cifrato);
  $("#sVai").disabled = !inf.cifrato || (modoSblocco === "B" && !$("#sConferma").checked);
}
$("#sConferma").onchange = () => aggiornaSblocca();

$("#sMostra").onclick = () => { const i = $("#sPw"); i.type = i.type === "password" ? "text" : "password"; };
$("#sAltro").onclick = async () => { const vv = await scegli(false, true, t("titoloApri")); if (vv.length) apriSblocca(vv[0]); };
$("#sPw").addEventListener("keydown", e => { if (e.key === "Enter") $("#sVai").click(); });
$("#sVai").onclick = async () => {
  if (!sblocca.voce) return;
  $("#sErrore").classList.add("hidden");
  attesa(true);
  let r;
  const inf = sblocca.info;
  const conPw = inf.servePassword || (modoSblocco === "A" && inf.bloccato);
  try { r = await go("goSblocca", sblocca.voce.id, conPw ? $("#sPw").value : "", $("#sConferma").checked); }
  catch (e) {
    attesa(false);
    $("#sErrore").textContent = e.codice === "password" ? t("pwSbagliata") : e.codice === "password_proprietario" ? t("serveProprietario") : testoErrore(e);
    $("#sErrore").classList.remove("hidden");
    return;
  }
  attesa(false);
  await salva(r.id, nomeCon(sblocca.voce.nome, t("suffSbloccato")));
  go("goDimentica", r.id).catch(() => {});
};

// ---------------------------------------------------------------- home, trascinamento, tasti

$("#hApri").onclick = apriDaDialogo;
$("#hUnisci").onclick = async () => { const vv = await scegli(true, false, t("titoloApriMolti")); if (vv.length) apriUnisci(vv); };
$("#hFirma").onclick = async () => { const vv = await scegli(false, true, t("titoloApri")); if (vv.length) apriFirma(vv[0]); };
$("#hSblocca").onclick = async () => { const vv = await scegli(false, true, t("titoloApri")); if (vv.length) apriSblocca(vv[0]); };

// azione: cosa fare con dei file (da una scheda della schermata iniziale, dal menu del tasto destro...).
async function azione(nome, voci) {
  const pdf = voci.filter(v => v.tipo === "pdf");
  const ok = voci.filter(v => ["pdf", "jpg", "png", "heic"].includes(v.tipo));
  if (!ok.length) return avvisa(t("e_formato"));
  switch (nome) {
    case "unisci": return apriUnisci(ok);
    case "firma": return pdf.length ? apriFirma(pdf[0]) : avvisa(t("e_non_pdf"));
    case "sblocca": return pdf.length ? apriSblocca(pdf[0]) : avvisa(t("e_non_pdf"));
    case "comprimi":
      if (!pdf.length) return avvisa(t("e_non_pdf"));
      for (const v of pdf) await apriDocumento(v);
      return __doc.usaStrumento("comprimi");
    case "apri":
      if (!pdf.length) return apriUnisci(ok);
      for (const v of pdf) await apriDocumento(v);
      return;
    default:
      if (pdf.length && pdf.length === ok.length) { for (const v of pdf) await apriDocumento(v); return; }
      return apriUnisci(ok);
  }
}

async function caricaTrascinati(files) {
  const voci = [];
  attesa(true, t("caricamento"));
  try {
    for (const f of files) voci.push(await caricaFile(f));
  } catch (e) {
    attesa(false);
    avvisaErrore(e);
    return null;
  }
  attesa(false);
  return voci;
}

async function gestisciFileTrascinati(files, sullaScheda) {
  if (!files.length) return;
  const voci = await caricaTrascinati(files);
  if (!voci) return;
  const ok = voci.filter(v => ["pdf", "jpg", "png", "heic"].includes(v.tipo));
  if (!ok.length) return avvisa(t("e_formato"));
  const pdf = ok.filter(v => v.tipo === "pdf");
  switch (stato.vista) {
    case "unisci": return aggiungiAUnisci(ok);
    case "firma": return pdf.length ? apriFirma(pdf[0], "", firma.alFine) : avvisa(t("e_non_pdf"));
    case "sblocca": return pdf.length ? apriSblocca(pdf[0], stato.ritorno === "doc") : avvisa(t("e_non_pdf"));
    case "doc": return azione("auto", ok);
    default: return azione(sullaScheda || "auto", ok);
  }
}

// Trascinando dei file sulla schermata iniziale, la scheda sotto il cursore si illumina nel suo
// colore: rilasciando lì parte quella azione.
let contaTrascina = 0;
const conFile = e => e.dataTransfer && [...e.dataTransfer.types].includes("Files");
const schedaSotto = e => (stato.vista === "home" && e.target && e.target.closest) ? e.target.closest(".scheda[data-azione]") : null;
function illumina(sch) { $$(".scheda[data-azione]").forEach(x => x.classList.toggle("bersaglio", x === sch)); }
window.addEventListener("dragenter", e => { if (conFile(e)) { contaTrascina++; $("#trascinaVelo").classList.add("on"); illumina(schedaSotto(e)); } });
window.addEventListener("dragleave", e => { if (conFile(e) && --contaTrascina <= 0) { contaTrascina = 0; $("#trascinaVelo").classList.remove("on"); illumina(null); } });
window.addEventListener("dragover", e => {
  e.preventDefault();
  if (conFile(e)) { const sch = schedaSotto(e); illumina(sch); if (e.dataTransfer) e.dataTransfer.dropEffect = "copy"; }
});
window.addEventListener("drop", e => {
  e.preventDefault();
  const sch = schedaSotto(e);
  contaTrascina = 0;
  $("#trascinaVelo").classList.remove("on");
  illumina(null);
  if (conFile(e) && e.dataTransfer.files.length) gestisciFileTrascinati([...e.dataTransfer.files], sch ? sch.dataset.azione : "");
});

document.addEventListener("keydown", e => {
  if (!(e.ctrlKey || e.metaKey)) return;
  if (e.key.toLowerCase() === "o") { e.preventDefault(); apriDaDialogo(); }
});
// Nessuna navigazione indietro con il tasto o il mouse.
window.addEventListener("keydown", e => { if (e.altKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) e.preventDefault(); });
window.open = () => null;

$("#toastX").onclick = () => $("#toast").classList.remove("on");
$("#toastMostra").onclick = () => { if (ultimoSalvato) go("goMostra", ultimoSalvato).catch(() => {}); };
// Informazioni, con la riga degli aggiornamenti: li gestisce Tasso (Tasso PDF non usa internet).
$("#btnInfo").onclick = async () => {
  const vxy = versione.split(".").slice(0, 2).join(".");
  const r = await chiedi("Tasso PDF " + versione, t("infoTesto", versione) + "\n\n" + t("aggVersione", vxy) + "\n\n" + t("infoSicurezza") + "\n\n" + t("infoComponenti"),
    [[t("aggControlla"), "agg", false], [t("ok"), true, true]]);
  if (r !== "agg") return;
  const esito = await go("goControllaAggiornamenti").catch(() => "errore");
  if (esito === "errore") { avvisa(t("aggNonAperto"), t("aggTitolo")); return; }
  if (esito !== "nessuno") return;
  if ((await chiedi(t("aggTitolo"), t("aggSenzaTasso"), [[t("annulla"), false, false], [t("aggApriPagina"), true, true]])) === true) go("goPaginaVersioni").catch(() => {});
};

// ---------------------------------------------------------------- avvio

let nomeUtente = ""; // "Il tuo nome" (vedi più sotto)

(async function avvio() {
  const box = $("#lingue");
  LINGUE.forEach(l => {
    const b = document.createElement("button");
    b.textContent = l.toUpperCase();
    b.dataset.l = l;
    b.onclick = () => { lingua = l; applicaLingua(); go("goLingua", l).catch(() => {}); };
    box.appendChild(b);
  });
  try {
    const a = await go("goAvvio");
    lingua = LINGUE.includes(a.lingua) ? a.lingua : "en";
    versione = a.versione || "";
    firmaSalvataEsiste = !!a.firma;
    if (a.sblocco === "A" || a.sblocco === "B") modoSblocco = a.sblocco;
    $("#hMenu").classList.toggle("hidden", !a.windows);
    $("#hMenuIn").checked = !!a.menu;
    nomeUtente = typeof a.nome === "string" ? a.nome : ""; // "Il tuo nome"
  } catch { /* */ }
  initDocumento();
  applicaLingua();
  window.__tassoPronto = true;
  // File dell'avvio (doppio clic su un PDF, menu del tasto destro) e quelli che arrivano subito dopo.
  let primo = true;
  for (let i = 0; i < 48; i++) {
    let fa = null;
    try { fa = await go("goFileAvvio"); } catch { break; }
    if (fa && fa.file && fa.file.length) {
      const modo = fa.modo || "apri";
      if (primo || modo === "apri" || modo === "comprimi") await azione(modo, fa.file);
      else if (stato.vista === "unisci") await aggiungiAUnisci(fa.file);
      else await azione(modo, fa.file);
      primo = false;
    } else if (!fa || !fa.modo) break;
    await new Promise(ok => setTimeout(ok, 250));
  }
})();

$("#hMenuIn").onchange = async () => {
  const on = $("#hMenuIn").checked;
  try { $("#hMenuIn").checked = await go("goImpostaMenu", on); if (on) avvisa(t("menuFatto")); }
  catch (e) { $("#hMenuIn").checked = !on; avvisaErrore(e); }
};

// ---------------------------------------------------------------- "Il tuo nome" (Ecosistema Tasso 3.2)
// Lo stesso nome di Tasso e Tasso Zip (HKCU\Software\Tasso\Nome, nome_utente.go), solo sul PC.
// Sulla pagina entra sempre con textContent. Le regole sono quelle di nome/nome.go.
/* ==== REGOLE DEL NOME: INIZIO ====
   Uguali, carattere per carattere, nelle pagine di Tasso, installatore, Tasso Zip e Tasso PDF, e
   uguali alle regole in Go (Controlla). Stessi elenchi espliciti di blocchi e di caratteri invisibili,
   così la pagina e Go danno lo stesso esito anche con versioni diverse di Unicode (3.2: tutte le
   scritture delle lingue vive, con tamil, bengali, etiope, 々...; contano come accenti, al massimo 3,
   solo i segni sopra o sotto la lettera, non le vocali che occupano posto). */
const NOME_MIN = 2, NOME_MAX = 20, NOME_MAX_ACCENTI = 3;
const NOME_BLOCCHI = [
  [0x0030, 0x0039], [0x0041, 0x005A], [0x0061, 0x007A], [0x00C0, 0x036F], [0x0370, 0x03FF],
  [0x0400, 0x052F], [0x0531, 0x058F], [0x0591, 0x05FF], [0x0600, 0x07FF], [0x08A0, 0x08FF],
  [0x0900, 0x0C5B], [0x0C5D, 0x0CDB], [0x0CDD, 0x0DFF], [0x0E00, 0x10FF], [0x1200, 0x167F],
  [0x1700, 0x19DF], [0x1A00, 0x1AAF], [0x1B00, 0x1C7F], [0x1C90, 0x1CBF], [0x1E00, 0x1FFF],
  [0x2D00, 0x2DDF], [0x3005, 0x3005], [0x3040, 0x30FF], [0x3100, 0x312F], [0x31A0, 0x31BF],
  [0x31F0, 0x31FF], [0x3400, 0x4DBF], [0x4E00, 0x9FFF], [0xA000, 0xA48F], [0xA4D0, 0xA63F],
  [0xA6A0, 0xA6FF], [0xA880, 0xAB2F], [0xAB70, 0xABFF], [0xAC00, 0xD7A3], [0xF900, 0xFAFF],
  [0x104B0, 0x104FF], [0x11100, 0x1114F], [0x16B00, 0x16B8F], [0x16F00, 0x16F9F], [0x1E100, 0x1E14F],
  [0x1E2C0, 0x1E2FF], [0x1E900, 0x1E95F], [0x20000, 0x2B739], [0x2B740, 0x2CEA1], [0x2CEB0, 0x2EBEF],
  [0x2F800, 0x2FA1F], [0x30000, 0x3134F],
];
const NOME_INVISIBILI = [
  [0x0000, 0x001F], [0x007F, 0x009F], [0x00AD, 0x00AD], [0x034F, 0x034F], [0x0600, 0x0605],
  [0x061C, 0x061C], [0x06DD, 0x06DD], [0x070F, 0x070F], [0x08E2, 0x08E2], [0x115F, 0x1160],
  [0x17B4, 0x17B5], [0x180B, 0x180F], [0x2000, 0x200F], [0x2028, 0x202F], [0x205F, 0x206F],
  [0x3000, 0x3000], [0x3164, 0x3164], [0xFE00, 0xFE0F], [0xFEFF, 0xFEFF], [0xFFA0, 0xFFA0],
  [0xFFF0, 0xFFFB], [0xE0000, 0xE0FFF],
];
function nomeDentro(c, t) { for (const x of t) if (c >= x[0] && c <= x[1]) return true; return false; }
function nomePulisci(s) { return String(s == null ? "" : s).replace(/[\t\n\v\f\r \u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000\ufeff]+/g, " ").trim(); }
function nomeControlla(s) {
  const c = [...s];
  if (c.length < NOME_MIN) return { codice: "corto" };
  if (c.length > NOME_MAX) return { codice: "lungo" };
  let lettera = false, dopoLettera = false, accenti = 0;
  for (const r of c) {
    const k = r.codePointAt(0);
    if (r === " " || r === "." || r === "-" || r === "_") { dopoLettera = false; accenti = 0; }
    else if (nomeDentro(k, NOME_INVISIBILI)) return { codice: "invisibile" };
    else if (!nomeDentro(k, NOME_BLOCCHI)) return { codice: "simbolo", simbolo: r };
    else if (/^[\p{L}\p{Nd}]$/u.test(r)) { lettera = true; dopoLettera = true; accenti = 0; }
    else if (/^\p{M}$/u.test(r)) { if (!dopoLettera || (!/^\p{Mc}$/u.test(r) && ++accenti > NOME_MAX_ACCENTI)) return { codice: "accenti" }; }
    else return { codice: "simbolo", simbolo: r };
  }
  return lettera ? { codice: "" } : { codice: "senza" };
}
/* ==== REGOLE DEL NOME: FINE ==== */
const NOME_ERR = { corto: "nomeErrCorto", lungo: "nomeErrLungo", simbolo: "nomeErrSimbolo", invisibile: "nomeErrInvisibile", senza: "nomeErrSenza", accenti: "nomeErrAccenti", registro: "nomeErrRegistro" };
function nomeDisegna() {
  const n = nomeUtente, h = $("#v-home h1");
  if (h) h.textContent = n ? t("cosaNome", n) : t("cosa");
  $("#nomeB").textContent = n; $("#nomeB").hidden = !n;
  $("#nomeBtn").textContent = n ? t("nomeCambia") : t("nomeAggiungi");
}
function nomeCampo() {
  const p = nomePulisci($("#nomeIn").value), e = nomeControlla(p), err = !!p && !!e.codice, m = $("#nomeMsg");
  $("#nomeCont").textContent = [...p].length + " / 20"; $("#nomeCont").classList.toggle("err", err);
  $("#nomeIn").classList.toggle("err", err); $("#nomeIn").setAttribute("aria-invalid", err ? "true" : "false");
  m.className = "msg" + (err ? " err" : "");
  m.textContent = err ? t(NOME_ERR[e.codice], e.simbolo || "") : t("nomeRegole");
  $("#nomeSi").disabled = !!e.codice;
  return e;
}
function nomeApri() {
  $("#nomeIn").value = nomeUtente; nomeCampo();
  $("#nomeMod").classList.add("on"); setTimeout(() => { $("#nomeIn").focus(); $("#nomeIn").select(); }, 30);
}
function nomeChiudi() { $("#nomeMod").classList.remove("on"); $("#nomeBtn").focus(); }
async function nomeSalva() {
  if ($("#nomeSi").disabled || nomeCampo().codice) return;
  $("#nomeSi").disabled = true;
  let r = null;
  try { r = await go("goNomeSalva", nomePulisci($("#nomeIn").value)); } catch { r = null; }
  $("#nomeSi").disabled = false;
  if (!r || !r.ok) { const m = $("#nomeMsg"); m.className = "msg err"; m.textContent = t(NOME_ERR[(r && r.errore) || "registro"] || "nomeErrRegistro", (r && r.simbolo) || ""); return; }
  nomeUtente = r.nome; nomeChiudi(); nomeDisegna();
  const f = $("#nomeFatto"); f.textContent = t("nomeSalvato", r.nome); clearTimeout(nomeSalva.t); nomeSalva.t = setTimeout(() => { f.textContent = ""; }, 4000);
}
$("#nomeBtn").onclick = nomeApri;
$("#nomeNo").onclick = nomeChiudi;
$("#nomeSi").onclick = nomeSalva;
$("#nomeIn").addEventListener("input", nomeCampo);
$("#nomeIn").addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); nomeSalva(); } else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); nomeChiudi(); } });
$("#nomeMod").addEventListener("mousedown", e => { if (e.target === $("#nomeMod")) nomeChiudi(); });
// cambiato in Tasso o in Tasso Zip mentre Tasso PDF era aperto: lo rileggo quando la finestra torna in primo piano
window.addEventListener("focus", async () => { if ($("#nomeMod").classList.contains("on")) return; try { const n = await go("goNome"); if (typeof n === "string" && n !== nomeUtente) { nomeUtente = n; nomeDisegna(); } } catch { /* */ } });

// per le prove automatiche
window.__tasso = { stato, unisci, firma, sblocca, gestisciFileTrascinati, apriDocumento, apriDaDialogo, apriFirma, apriSblocca, apriUnisci, aggiungiElemento, impostaFirma, pngTesto, azione, doc: __doc };
