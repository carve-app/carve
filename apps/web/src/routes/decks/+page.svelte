<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchDecks, subscribeDeck, unsubscribeDeck, rateDeck, createDeck, updateDeck, deleteDeckById, ApiError, type Deck } from '$lib/api';

  type State = 'loading' | 'loaded' | 'unauthenticated' | 'error';

  let state: State = 'loading';
  let decks: Deck[] = [];
  let errorMessage = '';
  let activeTab: 'browse' | 'mine' = 'browse';
  let subscribing: Record<string, boolean> = {};
  let ratingCard: string | null = null;

  // Deck creation
  let showCreate = false;
  let creating = false;
  let newName = '';
  let newDesc = '';
  let newPublic = false;
  let createError = '';

  // Deck editing
  let editingDeck: string | null = null;
  let editName = '';
  let editDesc = '';
  let editPublic = false;
  let editSaving = false;

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
      const res = await fetchDecks('ja', activeTab === 'mine');
      decks = res.decks ?? [];
      state = 'loaded';
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        state = 'unauthenticated';
      } else {
        errorMessage = err instanceof Error ? err.message : 'Unknown error';
        state = 'error';
      }
    }
  }

  async function switchTab(tab: 'browse' | 'mine') {
    activeTab = tab;
    await load();
  }

  async function toggleSubscribe(deck: Deck) {
    subscribing[deck.id] = true;
    try {
      if (deck.is_subscribed) {
        await unsubscribeDeck(deck.id);
        deck.is_subscribed = false;
        deck.download_count = Math.max(0, deck.download_count - 1);
      } else {
        await subscribeDeck(deck.id);
        deck.is_subscribed = true;
        deck.download_count++;
      }
      decks = [...decks]; // trigger reactivity
    } catch {
      // no-op
    }
    subscribing[deck.id] = false;
  }

  function starRating(n: number): 1 | 2 | 3 | 4 | 5 { return n as 1 | 2 | 3 | 4 | 5; }

  async function submitRating(deckId: string, rating: 1 | 2 | 3 | 4 | 5) {
    try {
      await rateDeck(deckId, rating);
      ratingCard = null;
      await load();
    } catch {
      // no-op
    }
  }

  async function submitCreate() {
    if (!newName.trim()) return;
    creating = true;
    createError = '';
    try {
      await createDeck({ name: newName.trim(), description: newDesc.trim() || undefined, is_public: newPublic });
      newName = '';
      newDesc = '';
      newPublic = false;
      showCreate = false;
      await load();
    } catch (err) {
      createError = err instanceof Error ? err.message : 'Failed to create deck';
    }
    creating = false;
  }

  function startEdit(deck: Deck) {
    editingDeck = deck.id;
    editName = deck.name;
    editDesc = deck.description ?? '';
    editPublic = deck.is_public;
  }

  async function saveEdit(deckId: string) {
    editSaving = true;
    try {
      await updateDeck(deckId, { name: editName, description: editDesc || undefined, is_public: editPublic });
      editingDeck = null;
      await load();
    } catch {
      // no-op
    }
    editSaving = false;
  }

  async function removeDeck(deckId: string) {
    if (!confirm('Delete this deck? This cannot be undone.')) return;
    try {
      await deleteDeckById(deckId);
      await load();
    } catch {
      // no-op
    }
  }
</script>

<main>
  <header>
    <h1>Decks</h1>
    <nav><a href="/">Home</a><a href="/review">Review</a></nav>
  </header>

  <div class="tabs-row">
    <div class="tabs">
      <button class="tab" class:active={activeTab === 'browse'} on:click={() => switchTab('browse')}>Browse</button>
      <button class="tab" class:active={activeTab === 'mine'} on:click={() => switchTab('mine')}>My Decks</button>
    </div>
    {#if activeTab === 'mine' && state === 'loaded'}
      <button class="btn-new" on:click={() => showCreate = !showCreate}>
        {showCreate ? '✕ Cancel' : '+ New Deck'}
      </button>
    {/if}
  </div>

  {#if showCreate && activeTab === 'mine'}
    <div class="create-form">
      <h3>Create Deck</h3>
      <input class="text-input" placeholder="Deck name" bind:value={newName} />
      <input class="text-input" placeholder="Description (optional)" bind:value={newDesc} />
      <label class="check-label">
        <input type="checkbox" bind:checked={newPublic} /> Make public (others can subscribe)
      </label>
      {#if createError}<p class="form-error">{createError}</p>{/if}
      <button class="btn" on:click={submitCreate} disabled={creating || !newName.trim()}>
        {creating ? 'Creating…' : 'Create'}
      </button>
    </div>
  {/if}

  {#if state === 'loading'}
    <p class="msg">Loading…</p>

  {:else if state === 'unauthenticated'}
    <div class="msg"><p>Please <a href="/login">log in</a> to browse decks.</p></div>

  {:else if state === 'error'}
    <div class="msg error"><p>{errorMessage}</p><button class="btn" on:click={load}>Retry</button></div>

  {:else if decks.length === 0}
    <div class="msg"><p>No decks found.</p></div>

  {:else}
    <ul class="deck-list">
      {#each decks as deck (deck.id)}
        <li class="deck-card">
          <div class="deck-top">
            <div>
              <h2 class="deck-name">{deck.name}</h2>
              {#if deck.is_official}
                <span class="badge-official">Official</span>
              {/if}
              {#if deck.description}
                <p class="deck-desc">{deck.description}</p>
              {/if}
            </div>
            <button
              class="sub-btn"
              class:subscribed={deck.is_subscribed}
              disabled={subscribing[deck.id]}
              on:click={() => toggleSubscribe(deck)}
            >
              {deck.is_subscribed ? 'Unsubscribe' : 'Add to Queue'}
            </button>
          </div>

          <div class="deck-meta">
            <span>{deck.card_count} cards</span>
            <span>{deck.download_count} learners</span>
            {#if deck.avg_rating != null}
              <span>★ {deck.avg_rating.toFixed(1)}</span>
            {/if}
            {#if deck.tags?.length}
              <span class="tags">{deck.tags.join(', ')}</span>
            {/if}
          </div>

          {#if deck.is_subscribed && !deck.is_official}
            <div class="rate-row">
              {#if ratingCard === deck.id}
                <span class="rate-label">Rate this deck:</span>
                {#each [1,2,3,4,5] as r}
                  <button class="star" on:click={() => submitRating(deck.id, starRating(r))}>{'★'.repeat(r)}{'☆'.repeat(5-r)}</button>
                {/each}
                <button class="cancel-rate" on:click={() => ratingCard = null}>✕</button>
              {:else}
                <button class="rate-open-btn" on:click={() => ratingCard = deck.id}>Rate deck</button>
              {/if}
            </div>
          {/if}

          {#if activeTab === 'mine'}
            {#if editingDeck === deck.id}
              <div class="edit-form">
                <input class="text-input-sm" bind:value={editName} placeholder="Name" />
                <input class="text-input-sm" bind:value={editDesc} placeholder="Description" />
                <label class="check-label-sm">
                  <input type="checkbox" bind:checked={editPublic} /> Public
                </label>
                <button class="btn-sm" on:click={() => saveEdit(deck.id)} disabled={editSaving}>
                  {editSaving ? '…' : 'Save'}
                </button>
                <button class="btn-ghost-sm" on:click={() => editingDeck = null}>Cancel</button>
              </div>
            {:else}
              <div class="owner-actions">
                <button class="btn-ghost-sm" on:click={() => startEdit(deck)}>Edit</button>
                <button class="btn-danger-sm" on:click={() => removeDeck(deck.id)}>Delete</button>
              </div>
            {/if}
          {/if}
        </li>
      {/each}
    </ul>
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

  .tab { background: #1e2128; border: 1px solid #2a2d36; color: #9ba8c0; padding: 0.5rem 1.2rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem; transition: all 0.15s; }
  .tab.active { background: #4caf50; border-color: #4caf50; color: #fff; }
  .tab:hover:not(.active) { color: #e8eaf0; }

  .msg { text-align: center; margin-top: 3rem; color: #9ba8c0; }
  .msg.error { color: #e57373; }
  .msg a { color: #4caf50; }

  .deck-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 1rem; }

  .deck-card { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.25rem 1.5rem; }

  .deck-top { display: flex; justify-content: space-between; align-items: flex-start; gap: 1rem; margin-bottom: 0.75rem; }

  .deck-name { margin: 0 0 0.2rem; font-size: 1.1rem; font-weight: 600; }
  .badge-official { display: inline-block; font-size: 0.68rem; font-weight: 700; background: #1a2e40; color: #42a5f5; padding: 0.1rem 0.45rem; border-radius: 4px; text-transform: uppercase; letter-spacing: 0.04em; margin-left: 0.4rem; }
  .deck-desc { margin: 0.25rem 0 0; font-size: 0.88rem; color: #9ba8c0; }

  .sub-btn { background: #2a3320; border: 1px solid #4caf50; color: #4caf50; padding: 0.5rem 1rem; border-radius: 7px; cursor: pointer; font-size: 0.88rem; font-weight: 500; white-space: nowrap; transition: background 0.15s; flex-shrink: 0; }
  .sub-btn:hover:not(:disabled) { background: #3a4830; }
  .sub-btn.subscribed { background: #2a2d36; border-color: #9ba8c0; color: #9ba8c0; }
  .sub-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .deck-meta { display: flex; flex-wrap: wrap; gap: 1rem; font-size: 0.8rem; color: #6b7591; }
  .tags { font-style: italic; }

  .rate-row { display: flex; align-items: center; gap: 0.5rem; margin-top: 0.75rem; flex-wrap: wrap; }
  .rate-label { font-size: 0.8rem; color: #9ba8c0; }
  .star { background: none; border: none; color: #ffb74d; cursor: pointer; font-size: 0.85rem; padding: 0.2rem 0.4rem; border-radius: 4px; }
  .star:hover { background: #2a2d36; }
  .cancel-rate { background: none; border: none; color: #6b7591; cursor: pointer; font-size: 0.9rem; }
  .rate-open-btn { background: none; border: 1px solid #3a3d47; color: #9ba8c0; padding: 0.25rem 0.75rem; border-radius: 5px; cursor: pointer; font-size: 0.78rem; }
  .rate-open-btn:hover { background: #2a2d36; }

  .btn { display: inline-block; padding: 0.6rem 1.4rem; background: #4caf50; color: #fff; border-radius: 6px; font-size: 0.9rem; font-weight: 500; border: none; cursor: pointer; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }

  .tabs-row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; }
  .tabs { display: flex; gap: 0.5rem; }
  .btn-new { background: #1e2128; border: 1px solid #4caf50; color: #4caf50; padding: 0.45rem 1rem; border-radius: 6px; cursor: pointer; font-size: 0.88rem; font-weight: 500; transition: background 0.15s; }
  .btn-new:hover { background: #2a3320; }

  .create-form { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.25rem 1.5rem; margin-bottom: 1.5rem; display: flex; flex-direction: column; gap: 0.75rem; }
  .create-form h3 { margin: 0; font-size: 1rem; }
  .text-input { background: #13151a; border: 1px solid #3a3d47; color: #e8eaf0; padding: 0.5rem 0.75rem; border-radius: 6px; font-size: 0.95rem; width: 100%; box-sizing: border-box; }
  .text-input:focus { outline: none; border-color: #4caf50; }
  .check-label { font-size: 0.88rem; color: #9ba8c0; display: flex; align-items: center; gap: 0.4rem; cursor: pointer; }
  .form-error { color: #ef5350; font-size: 0.85rem; margin: 0; }

  .edit-form { display: flex; flex-wrap: wrap; gap: 0.5rem; align-items: center; margin-top: 0.75rem; padding-top: 0.75rem; border-top: 1px solid #2a2d36; }
  .text-input-sm { background: #13151a; border: 1px solid #3a3d47; color: #e8eaf0; padding: 0.35rem 0.6rem; border-radius: 5px; font-size: 0.85rem; flex: 1; min-width: 120px; }
  .check-label-sm { font-size: 0.82rem; color: #9ba8c0; display: flex; align-items: center; gap: 0.3rem; }
  .btn-sm { background: #4caf50; color: #fff; border: none; padding: 0.35rem 0.8rem; border-radius: 5px; font-size: 0.82rem; cursor: pointer; }
  .btn-sm:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-ghost-sm { background: none; border: 1px solid #3a3d47; color: #9ba8c0; padding: 0.3rem 0.7rem; border-radius: 5px; font-size: 0.82rem; cursor: pointer; }
  .btn-ghost-sm:hover { background: #2a2d36; }
  .btn-danger-sm { background: none; border: 1px solid #e57373; color: #e57373; padding: 0.3rem 0.7rem; border-radius: 5px; font-size: 0.82rem; cursor: pointer; }
  .btn-danger-sm:hover { background: #2d1515; }

  .owner-actions { display: flex; gap: 0.5rem; margin-top: 0.75rem; padding-top: 0.75rem; border-top: 1px solid #2a2d36; }
</style>
