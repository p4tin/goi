// Minimal headless-Chrome driver over CDP for checking the Goi dev server.
// usage: node cdp.mjs <steps.json>
// steps.json: { "url": "...", "out": "/dir", "steps": [ {"wait":ms} | {"eval":"js"} |
//   {"key":"Enter","mods":["Meta"]} | {"type":"text"} | {"shot":"name.png"} ] }
import { spawn } from 'node:child_process';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const spec = JSON.parse(readFileSync(process.argv[2], 'utf8'));
const out = spec.out;
mkdirSync(out, { recursive: true });
const port = 9333 + Math.floor(Math.random() * 500);
const chrome = spawn(CHROME, [
  '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
  `--user-data-dir=${join(out, 'profile')}`, `--remote-debugging-port=${port}`,
  '--window-size=1280,800', 'about:blank',
], { stdio: 'ignore' });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function targets() {
  for (let i = 0; i < 50; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${port}/json`);
      const list = await r.json();
      const page = list.find((t) => t.type === 'page');
      if (page) return page;
    } catch {}
    await sleep(100);
  }
  throw new Error('chrome did not start');
}

const page = await targets();
const ws = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((r) => ws.addEventListener('open', r, { once: true }));
let id = 0;
const pending = new Map();
ws.addEventListener('message', (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg);
    pending.delete(msg.id);
  } else if (msg.method === 'Runtime.exceptionThrown') {
    console.log('EXCEPTION', msg.params.exceptionDetails.exception?.description ?? msg.params.exceptionDetails.text);
  } else if (msg.method === 'Runtime.consoleAPICalled' && ['error', 'warning'].includes(msg.params.type)) {
    console.log('CONSOLE', msg.params.type, msg.params.args.map((a) => a.value ?? a.description).join(' '));
  }
});
const send = (method, params = {}) =>
  new Promise((resolve) => {
    const i = ++id;
    pending.set(i, resolve);
    ws.send(JSON.stringify({ id: i, method, params }));
  });

await send('Runtime.enable');
await send('Page.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 800, deviceScaleFactor: 1, mobile: false });
await send('Page.navigate', { url: spec.url });
await sleep(spec.initialWait ?? 2500);

const keyCode = { Enter: 13, Escape: 27, Tab: 9, ArrowDown: 40, ArrowUp: 38, Backspace: 8, Shift: 16, F2: 113 };
for (const s of spec.steps) {
  if (s.wait) await sleep(s.wait);
  if (s.eval) {
    const r = await send('Runtime.evaluate', { expression: s.eval, awaitPromise: true, returnByValue: true });
    const v = r.result?.result?.value ?? r.result?.exceptionDetails?.exception?.description;
    console.log('EVAL', s.label ?? '', JSON.stringify(v));
  }
  if (s.key) {
    const mods = (s.mods ?? []).reduce((m, k) => m | ({ Alt: 1, Control: 2, Meta: 4, Shift: 8 }[k] ?? 0), 0);
    const base = { key: s.key, code: s.code ?? (s.key.length === 1 ? `Key${s.key.toUpperCase()}` : s.key), modifiers: mods, windowsVirtualKeyCode: keyCode[s.key] ?? s.key.toUpperCase().charCodeAt(0) };
    await send('Input.dispatchKeyEvent', { type: 'keyDown', ...base, text: s.key.length === 1 && !(mods & 6) ? s.key : undefined });
    await send('Input.dispatchKeyEvent', { type: 'keyUp', ...base });
  }
  if (s.type) await send('Input.insertText', { text: s.type });
  if (s.shot) {
    const r = await send('Page.captureScreenshot', { format: 'png' });
    writeFileSync(join(out, s.shot), Buffer.from(r.result.data, 'base64'));
    console.log('SHOT', join(out, s.shot));
  }
}
ws.close();
chrome.kill();
process.exit(0);
