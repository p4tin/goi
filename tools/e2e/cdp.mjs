// Minimal headless-Chrome driver over CDP for checking the Goi dev server.
// usage: node cdp.mjs <steps.json>
// steps.json: { "url": "...", "out": "/dir", "steps": [ {"wait":ms} | {"eval":"js"} |
//   {"key":"Enter","mods":["Meta"]} | {"type":"text"} | {"shot":"name.png"} |
//   {"newTab":"url"} | {"tab":n} ] }
// Steps act on the current tab (0 is the first page); newTab opens another and makes it current.
// Optional "server": { "cmd": ["bin", ...args], "env": {...}, "port": 0 } starts a Wails v3
// server-mode build (WAILS_SERVER_PORT, a free port when 0) and replaces {{server}} in urls.
import { spawn } from 'node:child_process';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { createServer } from 'node:net';
import { join } from 'node:path';

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const spec = JSON.parse(readFileSync(process.argv[2], 'utf8'));
const out = spec.out;
mkdirSync(out, { recursive: true });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const freePort = () =>
  new Promise((resolve, reject) => {
    const srv = createServer();
    srv.once('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
  });

let server = null;
let serverURL = '';
if (spec.server) {
  const sport = spec.server.port || (await freePort());
  serverURL = `http://127.0.0.1:${sport}`;
  const [cmd, ...args] = spec.server.cmd;
  server = spawn(cmd, args, { stdio: ['ignore', 'inherit', 'inherit'], env: { ...process.env, ...spec.server.env, WAILS_SERVER_PORT: String(sport) } });
  let up = false;
  for (let i = 0; i < 100 && !up; i++) {
    try {
      await fetch(serverURL + '/');
      up = true;
    } catch {
      await sleep(100);
    }
  }
  if (!up) {
    server.kill();
    throw new Error(`server did not answer on ${serverURL}`);
  }
}
const expand = (url) => url.replaceAll('{{server}}', serverURL);

const port = 9333 + Math.floor(Math.random() * 500);
const chrome = spawn(CHROME, [
  '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
  `--user-data-dir=${join(out, 'profile')}`, `--remote-debugging-port=${port}`,
  '--window-size=1280,800', 'about:blank',
], { stdio: 'ignore' });
const stop = () => {
  chrome.kill();
  server?.kill();
};
for (const ev of ['uncaughtException', 'unhandledRejection']) {
  process.on(ev, (err) => {
    console.error(err);
    stop();
    process.exit(1);
  });
}

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

// connect opens a CDP connection to one page target. Console output is
// prefixed with the tab index for tabs after the first.
async function connect(target, index) {
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener('open', r, { once: true }));
  const tag = index > 0 ? `[tab ${index}] ` : '';
  let id = 0;
  const pending = new Map();
  ws.addEventListener('message', (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      pending.get(msg.id)(msg);
      pending.delete(msg.id);
    } else if (msg.method === 'Runtime.exceptionThrown') {
      console.log(tag + 'EXCEPTION', msg.params.exceptionDetails.exception?.description ?? msg.params.exceptionDetails.text);
    } else if (msg.method === 'Runtime.consoleAPICalled' && ['error', 'warning'].includes(msg.params.type)) {
      console.log(tag + 'CONSOLE', msg.params.type, msg.params.args.map((a) => a.value ?? a.description).join(' '));
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
  return { ws, send, index, tag };
}

const tabs = [await connect(await targets(), 0)];
let tab = tabs[0];
await tab.send('Page.navigate', { url: expand(spec.url) });
await sleep(spec.initialWait ?? 2500);

const keyCode = { Enter: 13, Escape: 27, Tab: 9, ArrowDown: 40, ArrowUp: 38, Backspace: 8, Shift: 16, '[': 219, ']': 221, ...Object.fromEntries(Array.from({ length: 12 }, (_, i) => [`F${i + 1}`, 112 + i])) };
for (const s of spec.steps) {
  if (s.newTab) {
    const r = await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: 'PUT' });
    tab = await connect(await r.json(), tabs.length);
    tabs.push(tab);
    await tab.send('Page.navigate', { url: expand(s.newTab) });
    await sleep(s.initialWait ?? spec.initialWait ?? 2500);
  }
  if (s.tab !== undefined) {
    tab = tabs[s.tab];
    if (!tab) throw new Error(`no tab ${s.tab}`);
  }
  const { send, tag } = tab;
  if (s.wait) await sleep(s.wait);
  if (s.eval) {
    const r = await send('Runtime.evaluate', { expression: s.eval, awaitPromise: true, returnByValue: true });
    const v = r.result?.result?.value ?? r.result?.exceptionDetails?.exception?.description;
    console.log(tag + 'EVAL', s.label ?? '', JSON.stringify(v));
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
    const name = tab.index > 0 ? s.shot.replace(/(\.png)?$/, `-tab${tab.index}$1`) : s.shot;
    writeFileSync(join(out, name), Buffer.from(r.result.data, 'base64'));
    console.log(tag + 'SHOT', join(out, name));
  }
}
for (const t of tabs) t.ws.close();
stop();
process.exit(0);
