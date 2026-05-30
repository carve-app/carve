<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch, ApiError } from '$lib/api';

  type State = 'loading' | 'loaded' | 'unauthenticated' | 'error';

  let state: State = 'loading';
  let items: any[] = [];
  let error = '';
  let adding = false;
  let newUrl = '';
  let addError = '';
  let addSuccess = '';

  onMount(async () => {
    if (!localStorage.getItem('carve_access_token')) {
      state = 'unauthenticated';
      return;
    }
    await load();
  });

  async function load() {
    state = 'loading';
    try {
      const res = await apiFetch('/v1/library?language=ja') as { items: any[] };
      items = res.items ?? [];
      state = 'loaded';
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) { state = 'unauthenticated'; }
      else { error = err instanceof Error ? err.message : 'Unknown error'; state = 'error'; }
    }
  }

  async function addUrl() {
    if (!newUrl.trim()) return;
    adding = true;
    addError = '';
    addSuccess = '';
    try {
      const item = await apiFetch('/v1/library', {
        method: 'POST',
        body: JSON.stringify({ url: newUrl.trim(), language: 'ja' }),
      }) as any;
      newUrl = '';
      addSuccess = `Added: ${item.title ?? item.url}`;
      setTimeout(() => { addSuccess = ''; }, 3000);
      await load();
    } catch (err) {
      addError = err instanceof Error ? err.message : 'Failed to add URL';
    }
    adding = false;
  }

  async function removeItem(id: string) {
    try {
      await apiFetch(`/v1/library/${id}`, { method: 'DELETE' });
      items = items.filter(i => i.id !== id);
    } catch { /* no-op */ }
  }

  function comprehensionColor(pct: number | null): string {
    if (pct == null) return '#9ba8c0';
    if (pct >= 95) return '#4caf50';
    if (pct >= 80) return '#ffa726';
    return '#ef5350';
  }
</script>

<main>
  <header>
    <h1>Library</h1>
    <nav>
      <a href="/">Home</a>
      <a href="/stats">Stats</a>
      <a href="/review">Review</a>
    </nav>
  </header>

  {#if state === 'unauthenticated'}
    <div class="msg"><p>Please <a href="/login">log in</a> to use the library.</p></div>

  {:else}
    <div class="add-section">
      <h2>Save a URL</h2>
      <p class="add-hint">Paste any Japanese web page URL to score its comprehension and add it to your reading list.</p>
      <div class="add-row">
        <input
          class="url-input"
          type="url"
          placeholder="https://nhk.or.jp/…"
          bind:value={newUrl}
          on:keydown={(e) => { if (e.key === 'Enter') addUrl(); }}
        />
        <button class="btn" on:click={addUrl} disabled={adding || !newUrl.trim()}>
          {adding ? 'Scoring…' : 'Add'}
        </button>
      </div>
      {#if addError}<p class="error">{addError}</p>{/if}
      {#if addSuccess}<p class="success">{addSuccess}</p>{/if}
    </div>

    {#if state === 'loading'}
      <p class="msg">Loading…</p>

    {:else if state === 'error'}
      <p class="msg error">{error}</p>

    {:else if items.length === 0}
      <div class="msg empty">
        <p>Your library is empty. Add a URL above to get started.</p>
      </div>

    {:else}
      <ul class="item-list">
        {#each items as item (item.id)}
          <li class="item-card">
            <div class="item-main">
              <div class="item-top">
                <a href={item.url} target="_blank" rel="noopener" class="item-title">{item.title ?? item.url}</a>
                <button class="remove-btn" on:click={() => removeItem(item.id)} title="Remove">✕</button>
              </div>
              {#if item.url}
                <div class="item-url">{item.url}</div>
              {/if}
            </div>
            <div class="item-score">
              {#if item.comprehension_pct != null}
                <div class="score-pct" style="color:{comprehensionColor(item.comprehension_pct)}">
                  {Math.round(item.comprehension_pct)}%
                </div>
                <div class="score-label">comprehension</div>
                {#if item.unknown_word_count != null}
                  <div class="score-unknown">{item.unknown_word_count} unknown</div>
                {/if}
              {:else}
                <div class="score-pct" style="color:#6b7591">—</div>
                <div class="score-label">not scored</div>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

<style>
  :global(body) { background: #13151a; color: #e8eaf0; font-family: system-ui, sans-serif; margin: 0; }
  main { max-width: 760px; margin: 0 auto; padding: 1.5rem 1rem; }

  header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1.5rem; border-bottom: 1px solid #2a2d36; padding-bottom: 1rem; }
  header h1 { margin: 0; font-size: 1.4rem; }
  nav { display: flex; gap: 1rem; }
  nav a { color: #9ba8c0; text-decoration: none; font-size: 0.9rem; }
  nav a:hover { color: #e8eaf0; }

  .msg { text-align: center; margin-top: 2rem; color: #9ba8c0; }
  .msg.error { color: #ef5350; }
  .msg.empty { border: 1px dashed #2a2d36; border-radius: 10px; padding: 2.5rem; }
  .msg a { color: #4caf50; }

  .add-section {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1.25rem 1.5rem;
    margin-bottom: 1.5rem;
  }
  .add-section h2 { margin: 0 0 0.4rem; font-size: 1rem; }
  .add-hint { font-size: 0.83rem; color: #6b7591; margin-bottom: 1rem; }
  .add-row { display: flex; gap: 0.75rem; }
  .url-input {
    flex: 1;
    background: #13151a;
    border: 1px solid #2a2d36;
    border-radius: 7px;
    color: #e8eaf0;
    padding: 0.55rem 0.8rem;
    font-size: 0.92rem;
    outline: none;
  }
  .url-input:focus { border-color: #4caf50; }
  .btn { background: #4caf50; color: #fff; border: none; padding: 0.55rem 1.2rem; border-radius: 7px; font-size: 0.92rem; font-weight: 500; cursor: pointer; white-space: nowrap; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }
  .error { color: #ef5350; font-size: 0.85rem; margin-top: 0.5rem; }
  .success { color: #4caf50; font-size: 0.85rem; margin-top: 0.5rem; }

  .item-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.75rem; }

  .item-card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1rem 1.25rem;
    display: flex;
    align-items: flex-start;
    gap: 1.25rem;
    justify-content: space-between;
  }

  .item-main { flex: 1; min-width: 0; }
  .item-top { display: flex; justify-content: space-between; align-items: flex-start; gap: 0.75rem; margin-bottom: 0.25rem; }
  .item-title {
    color: #e8eaf0;
    text-decoration: none;
    font-size: 0.95rem;
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .item-title:hover { color: #4caf50; }
  .item-url { font-size: 0.75rem; color: #6b7591; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .remove-btn { background: none; border: none; color: #6b7591; cursor: pointer; font-size: 0.9rem; padding: 0; flex-shrink: 0; }
  .remove-btn:hover { color: #ef5350; }

  .item-score { text-align: center; min-width: 70px; }
  .score-pct { font-size: 1.5rem; font-weight: 700; line-height: 1.1; }
  .score-label { font-size: 0.7rem; color: #6b7591; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 0.2rem; }
  .score-unknown { font-size: 0.72rem; color: #6b7591; margin-top: 0.2rem; }
</style>
