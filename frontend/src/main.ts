import './style.css';
import {GetSettings, SaveSettings, PickWorkspace, NewSession, ListSessions, LoadSession, DeleteSession, SendMessage, CancelRun, ResolveApproval, FetchModels, MuseStatus} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';

// ContextStats is a new Go binding; call it through the bridge defensively
// so the UI also runs against bindings generated before it existed.
async function contextStats(id: string): Promise<any | null> {
  try {
    const w = window as any;
    const fn = w?.go?.main?.App?.ContextStats;
    if (typeof fn !== 'function') return null;
    return await fn(id);
  } catch {
    return null;
  }
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const escAttr = (s: string) => esc(s).replace(/"/g, '&quot;');
const basename = (p: string) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;

// ---------- icons ----------
const icon = (d: string, size = 16) =>
  `<svg width="${size}" height="${size}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}</svg>`;

const svg = {
  folder: icon('<path d="M1.75 4.25c0-.69.56-1.25 1.25-1.25h2.7l1.3 1.5H13c.69 0 1.25.56 1.25 1.25v6c0 .69-.56 1.25-1.25 1.25H3c-.69 0-1.25-.56-1.25-1.25v-7.5z"/>', 15),
  plus: icon('<path d="M8 3.25v9.5M3.25 8h9.5"/>', 15),
  trash: icon('<path d="M2.75 4.25h10.5M6.5 2.5h3M4.25 4.25l.6 8.6c.05.7.63 1.15 1.3 1.15h3.7c.67 0 1.25-.45 1.3-1.15l.6-8.6"/>', 14),
  stop: '<svg width="11" height="11" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true"><rect x="1" y="1" width="10" height="10" rx="2.5"/></svg>',
  gear: icon('<circle cx="8" cy="8" r="2.1"/><path d="M8 1.75v1.6M8 12.65v1.6M1.75 8h1.6M12.65 8h1.6M3.6 3.6l1.1 1.1M11.3 11.3l1.1 1.1M12.4 3.6l-1.1 1.1M4.7 11.3l-1.1 1.1"/>'),
  send: icon('<path d="M8 13V3.5M3.75 7.75L8 3.5l4.25 4.25"/>', 16),
  search: icon('<circle cx="7" cy="7" r="4.25"/><path d="M10.25 10.25l3.25 3.25"/>', 14),
  file: icon('<path d="M4 1.75h5l3.25 3.25v8.25c0 .55-.45 1-1 1H4c-.55 0-1-.45-1-1v-10.5c0-.55.45-1 1-1z"/><path d="M9 1.75V5h3.25"/>', 14),
  filePlus: icon('<path d="M4 1.75h5l3.25 3.25v8.25c0 .55-.45 1-1 1H4c-.55 0-1-.45-1-1v-10.5c0-.55.45-1 1-1z"/><path d="M7.75 7.25v4.5M5.5 9.5H10"/>', 14),
  pencil: icon('<path d="M10.5 2.75l2.75 2.75L6 12.75l-3.25.5.5-3.25z"/>', 14),
  terminal: icon('<rect x="1.75" y="2.75" width="12.5" height="10.5" rx="1.75"/><path d="M4.5 6.25L6.5 8l-2 1.75M8.25 10h3"/>', 14),
  list: icon('<path d="M5.75 4h7.5M5.75 8h7.5M5.75 12h7.5M2.75 4h.01M2.75 8h.01M2.75 12h.01"/>', 14),
  tool: icon('<path d="M10.5 2.5c1.4 0 2.6 1.1 2.9 2.6l-3.3 3.3-2.3-.6-.6-2.3 3.3-3zM6 8l-3.5 3.5c-.7.7-.7 1.8 0 2.5s1.8.7 2.5 0L8.5 10.5"/>', 14),
  check: icon('<path d="M3.5 8.25l3 3 6-6.5"/>', 12),
  x: icon('<path d="M4.25 4.25l7.5 7.5M11.75 4.25l-7.5 7.5"/>', 14),
  shield: icon('<path d="M8 1.75l5.25 2v4c0 3.1-2.2 5.6-5.25 6.5-3.05-.9-5.25-3.4-5.25-6.5v-4z"/><path d="M5.75 8l1.6 1.6 2.9-3.1"/>', 20),
  chevron: icon('<path d="M6 4l4 4-4 4"/>', 12),
  copy: icon('<rect x="5.25" y="5.25" width="8" height="8" rx="1.5"/><path d="M10.75 5.25v-1.5c0-.55-.45-1-1-1h-6c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h1.5"/>', 13),
  compass: icon('<circle cx="8" cy="8" r="6.25"/><path d="M10.6 5.4l-1.5 3.7-3.7 1.5 1.5-3.7z"/>', 18),
  bug: icon('<rect x="4.75" y="4.5" width="6.5" height="9" rx="3.25"/><path d="M8 7.5v6M2.5 9h2.25M11.25 9h2.25M3 5l1.9 1.3M13 5l-1.9 1.3M3 13.25l1.9-1.25M13 13.25l-1.9-1.25M6.25 2.75l.75 1.5M9.75 2.75L9 4.25"/>', 18),
  flask: icon('<path d="M6 1.75h4M6.5 1.75v4.5l-3.6 6.15c-.4.7.1 1.6.9 1.6h8.4c.8 0 1.3-.9.9-1.6L9.5 6.25v-4.5M4.4 10.25h7.2"/>', 18),
  doc: icon('<path d="M4 1.75h5l3.25 3.25v8.25c0 .55-.45 1-1 1H4c-.55 0-1-.45-1-1v-10.5c0-.55.45-1 1-1z"/><path d="M9 1.75V5h3.25M5.5 8.25h5M5.5 10.75h3.5"/>', 18),
  spark: '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M12 2.5c.7 5.1 4.4 8.8 9.5 9.5-5.1.7-8.8 4.4-9.5 9.5-.7-5.1-4.4-8.8-9.5-9.5 5.1-.7 8.8-4.4 9.5-9.5z"/></svg>',
};

const orb = (cls = '') => `<span class="orb ${cls}" aria-hidden="true">${svg.spark}</span>`;

const toolIcons: Record<string, string> = {
  list_files: svg.list, read_file: svg.file, search: svg.search,
  write_file: svg.filePlus, edit_file: svg.pencil, run_command: svg.terminal,
};

const SUGGESTIONS = [
  {icon: svg.compass, title: 'Map this codebase', desc: 'Architecture, entry points and how the pieces fit.',
    prompt: 'Give me a tour of this codebase: the architecture, the main entry points, and how the pieces fit together.'},
  {icon: svg.bug, title: 'Hunt for bugs', desc: 'Find likely bugs and risky code paths.',
    prompt: 'Look for likely bugs or risky code paths in this project and list the most important ones with file references.'},
  {icon: svg.flask, title: 'Run the tests', desc: 'Run the suite and fix whatever fails.',
    prompt: "Run the project's tests. If anything fails, find the cause and propose a fix."},
  {icon: svg.doc, title: 'Write a README', desc: 'Draft or refresh the project docs.',
    prompt: 'Read through the project and draft a clear README covering what it does, setup, and usage.'},
];

// ---------- markdown ----------
function blockHtml(p: string): string {
  let out = '';
  let para: string[] = [];
  let list: {tag: string; items: string[]} | null = null;
  const flushPara = () => { if (para.length) { out += `<p>${para.join('<br>')}</p>`; para = []; } };
  const flushList = () => {
    if (list) { out += `<${list.tag}>${list.items.map((i) => `<li>${i}</li>`).join('')}</${list.tag}>`; list = null; }
  };
  for (const raw of p.split('\n')) {
    const l = raw.trim();
    const ul = l.match(/^[-*]\s+(.*)$/), ol = l.match(/^\d+[.)]\s+(.*)$/);
    const item = ul || ol;
    if (item) {
      const tag = ul ? 'ul' : 'ol';
      flushPara();
      if (list && list.tag !== tag) flushList();
      if (!list) list = {tag, items: []};
      list.items.push(item[1]);
      continue;
    }
    flushList();
    const hm = l.match(/^(#{1,3})\s+(.+)$/);
    if (hm) { flushPara(); out += `<h${hm[1].length}>${hm[2]}</h${hm[1].length}>`; }
    else if (l) para.push(l);
  }
  flushPara(); flushList();
  return out;
}

function md(s: string): string {
  const fences: string[] = [];
  const fence = (lang: string, code: string) => {
    fences.push(`<div class="code"><div class="code-head"><span>${lang || 'text'}</span>` +
      `<button class="copy" type="button" title="Copy code">${svg.copy}<span>Copy</span></button></div>` +
      `<pre>${code.replace(/^\n/, '').replace(/\n$/, '')}</pre></div>`);
    return `\n\n\u0000${fences.length - 1}\u0000\n\n`;
  };
  let h = esc(s);
  h = h.replace(/```(\w*)\n([\s\S]*?)```/g, (_m, lang, code) => fence(lang, code));
  // An unterminated fence while streaming still renders as code.
  h = h.replace(/```(\w*)\n?([\s\S]*)$/, (_m, lang, code) => fence(lang, code));
  h = h.replace(/`([^`\n]+)`/g, '<code>$1</code>').replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>');
  return h.split(/\n{2,}/).map((p) => {
    if (/^\u0000\d+\u0000$/.test(p.trim())) return p.trim();
    return blockHtml(p);
  }).join('').replace(/\u0000(\d+)\u0000/g, (_m, i) => fences[Number(i)]);
}

function diffHtml(d: string): string {
  if (!d) return '';
  return `<div class="diff">${d.split('\n').map((l) => {
    const c = l.startsWith('@@') ? 'hunk' : (l.startsWith('+++') || l.startsWith('---')) ? 'meta'
      : l.startsWith('+') ? 'add' : l.startsWith('-') ? 'del' : '';
    return `<div class="${c}">${esc(l) || ' '}</div>`;
  }).join('')}</div>`;
}

function diffStats(d: string): string {
  if (!d) return '';
  let add = 0, del = 0;
  for (const l of d.split('\n')) {
    if (l.startsWith('+++') || l.startsWith('---')) continue;
    if (l.startsWith('+')) add++; else if (l.startsWith('-')) del++;
  }
  return `<span class="dstat"><span class="a">+${add}</span><span class="d">−${del}</span></span>`;
}

function relTime(iso: string): string {
  const t = new Date(iso).getTime();
  if (!t) return '';
  const mins = Math.max(0, Math.floor((Date.now() - t) / 60000));
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

function dayBucket(iso: string): string {
  const t = new Date(iso).getTime();
  if (!t) return 'Earlier';
  const start = new Date(); start.setHours(0, 0, 0, 0);
  const day = 86400000;
  if (t >= start.getTime()) return 'Today';
  if (t >= start.getTime() - day) return 'Yesterday';
  if (t >= start.getTime() - 6 * day) return 'Previous 7 days';
  return 'Earlier';
}

function argTarget(args: string): string {
  try {
    const a = JSON.parse(args || '{}');
    const v = a.command ?? a.pattern ?? a.path ?? '';
    return typeof v === 'string' ? v : '';
  } catch {
    return '';
  }
}

function prettyArgs(args: string): string {
  try { return JSON.stringify(JSON.parse(args || '{}'), null, 2); }
  catch { return args || ''; }
}

// ---------- shell ----------
document.querySelector('#app')!.innerHTML = `
<aside>
  <div class="brand">${orb()}<span class="wordmark">muse</span><span class="brand-tag">agent</span></div>
  <div class="side-top">
    <button id="new" class="btn primary block" type="button">${svg.plus} New chat</button>
    <p class="side-label">Workspace</p>
    <button id="pick" class="ws-card empty" type="button" title="Choose workspace folder">
      <span class="ws-icon">${svg.folder}</span>
      <span class="ws-text"><b id="wsName">No folder yet</b><span id="ws">Pick the project Muse may work in</span></span>
      <span class="ws-action" id="wsAction">Choose</span>
    </button>
  </div>
  <div id="sessions" role="listbox" aria-label="Chats"></div>
  <div class="side-foot">
    <button id="backendBtn" class="backend" type="button" title="Backend status — open settings">
      <span class="bdot" id="bdot"></span><span id="backendText">Checking backend…</span>
    </button>
    <button id="settingsBtn" class="icon-btn" type="button" title="Settings" aria-label="Settings">${svg.gear}</button>
  </div>
</aside>
<main>
  <header>
    <div class="head-title"><span id="title">New chat</span><span class="run-state hidden" id="runState"><span class="run-dot"></span>Working</span></div>
    <span class="chip warn hidden" id="autoChip" title="Writes and commands run without asking. Turn off in Settings.">Auto-approve on</span>
    <button class="chip model" id="modelChip" type="button" title="Model and backend — open settings"><span class="chip-spark">${svg.spark}</span><span id="modelText">…</span></button>
    <span class="ctx hidden" id="ctx" title="Estimated context usage"><span class="bar"><i id="ctxBar" style="width:0%"></i></span><span id="ctxText"></span></span>
  </header>
  <div id="chat" role="log" aria-live="off" aria-label="Conversation"></div>
  <div class="composer-wrap">
    <div class="composer">
      <textarea id="input" rows="1" placeholder="Ask Muse to inspect or change code…" aria-label="Message the agent"></textarea>
      <div class="composer-bar">
        <span class="composer-ws" id="composerWs" title="">${svg.folder}<span id="composerWsText">No workspace</span></span>
        <span class="composer-hint"><kbd>Enter</kbd> to send · <kbd>Shift</kbd> + <kbd>Enter</kbd> for a new line</span>
        <button id="stop" class="send-btn stop hidden" type="button" title="Stop" aria-label="Stop">${svg.stop}</button>
        <button id="send" class="send-btn" type="button" title="Send" aria-label="Send" disabled>${svg.send}</button>
      </div>
    </div>
  </div>
  <div class="toasts" id="toasts" aria-live="polite"></div>
</main>
<div class="modal hidden" id="approval" role="dialog" aria-modal="true" aria-labelledby="apTitle"><div class="card approval">
  <div class="ap-head"><span class="ap-icon">${svg.shield}</span><div><h3 id="apTitle">Allow this action?</h3>
    <p class="muted">Muse wants to run a tool that can change your workspace.</p></div></div>
  <div class="ap-tool"><span class="ticon" id="apIcon"></span><span id="apTool" class="tname"></span><span id="apTarget" class="ap-target"></span></div>
  <div id="apDiff"></div>
  <details id="apArgsBox" class="ap-args"><summary>Arguments</summary><pre id="apArgs"></pre></details>
  <div class="btn-row end"><button id="apNo" class="btn" type="button">Deny <kbd>Esc</kbd></button><button id="apYes" class="btn primary" type="button">Approve <kbd>Enter</kbd></button></div>
</div></div>
<div class="modal hidden" id="confirm" role="alertdialog" aria-modal="true" aria-labelledby="cfTitle" aria-describedby="cfBody"><div class="card small">
  <h3 id="cfTitle"></h3><p class="muted" id="cfBody"></p>
  <div class="btn-row end"><button id="cfNo" class="btn ghost" type="button">Cancel</button><button id="cfYes" class="btn danger" type="button">Delete</button></div>
</div></div>
<div class="modal hidden" id="settings" role="dialog" aria-modal="true" aria-labelledby="setTitle"><div class="card settings">
  <div class="card-head"><h3 id="setTitle">Settings</h3><button id="sX" class="icon-btn" type="button" aria-label="Close settings">${svg.x}</button></div>
  <div class="card-body">
    <section>
      <h4>Backend</h4>
      <div class="seg" role="radiogroup" aria-label="Backend">
        <label class="opt"><input type="radio" name="backend" value="cli"><span class="opt-body"><b>Muse subscription</b><span>Drives the official Muse Code CLI (<code>muse exec</code>). No API key.</span></span></label>
        <label class="opt"><input type="radio" name="backend" value="api"><span class="opt-body"><b>Model API key</b><span>Meta Model API or any OpenAI-compatible endpoint, pay-as-you-go.</span></span></label>
      </div>
      <div class="status-line" id="cliInfo">Checking Muse CLI…</div>
    </section>
    <section>
      <h4>Model</h4>
      <label for="sModel">Model</label>
      <div class="input-row"><input id="sModel" list="models" spellcheck="false"><datalist id="models"></datalist><button id="sFetch" class="btn" type="button">Fetch models</button></div>
    </section>
    <section id="apiSec">
      <h4>API endpoint</h4>
      <label for="sPreset">Provider preset</label>
      <select id="sPreset"><option value="meta">Meta Model API (api.meta.ai/v1, muse-spark-1.3)</option><option value="custom">Custom OpenAI-compatible endpoint</option></select>
      <label for="sBase">Base URL</label><input id="sBase" spellcheck="false">
      <label for="sKey">API key</label><input id="sKey" type="password" placeholder="Not stored yet" autocomplete="off">
      <div class="sub" id="keyInfo"></div>
      <p class="hint">Leave blank to keep the existing key. Env MUSE_API_KEY / META_API_KEY / MODEL_API_KEY / OPENAI_API_KEY also work.</p>
      <label for="sTemp">Temperature <span class="opt-note">blank = provider default</span></label><input id="sTemp" type="number" step="0.1" min="0" max="2">
    </section>
    <section>
      <h4>Agent</h4>
      <div class="grid2">
        <div><label for="sSteps">Max agent steps</label><input id="sSteps" type="number" min="1" max="100"></div>
        <div><label for="sBudget">Context budget <span class="opt-note">tokens, estimate</span></label><input id="sBudget" type="number" step="1000"></div>
      </div>
      <label class="check-row" for="sAuto"><input id="sAuto" type="checkbox"><span><b>Auto-approve writes &amp; commands</b><span class="sub">Skips the approval prompt for this session (API backend). Use with care.</span></span></label>
    </section>
    <details class="explain"><summary>How the backends work</summary><div class="notice"><b>Subscription backend:</b> sign in once in Meta's official CLI (run <code>muse</code> in a terminal, browser sign-in). This app then drives it headlessly with <code>muse exec --json</code> in your chosen workspace — the subscription credential stays inside that CLI and is never read here. In CLI mode the CLI owns tools, approvals (prompts off, its OS sandbox stays on) and sessions; this app's own approval modal and tools apply only to the API backend. API backend: use a Meta Model API key (pay-as-you-go) or any OpenAI-compatible provider.</div></details>
  </div>
  <div class="card-foot"><span class="sub" id="sMsg"></span><button id="sClose" class="btn ghost" type="button">Close</button><button id="sSave" class="btn primary" type="button">Save changes</button></div>
</div></div>`;

const $ = (id: string) => document.getElementById(id)!;
const chat = $('chat'), input = $('input') as HTMLTextAreaElement, sendBtn = $('send') as HTMLButtonElement;
let sessionId = '', workspace = '', running = false, streamingEl: HTMLElement | null = null, streamingText = '', replyModel = '';
let thinkingEl: HTMLElement | null = null;
let backendMode = 'cli', autoApproveOn = false;

const scrollChat = () => { chat.scrollTop = chat.scrollHeight; };

// ---------- toasts & confirm ----------
function toast(msg: string, kind: 'info' | 'warn' | 'error' = 'info', action?: {label: string; run: () => void}) {
  const t = document.createElement('div');
  t.className = 'toast ' + kind;
  t.setAttribute('role', kind === 'error' ? 'alert' : 'status');
  t.innerHTML = `<span>${esc(msg)}</span>${action ? `<button type="button" class="toast-act">${esc(action.label)}</button>` : ''}`;
  if (action) t.querySelector('button')!.addEventListener('click', () => { action.run(); t.remove(); });
  $('toasts').appendChild(t);
  setTimeout(() => { t.classList.add('out'); setTimeout(() => t.remove(), 260); }, action ? 6500 : 4000);
}

let confirmResolve: ((v: boolean) => void) | null = null;
function confirmDialog(title: string, body: string, okLabel: string): Promise<boolean> {
  $('cfTitle').textContent = title; $('cfBody').textContent = body; $('cfYes').textContent = okLabel;
  $('confirm').classList.remove('hidden');
  ($('cfNo') as HTMLButtonElement).focus();
  return new Promise((r) => { confirmResolve = r; });
}
function closeConfirm(v: boolean) {
  $('confirm').classList.add('hidden');
  confirmResolve?.(v); confirmResolve = null;
}
$('cfYes').addEventListener('click', () => closeConfirm(true));
$('cfNo').addEventListener('click', () => closeConfirm(false));

// ---------- workspace ----------
function setWorkspaceUI() {
  const card = $('pick');
  card.classList.toggle('empty', !workspace);
  $('wsName').textContent = workspace ? basename(workspace) : 'No folder yet';
  $('ws').textContent = workspace || 'Pick the project Muse may work in';
  $('wsAction').textContent = workspace ? 'Change' : 'Choose';
  card.title = workspace ? `Workspace: ${workspace} — click to change` : 'Choose workspace folder';
  $('composerWsText').textContent = workspace ? basename(workspace) : 'No workspace';
  $('composerWs').title = workspace;
  if ($('hero')) renderEmpty();
}

// ---------- chat rendering ----------
function modelCaption(model: string): string {
  return model ? `<div class="model-caption">${esc(model)}</div>` : '';
}

function msgHtml(role: string, text: string, model = ''): string {
  return role === 'user'
    ? `<div class="bubble">${md(text)}</div>`
    : `${orb('avatar')}<div class="body">${md(text)}${modelCaption(model)}</div>`;
}

function clearEmpty() { document.getElementById('hero')?.remove(); }

function renderEmpty() {
  // Approval prompts only exist on the API backend; the CLI runs headless in
  // its own sandbox with prompts off, so say what actually applies.
  const safety = backendMode === 'api'
    ? (autoApproveOn ? 'Auto-approve is on: writes and commands run without asking.' : 'Reads run automatically; writes and commands ask first.')
    : 'It runs through the Muse Code CLI, inside its sandbox.';
  const sub = workspace
    ? `Working in <code title="${escAttr(workspace)}">${esc(basename(workspace))}</code>. ${safety}`
    : 'Choose a workspace folder to begin. Muse works inside the folder you pick.';
  const actions = workspace
    ? `<div class="suggest">${SUGGESTIONS.map((s, i) =>
      `<button class="sug" data-sug="${i}" type="button"><span class="sug-icon">${s.icon}</span><b>${s.title}</b><span>${s.desc}</span></button>`).join('')}</div>`
    : `<button class="btn primary lg" id="heroPick" type="button">${svg.folder} Choose workspace</button>`;
  chat.innerHTML = `<div class="hero" id="hero">${orb('big')}
    <h2>What should we build today?</h2>
    <p class="hero-sub">${sub}</p>${actions}</div>`;
}

function addMsg(role: string, text: string, model = '') {
  clearEmpty();
  const d = document.createElement('div');
  d.className = 'msg ' + role;
  d.innerHTML = msgHtml(role, text, model);
  chat.appendChild(d); scrollChat();
  return d;
}

function addNotice(text: string) {
  clearEmpty();
  const d = document.createElement('div');
  d.className = 'compact';
  d.textContent = text;
  chat.appendChild(d); scrollChat();
}

function addError(text: string) {
  clearEmpty();
  const el = document.createElement('div');
  el.className = 'err';
  el.setAttribute('role', 'alert');
  el.innerHTML = `<span class="err-icon">${svg.x}</span><span></span>`;
  el.lastElementChild!.textContent = text;
  chat.appendChild(el); scrollChat();
}

function showThinking() {
  clearThinking(); clearEmpty();
  const d = document.createElement('div');
  d.className = 'msg assistant thinking';
  d.innerHTML = `${orb('avatar live')}<div class="body"><span class="think-text">Thinking</span><span class="dots"><i></i><i></i><i></i></span></div>`;
  chat.appendChild(d); thinkingEl = d; scrollChat();
}
function clearThinking() { thinkingEl?.remove(); thinkingEl = null; }

function endStream() {
  streamingEl?.classList.remove('streaming');
  streamingEl = null;
}

function isCheckpoint(t: string): boolean {
  return t.startsWith('[Context checkpoint:');
}

function ensureStream(): HTMLElement {
  if (!streamingEl) {
    clearThinking();
    streamingEl = addMsg('assistant', '', replyModel);
    streamingEl.classList.add('streaming');
  }
  return streamingEl;
}

function renderStream() {
  const el = ensureStream();
  el.innerHTML = msgHtml('assistant', streamingText, replyModel);
  scrollChat();
}

function toolCard(status: string, name: string, args: string, result: string, diff: string): string {
  const st = status === 'running' ? 'running' : status === 'error' ? 'error' : 'done';
  const denied = (result || '').startsWith('user denied');
  const label = st === 'running' ? 'Running' : denied ? 'Denied' : st === 'error' ? 'Failed' : 'Done';
  const mark = st === 'running' ? '<span class="spin"></span>' : st === 'error' ? svg.x : svg.check;
  const target = argTarget(args);
  const a = args ? prettyArgs(args) : '';
  const open = !!diff || st === 'error';
  return `<details${open ? ' open' : ''}><summary class="tool-head">` +
    `<span class="ticon">${toolIcons[name] || svg.tool}</span>` +
    `<span class="tname">${esc(name || 'tool')}</span>` +
    `<span class="ttarget"${target ? ` title="${escAttr(target)}"` : ''}>${esc(target)}</span>` +
    `${diffStats(diff)}<span class="tstatus ${st}">${mark}${label}</span><span class="chev">${svg.chevron}</span></summary>` +
    `<div class="tool-body">` +
    `${a && a !== '{}' ? `<div class="tsec"><div class="tlabel">Arguments</div><pre>${esc(a)}</pre></div>` : ''}` +
    `${result ? `<div class="tsec"><div class="tlabel">${st === 'error' ? 'Error' : 'Result'}</div><pre class="tresult">${esc(result)}</pre></div>` : ''}` +
    `${diffHtml(diff || '')}</div></details>`;
}

// ---------- sessions ----------
async function refreshSessions() {
  const list: any[] = await ListSessions().catch(() => []);
  let lastBucket = '';
  const items = list.map((s: any) => {
    const bucket = dayBucket(s.updatedAt);
    const head = bucket !== lastBucket ? `<div class="sess-group" role="presentation">${bucket}</div>` : '';
    lastBucket = bucket;
    const meta = [s.workspace ? basename(s.workspace) : '', s.updatedAt ? relTime(s.updatedAt) : ''].filter(Boolean).join(' · ');
    const active = s.id === sessionId;
    return `${head}<div class="sess ${active ? 'active' : ''}" role="option" aria-selected="${active}" tabindex="0" data-id="${escAttr(s.id)}" title="${escAttr(s.workspace || '')}">` +
      `<span class="sess-text"><b>${esc(s.title || 'Untitled')}</b><span>${esc(meta)}</span></span>` +
      `<button class="sess-del" type="button" data-del="${escAttr(s.id)}" title="Delete chat" aria-label="Delete chat ${escAttr(s.title || 'Untitled')}">${svg.trash}</button></div>`;
  }).join('');
  $('sessions').innerHTML = items || `<div class="empty-note">No chats yet.<br>Start one with <b>New chat</b>.</div>`;
}

$('sessions').addEventListener('click', (e) => {
  const t = e.target as HTMLElement;
  const del = t.closest('.sess-del') as HTMLElement | null;
  if (del) { e.stopPropagation(); deleteChat(del.dataset.del!); return; }
  const item = t.closest('.sess') as HTMLElement | null;
  if (item) openSession(item.dataset.id!);
});
$('sessions').addEventListener('keydown', (e) => {
  const item = (e.target as HTMLElement).closest('.sess') as HTMLElement | null;
  if (!item || e.target !== item) return;
  if (e.key === 'Enter') openSession(item.dataset.id!);
  else if (e.key === 'Delete') deleteChat(item.dataset.id!);
});

async function deleteChat(id: string) {
  const ok = await confirmDialog('Delete this chat?', 'Its conversation history is removed from this computer. Files in your workspace are not touched.', 'Delete chat');
  if (!ok) return;
  await DeleteSession(id);
  if (id === sessionId) {
    sessionId = ''; $('title').textContent = 'New chat';
    renderEmpty();
  }
  await refreshSessions();
  await updateCtx();
}

async function updateCtx() {
  const box = $('ctx'), txt = $('ctxText'), bar = $('ctxBar') as HTMLElement;
  box.classList.toggle('hidden', !sessionId);
  if (!sessionId) { txt.textContent = ''; box.classList.remove('hot'); bar.style.width = '0%'; return; }
  const st = await contextStats(sessionId);
  if (!st) { txt.textContent = ''; box.classList.add('hidden'); return; }
  const est = st.estimatedTokens || 0, budget = st.budgetTokens || 1;
  const pct = Math.min(100, Math.round((est / budget) * 100));
  const fmt = (n: number) => n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`;
  txt.textContent = `${fmt(est)} / ${fmt(budget)}`;
  bar.style.width = `${pct}%`;
  box.classList.toggle('hot', pct >= 80);
  const u = st.usage || {};
  const hit = u.cacheHitTokens || 0, miss = u.cacheMissTokens || 0;
  const rate = hit + miss > 0 ? `, cache hit ${(100 * hit / (hit + miss)).toFixed(0)}%` : '';
  const comp = st.compactedMsgs ? `, ${st.compactedMsgs} msgs compacted` : '';
  box.title = `Estimated context ${est} of ${budget} tokens${comp}${rate}`;
}

async function openSession(id: string) {
  const s: any = await LoadSession(id);
  sessionId = s.id; workspace = s.workspace || workspace;
  $('title').textContent = s.title || 'Chat';
  endStream(); clearThinking();
  chat.innerHTML = '';
  setWorkspaceUI();
  const calls: Record<string, string> = {};
  (s.messages || []).forEach((m: any) => {
    if (m.role === 'assistant') (m.tool_calls || []).forEach((c: any) => { if (c.id) calls[c.id] = c.arguments || ''; });
    if (m.role === 'user' && isCheckpoint(m.content || '')) { addNotice(m.content || ''); return; }
    if (m.role === 'user') addMsg('user', m.content || '');
    else if (m.role === 'assistant' && m.content) addMsg('assistant', m.content, m.model || '');
    else if (m.role === 'tool') {
      const c = m.content || '';
      const err = c.startsWith('error:') || c.startsWith('user denied');
      const d = document.createElement('div');
      d.className = 'tool ' + (err ? 'error' : 'done');
      d.innerHTML = toolCard(err ? 'error' : 'done', m.name || 'tool', calls[m.tool_call_id] || '', c, '');
      chat.appendChild(d);
    }
  });
  if (!chat.children.length) renderEmpty();
  scrollChat();
  await refreshSessions();
  await updateCtx();
}

async function newChat() {
  const m: any = await NewSession(workspace);
  sessionId = m.id; $('title').textContent = 'New chat';
  endStream(); clearThinking();
  renderEmpty();
  await refreshSessions();
  await updateCtx();
  input.focus();
}

function setRunning(on: boolean) {
  running = on;
  sendBtn.disabled = on || !input.value.trim();
  sendBtn.classList.toggle('hidden', on);
  $('stop').classList.toggle('hidden', !on);
  $('runState').classList.toggle('hidden', !on);
}

async function pickWorkspace() {
  const d = await PickWorkspace().catch((e) => { toast(String(e), 'error'); return ''; });
  if (!d) return;
  workspace = d;
  setWorkspaceUI();
  if (!sessionId) await newChat();
  else toast(`Workspace set to ${basename(d)}. It applies to new chats.`, 'info', {label: 'New chat', run: newChat});
}

$('pick').addEventListener('click', pickWorkspace);
$('new').addEventListener('click', newChat);
$('stop').addEventListener('click', () => CancelRun());
$('send').addEventListener('click', send);

chat.addEventListener('click', async (e) => {
  const t = e.target as HTMLElement;
  const copy = t.closest('.copy') as HTMLElement | null;
  if (copy) {
    const code = copy.closest('.code')?.querySelector('pre')?.textContent || '';
    try {
      await navigator.clipboard.writeText(code);
      copy.classList.add('done'); copy.lastElementChild!.textContent = 'Copied';
      setTimeout(() => { copy.classList.remove('done'); copy.lastElementChild!.textContent = 'Copy'; }, 1400);
    } catch { toast('Could not copy to clipboard.', 'error'); }
    return;
  }
  const sug = t.closest('.sug') as HTMLElement | null;
  if (sug) {
    input.value = SUGGESTIONS[Number(sug.dataset.sug)].prompt;
    autoresize(); input.focus();
    return;
  }
  if (t.closest('#heroPick')) pickWorkspace();
});

function autoresize() {
  input.style.height = 'auto';
  input.style.height = Math.min(220, input.scrollHeight) + 'px';
  sendBtn.disabled = running || !input.value.trim();
}
input.addEventListener('input', autoresize);
input.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
});

async function send() {
  const text = input.value.trim();
  if (!text || running) return;
  if (!workspace) {
    toast('Choose a workspace folder first.', 'warn', {label: 'Choose', run: pickWorkspace});
    return;
  }
  if (!sessionId) await newChat();
  input.value = ''; autoresize(); addMsg('user', text);
  setRunning(true); streamingEl = null; replyModel = '';
  showThinking();
  try { await SendMessage(sessionId, text); }
  catch (e) { addError('Error: ' + String(e)); done(); }
}

function done() {
  clearThinking(); endStream();
  setRunning(false);
  refreshSessions(); updateCtx();
}

// ---------- backend events ----------
EventsOn('chat:delta', (d: any) => {
  if (d.sessionID !== sessionId) return;
  if (d.model) replyModel = d.model;
  // A new reply bubble starts from empty text; reset before appending so the
  // first chunk is not lost.
  if (!streamingEl) streamingText = '';
  streamingText += d.text || '';
  renderStream();
});

let pendingTool: HTMLElement | null = null;
EventsOn('chat:tool', (d: any) => {
  if (d.sessionID !== sessionId) return;
  clearThinking(); endStream(); clearEmpty();
  const el = pendingTool ? pendingTool : document.createElement('div');
  if (d.status === 'running') pendingTool = el; else pendingTool = null;
  el.className = 'tool ' + (d.status || '');
  el.innerHTML = toolCard(d.status || '', d.name || '', d.args || '', d.result || '', d.diff || '');
  if (!el.isConnected) chat.appendChild(el);
  if (d.status !== 'running' && running) showThinking();
  scrollChat();
});

EventsOn('chat:compact', (d: any) => {
  if (d.sessionID !== sessionId) return;
  clearThinking(); endStream();
  addNotice(d.text || 'Older context compacted into a checkpoint summary.');
  if (running) showThinking();
  updateCtx();
});

EventsOn('chat:done', (d: any) => {
  if (d.sessionID !== sessionId) return;
  if (d.model && streamingEl) { replyModel = d.model; renderStream(); }
  done();
});
EventsOn('chat:error', (d: any) => {
  if (d.sessionID !== sessionId) return;
  clearThinking();
  addError('Error: ' + (d.message || 'unknown'));
  done();
});

let approvalId = '';
EventsOn('approval:request', (d: any) => {
  approvalId = d.id;
  const tool = d.tool || '';
  $('apIcon').innerHTML = toolIcons[tool] || svg.tool;
  $('apTool').textContent = tool;
  $('apTarget').textContent = argTarget(d.args || '');
  $('apArgs').textContent = prettyArgs(d.args || '');
  $('apDiff').innerHTML = diffHtml(d.diff || '');
  ($('apArgsBox') as HTMLDetailsElement).open = !d.diff;
  $('approval').classList.remove('hidden');
  ($('apYes') as HTMLButtonElement).focus();
});
function closeApproval(approved: boolean) {
  $('approval').classList.add('hidden');
  ResolveApproval(approvalId, approved);
}
$('apYes').addEventListener('click', () => closeApproval(true));
$('apNo').addEventListener('click', () => closeApproval(false));
document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  if (!$('approval').classList.contains('hidden')) closeApproval(false);
  else if (!$('confirm').classList.contains('hidden')) closeConfirm(false);
  else if (!$('settings').classList.contains('hidden')) closeSettings();
});

// ---------- settings ----------
const backendRadios = () => Array.from(document.querySelectorAll<HTMLInputElement>('input[name="backend"]'));
const selectedBackend = () => backendRadios().find((r) => r.checked)?.value || 'cli';
function updateBackendUI() {
  const b = selectedBackend();
  $('apiSec').classList.toggle('hidden', b !== 'api');
  $('cliInfo').classList.toggle('hidden', b !== 'cli');
}
backendRadios().forEach((r) => r.addEventListener('change', updateBackendUI));

async function refreshStatus(s?: any) {
  const st: any = s || await GetSettings().catch(() => null);
  if (!st) return;
  backendMode = st.backend === 'api' ? 'api' : 'cli';
  autoApproveOn = !!st.autoApprove;
  if ($('hero')) renderEmpty();
  const model = st.model || 'default model';
  $('modelText').textContent = `${model} · ${st.backend === 'api' ? 'API' : 'CLI'}`;
  $('autoChip').classList.toggle('hidden', !(st.autoApprove && st.backend === 'api'));
  const dot = $('bdot'), txt = $('backendText');
  if (st.backend === 'api') {
    dot.className = 'bdot ' + (st.hasAPIKey ? 'ok' : 'warn');
    txt.textContent = st.hasAPIKey ? `API key …${st.apiKeyLast4}` : 'API backend · no key';
  } else {
    const c: any = await MuseStatus().catch(() => null);
    const ver = (c?.version || '').split('\n')[0].slice(0, 32);
    dot.className = 'bdot ' + (c?.found ? 'ok' : 'warn');
    txt.textContent = c?.found ? `Muse CLI${ver ? ' ' + ver : ''}` : 'Muse CLI not found';
  }
}

async function openSettings() {
  const s: any = await GetSettings();
  backendRadios().forEach((r) => { r.checked = r.value === (s.backend || 'cli'); });
  updateBackendUI();
  MuseStatus().then((c: any) => {
    $('cliInfo').textContent = c.found
      ? `Muse CLI found: ${c.version || ''} (${c.path}). Subscription sign-in lives in that CLI — run \`muse\` once to sign in.`
      : 'Muse CLI not found on PATH. Install Muse Code, run `muse` once and sign in, or use the API backend.';
    $('cliInfo').classList.toggle('bad', !c.found);
  }).catch(() => { $('cliInfo').textContent = ''; });
  ($('sBase') as HTMLInputElement).value = s.baseURL || '';
  ($('sModel') as HTMLInputElement).value = s.model || '';
  ($('sTemp') as HTMLInputElement).value = s.hasTemperature ? String(s.temperature) : '';
  ($('sSteps') as HTMLInputElement).value = String(s.maxSteps || 25);
  ($('sBudget') as HTMLInputElement).value = String(s.contextBudgetTokens || 96000);
  ($('sAuto') as HTMLInputElement).checked = !!s.autoApprove;
  ($('sPreset') as HTMLSelectElement).value = (s.baseURL === 'https://api.meta.ai/v1') ? 'meta' : 'custom';
  $('keyInfo').textContent = s.hasAPIKey
    ? `Key present (…${s.apiKeyLast4})${s.apiKeyFromEnv ? ' from environment' : ''}. Stored plaintext in your OS config dir settings.json — not encrypted.`
    : 'No key yet.';
  $('settings').classList.remove('hidden');
  refreshStatus(s);
}
function closeSettings() { $('settings').classList.add('hidden'); }
$('settingsBtn').addEventListener('click', openSettings);
$('backendBtn').addEventListener('click', openSettings);
$('modelChip').addEventListener('click', openSettings);
$('sClose').addEventListener('click', closeSettings);
$('sX').addEventListener('click', closeSettings);
($('sPreset') as HTMLSelectElement).addEventListener('change', (e) => {
  if ((e.target as HTMLSelectElement).value === 'meta') {
    ($('sBase') as HTMLInputElement).value = 'https://api.meta.ai/v1';
    ($('sModel') as HTMLInputElement).value = 'muse-spark-1.3';
  }
});
$('sFetch').addEventListener('click', async () => {
  try {
    const ms: string[] = await FetchModels();
    $('models').innerHTML = ms.map((m) => `<option value="${escAttr(m)}">`).join('');
    $('sMsg').textContent = ms.length + ' models available';
  }
  catch (e) { $('sMsg').textContent = 'Fetch failed (free-text model still works): ' + String(e); }
});
$('sSave').addEventListener('click', async () => {
  const temp = ($('sTemp') as HTMLInputElement).value;
  await SaveSettings({
    backend: selectedBackend(),
    baseURL: ($('sBase') as HTMLInputElement).value, apiKey: ($('sKey') as HTMLInputElement).value,
    model: ($('sModel') as HTMLInputElement).value, temperature: temp === '' ? 0 : parseFloat(temp), hasTemperature: temp !== '',
    maxSteps: parseInt(($('sSteps') as HTMLInputElement).value || '25', 10),
    contextBudgetTokens: parseInt(($('sBudget') as HTMLInputElement).value || '96000', 10),
    autoApprove: ($('sAuto') as HTMLInputElement).checked, lastWorkspace: workspace,
  });
  ($('sKey') as HTMLInputElement).value = ''; $('sMsg').textContent = 'Saved.';
  updateCtx(); openSettings();
});

// ---------- boot ----------
(async () => {
  const s: any = await GetSettings().catch(() => null);
  if (s?.lastWorkspace) workspace = s.lastWorkspace;
  setWorkspaceUI();
  refreshStatus(s || undefined);
  await refreshSessions();
  const list: any[] = await ListSessions().catch(() => []);
  if (list.length) await openSession(list[0].id);
  else {
    renderEmpty();
    await updateCtx();
  }
})();
