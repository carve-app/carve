<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { apiFetch } from '$lib/api';
  import { lang } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';

  type State = 'loading' | 'loaded' | 'error';

  interface DiscoverItem {
    id: string;
    source: string;
    title: string;
    summary: string;
    url: string;
    published_at: string;
    comprehension_pct: number;
    unknown_count: number;
    recommended_mode: string;
    fit_score: number;
  }

  let state: State = 'loading';
  let items: DiscoverItem[] = [];
  let error = '';
  let savingId: string | null = null;

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
      const res = await apiFetch(`/v1/discover/feed?language=${language}&limit=20`) as { items: DiscoverItem[] };
      items = res.items ?? [];
      state = 'loaded';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unknown error';
      toasts.add(error);
      state = 'error';
    }
  }

  async function saveToLibrary(item: DiscoverItem) {
    savingId = item.id;
    try {
      await apiFetch('/v1/library', {
        method: 'POST',
        body: JSON.stringify({ url: item.url, language: get(lang) }),
      });
      toasts.add(`Saved: ${item.title}`);
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Failed to save');
    }
    savingId = null;
  }

  function comprehensionColor(pct: number): string {
    if (pct >= 95) return '#4caf50';
    if (pct >= 90) return '#9ccc65';
    if (pct >= 80) return '#ffa726';
    return '#ef5350';
  }

  function modeLabel(mode: string): string {
    switch (mode) {
      case 'flow_read':   return 'flow';
      case 'mining_read': return 'mining';
      case 'study_read':  return 'study';
      default:            return mode;
    }
  }

  function sourceLabel(s: string): string {
    if (s === 'nhk-easy') return 'NHK Easy';
    return s;
  }

  function formatDate(s: string | undefined): string {
    if (!s) return '';
    const d = new Date(s);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
</script>

<svelte:head><title>Discover · Carve</title></svelte:head>

<main>
  <header class="page-head">
    <h1>Discover</h1>
    <p class="subtitle">Fresh content ranked by what you'll find at your i+1 level.</p>
  </header>

  {#if state === 'loading'}
    <p class="msg">Loading recommendations…</p>

  {:else if state === 'error'}
    <p class="msg error">{error}</p>

  {:else if items.length === 0}
    <div class="msg empty">
      <p>No recommendations yet — check back after the next refresh, or grow your known-word list to unlock more sources.</p>
    </div>

  {:else}
    <ul class="feed">
      {#each items as item (item.id)}
        <li class="card">
          <div class="card-main">
            <div class="meta">
              <span class="source">{sourceLabel(item.source)}</span>
              {#if item.published_at}<span class="dot">·</span><span class="date">{formatDate(item.published_at)}</span>{/if}
              <span class="dot">·</span>
              <span class="mode mode-{item.recommended_mode}">{modeLabel(item.recommended_mode)}</span>
            </div>
            <a class="title" href={item.url} target="_blank" rel="noopener">{item.title}</a>
            {#if item.summary}<p class="summary">{item.summary}</p>{/if}
            <div class="actions">
              <button class="btn-save" on:click={() => saveToLibrary(item)} disabled={savingId === item.id}>
                {savingId === item.id ? 'Saving…' : 'Save to Library'}
              </button>
              <a class="btn-open" href={item.url} target="_blank" rel="noopener">Open ↗</a>
            </div>
          </div>
          <div class="score">
            <div class="score-pct" style="color:{comprehensionColor(item.comprehension_pct)}">{Math.round(item.comprehension_pct)}%</div>
            <div class="score-label">comprehension</div>
            <div class="score-unknown">{item.unknown_count} new words</div>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  main { max-width: 800px; margin: 0 auto; padding: 1.5rem 1rem; }
  .page-head { margin-bottom: 1.5rem; }
  .page-head h1 { margin: 0 0 0.3rem; font-size: 1.5rem; }
  .subtitle { margin: 0; color: #9ba8c0; font-size: 0.9rem; }

  .msg { text-align: center; margin-top: 2rem; color: #9ba8c0; }
  .msg.error { color: #ef5350; }
  .msg.empty { border: 1px dashed #2a2d36; border-radius: 10px; padding: 2.5rem; }

  .feed { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.75rem; }

  .card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1rem 1.25rem;
    display: flex;
    gap: 1.25rem;
    align-items: flex-start;
    justify-content: space-between;
  }
  .card-main { flex: 1; min-width: 0; }

  .meta { display: flex; gap: 0.4rem; align-items: center; font-size: 0.75rem; color: #6b7591; margin-bottom: 0.4rem; }
  .source { font-weight: 600; color: #9ba8c0; }
  .dot { opacity: 0.5; }
  .mode { font-weight: 500; padding: 0.05rem 0.45rem; border-radius: 6px; }
  .mode-mining_read { background: rgba(76,175,80,0.13); color: #81c784; }
  .mode-study_read  { background: rgba(255,167,38,0.13); color: #ffa726; }
  .mode-flow_read   { background: rgba(120,160,255,0.13); color: #88aaff; }

  .title { display: block; color: #e8eaf0; text-decoration: none; font-size: 1rem; font-weight: 600; line-height: 1.3; margin-bottom: 0.3rem; }
  .title:hover { color: #4caf50; }
  .summary { margin: 0 0 0.6rem; font-size: 0.85rem; color: #9ba8c0; line-height: 1.45; }

  .actions { display: flex; gap: 0.6rem; margin-top: 0.5rem; }
  .btn-save { background: #4caf50; color: #fff; border: none; padding: 0.4rem 0.85rem; border-radius: 6px; font-size: 0.8rem; cursor: pointer; }
  .btn-save:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-open { background: #2a2d36; color: #c8c8c8; padding: 0.4rem 0.85rem; border-radius: 6px; font-size: 0.8rem; text-decoration: none; }
  .btn-open:hover { background: #353944; }

  .score { text-align: center; min-width: 80px; }
  .score-pct { font-size: 1.5rem; font-weight: 700; line-height: 1.1; }
  .score-label { font-size: 0.7rem; color: #6b7591; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 0.2rem; }
  .score-unknown { font-size: 0.72rem; color: #6b7591; margin-top: 0.2rem; }
</style>
