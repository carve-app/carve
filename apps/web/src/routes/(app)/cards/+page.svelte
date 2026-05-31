<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { fetchCardsFiltered, bulkCards, type Card, type FsrsState } from '$lib/api';
  import { lang } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';

  type PageState = 'loading' | 'loaded' | 'error';

  let state: PageState = 'loading';
  let cards: Card[] = [];
  let total = 0;
  let errorMessage = '';

  // Filters
  let search = '';
  let filterState: FsrsState | '' = '';
  let filterSuspended: 'all' | 'suspended' | 'active' = 'all';
  let filterLeech: 'all' | 'leech' | 'normal' = 'all';
  let sortBy: 'created' | 'due' | 'lapses' | 'alpha' | 'last_reviewed' = 'created';
  let searchDebounce: ReturnType<typeof setTimeout> | null = null;

  // Bulk selection
  let selected = new Set<string>();
  let bulkAction: 'suspend' | 'unsuspend' | 'bury' | 'delete' = 'suspend';
  let bulkRunning = false;

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
    const unsub = lang.subscribe(l => { if (init) load(); });
    load();
    init = true;
    return unsub;
  });

  async function load() {
    state = 'loading';
    selected = new Set();
    try {
      const res = await fetchCardsFiltered({
        language: get(lang),
        limit: 100,
        search: search || undefined,
        state: filterState || undefined,
        suspended: filterSuspended === 'suspended' ? true : filterSuspended === 'active' ? false : null,
        is_leech: filterLeech === 'leech' ? true : filterLeech === 'normal' ? false : null,
        sort: sortBy,
      });
      cards = res.cards;
      total = res.total;
      state = 'loaded';
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : 'Unknown error';
      toasts.add(errorMessage);
      state = 'error';
    }
  }

  function onSearchInput() {
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(() => load(), 300);
  }

  function toggleSelect(id: string) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
  }

  function toggleSelectAll() {
    if (selected.size === cards.length) {
      selected = new Set();
    } else {
      selected = new Set(cards.map(c => c.id));
    }
  }

  async function runBulk() {
    if (selected.size === 0 || bulkRunning) return;
    bulkRunning = true;
    try {
      const result = await bulkCards(bulkAction, Array.from(selected));
      toasts.add(`${result.affected} card${result.affected === 1 ? '' : 's'} ${bulkAction}ed`);
      await load();
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Bulk action failed');
    }
    bulkRunning = false;
  }
</script>

<main>
  <!-- ── Toolbar ── -->
  <div class="toolbar">
    <input
      class="search-input"
      type="search"
      bind:value={search}
      on:input={onSearchInput}
      placeholder="Search cards…"
    />
    <select class="filter-select" bind:value={filterState} on:change={load}>
      <option value="">All states</option>
      <option value="new">New</option>
      <option value="learning">Learning</option>
      <option value="review">Review</option>
      <option value="relearning">Re-learning</option>
    </select>
    <select class="filter-select" bind:value={filterSuspended} on:change={load}>
      <option value="all">All</option>
      <option value="active">Active</option>
      <option value="suspended">Suspended</option>
    </select>
    <select class="filter-select" bind:value={filterLeech} on:change={load}>
      <option value="all">All</option>
      <option value="normal">Not leech</option>
      <option value="leech">Leech</option>
    </select>
    <select class="filter-select" bind:value={sortBy} on:change={load}>
      <option value="created">Newest</option>
      <option value="due">Due soonest</option>
      <option value="lapses">Most lapses</option>
      <option value="alpha">A–Z</option>
      <option value="last_reviewed">Last reviewed</option>
    </select>
  </div>

  <!-- ── Bulk actions bar ── -->
  {#if selected.size > 0}
    <div class="bulk-bar">
      <span class="bulk-count">{selected.size} selected</span>
      <select class="filter-select" bind:value={bulkAction}>
        <option value="suspend">Suspend</option>
        <option value="unsuspend">Unsuspend</option>
        <option value="bury">Bury</option>
        <option value="delete">Delete</option>
      </select>
      <button class="btn-bulk" on:click={runBulk} disabled={bulkRunning}>
        {bulkRunning ? 'Running…' : 'Apply'}
      </button>
      <button class="btn-bulk btn-bulk-ghost" on:click={() => { selected = new Set(); }}>Clear</button>
    </div>
  {/if}

  {#if state === 'loading'}
    <p class="status">Loading…</p>

  {:else if state === 'error'}
    <div class="prompt error">
      <p>Failed to load cards: {errorMessage}</p>
      <button class="btn" on:click={load}>Retry</button>
    </div>

  {:else if cards.length === 0}
    <div class="empty">
      <p class="empty-title">{search || filterState || filterSuspended !== 'all' || filterLeech !== 'all' ? 'No cards match' : 'No cards yet'}</p>
      {#if !search && !filterState && filterSuspended === 'all' && filterLeech === 'all'}
        <p class="empty-hint">Mine cards with the extension while reading content.</p>
      {/if}
    </div>

  {:else}
    <div class="count-bar">
      <label class="select-all-label">
        <input
          type="checkbox"
          checked={selected.size === cards.length && cards.length > 0}
          on:change={toggleSelectAll}
        />
        <span>{total} card{total === 1 ? '' : 's'}</span>
      </label>
    </div>

    <ul class="card-list">
      {#each cards as card (card.id)}
        <li class="card" class:selected={selected.has(card.id)} class:suspended={card.suspended}>
          <label class="card-check">
            <input type="checkbox" checked={selected.has(card.id)} on:change={() => toggleSelect(card.id)} />
          </label>
          <a href="/cards/{card.id}" class="card-link">
            <div class="card-top">
              <span class="lemma">{card.front_text}</span>
              <div class="badge-row">
                <span class="badge badge-{card.fsrs_state}">
                  {STATE_LABELS[card.fsrs_state] ?? card.fsrs_state}
                </span>
                {#if card.suspended}
                  <span class="badge badge-suspended">Suspended</span>
                {/if}
                {#if card.is_leech}
                  <span class="badge badge-leech">Leech</span>
                {/if}
              </div>
            </div>
            {#if card.back_text}
              <p class="back-text">{card.back_text}</p>
            {/if}
            {#if card.sentence}
              <p class="sentence">{card.sentence}</p>
            {/if}
            {#if card.tags && card.tags.length > 0}
              <div class="tag-row">
                {#each card.tags as tag}
                  <span class="tag">{tag}</span>
                {/each}
              </div>
            {/if}
            <p class="date">{relativeDate(card.created_at)}</p>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  main { max-width: 760px; margin: 0 auto; padding: 1.5rem 1rem 3rem; }

  /* ── Toolbar ── */
  .toolbar {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-bottom: 1rem;
    align-items: center;
  }
  .search-input {
    flex: 1;
    min-width: 180px;
    background: #1e2128;
    border: 1px solid #2a2d36;
    color: #c8d0e0;
    padding: 0.45rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    transition: border-color 0.15s;
  }
  .search-input:focus { outline: none; border-color: #4caf50; }
  .filter-select {
    background: #1e2128;
    border: 1px solid #2a2d36;
    color: #9ba8c0;
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
  }
  .filter-select:focus { outline: none; border-color: #4caf50; }

  /* ── Bulk bar ── */
  .bulk-bar {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a2038;
    border: 1px solid #263060;
    border-radius: 8px;
    padding: 0.5rem 0.9rem;
    margin-bottom: 0.75rem;
    flex-wrap: wrap;
  }
  .bulk-count { color: #64b5f6; font-size: 0.88rem; font-weight: 600; }
  .btn-bulk {
    background: #4caf50;
    border: none;
    color: #fff;
    padding: 0.35rem 0.9rem;
    border-radius: 5px;
    font-size: 0.82rem;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-bulk:hover:not(:disabled) { background: #43a047; }
  .btn-bulk:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-bulk-ghost { background: transparent; border: 1px solid #2a2d36; color: #9ba8c0; }
  .btn-bulk-ghost:hover:not(:disabled) { background: #2a2d36; }

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

  .empty { text-align: center; margin-top: 4rem; }
  .empty-title { font-size: 1.2rem; color: #9ba8c0; margin-bottom: 0.5rem; }
  .empty-hint { color: #6b7591; font-size: 0.9rem; }

  /* ── Count bar ── */
  .count-bar { margin-bottom: 0.5rem; }
  .select-all-label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: #9ba8c0;
    font-size: 0.85rem;
    cursor: pointer;
    user-select: none;
  }

  /* ── Card list ── */
  .card-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.75rem; }

  .card {
    background: #1e2128;
    border-radius: 8px;
    border: 1px solid #2a2d36;
    transition: border-color 0.15s;
    display: flex;
    align-items: stretch;
  }
  .card:hover { border-color: #4a5568; }
  .card.selected { border-color: #4caf50; background: #1a2420; }
  .card.suspended { opacity: 0.6; }

  .card-check {
    display: flex;
    align-items: center;
    padding: 0 0.5rem 0 0.75rem;
    cursor: pointer;
  }

  .card-link {
    flex: 1;
    display: block;
    padding: 1rem 1.25rem 1rem 0.25rem;
    text-decoration: none;
    color: inherit;
  }

  .card-top { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.4rem; flex-wrap: wrap; }
  .lemma { font-size: 1.5rem; font-weight: 600; }

  .badge-row { display: flex; gap: 0.35rem; flex-wrap: wrap; }
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
  .badge-suspended  { background: #2d2000; color: #ffc107; }
  .badge-leech      { background: #2a0a0a; color: #e57373; }

  .back-text { color: #b0bec5; font-size: 0.95rem; margin: 0.25rem 0; }
  .sentence { color: #6b7591; font-size: 0.88rem; font-style: italic; margin: 0.35rem 0 0; line-height: 1.5; }
  .date { color: #4a5270; font-size: 0.78rem; margin: 0.5rem 0 0; }

  .tag-row { display: flex; gap: 0.35rem; flex-wrap: wrap; margin-top: 0.35rem; }
  .tag {
    background: #1a2040;
    color: #64b5f6;
    font-size: 0.68rem;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    border: 1px solid #263050;
  }
</style>
