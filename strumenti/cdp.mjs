// Tasso PDF - piccolo client Chrome DevTools Protocol per le prove dell'interfaccia (solo prove).
import { spawn } from "node:child_process";
import { setTimeout as dorme } from "node:timers/promises";

export async function avviaChrome(porta = 9333, larghezza = 1240, altezza = 860) {
  const p = spawn("google-chrome", ["--headless=new", "--no-sandbox", "--disable-gpu", `--remote-debugging-port=${porta}`,
    `--window-size=${larghezza},${altezza}`, "--user-data-dir=/tmp/tassopdf-chrome", "about:blank"], { stdio: "ignore" });
  for (let i = 0; i < 50; i++) {
    try { const r = await fetch(`http://127.0.0.1:${porta}/json/list`); const l = await r.json(); const pg = l.find(x => x.type === "page"); if (pg) return { proc: p, ws: pg.webSocketDebuggerUrl }; } catch { }
    await dorme(200);
  }
  throw new Error("chrome non parte");
}

export async function collega(wsUrl) {
  const ws = new WebSocket(wsUrl);
  await new Promise((ok, ko) => { ws.onopen = ok; ws.onerror = ko; });
  let id = 0; const attese = new Map(); const ascolti = [];
  ws.onmessage = m => { const d = JSON.parse(m.data); if (d.id && attese.has(d.id)) { const { ok, ko } = attese.get(d.id); attese.delete(d.id); d.error ? ko(new Error(JSON.stringify(d.error))) : ok(d.result); } else if (d.method) ascolti.forEach(f => f(d)); };
  const invia = (method, params = {}) => new Promise((ok, ko) => { const i = ++id; attese.set(i, { ok, ko }); ws.send(JSON.stringify({ id: i, method, params })); });
  const valuta = async (expr) => { const r = await invia("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true }); if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text); return r.result.value; };
  return { invia, valuta, ascolta: f => ascolti.push(f), chiudi: () => ws.close() };
}

export { dorme };
