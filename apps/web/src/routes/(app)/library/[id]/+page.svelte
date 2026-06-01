<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { fetchLibraryReader, apiFetch, type ReaderToken, type ReaderUnknownWord } from '$lib/api';
  import { toasts } from '$lib/stores/toast';

  let loading = true;
  let title = '';
  let tokens: ReaderToken[] = [];
  let comprehensionPct: number | null = null;
  let unknownWords: ReaderUnknownWord[] = [];
  let language = 'ja';

  // Popup state
  let popupWord: { lemma: string; reading: string; entry: any | null } | null = null;
  let popupEl: HTMLElement | null = null;
  let lookupLoading = false;

  // Mining state
  let miningLemma: string | null = null;
  let mineSuccess: string | null = null;

  onMount(async () => {
    const id = $page.params.id ?? '';
    try {
      const data = await fetchLibraryReader(id);
      title = data.title;
      tokens = data.tokens;
      comprehensionPct = data.comprehension_pct;
      unknownWords = data.unknown_words;
      language = data.language;
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Failed to load reader');
      goto('/library');
    }
    loading = false;
  });

  async function showPopup(tok: ReaderToken, event: MouseEvent) {
    if (popupWord?.lemma === tok.lemma) {
      popupWord = null;
      return;
    }
    popupWord = { lemma: tok.lemma, reading: tok.reading_hira, entry: null };
    lookupLoading = true;

    // Position popup near the clicked token
    if (popupEl) {
      const target = event.currentTarget as HTMLElement;
      const rect = target.getBoundingClientRect();
      const above = rect.top > 220;
      popupEl.style.top = above
        ? `${rect.top + window.scrollY - 220}px`
        : `${rect.bottom + window.scrollY + 8}px`;
      popupEl.style.left = `${Math.min(rect.left, window.innerWidth - 340)}px`;
    }

    try {
      const res = await apiFetch<{ entry: any }>('/v1/nlp/lookup', {
        method: 'POST',
        body: JSON.stringify({ surface: tok.lemma, language }),
      });
      if (popupWord?.lemma === tok.lemma) {
        popupWord = { ...popupWord, entry: res.entry };
      }
    } catch {
      // lookup is optional
    }
    lookupLoading = false;
  }

  async function mineWord(lemma: string, reading: string) {
    miningLemma = lemma;
    try {
      const def = popupWord?.entry?.definitions?.[0]?.definition ?? '';
      await apiFetch('/v1/cards', {
        method: 'POST',
        body: JSON.stringify({
          language_code: language,
          lemma,
          reading,
          back_text: def || undefined,
          source_url: typeof window !== 'undefined' ? window.location.href : undefined,
        }),
      });
      mineSuccess = lemma;
      setTimeout(() => { mineSuccess = null; }, 2500);
      // Update token statuses locally
      tokens = tokens.map(t =>
        t.lemma === lemma ? { ...t, user_status: 'learning' } : t,
      );
      unknownWords = unknownWords.filter(w => w.lemma !== lemma);
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Mine failed');
    }
    miningLemma = null;
  }

  async function prelearnAll() {
    const top = unknownWords.slice(0, 12);
    for (const w of top) {
      try {
        await apiFetch('/v1/cards', {
          method: 'POST',
          body: JSON.stringify({ language_code: language, lemma: w.lemma, reading: w.reading }),
        });
        tokens = tokens.map(t => t.lemma === w.lemma ? { ...t, user_status: 'learning' } : t);
      } catch { /* skip duplicate */ }
    }
    unknownWords = unknownWords.filter(w => !top.find(t => t.lemma === w.lemma));
    toasts.add(`Added ${top.length} cards to review`);
  }

  function statusClass(status: string) {
    if (status === 'unknown') return 'tok-unknown';
    if (status === 'learning') return 'tok-learning';
    if (status === 'known') return 'tok-known';
    return 'tok-func';
  }

  function closePopup() { popupWord = null; }
</script>

<svelte:window on:keydown={(e) => { if (e.key === 'Escape') closePopup(); }} />

<div class="reader-layout">
  <!-- Sidebar: unknown words -->
  <aside class="sidebar">
    <div class="sidebar-head">
      <h3>Unknown words</h3>
      {#if comprehensionPct != null}
        <span class="comp-badge" class:comp-high={comprehensionPct >= 95} class:comp-mid={comprehensionPct >= 80}>
          {Math.round(comprehensionPct)}%
        </span>
      {/if}
    </div>

    {#if unknownWords.length > 0}
      <button class="prelearn-btn" on:click={prelearnAll}>
        Pre-learn top {Math.min(12, unknownWords.length)} words
      </button>

      <ul class="word-list">
        {#each unknownWords.slice(0, 50) as w (w.lemma)}
          <li class="word-item">
            <div class="word-text">{w.lemma}</div>
            <div class="word-reading">{w.reading}</div>
            <button
              class="mine-small"
              disabled={miningLemma === w.lemma}
              on:click={() => mineWord(w.lemma, w.reading)}
            >+</button>
          </li>
        {/each}
      </ul>
    {:else if !loading}
      <p class="no-unknown">No unknown words — great comprehension!</p>
    {/if}
  </aside>

  <!-- Main reader -->
  <main class="reader-main">
    <div class="reader-header">
      <a href="/library" class="back">← Library</a>
      {#if title}<h1 class="reader-title">{title}</h1>{/if}
    </div>

    {#if loading}
      <div class="loading-msg">Tokenizing text…</div>
    {:else}
      <!-- The reader text is a navigable document; tokens themselves are buttons.
           Click-outside-to-close is handled by the popup's own backdrop. -->
      <div class="reader-text">
        {#each tokens as tok (tok.surface + tok.lemma)}
          {#if tok.is_content_word}
            <button
              type="button"
              class="tok {statusClass(tok.user_status)}"
              title={tok.reading_hira}
              on:click={(e) => showPopup(tok, e)}
            >{tok.surface}</button>
          {:else}
            <span class="tok tok-func">{tok.surface}</span>
          {/if}
        {/each}
      </div>
    {/if}

    {#if mineSuccess}
      <div class="mine-toast">✓ Saved "{mineSuccess}"</div>
    {/if}
  </main>
</div>

<!-- Word popup -->
{#if popupWord}
  <div class="popup" bind:this={popupEl}>
    <div class="popup-word">{popupWord.lemma}</div>
    <div class="popup-reading">{popupWord.reading}</div>
    {#if lookupLoading}
      <div class="popup-loading">Looking up…</div>
    {:else if popupWord.entry?.definitions?.length}
      <div class="popup-defs">
        {#each popupWord.entry.definitions.slice(0, 3) as d}
          <div class="popup-def">
            <span class="popup-pos">{d.pos}</span> {d.definition}
          </div>
        {/each}
      </div>
    {:else}
      <div class="popup-def popup-notfound">Not found in dictionary</div>
    {/if}
    <div class="popup-actions">
      <button
        class="popup-mine"
        disabled={miningLemma === popupWord.lemma}
        on:click={() => { const pw = popupWord; if (pw) mineWord(pw.lemma, pw.reading); }}
      >
        {miningLemma === popupWord.lemma ? 'Saving…' : 'Mine'}
      </button>
      <button class="popup-close" on:click={closePopup}>✕</button>
    </div>
  </div>
{/if}

<style>
  .reader-layout {
    display: grid;
    grid-template-columns: 220px 1fr;
    gap: 0;
    min-height: calc(100vh - 60px);
    max-width: 1100px;
    margin: 0 auto;
  }

  .sidebar {
    padding: 1.5rem 1rem 2rem;
    border-right: 1px solid #1e2128;
    position: sticky;
    top: 60px;
    height: calc(100vh - 60px);
    overflow-y: auto;
  }

  .sidebar-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.75rem;
  }

  .sidebar-head h3 { margin: 0; font-size: 0.82rem; color: #9ba8c0; text-transform: uppercase; letter-spacing: 0.06em; }

  .comp-badge {
    font-size: 0.82rem;
    font-weight: 700;
    padding: 0.1rem 0.45rem;
    border-radius: 4px;
    background: #2d1515;
    color: #ef5350;
  }
  .comp-badge.comp-mid { background: #2a2010; color: #ffa726; }
  .comp-badge.comp-high { background: #1a2d1a; color: #4caf50; }

  .prelearn-btn {
    width: 100%;
    background: #1a2d1a;
    border: 1px solid #2a3d2a;
    color: #4caf50;
    padding: 0.4rem 0.75rem;
    border-radius: 6px;
    font-size: 0.78rem;
    cursor: pointer;
    margin-bottom: 0.75rem;
    text-align: center;
  }
  .prelearn-btn:hover { background: #22382a; }

  .word-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.1rem; }

  .word-item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0;
    border-bottom: 1px solid #1e2128;
  }

  .word-text { font-size: 0.9rem; color: #e8eaf0; flex: 1; }
  .word-reading { font-size: 0.7rem; color: #6b7591; min-width: 40px; }

  .mine-small {
    background: none;
    border: 1px solid #2a3d2a;
    color: #4caf50;
    border-radius: 4px;
    width: 22px;
    height: 22px;
    font-size: 0.9rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .mine-small:disabled { opacity: 0.4; cursor: not-allowed; }

  .no-unknown { font-size: 0.8rem; color: #4a5568; }

  .reader-main { padding: 1.5rem 2rem 3rem; }

  .reader-header { display: flex; align-items: baseline; gap: 1.25rem; margin-bottom: 1.5rem; }
  .back { color: #6b7591; text-decoration: none; font-size: 0.85rem; flex-shrink: 0; }
  .back:hover { color: #e8eaf0; }
  .reader-title { margin: 0; font-size: 1rem; color: #c8d0e0; font-weight: 500; }

  .loading-msg { color: #6b7591; margin-top: 2rem; }

  .reader-text {
    font-size: 1.1rem;
    line-height: 2;
    color: #c8d0e0;
    word-break: break-word;
  }

  .tok { cursor: default; background: transparent; border: none; color: inherit; font: inherit; padding: 0; }
  button.tok { display: inline; }
  .tok-unknown { color: #ef9a9a; cursor: pointer; border-bottom: 1px dashed #ef5350; }
  .tok-unknown:hover { background: rgba(239,83,80,0.1); border-radius: 2px; }
  .tok-learning { color: #ffa726; cursor: pointer; }
  .tok-learning:hover { background: rgba(255,167,38,0.1); border-radius: 2px; }
  .tok-known { color: #c8d0e0; cursor: pointer; }
  .tok-known:hover { background: rgba(255,255,255,0.04); border-radius: 2px; }
  .tok-func { color: #7a8aa6; }

  .mine-toast {
    position: fixed;
    bottom: 2rem;
    left: 50%;
    transform: translateX(-50%);
    background: #1a2d1a;
    border: 1px solid #2a3d2a;
    color: #4caf50;
    padding: 0.5rem 1.25rem;
    border-radius: 8px;
    font-size: 0.9rem;
    z-index: 1000;
    pointer-events: none;
  }

  /* Popup */
  .popup {
    position: absolute;
    z-index: 10000;
    background: #13151a;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 0.75rem 1rem;
    min-width: 240px;
    max-width: 340px;
    box-shadow: 0 8px 32px rgba(0,0,0,0.5);
  }

  .popup-word { font-size: 1.5rem; font-weight: 600; color: #e8eaf0; }
  .popup-reading { font-size: 0.8rem; color: #9ba8c0; margin-bottom: 0.5rem; }
  .popup-loading { color: #6b7591; font-size: 0.85rem; }
  .popup-defs { display: flex; flex-direction: column; gap: 0.25rem; margin-bottom: 0.5rem; }
  .popup-def { font-size: 0.85rem; color: #c8d0e0; }
  .popup-pos { font-size: 0.7rem; color: #6b7591; background: #1e2128; padding: 0.05rem 0.3rem; border-radius: 3px; margin-right: 0.3rem; }
  .popup-notfound { color: #6b7591; }
  .popup-actions { display: flex; gap: 0.5rem; margin-top: 0.5rem; }
  .popup-mine {
    flex: 1;
    background: #1a2d1a;
    border: 1px solid #2a3d2a;
    color: #4caf50;
    padding: 0.3rem 0.75rem;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
  }
  .popup-mine:disabled { opacity: 0.5; cursor: not-allowed; }
  .popup-close {
    background: none;
    border: 1px solid #2a2d36;
    color: #6b7591;
    padding: 0.3rem 0.5rem;
    border-radius: 6px;
    font-size: 0.78rem;
    cursor: pointer;
  }

  @media (max-width: 640px) {
    .reader-layout { grid-template-columns: 1fr; }
    .sidebar { position: static; height: auto; border-right: none; border-bottom: 1px solid #1e2128; }
  }
</style>
