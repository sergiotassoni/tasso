// Tasso PDF - la vista documento: schede dei file aperti, sezioni di strumenti, barra rapida,
// documento (PDF.js) con miniature, editor di pagina e "Organizza pagine".
// Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later.
//
// Regola d'oro: ogni modifica crea una NUOVA versione in memoria (il file originale non si
// tocca mai). "Salva una copia" scrive la versione corrente in un file nuovo.

import * as pdfjsLib from "./pdfjs/build/pdf.mjs";
import { ico } from "./icone.js";
import {
  $, $$, t, go, testoErrore, attesa, chiedi, avvisa, avvisaErrore, chiediPassword, toast, nomeCon,
  caricaArray, scegli, salva, vista, stato, OPZ_PDFJS, chiediLink, lingua, localePdfjs, apriFirmaDaDocumento,
  apriUnisci, apriSblocca, base64DaBytes, oggi,
} from "./app.js";

// ---------------------------------------------------------------- stato

const D = {
  schede: [], // { k, nome, originale, versioni: [{voce, pw}], pagina, info, moduli, chiuse:Set, salvata }
  att: null,
  sez: "tutti",
  strumento: null,
  rapido: "seleziona",
  pannello: true,
  mini: true,
  conta: 0,
};
const corrente = sc => sc.versioni[sc.versioni.length - 1];
const modificato = sc => sc.versioni.length > 1;

const COLORI = { blu: "#4DA8FF", viola: "#A77BFF", rosso: "#FF6B5C", verde: "#2FF3A6", giallo: "#FFB547", ciano: "#3DD6E0", rosa: "#FF7AB8", arancio: "#FF9A4D" };

// ---------------------------------------------------------------- PDF.js (visualizzatore)

let visorePronto = null;
let visoreLingua = "";
let appV = null; // PDFViewerApplication dell'iframe

document.addEventListener("webviewerloaded", e => {
  const src = e.detail && e.detail.source;
  if (!src || src !== $("#visore").contentWindow) return;
  const O = src.PDFViewerApplicationOptions;
  const imposta = (k, v) => { try { O.set(k, v); } catch { /* opzione non presente */ } };
  imposta("disablePreferences", true);
  imposta("defaultUrl", "");
  imposta("enableScripting", false); // JavaScript dei PDF spento
  imposta("viewerCssTheme", 2);
  imposta("annotationEditorMode", 0); // note, evidenziatore e penna dalla barra rapida
  imposta("disableHistory", true);
  imposta("localeProperties", { lang: localePdfjs() });
  imposta("enableAltTextModelDownload", false);
  imposta("enableGuessAltText", false);
  imposta("enableSignatureEditor", false);
  imposta("enableComment", false);
  imposta("enableHighlightFloatingButton", false);
  imposta("enableUpdatedAddImage", false);
  imposta("enableNewAltTextWhenAddingImage", false);
  imposta("externalLinkTarget", 0);
  imposta("supportsDownloading", false);
  imposta("sidebarViewOnLoad", 0);
});

function preparaVisore() {
  if (visorePronto && visoreLingua === lingua) return visorePronto;
  visoreLingua = lingua;
  const ifr = $("#visore");
  visorePronto = new Promise((ok, ko) => {
    ifr.onload = async () => {
      try {
        const w = ifr.contentWindow;
        const app = w.PDFViewerApplication;
        await app.initializedPromise;
        sistemaVisore(w, app);
        appV = app;
        ok(app);
      } catch (e) { ko(e); }
    };
    ifr.src = "/pdfjs/web/viewer.html";
  });
  return visorePronto;
}

// sistemaVisore: niente barre di PDF.js (le nostre sono fuori), collegamenti solo con conferma,
// i nostri tasti, i file trascinati al programma.
function sistemaVisore(w, app) {
  const d = w.document;
  // Foglio di stile costruito (la pagina del visualizzatore non accetta <style> in linea).
  const st = new w.CSSStyleSheet();
  st.replaceSync(`
    #toolbarContainer,#viewsManager,#sidebarContainer,#secondaryToolbar,#editorModeButtons,#findbar,#loadingBar{display:none!important}
    :root{--toolbar-height:0px!important;--main-color:#F2F4F8;--body-bg-color:#1d1f27;--toolbar-bg-color:#13151F;--toolbar-border-color:#23273A}
    #viewerContainer{inset:0!important;top:0!important}
    #outerContainer,#mainContainer{min-width:0!important}
    .pdfViewer{padding:12px 0 24px}
    .pdfViewer .page{box-shadow:0 6px 26px rgba(0,0,0,.45);border:0!important;margin:14px auto!important}
    ::-webkit-scrollbar{width:12px;height:12px}::-webkit-scrollbar-thumb{background:#2c3044;border-radius:8px;border:3px solid #1d1f27}::-webkit-scrollbar-track{background:transparent}
  `);
  d.adoptedStyleSheets = [...d.adoptedStyleSheets, st];
  try { w.open = () => null; } catch { /* */ }
  d.addEventListener("click", e => {
    const a = e.target && e.target.closest && e.target.closest("a[href]");
    if (!a) return;
    const href = a.getAttribute("href") || "";
    if (href.startsWith("#") || (a.href.startsWith(location.origin + "/pdfjs/") && a.href.includes("#"))) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    if (/^(https?:|mailto:)/i.test(a.href)) chiediLink(a.href);
  }, true);
  w.addEventListener("keydown", e => {
    if (!(e.ctrlKey || e.metaKey)) return;
    const k = e.key.toLowerCase();
    if (["o", "s", "f", "p"].includes(k)) { e.preventDefault(); e.stopImmediatePropagation(); tastoDoc(k); }
  }, true);
  w.addEventListener("dragover", e => { e.preventDefault(); e.stopImmediatePropagation(); }, true);
  w.addEventListener("drop", e => {
    e.preventDefault(); e.stopImmediatePropagation();
    if (e.dataTransfer && e.dataTransfer.files.length) window.__tasso.gestisciFileTrascinati([...e.dataTransfer.files]);
  }, true);
  const eb = app.eventBus;
  eb.on("pagechanging", ev => { if (D.att) D.att.pagina = ev.pageNumber; aggiornaNav(); evidenziaMini(); });
  eb.on("scalechanging", () => aggiornaNav());
  eb.on("rotationchanging", () => aggiornaNav());
  eb.on("updatefindmatchescount", ev => mostraConteggio(ev.matchesCount));
  eb.on("updatefindcontrolstate", ev => mostraConteggio(ev.matchesCount, ev.state));
}

// ---------------------------------------------------------------- documenti PDF.js per miniature, editor, esporta

const cacheDoc = new Map(); // id voce -> promise del documento PDF.js
function docJS(v) {
  const id = v.voce.id;
  if (!cacheDoc.has(id)) {
    cacheDoc.set(id, pdfjsLib.getDocument({ url: "/file/" + id, password: v.pw || undefined, ...OPZ_PDFJS }).promise);
  }
  return cacheDoc.get(id);
}
function dimenticaDoc(id) {
  const p = cacheDoc.get(id);
  if (p) { p.then(d => d.destroy()).catch(() => { }); cacheDoc.delete(id); }
}

// dimensioni (punti, come si vede) di ogni pagina
async function dimensioniPagina(v, n) {
  const doc = await docJS(v);
  const pag = await doc.getPage(n);
  const vp = pag.getViewport({ scale: 1 });
  return { w: vp.width, h: vp.height };
}

// ---------------------------------------------------------------- apertura

// apriDocumento: apre un PDF in una nuova scheda (chiede la password se serve, lo ripara se è rovinato).
export async function apriDocumento(voce) {
  if (voce.tipo !== "pdf") return avvisa(t("e_non_pdf"));
  const gia = D.schede.find(s => s.originale.id === voce.id);
  if (gia) return attiva(gia);
  let pw = "", info = null;
  try {
    info = await go("goInfo", voce.id, "");
    let sbagliata = false;
    while (info.servePassword && !info.passwordGiusta) {
      const p = await chiediPassword(voce.nome, sbagliata);
      if (p === null) return;
      info = await go("goInfo", voce.id, p);
      if (info.passwordGiusta) pw = p;
      sbagliata = true;
    }
  } catch (e) {
    if (e.codice !== "rovinato") return avvisaErrore(e);
  }
  const sc = { k: ++D.conta, nome: voce.nome, originale: voce, versioni: [{ voce, pw }], pagina: 1, info, chiuse: new Set(), riparato: false, salvata: true };
  // PDF.js deve riuscire a leggerlo; se no, provo a ripararlo.
  try {
    await docJS(sc.versioni[0]);
  } catch {
    dimenticaDoc(voce.id);
    attesa(true, t("riparando"));
    try {
      const r = await go("goRipara", voce.id, pw);
      sc.versioni = [{ voce: r, pw: "" }];
      sc.riparato = true;
      await docJS(sc.versioni[0]);
    } catch {
      attesa(false);
      return avvisa(t("nonRiparabile"));
    }
    attesa(false);
  }
  D.schede.push(sc);
  await attiva(sc);
}

async function attiva(sc) {
  if (D.att && D.att !== sc && appV) D.att.pagina = appV.page || D.att.pagina;
  chiudiStrumento(true);
  D.att = sc;
  vista("doc");
  disegnaSchede();
  disegnaSezioni();
  disegnaPannello();
  await caricaNelVisore();
}

async function caricaNelVisore() {
  const sc = D.att;
  if (!sc) return;
  const v = corrente(sc);
  aggiornaFasce();
  let app;
  try {
    app = await preparaVisore();
    const pag = sc.pagina;
    await app.open({ url: "/file/" + v.voce.id, password: v.pw || undefined, useWorkerFetch: false, isEvalSupported: false });
    const doc = app.pdfDocument;
    app.eventBus.on("pagesinit", function una() {
      app.eventBus.off("pagesinit", una);
      if (pag > 1 && pag <= app.pagesCount) app.page = pag;
      applicaRapido(D.rapido);
    });
    if (doc) {
      try { const f = await doc.getFieldObjects(); sc.moduli = !!(f && Object.keys(f).length); } catch { sc.moduli = false; }
    }
  } catch (e) {
    avvisaErrore(e);
  }
  aggiornaFasce();
  aggiornaNav();
  disegnaMiniature();
  aggiornaAnnulla();
}

// ---------------------------------------------------------------- schede in alto

function disegnaSchede() {
  const box = $("#dTabs");
  box.innerHTML = "";
  for (const sc of D.schede) {
    const b = document.createElement("div");
    b.className = "dTab" + (sc === D.att ? " on" : "");
    b.title = sc.nome;
    b.innerHTML = `<svg class="tf" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg><span class="tn"></span>${!sc.salvata ? '<span class="tm" title="' + t("nonSalvato") + '"></span>' : ""}<button class="tx" title="${t("chiudi")}">${ico("x", 14)}</button>`;
    $(".tn", b).textContent = sc.nome;
    b.onclick = e => { if (!e.target.closest(".tx")) attiva(sc); };
    $(".tx", b).onclick = () => chiudiScheda(sc);
    box.appendChild(b);
  }
  disegnaAperti();
}

async function chiudiScheda(sc) {
  if (!sc.salvata) {
    const r = await chiedi(t("chiudiT"), t("chiudiD", sc.nome), [[t("annulla"), 0], [t("chiudiSenza"), 1], [t("salvaCopia"), 2, true]]);
    if (r === 0) return;
    if (r === 2) { if (D.att !== sc) await attiva(sc); const p = await salvaCopiaDoc(); if (!p) return; }
  }
  const i = D.schede.indexOf(sc);
  D.schede.splice(i, 1);
  for (const v of sc.versioni) {
    dimenticaDoc(v.voce.id);
    if (v.voce.id !== sc.originale.id) go("goDimentica", v.voce.id).catch(() => { });
  }
  if (D.att === sc) {
    D.att = null;
    if (D.schede.length) await attiva(D.schede[Math.max(0, i - 1)]);
    else { if (appV) appV.close().catch(() => { }); vista("home"); }
  }
  disegnaSchede();
}

// I documenti aperti, anche nella schermata iniziale.
function disegnaAperti() {
  const box = $("#hApertiLista");
  if (!box) return;
  box.innerHTML = "";
  for (const sc of D.schede) {
    const c = document.createElement("button");
    c.className = "chip";
    c.innerHTML = ico("pdf", 15) + "<span></span>";
    $("span", c).textContent = sc.nome;
    c.onclick = () => attiva(sc);
    box.appendChild(c);
  }
  $("#hAperti").classList.toggle("hidden", !D.schede.length);
}

// ---------------------------------------------------------------- fasce di avviso

function aggiornaFasce() {
  const sc = D.att, box = $("#dFasce");
  box.innerHTML = "";
  if (!sc) return;
  const f = [];
  if (sc.riparato) f.push(["riparato", "giallo", t("riparato")]);
  if (sc.info && sc.info.cifrato) f.push(["protetto", "giallo", t("fasciaProtetto")]);
  if (sc.moduli) f.push(["moduli", "", t("fasciaModuli")]);
  for (const [k, c, testo] of f) {
    if (sc.chiuse.has(k)) continue;
    const d = document.createElement("div");
    d.className = "fascia " + c;
    d.innerHTML = ico(c ? "avviso" : "info", 17) + `<span></span><button class="dIco" title="${t("chiudi")}">${ico("x", 15)}</button>`;
    $("span", d).textContent = testo;
    $("button", d).onclick = () => { sc.chiuse.add(k); aggiornaFasce(); };
    box.appendChild(d);
  }
}

// ---------------------------------------------------------------- versioni

// consolida: le note, i disegni e i moduli compilati nel visualizzatore diventano una versione,
// prima di uno strumento che lavora sul file.
async function consolida() {
  const sc = D.att;
  const doc = appV && appV.pdfDocument;
  if (!sc || !doc || !doc.annotationStorage || doc.annotationStorage.size === 0) return;
  attesa(true);
  try {
    const bytes = await doc.saveDocument();
    const v = await caricaArray(bytes, sc.nome);
    sc.versioni.push({ voce: v, pw: corrente(sc).pw });
    sc.salvata = false;
  } finally { attesa(false); }
}

async function nuovaVersione(r) {
  const sc = D.att;
  sc.pagina = appV ? appV.page || sc.pagina : sc.pagina;
  sc.versioni.push({ voce: r, pw: "" });
  sc.salvata = false;
  sc.info = Object.assign({}, sc.info || {}, { cifrato: false });
  disegnaSchede();
  await caricaNelVisore();
}

async function annullaModifica() {
  const sc = D.att;
  if (!sc || sc.versioni.length < 2) return;
  const v = sc.versioni.pop();
  dimenticaDoc(v.voce.id);
  go("goDimentica", v.voce.id).catch(() => { });
  if (sc.versioni.length === 1) sc.salvata = true;
  chiudiStrumento(true);
  disegnaSchede();
  disegnaPannello();
  await caricaNelVisore();
}

function aggiornaAnnulla() {
  $("#dAnnulla").classList.toggle("hidden", !D.att || D.att.versioni.length < 2);
}

// esegui: chiama una funzione del programma sulla versione corrente; il risultato diventa una versione nuova.
async function applica(nome, ...args) {
  await consolida();
  const v = corrente(D.att);
  attesa(true);
  let r;
  try { r = await go(nome, v.voce.id, v.pw, ...args); }
  catch (e) { attesa(false); avvisaErrore(e); return null; }
  attesa(false);
  await nuovaVersione(r);
  return r;
}

// produci: come applica, ma il risultato non diventa una versione: si salva subito (Proteggi, Estrai...).
async function produci(nome, suffisso, ...args) {
  await consolida();
  const v = corrente(D.att);
  attesa(true);
  let r;
  try { r = await go(nome, v.voce.id, v.pw, ...args); }
  catch (e) { attesa(false); avvisaErrore(e); return null; }
  attesa(false);
  return r;
}

export async function salvaCopiaDoc() {
  const sc = D.att;
  if (!sc) return "";
  const doc = appV && appV.pdfDocument;
  let id = corrente(sc).voce.id, tmp = null;
  try {
    if (doc && doc.annotationStorage && doc.annotationStorage.size > 0) {
      attesa(true);
      try { tmp = await caricaArray(await doc.saveDocument(), sc.nome); id = tmp.id; } finally { attesa(false); }
    }
  } catch (e) { avvisaErrore(e); return ""; }
  const suff = sc.riparato && !modificato(sc) && !tmp ? t("suffRiparato") : (modificato(sc) || tmp ? t("suffModificato") : t("suffCopia"));
  const p = await salva(id, nomeCon(sc.nome, suff));
  if (tmp) go("goDimentica", tmp.id).catch(() => { });
  if (p) { sc.salvata = true; disegnaSchede(); }
  return p;
}

// ---------------------------------------------------------------- sezioni e pannello

const STRUMENTI = [
  // id, colore, icona, sezioni
  { id: "modifica", c: "rosso", i: "modifica", sez: ["tutti"], vai: "modifica" },
  { id: "organizza", c: "viola", i: "organizza", sez: ["tutti", "modifica"], fai: () => apriOrganizza() },
  { id: "esporta", c: "blu", i: "esporta", sez: ["tutti"], p: pEsporta },
  { id: "crea", c: "verde", i: "crea", sez: ["tutti"], fai: () => creaDaImmagini() },
  { id: "combina", c: "ciano", i: "combina", sez: ["tutti"], fai: () => combina() },
  { id: "firmaDoc", c: "rosa", i: "firma", sez: ["tutti", "firma"], fai: () => firmaDoc() },
  { id: "proteggi", c: "giallo", i: "proteggi", sez: ["tutti"], p: pProteggi },
  { id: "sbloccaDoc", c: "verde", i: "sblocca", sez: ["tutti"], fai: () => apriSblocca(D.att.originale, true) },
  { id: "comprimi", c: "arancio", i: "comprimi", sez: ["tutti"], p: pComprimi },
  { id: "oscura", c: "rosso", i: "oscura", sez: ["tutti"], p: pOscura },
  { id: "converti", c: "blu", i: "converti", sez: ["tutti"], vai: "converti" },
  // Modifica
  { id: "testo", c: "rosso", i: "testo", sez: ["modifica"], p: pTesto, g: "gContenuto" },
  { id: "immagine", c: "verde", i: "immagine", sez: ["modifica"], p: pImmagine },
  { id: "intestazione", c: "blu", i: "intestazione", sez: ["modifica"], p: pIntestazione },
  { id: "filigrana", c: "ciano", i: "filigrana", sez: ["modifica"], p: pFiligrana },
  { id: "link", c: "viola", i: "link", sez: ["modifica"], p: pLink },
  { id: "commenti", c: "giallo", i: "commenti", sez: ["modifica"], p: pCommenti },
  { id: "ruota", c: "arancio", i: "ruotaDx", sez: ["modifica"], p: pRuota, g: "gPagine" },
  { id: "inserisci", c: "verde", i: "inserisci", sez: ["modifica"], p: pInserisci },
  { id: "elimina", c: "rosso", i: "elimina", sez: ["modifica"], p: pElimina },
  { id: "estrai", c: "blu", i: "estrai", sez: ["modifica"], p: pEstrai },
  // Converti
  { id: "pdfImmagini", c: "blu", i: "pdfImmagini", sez: ["converti"], p: pEsporta },
  { id: "pdfTesto", c: "viola", i: "pdfTesto", sez: ["converti"], p: pEsportaTesto },
  { id: "immaginiPdf", c: "verde", i: "crea", sez: ["converti"], fai: () => creaDaImmagini() },
  { id: "ufficio", c: "giallo", i: "ufficio", sez: ["converti"], nd: true },
  // Firma
  { id: "firmaInfo", c: "rosa", i: "timbro", sez: ["firma"], fai: () => firmaDoc() },
];
const strumento = id => STRUMENTI.find(s => s.id === id);

function disegnaSezioni() {
  $$("#dSez [data-sez]").forEach(b => b.classList.toggle("on", b.dataset.sez === D.sez));
}

function vaiSezione(s) {
  D.sez = s;
  chiudiStrumento();
  disegnaSezioni();
  disegnaPannello();
  if (!D.pannello) mostraPannello(true);
}

function disegnaPannello() {
  const pan = $("#dPannello");
  pan.innerHTML = "";
  if (D.strumento) {
    const s = strumento(D.strumento);
    const ind = document.createElement("button");
    ind.className = "pIndietro";
    ind.innerHTML = ico("sx", 15) + `<span>${t("strumentiDi", t("sez_" + D.sez))}</span>`;
    ind.onclick = () => chiudiStrumento();
    pan.appendChild(ind);
    const testa = document.createElement("div");
    testa.className = "pTesta";
    testa.innerHTML = `<span class="pIco" style="--c:${COLORI[s.c]}">${ico(s.i)}</span><h3>${t("s_" + s.id)}</h3>`;
    pan.appendChild(testa);
    const corpo = document.createElement("div");
    pan.appendChild(corpo);
    s.p(corpo);
    return;
  }
  const tit = document.createElement("div");
  tit.className = "pTit";
  tit.textContent = t("sez_" + D.sez);
  pan.appendChild(tit);
  for (const s of STRUMENTI.filter(x => x.sez.includes(D.sez))) {
    if (s.g && D.sez === "modifica") {
      const g = document.createElement("div");
      g.className = "pGruppo";
      g.textContent = t(s.g);
      pan.appendChild(g);
    }
    const b = document.createElement("button");
    b.className = "pVoce" + (s.nd ? " pNd" : "");
    b.dataset.strumento = s.id;
    b.innerHTML = `<span class="pIco" style="--c:${COLORI[s.c]}">${ico(s.i)}</span><span><span class="nm"></span>${s.nd ? `<small>${t("nonDisponibile")}</small>` : ""}</span>`;
    $(".nm", b).textContent = t("s_" + s.id);
    b.title = t("d_" + s.id);
    b.onclick = () => usaStrumento(s);
    pan.appendChild(b);
  }
}

async function usaStrumento(s) {
  if (!D.att) return;
  if (s.nd) return avvisa(t("d_" + s.id), t("s_" + s.id));
  if (s.vai) return vaiSezione(s.vai);
  if (s.fai) return s.fai();
  chiudiStrumento(true);
  D.strumento = s.id;
  disegnaPannello();
}

function chiudiStrumento(muto) {
  if (E.tipo) chiudiEditor();
  if (O.aperto) chiudiOrganizza();
  D.strumento = null;
  if (!muto) disegnaPannello();
}

function mostraPannello(on) {
  D.pannello = on;
  $("#dPannello").classList.toggle("chiuso", !on);
  $("#dChiudiPan").classList.toggle("chiuso", !on);
  $("#dChiudiPan").innerHTML = ico(on ? "sx" : "dx", 15);
}

// ---------------------------------------------------------------- piccoli aiuti per i pannelli

function el(tag, cls, html) { const e = document.createElement(tag); if (cls) e.className = cls; if (html !== undefined) e.innerHTML = html; return e; }
function testoP(box, chiave, cls = "pDesc") { const p = el("p", cls); p.textContent = t(chiave); box.appendChild(p); return p; }
function campo(box, etichetta, input) { const l = el("label", "pCampo"); const s = el("span"); s.textContent = etichetta; l.append(s, input); box.appendChild(l); return input; }
function inp(tipo, valore = "", attr = {}) { const i = document.createElement("input"); i.type = tipo; i.value = valore; i.spellcheck = false; i.autocomplete = "off"; Object.assign(i, attr); return i; }
function sel(opzioni, valore) { const s = document.createElement("select"); for (const [v, n] of opzioni) { const o = document.createElement("option"); o.value = v; o.textContent = n; s.appendChild(o); } s.value = valore; return s; }
function spunta(box, chiave, on = false) { const l = el("label", "pCheck"); const i = inp("checkbox"); i.checked = on; const s = el("span"); s.textContent = t(chiave); l.append(i, s); box.appendChild(l); return i; }
function bottone(box, chiave, prim, f) { const b = el("button", "btn" + (prim ? " prim" : "")); b.textContent = t(chiave); b.onclick = f; box.appendChild(b); return b; }
function azioni(box) { const a = el("div", "pAzioni"); box.appendChild(a); return a; }
function scelte(opzioni, valore, onCambia) {
  const box = el("div", "pScelte");
  box.valore = valore;
  for (const [v, n] of opzioni) {
    const b = el("button");
    b.textContent = n;
    b.classList.toggle("on", v === valore);
    b.onclick = () => { box.valore = v; $$("button", box).forEach(x => x.classList.toggle("on", x === b)); onCambia && onCambia(v); };
    box.appendChild(b);
  }
  return box;
}
function colori(valore, onCambia, lista = ["#111111", "#1a3ea8", "#C62828", "#2E7D32", "#6A1B9A", "#888888", "#FFFFFF"]) {
  const box = el("div", "pColori");
  box.valore = valore;
  for (const c of lista) {
    const b = el("button");
    b.style.background = c;
    b.title = c;
    b.classList.toggle("on", c === valore);
    b.onclick = () => { box.valore = c; $$("button", box).forEach(x => x.classList.toggle("on", x === b)); onCambia && onCambia(c); };
    box.appendChild(b);
  }
  return box;
}

const totPagine = () => (appV && appV.pagesCount) || 0;

// pagineDa: "1-3, 5, 8-" -> [1,2,3,5,8,...]; "" = tutte; null se non va bene.
export function pagineDa(s, n) {
  s = String(s || "").trim().toLowerCase();
  if (!s || s === t("tutte").toLowerCase()) return Array.from({ length: n }, (_, i) => i + 1);
  const set = new Set();
  for (const parte of s.split(/[,; ]+/).filter(Boolean)) {
    const m = /^(\d*)\s*[-–]\s*(\d*)$/.exec(parte);
    let a, b;
    if (m) { a = m[1] ? +m[1] : 1; b = m[2] ? +m[2] : n; }
    else if (/^\d+$/.test(parte)) { a = b = +parte; }
    else return null;
    if (a < 1 || b < a || a > n) return null;
    for (let i = a; i <= Math.min(b, n); i++) set.add(i);
  }
  return set.size ? [...set].sort((x, y) => x - y) : null;
}

function campoPagine(box, chiave = "quali") {
  const i = inp("text", "", { placeholder: t("esempioPagine") });
  campo(box, t(chiave), i);
  const p = el("p", "pPiccolo");
  p.textContent = t("pagineAiuto", totPagine());
  box.appendChild(p);
  return i;
}

function leggiPagine(i, vuotoTutte = true) {
  const n = totPagine();
  if (!i.value.trim() && !vuotoTutte) { avvisa(t("e_intervallo")); return null; }
  const pp = pagineDa(i.value, n);
  if (!pp) { avvisa(t("e_intervallo")); return null; }
  return pp;
}

const sequenzaDi = n => Array.from({ length: n }, (_, i) => ({ f: 0, p: i + 1, r: 0 }));

// ---------------------------------------------------------------- pannelli: pagine (scorciatoie)

function pRuota(box) {
  testoP(box, "d_ruota");
  const qp = campoPagine(box);
  const a = azioni(box);
  const gira = async gradi => {
    const pp = leggiPagine(qp); if (!pp) return;
    const set = new Set(pp);
    await applica("goOrganizza", [], sequenzaDi(totPagine()).map(s => set.has(s.p) ? { ...s, r: gradi } : s));
  };
  const r = el("div", "pRiga");
  bottone(r, "girSx", false, () => gira(270)).innerHTML = ico("ruotaSx", 16) + " " + t("girSx");
  bottone(r, "girDx", false, () => gira(90)).innerHTML = ico("ruotaDx", 16) + " " + t("girDx");
  a.appendChild(r);
  bottone(a, "apriOrganizza", false, () => apriOrganizza());
}

function pElimina(box) {
  testoP(box, "d_elimina");
  const qp = inp("text", "", { placeholder: t("esempioPagine") });
  campo(box, t("quali"), qp);
  const a = azioni(box);
  bottone(a, "eliminaPagine", true, async () => {
    const pp = leggiPagine(qp, false); if (!pp) return;
    const set = new Set(pp);
    const seq = sequenzaDi(totPagine()).filter(s => !set.has(s.p));
    if (!seq.length) return avvisa(t("e_nessuna_pagina"));
    await applica("goOrganizza", [], seq);
  });
  bottone(a, "apriOrganizza", false, () => apriOrganizza());
}

function pEstrai(box) {
  testoP(box, "d_estrai");
  const qp = inp("text", "", { placeholder: t("esempioPagine") });
  campo(box, t("quali"), qp);
  const a = azioni(box);
  bottone(a, "estraiSalva", true, async () => {
    const pp = leggiPagine(qp, false); if (!pp) return;
    await estraiESalva(pp.map(p => ({ f: 0, p, r: 0 })), []);
  });
}

async function estraiESalva(seq, altri) {
  const r = await produci("goOrganizza", null, altri, seq);
  if (!r) return;
  await salva(r.id, nomeCon(D.att.nome, t("suffEstratto")));
  go("goDimentica", r.id).catch(() => { });
}

function pInserisci(box) {
  testoP(box, "d_inserisci");
  const n = totPagine();
  const cosa = scelte([["pdf", t("daAltroPDF")], ["vuota", t("paginaVuota")]], "pdf");
  campo(box, t("cosaInserire"), cosa);
  const dove = sel([["fine", t("allaFine")], ["inizio", t("allInizio")], ["dopo", t("dopoPagina")]], "fine");
  campo(box, t("dove"), dove);
  const dopo = inp("number", String(D.att.pagina || 1), { min: 1, max: n });
  const ld = campo(box, t("numeroPagina"), dopo).parentElement;
  ld.classList.add("hidden");
  dove.onchange = () => ld.classList.toggle("hidden", dove.value !== "dopo");
  const a = azioni(box);
  bottone(a, "inserisci", true, async () => {
    let pos = dove.value === "fine" ? n : dove.value === "inizio" ? 0 : Math.max(0, Math.min(n, parseInt(dopo.value, 10) || 0));
    const seq = sequenzaDi(n);
    let nuove, altri = [];
    if (cosa.valore === "vuota") {
      const d = await dimensioniPagina(corrente(D.att), Math.max(1, Math.min(n, pos || 1)));
      nuove = [{ f: -1, p: 0, r: 0, w: d.w, h: d.h }];
    } else {
      const altro = await scegliAltroPDF();
      if (!altro) return;
      altri = [{ id: altro.voce.id, pw: altro.pw }];
      nuove = Array.from({ length: altro.pagine }, (_, i) => ({ f: 1, p: i + 1, r: 0 }));
    }
    seq.splice(pos, 0, ...nuove);
    await applica("goOrganizza", altri, seq);
  });
}

// scegliAltroPDF: chiede un PDF (e la sua password, se serve). { voce, pw, pagine }
async function scegliAltroPDF() {
  const vv = await scegli(false, true, t("titoloApri"));
  if (!vv.length) return null;
  const v = vv[0];
  try {
    let inf = await go("goInfo", v.id, ""), pw = "", sbagliata = false;
    while (inf.servePassword && !inf.passwordGiusta) {
      const p = await chiediPassword(v.nome, sbagliata);
      if (p === null) return null;
      inf = await go("goInfo", v.id, p);
      if (inf.passwordGiusta) pw = p;
      sbagliata = true;
    }
    return { voce: v, pw, pagine: inf.pagine || 1 };
  } catch (e) { avvisaErrore(e); return null; }
}

// ---------------------------------------------------------------- pannelli: tutti gli strumenti

function pProteggi(box) {
  testoP(box, "d_proteggi");
  const p1 = campo(box, t("pwApertura"), inp("password", "", { placeholder: t("facoltativa") }));
  const p2 = campo(box, t("pwRipeti"), inp("password"));
  const blocca = spunta(box, "limitaPermessi");
  const pb = el("div", "hidden");
  const p3 = campo(pb, t("pwPermessi"), inp("password"));
  const st = spunta(pb, "permStampa", true), cp = spunta(pb, "permCopia", false), md = spunta(pb, "permModifica", false);
  box.appendChild(pb);
  blocca.onchange = () => pb.classList.toggle("hidden", !blocca.checked);
  const mostra = spunta(box, "mostraPassword");
  mostra.onchange = () => [p1, p2, p3].forEach(i => i.type = mostra.checked ? "text" : "password");
  testoP(box, "ricordaPassword", "pAvviso");
  const a = azioni(box);
  bottone(a, "proteggiSalva", true, async () => {
    if (!p1.value && !blocca.checked) return avvisa(t("e_vuoto_proteggi"));
    if (p1.value !== p2.value) return avvisa(t("e_pw_diverse"));
    const o = { apertura: p1.value, blocca: blocca.checked, permessi: p3.value, stampa: st.checked, copia: cp.checked, modifica: md.checked };
    const r = await produci("goProteggi", null, o);
    if (!r) return;
    await salva(r.id, nomeCon(D.att.nome, t("suffProtetto")));
    go("goDimentica", r.id).catch(() => { });
  });
}

const mb = n => (n / 1048576).toLocaleString(lingua, { maximumFractionDigits: n < 10485760 ? 2 : 1 }) + " MB";

function pComprimi(box) {
  testoP(box, "d_comprimi");
  const modo = scelte([["leggera", t("compLeggera")], ["forte", t("compForte")]], "forte", v => spiega.textContent = t(v === "forte" ? "compForteD" : "compLeggeraD"));
  campo(box, t("livello"), modo);
  const spiega = testoP(box, "compForteD", "pPiccolo");
  const ris = el("div", "pRisultato hidden");
  const a = azioni(box);
  let ultimo = null;
  const salvaB = el("button", "btn prim hidden");
  salvaB.textContent = t("salvaCompressa");
  bottone(a, "comprimiB", true, async () => {
    if (ultimo) { go("goDimentica", ultimo.id).catch(() => { }); ultimo = null; }
    const prima = corrente(D.att).voce.dim;
    const r = await produci("goComprimi", null, modo.valore === "forte");
    if (!r) return;
    ultimo = r;
    ris.classList.remove("hidden");
    if (r.dim < prima) {
      const perc = Math.round((1 - r.dim / prima) * 100);
      ris.innerHTML = `${t("compDa")}<b><span class="verde"></span></b><span></span>`;
      $(".verde", ris).textContent = t("daA", mb(prima), mb(r.dim));
      ris.lastChild.textContent = t("compPerc", perc);
      salvaB.classList.remove("hidden");
    } else {
      ris.textContent = t("compGiaOttimo");
      salvaB.classList.add("hidden");
    }
  });
  box.insertBefore(ris, a);
  a.appendChild(salvaB);
  salvaB.onclick = async () => { if (ultimo) await salva(ultimo.id, nomeCon(D.att.nome, t("suffCompresso"))); };
}

function pEsporta(box) {
  testoP(box, "d_esporta");
  const fmt = scelte([["png", "PNG"], ["jpg", "JPG"]], "png");
  campo(box, t("formato"), fmt);
  const ris = sel([["1.5", t("risNormale")], ["2.0833", t("risBuona")], ["4.1667", t("risAlta")]], "2.0833");
  campo(box, t("risoluzione"), ris);
  const qp = campoPagine(box);
  const a = azioni(box);
  bottone(a, "esportaImmagini", true, async () => {
    const pp = leggiPagine(qp); if (!pp) return;
    await esportaImmagini(pp, fmt.valore, parseFloat(ris.value));
  });
  if (D.strumento === "esporta") {
    testoP(box, "oppure", "pPiccolo");
    bottone(azioni(box), "esportaTesto", false, () => esportaTesto());
  }
}

function pEsportaTesto(box) {
  testoP(box, "d_pdfTesto");
  testoP(box, "testoScansioni", "pAvviso");
  bottone(azioni(box), "esportaTesto", true, () => esportaTesto());
}

async function esportaImmagini(pp, fmt, scala) {
  await consolida();
  const v = corrente(D.att);
  let tok;
  try { tok = await go("goSerieInizio", v.voce.id, D.att.nome.replace(/\.pdf$/i, "") + "." + fmt, t("titoloEsporta"), fmt); }
  catch (e) { return avvisaErrore(e); }
  if (!tok) return;
  let ultimo = "";
  try {
    const doc = await docJS(v);
    for (let i = 0; i < pp.length; i++) {
      attesa(true, t("esportoPagina", i + 1, pp.length));
      const pag = await doc.getPage(pp[i]);
      const vp = pag.getViewport({ scale: scala });
      const c = document.createElement("canvas");
      c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
      const x = c.getContext("2d");
      x.fillStyle = "#fff"; x.fillRect(0, 0, c.width, c.height);
      await pag.render({ canvasContext: x, viewport: vp }).promise;
      const blob = await new Promise(ok => c.toBlob(ok, fmt === "jpg" ? "image/jpeg" : "image/png", 0.9));
      const u8 = new Uint8Array(await blob.arrayBuffer());
      ultimo = await go("goSerieScrivi", tok, pp[i], base64DaBytes(u8));
      attesa(false);
      pag.cleanup();
    }
  } catch (e) { attesa(false); avvisaErrore(e); }
  go("goSerieFine", tok).catch(() => { });
  if (ultimo) toast(ultimo);
}

async function esportaTesto() {
  await consolida();
  const v = corrente(D.att);
  attesa(true);
  let r;
  try {
    const doc = await docJS(v);
    const righe = [];
    for (let n = 1; n <= doc.numPages; n++) {
      const pag = await doc.getPage(n);
      const tc = await pag.getTextContent();
      let s = "";
      for (const it of tc.items) { s += it.str || ""; if (it.hasEOL) s += "\n"; }
      righe.push(s.replace(/[ \t]+\n/g, "\n").trim());
    }
    const testo = righe.map((s, i) => `--- ${t("pagina", i + 1, righe.length)} ---\n${s}`).join("\n\n") + "\n";
    if (!righe.join("").trim()) { attesa(false); return avvisa(t("nessunTesto")); }
    r = await go("goCaricaTesto", D.att.nome.replace(/\.pdf$/i, "") + ".txt", testo);
  } catch (e) { attesa(false); return avvisaErrore(e); }
  attesa(false);
  const p = await go("goSalvaTipo", r.id, D.att.nome.replace(/\.pdf$/i, "") + ".txt", t("titoloSalva"), "txt").catch(e => { avvisaErrore(e); return ""; });
  if (p) toast(p);
  go("goDimentica", r.id).catch(() => { });
}

function creaDaImmagini() { scegli(true, false, t("titoloApriMolti")).then(vv => { if (vv.length) apriUnisci(vv, true); }); }
async function combina() { await consolida(); const v = corrente(D.att); apriUnisci([Object.assign({}, v.voce, { nome: D.att.nome })], true, v.pw); }
async function firmaDoc() {
  await consolida();
  const v = corrente(D.att);
  apriFirmaDaDocumento(Object.assign({}, v.voce, { nome: D.att.nome }), v.pw, async r => { vista("doc"); await nuovaVersione(r); });
}

function pCommenti(box) {
  testoP(box, "d_commenti");
  for (const [k, i] of [["r_nota", "nota"], ["r_evidenzia", "evidenzia"], ["r_penna", "penna"]]) {
    const b = el("button", "pVoce");
    b.innerHTML = `<span class="pIco" style="--c:${COLORI.giallo}">${ico(i)}</span><span></span>`;
    b.lastChild.textContent = t(k);
    b.onclick = () => applicaRapido(i);
    box.appendChild(b);
  }
  testoP(box, "commentiSalvati", "pPiccolo");
}

// ---------------------------------------------------------------- testo nei caratteri standard

// Le stesse lettere che il programma sa scrivere come testo vero (Windows-1252).
const W1252 = "€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ";
const testoAdatto = s => [...s].every(c => { const n = c.codePointAt(0); return c === "\n" || c === "\t" || (n >= 32 && n < 127) || (n >= 0xa0 && n <= 0xff) || W1252.includes(c); });

const FONT_CSS = { helv: "Arial, Helvetica, 'Liberation Sans', sans-serif", times: "'Times New Roman', Times, 'Liberation Serif', serif", cour: "'Courier New', Courier, 'Liberation Mono', monospace" };
const famiglia = f => FONT_CSS[f.replace(/b$/, "")] || FONT_CSS.helv;
const grassetto = f => /b$/.test(f);

// pngTestoBox: il testo come immagine (per le lettere che i caratteri standard non hanno).
function pngTestoBox(testo, font, dimPt, colore, allinea = "s") {
  const k = 4; // pixel per punto
  const righe = testo.split("\n");
  const c = document.createElement("canvas");
  const x = c.getContext("2d");
  const f = `${grassetto(font) ? "700" : "400"} ${dimPt * k}px ${famiglia(font)}`;
  x.font = f;
  const w = Math.max(...righe.map(r => x.measureText(r).width), 1);
  c.width = Math.ceil(w) + 4; c.height = Math.ceil(righe.length * dimPt * 1.2 * k);
  x.font = f; x.fillStyle = colore; x.textBaseline = "alphabetic";
  righe.forEach((r, i) => {
    const lw = x.measureText(r).width;
    const xx = allinea === "c" ? (c.width - lw) / 2 : allinea === "d" ? c.width - lw - 2 : 2;
    x.fillText(r, xx, (i * 1.2 + 0.95) * dimPt * k);
  });
  return { png: c.toDataURL("image/png"), wPt: c.width / k, hPt: c.height / k };
}

// misuraTesto: larghezza e altezza (punti) del testo nel carattere scelto.
function misuraTesto(testo, font, dimPt) {
  const x = document.createElement("canvas").getContext("2d");
  x.font = `${grassetto(font) ? "700" : "400"} ${dimPt}px ${famiglia(font)}`;
  const righe = testo.split("\n");
  return { w: Math.max(...righe.map(r => x.measureText(r).width), 1) + dimPt * 0.3, h: righe.length * dimPt * 1.2 };
}

// elementoTesto: testo vero se possibile, altrimenti immagine nello stesso posto.
function elementoTesto(base, testo, font, dim, colore, allinea) {
  if (testoAdatto(testo)) return { ...base, tipo: "testo", testo, font, dim, colore, allinea };
  const im = pngTestoBox(testo, font, dim, colore, allinea);
  return { ...base, tipo: "immagine", img: im.png };
}

// ---------------------------------------------------------------- pannelli: intestazione e filigrana

function pIntestazione(box) {
  testoP(box, "d_intestazione");
  const alto = campo(box, t("testoAlto"), inp("text", "", { placeholder: t("esempioAlto") }));
  const altoA = scelte([["s", t("sinistra")], ["c", t("centro")], ["d", t("destra")]], "c");
  box.appendChild(altoA);
  const basso = campo(box, t("testoBasso"), inp("text"));
  const bassoA = scelte([["s", t("sinistra")], ["c", t("centro")], ["d", t("destra")]], "s");
  box.appendChild(bassoA);
  box.appendChild(el("div", "", "&nbsp;"));
  const num = sel([["", t("numNessuno")], ["lungo", t("numLungo", 1, 9)], ["corto", "1 / 9"], ["solo", "1"]], "lungo");
  campo(box, t("numeriPagina"), num);
  const numA = sel([["bd", t("bassoDestra")], ["bc", t("bassoCentro")], ["ad", t("altoDestra")]], "bc");
  campo(box, t("posizione"), numA);
  const data = spunta(box, "aggiungiData");
  const r = el("div", "pRiga");
  const font = sel([["helv", "Sans"], ["times", "Serif"], ["cour", "Mono"]], "helv");
  const dim = inp("number", "10", { min: 6, max: 24 });
  campo(r, t("carattere"), font); campo(r, t("dimensione"), dim);
  box.appendChild(r);
  const qp = campoPagine(box, "suQualiPagine");
  bottone(azioni(box), "applica", true, async () => {
    const n = totPagine();
    if (!leggiPagine(qp)) return;
    const pagine = qp.value.trim() ? qp.value : "";
    const d = Math.max(6, Math.min(24, parseFloat(dim.value) || 10));
    const P = await dimensioniPagina(corrente(D.att), 1);
    const m = 0.045, altezza = d * 1.25 / P.h;
    const yA = 26 / P.h, yB = 1 - 26 / P.h - altezza;
    const ee = [];
    const voce = (testo, y, all) => {
      if (!testo) return;
      const base = { pagina: 0, pagine, x: m, y, l: 1 - 2 * m, a: altezza };
      if (testoAdatto(testo)) ee.push({ ...base, tipo: "testo", testo, font: font.value, dim: d, colore: "#222222", allinea: all });
      else {
        const im = pngTestoBox(testo, font.value, d, "#222222");
        const l = im.wPt / P.w, a = im.hPt / P.h;
        ee.push({ ...base, tipo: "immagine", img: im.png, l, a, x: all === "c" ? (1 - l) / 2 : all === "d" ? 1 - m - l : m });
      }
    };
    voce(alto.value.trim(), yA, altoA.valore);
    voce(basso.value.trim(), yB, bassoA.valore);
    if (num.value) {
      const tpl = num.value === "lungo" ? t("numLungo", "{p}", "{n}") : num.value === "corto" ? "{p} / {n}" : "{p}";
      const pos = numA.value;
      ee.push({ tipo: "testo", pagina: 0, pagine, x: m, y: pos[0] === "a" ? yA : yB, l: 1 - 2 * m, a: altezza, testo: tpl, font: font.value, dim: d, colore: "#222222", allinea: pos[1] === "c" ? "c" : "d" });
    }
    if (data.checked) ee.push({ tipo: "testo", pagina: 0, pagine, x: m, y: yA, l: 1 - 2 * m, a: altezza, testo: oggi(), font: font.value, dim: d, colore: "#222222", allinea: altoA.valore === "d" ? "s" : "d" });
    if (!ee.length) return avvisa(t("e_vuoto_intestazione"));
    if (n) await applica("goDisegna", ee);
  });
}

function pFiligrana(box) {
  testoP(box, "d_filigrana");
  let immagine = null;
  const tipo = scelte([["testo", t("testo")], ["immagine", t("s_immagine")]], "testo", v => { bt.classList.toggle("hidden", v !== "testo"); bi.classList.toggle("hidden", v !== "immagine"); });
  box.appendChild(tipo);
  box.appendChild(el("div", "", "&nbsp;"));
  const bt = el("div"), bi = el("div", "hidden");
  const testo = campo(bt, t("testo"), inp("text", t("filigranaEsempio")));
  const r = el("div", "pRiga");
  const font = sel([["helvb", "Sans"], ["timesb", "Serif"], ["courb", "Mono"]], "helvb");
  const dim = inp("number", "64", { min: 10, max: 200 });
  campo(r, t("carattere"), font); campo(r, t("dimensione"), dim);
  bt.appendChild(r);
  let colore = "#C62828";
  campo(bt, t("colore"), colori(colore, c => colore = c, ["#C62828", "#888888", "#111111", "#1a3ea8", "#2E7D32"]));
  const file = inp("file"); file.accept = "image/png,image/jpeg"; file.className = "hidden";
  const scegliB = bottone(bi, "scegliImmagine", false, () => file.click());
  const nomeI = el("p", "pPiccolo"); bi.appendChild(nomeI); bi.appendChild(file);
  file.onchange = async () => { const f = file.files[0]; if (!f) return; immagine = await leggiDataURL(f); nomeI.textContent = f.name; scegliB.textContent = t("cambiaImmagine"); };
  const scala = inp("range", "50", { min: 10, max: 100 }); scala.className = "pRange";
  campo(bi, t("larghezzaPagina"), scala);
  box.append(bt, bi);
  const op = inp("range", "25", { min: 5, max: 100 }); op.className = "pRange";
  campo(box, t("opacita"), op);
  const rot = sel([["45", "45°"], ["0", "0°"], ["90", "90°"], ["-45", "−45°"]], "45");
  campo(box, t("rotazione"), rot);
  const dietro = spunta(box, "dietroContenuto");
  const qp = campoPagine(box, "suQualiPagine");
  bottone(azioni(box), "applica", true, async () => {
    if (!leggiPagine(qp)) return;
    const pagine = qp.value.trim() ? qp.value : "";
    const P = await dimensioniPagina(corrente(D.att), 1);
    const base = { pagina: 0, pagine, opacita: (+op.value) / 100, rot: +rot.value, sotto: dietro.checked };
    let e;
    if (tipo.valore === "testo") {
      const s = testo.value.trim();
      if (!s) return avvisa(t("e_vuoto_filigrana"));
      const d = Math.max(10, Math.min(200, parseFloat(dim.value) || 64));
      const ms = misuraTesto(s, font.value, d);
      const l = Math.min(1.6, ms.w / P.w), a = ms.h / P.h;
      e = elementoTesto({ ...base, x: (1 - l) / 2, y: (1 - a) / 2, l, a }, s, font.value, d, colore, "c");
      if (e.tipo === "immagine") { const im = pngTestoBox(s, font.value, d, colore); e.l = im.wPt / P.w; e.a = im.hPt / P.h; e.x = (1 - e.l) / 2; e.y = (1 - e.a) / 2; }
    } else {
      if (!immagine) return avvisa(t("e_vuoto_immagine"));
      const { w, h } = await misuraImg(immagine);
      const l = (+scala.value) / 100, a = l * (h / w) * (P.w / P.h);
      e = { ...base, tipo: "immagine", img: immagine, x: (1 - l) / 2, y: (1 - a) / 2, l, a };
    }
    await applica("goDisegna", [e]);
  });
}

const leggiDataURL = f => new Promise((ok, ko) => { const r = new FileReader(); r.onload = () => ok(r.result); r.onerror = ko; r.readAsDataURL(f); });
const misuraImg = src => new Promise((ok, ko) => { const i = new Image(); i.onload = () => ok({ w: i.naturalWidth, h: i.naturalHeight }); i.onerror = ko; i.src = src; });

// ---------------------------------------------------------------- editor di pagina (testo, immagine, link, oscura)

const E = { tipo: null, pag: 1, n: 0, elementi: [], sel: null, dimPag: null, pxPt: 1, doc: null, conta: 0, pannelloSel: null };

async function apriEditor(tipo, aiuto) {
  await consolida();
  const v = corrente(D.att);
  E.tipo = tipo; E.elementi = []; E.sel = null;
  E.doc = await docJS(v);
  E.n = E.doc.numPages;
  E.pag = Math.min(E.n, (appV && appV.page) || 1);
  $("#dEditor").classList.remove("hidden");
  $("#dEditor").classList.toggle("disegna", tipo === "link" || tipo === "oscura" || tipo === "testo");
  $("#eAiuto").textContent = aiuto || "";
  aggiornaRapidi();
  await disegnaPaginaEditor();
}

function chiudiEditor() {
  E.pannelloSel = null;
  E.tipo = null; E.elementi = []; E.sel = null;
  $("#dEditor").classList.add("hidden");
  aggiornaRapidi();
  aggiornaNav();
}

let renderEditor = null;
async function disegnaPaginaEditor() {
  if (!E.tipo) return;
  const pag = await E.doc.getPage(E.pag);
  const v1 = pag.getViewport({ scale: 1 });
  E.dimPag = { w: v1.width, h: v1.height };
  const area = $("#dEditor");
  const maxW = Math.max(300, area.clientWidth - 60), maxH = Math.max(300, area.clientHeight - 80);
  const sc = Math.min(maxW / v1.width, maxH / v1.height, 2);
  E.pxPt = sc;
  const dpr = window.devicePixelRatio || 1;
  const vp = pag.getViewport({ scale: sc * dpr });
  const c = $("#eCanvas");
  c.width = Math.floor(vp.width); c.height = Math.floor(vp.height);
  c.style.width = Math.floor(v1.width * sc) + "px"; c.style.height = Math.floor(v1.height * sc) + "px";
  if (renderEditor) { try { renderEditor.cancel(); } catch { /* */ } }
  renderEditor = pag.render({ canvasContext: c.getContext("2d"), viewport: vp });
  try { await renderEditor.promise; } catch { /* annullato */ }
  renderEditor = null;
  $("#ePagNum").textContent = t("pagina", E.pag, E.n);
  $("#ePrec").disabled = E.pag <= 1; $("#eSucc").disabled = E.pag >= E.n;
  disegnaElementiEditor();
  aggiornaNav();
}

function vaiPaginaEditor(n) { if (n >= 1 && n <= E.n && n !== E.pag) { E.pag = n; disegnaPaginaEditor(); } }

function disegnaElementiEditor() {
  const fo = $("#eFoglio");
  $$(".el", fo).forEach(e => e.remove());
  for (const it of E.elementi) {
    if (it.pagina !== E.pag) continue;
    const d = document.createElement("div");
    d.className = "el " + it.tipo + (E.sel === it ? " sel" : "");
    Object.assign(d.style, { left: it.x * 100 + "%", top: it.y * 100 + "%", width: it.l * 100 + "%", height: it.a * 100 + "%" });
    if (it.tipo === "immagine") { const img = new Image(); img.src = it.img; img.alt = ""; d.appendChild(img); }
    if (it.tipo === "testo") {
      const s = el("div", "tt");
      s.textContent = it.testo;
      Object.assign(s.style, { fontFamily: famiglia(it.font), fontWeight: grassetto(it.font) ? 700 : 400, fontSize: it.dim * E.pxPt + "px", color: it.colore });
      d.appendChild(s);
    }
    if (it.tipo === "link") { const l = el("span", "lb"); l.textContent = it.url ? it.url : it.verso ? t("versoPagina", it.verso) : t("linkDaImpostare"); d.appendChild(l); }
    const ang = el("div", "ang"); d.appendChild(ang);
    const x = el("button", "x"); x.textContent = "✕"; x.title = t("elimina");
    x.onpointerdown = e => e.stopPropagation();
    x.onclick = () => togliElemento(it);
    d.appendChild(x);
    trascinabileE(d, it, ang);
    fo.appendChild(d);
  }
  if (E.pannelloSel) E.pannelloSel(E.sel);
}

function togliElemento(it) {
  E.elementi = E.elementi.filter(e => e !== it);
  if (E.sel === it) E.sel = null;
  disegnaElementiEditor();
}

function selezionaE(it) { E.sel = it; disegnaElementiEditor(); }

// adattaTesto: la casella del testo segue il testo (larghezza e altezza).
function adattaTesto(it) {
  const m = misuraTesto(it.testo || " ", it.font, it.dim);
  it.l = Math.min(1, m.w / E.dimPag.w); it.a = Math.min(1, m.h / E.dimPag.h);
  it.x = Math.min(it.x, 1 - it.l); it.y = Math.min(it.y, 1 - it.a);
}

function trascinabileE(div, it, ang) {
  div.addEventListener("pointerdown", e => {
    if (e.button !== 0) return;
    e.preventDefault(); e.stopPropagation();
    const fo = $("#eFoglio").getBoundingClientRect();
    const ridim = e.target === ang;
    const x0 = e.clientX, y0 = e.clientY;
    const s = { x: it.x, y: it.y, l: it.l, a: it.a, dim: it.dim };
    if (E.sel !== it) { E.sel = it; $$("#eFoglio .el").forEach(d => d.classList.remove("sel")); div.classList.add("sel"); if (E.pannelloSel) E.pannelloSel(it); }
    div.setPointerCapture(e.pointerId);
    const muovi = ev => {
      const dx = (ev.clientX - x0) / fo.width, dy = (ev.clientY - y0) / fo.height;
      if (ridim) {
        if (it.tipo === "testo") {
          it.dim = Math.max(4, Math.min(300, s.dim * Math.max(0.1, (s.l + dx) / s.l)));
          adattaTesto(it);
          const tt = $(".tt", div); if (tt) tt.style.fontSize = it.dim * E.pxPt + "px";
        } else if (it.tipo === "immagine") {
          const rap = s.a / s.l;
          let l = Math.max(0.02, Math.min(1 - s.x, s.l + dx)), a = l * rap;
          if (s.y + a > 1) { a = 1 - s.y; l = a / rap; }
          it.l = l; it.a = a;
        } else {
          it.l = Math.max(0.01, Math.min(1 - s.x, s.l + dx)); it.a = Math.max(0.005, Math.min(1 - s.y, s.a + dy));
        }
      } else {
        it.x = Math.max(0, Math.min(1 - it.l, s.x + dx));
        it.y = Math.max(0, Math.min(1 - it.a, s.y + dy));
      }
      Object.assign(div.style, { left: it.x * 100 + "%", top: it.y * 100 + "%", width: it.l * 100 + "%", height: it.a * 100 + "%" });
    };
    const fine = () => { div.removeEventListener("pointermove", muovi); div.removeEventListener("pointerup", fine); if (E.pannelloSel) E.pannelloSel(it); };
    div.addEventListener("pointermove", muovi);
    div.addEventListener("pointerup", fine);
  });
}

// Disegnare un'area (link, oscura) o mettere un testo con un clic sulla pagina.
function initEditor() {
$("#eFoglio").addEventListener("pointerdown", e => {
  if (!E.tipo || e.target.id !== "eCanvas" || e.button !== 0) return;
  const fo = $("#eFoglio").getBoundingClientRect();
  const px = (e.clientX - fo.left) / fo.width, py = (e.clientY - fo.top) / fo.height;
  if (E.tipo === "testo") {
    const it = { id: ++E.conta, tipo: "testo", pagina: E.pag, x: px, y: py, l: .1, a: .02, testo: t("testoEsempio"), font: T.font, dim: T.dim, colore: T.colore };
    adattaTesto(it);
    it.y = Math.max(0, Math.min(1 - it.a, py - it.a / 2));
    E.elementi.push(it);
    selezionaE(it);
    setTimeout(() => { const ta = $("#tTesto"); if (ta) { ta.focus(); ta.select(); } }, 30);
    return;
  }
  if (E.tipo !== "link" && E.tipo !== "oscura") { if (E.sel) selezionaE(null); return; }
  e.preventDefault();
  const it = { id: ++E.conta, tipo: E.tipo, pagina: E.pag, x: px, y: py, l: 0, a: 0, url: "", verso: 0 };
  const d = el("div", "el " + E.tipo);
  Object.assign(d.style, { left: px * 100 + "%", top: py * 100 + "%", width: 0, height: 0 });
  $("#eFoglio").appendChild(d);
  const fog = $("#eFoglio");
  fog.setPointerCapture(e.pointerId);
  const muovi = ev => {
    const qx = Math.max(0, Math.min(1, (ev.clientX - fo.left) / fo.width)), qy = Math.max(0, Math.min(1, (ev.clientY - fo.top) / fo.height));
    it.x = Math.min(px, qx); it.y = Math.min(py, qy); it.l = Math.abs(qx - px); it.a = Math.abs(qy - py);
    Object.assign(d.style, { left: it.x * 100 + "%", top: it.y * 100 + "%", width: it.l * 100 + "%", height: it.a * 100 + "%" });
  };
  const fine = () => {
    fog.removeEventListener("pointermove", muovi); fog.removeEventListener("pointerup", fine);
    d.remove();
    if (it.l * fo.width < 6 || it.a * fo.height < 6) { disegnaElementiEditor(); return; }
    E.elementi.push(it);
    selezionaE(it);
  };
  fog.addEventListener("pointermove", muovi);
  fog.addEventListener("pointerup", fine);
});
$("#ePrec").onclick = () => vaiPaginaEditor(E.pag - 1);
$("#eSucc").onclick = () => vaiPaginaEditor(E.pag + 1);
}
document.addEventListener("keydown", e => {
  if (stato.vista !== "doc" || !E.tipo || !E.sel) return;
  if (document.activeElement && /INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName)) return;
  if ($(".velo.on")) return;
  if (e.key === "Delete" || e.key === "Backspace") { e.preventDefault(); togliElemento(E.sel); }
});
window.addEventListener("resize", () => { if (stato.vista === "doc" && E.tipo) disegnaPaginaEditor(); });

// riepilogoEditor: "3 elementi su 2 pagine" e i tasti Applica / Annulla.
function piedeEditor(box, chiaveApplica, f) {
  const info = el("p", "pPiccolo");
  box.appendChild(info);
  const a = azioni(box);
  const b = bottone(a, chiaveApplica, true, f);
  bottone(a, "annulla", false, () => chiudiStrumento());
  const agg = () => {
    const n = E.elementi.length, p = new Set(E.elementi.map(e => e.pagina)).size;
    info.textContent = n ? t("elementiPronti", n, p) : t("nessunoAncora");
    b.disabled = !n;
  };
  return agg;
}

const T = { font: "helv", dim: 14, colore: "#111111" };

function pTesto(box) {
  testoP(box, "d_testo");
  const sb = el("div");
  box.appendChild(sb);
  const agg = piedeEditor(box, "applica", async () => {
    const ee = E.elementi.map(it => elementoTesto({ pagina: it.pagina, x: it.x, y: it.y, l: it.l, a: it.a }, it.testo, it.font, it.dim, it.colore, "s")).filter(e => e.testo !== "" || e.tipo === "immagine");
    if (await applica("goDisegna", ee)) chiudiStrumento();
  });
  const ta = el("textarea"); ta.id = "tTesto"; ta.spellcheck = false;
  campo(sb, t("testo"), ta);
  const r = el("div", "pRiga");
  const font = sel([["helv", "Sans"], ["helvb", "Sans " + t("grassetto")], ["times", "Serif"], ["timesb", "Serif " + t("grassetto")], ["cour", "Mono"]], T.font);
  const dim = inp("number", String(T.dim), { min: 4, max: 300 });
  campo(r, t("carattere"), font); campo(r, t("dimensione"), dim);
  sb.appendChild(r);
  const col = colori(T.colore, c => { T.colore = c; if (E.sel) { E.sel.colore = c; disegnaElementiEditor(); } });
  campo(sb, t("colore"), col);
  const nota = el("p", "pAvviso hidden"); nota.textContent = t("testoComeImmagine"); sb.appendChild(nota);
  ta.oninput = () => { if (E.sel) { E.sel.testo = ta.value; adattaTesto(E.sel); disegnaElementiEditor(); } };
  font.onchange = () => { T.font = font.value; if (E.sel) { E.sel.font = font.value; adattaTesto(E.sel); disegnaElementiEditor(); } };
  dim.onchange = () => { T.dim = Math.max(4, Math.min(300, parseFloat(dim.value) || 14)); if (E.sel) { E.sel.dim = T.dim; adattaTesto(E.sel); disegnaElementiEditor(); } };
  E.pannelloSel = it => {
    sb.classList.toggle("pNd", !it);
    ta.disabled = !it;
    if (it && document.activeElement !== ta) ta.value = it.testo;
    if (it) { font.value = it.font; if (document.activeElement !== dim) dim.value = Math.round(it.dim * 10) / 10; }
    nota.classList.toggle("hidden", !it || testoAdatto(it.testo));
    agg();
  };
  apriEditor("testo", t("aiutoTesto")).then(() => E.pannelloSel(null));
}

function pImmagine(box) {
  testoP(box, "d_immagine");
  const file = inp("file"); file.accept = "image/png,image/jpeg"; file.className = "hidden"; file.multiple = false;
  box.appendChild(file);
  bottone(azioni(box), "scegliImmagine", false, () => file.click());
  const agg = piedeEditor(box, "applica", async () => {
    const ee = E.elementi.map(it => ({ tipo: "immagine", pagina: it.pagina, x: it.x, y: it.y, l: it.l, a: it.a, img: it.img }));
    if (await applica("goDisegna", ee)) chiudiStrumento();
  });
  file.onchange = async () => {
    const f = file.files[0]; if (!f) return;
    let img;
    try { img = await leggiDataURL(f); await misuraImg(img); } catch { return avvisa(t("e_foto")); }
    // JPEG e PNG vanno bene così come sono; il PDF le tiene uguali.
    const { w, h } = await misuraImg(img);
    const P = E.dimPag;
    let l = Math.min(0.5, (w / 2) / P.w), a = l * (h / w) * (P.w / P.h);
    if (a > 0.6) { a = 0.6; l = a / ((h / w) * (P.w / P.h)); }
    const it = { id: ++E.conta, tipo: "immagine", pagina: E.pag, x: (1 - l) / 2, y: (1 - a) / 2, l, a, img };
    E.elementi.push(it); selezionaE(it); file.value = "";
  };
  E.pannelloSel = () => agg();
  apriEditor("immagine", t("aiutoImmagine")).then(() => agg());
}

function pLink(box) {
  testoP(box, "d_link");
  const sb = el("div", "pNd");
  box.appendChild(sb);
  const tipo = scelte([["url", t("indirizzoWeb")], ["pagina", t("unaPagina")]], "url", v => { if (E.sel) { if (v === "url") E.sel.verso = 0; else E.sel.url = ""; } mostra(); });
  campo(sb, t("portaA"), tipo);
  const url = campo(sb, t("indirizzoWeb"), inp("text", "", { placeholder: "https://www.esempio.it" }));
  const pag = campo(sb, t("numeroPagina"), inp("number", "1", { min: 1 }));
  const mostra = () => {
    url.parentElement.classList.toggle("hidden", tipo.valore !== "url");
    pag.parentElement.classList.toggle("hidden", tipo.valore !== "pagina");
  };
  url.oninput = () => { if (E.sel) { E.sel.url = url.value.trim(); E.sel.verso = 0; disegnaElementiEditor(); } };
  pag.oninput = () => { if (E.sel) { E.sel.verso = Math.max(1, Math.min(E.n, parseInt(pag.value, 10) || 1)); E.sel.url = ""; disegnaElementiEditor(); } };
  testoP(box, "linkSicuri", "pPiccolo");
  const agg = piedeEditor(box, "applica", async () => {
    const senza = E.elementi.find(it => !it.url && !it.verso);
    if (senza) { E.pag = senza.pagina; await disegnaPaginaEditor(); selezionaE(senza); return avvisa(t("linkSenzaMeta")); }
    const cc = E.elementi.map(it => ({ pagina: it.pagina, x: it.x, y: it.y, l: it.l, a: it.a, url: it.url || "", verso: it.url ? 0 : it.verso }));
    if (await applica("goCollegamenti", cc)) chiudiStrumento();
  });
  E.pannelloSel = it => {
    sb.classList.toggle("pNd", !it);
    if (it) {
      const v = it.verso && !it.url ? "pagina" : "url";
      tipo.valore = v; $$("button", tipo).forEach((b, i) => b.classList.toggle("on", (i === 0) === (v === "url")));
      if (document.activeElement !== url) url.value = it.url || "";
      if (document.activeElement !== pag) pag.value = it.verso || 1;
      pag.max = E.n;
    }
    mostra(); agg();
  };
  mostra();
  apriEditor("link", t("aiutoLink")).then(() => E.pannelloSel(null));
}

function pOscura(box) {
  testoP(box, "d_oscura");
  testoP(box, "oscuraAvviso", "pAvviso rosso");
  const agg = piedeEditor(box, "oscuraApplica", async () => {
    const pagine = [...new Set(E.elementi.map(e => e.pagina))].sort((a, b) => a - b);
    const ok = await chiedi(t("s_oscura"), t("oscuraConferma", pagine.join(", ")), [[t("annulla"), false], [t("oscuraApplica"), true, true]]);
    if (!ok) return;
    attesa(true, t("oscurando"));
    const pp = [];
    try {
      for (const n of pagine) {
        const pag = await E.doc.getPage(n);
        const vp = pag.getViewport({ scale: 200 / 72 }); // 200 punti per pollice
        const c = document.createElement("canvas");
        c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
        const x = c.getContext("2d");
        x.fillStyle = "#fff"; x.fillRect(0, 0, c.width, c.height);
        await pag.render({ canvasContext: x, viewport: vp, annotationMode: pdfjsLib.AnnotationMode.ENABLE_FORMS }).promise;
        x.fillStyle = "#000";
        for (const it of E.elementi.filter(e => e.pagina === n)) x.fillRect(Math.floor(it.x * c.width) - 1, Math.floor(it.y * c.height) - 1, Math.ceil(it.l * c.width) + 2, Math.ceil(it.a * c.height) + 2);
        const blob = await new Promise(ok2 => c.toBlob(ok2, "image/jpeg", 0.88));
        pp.push({ pagina: n, img: base64DaBytes(new Uint8Array(await blob.arrayBuffer())) });
      }
    } catch (e) { attesa(false); return avvisaErrore(e); }
    attesa(false);
    if (await applica("goOscura", pp)) chiudiStrumento();
  });
  E.pannelloSel = () => agg();
  apriEditor("oscura", t("aiutoOscura")).then(() => agg());
}

// ---------------------------------------------------------------- Organizza pagine

const O = { aperto: false, seq: [], fonti: [], sel: new Set(), conta: 0, osservatore: null };

async function apriOrganizza() {
  if (!D.att) return;
  chiudiStrumento(true);
  await consolida();
  D.strumento = "organizza";
  disegnaPannello();
  const v = corrente(D.att);
  O.aperto = true;
  O.fonti = [{ voce: v.voce, pw: v.pw, nome: D.att.nome }];
  const n = totPagine() || (await docJS(v)).numPages;
  O.seq = Array.from({ length: n }, (_, i) => ({ k: ++O.conta, f: 0, p: i + 1, r: 0 }));
  O.sel = new Set();
  $("#dOrg").classList.remove("hidden");
  aggiornaRapidi();
  disegnaOrganizza();
}

function chiudiOrganizza() {
  O.aperto = false;
  if (O.osservatore) O.osservatore.disconnect();
  $("#dOrg").classList.add("hidden");
  $("#oGriglia").innerHTML = "";
  aggiornaRapidi();
}

function pOrganizza(box) {
  testoP(box, "d_organizza");
  testoP(box, "organizzaAiuto", "pPiccolo");
  const a = azioni(box);
  bottone(a, "applicaModifiche", true, () => applicaOrganizza());
  bottone(a, "annulla", false, () => chiudiStrumento());
}
strumento("organizza").p = pOrganizza;

async function applicaOrganizza() {
  if (!O.seq.length) return avvisa(t("e_nessuna_pagina"));
  const invariata = O.seq.length === totPagine() && O.seq.every((s, i) => s.f === 0 && s.p === i + 1 && !s.r);
  if (invariata) return chiudiStrumento();
  const altri = O.fonti.slice(1).map(f => ({ id: f.voce.id, pw: f.pw }));
  if (await applica("goOrganizza", altri, O.seq.map(({ f, p, r, w, h }) => ({ f, p, r, w, h })))) chiudiStrumento();
}

function barraOrganizza() {
  const b = $("#oBarra");
  b.innerHTML = "";
  const n = O.sel.size;
  const tasto = (i, k, f, dis) => { const x = el("button", "btn picc"); x.innerHTML = ico(i, 16) + `<span>${t(k)}</span>`; x.onclick = f; x.disabled = !!dis; b.appendChild(x); return x; };
  tasto("tutte", n === O.seq.length && n ? "deselezionaTutte" : "selezionaTutte", () => { if (O.sel.size === O.seq.length) O.sel.clear(); else O.seq.forEach(s => O.sel.add(s.k)); disegnaOrganizza(); });
  tasto("ruotaSx", "girSx", () => ruotaSel(270), !n);
  tasto("ruotaDx", "girDx", () => ruotaSel(90), !n);
  tasto("elimina", "elimina", () => { O.seq = O.seq.filter(s => !O.sel.has(s.k)); O.sel.clear(); disegnaOrganizza(); }, !n);
  tasto("estrai", "estraiSel", async () => {
    const seq = O.seq.filter(s => O.sel.has(s.k)).map(({ f, p, r, w, h }) => ({ f, p, r, w, h }));
    await estraiESalva(seq, O.fonti.slice(1).map(f => ({ id: f.voce.id, pw: f.pw })));
  }, !n);
  b.appendChild(el("span", "nSep"));
  tasto("vuota", "paginaVuota", () => inserisciInOrganizza("vuota"));
  tasto("inserisci", "daAltroPDF", () => inserisciInOrganizza("pdf"));
  const info = el("span", "info");
  info.textContent = n ? t("selezionate", n, O.seq.length) : t("pagineTot", O.seq.length);
  b.appendChild(el("div", "spazio")); b.appendChild(info);
}

function ruotaSel(g) { O.seq.forEach(s => { if (O.sel.has(s.k)) s.r = (s.r + g) % 360; }); disegnaOrganizza(); }

// posInserimento: dopo l'ultima pagina selezionata, altrimenti alla fine.
function posInserimento() {
  let pos = O.seq.length;
  O.seq.forEach((s, i) => { if (O.sel.has(s.k)) pos = i + 1; });
  return pos;
}

async function inserisciInOrganizza(cosa) {
  const pos = posInserimento();
  let nuove;
  if (cosa === "vuota") {
    const vicino = O.seq[Math.max(0, pos - 1)];
    let d = { w: 595.28, h: 841.89 };
    if (vicino && vicino.f >= 0) { try { const doc = await docFonte(vicino.f); const vp = (await doc.getPage(vicino.p)).getViewport({ scale: 1 }); d = { w: vp.width, h: vp.height }; } catch { /* A4 */ } }
    nuove = [{ k: ++O.conta, f: -1, p: 0, r: 0, w: d.w, h: d.h }];
  } else {
    const altro = await scegliAltroPDF();
    if (!altro) return;
    O.fonti.push({ voce: altro.voce, pw: altro.pw, nome: altro.voce.nome });
    const f = O.fonti.length - 1;
    nuove = Array.from({ length: altro.pagine }, (_, i) => ({ k: ++O.conta, f, p: i + 1, r: 0 }));
  }
  O.seq.splice(pos, 0, ...nuove);
  O.sel = new Set(nuove.map(s => s.k));
  disegnaOrganizza();
}

const docFonti = new Map();
function docFonte(f) {
  const fo = O.fonti[f];
  if (f === 0) return docJS({ voce: fo.voce, pw: fo.pw });
  if (!docFonti.has(fo.voce.id)) docFonti.set(fo.voce.id, pdfjsLib.getDocument({ url: "/file/" + fo.voce.id, password: fo.pw || undefined, ...OPZ_PDFJS }).promise);
  return docFonti.get(fo.voce.id);
}

const cacheMinOrg = new Map(); // "fonte:pagina" -> canvas (dataURL)
let trascinaOrg = null;

function disegnaOrganizza() {
  barraOrganizza();
  const g = $("#oGriglia");
  g.innerHTML = "";
  if (O.osservatore) O.osservatore.disconnect();
  O.osservatore = new IntersectionObserver(voci => {
    for (const v of voci) if (v.isIntersecting) { O.osservatore.unobserve(v.target); riempiMinOrg(v.target); }
  }, { root: $("#dOrg"), rootMargin: "400px" });
  O.seq.forEach((s, i) => {
    const c = el("div", "oCarta" + (O.sel.has(s.k) ? " sel" : ""));
    c.draggable = true;
    c.dataset.i = i;
    const altra = s.f > 0 ? `<em title="${O.fonti[s.f].nome}">+</em>` : "";
    c.innerHTML = `<input type="checkbox" class="oSpunta" ${O.sel.has(s.k) ? "checked" : ""}><div class="oMin"></div><div class="oNum">${i + 1}${altra}</div>
      <div class="oAz"><button data-a="sx" title="${t("girSx")}">${ico("ruotaSx", 15)}</button><button data-a="dx" title="${t("girDx")}">${ico("ruotaDx", 15)}</button><button data-a="x" title="${t("elimina")}">${ico("elimina", 15)}</button></div>`;
    const min = $(".oMin", c);
    if (s.f === -1) { const v = el("div", "vuota"); v.style.aspectRatio = `${s.w} / ${s.h}`; v.style.height = "auto"; min.appendChild(v); }
    else { min.dataset.chiave = s.f + ":" + s.p; O.osservatore.observe(min); }
    if (s.r) { const k = s.r % 180 ? 0.78 : 1; requestAnimationFrame(() => { const x = min.firstChild; if (x) x.style.transform = `rotate(${s.r}deg) scale(${k})`; }); }
    min.dataset.r = s.r;
    c.onclick = e => {
      if (e.target.closest(".oAz")) return;
      if (O.sel.has(s.k)) O.sel.delete(s.k); else O.sel.add(s.k);
      c.classList.toggle("sel", O.sel.has(s.k)); $(".oSpunta", c).checked = O.sel.has(s.k);
      barraOrganizza();
    };
    $$("[data-a]", c).forEach(b => b.onclick = () => {
      if (b.dataset.a === "x") { O.seq.splice(i, 1); O.sel.delete(s.k); }
      else s.r = (s.r + (b.dataset.a === "dx" ? 90 : 270)) % 360;
      disegnaOrganizza();
    });
    c.addEventListener("dragstart", e => { trascinaOrg = i; e.dataTransfer.setData("text/x-tasso-pagina", String(i)); e.dataTransfer.effectAllowed = "move"; c.classList.add("trascina"); });
    c.addEventListener("dragend", () => { trascinaOrg = null; c.classList.remove("trascina"); });
    c.addEventListener("dragover", e => { if (trascinaOrg === null) return; e.preventDefault(); e.stopPropagation(); c.classList.add("sopra"); });
    c.addEventListener("dragleave", () => c.classList.remove("sopra"));
    c.addEventListener("drop", e => {
      if (trascinaOrg === null) return;
      e.preventDefault(); e.stopPropagation();
      // Se ci sono pagine selezionate e trascini una di loro, si spostano tutte insieme.
      const da = O.seq[trascinaOrg];
      const gruppo = O.sel.has(da.k) ? O.seq.filter(x => O.sel.has(x.k)) : [da];
      const dest = O.seq[i];
      if (gruppo.includes(dest)) { trascinaOrg = null; return disegnaOrganizza(); }
      O.seq = O.seq.filter(x => !gruppo.includes(x));
      const j = O.seq.indexOf(dest);
      O.seq.splice(trascinaOrg < i ? j + 1 : j, 0, ...gruppo);
      trascinaOrg = null;
      disegnaOrganizza();
    });
    g.appendChild(c);
  });
}

async function riempiMinOrg(min) {
  const [f, p] = min.dataset.chiave.split(":").map(Number);
  const chiave = O.fonti[f].voce.id + ":" + p;
  let url = cacheMinOrg.get(chiave);
  if (!url) {
    try {
      const doc = await docFonte(f);
      const pag = await doc.getPage(p);
      const v1 = pag.getViewport({ scale: 1 });
      const vp = pag.getViewport({ scale: 2 * 150 / Math.max(v1.width, v1.height) });
      const c = document.createElement("canvas");
      c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
      await pag.render({ canvasContext: c.getContext("2d"), viewport: vp }).promise;
      url = c.toDataURL("image/jpeg", 0.8);
      cacheMinOrg.set(chiave, url);
    } catch { return; }
  }
  const img = new Image();
  img.src = url; img.alt = "";
  const r = +min.dataset.r || 0;
  if (r) img.style.transform = `rotate(${r}deg) scale(${r % 180 ? 0.78 : 1})`;
  min.innerHTML = "";
  min.appendChild(img);
}

// ---------------------------------------------------------------- miniature a destra

let osservaMini = null;
async function disegnaMiniature() {
  const box = $("#dMini");
  box.innerHTML = "";
  if (osservaMini) osservaMini.disconnect();
  box.classList.toggle("hidden", !D.mini);
  $("#nMini").classList.toggle("on", D.mini);
  if (!D.mini || !D.att) return;
  const v = corrente(D.att);
  let doc;
  try { doc = await docJS(v); } catch { return; }
  if (corrente(D.att) !== v) return;
  let rap = 1.414;
  try { const vp = (await doc.getPage(1)).getViewport({ scale: 1 }); rap = vp.height / vp.width; } catch { /* */ }
  osservaMini = new IntersectionObserver(voci => {
    for (const x of voci) if (x.isIntersecting) { osservaMini.unobserve(x.target); disegnaMini(doc, x.target); }
  }, { root: box, rootMargin: "300px" });
  for (let n = 1; n <= doc.numPages; n++) {
    const b = el("button", "mPag");
    b.dataset.n = n;
    b.innerHTML = `<div class="mBox" style="height:${Math.round(104 * rap)}px"></div><span>${n}</span>`;
    b.onclick = () => { if (E.tipo) vaiPaginaEditor(n); else if (appV) appV.page = n; };
    box.appendChild(b);
    osservaMini.observe(b);
  }
  evidenziaMini();
}

async function disegnaMini(doc, b) {
  try {
    const pag = await doc.getPage(+b.dataset.n);
    const v1 = pag.getViewport({ scale: 1 });
    const sc = 208 / v1.width;
    const vp = pag.getViewport({ scale: sc });
    const c = document.createElement("canvas");
    c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
    c.style.width = "104px";
    await pag.render({ canvasContext: c.getContext("2d"), viewport: vp }).promise;
    const box = $(".mBox", b);
    box.style.height = "auto";
    box.innerHTML = "";
    box.appendChild(c);
  } catch { /* documento chiuso nel frattempo */ }
}

function evidenziaMini() {
  const n = E.tipo ? E.pag : (appV && appV.page) || 1;
  $$("#dMini .mPag").forEach(b => b.classList.toggle("on", +b.dataset.n === n));
  const on = $("#dMini .mPag.on");
  if (on && D.mini) {
    const box = $("#dMini"), r = on.getBoundingClientRect(), rb = box.getBoundingClientRect();
    if (r.top < rb.top || r.bottom > rb.bottom) on.scrollIntoView({ block: "nearest" });
  }
}

// ---------------------------------------------------------------- colonna di navigazione

function aggiornaNav() {
  const ed = !!E.tipo;
  const n = ed ? E.pag : (appV && appV.page) || 1;
  const tot = ed ? E.n : totPagine();
  if (document.activeElement !== $("#nPagIn")) $("#nPagIn").value = tot ? n : "";
  $("#nPagTot").textContent = tot ? t("diN", tot) : "";
  $("#nSu").disabled = n <= 1; $("#nGiu").disabled = n >= tot;
  const blocca = ed || O.aperto;
  ["#nRuota", "#nPiu", "#nMeno", "#nAdatta"].forEach(s => $(s).disabled = blocca);
  $("#nZoom").textContent = appV && appV.pdfViewer && !blocca ? Math.round((appV.pdfViewer.currentScale || 1) * 100) + "%" : "";
  evidenziaMini();
}

function vaiPagina(n) {
  if (E.tipo) return vaiPaginaEditor(n);
  if (appV && n >= 1 && n <= totPagine()) appV.page = n;
}

// ---------------------------------------------------------------- barra rapida

const RAPIDI = [
  ["seleziona", "puntatore"], ["selTesto", "cursoreTesto"], null,
  ["nota", "nota"], ["evidenzia", "evidenzia"], ["penna", "penna"], null,
  ["firma", "timbro"], ["istantanea", "istantanea"],
];
const COLORI_RAPIDI = { nota: ["#111111", "#C62828", "#1a3ea8", "#2E7D32"], evidenzia: ["#FFFF98", "#53FFBC", "#80EBFF", "#FFCBE6", "#FF4F5F"], penna: ["#111111", "#C62828", "#1a3ea8", "#2E7D32", "#FF9A4D"] };
const coloreRapido = { nota: "#111111", evidenzia: "#FFFF98", penna: "#1a3ea8" };
const spessorePenna = { v: 2 };

function disegnaRapidi() {
  const box = $("#dRapidi");
  box.innerHTML = "";
  for (const r of RAPIDI) {
    if (!r) { box.appendChild(el("div", "sep")); continue; }
    const [k, i] = r;
    const b = el("button", "dIco");
    b.dataset.r = k;
    b.innerHTML = ico(i, 19);
    b.title = t("r_" + k);
    b.onclick = () => applicaRapido(k);
    box.appendChild(b);
  }
  aggiornaRapidi();
}

function aggiornaRapidi() {
  const bloccati = !!E.tipo || O.aperto;
  $$("#dRapidi [data-r]").forEach(b => { b.classList.toggle("on", b.dataset.r === D.rapido && !bloccati); b.disabled = bloccati; });
  const pop = $("#rColori");
  if (pop) pop.remove();
  if (!bloccati && COLORI_RAPIDI[D.rapido]) {
    const b = $(`#dRapidi [data-r="${D.rapido}"]`);
    const p = el("div", "rColori"); p.id = "rColori";
    p.style.top = (b.offsetTop) + "px";
    p.appendChild(colori(coloreRapido[D.rapido], c => { coloreRapido[D.rapido] = c; parametriEditor(); }, COLORI_RAPIDI[D.rapido]));
    if (D.rapido === "penna") {
      const s = inp("range", String(spessorePenna.v), { min: 1, max: 12 }); s.className = "pRange"; s.title = t("spessore");
      s.style.width = "24px"; s.style.writingMode = "vertical-lr"; s.style.direction = "rtl"; s.style.height = "70px";
      s.oninput = () => { spessorePenna.v = +s.value; parametriEditor(); };
      p.appendChild(s);
    }
    $("#dRapidi").appendChild(p);
  }
}

const MODO_EDITOR = { seleziona: 0, selTesto: 0, nota: 3, evidenzia: 9, penna: 15 };

async function applicaRapido(k) {
  if (k === "firma") return firmaDoc();
  if (k === "istantanea") return iniziaIstantanea();
  if (E.tipo || O.aperto) chiudiStrumento();
  D.rapido = k;
  aggiornaRapidi();
  const app = appV;
  if (!app || !app.pdfDocument) return;
  const eb = app.eventBus;
  try {
    eb.dispatch("switchcursortool", { source: null, tool: k === "seleziona" ? 1 : 0 });
    eb.dispatch("switchannotationeditormode", { source: null, mode: MODO_EDITOR[k] ?? 0 });
    parametriEditor();
  } catch { /* versione di PDF.js senza questo strumento */ }
}

function parametriEditor() {
  const app = appV;
  if (!app) return;
  const eb = app.eventBus;
  const P = pdfjsLib.AnnotationEditorParamsType;
  try {
    if (D.rapido === "nota") { eb.dispatch("switchannotationeditorparams", { source: null, type: P.FREETEXT_COLOR, value: coloreRapido.nota }); eb.dispatch("switchannotationeditorparams", { source: null, type: P.FREETEXT_SIZE, value: 14 }); }
    if (D.rapido === "evidenzia") eb.dispatch("switchannotationeditorparams", { source: null, type: P.HIGHLIGHT_COLOR, value: coloreRapido.evidenzia });
    if (D.rapido === "penna") { eb.dispatch("switchannotationeditorparams", { source: null, type: P.INK_COLOR, value: coloreRapido.penna }); eb.dispatch("switchannotationeditorparams", { source: null, type: P.INK_THICKNESS, value: spessorePenna.v }); }
  } catch { /* */ }
}

// Istantanea: trascina un rettangolo sul documento, l'immagine va negli appunti.
function iniziaIstantanea() {
  if (!appV || !appV.pdfDocument || E.tipo || O.aperto) return;
  const v = $("#dScatto"), r = $("#dScattoR");
  v.classList.remove("hidden");
  $$("#dRapidi [data-r]").forEach(b => b.classList.toggle("on", b.dataset.r === "istantanea"));
  const fine = () => { v.classList.add("hidden"); r.style.display = "none"; v.onpointerdown = null; aggiornaRapidi(); document.removeEventListener("keydown", esc); };
  const esc = e => { if (e.key === "Escape") fine(); };
  document.addEventListener("keydown", esc);
  v.onpointerdown = e => {
    const b = v.getBoundingClientRect();
    const x0 = e.clientX - b.left, y0 = e.clientY - b.top;
    v.setPointerCapture(e.pointerId);
    let q = { x: x0, y: y0, w: 0, h: 0 };
    r.style.display = "block";
    const muovi = ev => {
      const x1 = ev.clientX - b.left, y1 = ev.clientY - b.top;
      q = { x: Math.min(x0, x1), y: Math.min(y0, y1), w: Math.abs(x1 - x0), h: Math.abs(y1 - y0) };
      Object.assign(r.style, { left: q.x + "px", top: q.y + "px", width: q.w + "px", height: q.h + "px" });
    };
    v.onpointermove = muovi;
    v.onpointerup = async () => {
      v.onpointermove = v.onpointerup = null;
      fine();
      if (q.w > 6 && q.h > 6) await scatta(q);
    };
  };
}

async function scatta(q) {
  const d = $("#visore").contentDocument;
  const dpr = window.devicePixelRatio || 1;
  const out = document.createElement("canvas");
  out.width = Math.round(q.w * dpr * 2); out.height = Math.round(q.h * dpr * 2);
  const x = out.getContext("2d");
  x.fillStyle = "#1d1f27"; x.fillRect(0, 0, out.width, out.height);
  const k = out.width / q.w;
  for (const c of $$(".page canvas", d)) {
    const r = c.getBoundingClientRect();
    const ix = Math.max(q.x, r.left), iy = Math.max(q.y, r.top), ax = Math.min(q.x + q.w, r.right), ay = Math.min(q.y + q.h, r.bottom);
    if (ax <= ix || ay <= iy) continue;
    const sx = c.width / r.width, sy = c.height / r.height;
    x.drawImage(c, (ix - r.left) * sx, (iy - r.top) * sy, (ax - ix) * sx, (ay - iy) * sy, (ix - q.x) * k, (iy - q.y) * k, (ax - ix) * k, (ay - iy) * k);
  }
  const blob = await new Promise(ok => out.toBlob(ok, "image/png"));
  try {
    await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
    avvisoBreve(t("copiatoAppunti"));
  } catch {
    // Appunti non disponibili: salvo l'immagine come file.
    const v = await caricaArray(new Uint8Array(await blob.arrayBuffer()), "istantanea.png");
    const p = await go("goSalvaTipo", v.id, D.att.nome.replace(/\.pdf$/i, "") + " - " + t("suffIstantanea") + ".png", t("titoloSalva"), "png").catch(e => { avvisaErrore(e); return ""; });
    if (p) toast(p);
    go("goDimentica", v.id).catch(() => { });
  }
}

function avvisoBreve(testo) {
  $("#toastT").textContent = testo;
  $("#toastMostra").classList.add("hidden");
  $("#toast").classList.add("on");
  setTimeout(() => { $("#toast").classList.remove("on"); $("#toastMostra").classList.remove("hidden"); }, 2600);
}

// ---------------------------------------------------------------- cerca

function apriCerca() {
  $("#dCercaBox").classList.remove("hidden");
  const i = $("#dCercaIn");
  i.focus(); i.select();
}
function cerca(indietro, nuova) {
  const q = $("#dCercaIn").value;
  if (!appV || !q) { $("#dCercaN").textContent = ""; return; }
  appV.eventBus.dispatch("find", { source: null, type: nuova ? "" : "again", query: q, caseSensitive: false, entireWord: false, highlightAll: true, findPrevious: !!indietro, matchDiacritics: false });
}
function mostraConteggio(m, statoF) {
  const s = $("#dCercaN");
  if (statoF === 1) { s.textContent = t("nonTrovato"); return; }
  if (m && m.total) s.textContent = `${m.current}/${m.total}`;
  else if (m) s.textContent = $("#dCercaIn").value ? "0" : "";
}

// ---------------------------------------------------------------- tasti

async function tastoDoc(k) {
  if (k === "o") return window.__tasso.apriDaDialogo();
  if (k === "s") return salvaCopiaDoc();
  if (k === "f") return apriCerca();
  if (k === "p") { const app = await visorePronto; app && app.pdfDocument && app.triggerPrinting(); }
}

document.addEventListener("keydown", e => {
  if (stato.vista !== "doc") return;
  if ((e.ctrlKey || e.metaKey) && ["s", "f", "p"].includes(e.key.toLowerCase())) { e.preventDefault(); tastoDoc(e.key.toLowerCase()); return; }
  if (e.key === "Escape" && !$(".velo.on")) {
    if (!$("#dCercaBox").classList.contains("hidden") && document.activeElement === $("#dCercaIn")) { $("#dCercaBox").classList.add("hidden"); return; }
    if (E.tipo || O.aperto || D.strumento) chiudiStrumento();
  }
});

// ---------------------------------------------------------------- avvio della vista

export function initDocumento() {
  $("#dHome").innerHTML = ico("home", 19);
  $("#dPiu").innerHTML = ico("piu", 18);
  $("#dCerca").innerHTML = ico("cerca", 18);
  $("#dStampa").innerHTML = ico("stampa", 18);
  $("#dSalvaIco").innerHTML = ico("salva", 16);
  $("#dAnnullaIco").innerHTML = ico("annulla", 15);
  $("#dCercaSu").innerHTML = ico("su", 16);
  $("#dCercaGiu").innerHTML = ico("giu", 16);
  $("#nMini").innerHTML = ico("miniature", 18);
  $("#nSu").innerHTML = ico("su", 18);
  $("#nGiu").innerHTML = ico("giu", 18);
  $("#nRuota").innerHTML = ico("ruotaDx", 18);
  $("#nPiu").innerHTML = ico("zoomPiu", 18);
  $("#nMeno").innerHTML = ico("zoomMeno", 18);
  $("#nAdatta").innerHTML = ico("adatta", 18);
  mostraPannello(true);
  disegnaRapidi();
  initEditor();

  $("#dHome").onclick = () => { vista("home"); disegnaAperti(); };
  $("#dPiu").onclick = () => window.__tasso.apriDaDialogo();
  $("#dSalva").onclick = () => salvaCopiaDoc();
  $("#dStampa").onclick = () => tastoDoc("p");
  $("#dCerca").onclick = () => { if ($("#dCercaBox").classList.contains("hidden")) apriCerca(); else $("#dCercaBox").classList.add("hidden"); };
  $("#dCercaIn").addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); cerca(e.shiftKey, false); } if (e.key === "Escape") $("#dCercaBox").classList.add("hidden"); });
  $("#dCercaIn").addEventListener("input", () => cerca(false, true));
  $("#dCercaSu").onclick = () => cerca(true, false);
  $("#dCercaGiu").onclick = () => cerca(false, false);
  $("#dAnnulla").onclick = () => annullaModifica();
  $$("#dSez [data-sez]").forEach(b => b.onclick = () => vaiSezione(b.dataset.sez));
  $("#dChiudiPan").onclick = () => mostraPannello(!D.pannello);
  $("#nMini").onclick = () => { D.mini = !D.mini; disegnaMiniature(); };
  $("#nSu").onclick = () => vaiPagina((E.tipo ? E.pag : appV && appV.page) - 1);
  $("#nGiu").onclick = () => vaiPagina((E.tipo ? E.pag : appV && appV.page) + 1);
  $("#nPagIn").addEventListener("keydown", e => { if (e.key === "Enter") { vaiPagina(parseInt($("#nPagIn").value, 10) || 1); $("#nPagIn").blur(); } });
  $("#nPagIn").addEventListener("blur", () => aggiornaNav());
  $("#nRuota").onclick = () => { if (appV) appV.pdfViewer.pagesRotation = (appV.pdfViewer.pagesRotation + 90) % 360; };
  $("#nPiu").onclick = () => appV && appV.pdfViewer.increaseScale();
  $("#nMeno").onclick = () => appV && appV.pdfViewer.decreaseScale();
  $("#nAdatta").onclick = () => { if (appV) appV.pdfViewer.currentScaleValue = appV.pdfViewer.currentScaleValue === "page-width" ? "page-fit" : "page-width"; };
}

// Quando cambia la lingua: ridisegno i testi della vista.
export function linguaDocumento() {
  disegnaRapidi();
  if (stato.vista === "doc") { disegnaSchede(); disegnaPannello(); aggiornaFasce(); aggiornaNav(); }
  disegnaAperti();
}

export function documentiAperti() { return D.schede.length; }
export function salvaTutto() { return D.schede.some(s => !s.salvata); }
export const __doc = { D, E, O, apriOrganizza, usaStrumento: id => usaStrumento(strumento(id)), vaiSezione, applicaRapido, pagineDa, nuovaVersione };
