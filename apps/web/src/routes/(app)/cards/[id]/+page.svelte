<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { fetchCard, apiFetch, updateCard, suspendCard, unsuspendCard, buryCard, type CardDetail } from '$lib/api';
  import { toasts } from '$lib/stores/toast';

  let card: CardDetail | null = null;
  let loading = true;
  let deleting = false;
  let editing = false;
  let saving = false;
  let suspending = false;

  // Edit form state
  let editFrontText = '';
  let editFrontReading = '';
  let editBackText = '';
  let editSentence = '';
  let editTranslation = '';
  let editNotes = '';
  let editTagsRaw = '';

  const STATE_LABELS: Record<string, string> = {
    new: 'New', learning: 'Learning', review: 'Review', relearning: 'Re-learning',
  };

  onMount(async () => {
    const id = $page.params.id ?? '';
    try {
      card = await fetchCard(id);
    } catch {
      toasts.add('Card not found');
      goto('/cards');
    }
    loading = false;
  });

  function startEdit() {
    if (!card) return;
    editFrontText = card.front_text;
    editFrontReading = '';
    editBackText = card.back_text ?? '';
    editSentence = card.sentence ?? '';
    editTranslation = card.subtitle_translation ?? '';
    editNotes = card.notes ?? '';
    editTagsRaw = (card.tags ?? []).join(', ');
    editing = true;
  }

  async function saveEdit() {
    if (!card || saving) return;
    saving = true;
    const tags = editTagsRaw
      .split(',')
      .map(t => t.trim())
      .filter(Boolean);

    const patch: Record<string, unknown> = {};
    if (editFrontText !== card.front_text) patch.front_text = editFrontText;
    if (editFrontReading) patch.front_reading = editFrontReading;
    if (editBackText !== (card.back_text ?? '')) patch.back_text = editBackText;
    if (editSentence !== (card.sentence ?? '')) patch.sentence = editSentence;
    if (editTranslation !== (card.subtitle_translation ?? '')) patch.subtitle_translation = editTranslation;
    if (editNotes !== (card.notes ?? '')) patch.notes = editNotes;
    // Always send tags when editing
    patch.tags = tags;

    try {
      await updateCard(card.id, patch);
      card = await fetchCard(card.id);
      editing = false;
      toasts.add('Card saved');
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Save failed');
    }
    saving = false;
  }

  async function deleteCard() {
    if (!card) return;
    deleting = true;
    try {
      await apiFetch(`/v1/cards/${card.id}`, { method: 'DELETE' });
      goto('/cards');
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Delete failed');
      deleting = false;
    }
  }

  async function toggleSuspend() {
    if (!card || suspending) return;
    suspending = true;
    try {
      if (card.suspended) {
        await unsuspendCard(card.id);
        card = { ...card, suspended: false };
        toasts.add('Card unsuspended');
      } else {
        await suspendCard(card.id);
        card = { ...card, suspended: true };
        toasts.add('Card suspended');
      }
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Failed');
    }
    suspending = false;
  }

  async function doBury() {
    if (!card) return;
    try {
      await buryCard(card.id);
      card = { ...card, fsrs_state: card.fsrs_state };
      toasts.add('Card buried until tomorrow');
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Failed to bury');
    }
  }

  function playAudio() {
    (document.getElementById('detail-audio') as HTMLAudioElement | null)?.play();
  }

  function formatMs(ms: number | null): string {
    if (ms == null) return '—';
    const s = Math.floor(ms / 1000);
    const m = Math.floor(s / 60);
    const rem = s % 60;
    return `${m}:${String(rem).padStart(2, '0')}`;
  }

  function relativeDate(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime();
    const days = Math.floor(diff / 86_400_000);
    if (days === 0) return 'today';
    if (days === 1) return 'yesterday';
    if (days < 30) return `${days} days ago`;
    return new Date(iso).toLocaleDateString();
  }
</script>

<main>
  {#if loading}
    <p class="msg">Loading…</p>

  {:else if card}
    <div class="header">
      <a href="/cards" class="back">← Cards</a>
      <span class="badge badge-{card.fsrs_state}">{STATE_LABELS[card.fsrs_state] ?? card.fsrs_state}</span>
      {#if card.suspended}
        <span class="badge badge-suspended">Suspended</span>
      {/if}
      {#if card.is_leech}
        <span class="badge badge-leech">Leech</span>
      {/if}
    </div>

    {#if editing}
      <!-- ── Edit mode ── -->
      <div class="edit-form">
        <div class="edit-title">Edit card</div>
        <div class="edit-field">
          <label class="edit-label" for="ed-word">Word</label>
          <input id="ed-word" class="edit-input" bind:value={editFrontText} />
        </div>
        <div class="edit-field">
          <label class="edit-label" for="ed-reading">Reading <span class="optional">(leave blank to keep current)</span></label>
          <input id="ed-reading" class="edit-input" bind:value={editFrontReading} placeholder="e.g. ねこ" />
        </div>
        <div class="edit-field">
          <label class="edit-label" for="ed-def">Definition</label>
          <input id="ed-def" class="edit-input" bind:value={editBackText} />
        </div>
        <div class="edit-field">
          <label class="edit-label" for="ed-sentence">Sentence</label>
          <textarea id="ed-sentence" class="edit-input edit-textarea" bind:value={editSentence} rows="3"></textarea>
        </div>
        <div class="edit-field">
          <label class="edit-label" for="ed-translation">Translation</label>
          <input id="ed-translation" class="edit-input" bind:value={editTranslation} />
        </div>
        <div class="edit-field">
          <label class="edit-label" for="ed-notes">Notes</label>
          <textarea id="ed-notes" class="edit-input edit-textarea" bind:value={editNotes} rows="2"></textarea>
        </div>
        <div class="edit-field">
          <label class="edit-label" for="ed-tags">Tags <span class="optional">(comma-separated)</span></label>
          <input id="ed-tags" class="edit-input" bind:value={editTagsRaw} placeholder="n5, animals, nouns" />
        </div>
        <div class="edit-actions">
          <button class="btn" on:click={saveEdit} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button class="btn btn-ghost" on:click={() => { editing = false; }}>Cancel</button>
        </div>
      </div>

    {:else}
      <!-- ── View mode ── -->
      <div class="card-hero">
        <div class="front-word">{card.front_text}</div>
        {#if card.audio_url}
          <audio id="detail-audio" src={card.audio_url} preload="auto"></audio>
          <button class="audio-btn" on:click={playAudio} title="Play audio">▶ Play</button>
        {/if}
      </div>

      {#if card.image_url}
        <img class="context-img" src={card.image_url} alt="context screenshot" />
      {/if}

      <div class="fields">
        {#if card.back_text}
          <div class="field">
            <div class="field-label">Definition</div>
            <div class="field-value">{card.back_text}</div>
          </div>
        {/if}

        {#if card.sentence}
          <div class="field">
            <div class="field-label">Sentence</div>
            <div class="field-value sentence">{card.sentence}</div>
          </div>
        {/if}

        {#if card.subtitle_translation}
          <div class="field">
            <div class="field-label">Translation</div>
            <div class="field-value translation">{card.subtitle_translation}</div>
          </div>
        {/if}

        {#if card.notes}
          <div class="field">
            <div class="field-label">Notes</div>
            <div class="field-value">{card.notes}</div>
          </div>
        {/if}

        {#if card.tags && card.tags.length > 0}
          <div class="field">
            <div class="field-label">Tags</div>
            <div class="field-value tag-row">
              {#each card.tags as tag}
                <span class="tag">{tag}</span>
              {/each}
            </div>
          </div>
        {/if}

        {#if card.source_url || card.video_source_url}
          <div class="field">
            <div class="field-label">Source</div>
            <div class="field-value">
              <a href={card.video_source_url ?? card.source_url ?? '#'} target="_blank" rel="noopener" class="source-link">
                {card.video_source_url ?? card.source_url}
              </a>
            </div>
          </div>
        {/if}

        {#if card.subtitle_start_ms != null}
          <div class="field">
            <div class="field-label">Timestamp</div>
            <div class="field-value">{formatMs(card.subtitle_start_ms)} – {formatMs(card.subtitle_end_ms)}</div>
          </div>
        {/if}

        <div class="field">
          <div class="field-label">SRS stats</div>
          <div class="field-value meta-row">
            <span>Reps: {card.reps}</span>
            <span>Lapses: {card.lapses}</span>
            {#if card.stability != null}
              <span>Stability: {card.stability.toFixed(1)}d</span>
            {/if}
            {#if card.difficulty != null}
              <span>Difficulty: {card.difficulty.toFixed(2)}</span>
            {/if}
          </div>
        </div>

        <div class="field">
          <div class="field-label">Added</div>
          <div class="field-value">{relativeDate(card.created_at)}</div>
        </div>
      </div>

      <div class="actions">
        <button class="btn" on:click={startEdit}>Edit</button>
        <button class="btn btn-ghost" on:click={toggleSuspend} disabled={suspending}>
          {card.suspended ? 'Unsuspend' : 'Suspend'}
        </button>
        <button class="btn btn-ghost" on:click={doBury}>Bury</button>
        <button class="btn-danger" on:click={deleteCard} disabled={deleting}>
          {deleting ? 'Deleting…' : 'Delete'}
        </button>
      </div>
    {/if}
  {/if}
</main>

<style>
  main { max-width: 640px; margin: 0 auto; padding: 1.5rem 1rem 3rem; }

  .msg { color: #9ba8c0; text-align: center; margin-top: 3rem; }

  .header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }

  .back {
    color: #9ba8c0;
    text-decoration: none;
    font-size: 0.9rem;
  }
  .back:hover { color: #e8eaf0; }

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

  /* ── Edit form ── */
  .edit-form {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 12px;
    padding: 1.5rem;
  }
  .edit-title {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 1.25rem;
    color: #e8eaf0;
  }
  .edit-field { margin-bottom: 0.9rem; }
  .edit-label {
    display: block;
    font-size: 0.72rem;
    color: #4a5568;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin-bottom: 0.3rem;
  }
  .optional { text-transform: none; letter-spacing: 0; font-style: italic; color: #374060; }
  .edit-input {
    width: 100%;
    background: #12141a;
    border: 1px solid #2a2d36;
    color: #c8d0e0;
    border-radius: 6px;
    padding: 0.5rem 0.75rem;
    font-size: 0.95rem;
    box-sizing: border-box;
    transition: border-color 0.15s;
  }
  .edit-input:focus {
    outline: none;
    border-color: #4caf50;
  }
  .edit-textarea { resize: vertical; min-height: 64px; font-family: inherit; }
  .edit-actions { display: flex; gap: 0.75rem; margin-top: 1.25rem; }

  /* ── Card hero (view mode) ── */
  .card-hero {
    text-align: center;
    padding: 2rem 1rem 1.5rem;
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 12px;
    margin-bottom: 1rem;
  }
  .front-word {
    font-size: 3rem;
    font-weight: 600;
    color: #e8eaf0;
    letter-spacing: -0.02em;
  }
  .audio-btn {
    margin-top: 0.75rem;
    background: #1a2d1a;
    border: 1px solid #2a3d2a;
    color: #4caf50;
    padding: 0.4rem 1rem;
    border-radius: 6px;
    font-size: 0.85rem;
    cursor: pointer;
    transition: background 0.15s;
  }
  .audio-btn:hover { background: #22382a; }

  .context-img {
    width: 100%;
    border-radius: 8px;
    border: 1px solid #2a2d36;
    margin-bottom: 1rem;
    max-height: 280px;
    object-fit: cover;
    object-position: center;
  }

  .fields { display: flex; flex-direction: column; gap: 0; }
  .field { padding: 0.75rem 0; border-bottom: 1px solid #1e2128; }
  .field-label {
    font-size: 0.72rem;
    color: #4a5568;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin-bottom: 0.25rem;
  }
  .field-value { color: #c8d0e0; font-size: 0.95rem; line-height: 1.5; }
  .sentence { font-style: italic; color: #9ba8c0; }
  .translation { color: #7a8aa6; }
  .source-link {
    color: #4caf50;
    text-decoration: none;
    font-size: 0.82rem;
    word-break: break-all;
  }
  .source-link:hover { text-decoration: underline; }
  .meta-row { display: flex; gap: 1.25rem; flex-wrap: wrap; font-size: 0.88rem; color: #6b7591; }

  .tag-row { display: flex; gap: 0.4rem; flex-wrap: wrap; }
  .tag {
    background: #1a2040;
    color: #64b5f6;
    font-size: 0.72rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    border: 1px solid #263050;
  }

  .actions {
    margin-top: 2rem;
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
    align-items: center;
  }

  .btn {
    display: inline-block;
    padding: 0.55rem 1.1rem;
    background: #4caf50;
    color: #fff;
    border-radius: 7px;
    text-decoration: none;
    font-size: 0.88rem;
    font-weight: 500;
    border: none;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn:hover:not(:disabled) { background: #43a047; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .btn-ghost {
    background: transparent;
    border: 1px solid #3a3d47;
    color: #9ba8c0;
  }
  .btn-ghost:hover:not(:disabled) { background: #1e2128; border-color: #4caf50; color: #4caf50; }

  .btn-danger {
    background: transparent;
    border: 1px solid #c62828;
    color: #ef5350;
    padding: 0.55rem 1.1rem;
    border-radius: 7px;
    font-size: 0.88rem;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-danger:hover:not(:disabled) { background: #2d1515; }
  .btn-danger:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
