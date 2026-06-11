<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import {
    fetchReviewSession,
    submitReviewEvent,
    fetchIntervals,
    undoReview,
    suspendCard,
    buryCard,
    type Card,
    type IntervalsResponse,
  } from '$lib/api';
  import { lang } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';
  import { queueEvent, flushQueue } from '$lib/offline';

  const REVIEW_SESSION_LIMIT = 50;

  type Phase = 'loading' | 'front' | 'back' | 'done' | 'error';

  let phase: Phase = 'loading';
  let queue: Card[] = [];
  let current: Card | null = null;
  let intervals: IntervalsResponse | null = null;
  let errorMessage = '';
  let sessionStartTime = 0;
  let cardStartTime = 0;
  let reviewedCount = 0;
  let isLeechNotif = false;
  let submitting = false;
  let showHelp = false;
  // undo stack: stores {card, intervals, priorCount} so Z re-inserts it
  let lastCard: Card | null = null;

  function ratingVal(n: number): 1 | 2 | 3 | 4 { return n as 1 | 2 | 3 | 4; }

  // ── Card type (recognition vs production) ───────────────────────────────────
  // `recognition` (default): prompt with the target word, recall the meaning.
  // `production`/`recall`: prompt with the meaning/sentence, produce the word.
  // Unknown values fall back to recognition behaviour.
  function isProduction(c: Card | null): boolean {
    if (!c) return false;
    return c.card_type === 'production' || c.card_type === 'recall';
  }

  function cardTypeLabel(c: Card | null): string {
    if (!c) return '';
    switch (c.card_type) {
      case 'production': return 'Production';
      case 'recall': return 'Recall';
      default: return 'Recognition';
    }
  }

  // Production prompt: show the sentence with the target word blanked out so the
  // learner must produce it. Falls back to the meaning/definition when there is
  // no sentence to mask.
  function productionPrompt(c: Card): string {
    if (c.sentence && c.front_text && c.sentence.includes(c.front_text)) {
      return c.sentence.split(c.front_text).join('____');
    }
    return c.back_text ?? c.sentence ?? '';
  }

  function playAudio() {
    const el = document.getElementById('card-audio') as HTMLAudioElement | null;
    el?.play();
  }

  function playSentenceAudio() {
    const el = document.getElementById('card-sentence-audio') as HTMLAudioElement | null;
    el?.play();
  }

  function formatInterval(isoOrDuration: string): string {
    const now = Date.now();
    const due = new Date(isoOrDuration).getTime();
    const diff = due - now;
    if (diff < 0) return 'now';
    const mins = Math.round(diff / 60_000);
    if (mins < 60) return `${mins}m`;
    const hours = Math.round(diff / 3_600_000);
    if (hours < 24) return `${hours}h`;
    const days = Math.round(diff / 86_400_000);
    return `${days}d`;
  }

  function formatStability(s: number | null): string {
    if (s == null) return '–';
    if (s < 1) return `${Math.round(s * 24)}h`;
    return `${s.toFixed(1)}d`;
  }

  onMount(() => {
    let init = false;
    const unsub = lang.subscribe(l => { if (init) loadSession(l); });
    loadSession(get(lang));
    init = true;
    window.addEventListener('keydown', handleKey);
    window.addEventListener('online', onOnline);
    flushOfflineQueueIfAny();
    return () => {
      unsub();
      window.removeEventListener('keydown', handleKey);
      window.removeEventListener('online', onOnline);
    };
  });

  async function onOnline() {
    await flushOfflineQueueIfAny();
  }

  async function flushOfflineQueueIfAny() {
    try {
      const { flushed } = await flushQueue(async (payload) => {
        const { card_id, rating, time_taken_ms } = payload as { card_id: string; rating: 1|2|3|4; time_taken_ms: number };
        await submitReviewEvent(card_id, rating, time_taken_ms);
      });
      if (flushed > 0) toasts.add(`Synced ${flushed} offline review${flushed === 1 ? '' : 's'}`);
    } catch { /* ignore */ }
  }

  function handleKey(e: KeyboardEvent) {
    // Ignore when typing in inputs/textareas
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;

    if (e.key === '?') { showHelp = !showHelp; return; }
    if (showHelp) { showHelp = false; return; }

    switch (e.key) {
      case ' ':
        e.preventDefault();
        if (phase === 'front') flip();
        break;
      case '1': if (phase === 'back') rate(1); break;
      case '2': if (phase === 'back') rate(2); break;
      case '3': if (phase === 'back') rate(3); break;
      case '4': if (phase === 'back') rate(4); break;
      case 'z':
      case 'Z':
        undo();
        break;
      case 'e':
      case 'E':
        if (current) window.open(`/cards/${current.id}`, '_blank');
        break;
      case 's':
      case 'S':
        if (current) doSuspend();
        break;
      case 'b':
      case 'B':
        if (current) doBury();
        break;
      case 'a':
      case 'A':
        playAudio();
        break;
    }
  }

  async function loadSession(language = get(lang)) {
    phase = 'loading';
    reviewedCount = 0;
    queue = [];
    lastCard = null;
    try {
      const res = await fetchReviewSession(language, REVIEW_SESSION_LIMIT);
      queue = res.cards;
      sessionStartTime = Date.now();
      advance();
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : 'Unknown error';
      phase = 'error';
    }
  }

  function advance() {
    if (queue.length === 0) { phase = 'done'; return; }
    current = queue.shift()!;
    intervals = null;
    cardStartTime = Date.now();
    phase = 'front';
  }

  async function flip() {
    if (!current || phase !== 'front') return;
    phase = 'back';
    try {
      intervals = await fetchIntervals(current.id);
    } catch {
      // interval preview is optional
    }
  }

  async function rate(rating: 1 | 2 | 3 | 4) {
    if (!current || submitting) return;
    submitting = true;
    const timeTakenMs = Date.now() - cardStartTime;
    const cardBeforeRate = current;
    try {
      const result = await submitReviewEvent(current.id, rating, timeTakenMs);
      reviewedCount++;
      lastCard = cardBeforeRate;
      if (result.is_leech) {
        isLeechNotif = true;
        setTimeout(() => { isLeechNotif = false; }, 4000);
      }
    } catch (err) {
      // Queue offline if we can't reach the server — works in airplane mode too.
      const isNetwork = typeof navigator !== 'undefined' && !navigator.onLine;
      if (isNetwork) {
        await queueEvent({ card_id: cardBeforeRate.id, rating, time_taken_ms: timeTakenMs });
        reviewedCount++;
        lastCard = cardBeforeRate;
        toasts.add('Saved offline — will sync when reconnected');
      } else {
        toasts.add(err instanceof Error ? err.message : 'Failed to save review');
      }
    }
    if (reviewedCount === 1 && typeof localStorage !== 'undefined') {
      localStorage.setItem('carve_reviewed_once', '1');
    }
    submitting = false;
    advance();
  }

  // ── Swipe gestures for touch devices ────────────────────────────────────────
  let touchStartX = 0;
  let touchStartY = 0;
  let touchStartT = 0;
  const SWIPE_THRESHOLD = 60;       // px
  const SWIPE_MAX_DURATION = 600;   // ms
  const TAP_THRESHOLD = 10;         // px

  function onTouchStart(e: TouchEvent) {
    const t = e.touches[0];
    touchStartX = t.clientX;
    touchStartY = t.clientY;
    touchStartT = Date.now();
  }

  function onTouchEnd(e: TouchEvent) {
    const t = e.changedTouches[0];
    const dx = t.clientX - touchStartX;
    const dy = t.clientY - touchStartY;
    const dt = Date.now() - touchStartT;
    const absX = Math.abs(dx);
    const absY = Math.abs(dy);

    // Tap → flip (front → back)
    if (absX < TAP_THRESHOLD && absY < TAP_THRESHOLD && dt < 300) {
      if (phase === 'front') flip();
      return;
    }

    if (dt > SWIPE_MAX_DURATION) return;
    if (Math.max(absX, absY) < SWIPE_THRESHOLD) return;
    if (phase !== 'back') return;

    if (absX > absY) {
      // Horizontal: left → Again (1), right → Easy (4)
      if (dx < 0) rate(1);
      else        rate(4);
    } else {
      // Vertical: down → Hard (2), up → Good (3)
      if (dy > 0) rate(2);
      else        rate(3);
    }
  }

  async function undo() {
    if (!lastCard || submitting) return;
    submitting = true;
    try {
      await undoReview();
      // Re-insert the card at the front of the queue
      if (current) queue.unshift(current);
      current = lastCard;
      lastCard = null;
      intervals = null;
      reviewedCount = Math.max(0, reviewedCount - 1);
      phase = 'front';
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Nothing to undo');
    }
    submitting = false;
  }

  async function doSuspend() {
    if (!current || submitting) return;
    submitting = true;
    try {
      await suspendCard(current.id);
      toasts.add('Card suspended');
      advance();
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Failed to suspend');
    }
    submitting = false;
  }

  async function doBury() {
    if (!current || submitting) return;
    submitting = true;
    try {
      await buryCard(current.id);
      advance();
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Failed to bury');
    }
    submitting = false;
  }

  const RATING_LABELS: Record<number, string> = { 1: 'Again', 2: 'Hard', 3: 'Good', 4: 'Easy' };
  const RATING_COLORS: Record<number, string> = {
    1: '#e57373', 2: '#ffb74d', 3: '#4caf50', 4: '#42a5f5',
  };

  const SHORTCUTS = [
    { key: 'Space',     desc: 'Show answer' },
    { key: '1–4',       desc: 'Rate card (Again / Hard / Good / Easy)' },
    { key: 'Z',         desc: 'Undo last review' },
    { key: 'A',         desc: 'Play audio' },
    { key: 'E',         desc: 'Edit card (opens in new tab)' },
    { key: 'S',         desc: 'Suspend card' },
    { key: 'B',         desc: 'Bury card' },
    { key: '?',         desc: 'Toggle this help' },
    { key: 'Tap',       desc: 'Touch: flip card' },
    { key: '◀ swipe',   desc: 'Touch: Again' },
    { key: 'swipe ▶',   desc: 'Touch: Easy' },
    { key: '▼ swipe',   desc: 'Touch: Hard' },
    { key: '▲ swipe',   desc: 'Touch: Good' },
  ];
</script>

<main>
  {#if isLeechNotif}
    <div class="leech-banner">Card suspended — too many lapses (leech detected)</div>
  {/if}

  {#if showHelp}
    <div class="help-overlay" on:click={() => { showHelp = false; }} on:keydown={() => {}} role="button" tabindex="-1">
      <div class="help-box" on:click|stopPropagation on:keydown={() => {}} role="none">
        <div class="help-title">Keyboard shortcuts</div>
        <table class="help-table">
          {#each SHORTCUTS as s}
            <tr>
              <td class="help-key"><kbd>{s.key}</kbd></td>
              <td class="help-desc">{s.desc}</td>
            </tr>
          {/each}
        </table>
        <button class="help-close" on:click={() => { showHelp = false; }}>Close</button>
      </div>
    </div>
  {/if}

  {#if phase === 'loading'}
    <div class="center-msg"><p>Loading session…</p></div>

  {:else if phase === 'error'}
    <div class="center-msg error">
      <p>{errorMessage}</p>
      <button class="btn" on:click={() => loadSession()}>Retry</button>
    </div>

  {:else if phase === 'done'}
    <div class="done-screen">
      <p class="done-emoji">✓</p>
      <h2>Session complete</h2>
      <p class="done-sub">{reviewedCount} card{reviewedCount === 1 ? '' : 's'} reviewed</p>
      <div class="done-actions">
        <button class="btn" on:click={() => loadSession()}>Review more</button>
        <a href="/cards" class="btn btn-ghost">View cards</a>
      </div>
    </div>

  {:else if current}
    <div class="top-bar">
      <div class="progress-bar-wrap">
        <div class="progress-bar" style="width:{Math.max(4, (reviewedCount / (reviewedCount + queue.length + 1)) * 100)}%"></div>
      </div>
      <div class="session-actions">
        {#if lastCard}
          <button class="action-btn" on:click={undo} disabled={submitting} title="Undo (Z)">↩ Undo</button>
        {/if}
        <button class="action-btn" on:click={() => { showHelp = true; }} title="Keyboard shortcuts (?)">?</button>
      </div>
    </div>

    <div
      class="card-wrap"
      on:touchstart={onTouchStart}
      on:touchend={onTouchEnd}
    >
      <div class="flashcard" class:flipped={phase === 'back'}>
        <span class="card-type-badge" class:production={isProduction(current)}>
          {cardTypeLabel(current)}
        </span>

        {#if current.audio_url}
          <audio src={current.audio_url} preload="auto" id="card-audio"></audio>
        {/if}
        {#if current.sentence_audio_url}
          <audio src={current.sentence_audio_url} preload="auto" id="card-sentence-audio"></audio>
        {/if}

        {#if isProduction(current)}
          <!-- Production / recall: prompt with meaning, produce the word. -->
          <div class="card-front">
            <div class="prompt-label">Produce the word</div>
            <p class="definition">{productionPrompt(current)}</p>
            {#if current.subtitle_translation}
              <p class="translation">{current.subtitle_translation}</p>
            {/if}
            {#if phase === 'front'}
              <button class="show-btn" on:click={flip}>Show answer <span class="hint">Space</span></button>
            {/if}
          </div>

          {#if phase === 'back'}
            <div class="card-back">
              <div class="word">{current.front_text}</div>
              {#if current.audio_url}
                <button class="audio-btn" on:click={playAudio} title="Play audio (A)">▶</button>
              {/if}
              {#if current.sentence}
                <p class="sentence">{current.sentence}</p>
              {/if}
              {#if current.sentence_audio_url}
                <button class="audio-btn" on:click={playSentenceAudio} title="Play sentence audio (S)">▶ sentence</button>
              {/if}
              {#if current.image_url}
                <img class="card-image" src={current.image_url} alt="context screenshot" />
              {/if}
            </div>
          {/if}
        {:else}
          <!-- Recognition (default): prompt with the word, recall the meaning. -->
          <div class="card-front">
            <div class="word">{current.front_text}</div>
            {#if current.audio_url}
              <button class="audio-btn" on:click={playAudio} title="Play audio (A)">▶</button>
            {/if}
            {#if phase === 'front'}
              <button class="show-btn" on:click={flip}>Show answer <span class="hint">Space</span></button>
            {/if}
          </div>

          {#if phase === 'back'}
            <div class="card-back">
              {#if current.back_text}
                <p class="definition">{current.back_text}</p>
              {/if}
              {#if current.sentence}
                <p class="sentence">{current.sentence}</p>
              {/if}
              {#if current.subtitle_translation}
                <p class="translation">{current.subtitle_translation}</p>
              {/if}
              {#if current.sentence_audio_url}
                <button class="audio-btn" on:click={playSentenceAudio} title="Play sentence audio (S)">▶ sentence</button>
              {/if}
              {#if current.image_url}
                <img class="card-image" src={current.image_url} alt="context screenshot" />
              {/if}
            </div>
          {/if}
        {/if}
      </div>

      {#if phase === 'back' && (current.stability != null || current.difficulty != null)}
        <div class="stats-row">
          <span title="Stability">S: {formatStability(current.stability)}</span>
          <span title="Difficulty">D: {current.difficulty?.toFixed(1) ?? '–'}</span>
          <span title="Lapses">Lapses: {current.lapses}</span>
          <span title="Reviews">Reps: {current.reps}</span>
        </div>
      {/if}

      {#if phase === 'back'}
        <div class="rating-row">
          {#each [1, 2, 3, 4] as r}
            <button
              class="rating-btn"
              style="--color: {RATING_COLORS[r]}"
              disabled={submitting}
              on:click={() => rate(ratingVal(r))}
            >
              <span class="rating-label">{RATING_LABELS[r]}</span>
              <span class="key-hint">{r}</span>
              {#if intervals}
                <span class="interval-hint">
                  {r === 1 ? formatInterval(intervals.again)
                   : r === 2 ? formatInterval(intervals.hard)
                   : r === 3 ? formatInterval(intervals.good)
                   : formatInterval(intervals.easy)}
                </span>
              {/if}
            </button>
          {/each}
        </div>

        <div class="lifecycle-row">
          <button class="lc-btn" on:click={doSuspend} disabled={submitting} title="Suspend card (S)">Suspend</button>
          <button class="lc-btn" on:click={doBury} disabled={submitting} title="Bury card (B)">Bury</button>
          <a href="/cards/{current.id}" target="_blank" class="lc-btn" title="Edit card (E)">Edit</a>
        </div>
      {/if}
    </div>
  {/if}
</main>

<style>
  main {
    max-width: 720px;
    margin: 0 auto;
    padding: 1.5rem 1rem;
    min-height: calc(100vh - 50px);
  }

  .leech-banner {
    background: #3a1515;
    border: 1px solid #e57373;
    color: #e57373;
    padding: 0.6rem 1rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    font-size: 0.9rem;
    text-align: center;
  }

  /* ── Help overlay ── */
  .help-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.6);
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .help-box {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 12px;
    padding: 1.5rem 2rem;
    min-width: 320px;
  }
  .help-title {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 1rem;
    color: #e8eaf0;
  }
  .help-table { border-collapse: collapse; width: 100%; }
  .help-table tr + tr td { padding-top: 0.4rem; }
  .help-key { width: 80px; vertical-align: top; }
  kbd {
    background: #2a2d36;
    border: 1px solid #3a3d47;
    color: #9ba8c0;
    font-family: inherit;
    font-size: 0.78rem;
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
  }
  .help-desc { color: #9ba8c0; font-size: 0.88rem; }
  .help-close {
    margin-top: 1.25rem;
    background: #2a2d36;
    border: none;
    color: #9ba8c0;
    padding: 0.4rem 1rem;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.85rem;
  }
  .help-close:hover { background: #3a3d47; }

  /* ── Top bar ── */
  .top-bar {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }
  .progress-bar-wrap {
    flex: 1;
    background: #1e2128;
    border-radius: 4px;
    height: 4px;
    overflow: hidden;
  }
  .progress-bar {
    height: 100%;
    background: #2e7d32;
    transition: width 0.3s ease;
    border-radius: 4px;
  }
  .session-actions { display: flex; gap: 0.5rem; }
  .action-btn {
    background: #1e2128;
    border: 1px solid #2a2d36;
    color: #9ba8c0;
    padding: 0.25rem 0.6rem;
    border-radius: 5px;
    font-size: 0.8rem;
    cursor: pointer;
    transition: background 0.15s;
    white-space: nowrap;
  }
  .action-btn:hover:not(:disabled) { background: #2a2d36; }
  .action-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  /* ── Card ── */
  .card-wrap {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.25rem;
  }

  .flashcard {
    position: relative;
    width: 100%;
    min-height: 260px;
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 12px;
    padding: 2rem 2.5rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1rem;
    text-align: center;
  }

  .card-type-badge {
    position: absolute;
    top: 0.75rem;
    left: 0.75rem;
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: #8a96b3;
    background: #2a2d36;
    border: 1px solid #3a3d47;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
  }
  .card-type-badge.production {
    color: #ffb74d;
    border-color: #ffb74d;
    background: color-mix(in srgb, #ffb74d 12%, #1e2128);
  }

  .prompt-label {
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: #8a96b3;
  }

  .word { font-size: 3rem; font-weight: 700; line-height: 1.2; }
  .definition { font-size: 1.3rem; color: #b0bec5; margin: 0; }
  .sentence {
    font-size: 0.95rem;
    color: #8a96b3;
    font-style: italic;
    margin: 0.5rem 0 0;
    line-height: 1.6;
  }
  .translation {
    font-size: 0.9rem;
    color: #7a8aa6;
    margin: 0.35rem 0 0;
    line-height: 1.5;
  }
  .card-image {
    max-width: 100%;
    max-height: 180px;
    border-radius: 6px;
    object-fit: contain;
    margin-top: 0.5rem;
  }

  .audio-btn {
    background: #2a2d36;
    border: 1px solid #3a3d47;
    color: #9ba8c0;
    border-radius: 50%;
    width: 2.2rem;
    height: 2.2rem;
    cursor: pointer;
    font-size: 0.9rem;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.15s;
  }
  .audio-btn:hover { background: #3a3d47; }

  .show-btn {
    background: #2e7d32;
    border: none;
    color: #fff;
    padding: 0.7rem 2rem;
    border-radius: 8px;
    font-size: 1rem;
    cursor: pointer;
    margin-top: 0.5rem;
    font-weight: 500;
    transition: background 0.15s;
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .show-btn:hover { background: #43a047; }

  .hint {
    background: rgba(0,0,0,0.35);
    color: #fff;
    font-size: 0.72rem;
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
  }

  .stats-row {
    display: flex;
    gap: 1.25rem;
    font-size: 0.78rem;
    color: #8a96b3;
    flex-wrap: wrap;
    justify-content: center;
  }

  .rating-row {
    display: flex;
    gap: 0.75rem;
    width: 100%;
    justify-content: center;
    flex-wrap: wrap;
  }

  .rating-btn {
    flex: 1;
    min-width: 80px;
    max-width: 140px;
    padding: 0.75rem 0.5rem;
    background: #1e2128;
    border: 2px solid var(--color);
    color: var(--color);
    border-radius: 10px;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.2rem;
    font-size: 0.95rem;
    font-weight: 600;
    transition: background 0.15s;
  }
  .rating-btn:hover:not(:disabled) { background: color-mix(in srgb, var(--color) 15%, #1e2128); }
  .rating-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .rating-label { font-size: 0.9rem; }
  .key-hint {
    font-size: 0.65rem;
    font-weight: 400;
    opacity: 0.5;
    background: rgba(255,255,255,0.08);
    padding: 0.05rem 0.3rem;
    border-radius: 3px;
  }
  .interval-hint { font-size: 0.72rem; font-weight: 400; opacity: 0.8; }

  .lifecycle-row {
    display: flex;
    gap: 0.5rem;
    justify-content: center;
  }
  .lc-btn {
    background: transparent;
    border: 1px solid #2a2d36;
    color: #8a96b3;
    padding: 0.3rem 0.9rem;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
    text-decoration: none;
    transition: border-color 0.15s, color 0.15s;
  }
  .lc-btn:hover:not(:disabled) { border-color: #8a96b3; color: #9ba8c0; }
  .lc-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .center-msg { text-align: center; margin-top: 4rem; color: #9ba8c0; }
  .center-msg.error { color: #e57373; }

  .done-screen { text-align: center; margin-top: 5rem; }
  .done-emoji { font-size: 3rem; color: #4caf50; margin: 0; }
  .done-screen h2 { font-size: 1.8rem; margin: 0.5rem 0; }
  .done-sub { color: #9ba8c0; margin-bottom: 2rem; }
  .done-actions { display: flex; gap: 1rem; justify-content: center; }

  .btn {
    display: inline-block;
    padding: 0.65rem 1.5rem;
    background: #2e7d32;
    color: #fff;
    border-radius: 7px;
    text-decoration: none;
    font-size: 0.95rem;
    font-weight: 500;
    border: none;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn:hover { background: #43a047; }
  .btn-ghost {
    background: transparent;
    border: 1px solid #4caf50;
    color: #4caf50;
  }
  .btn-ghost:hover { background: rgba(76, 175, 80, 0.1); }
</style>
