// Tasso PDF - prova Playwright del tasto «Gira la vista» (FIX-PDF-RUOTA). SOLO PER LE PROVE.
// Rifà i passi visti sul PC: apri, gira, X; apri, salva una copia, gira, X; riapri il programma.
// Uso: node strumenti/prova_ruota.js <server provaui> <cartella di lavoro>
//   (il server si compila con: go build -tags provaui -o <server> .)
// Esce con codice 1 se una regola non è rispettata.
const { chromium } = require('/opt/npm-tools/node_modules/playwright');
const fs = require('fs'); const path = require('path'); const crypto = require('crypto');
const { execSync } = require('child_process');

const SRV = process.argv[2], W = process.argv[3];
const P = 8891;
const F = path.join(W, 'file'), SALVATI = path.join(W, 'salvati');
const ORIG = path.join(F, 'prova-unito - copia.pdf'), ALTRO = path.join(F, 'altro.pdf');
const sha = f => crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
let sbagli = 0;
const verifica = (cosa, ok, dati) => { console.log((ok ? 'OK   ' : 'MALE ') + cosa.padEnd(70), dati === undefined ? '' : JSON.stringify(dati)); if (!ok) sbagli++; };

function server(scegli) {
  execSync(`pkill -x '${path.basename(SRV).slice(0, 15)}' ; sleep 0.3; mkdir -p ${W}/t ${SALVATI}; TMPDIR=${W}/t TASSOPDF_PORTA=${P} TASSOPDF_LINGUA=it TASSOPDF_SCEGLI="${scegli}" TASSOPDF_SALVA=${SALVATI} setsid nohup ${SRV} > ${W}/s.log 2>&1 & sleep 1.2`, { shell: '/bin/bash' });
}
const chiusure = () => (fs.readFileSync(path.join(W, 's.log'), 'utf8').match(/CHIUDI FINESTRA/g) || []).length;

async function pagina(ctx) {
  const p = await ctx.newPage(); p.setDefaultTimeout(60000);
  p.on('pageerror', e => { console.log('PAGEERROR ' + e); sbagli++; });
  await p.goto(`http://127.0.0.1:${P}/`);
  await p.waitForFunction(() => window.__tassoPronto === true);
  return p;
}
async function fermo(p) { await p.waitForTimeout(300); await p.waitForFunction(() => !document.querySelector('#dAttesa.on'), null, { timeout: 120000 }); await p.waitForTimeout(700); }
async function apri(p, n = 1) { await p.click('#hApri'); await p.waitForFunction(n => window.__tasso.doc.D.schede.length >= n, n); await fermo(p); }
const giro = p => p.evaluate(() => document.querySelector('#visore').contentWindow.PDFViewerApplication.pdfViewer.pagesRotation);
const scheda = p => p.evaluate(() => { const sc = window.__tasso.doc.D.att; return { nome: sc.nome, versioni: sc.versioni.length, salvata: sc.salvata, segno: !!document.querySelector('.dTab.on .tm') }; });
const domanda = p => p.evaluate(() => document.querySelector('#dMsg').classList.contains('on') ? document.querySelector('#dMsgT').textContent : '');
async function X(p) { // la X della finestra, come la manda Windows
  const prima = chiusure();
  const r = await p.evaluate(async () => { const r = await (await fetch('/prova/x')).json(); if (r.chiedi) window.tassoChiudiFinestra(); return r; });
  await p.waitForTimeout(800);
  return { domanda: await domanda(p), chiusa: chiusure() > prima || !r.resta };
}
const rotazioni = f => execSync(`python3 -I -c "import pypdf,sys; print([p.rotation for p in pypdf.PdfReader(sys.argv[1]).pages])" "${f}"`).toString().trim();

(async () => {
  const br = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox', '--disable-dev-shm-usage'] });
  // Un solo contesto per tutta la prova: come il WebView2 di Windows, tiene localStorage tra un avvio e l'altro.
  const ctx = await br.newContext({ viewport: { width: 1280, height: 860 } });
  fs.rmSync(SALVATI, { recursive: true, force: true });
  const h0 = sha(ORIG), hAltro = sha(ALTRO);

  // 1. I passi di Sergio: apri, gira, X.
  server(ORIG);
  let p = await pagina(ctx); await apri(p);
  await p.click('#nRuota'); await p.waitForTimeout(500);
  verifica('1 dopo Gira la vista, la vista è girata di 90', await giro(p) === 90, await giro(p));
  const st1 = await scheda(p);
  verifica('1 Gira la vista non crea versioni e non segna la scheda', st1.versioni === 1 && st1.salvata && !st1.segno, st1);
  const x1 = await X(p);
  verifica('1 X: si chiude (non c\'è niente da salvare)', x1.chiusa && !x1.domanda, x1);
  verifica('1 l\'originale è identico (sha256)', sha(ORIG) === h0);
  await p.close();

  // 2. Riapri il programma: la vista non resta girata, né per lui né per un altro PDF.
  server(ORIG + ';' + ALTRO);
  p = await pagina(ctx); await apri(p, 2);
  verifica('2 riaperto il programma, l\'altro PDF non è girato', await giro(p) === 0, await giro(p));
  await p.click('.dTab[title="prova-unito - copia.pdf"]'); await fermo(p);
  verifica('2 riaperto il programma, il PDF girato prima non è girato', await giro(p) === 0, await giro(p));
  // la vista girata vale solo per la sua scheda
  await p.click('#nRuota'); await p.waitForTimeout(400);
  await p.click('.dTab[title="altro.pdf"]'); await fermo(p);
  verifica('2 la vista girata di una scheda non vale per l\'altra', await giro(p) === 0, await giro(p));
  await p.click('.dTab[title="prova-unito - copia.pdf"]'); await fermo(p);
  verifica('2 tornando alla scheda girata la vista è ancora girata', await giro(p) === 90, await giro(p));
  verifica('2 originali identici (sha256)', sha(ORIG) === h0 && sha(ALTRO) === hAltro);
  await p.close();

  // 3. Apri, salva una copia, gira, X: la copia resta identica.
  server(ORIG);
  p = await pagina(ctx); await apri(p);
  await p.click('#dSalva'); await fermo(p);
  const copie = fs.readdirSync(SALVATI).filter(n => n.endsWith('.pdf'));
  verifica('3 salvata una copia', copie.length === 1, copie);
  const COPIA = path.join(SALVATI, copie[0]); const hc = sha(COPIA);
  await p.click('#nRuota'); await p.waitForTimeout(400);
  const x3 = await X(p);
  verifica('3 X dopo Gira la vista: si chiude', x3.chiusa, x3);
  verifica('3 la copia salvata è identica (sha256)', sha(COPIA) === hc);
  verifica('3 l\'originale è identico (sha256)', sha(ORIG) === h0);
  await p.close();

  // 4. Gira la vista e poi Salva una copia: il programma lo dice e propone di girare le pagine.
  fs.rmSync(SALVATI, { recursive: true, force: true });
  server(ORIG);
  p = await pagina(ctx); await apri(p);
  await p.click('#nRuota'); await p.waitForTimeout(400);
  await p.click('#dSalva'); await p.waitForTimeout(600);
  const d4 = await domanda(p);
  verifica('4 Salva con la vista girata: chiede', !!d4, d4);
  const tasti = await p.evaluate(() => [...document.querySelectorAll('#dMsgTasti button')].map(b => b.textContent));
  console.log('     tasti:', JSON.stringify(tasti));
  await p.click('#dMsgTasti button >> nth=-1'); await fermo(p); // «Gira le pagine e salva»
  const c4 = fs.readdirSync(SALVATI).filter(n => n.endsWith('.pdf'));
  verifica('4 salvata una copia', c4.length === 1, c4);
  verifica('4 la copia ha le pagine girate come a schermo', c4.length === 1 && rotazioni(path.join(SALVATI, c4[0])) === '[90, 90, 90, 90, 90]', c4.length && rotazioni(path.join(SALVATI, c4[0])));
  const st4 = await scheda(p);
  verifica('4 la scheda ha la versione girata, salvata, e la vista non gira due volte', st4.versioni === 2 && st4.salvata && await giro(p) === 0, { ...st4, giro: await giro(p) });
  verifica('4 l\'originale è identico (sha256)', sha(ORIG) === h0);
  // Annulla torna all'originale (che è sul disco così com'è): la X non ha niente da chiedere (ZP05-4)
  await p.click('#dAnnulla'); await fermo(p);
  const x4 = await X(p);
  verifica('4 Annulla fino all\'originale, X: si chiude', x4.chiusa && !x4.domanda, x4);
  verifica('4 originale e copia girata intatti (sha256)', sha(ORIG) === h0 && c4.length === 1 && rotazioni(path.join(SALVATI, c4[0])) === '[90, 90, 90, 90, 90]');
  await p.close();

  // 5. Gira la vista, Salva, «Salva senza girare»: la copia è come il file, e lo sa.
  fs.rmSync(SALVATI, { recursive: true, force: true });
  server(ORIG);
  p = await pagina(ctx); await apri(p);
  await p.click('#nRuota'); await p.waitForTimeout(400);
  await p.click('#dSalva'); await p.waitForTimeout(600);
  await p.click('#dMsgTasti button >> nth=1'); await fermo(p);
  const c5 = fs.readdirSync(SALVATI).filter(n => n.endsWith('.pdf'));
  verifica('5 «Salva senza girare»: copia identica all\'originale', c5.length === 1 && sha(path.join(SALVATI, c5[0])) === h0, c5);
  // 6. Organizza/Ruota pagine (modifica vera): scheda segnata, X chiede, originale intatto.
  await p.click('#dSez [data-sez=modifica]'); await p.click('#dPannello .pVoce[data-strumento=ruota]'); await p.waitForTimeout(300);
  await p.fill('#dPannello input[type=text]', '1-5');
  await p.click('#dPannello .pAzioni button >> nth=1'); await fermo(p);
  const st6 = await scheda(p);
  verifica('6 Ruota pagine: scheda modificata e segnata', st6.versioni === 2 && !st6.salvata && st6.segno, st6);
  const x6 = await X(p);
  verifica('6 Ruota pagine, X: chiede', !!x6.domanda && !x6.chiusa, x6);
  if (x6.domanda) { await p.click('#dMsgTasti button >> nth=1'); await p.waitForTimeout(800); } // chiudi senza salvare
  verifica('6 l\'originale è identico (sha256)', sha(ORIG) === h0);
  await p.close();

  await br.close();
  execSync(`pkill -x '${path.basename(SRV).slice(0, 15)}' ; true`, { shell: '/bin/bash' });
  console.log(sbagli ? `\n${sbagli} PROVE SBAGLIATE` : '\nTUTTO OK');
  process.exit(sbagli ? 1 : 0);
})().catch(e => { console.error(e); execSync(`pkill -x '${path.basename(SRV).slice(0, 15)}' ; true`, { shell: '/bin/bash' }); process.exit(2); });
