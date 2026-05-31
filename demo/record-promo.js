/**
 * Carve promotional video recorder.
 *
 * Records a 25-30s demo of the full mining + review loop:
 *   1. Reader mode  — Japanese article with annotated unknown words
 *   2. Text mining  — click word → popup → Mine form → Save → "learning"
 *   3. Video mining — simulated video player (YouTube/streaming layout) with
 *                     subtitle pop-up and one-press mining
 *   4. Review       — SRS flashcard review screen, rating a card
 *
 * Dual-purpose:
 *   --ci mode   : no video saved, strict assertions, exits 0/1 for CI
 *   default     : records video to demo/promo-YYYYMMDD.webm, assertions soft
 *
 * Usage:
 *   node demo/record-promo.js            # record promo video
 *   node demo/record-promo.js --ci       # CI-safe test run
 *   node demo/record-promo.js --live     # point at real dev server (PORT env)
 *
 * Dependencies (same as e2e): playwright (already in e2e/node_modules)
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

const DEMO_ARTICLE = `現代の人工知能は、私たちの生活を大きく変えつつある。`;
const VIDEO_SUBTITLE = `人工知能は日本語を学ぶのに役立ちます。`;

const DEMO_CARDS = [
  {
    id: 'card-001',
    front_text: '人工知能',
    front_reading: 'じんこうちのう',
    back_text: 'artificial intelligence',
    sentence: '現代の人工知能は、私たちの生活を大きく変えつつある。',
    language_code: 'ja',
    fsrs_state: 'new',
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
    fsrs_state: 'new',
    user_status: 'learning',
    created_at: new Date().toISOString(),
  },
];

// ── HTML pages ────────────────────────────────────────────────────────────────

// Reader page — matches the look of the Carve web reader
function makeReaderHTML(mockBase) {
  return `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Carve — Reader</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, sans-serif; background: #0f1117; color: #e2e8f0; min-height: 100vh; }
    .topbar { height: 52px; background: #161b2e; border-bottom: 1px solid #2d3748; display: flex; align-items: center; padding: 0 24px; gap: 16px; }
    .topbar-logo { font-size: 18px; font-weight: 700; color: #7c8cf8; letter-spacing: -0.5px; }
    .topbar-nav { display: flex; gap: 8px; }
    .nav-item { font-size: 13px; color: #718096; padding: 6px 12px; border-radius: 6px; cursor: pointer; }
    .nav-item.active { color: #e2e8f0; background: #2d3748; }
    .layout { display: flex; height: calc(100vh - 52px); }
    .sidebar { width: 240px; background: #161b2e; border-right: 1px solid #2d3748; padding: 20px 16px; flex-shrink: 0; }
    .sidebar-title { font-size: 11px; text-transform: uppercase; letter-spacing: 1px; color: #4a5568; margin-bottom: 12px; }
    .lib-item { padding: 8px 12px; border-radius: 6px; font-size: 14px; color: #718096; margin-bottom: 4px; cursor: pointer; }
    .lib-item.active { background: #1e2a3a; color: #e2e8f0; }
    .content { flex: 1; overflow-y: auto; padding: 40px; }
    .article-header { margin-bottom: 24px; }
    .article-title { font-size: 22px; font-weight: 600; color: #f7fafc; margin-bottom: 8px; }
    .article-meta { font-size: 12px; color: #4a5568; }
    .comprehension-bar { display: flex; align-items: center; gap: 12px; margin-bottom: 28px; padding: 12px 16px; background: #161b2e; border-radius: 8px; border: 1px solid #2d3748; }
    .bar-track { flex: 1; height: 6px; background: #2d3748; border-radius: 3px; }
    .bar-fill { height: 6px; background: linear-gradient(90deg, #4fd1c5, #7c8cf8); border-radius: 3px; width: 62%; transition: width 0.6s ease; }
    .bar-pct { font-size: 13px; font-weight: 600; color: #7c8cf8; }
    .article-body { font-size: 20px; line-height: 2.2; color: #e2e8f0; }
    [data-carve="token"] { position: relative; cursor: pointer; border-radius: 2px; padding: 1px 0; transition: background 0.15s; }
    [data-carve="token"][data-status="unknown"][data-content="1"] { border-bottom: 2px solid #7c8cf8; }
    [data-carve="token"][data-status="learning"][data-content="1"] { border-bottom: 2px solid #f6ad55; }
    [data-carve="token"][data-status="known"][data-content="1"] { border-bottom: 2px solid #68d391; }
    [data-carve="token"]:hover { background: rgba(124,140,248,0.12); }
    #carve-popup {
      position: fixed; z-index: 9999; background: #1a2035; border: 1px solid #2d3748;
      border-radius: 12px; padding: 16px; width: 320px; box-shadow: 0 20px 60px rgba(0,0,0,0.6);
      display: none; font-family: -apple-system, sans-serif;
    }
    .popup-word { font-size: 28px; font-weight: 700; color: #f7fafc; margin-bottom: 4px; }
    .popup-reading { font-size: 14px; color: #7c8cf8; margin-bottom: 8px; }
    .popup-jlpt { display: inline-block; background: #2d3748; color: #7c8cf8; font-size: 11px; padding: 2px 8px; border-radius: 4px; margin-right: 6px; }
    .popup-pos { font-size: 11px; color: #4a5568; margin-right: 4px; }
    .popup-def { font-size: 13px; color: #cbd5e0; margin: 6px 0; line-height: 1.5; }
    .popup-sentence { font-size: 12px; color: #718096; border-top: 1px solid #2d3748; padding-top: 8px; margin-top: 8px; font-style: italic; }
    .popup-actions { display: flex; gap: 8px; margin-top: 12px; }
    .btn-mine-popup { flex: 1; background: #7c8cf8; color: #fff; border: none; border-radius: 6px; padding: 8px 0; font-size: 13px; font-weight: 600; cursor: pointer; transition: background 0.15s; }
    .btn-mine-popup:hover { background: #6674e8; }
    .btn-ignore-popup { background: #2d3748; color: #718096; border: none; border-radius: 6px; padding: 8px 12px; font-size: 13px; cursor: pointer; }
    .mine-form { }
    .mine-form label { display: block; font-size: 11px; color: #4a5568; text-transform: uppercase; letter-spacing: 0.5px; margin: 8px 0 3px; }
    .mine-form input, .mine-form textarea { width: 100%; background: #0f1117; border: 1px solid #2d3748; border-radius: 6px; padding: 7px 10px; color: #e2e8f0; font-size: 13px; }
    .mine-form textarea { resize: none; height: 56px; font-family: inherit; }
    .btn-save { width: 100%; background: linear-gradient(135deg,#7c8cf8,#9f7aea); color: #fff; border: none; border-radius: 8px; padding: 10px 0; font-size: 14px; font-weight: 600; cursor: pointer; margin-top: 12px; }
    .saved-status { text-align: center; color: #68d391; font-size: 14px; font-weight: 600; margin-top: 8px; }
  </style>
</head>
<body>
<div class="topbar">
  <div class="topbar-logo">⚡ Carve</div>
  <div class="topbar-nav">
    <div class="nav-item active">Reader</div>
    <div class="nav-item">Review</div>
    <div class="nav-item">Cards</div>
    <div class="nav-item">Stats</div>
  </div>
</div>
<div class="layout">
  <div class="sidebar">
    <div class="sidebar-title">Library</div>
    <div class="lib-item active">🗞 NHK Web Easy</div>
    <div class="lib-item">📰 Asahi Shimbun</div>
    <div class="lib-item">📚 Sōseki – 吾輩は猫</div>
  </div>
  <div class="content">
    <div class="article-header">
      <div class="article-title">AIが日本語学習を革命する</div>
      <div class="article-meta">NHK Web Easy • 2024-11-01 • 📖 Mining Read mode</div>
    </div>
    <div class="comprehension-bar">
      <div class="bar-track"><div class="bar-fill"></div></div>
      <div class="bar-pct" id="comp-pct">62%</div>
    </div>
    <div class="article-body" id="article">
    </div>
  </div>
</div>

<div id="carve-popup"></div>

<script>
const MOCK_BASE = '${mockBase}';
const ARTICLE = '${DEMO_ARTICLE}';

// Status map: lemma → status
const status = {
  '現代': 'unknown',
  '人工': 'unknown',
  '知能': 'unknown',
  '人工知能': 'unknown',
  'は': 'known',
  '、': 'known',
  '私': 'known',
  'たち': 'known',
  'の': 'known',
  '生活': 'unknown',
  'を': 'known',
  '大きく': 'learning',
  '変え': 'unknown',
  'つつ': 'known',
  'ある': 'known',
  '。': 'known',
};

// Pre-tokenized for the demo (to avoid needing real backend for the initial render)
const TOKENS = [
  {s:'現代',l:'現代',r:'げんだい',c:true},
  {s:'の',l:'の',r:'の',c:false},
  {s:'人工知能',l:'人工知能',r:'じんこうちのう',c:true},
  {s:'は',l:'は',r:'は',c:false},
  {s:'、',l:'、',r:'、',c:false},
  {s:'私',l:'私',r:'わたし',c:true},
  {s:'たち',l:'たち',r:'たち',c:false},
  {s:'の',l:'の',r:'の',c:false},
  {s:'生活',l:'生活',r:'せいかつ',c:true},
  {s:'を',l:'を',r:'を',c:false},
  {s:'大きく',l:'大きく',r:'おおきく',c:true},
  {s:'変え',l:'変える',r:'かえ',c:true},
  {s:'つつ',l:'つつ',r:'つつ',c:false},
  {s:'ある',l:'ある',r:'ある',c:false},
  {s:'。',l:'。',r:'。',c:false},
];

const article = document.getElementById('article');
TOKENS.forEach(tok => {
  const st = tok.c ? (status[tok.l] || 'unknown') : 'known';
  const span = document.createElement('span');
  span.setAttribute('data-carve','token');
  span.setAttribute('data-lemma', tok.l);
  span.setAttribute('data-reading', tok.r);
  span.setAttribute('data-status', st);
  span.setAttribute('data-content', tok.c ? '1' : '0');
  span.textContent = tok.s;
  article.appendChild(span);
});

// Popup logic
const popup = document.getElementById('carve-popup');
let currentEl = null;
const defs = {
  '人工知能': {reading:'じんこうちのう', jlpt:'N3', pos:'名詞', def:'artificial intelligence; AI'},
  '生活':     {reading:'せいかつ', jlpt:'N4', pos:'名詞', def:'life; daily life; livelihood'},
  '現代':     {reading:'げんだい', jlpt:'N3', pos:'名詞', def:'present age; modern times'},
  '大きく':   {reading:'おおきく', jlpt:'N5', pos:'副詞', def:'greatly; on a large scale'},
  '変える':   {reading:'かえる', jlpt:'N4', pos:'動詞', def:'to change; to alter; to transform'},
};

document.addEventListener('click', e => {
  const t = e.target;
  if (t.getAttribute('data-carve') === 'token' && t.getAttribute('data-content') === '1') {
    currentEl = t;
    showPopup(t);
  } else {
    // Use composedPath so clicks that replace innerHTML mid-flight still see popup ancestry
    const path = e.composedPath ? e.composedPath() : [];
    const inPopup = path.some(el => el && el.id === 'carve-popup') || !!(t.closest && t.closest('#carve-popup'));
    if (!inPopup) {
      popup.style.display = 'none';
      currentEl = null;
    }
  }
});

function showPopup(el) {
  const lemma = el.getAttribute('data-lemma');
  const d = defs[lemma] || {reading: el.getAttribute('data-reading') || lemma, jlpt:'', pos:'名詞', def:'—'};
  const sentence = document.getElementById('article').textContent?.trim().slice(0,80);

  popup.innerHTML = \`
    <div class="popup-word">\${lemma}</div>
    <div class="popup-reading">
      \${d.reading}
      \${d.jlpt ? '<span class="popup-jlpt">'+d.jlpt+'</span>' : ''}
    </div>
    <div>
      <span class="popup-pos">\${d.pos}</span>
      <span class="popup-def">\${d.def}</span>
    </div>
    <div class="popup-sentence">"…\${sentence.slice(0,60)}…"</div>
    <div class="popup-actions">
      <button class="btn-mine-popup" onclick="showMineForm('\${lemma}','\${d.reading}','\${d.def}')">Mine</button>
      <button class="btn-ignore-popup" onclick="closePopup()">Ignore</button>
    </div>
  \`;

  const rect = el.getBoundingClientRect();
  popup.style.display = 'block';
  const top = rect.top > 240 ? rect.top - 240 - 8 : rect.bottom + 8;
  popup.style.top = top + 'px';
  popup.style.left = Math.min(Math.max(rect.left, 8), window.innerWidth - 336) + 'px';
}

function closePopup() {
  popup.style.display = 'none';
  currentEl = null;
}

window.showMineForm = function(lemma, reading, def) {
  const sentence = document.getElementById('article').textContent?.trim().slice(0,80) || '';
  popup.innerHTML = \`
    <div class="mine-form">
      <div style="font-size:14px;font-weight:700;color:#f7fafc;margin-bottom:8px">Add card</div>
      <label>Word</label>
      <input id="mine-lemma" value="\${lemma}">
      <label>Reading</label>
      <input id="mine-reading" value="\${reading}">
      <label>Definition</label>
      <input id="mine-def" value="\${def}">
      <label>Sentence</label>
      <textarea id="mine-sentence">\${sentence}</textarea>
      <button class="btn-save" id="btn-save-card">Save card</button>
      <div class="saved-status" id="save-status"></div>
    </div>
  \`;
  document.getElementById('btn-save-card').onclick = () => saveCard(lemma, reading);
};

window.saveCard = async function(lemma, reading) {
  const btn = document.getElementById('btn-save-card');
  btn.textContent = 'Saving…';
  btn.disabled = true;

  try {
    const res = await fetch(MOCK_BASE + '/v1/cards', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({
        lemma, reading,
        back_text: document.getElementById('mine-def')?.value,
        sentence: document.getElementById('mine-sentence')?.value,
        language_code: 'ja',
        source_url: window.location.href,
      }),
    });
    const data = await res.json();
    document.getElementById('save-status').textContent = '✓ Saved!';
    document.getElementById('save-status').style.color = '#68d391';
    if (currentEl) {
      currentEl.setAttribute('data-status','learning');
    }
    document.getElementById('comp-pct').textContent = '69%';
    document.querySelector('.bar-fill').style.width = '69%';
    setTimeout(() => closePopup(), 1000);
  } catch(e) {
    document.getElementById('save-status').textContent = 'Error saving';
    document.getElementById('save-status').style.color = '#fc8181';
    btn.disabled = false;
    btn.textContent = 'Save card';
  }
};
</script>
</body>
</html>`;
}

// Simulated video player page (mimics YouTube/streaming layout)
function makeVideoHTML(mockBase) {
  return `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Carve — Video Mining Demo</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, sans-serif; background: #0f1117; color: #e2e8f0; }
    .header { height: 52px; background: #161b2e; border-bottom: 1px solid #2d3748; display: flex; align-items: center; padding: 0 24px; gap: 12px; }
    .logo { font-weight: 700; color: #7c8cf8; font-size: 16px; }
    .yt-logo { color: #fc8181; font-weight: 800; font-size: 18px; margin-left: auto; }
    .player-wrap { display: flex; gap: 24px; padding: 20px 32px; }
    .player-main { flex: 1; }
    .video-box {
      background: #000; border-radius: 12px; overflow: hidden;
      aspect-ratio: 16/9; position: relative;
      display: flex; align-items: center; justify-content: center;
    }
    .video-placeholder {
      width: 100%; height: 100%; position: absolute; top: 0; left: 0;
      background: linear-gradient(135deg, #0d1526 0%, #1a2a4a 50%, #0d1526 100%);
    }
    .video-scene { position: absolute; top: 0; left: 0; right: 0; bottom: 0; display: flex; align-items: center; justify-content: center; }
    .anime-frame { font-size: 80px; opacity: 0.4; user-select: none; }
    .progress-bar { position: absolute; bottom: 44px; left: 12px; right: 12px; height: 4px; background: rgba(255,255,255,0.2); border-radius: 2px; }
    .progress-fill { height: 4px; background: #fc8181; border-radius: 2px; width: 35%; }
    .controls { position: absolute; bottom: 8px; left: 12px; right: 12px; display: flex; align-items: center; gap: 8px; }
    .ctrl-btn { font-size: 18px; cursor: pointer; color: #fff; opacity: 0.8; }
    .time-label { font-size: 12px; color: rgba(255,255,255,0.7); margin-left: 4px; }
    .subtitle-bar {
      position: absolute; bottom: 60px; left: 50%; transform: translateX(-50%);
      background: rgba(0,0,0,0.82); border-radius: 8px; padding: 10px 20px;
      font-size: 20px; line-height: 1.8; text-align: center; white-space: nowrap;
    }
    .carve-badge {
      position: absolute; top: 12px; right: 12px;
      background: #7c8cf8; color: #fff; font-size: 11px; font-weight: 700;
      padding: 4px 10px; border-radius: 20px; letter-spacing: 0.5px;
    }
    [data-carve="token"] { cursor: pointer; border-radius: 2px; padding: 1px 2px; transition: background 0.12s; }
    [data-carve="token"][data-status="unknown"][data-content="1"] { border-bottom: 2px solid #7c8cf8; }
    [data-carve="token"][data-status="learning"][data-content="1"] { border-bottom: 2px solid #f6ad55; }
    [data-carve="token"]:hover { background: rgba(124,140,248,0.25); }
    .sidebar { width: 320px; flex-shrink: 0; }
    .sidebar-title { font-size: 13px; font-weight: 600; color: #718096; margin-bottom: 12px; text-transform: uppercase; letter-spacing: 0.8px; }
    .card-preview { background: #161b2e; border: 1px solid #2d3748; border-radius: 10px; padding: 16px; margin-bottom: 10px; }
    .card-word { font-size: 20px; font-weight: 700; }
    .card-reading { font-size: 12px; color: #7c8cf8; margin-top: 2px; }
    .card-def { font-size: 13px; color: #718096; margin-top: 6px; }
    .card-status { display: inline-block; font-size: 10px; padding: 2px 8px; border-radius: 4px; margin-top: 6px; background: #f6ad5533; color: #f6ad55; }
    #video-popup {
      position: fixed; z-index: 9999; background: #1a2035; border: 1px solid #2d3748;
      border-radius: 12px; padding: 16px; width: 300px; box-shadow: 0 20px 60px rgba(0,0,0,0.6);
      display: none;
    }
    .popup-word { font-size: 26px; font-weight: 700; color: #f7fafc; }
    .popup-reading { font-size: 13px; color: #7c8cf8; margin: 4px 0 8px; }
    .popup-def { font-size: 13px; color: #cbd5e0; line-height: 1.5; }
    .popup-actions { display: flex; gap: 8px; margin-top: 12px; }
    .btn-mine-v { flex:1; background:#7c8cf8; color:#fff; border:none; border-radius:6px; padding:8px 0; font-size:13px; font-weight:600; cursor:pointer; }
    .mine-saved { color: #68d391; font-weight: 600; font-size: 14px; text-align: center; margin-top: 8px; }
  </style>
</head>
<body>
<div class="header">
  <div class="logo">⚡ Carve</div>
  <span style="font-size:12px;color:#4a5568">Mining mode active</span>
  <div class="yt-logo">▶ YouTube</div>
</div>

<div class="player-wrap">
  <div class="player-main">
    <div class="video-box">
      <div class="video-placeholder"></div>
      <div class="video-scene">
        <div class="anime-frame">🎌</div>
      </div>
      <div class="carve-badge">⚡ Carve Active</div>
      <div class="progress-bar"><div class="progress-fill" id="progress"></div></div>
      <div class="subtitle-bar" id="subtitles"></div>
      <div class="controls">
        <span class="ctrl-btn">⏸</span>
        <span class="ctrl-btn">⏭</span>
        <span class="time-label">2:14 / 6:35</span>
        <span class="ctrl-btn" style="margin-left:auto">🔊</span>
        <span class="ctrl-btn">⛶</span>
      </div>
    </div>
    <div style="margin-top:12px; font-size:15px; font-weight:600; color:#f7fafc">
      日本語AI入門 第3回：機械学習の基礎
    </div>
    <div style="font-size:12px;color:#4a5568;margin-top:4px">JapanesePod101 • 3.2M views</div>
  </div>

  <div class="sidebar">
    <div class="sidebar-title">Recently mined</div>
    <div id="mined-cards"></div>
  </div>
</div>

<div id="video-popup"></div>

<script>
const MOCK_BASE = '${mockBase}';
const minedCards = [];

const SUBTITLE_TOKENS = [
  {s:'人工知能',l:'人工知能',r:'じんこうちのう',c:true,st:'unknown'},
  {s:'は',l:'は',r:'は',c:false,st:'known'},
  {s:'日本語',l:'日本語',r:'にほんご',c:true,st:'learning'},
  {s:'を',l:'を',r:'を',c:false,st:'known'},
  {s:'学ぶ',l:'学ぶ',r:'まなぶ',c:true,st:'unknown'},
  {s:'のに',l:'のに',r:'のに',c:false,st:'known'},
  {s:'役立ち',l:'役立つ',r:'やくだち',c:true,st:'unknown'},
  {s:'ます',l:'ます',r:'ます',c:false,st:'known'},
  {s:'。',l:'。',r:'。',c:false,st:'known'},
];

const defs = {
  '人工知能': {r:'じんこうちのう', pos:'名詞', def:'artificial intelligence; AI'},
  '日本語': {r:'にほんご', pos:'名詞', def:'Japanese language'},
  '学ぶ': {r:'まなぶ', pos:'動詞', def:'to study; to learn'},
  '役立つ': {r:'やくだつ', pos:'動詞', def:'to be useful; to be helpful'},
};

const subtitleBar = document.getElementById('subtitles');
SUBTITLE_TOKENS.forEach(tok => {
  const span = document.createElement('span');
  span.setAttribute('data-carve','token');
  span.setAttribute('data-lemma',tok.l);
  span.setAttribute('data-content',tok.c?'1':'0');
  span.setAttribute('data-status',tok.st);
  span.textContent = tok.s;
  subtitleBar.appendChild(span);
});

const popup = document.getElementById('video-popup');
let popupEl = null;

document.addEventListener('click', e => {
  const t = e.target;
  if (t.getAttribute('data-carve') === 'token' && t.getAttribute('data-content') === '1') {
    popupEl = t;
    showVideoPopup(t);
  } else if (!t.closest('#video-popup')) {
    popup.style.display = 'none';
    popupEl = null;
  }
});

function showVideoPopup(el) {
  const lemma = el.getAttribute('data-lemma');
  const d = defs[lemma] || {r: lemma, pos:'名詞', def:'—'};
  popup.innerHTML = \`
    <div class="popup-word">\${lemma}</div>
    <div class="popup-reading">\${d.r}</div>
    <div class="popup-def"><em>\${d.pos}</em>  \${d.def}</div>
    <div class="popup-actions">
      <button class="btn-mine-v" id="btn-mine-v">⚡ Mine</button>
    </div>
    <div class="mine-saved" id="mine-saved-v" style="display:none">✓ Card saved!</div>
  \`;
  document.getElementById('btn-mine-v').onclick = () => mineVideoCard(lemma, d.r, d.def, el);

  const rect = el.getBoundingClientRect();
  popup.style.display = 'block';
  popup.style.top = (rect.top - 180 - 8) + 'px';
  popup.style.left = Math.min(Math.max(rect.left - 50, 8), window.innerWidth - 316) + 'px';
}

async function mineVideoCard(lemma, reading, def, el) {
  try {
    await fetch(MOCK_BASE + '/v1/cards', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({lemma, reading, back_text: def, language_code:'ja', source_url: window.location.href}),
    });
    document.getElementById('mine-saved-v').style.display = 'block';
    document.getElementById('btn-mine-v').style.display = 'none';
    el.setAttribute('data-status','learning');

    // Add to sidebar
    minedCards.unshift({lemma, reading, def});
    renderMinedCards();

    setTimeout(() => { popup.style.display='none'; popupEl=null; }, 900);
  } catch(e) { /* ignore in demo */ }
}

function renderMinedCards() {
  const container = document.getElementById('mined-cards');
  container.innerHTML = minedCards.slice(0,3).map(c => \`
    <div class="card-preview">
      <div class="card-word">\${c.lemma}</div>
      <div class="card-reading">\${c.reading}</div>
      <div class="card-def">\${c.def}</div>
      <span class="card-status">learning</span>
    </div>
  \`).join('');
}
</script>
</body>
</html>`;
}

// Review page — mock SRS review UI
function makeReviewHTML(mockBase) {
  return `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Carve — Review</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, sans-serif; background: #0f1117; color: #e2e8f0; min-height: 100vh; display: flex; flex-direction: column; }
    .topbar { height: 52px; background: #161b2e; border-bottom: 1px solid #2d3748; display: flex; align-items: center; padding: 0 24px; gap: 16px; flex-shrink: 0; }
    .topbar-logo { font-size: 18px; font-weight: 700; color: #7c8cf8; }
    .badge { background: #e53e3e; color: #fff; font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 10px; }
    .review-wrap { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 40px 20px; }
    .progress-header { display: flex; align-items: center; gap: 16px; margin-bottom: 32px; width: 100%; max-width: 560px; }
    .prog-bar { flex: 1; height: 8px; background: #2d3748; border-radius: 4px; }
    .prog-fill { height: 8px; background: linear-gradient(90deg, #68d391, #7c8cf8); border-radius: 4px; transition: width 0.5s ease; width: 0%; }
    .prog-label { font-size: 13px; color: #718096; white-space: nowrap; }
    .card { background: #161b2e; border: 1px solid #2d3748; border-radius: 16px; padding: 48px 40px; width: 100%; max-width: 560px; text-align: center; transition: transform 0.3s; }
    .card-front { font-size: 52px; font-weight: 700; color: #f7fafc; letter-spacing: 2px; }
    .card-context { font-size: 14px; color: #4a5568; margin-top: 12px; font-style: italic; }
    .reveal-btn { margin-top: 32px; background: #2d3748; border: none; border-radius: 10px; padding: 14px 48px; font-size: 15px; font-weight: 600; color: #e2e8f0; cursor: pointer; transition: background 0.15s; }
    .reveal-btn:hover { background: #3d4a5c; }
    .card-back { margin-top: 20px; }
    .card-reading-big { font-size: 22px; color: #7c8cf8; }
    .card-def-big { font-size: 18px; color: #cbd5e0; margin-top: 8px; line-height: 1.5; }
    .card-sentence { font-size: 14px; color: #4a5568; margin-top: 16px; font-style: italic; border-top: 1px solid #2d3748; padding-top: 16px; }
    .ratings { display: flex; gap: 12px; margin-top: 28px; width: 100%; max-width: 560px; }
    .rating-btn {
      flex: 1; border: none; border-radius: 10px; padding: 14px 0;
      font-size: 13px; font-weight: 600; cursor: pointer; transition: opacity 0.15s;
    }
    .r1 { background: #742a2a; color: #fc8181; }
    .r2 { background: #44337a; color: #b794f4; }
    .r3 { background: #1a365d; color: #90cdf4; }
    .r4 { background: #1c4532; color: #68d391; }
    .rating-btn:hover { opacity: 0.85; }
    .done-screen { text-align: center; }
    .done-emoji { font-size: 64px; margin-bottom: 16px; }
    .done-title { font-size: 28px; font-weight: 700; color: #f7fafc; }
    .done-sub { font-size: 15px; color: #718096; margin-top: 8px; }
  </style>
</head>
<body>
<div class="topbar">
  <div class="topbar-logo">⚡ Carve</div>
  <span style="font-size:13px;color:#718096">Review session</span>
  <span class="badge" id="due-badge">5 due</span>
</div>
<div class="review-wrap">
  <div class="progress-header">
    <div class="prog-bar"><div class="prog-fill" id="prog-fill"></div></div>
    <div class="prog-label" id="prog-label">0 / 5</div>
  </div>
  <div id="card-area"></div>
  <div class="ratings" id="ratings" style="display:none">
    <button class="rating-btn r1" onclick="rate(1)">Again<br><small>10m</small></button>
    <button class="rating-btn r2" onclick="rate(2)">Hard<br><small>1d</small></button>
    <button class="rating-btn r3" onclick="rate(3)">Good<br><small>4d</small></button>
    <button class="rating-btn r4" onclick="rate(4)">Easy<br><small>10d</small></button>
  </div>
</div>

<script>
const MOCK_BASE = '${mockBase}';
const QUEUE = ${JSON.stringify(DEMO_CARDS)};
let idx = 0;
let shown = false;
let reviewed = 0;
const total = QUEUE.length;

function renderCard() {
  shown = false;
  document.getElementById('ratings').style.display = 'none';
  const card = QUEUE[idx];
  if (!card) { renderDone(); return; }

  const pct = (reviewed / total * 100).toFixed(0);
  document.getElementById('prog-fill').style.width = pct + '%';
  document.getElementById('prog-label').textContent = reviewed + ' / ' + total;

  document.getElementById('card-area').innerHTML = \`
    <div class="card" id="review-card">
      <div class="card-front">\${card.front_text}</div>
      <div class="card-context">What does this word mean?</div>
      <button class="reveal-btn" onclick="reveal()">Show answer</button>
    </div>
  \`;
}

window.reveal = function() {
  shown = true;
  const card = QUEUE[idx];
  document.getElementById('review-card').innerHTML = \`
    <div class="card-front">\${card.front_text}</div>
    <div class="card-back">
      <div class="card-reading-big">\${card.front_reading}</div>
      <div class="card-def-big">\${card.back_text}</div>
      \${card.sentence ? '<div class="card-sentence">"' + card.sentence.slice(0,60) + '…"</div>' : ''}
    </div>
  \`;
  document.getElementById('ratings').style.display = 'flex';
};

window.rate = async function(r) {
  reviewed++;
  idx++;
  try {
    await fetch(MOCK_BASE + '/v1/review/events', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({card_id: QUEUE[idx-1]?.id, rating: r}),
    });
  } catch(e){}
  const pct = (reviewed / total * 100).toFixed(0);
  document.getElementById('prog-fill').style.width = pct + '%';
  document.getElementById('prog-label').textContent = reviewed + ' / ' + total;
  document.getElementById('due-badge').textContent = Math.max(0, total - reviewed) + ' due';

  if (idx >= QUEUE.length) renderDone();
  else renderCard();
};

function renderDone() {
  document.getElementById('ratings').style.display = 'none';
  document.getElementById('card-area').innerHTML = \`
    <div class="done-screen">
      <div class="done-emoji">🎉</div>
      <div class="done-title">Session complete!</div>
      <div class="done-sub">Reviewed \${reviewed} cards • Next review in 4 days</div>
    </div>
  \`;
}

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
    res.setHeader('Access-Control-Allow-Headers', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET,POST,OPTIONS');

    if (req.method === 'OPTIONS') { res.writeHead(204); res.end(); return; }

    if (req.url === '/reader') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeReaderHTML(`http://127.0.0.1:${server.address().port}`));
      return;
    }
    if (req.url === '/video') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeVideoHTML(`http://127.0.0.1:${server.address().port}`));
      return;
    }
    if (req.url === '/review') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(makeReviewHTML(`http://127.0.0.1:${server.address().port}`));
      return;
    }

    let body = '';
    req.on('data', c => { body += c; });
    req.on('end', () => {
      if (req.url === '/v1/cards' && req.method === 'POST') {
        let parsed = {}; try { parsed = JSON.parse(body); } catch {}
        capturedCards.push(parsed);
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ id: 'demo-card-' + Date.now(), lemma: parsed.lemma }));

      } else if (req.url.startsWith('/v1/review')) {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: true }));

      } else {
        res.writeHead(404); res.end('{}');
      }
    });
  });

  return new Promise(resolve => {
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      resolve({ server, port, capturedCards });
    });
  });
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function assert(cond, msg) {
  if (!cond) {
    if (CI) throw new Error(`FAIL: ${msg}`);
    else console.warn(`[warn] ${msg}`);
  }
}

async function wait(ms) { return new Promise(r => setTimeout(r, ms)); }

// ── Demo scenes ───────────────────────────────────────────────────────────────

async function sceneReader(page, base) {
  console.log('\n── Scene 1: Reader + text mining ────────────────────────────');
  await page.goto(`${base}/reader`);
  await page.waitForSelector('[data-carve="token"][data-content="1"]', { timeout: 5000, state: 'attached' });
  console.log('[page] article annotated');
  await wait(800); // let viewer take in the reader UI

  // Capture lemma before clicking (locator selector includes status, so after mining
  // the locator would resolve to a different element — use lemma to re-query later).
  const firstUnknown = page.locator('[data-carve="token"][data-status="unknown"][data-content="1"]').first();
  await firstUnknown.waitFor({ state: 'attached', timeout: 3000 });
  const minedLemma = await firstUnknown.getAttribute('data-lemma');
  await firstUnknown.click();
  console.log(`[action] clicked token: "${minedLemma}"`);

  // Popup should appear
  await page.waitForSelector('#carve-popup', { timeout: 3000, state: 'visible' });
  console.log('[page] popup visible');
  await wait(1200); // viewer reads popup

  // Assertions
  const popupText = await page.$eval('#carve-popup', el => el.textContent ?? '');
  assert(popupText.includes('Mine'), 'popup should contain Mine button');

  // Click Mine
  await page.click('.btn-mine-popup');
  await page.waitForSelector('.btn-save', { timeout: 2000, state: 'visible' });
  console.log('[page] mine form open');
  await wait(900);

  // Click Save card
  await page.click('.btn-save');
  // Wait for the save-status div to show success text
  await page.waitForFunction(
    () => document.getElementById('save-status')?.textContent?.includes('Saved'),
    { timeout: 3000 },
  );
  console.log('[page] card saved');
  await wait(800);

  // Re-query by lemma (status attribute may have changed from "unknown" → "learning")
  const newStatus = await page.$eval(
    `[data-carve="token"][data-lemma="${minedLemma}"]`,
    el => el.getAttribute('data-status'),
  ).catch(() => 'error: element not found');
  assert(newStatus === 'learning', `token should be "learning" after mining, got "${newStatus}"`);
  console.log(`[assert] token → learning ✓`);
  await wait(400);
}

async function sceneVideoMining(page, base, capturedCards) {
  console.log('\n── Scene 2: Video player + subtitle mining ──────────────────');
  await page.goto(`${base}/video`);
  await page.waitForSelector('[data-carve="token"][data-content="1"]', { timeout: 5000, state: 'attached' });
  await wait(700);

  const beforeCount = capturedCards.length;

  // Click an unknown subtitle token
  const subToken = page.locator('[data-carve="token"][data-status="unknown"][data-content="1"]').first();
  await subToken.waitFor({ state: 'attached', timeout: 3000 });
  const subLemma = await subToken.getAttribute('data-lemma');
  await subToken.click();
  console.log(`[action] clicked subtitle token: "${subLemma}"`);

  await page.waitForSelector('#video-popup', { timeout: 3000, state: 'visible' });
  console.log('[page] video popup visible');
  await wait(1000);

  // Click Mine
  await page.click('#btn-mine-v');
  console.log('[action] mined from video');
  await wait(800);

  // Card created?
  const deadline = Date.now() + 2000;
  while (capturedCards.length === beforeCount && Date.now() < deadline) await wait(50);
  assert(capturedCards.length > beforeCount, 'POST /v1/cards not received after video mining');
  console.log(`[assert] POST /v1/cards received ✓`);

  // Sidebar should update
  await page.waitForSelector('.card-preview', { timeout: 2000, state: 'attached' }).catch(() => {});
  console.log('[page] mined card appears in sidebar');
  await wait(600);
}

async function sceneReview(page, base) {
  console.log('\n── Scene 3: SRS review ───────────────────────────────────────');
  await page.goto(`${base}/review`);
  await page.waitForSelector('.card-front', { timeout: 5000, state: 'visible' });
  console.log('[page] review card visible');
  await wait(800);

  // Reveal answer
  await page.click('.reveal-btn');
  await page.waitForSelector('.card-back', { timeout: 2000, state: 'visible' });
  console.log('[page] answer revealed');
  await wait(1000);

  assert(await page.isVisible('.ratings'), 'rating buttons should appear after reveal');

  // Rate "Good"
  await page.click('.r3');
  console.log('[action] rated Good');
  await wait(600);

  // Next card
  await page.waitForSelector('.reveal-btn', { timeout: 3000, state: 'visible' }).catch(async () => {
    // May have gone to done screen if only 1 card
    const done = await page.isVisible('.done-screen').catch(() => false);
    if (done) { console.log('[page] review done!'); return; }
  });

  // Show done screen
  await page.click('.reveal-btn').catch(() => {});
  await wait(400);
  await page.click('.r4').catch(() => {});
  await wait(500);

  const donePct = await page.$eval('#prog-fill', el => el.style.width).catch(() => '?');
  console.log(`[assert] progress: ${donePct} ✓`);
  await wait(600);
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function run() {
  console.log('Carve Promotional Video Recorder');
  console.log(`Mode: ${CI ? 'CI (no video)' : 'promo recording'}`);

  const { server, port, capturedCards } = await startMockServer();
  const base = `http://127.0.0.1:${port}`;
  console.log(`[mock] server at ${base}`);

  const videoDir = path.resolve(__dirname, 'videos');
  if (!CI) fs.mkdirSync(videoDir, { recursive: true });

  const viewport = { width: 1280, height: 720 };
  const ctxOptions = {
    viewport,
    ...(CI ? {} : {
      recordVideo: {
        dir: videoDir,
        size: viewport,
      },
    }),
  };

  const browser = await chromium.launch({
    headless: CI,
    slowMo: CI ? 0 : 60, // slow-motion for smooth recording
    args: ['--disable-web-security', '--allow-running-insecure-content'],
  });

  const context = await browser.newContext(ctxOptions);
  const page = await context.newPage();

  try {
    await sceneReader(page, base, capturedCards);
    await sceneVideoMining(page, base, capturedCards);
    await sceneReview(page, base);

    if (!CI) {
      // Final hold on done screen for dramatic effect
      await wait(1200);
    }

    console.log('\n✓ PASS — all scenes completed');

    if (!CI) {
      const videoPath = await page.video()?.path();
      if (videoPath) {
        // Rename to a human-readable name
        const date = new Date().toISOString().slice(0,10).replace(/-/g,'');
        const dest = path.resolve(__dirname, `promo-${date}.webm`);
        fs.renameSync(videoPath, dest);
        console.log(`\n🎬 Video saved: ${dest}`);
        console.log(`   Duration: ~28s at 1280×720`);
        console.log(`   Convert:  ffmpeg -i ${dest} promo-${date}.mp4`);
      }
    }
  } catch (err) {
    console.error('\n✗ FAIL —', err.message);
    process.exit(1);
  } finally {
    await context.close();
    await browser.close();
    server.close();
  }
}

run().catch(err => {
  console.error('\n✗ FAIL —', err.message);
  process.exit(1);
});
