<script lang="ts">
  import { onMount } from 'svelte';
  import {
    fetchReviewSession,
    submitReviewEvent,
    fetchIntervals,
    ApiError,
    type Card,
    type IntervalsResponse,
  } from '$lib/api';

  type Phase = 'loading' | 'front' | 'back' | 'done' | 'unauthenticated' | 'error';

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

  function ratingVal(n: number): 1 | 2 | 3 | 4 { return n as 1 | 2 | 3 | 4; }

  function playAudio() {
    const el = document.getElementById('card-audio') as HTMLAudioElement | null;
    el?.play();
  }

  // ── helpers ───────────────────────────────────────────────────────────────────

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

  // ── lifecycle ─────────────────────────────────────────────────────────────────

  onMount(async () => {
    const token = localStorage.getItem('carve_access_token');
    if (!token) {
      phase = 'unauthenticated';
      return;
    }
    await loadSession();
  });

  async function loadSession() {
    phase = 'loading';
    try {
      const res = await fetchReviewSession('ja', 20);
      queue = res.cards;
      sessionStartTime = Date.now();
      advance();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        phase = 'unauthenticated';
      } else {
        errorMessage = err instanceof Error ? err.message : 'Unknown error';
        phase = 'error';
      }
    }
  }

  function advance() {
    if (queue.length === 0) {
      phase = 'done';
      return;
    }
    current = queue.shift()!;
    intervals = null;
    cardStartTime = Date.now();
    phase = 'front';
  }

  async function flip() {
    if (!current || phase !== 'front') return;
    phase = 'back';
    // Load interval previews in background
    try {
      intervals = await fetchIntervals(current.id);
    } catch {
      // non-fatal
    }
  }

  async function rate(rating: 1 | 2 | 3 | 4) {
    if (!current || submitting) return;
    submitting = true;
    const timeTakenMs = Date.now() - cardStartTime;
    try {
      const result = await submitReviewEvent(current.id, rating, timeTakenMs);
      reviewedCount++;
      if (result.is_leech) {
        isLeechNotif = true;
        setTimeout(() => { isLeechNotif = false; }, 4000);
      }
    } catch {
      // non-fatal: continue session even if save fails
    }
    submitting = false;
    advance();
  }

  const RATING_LABELS: Record<number, string> = {
    1: 'Again',
    2: 'Hard',
    3: 'Good',
    4: 'Easy',
  };

  const RATING_COLORS: Record<number, string> = {
    1: '#e57373',
    2: '#ffb74d',
    3: '#4caf50',
    4: '#42a5f5',
  };
</script>

<main>
  <header>
    <h1>Review</h1>
    <nav>
      <a href="/">Home</a>
      <a href="/cards">Cards</a>
    </nav>
  </header>

  {#if isLeechNotif}
    <div class="leech-banner">
      Card suspended — too many lapses (leech detected)
    </div>
  {/if}

  {#if phase === 'loading'}
    <div class="center-msg"><p>Loading session…</p></div>

  {:else if phase === 'unauthenticated'}
    <div class="center-msg">
      <p>You need to log in to review.</p>
      <a href="/login" class="btn">Log in</a>
    </div>

  {:else if phase === 'error'}
    <div class="center-msg error">
      <p>{errorMessage}</p>
      <button class="btn" on:click={loadSession}>Retry</button>
    </div>

  {:else if phase === 'done'}
    <div class="done-screen">
      <p class="done-emoji">✓</p>
      <h2>Session complete</h2>
      <p class="done-sub">{reviewedCount} card{reviewedCount === 1 ? '' : 's'} reviewed</p>
      <div class="done-actions">
        <button class="btn" on:click={loadSession}>Review more</button>
        <a href="/cards" class="btn btn-ghost">View cards</a>
      </div>
    </div>

  {:else if current}
    <div class="progress-bar-wrap">
      <div class="progress-bar" style="width:{Math.max(4, (reviewedCount / (reviewedCount + queue.length + 1)) * 100)}%"></div>
    </div>

    <div class="card-wrap">
      <!-- Card face -->
      <div class="flashcard" class:flipped={phase === 'back'}>
        <div class="card-front">
          <div class="word">{current.front_text}</div>
          {#if current.audio_url}
            <audio src={current.audio_url} preload="auto" id="card-audio"></audio>
            <button class="audio-btn" on:click={playAudio} title="Play audio">
              ▶
            </button>
          {/if}
          {#if phase === 'front'}
            <button class="show-btn" on:click={flip}>Show answer</button>
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
            {#if current.image_url}
              <img class="card-image" src={current.image_url} alt="context screenshot" />
            {/if}
          </div>
        {/if}
      </div>

      <!-- Stats row (shown on back) -->
      {#if phase === 'back' && (current.stability != null || current.difficulty != null)}
        <div class="stats-row">
          <span title="Stability">S: {formatStability(current.stability)}</span>
          <span title="Difficulty">D: {current.difficulty?.toFixed(1) ?? '–'}</span>
          <span title="Lapses">Lapses: {current.lapses}</span>
          <span title="Reviews">Reps: {current.reps}</span>
        </div>
      {/if}

      <!-- Rating buttons (shown on back) -->
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
      {/if}
    </div>
  {/if}
</main>

<style>
  :global(body) {
    background: #13151a;
    color: #e8eaf0;
    font-family: system-ui, sans-serif;
    margin: 0;
  }

  main {
    max-width: 720px;
    margin: 0 auto;
    padding: 1.5rem 1rem;
    min-height: 100vh;
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1.5rem;
    border-bottom: 1px solid #2a2d36;
    padding-bottom: 1rem;
  }

  header h1 { margin: 0; font-size: 1.4rem; }

  nav { display: flex; gap: 1rem; }
  nav a { color: #9ba8c0; text-decoration: none; font-size: 0.9rem; }
  nav a:hover { color: #e8eaf0; }

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

  .progress-bar-wrap {
    background: #1e2128;
    border-radius: 4px;
    height: 4px;
    margin-bottom: 1.5rem;
    overflow: hidden;
  }
  .progress-bar {
    height: 100%;
    background: #4caf50;
    transition: width 0.3s ease;
    border-radius: 4px;
  }

  .card-wrap {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.25rem;
  }

  .flashcard {
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

  .word {
    font-size: 3rem;
    font-weight: 700;
    color: #e8eaf0;
    line-height: 1.2;
  }

  .definition {
    font-size: 1.3rem;
    color: #b0bec5;
    margin: 0;
  }

  .sentence {
    font-size: 0.95rem;
    color: #6b7591;
    font-style: italic;
    margin: 0.5rem 0 0;
    line-height: 1.6;
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
    background: #4caf50;
    border: none;
    color: #fff;
    padding: 0.7rem 2rem;
    border-radius: 8px;
    font-size: 1rem;
    cursor: pointer;
    margin-top: 0.5rem;
    font-weight: 500;
    transition: background 0.15s;
  }
  .show-btn:hover { background: #43a047; }

  .stats-row {
    display: flex;
    gap: 1.25rem;
    font-size: 0.78rem;
    color: #6b7591;
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
    gap: 0.25rem;
    font-size: 0.95rem;
    font-weight: 600;
    transition: background 0.15s;
  }
  .rating-btn:hover:not(:disabled) {
    background: color-mix(in srgb, var(--color) 15%, #1e2128);
  }
  .rating-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .rating-label { font-size: 0.9rem; }
  .interval-hint { font-size: 0.72rem; font-weight: 400; opacity: 0.8; }

  .center-msg {
    text-align: center;
    margin-top: 4rem;
    color: #9ba8c0;
  }
  .center-msg.error { color: #e57373; }

  .done-screen {
    text-align: center;
    margin-top: 5rem;
  }
  .done-emoji { font-size: 3rem; color: #4caf50; margin: 0; }
  .done-screen h2 { font-size: 1.8rem; margin: 0.5rem 0; }
  .done-sub { color: #9ba8c0; margin-bottom: 2rem; }
  .done-actions { display: flex; gap: 1rem; justify-content: center; }

  .btn {
    display: inline-block;
    padding: 0.65rem 1.5rem;
    background: #4caf50;
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
