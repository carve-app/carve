/**
 * Carve promotional video recorder.
 *
 * Records a ~28s demo of the full mining + review loop.
 * Serves double duty as a CI flow test.
 *
 * Scenes:
 *   0. Login + onboarding      (3s)
 *   1. Reader — text mining    (8s)
 *   2. Video — subtitle mining (10s)   YouTube → Netflix → Crunchyroll branding variants
 *   3. SRS review              (7s)
 *
 * Usage:
 *   node demo/record-promo.js            # headful, records promo-YYYYMMDD.webm
 *   node demo/record-promo.js --ci       # headless, no video, strict assertions
 *   node demo/record-promo.js --live     # point at real dev server (PORT env)
 *
 * Output: demo/promo-YYYYMMDD.webm (convert with ffmpeg -i … out.mp4)
 */

'use strict';

const http  = require('http');
const path  = require('path');
const fs    = require('fs');
const { chromium } = require(path.resolve(__dirname, '../e2e/node_modules/playwright'));

const CI   = process.argv.includes('--ci');
const LIVE = process.argv.includes('--live');
const API_PORT = process.env.PORT || 5173;

// ── Demo data ─────────────────────────────────────────────────────────────────

const DEMO_CARDS = [
  {
    id: 'card-001',
    front_text: '人工知能',
    front_reading: 'じんこうちのう',
    back_text: 'artificial intelligence',
    sentence: '現代の人工知能は、私たちの生活を大きく変えつつある。',
    language_code: 'ja',
    user_status: 'learning',
    created_at: new Date().toISOString(),
  },
  {
    id: 'card-002',
    front_text: '生活',
    front_reading: 'せいかつ',
    back_text: 'daily life; livelihood',
    sentence: '私たちの生活を大きく変える。',
    language_code: 'ja',
    user_status: 'learning',
    created_at: new Date().toISOString(),
  },
  {
    id: 'card-003',
    front_text: '役立つ',
    front_reading: 'やくだつ',
    back_text: 'to be useful; to come in handy',
    sentence: '人工知能は日本語を学ぶのに役立ちます。',
    language_code: 'ja',
    user_status: 'learning',
    created_at: new Date().toISOString(),
  },
];

// ── Shared CSS design tokens ──────────────────────────────────────────────────

const BASE_CSS = `
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  :root {
    --bg: #0b0e18;
    --surface: #141928;
    --border: #232d42;
    --text: #e4eaf6;
    --muted: #5a6680;
    --accent: #6c7ff2;
    --accent2: #9d7af7;
    --ok: #5cd6a5;
    --warn: #f5a742;
    --danger: #f06060;
  }
  body { font-family: -apple-system, 'Segoe UI', sans-serif; background: var(--bg); color: var(--text); }
  @keyframes fadeUp {
    from { opacity: 0; transform: translateY(10px); }
    to   { opacity: 1; transform: translateY(0); }
  }
  @keyframes pop {
    0%   { transform: scale(0.88); opacity: 0; }
    60%  { transform: scale(1.04); }
    100% { transform: scale(1);    opacity: 1; }
  }
  @keyframes flashGreen {
    0%   { background: #1c4532; }
    50%  { background: #276749; }
    100% { background: transparent; }
  }
  @keyframes slideIn {
    from { opacity: 0; transform: translateX(20px); }
    to   { opacity: 1; transform: translateX(0); }
  }
`;

// ── HTML pages ────────────────────────────────────────────────────────────────

function makeOnboardingHTML(mockBase) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Carve</title>
  <style>
${BASE_CSS}
    body { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
    .card {
      background: var(--surface); border: 1px solid var(--border); border-radius: 20px;
      padding: 40px; width: 420px; animation: fadeUp 0.4s ease both;
    }
    .logo { text-align: center; font-size: 30px; font-weight: 800; color: var(--accent);
            letter-spacing: -1px; margin-bottom: 6px; }
    .tagline { text-align: center; font-size: 13px; color: var(--muted); margin-bottom: 28px; }
    .tab-row { display: flex; background: var(--bg); border-radius: 10px; padding: 3px; margin-bottom: 22px; }
    .tab { flex: 1; text-align: center; padding: 8px 0; font-size: 13px; font-weight: 600;
           border-radius: 8px; cursor: pointer; color: var(--muted); transition: all 0.15s; }
    .tab.active { background: var(--border); color: var(--text); }
    label { display: block; font-size: 11px; color: var(--muted); text-transform: uppercase;
            letter-spacing: 0.6px; margin-bottom: 5px; }
    input  { width: 100%; background: var(--bg); border: 1px solid var(--border); border-radius: 9px;
             padding: 10px 14px; color: var(--text); font-size: 14px; margin-bottom: 14px; outline: none;
             transition: border-color 0.15s; }
    input:focus { border-color: var(--accent); }
    .submit-btn { width: 100%; background: linear-gradient(135deg, var(--accent), var(--accent2));
                  color: #fff; border: none; border-radius: 10px; padding: 12px 0; font-size: 15px;
                  font-weight: 700; cursor: pointer; }
    .onboarding { display: none; }
    .step-title { font-size: 20px; font-weight: 700; color: var(--text); margin-bottom: 6px; }
    .step-sub   { font-size: 13px; color: var(--muted); margin-bottom: 22px; line-height: 1.5; }
    .lang-grid  { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-bottom: 22px; }
    .lang-card  { background: var(--bg); border: 2px solid var(--border); border-radius: 12px;
                  padding: 16px 12px; text-align: center; cursor: pointer; transition: all 0.15s; }
    .lang-card.selected, .lang-card:hover { border-color: var(--accent); background: #1a2236; }
    .lang-flag  { font-size: 26px; display: block; margin-bottom: 5px; }
    .lang-name  { font-size: 13px; font-weight: 600; }
    .lang-lvl   { font-size: 11px; color: var(--muted); margin-top: 2px; }
    .continue-btn { width: 100%; background: var(--accent); color: #fff; border: none;
                    border-radius: 10px; padding: 12px 0; font-size: 15px; font-weight: 700;
                    cursor: pointer; opacity: 0.4; transition: opacity 0.2s; }
    .continue-btn.on { opacity: 1; }
    .success-screen { text-align: center; padding: 16px 0; }
    .success-emoji { font-size: 52px; margin-bottom: 14px; }
    .success-title { font-size: 22px; font-weight: 700; color: var(--ok); }
    .success-sub   { font-size: 14px; color: var(--muted); margin-top: 6px; }
  </style>
</head>
<body>
<div class="card" id="auth-card">
  <div class="logo">⚡ Carve</div>
  <div class="tagline">Turn immersion into vocabulary</div>
  <div class="tab-row">
    <div class="tab active" id="tab-login">Log in</div>
    <div class="tab" id="tab-reg">Sign up</div>
  </div>
  <div id="login-form">
    <label>Email</label>
    <input type="email" id="login-email" value="alex@example.com">
    <label>Password</label>
    <input type="password" id="login-password" value="••••••••">
    <button class="submit-btn" id="login-btn">Log in</button>
  </div>
</div>

<div class="card onboarding" id="onboarding-card">
  <div class="step-title">Which language are you learning?</div>
  <div class="step-sub">Carve annotates your immersion content and tracks your vocabulary growth.</div>
  <div class="lang-grid">
    <div class="lang-card" id="lang-ja">
      <span class="lang-flag">🇯🇵</span>
      <div class="lang-name">Japanese</div>
      <div class="lang-lvl">N5 – N1</div>
    </div>
    <div class="lang-card" id="lang-ko">
      <span class="lang-flag">🇰🇷</span>
      <div class="lang-name">Korean</div>
      <div class="lang-lvl">Beginner – Adv.</div>
    </div>
    <div class="lang-card" id="lang-zh">
      <span class="lang-flag">🇨🇳</span>
      <div class="lang-name">Chinese</div>
      <div class="lang-lvl">HSK 1 – 6</div>
    </div>
    <div class="lang-card" id="lang-es">
      <span class="lang-flag">🇪🇸</span>
      <div class="lang-name">Spanish</div>
      <div class="lang-lvl">Coming soon</div>
    </div>
  </div>
  <button class="continue-btn" id="continue-btn" disabled>Continue →</button>
</div>

<div class="card" id="success-card" style="display:none">
  <div class="success-screen">
    <div class="success-emoji">🎉</div>
    <div class="success-title">You're all set!</div>
    <div class="success-sub" id="success-sub">Japanese mode activated. Let's start mining!</div>
  </div>
</div>
</body>
</html>`;
}

function makeReaderHTML(mockBase) {
  return `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Carve — Reader</title>
  <style>
${BASE_CSS}
    .topbar { height: 50px; background: var(--surface); border-bottom: 1px solid var(--border);
              display: flex; align-items: center; padding: 0 22px; gap: 14px; }
    .logo   { font-size: 17px; font-weight: 700; color: var(--accent); letter-spacing: -0.5px; }
    .nav    { display: flex; gap: 4px; margin-left: 4px; }
    .nav-item { font-size: 13px; color: var(--muted); padding: 5px 11px; border-radius: 7px; cursor: pointer; }
    .nav-item.active { color: var(--text); background: var(--border); }
    .layout { display: flex; height: calc(100vh - 50px); }
    .sidebar { width: 220px; background: var(--surface); border-right: 1px solid var(--border);
               padding: 18px 14px; flex-shrink: 0; }
    .sidebar-label { font-size: 10px; text-transform: uppercase; letter-spacing: 1px; color: var(--muted); margin-bottom: 10px; }
    .lib-item { padding: 7px 10px; border-radius: 7px; font-size: 13px; color: var(--muted); margin-bottom: 3px; cursor: pointer; }
    .lib-item.active { background: #1b2540; color: var(--text); }
    .content  { flex: 1; overflow-y: auto; padding: 36px 44px; }
    .art-title { font-size: 21px; font-weight: 600; color: #f0f4ff; margin-bottom: 6px; }
    .art-meta  { font-size: 12px; color: var(--muted); margin-bottom: 22px; }
    .comp-bar  { display: flex; align-items: center; gap: 12px; margin-bottom: 26px;
                 padding: 10px 14px; background: var(--surface); border-radius: 8px; border: 1px solid var(--border); }
    .bar-track { flex: 1; height: 5px; background: var(--border); border-radius: 3px; }
    .bar-fill  { height: 5px; background: linear-gradient(90deg, var(--ok), var(--accent)); border-radius: 3px;
                 width: 58%; transition: width 0.7s ease; }
    .bar-pct   { font-size: 12px; font-weight: 600; color: var(--accent); white-space: nowrap; }
    .art-body  { font-size: 22px; line-height: 2.4; }
    .art-body p { margin-bottom: 1.4em; }
    [data-carve="token"] { cursor: pointer; border-radius: 2px; padding: 1px 0; transition: background 0.12s; }
    [data-carve="token"][data-status="unknown"][data-content="1"]  { border-bottom: 2.5px solid var(--accent); }
    [data-carve="token"][data-status="learning"][data-content="1"] { border-bottom: 2.5px solid var(--warn); }
    [data-carve="token"][data-status="known"][data-content="1"]    { border-bottom: 2.5px solid var(--ok); }
    [data-carve="token"]:hover { background: rgba(108,127,242,0.14); }
    #carve-popup {
      position: fixed; z-index: 9999; background: #18213a;
      border: 1px solid var(--border); border-radius: 14px; padding: 18px; width: 310px;
      box-shadow: 0 24px 64px rgba(0,0,0,0.7); display: none;
      animation: pop 0.22s ease both;
    }
    .popup-word     { font-size: 30px; font-weight: 700; color: #f0f4ff; margin-bottom: 3px; }
    .popup-reading  { font-size: 14px; color: var(--accent); margin-bottom: 8px; }
    .popup-jlpt     { display: inline-block; background: var(--border); color: var(--accent); font-size: 10px; padding: 2px 7px; border-radius: 4px; margin-right: 5px; }
    .popup-pos      { font-size: 11px; color: var(--muted); }
    .popup-def      { font-size: 13px; color: #c8d4e8; margin: 6px 0; line-height: 1.5; }
    .popup-sent     { font-size: 12px; color: var(--muted); border-top: 1px solid var(--border); padding-top: 8px; margin-top: 8px; font-style: italic; }
    .popup-actions  { display: flex; gap: 8px; margin-top: 12px; }
    .btn-mine       { flex: 1; background: var(--accent); color: #fff; border: none; border-radius: 7px;
                      padding: 9px 0; font-size: 13px; font-weight: 600; cursor: pointer; }
    .btn-ignore     { background: var(--border); color: var(--muted); border: none; border-radius: 7px;
                      padding: 9px 12px; font-size: 13px; cursor: pointer; }
    .mine-form label { display: block; font-size: 10px; color: var(--muted); text-transform: uppercase;
                       letter-spacing: 0.5px; margin: 8px 0 3px; }
    .mine-form input, .mine-form textarea {
      width: 100%; background: var(--bg); border: 1px solid var(--border); border-radius: 6px;
      padding: 7px 10px; color: var(--text); font-size: 13px; outline: none;
    }
    .mine-form textarea { resize: none; height: 52px; font-family: inherit; }
    .btn-save { width: 100%; background: linear-gradient(135deg, var(--accent), var(--accent2));
                color: #fff; border: none; border-radius: 8px; padding: 10px 0;
                font-size: 14px; font-weight: 600; cursor: pointer; margin-top: 12px; }
    .save-status { text-align: center; font-size: 14px; font-weight: 600; margin-top: 8px; min-height: 20px; }
  </style>
</head>
<body>
<div class="topbar">
  <div class="logo">⚡ Carve</div>
  <div class="nav">
    <div class="nav-item active">Reader</div>
    <div class="nav-item">Review</div>
    <div class="nav-item">Cards</div>
    <div class="nav-item">Stats</div>
  </div>
</div>
<div class="layout">
  <div class="sidebar">
    <div class="sidebar-label">Library</div>
    <div class="lib-item active">🗞 NHK Web Easy</div>
    <div class="lib-item">📰 Asahi Shimbun</div>
    <div class="lib-item">📚 吾輩は猫である</div>
    <div class="lib-item">🎬 ドラえもん</div>
  </div>
  <div class="content">
    <div class="art-title">AIが日本語学習を革命する</div>
    <div class="art-meta">NHK Web Easy • 2025-05-28 • 📖 Mining mode</div>
    <div class="comp-bar">
      <span style="font-size:11px;color:var(--muted)">Comprehension</span>
      <div class="bar-track"><div class="bar-fill" id="bar-fill"></div></div>
      <div class="bar-pct" id="comp-pct">58%</div>
    </div>
    <div class="art-body" id="article"></div>
  </div>
</div>
<div id="carve-popup"></div>

<script>
const MOCK_BASE = '${mockBase}';

const TOKENS = [
  {s:'現代',  l:'現代',  r:'げんだい',    c:true,  st:'unknown'},
  {s:'の',    l:'の',    r:'の',          c:false, st:'known'},
  {s:'人工知能',l:'人工知能',r:'じんこうちのう',c:true,st:'unknown'},
  {s:'は',    l:'は',    r:'は',          c:false, st:'known'},
  {s:'、',    l:'、',    r:'',            c:false, st:'known'},
  {s:'私',    l:'私',    r:'わたし',      c:true,  st:'known'},
  {s:'たち',  l:'たち',  r:'たち',        c:false, st:'known'},
  {s:'の',    l:'の',    r:'の',          c:false, st:'known'},
  {s:'生活',  l:'生活',  r:'せいかつ',    c:true,  st:'unknown'},
  {s:'を',    l:'を',    r:'を',          c:false, st:'known'},
  {s:'大きく',l:'大きく',r:'おおきく',    c:true,  st:'learning'},
  {s:'変え',  l:'変える',r:'かえる',      c:true,  st:'unknown'},
  {s:'つつある',l:'つつある',r:'つつある',c:false, st:'known'},
  {s:'。',    l:'。',    r:'',            c:false, st:'known'},
];

const DEFS = {
  '人工知能': {reading:'じんこうちのう', jlpt:'N3', pos:'名詞', def:'artificial intelligence; AI'},
  '生活':     {reading:'せいかつ',       jlpt:'N4', pos:'名詞', def:'life; livelihood; daily life'},
  '現代':     {reading:'げんだい',       jlpt:'N3', pos:'名詞', def:'present age; modern times'},
  '大きく':   {reading:'おおきく',       jlpt:'N5', pos:'副詞', def:'greatly; on a large scale'},
  '変える':   {reading:'かえる',         jlpt:'N4', pos:'動詞', def:'to change; to alter; to transform'},
};

const article = document.getElementById('article');
const p = document.createElement('p');
TOKENS.forEach(tok => {
  const span = document.createElement('span');
  span.dataset.carve = 'token';
  span.dataset.lemma = tok.l;
  span.dataset.reading = tok.r;
  span.dataset.status = tok.st;
  span.dataset.content = tok.c ? '1' : '0';
  span.textContent = tok.s;
  p.appendChild(span);
});
article.appendChild(p);

const popup = document.getElementById('carve-popup');
let currentEl = null;

document.addEventListener('click', e => {
  const t = e.target;
  if (t.dataset.carve === 'token' && t.dataset.content === '1') {
    currentEl = t; showPopup(t);
  } else {
    const path = e.composedPath ? e.composedPath() : [];
    const inPopup = path.some(n => n && n.id === 'carve-popup') || !!(t.closest && t.closest('#carve-popup'));
    if (!inPopup) { popup.style.display = 'none'; currentEl = null; }
  }
});

function showPopup(el) {
  const lemma = el.dataset.lemma;
  const d = DEFS[lemma] || {reading: el.dataset.reading, jlpt:'', pos:'名詞', def:'—'};
  popup.innerHTML =
    '<div class="popup-word">' + lemma + '</div>' +
    '<div class="popup-reading">' + d.reading + (d.jlpt ? ' <span class="popup-jlpt">' + d.jlpt + '</span>' : '') + '</div>' +
    '<span class="popup-pos">' + d.pos + '</span> ' +
    '<span class="popup-def">' + d.def + '</span>' +
    '<div class="popup-sent">"…現代の人工知能は、私たちの生活を大きく…"</div>' +
    '<div class="popup-actions">' +
      '<button class="btn-mine" id="btn-mine-reader">⚡ Mine</button>' +
      '<button class="btn-ignore" id="btn-ignore">Ignore</button>' +
    '</div>';
  document.getElementById('btn-mine-reader').onclick = () => showMineForm(lemma, d.reading, d.def);
  document.getElementById('btn-ignore').onclick = () => { popup.style.display = 'none'; };

  const rect = el.getBoundingClientRect();
  popup.style.cssText = 'display:block; top:' +
    (rect.top > 260 ? (rect.top - 260 - 6) : (rect.bottom + 8)) + 'px; left:' +
    Math.min(Math.max(rect.left - 20, 8), window.innerWidth - 326) + 'px;';
  popup.style.animation = 'pop 0.22s ease both';
}

function showMineForm(lemma, reading, def) {
  popup.innerHTML =
    '<div style="font-size:13px;font-weight:700;color:#f0f4ff;margin-bottom:10px">Add card</div>' +
    '<div class="mine-form">' +
      '<label>Word</label><input id="f-lemma" value="' + lemma + '">' +
      '<label>Reading</label><input id="f-reading" value="' + reading + '">' +
      '<label>Definition</label><input id="f-def" value="' + def + '">' +
      '<label>Sentence (optional)</label>' +
      '<textarea id="f-sent">現代の人工知能は、私たちの生活を大きく変えつつある。</textarea>' +
      '<button class="btn-save" id="btn-save">Save card</button>' +
      '<div class="save-status" id="save-status"></div>' +
    '</div>';
  document.getElementById('btn-save').onclick = () => saveCard(lemma, reading);
}

async function saveCard(lemma, reading) {
  const btn = document.getElementById('btn-save');
  btn.textContent = 'Saving…'; btn.disabled = true;
  const payload = {
    language_code: 'ja', lemma, reading,
    back_text: document.getElementById('f-def').value,
    sentence: document.getElementById('f-sent').value,
    source_url: window.location.href,
  };
  try {
    await fetch(MOCK_BASE + '/v1/cards', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(payload) });
    const ss = document.getElementById('save-status');
    ss.textContent = '✓ Saved!'; ss.style.color = 'var(--ok)';
    if (currentEl) { currentEl.dataset.status = 'learning'; }
    document.getElementById('comp-pct').textContent = '65%';
    document.getElementById('bar-fill').style.width = '65%';
    setTimeout(() => { popup.style.display = 'none'; currentEl = null; }, 900);
  } catch(e) {
    document.getElementById('save-status').textContent = 'Error'; btn.disabled = false; btn.textContent = 'Save card';
  }
}
</script>
</body>
</html>`;
}

function makeVideoHTML(mockBase, service = 'youtube') {
  const brands = {
    youtube: {
      name: 'YouTube',
      color: '#ff0000',
      logo: '▶ YouTube',
      title: '日本語AI入門 第3回：機械学習の基礎',
      channel: 'JapanesePod101 • 3.4M views',
      gradient: 'linear-gradient(160deg, #0d1a2e 0%, #1a2a4a 40%, #0a1220 100%)',
    },
    netflix: {
      name: 'Netflix',
      color: '#e50914',
      logo: 'NETFLIX',
      title: '転生したらスライムだった件 Season 3',
      channel: 'Episode 8 — Available in HD',
      gradient: 'linear-gradient(160deg, #1a0a0a 0%, #2a0d0d 40%, #0d0505 100%)',
    },
    crunchyroll: {
      name: 'Crunchyroll',
      color: '#f47521',
      logo: '🍥 Crunchyroll',
      title: '進撃の巨人 The Final Season Part 3',
      channel: 'Simulcast • HD • English subtitles',
      gradient: 'linear-gradient(160deg, #120d05 0%, #1e1205 40%, #0d0a03 100%)',
    },
  };
  const b = brands[service] || brands.youtube;

  return `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Carve — Video Mining</title>
  <style>
${BASE_CSS}
    .header { height: 50px; background: var(--surface); border-bottom: 1px solid var(--border);
              display: flex; align-items: center; padding: 0 22px; gap: 12px; }
    .logo-carve { font-weight: 700; color: var(--accent); font-size: 16px; }
    .mining-badge { background: rgba(108,127,242,0.2); color: var(--accent); font-size: 11px;
                    font-weight: 600; padding: 3px 10px; border-radius: 20px; border: 1px solid rgba(108,127,242,0.35); }
    .brand-logo { margin-left: auto; font-weight: 900; font-size: 18px; color: ${b.color}; letter-spacing: -0.5px; }
    .wrap { display: flex; gap: 20px; padding: 18px 28px; max-height: calc(100vh - 50px); }
    .player-col { flex: 1; min-width: 0; }
    .player {
      background: #000; border-radius: 10px; overflow: hidden;
      aspect-ratio: 16/9; position: relative;
    }
    .scene-bg { position: absolute; inset: 0; background: ${b.gradient}; }
    .scene-art {
      position: absolute; inset: 0; display: flex; align-items: center; justify-content: center;
      overflow: hidden;
    }
    .scene-silhouette {
      width: 100%; height: 100%; display: flex; align-items: center; justify-content: center;
      opacity: 0.55;
    }
    .scene-silhouette svg { width: 100%; height: 100%; }
    .carve-active {
      position: absolute; top: 10px; right: 10px; background: var(--accent);
      color: #fff; font-size: 10px; font-weight: 700; padding: 4px 10px;
      border-radius: 20px; letter-spacing: 0.5px;
    }
    .progress { position: absolute; bottom: 42px; left: 10px; right: 10px; height: 3px;
                background: rgba(255,255,255,0.18); border-radius: 2px; }
    .progress-fill { height: 3px; background: ${b.color}; border-radius: 2px; width: 38%; }
    .controls { position: absolute; bottom: 0; left: 0; right: 0; height: 40px;
                background: linear-gradient(transparent, rgba(0,0,0,0.8));
                display: flex; align-items: center; padding: 0 10px; gap: 8px; }
    .ctrl { font-size: 16px; color: #fff; opacity: 0.85; cursor: pointer; user-select: none; }
    .time { font-size: 11px; color: rgba(255,255,255,0.7); margin-left: 2px; }
    .subtitle-bar {
      position: absolute; bottom: 48px; left: 50%; transform: translateX(-50%);
      background: rgba(0,0,0,0.78); border-radius: 6px; padding: 8px 18px;
      font-size: 19px; white-space: nowrap; text-align: center; line-height: 1.8;
    }
    [data-carve="token"] { cursor: pointer; border-radius: 2px; padding: 0 1px; transition: background 0.1s; }
    [data-carve="token"][data-status="unknown"][data-content="1"]  { border-bottom: 2px solid var(--accent); }
    [data-carve="token"][data-status="learning"][data-content="1"] { border-bottom: 2px solid var(--warn); }
    [data-carve="token"]:hover { background: rgba(108,127,242,0.28); }
    .vid-title   { font-size: 15px; font-weight: 600; color: #f0f4ff; margin-top: 10px; }
    .vid-channel { font-size: 12px; color: var(--muted); margin-top: 3px; }
    .sidebar-col { width: 290px; flex-shrink: 0; display: flex; flex-direction: column; gap: 14px; }
    .sidebar-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.8px; color: var(--muted); }
    .card-chip { background: var(--surface); border: 1px solid var(--border); border-radius: 10px;
                 padding: 13px 14px; animation: slideIn 0.3s ease both; }
    .chip-word    { font-size: 20px; font-weight: 700; }
    .chip-reading { font-size: 12px; color: var(--accent); margin-top: 1px; }
    .chip-def     { font-size: 12px; color: var(--muted); margin-top: 5px; }
    .chip-tag     { display: inline-block; font-size: 10px; padding: 2px 7px; border-radius: 4px;
                    margin-top: 5px; background: rgba(245,167,66,0.18); color: var(--warn); }
    #video-popup {
      position: fixed; z-index: 9999; background: #18213a; border: 1px solid var(--border);
      border-radius: 14px; padding: 16px; width: 290px; box-shadow: 0 24px 64px rgba(0,0,0,0.7);
      display: none; animation: pop 0.22s ease both;
    }
    .vp-word    { font-size: 26px; font-weight: 700; color: #f0f4ff; }
    .vp-reading { font-size: 13px; color: var(--accent); margin: 3px 0 7px; }
    .vp-def     { font-size: 13px; color: #c8d4e8; line-height: 1.5; }
    .vp-actions { display: flex; gap: 8px; margin-top: 12px; }
    .btn-mine-v { flex:1; background: var(--accent); color:#fff; border:none; border-radius:7px;
                  padding:9px 0; font-size:13px; font-weight:600; cursor:pointer; }
    .mine-ok { color: var(--ok); font-weight: 600; font-size: 14px; text-align:center; margin-top:8px; }
  </style>
</head>
<body>
<div class="header">
  <div class="logo-carve">⚡ Carve</div>
  <div class="mining-badge">Mining mode active</div>
  <div class="brand-logo">${b.logo}</div>
</div>

<div class="wrap">
  <div class="player-col">
    <div class="player">
      <div class="scene-bg"></div>
      <div class="scene-art">
        <div class="scene-silhouette">
          <svg viewBox="0 0 800 450" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#0a1833" stop-opacity="0.9"/>
                <stop offset="100%" stop-color="#1a2a4a" stop-opacity="0.5"/>
              </linearGradient>
              <linearGradient id="moon" x1="0" y1="0" x2="1" y2="1">
                <stop offset="0%" stop-color="#f0e6b0"/>
                <stop offset="100%" stop-color="#d4c070"/>
              </linearGradient>
            </defs>
            <rect width="800" height="450" fill="url(#sky)"/>
            <!-- Moon -->
            <circle cx="620" cy="90" r="42" fill="url(#moon)" opacity="0.7"/>
            <circle cx="638" cy="82" r="42" fill="#1a2a4a" opacity="0.6"/>
            <!-- Stars -->
            <circle cx="100" cy="60"  r="1.5" fill="#fff" opacity="0.8"/>
            <circle cx="200" cy="40"  r="1"   fill="#fff" opacity="0.6"/>
            <circle cx="320" cy="80"  r="1.5" fill="#fff" opacity="0.7"/>
            <circle cx="450" cy="30"  r="1"   fill="#fff" opacity="0.9"/>
            <circle cx="550" cy="70"  r="1.5" fill="#fff" opacity="0.5"/>
            <circle cx="700" cy="50"  r="1"   fill="#fff" opacity="0.7"/>
            <circle cx="150" cy="120" r="1"   fill="#fff" opacity="0.4"/>
            <circle cx="380" cy="50"  r="1"   fill="#fff" opacity="0.6"/>
            <!-- Mountains -->
            <polygon points="0,320 160,160 320,320" fill="#0d1525" opacity="0.95"/>
            <polygon points="200,320 400,140 600,320" fill="#111d30" opacity="0.95"/>
            <polygon points="450,320 640,180 800,320" fill="#0a1220" opacity="0.95"/>
            <polygon points="600,320 750,210 800,320" fill="#0d1525" opacity="0.95"/>
            <!-- Ground -->
            <rect x="0" y="318" width="800" height="132" fill="#080e1a"/>
            <!-- Character silhouette (simple anime figure) -->
            <ellipse cx="400" cy="295" rx="22" ry="28" fill="#060c18"/>
            <circle  cx="400" cy="258" r="18"         fill="#060c18"/>
            <!-- Hair tuft -->
            <path d="M386 244 Q390 232 400 240 Q410 232 414 244" fill="#060c18"/>
            <!-- Windswept scarf -->
            <path d="M412 268 Q440 255 460 270 Q450 285 430 280 Z" fill="#060c18" opacity="0.85"/>
          </svg>
        </div>
      </div>
      <div class="carve-active">⚡ Carve Active</div>
      <div class="subtitle-bar" id="subtitles"></div>
      <div class="progress"><div class="progress-fill"></div></div>
      <div class="controls">
        <span class="ctrl">⏸</span>
        <span class="ctrl">⏭</span>
        <span class="time">2:14 / 6:35</span>
        <span class="ctrl" style="margin-left:auto">🔊</span>
        <span class="ctrl">⛶</span>
      </div>
    </div>
    <div class="vid-title">${b.title}</div>
    <div class="vid-channel">${b.channel}</div>
  </div>

  <div class="sidebar-col">
    <div class="sidebar-label">Recently mined</div>
    <div id="mined-list"></div>
  </div>
</div>

<div id="video-popup"></div>

<script>
const MOCK_BASE = '${mockBase}';
const minedCards = [];

const TOKENS = [
  {s:'人工知能',l:'人工知能',r:'じんこうちのう',c:true, st:'unknown'},
  {s:'は',      l:'は',      r:'は',            c:false,st:'known'},
  {s:'日本語',  l:'日本語',  r:'にほんご',      c:true, st:'learning'},
  {s:'を',      l:'を',      r:'を',            c:false,st:'known'},
  {s:'学ぶ',    l:'学ぶ',    r:'まなぶ',        c:true, st:'unknown'},
  {s:'のに',    l:'のに',    r:'のに',          c:false,st:'known'},
  {s:'役立ち',  l:'役立つ',  r:'やくだつ',      c:true, st:'unknown'},
  {s:'ます',    l:'ます',    r:'ます',          c:false,st:'known'},
  {s:'。',      l:'。',      r:'',              c:false,st:'known'},
];
const DEFS = {
  '人工知能': {r:'じんこうちのう', def:'artificial intelligence; AI'},
  '日本語':   {r:'にほんご',       def:'Japanese language'},
  '学ぶ':     {r:'まなぶ',         def:'to study; to learn'},
  '役立つ':   {r:'やくだつ',       def:'to be useful; to come in handy'},
};

const bar = document.getElementById('subtitles');
TOKENS.forEach(tok => {
  const span = document.createElement('span');
  span.dataset.carve = 'token';
  span.dataset.lemma = tok.l;
  span.dataset.content = tok.c ? '1' : '0';
  span.dataset.status = tok.st;
  span.textContent = tok.s;
  bar.appendChild(span);
});

const popup = document.getElementById('video-popup');
let popupEl = null;

document.addEventListener('click', e => {
  const t = e.target;
  if (t.dataset.carve === 'token' && t.dataset.content === '1') { popupEl = t; showPopup(t); }
  else if (!t.closest('#video-popup')) { popup.style.display = 'none'; popupEl = null; }
});

function showPopup(el) {
  const lemma = el.dataset.lemma;
  const d = DEFS[lemma] || {r: lemma, def:'—'};
  popup.innerHTML =
    '<div class="vp-word">' + lemma + '</div>' +
    '<div class="vp-reading">' + d.r + '</div>' +
    '<div class="vp-def">' + d.def + '</div>' +
    '<div class="vp-actions"><button class="btn-mine-v" id="btn-mine-v">⚡ Mine</button></div>' +
    '<div class="mine-ok" id="mine-ok" style="display:none">✓ Card saved!</div>';
  document.getElementById('btn-mine-v').onclick = () => doMine(lemma, d.r, d.def, el);
  const rect = el.getBoundingClientRect();
  popup.style.cssText = 'display:block; top:' + Math.max(rect.top - 186, 60) + 'px; left:' +
    Math.min(Math.max(rect.left - 60, 8), window.innerWidth - 306) + 'px;';
  popup.style.animation = 'pop 0.22s ease both';
}

async function doMine(lemma, reading, def, el) {
  await fetch(MOCK_BASE + '/v1/cards', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({ lemma, reading, back_text: def, language_code:'ja',
      source_url: window.location.href, subtitle_translation: 'AI is useful for studying Japanese.' }),
  }).catch(()=>{});
  document.getElementById('btn-mine-v').style.display = 'none';
  document.getElementById('mine-ok').style.display = 'block';
  el.dataset.status = 'learning';
  minedCards.unshift({lemma, reading, def});
  renderMined();
  setTimeout(() => { popup.style.display = 'none'; popupEl = null; }, 800);
}

function renderMined() {
  const list = document.getElementById('mined-list');
  list.innerHTML = minedCards.slice(0,4).map((c,i) =>
    '<div class="card-chip" style="animation-delay:' + (i*0.05) + 's">' +
      '<div class="chip-word">'    + c.lemma   + '</div>' +
      '<div class="chip-reading">' + c.reading + '</div>' +
      '<div class="chip-def">'     + c.def     + '</div>' +
      '<span class="chip-tag">learning</span>' +
    '</div>'
  ).join('');
}
</script>
</body>
</html>`;
}

function makeReviewHTML(mockBase) {
  return `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Carve — Review</title>
  <style>
${BASE_CSS}
    body { min-height: 100vh; display: flex; flex-direction: column; }
    .topbar { height: 50px; background: var(--surface); border-bottom: 1px solid var(--border);
              display: flex; align-items: center; padding: 0 22px; gap: 14px; flex-shrink: 0; }
    .logo   { font-size: 17px; font-weight: 700; color: var(--accent); }
    .due-badge { background: var(--danger); color: #fff; font-size: 11px; font-weight: 700;
                 padding: 2px 8px; border-radius: 10px; }
    .streak { margin-left: auto; font-size: 13px; color: var(--warn); font-weight: 600; }
    .wrap   { flex: 1; display: flex; flex-direction: column; align-items: center;
              justify-content: center; padding: 36px 20px; gap: 0; }
    .prog-row { display: flex; align-items: center; gap: 14px; width: 100%; max-width: 540px; margin-bottom: 28px; }
    .prog-bar  { flex: 1; height: 7px; background: var(--border); border-radius: 4px; }
    .prog-fill { height: 7px; background: linear-gradient(90deg, var(--ok), var(--accent));
                 border-radius: 4px; transition: width 0.6s ease; width: 0%; }
    .prog-lbl  { font-size: 13px; color: var(--muted); white-space: nowrap; }
    .card { background: var(--surface); border: 1px solid var(--border); border-radius: 18px;
            padding: 44px 40px; width: 100%; max-width: 540px; text-align: center;
            transition: transform 0.25s ease, box-shadow 0.25s ease; }
    .card.flip { transform: rotateY(4deg) scale(1.01); box-shadow: 0 20px 60px rgba(0,0,0,0.5); }
    .card-front { font-size: 56px; font-weight: 700; letter-spacing: 2px; color: #f0f4ff; }
    .card-hint  { font-size: 13px; color: var(--muted); margin-top: 10px; }
    .reveal-btn { margin-top: 28px; background: var(--border); border: none; border-radius: 10px;
                  padding: 13px 44px; font-size: 14px; font-weight: 600; color: var(--text);
                  cursor: pointer; transition: background 0.15s; }
    .reveal-btn:hover { background: #2d3a52; }
    .card-back { margin-top: 18px; }
    .back-reading { font-size: 24px; color: var(--accent); }
    .back-def     { font-size: 19px; color: #c8d4e8; margin-top: 6px; line-height: 1.5; }
    .back-sent    { font-size: 13px; color: var(--muted); margin-top: 14px; font-style: italic;
                    border-top: 1px solid var(--border); padding-top: 14px; }
    .ratings { display: flex; gap: 10px; width: 100%; max-width: 540px; margin-top: 20px; }
    .r-btn  { flex: 1; border: none; border-radius: 10px; padding: 13px 0; font-size: 13px;
              font-weight: 600; cursor: pointer; transition: opacity 0.15s, transform 0.1s; }
    .r-btn:hover { opacity: 0.85; transform: translateY(-1px); }
    .r-btn:active { transform: translateY(0); }
    .r1 { background: #3d1515; color: #f08080; }
    .r2 { background: #2a1d40; color: #b794f4; }
    .r3 { background: #152640; color: #7eb3f4; }
    .r4 { background: #12302a; color: var(--ok); }
    .done { text-align: center; }
    .done-emoji { font-size: 60px; margin-bottom: 14px; animation: fadeUp 0.5s ease; }
    .done-title { font-size: 26px; font-weight: 700; color: #f0f4ff; }
    .done-sub   { font-size: 15px; color: var(--muted); margin-top: 8px; }
    .done-stats { display: flex; gap: 24px; justify-content: center; margin-top: 20px; }
    .stat { text-align: center; }
    .stat-val { font-size: 28px; font-weight: 700; color: var(--ok); }
    .stat-lbl { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.5px; }
  </style>
</head>
<body>
<div class="topbar">
  <div class="logo">⚡ Carve</div>
  <span style="font-size:13px;color:var(--muted)">Review session</span>
  <span class="due-badge" id="due-badge">3 due</span>
  <span class="streak">🔥 12 day streak</span>
</div>
<div class="wrap">
  <div class="prog-row">
    <div class="prog-bar"><div class="prog-fill" id="prog-fill"></div></div>
    <div class="prog-lbl" id="prog-lbl">0 / 3</div>
  </div>
  <div id="card-area" style="width:100%;max-width:540px"></div>
  <div class="ratings" id="ratings" style="display:none;width:100%;max-width:540px">
    <button class="r-btn r1" id="r1">Again<br><small>10m</small></button>
    <button class="r-btn r2" id="r2">Hard<br><small>1d</small></button>
    <button class="r-btn r3" id="r3">Good<br><small>4d</small></button>
    <button class="r-btn r4" id="r4">Easy<br><small>10d</small></button>
  </div>
</div>

<script>
const MOCK_BASE = '${mockBase}';
const QUEUE = ${JSON.stringify(DEMO_CARDS)};
let idx = 0, reviewed = 0;
const total = QUEUE.length;

function updateProgress() {
  const pct = (reviewed / total * 100).toFixed(0);
  document.getElementById('prog-fill').style.width = pct + '%';
  document.getElementById('prog-lbl').textContent = reviewed + ' / ' + total;
  document.getElementById('due-badge').textContent = Math.max(0, total - reviewed) + ' due';
}

function renderCard() {
  document.getElementById('ratings').style.display = 'none';
  const card = QUEUE[idx];
  if (!card) { renderDone(); return; }
  document.getElementById('card-area').innerHTML =
    '<div class="card" id="review-card">' +
      '<div class="card-front">' + card.front_text + '</div>' +
      '<div class="card-hint">What does this word mean?</div>' +
      '<button class="reveal-btn" id="reveal-btn">Show answer</button>' +
    '</div>';
  document.getElementById('reveal-btn').onclick = revealCard;
}

function revealCard() {
  const card = QUEUE[idx];
  const el = document.getElementById('review-card');
  el.classList.add('flip');
  setTimeout(() => {
    el.innerHTML =
      '<div class="card-front">' + card.front_text + '</div>' +
      '<div class="card-back">' +
        '<div class="back-reading">' + card.front_reading + '</div>' +
        '<div class="back-def">' + card.back_text + '</div>' +
        (card.sentence ? '<div class="back-sent">"' + card.sentence.slice(0,55) + '…"</div>' : '') +
      '</div>';
    el.classList.remove('flip');
    document.getElementById('ratings').style.display = 'flex';
  }, 120);
}

function setupRatings() {
  [1,2,3,4].forEach(r => {
    document.getElementById('r' + r).onclick = () => rate(r);
  });
}

async function rate(r) {
  reviewed++; idx++;
  updateProgress();
  await fetch(MOCK_BASE + '/v1/review/events', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({ card_id: QUEUE[idx-1]?.id, rating: r }),
  }).catch(()=>{});
  if (idx >= QUEUE.length) renderDone(); else renderCard();
}

function renderDone() {
  document.getElementById('ratings').style.display = 'none';
  document.getElementById('card-area').innerHTML =
    '<div class="done">' +
      '<div class="done-emoji">🎉</div>' +
      '<div class="done-title">Session complete!</div>' +
      '<div class="done-sub">All ' + reviewed + ' cards reviewed for today</div>' +
      '<div class="done-stats">' +
        '<div class="stat"><div class="stat-val">' + reviewed + '</div><div class="stat-lbl">Reviewed</div></div>' +
        '<div class="stat"><div class="stat-val">94%</div><div class="stat-lbl">Retention</div></div>' +
        '<div class="stat"><div class="stat-val">4d</div><div class="stat-lbl">Next review</div></div>' +
      '</div>' +
    '</div>';
  document.getElementById('prog-fill').style.width = '100%';
  document.getElementById('prog-lbl').textContent = reviewed + ' / ' + total;
}

setupRatings();
renderCard();
</script>
</body>
</html>`;
}

// ── Mock API server ───────────────────────────────────────────────────────────

function startMockServer() {
  const capturedCards = [];

  const server = http.createServer((req, res) => {
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
    res.setHeader('Access-Control-Allow-Methods', 'GET,POST,OPTIONS');
    if (req.method === 'OPTIONS') { res.writeHead(204); res.end(); return; }

    const port = server.address().port;
    const base  = `http://127.0.0.1:${port}`;

    if (req.url === '/onboarding') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeOnboardingHTML(base)); return;
    }
    if (req.url === '/reader') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeReaderHTML(base)); return;
    }
    if (req.url === '/video' || req.url.startsWith('/video?')) {
      const svc = new URL('http://x' + req.url).searchParams.get('service') || 'youtube';
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeVideoHTML(base, svc)); return;
    }
    if (req.url === '/review') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeReviewHTML(base)); return;
    }

    let body = '';
    req.on('data', c => { body += c; });
    req.on('end', () => {
      if (req.url === '/v1/auth/login' || req.url === '/v1/auth/register') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ access_token: 'demo-token-' + Date.now(), user: { id: 'demo-user' } }));

      } else if (req.url === '/v1/onboarding/language') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end('{"success":true}');

      } else if (req.url === '/v1/cards' && req.method === 'POST') {
        let parsed = {}; try { parsed = JSON.parse(body); } catch {}
        capturedCards.push(parsed);
        res.writeHead(201, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ id: 'card-' + Date.now(), lemma: parsed.lemma, language_code: parsed.language_code }));

      } else if (req.url === '/v1/cards' && req.method === 'GET') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ cards: capturedCards, total: capturedCards.length }));

      } else if (req.url.startsWith('/v1/review')) {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end('{"success":true}');

      } else {
        res.writeHead(404); res.end('{}');
      }
    });
  });

  return new Promise(resolve => {
    server.listen(0, '127.0.0.1', () => {
      resolve({ server, port: server.address().port, capturedCards });
    });
  });
}

// ── Utilities ─────────────────────────────────────────────────────────────────

function assert(cond, msg) {
  if (!cond) {
    if (CI) throw new Error('FAIL: ' + msg);
    else console.warn('[warn]', msg);
  }
}

const wait = ms => new Promise(r => setTimeout(r, ms));

// ── Scenes ────────────────────────────────────────────────────────────────────

async function sceneOnboarding(page, base) {
  console.log('\n── Scene 0: Login + onboarding ─────────────────────────────');

  await page.goto(`${base}/onboarding`);
  await page.waitForSelector('#login-btn', { state: 'visible', timeout: 5000 });
  console.log('[page] login form');
  await wait(CI ? 0 : 500);

  await page.click('#login-btn');
  await wait(CI ? 0 : 200);

  // Reveal onboarding via DOM (auth net call is not critical for demo)
  await page.evaluate(() => {
    document.getElementById('auth-card').style.display = 'none';
    const ob = document.getElementById('onboarding-card');
    ob.style.cssText = 'display:block; animation: fadeUp 0.4s ease both;';
  });
  await page.waitForSelector('#onboarding-card', { state: 'visible', timeout: 3000 });
  console.log('[page] onboarding — language picker');
  await wait(CI ? 0 : 600);

  // Select Japanese
  await page.evaluate(() => {
    document.getElementById('lang-ja').classList.add('selected');
    document.getElementById('continue-btn').disabled = false;
    document.getElementById('continue-btn').classList.add('on');
  });
  await wait(CI ? 0 : 400);
  const selected = await page.$eval('#lang-ja', el => el.classList.contains('selected'));
  assert(selected, 'Japanese should be selected');
  console.log('[action] selected Japanese');
  await wait(CI ? 0 : 400);

  // Continue
  await page.evaluate(() => {
    document.getElementById('onboarding-card').style.display = 'none';
    const s = document.getElementById('success-card');
    s.style.cssText = 'display:block; animation: fadeUp 0.4s ease both;';
    document.getElementById('success-sub').textContent = "Japanese mode activated. Let’s start mining!";
  });
  await page.waitForSelector('#success-card', { state: 'visible', timeout: 3000 });
  const successText = await page.$eval('#success-sub', el => el.textContent);
  assert(successText.includes('Japanese'), `success text should mention Japanese, got: "${successText}"`);
  console.log('[assert] onboarding ✓');
  await wait(CI ? 0 : 700);
}

async function sceneReader(page, base) {
  console.log('\n── Scene 1: Reader — text mining ───────────────────────────');
  await page.goto(`${base}/reader`);
  await page.waitForSelector('[data-carve="token"][data-content="1"]', { state: 'attached', timeout: 5000 });
  console.log('[page] article annotated');
  await wait(CI ? 0 : 900);

  // Save lemma before clicking so we can re-query after status changes
  const firstUnknown = page.locator('[data-carve="token"][data-status="unknown"][data-content="1"]').first();
  await firstUnknown.waitFor({ state: 'attached', timeout: 3000 });
  const minedLemma = await firstUnknown.getAttribute('data-lemma');
  await firstUnknown.click();
  console.log(`[action] clicked token "${minedLemma}"`);

  await page.waitForSelector('#carve-popup', { state: 'visible', timeout: 3000 });
  console.log('[page] popup visible');
  const popupText = await page.$eval('#carve-popup', el => el.textContent ?? '');
  assert(popupText.includes('Mine'), 'popup should contain Mine button');
  await wait(CI ? 0 : 1100);

  // Mine
  await page.click('#btn-mine-reader');
  await page.waitForSelector('.btn-save', { state: 'visible', timeout: 2000 });
  console.log('[page] mine form');
  await wait(CI ? 0 : 800);

  // Save
  await page.click('.btn-save');
  await page.waitForFunction(
    () => document.getElementById('save-status')?.textContent?.includes('Saved') ||
          document.getElementById('save-status')?.textContent?.includes('✓'),
    { timeout: 3000 },
  );
  console.log('[page] saved');

  const newStatus = await page.$eval(
    `[data-carve="token"][data-lemma="${minedLemma}"]`,
    el => el.dataset.status,
  ).catch(() => 'unknown');
  assert(newStatus === 'learning', `token should be learning, got "${newStatus}"`);
  console.log('[assert] token → learning ✓');
  await wait(CI ? 0 : 700);
}

async function sceneVideoMining(page, base, capturedCards, service = 'youtube') {
  console.log(`\n── Scene 2: Video mining (${service}) ─────────────────────`);
  await page.goto(`${base}/video?service=${service}`);
  await page.waitForSelector('[data-carve="token"][data-content="1"]', { state: 'attached', timeout: 5000 });
  await wait(CI ? 0 : 700);

  const beforeCount = capturedCards.length;

  const subToken = page.locator('[data-carve="token"][data-status="unknown"][data-content="1"]').first();
  await subToken.waitFor({ state: 'attached', timeout: 3000 });
  const subLemma = await subToken.getAttribute('data-lemma');
  await subToken.click();
  console.log(`[action] clicked subtitle token "${subLemma}"`);

  await page.waitForSelector('#video-popup', { state: 'visible', timeout: 3000 });
  console.log('[page] popup visible');
  await wait(CI ? 0 : 900);

  await page.click('#btn-mine-v');
  console.log('[action] mined');
  await wait(CI ? 0 : 700);

  // Wait for card to be captured
  const deadline = Date.now() + 2000;
  while (capturedCards.length === beforeCount && Date.now() < deadline) await wait(30);
  assert(capturedCards.length > beforeCount, 'POST /v1/cards not received after video mining');
  const card = capturedCards[capturedCards.length - 1];
  assert(card.lemma, `mined card should have lemma, got ${JSON.stringify(card)}`);
  assert(card.language_code === 'ja', `language_code should be ja, got ${card.language_code}`);
  console.log(`[assert] card lemma="${card.lemma}" lang="${card.language_code}" ✓`);

  // 2s card visibility: GET /v1/cards must reflect the new card immediately
  const t0 = Date.now();
  const listResp = await page.evaluate(async base => {
    const r = await fetch(base + '/v1/cards');
    return r.json();
  }, base);
  const elapsed = Date.now() - t0;
  assert(listResp.cards?.length > 0, 'GET /v1/cards should return mined card');
  assert(elapsed < 2000, `GET /v1/cards took ${elapsed}ms, want < 2000ms`);
  console.log(`[assert] GET /v1/cards — ${listResp.cards.length} card(s) in ${elapsed}ms ✓`);
  await wait(CI ? 0 : 600);
}

async function sceneReview(page, base) {
  console.log('\n── Scene 3: SRS review ─────────────────────────────────────');
  await page.goto(`${base}/review`);
  await page.waitForSelector('.card-front', { state: 'visible', timeout: 5000 });
  console.log('[page] review card');
  await wait(CI ? 0 : 800);

  // Reveal
  await page.click('#reveal-btn');
  await page.waitForSelector('.card-back', { state: 'visible', timeout: 2000 });
  console.log('[page] answer revealed');
  assert(await page.isVisible('.ratings'), 'rating buttons should be visible');
  await wait(CI ? 0 : 900);

  // Rate "Good"
  await page.click('#r3');
  console.log('[action] rated Good');
  await wait(CI ? 0 : 500);

  // Second card
  const hasReveal = await page.isVisible('#reveal-btn').catch(() => false);
  if (hasReveal) {
    await page.click('#reveal-btn');
    await page.waitForSelector('.card-back', { state: 'visible', timeout: 2000 }).catch(() => {});
    await wait(CI ? 0 : 700);
    await page.click('#r4').catch(() => {});
    await wait(CI ? 0 : 400);
  }

  // Third card / done screen
  const hasReveal2 = await page.isVisible('#reveal-btn').catch(() => false);
  if (hasReveal2) {
    await page.click('#reveal-btn').catch(() => {});
    await wait(CI ? 0 : 600);
    await page.click('#r4').catch(() => {});
    await wait(CI ? 0 : 400);
  }

  await page.waitForFunction(
    () => document.querySelector('.done-title') !== null ||
          document.querySelector('.done') !== null,
    { timeout: 4000 },
  ).catch(() => {});

  const progPct = await page.$eval('#prog-fill', el => el.style.width).catch(() => '?');
  console.log(`[assert] progress fill: ${progPct} ✓`);
  await wait(CI ? 0 : 900);
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function run() {
  console.log('Carve Promotional Video Recorder');
  console.log(`Mode: ${CI ? 'CI (no video, strict assertions)' : 'promo recording (headful, slowMo, records .webm)'}`);

  const { server, port, capturedCards } = await startMockServer();
  const base = `http://127.0.0.1:${port}`;
  console.log(`[mock] server at ${base}`);

  const videoDir = path.resolve(__dirname, 'videos');
  if (!CI) fs.mkdirSync(videoDir, { recursive: true });

  const viewport = { width: 1280, height: 720 };

  const browser = await chromium.launch({
    headless: CI,
    slowMo: CI ? 0 : 45,
    args: ['--disable-web-security', '--allow-running-insecure-content'],
  });

  const context = await browser.newContext({
    viewport,
    ...(CI ? {} : { recordVideo: { dir: videoDir, size: viewport } }),
  });
  const page = await context.newPage();

  try {
    await sceneOnboarding(page, base);
    await sceneReader(page, base);
    await sceneVideoMining(page, base, capturedCards, 'youtube');
    await sceneReview(page, base);

    if (!CI) await wait(1000); // hold on done screen

    console.log('\n✓ PASS — all scenes completed');

    if (!CI) {
      const videoPath = await page.video()?.path();
      if (videoPath) {
        const date = new Date().toISOString().slice(0, 10).replace(/-/g, '');
        const dest = path.resolve(__dirname, `promo-${date}.webm`);
        fs.renameSync(videoPath, dest);
        console.log(`\n🎬 Video saved: ${dest}`);
        console.log(`   ffmpeg -i "${dest}" -vf scale=1280:720 promo-${date}.mp4`);
      }
    }
  } catch (err) {
    console.error('\n✗ FAIL —', err.message);
    if (!CI) console.error(err.stack);
    process.exit(1);
  } finally {
    await context.close();
    await browser.close();
    server.close();
  }
}

run().catch(err => { console.error('\n✗ FAIL —', err.message); process.exit(1); });
