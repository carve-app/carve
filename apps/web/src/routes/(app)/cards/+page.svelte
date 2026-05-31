<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { fetchCards, type Card } from '$lib/api';
  import { lang } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';

  type State = 'loading' | 'loaded' | 'error';

  let state: State = 'loading';
  let cards: Card[] = [];
  let errorMessage = '';

  const STATE_LABELS: Record<string, string> = {
    new: 'New', learning: 'Learning', review: 'Review', relearning: 'Re-learning',
  };

  function relativeDate(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime();
    const minutes = Math.floor(diff / 60_000);
    if (minutes < 1) return 'just now';
    if (minutes < 60) return `${minutes} min ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days} day${days === 1 ? '' : 's'} ago`;
    const months = Math.floor(days / 30);
    if (months < 12) return `${months} month${months === 1 ? '' : 's'} ago`;
    const years = Math.floor(months / 12);
    return `${years} year${years === 1 ? '' : 's'} ago`;
  }

  onMount(() => {
    let init = false;
    const unsub = lang.subscribe(l => { if (init) load(l); });
    load(get(lang));
    init = true;
    return unsub;
  });

  async function load(language = get(lang)) {
    state = 'loading';
    try {
      const res = await fetchCards(language, 50);
      cards = res.cards;
      state = 'loaded';
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : 'Unknown error';
      toasts.add(errorMessage);
      state = 'error';
    }
  }
</script>

<main>
  {#if state === 'loading'}
    <p class="status">Loading…</p>

  {:else if state === 'error'}
    <div class="prompt error">
      <p>Failed to load cards: {errorMessage}</p>
      <button class="btn" on:click={() => load()}>Retry</button>
    </div>

  {:else if cards.length === 0}
    <div class="empty">
      <p class="empty-title">No cards yet</p>
      <p class="empty-hint">Mine cards with the extension while reading content.</p>
    </div>

  {:else}
    <p class="count">{cards.length} card{cards.length === 1 ? '' : 's'}</p>
    <ul class="card-list">
      {#each cards as card (card.id)}
        <li class="card">
          <a href="/cards/{card.id}" class="card-link">
            <div class="card-top">
              <span class="lemma">{card.front_text}</span>
              <span class="badge badge-{card.fsrs_state}">
                {STATE_LABELS[card.fsrs_state] ?? card.fsrs_state}
              </span>
            </div>
            {#if card.back_text}
              <p class="back-text">{card.back_text}</p>
            {/if}
            {#if card.sentence}
              <p class="sentence">{card.sentence}</p>
            {/if}
            <p class="date">{relativeDate(card.created_at)}</p>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  main { max-width: 760px; margin: 0 auto; padding: 2rem 1rem; }

  .status { color: #9ba8c0; text-align: center; margin-top: 3rem; }

  .prompt { text-align: center; margin-top: 3rem; color: #9ba8c0; }
  .prompt.error { color: #e57373; }

  .btn {
    display: inline-block;
    margin-top: 1rem;
    padding: 0.6rem 1.4rem;
    background: #4caf50;
    color: #fff;
    border-radius: 6px;
    text-decoration: none;
    font-size: 0.95rem;
    font-weight: 500;
    border: none;
    cursor: pointer;
  }
  .btn:hover { background: #43a047; }

  .count { color: #9ba8c0; font-size: 0.85rem; margin-bottom: 1rem; }

  .empty { text-align: center; margin-top: 4rem; }
  .empty-title { font-size: 1.2rem; color: #9ba8c0; margin-bottom: 0.5rem; }
  .empty-hint { color: #6b7591; font-size: 0.9rem; }

  .card-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.75rem; }

  .card {
    background: #1e2128;
    border-radius: 8px;
    border: 1px solid #2a2d36;
    transition: border-color 0.15s;
  }
  .card:hover { border-color: #4a5568; }

  .card-link {
    display: block;
    padding: 1rem 1.25rem;
    text-decoration: none;
    color: inherit;
  }

  .card-top { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.4rem; }

  .lemma { font-size: 1.5rem; font-weight: 600; }

  .badge {
    font-size: 0.72rem;
    font-weight: 600;
    padding: 0.2rem 0.55rem;
    border-radius: 4px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .badge-new        { background: #263851; color: #64b5f6; }
  .badge-learning   { background: #2a3320; color: #aed581; }
  .badge-review     { background: #1e3320; color: #4caf50; }
  .badge-relearning { background: #3a2010; color: #ffb74d; }

  .back-text { color: #b0bec5; font-size: 0.95rem; margin: 0.25rem 0; }

  .sentence { color: #6b7591; font-size: 0.88rem; font-style: italic; margin: 0.35rem 0 0; line-height: 1.5; }

  .date { color: #4a5270; font-size: 0.78rem; margin: 0.5rem 0 0; }
</style>
