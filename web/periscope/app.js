const api = path => fetch(path).then(r => r.json());

let activeLoomId = null;
let activeConvId = null;
let liveSocket = null;
let term = null;
let fitAddon = null;

// ── DOM refs ──────────────────────────────────────────────────────────────────
const loomList    = document.getElementById('loom-list');
const convList    = document.getElementById('conv-list');
const historyEl   = document.getElementById('history');
const injectInput = document.getElementById('inject-input');
const injectBtn   = document.getElementById('inject-btn');
const tabs        = document.querySelectorAll('.tab');
const panels      = document.querySelectorAll('.panel');

// ── xterm.js terminal ─────────────────────────────────────────────────────────
function initTerminal() {
  if (term) {
    term.dispose();
  }
  fitAddon = new FitAddon.FitAddon();
  term = new Terminal({
    theme: {
      background: '#0d1117',
      foreground: '#e1e4e8',
      cursor:     '#58a6ff',
      selection:  'rgba(88,166,255,0.3)',
    },
    fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
    fontSize: 13,
    lineHeight: 1.4,
    convertEol: true,
    scrollback: 5000,
  });
  term.loadAddon(fitAddon);
  const container = document.getElementById('terminal-container');
  container.innerHTML = '';
  term.open(container);
  fitAddon.fit();
  term.writeln('\x1b[2m— waiting for loom —\x1b[0m');
}

function sendResize() {
  if (liveSocket && liveSocket.readyState === WebSocket.OPEN && term) {
    liveSocket.send(JSON.stringify({type: 'resize', cols: term.cols, rows: term.rows}));
  }
}

window.addEventListener('resize', () => {
  if (fitAddon) fitAddon.fit();
  sendResize();
});

initTerminal();

// ── Tab switching ─────────────────────────────────────────────────────────────
tabs.forEach(tab => {
  tab.addEventListener('click', () => {
    tabs.forEach(t => t.classList.remove('active'));
    panels.forEach(p => p.classList.remove('active'));
    tab.classList.add('active');
    document.getElementById('panel-' + tab.dataset.panel).classList.add('active');
    if (tab.dataset.panel === 'history' && activeConvId) loadHistory(activeConvId);
    if (tab.dataset.panel === 'conversations') loadConversations();
    if (tab.dataset.panel === 'live') term && term.focus();
  });
});

// ── Looms ─────────────────────────────────────────────────────────────────────
async function loadLooms() {
  const looms = await api('/api/looms').catch(() => []);
  loomList.innerHTML = '';
  if (!looms.length) {
    loomList.innerHTML = '<div class="empty">No active looms</div>';
    return;
  }
  looms.forEach(l => {
    const el = document.createElement('div');
    el.className = 'loom-item' + (l.id === activeLoomId ? ' active' : '');
    el.innerHTML = `
      <div class="cmd"><span class="dot"></span>${escHtml(l.command)}</div>
      <div class="meta">${l.id.slice(0, 8)} · ${timeAgo(l.started_at)}</div>`;
    el.addEventListener('click', () => selectLoom(l, el));
    loomList.appendChild(el);
  });
}

function selectLoom(l, el) {
  activeLoomId = l.id;
  activeConvId = l.conversation_id;
  document.querySelectorAll('.loom-item').forEach(e => e.classList.remove('active'));
  el.classList.add('active');
  connectLive(l.id);
  tabs.forEach(t => t.classList.remove('active'));
  panels.forEach(p => p.classList.remove('active'));
  document.querySelector('[data-panel="live"]').classList.add('active');
  document.getElementById('panel-live').classList.add('active');
}

// ── Live output via xterm.js ──────────────────────────────────────────────────
function connectLive(loomId) {
  if (liveSocket) liveSocket.close();
  initTerminal();

  const wsUrl = `ws://${location.host}/api/looms/${loomId}/ws`;
  liveSocket = new WebSocket(wsUrl);
  liveSocket.binaryType = 'arraybuffer';

  liveSocket.onopen = () => sendResize();
  liveSocket.onmessage = e => {
    const data = e.data instanceof ArrayBuffer
      ? new Uint8Array(e.data)
      : e.data;
    term.write(data);
  };
  liveSocket.onerror = () => term.writeln('\r\n\x1b[31m[connection error]\x1b[0m');
  liveSocket.onclose = () => term.writeln('\r\n\x1b[2m[disconnected]\x1b[0m');
}

// ── Inject ────────────────────────────────────────────────────────────────────
injectBtn.addEventListener('click', sendInject);
injectInput.addEventListener('keydown', e => { if (e.key === 'Enter') sendInject(); });

async function sendInject() {
  const msg = injectInput.value.trim();
  if (!msg || !activeLoomId) return;
  injectInput.value = '';
  await fetch(`/api/looms/${activeLoomId}/inject`, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({message: msg}),
  });
}

// ── History ───────────────────────────────────────────────────────────────────
async function loadHistory(convId) {
  historyEl.innerHTML = '';
  const msgs = await api(`/api/conversations/${convId}/messages`).catch(() => []);
  if (!msgs.length) {
    historyEl.innerHTML = '<div class="empty">No messages yet</div>';
    return;
  }
  msgs.forEach(m => {
    const el = document.createElement('div');
    el.className = `message ${m.role}`;
    el.innerHTML = `<div class="role">${m.role}</div>${escHtml(m.content)}`;
    historyEl.appendChild(el);
  });
  historyEl.scrollTop = historyEl.scrollHeight;
}

// ── Conversations ─────────────────────────────────────────────────────────────
async function loadConversations() {
  const convs = await api('/api/conversations').catch(() => []);
  convList.innerHTML = '';
  if (!convs.length) {
    convList.innerHTML = '<div class="empty">No conversations</div>';
    return;
  }
  convs.forEach(c => {
    const el = document.createElement('div');
    el.className = 'conv-item' + (c.id === activeConvId ? ' active' : '');
    el.innerHTML = `
      <div class="cmd">${escHtml(c.command)}</div>
      <div class="meta">${c.id.slice(0, 8)} · ${fmtDate(c.started_at)}</div>`;
    el.addEventListener('click', () => {
      document.querySelectorAll('.conv-item').forEach(e => e.classList.remove('active'));
      el.classList.add('active');
      activeConvId = c.id;
      loadHistory(c.id);
      tabs.forEach(t => t.classList.remove('active'));
      panels.forEach(p => p.classList.remove('active'));
      document.querySelector('[data-panel="history"]').classList.add('active');
      document.getElementById('panel-history').classList.add('active');
    });
    convList.appendChild(el);
  });
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function escHtml(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
function timeAgo(iso) {
  const d = Math.floor((Date.now() - new Date(iso)) / 1000);
  if (d < 60) return `${d}s ago`;
  if (d < 3600) return `${Math.floor(d/60)}m ago`;
  return `${Math.floor(d/3600)}h ago`;
}
function fmtDate(iso) {
  return new Date(iso).toLocaleString();
}

// ── Init ──────────────────────────────────────────────────────────────────────
loadLooms();
setInterval(loadLooms, 5000);
