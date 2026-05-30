<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch, ApiError } from '$lib/api';

  type Phase = 'loading' | 'exercises' | 'reviewing' | 'shadowing' | 'done' | 'unauthenticated' | 'error';

  interface Exercise {
    id: string;
    exercise_type: 'writing' | 'cloze' | 'shadowing' | 'speaking';
    prompt: string;
    target_word: string;
    sentence?: string;
    audio_url?: string;
    created_at: string;
  }

  interface ShadowItem {
    id: string;
    audio_url: string;
    transcript?: string;
    times_shadowed: number;
    card_front?: string;
  }

  interface FeedbackDetail {
    grammar: string;
    vocabulary: string;
    naturalness: string;
  }

  interface Feedback {
    score: number;
    feedback: string;
    feedback_detail: FeedbackDetail;
  }

  let phase: Phase = 'loading';
  let exercises: Exercise[] = [];
  let shadowItems: ShadowItem[] = [];
  let currentIdx = 0;
  let answer = '';
  let submitting = false;
  let feedback: Feedback | null = null;
  let activeTab: 'writing' | 'shadowing' = 'writing';
  let errorMsg = '';
  let lang = 'ja';

  onMount(async () => {
    if (!localStorage.getItem('carve_access_token')) {
      phase = 'unauthenticated';
      return;
    }
    await loadAll();
  });

  async function loadAll() {
    phase = 'loading';
    try {
      const [exRes, shRes] = await Promise.all([
        apiFetch<{ exercises: Exercise[] }>(`/v1/output/exercises?language=${lang}`),
        apiFetch<{ items: ShadowItem[] }>(`/v1/output/shadowing?language=${lang}`),
      ]);
      exercises = (exRes.exercises ?? []).filter(e => e.exercise_type !== 'shadowing');
      shadowItems = shRes.items ?? [];
      currentIdx = 0;
      feedback = null;
      answer = '';
      phase = exercises.length > 0 ? 'exercises' : 'done';
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        phase = 'unauthenticated';
      } else {
        errorMsg = err instanceof Error ? err.message : 'Unknown error';
        phase = 'error';
      }
    }
  }

  async function submitAnswer() {
    const ex = exercises[currentIdx];
    if (!ex || !answer.trim()) return;
    submitting = true;
    feedback = null;
    try {
      const res = await apiFetch<Feedback>('/v1/output/submit', {
        method: 'POST',
        body: JSON.stringify({ exercise_id: ex.id, answer_text: answer }),
      });
      feedback = res;
      phase = 'reviewing';
    } catch {
      // no-op
    }
    submitting = false;
  }

  function nextExercise() {
    feedback = null;
    answer = '';
    if (currentIdx + 1 < exercises.length) {
      currentIdx++;
      phase = 'exercises';
    } else {
      phase = 'done';
    }
  }

  async function completeShadow(id: string) {
    try {
      await apiFetch(`/v1/output/shadowing/${id}/complete`, { method: 'POST' });
      shadowItems = shadowItems.map(s => s.id === id ? { ...s, times_shadowed: s.times_shadowed + 1 } : s);
    } catch { /* no-op */ }
  }

  function scoreColor(score: number): string {
    if (score >= 80) return '#4caf50';
    if (score >= 50) return '#ffa726';
    return '#ef5350';
  }

  $: currentEx = exercises[currentIdx] ?? null;
  $: progressPct = exercises.length > 0 ? Math.round((currentIdx / exercises.length) * 100) : 0;
</script>

<main>
  <header>
    <h1>Output Practice</h1>
    <nav>
      <a href="/">Home</a>
      <a href="/review">Review</a>
      <a href="/stats">Stats</a>
    </nav>
  </header>

  <div class="tabs-row">
    <div class="tabs">
      <button class="tab" class:active={activeTab === 'writing'} on:click={() => activeTab = 'writing'}>Writing & Cloze</button>
      <button class="tab" class:active={activeTab === 'shadowing'} on:click={() => activeTab = 'shadowing'}>Shadowing</button>
    </div>
    <select class="lang-select" bind:value={lang} on:change={loadAll}>
      <option value="ja">Japanese</option>
      <option value="zh-cn">Chinese (Simplified)</option>
      <option value="ko">Korean</option>
    </select>
  </div>

  {#if activeTab === 'writing'}

    {#if phase === 'loading'}
      <p class="msg">Loading exercises…</p>

    {:else if phase === 'unauthenticated'}
      <div class="msg"><p>Please <a href="/login">log in</a> to practice.</p></div>

    {:else if phase === 'error'}
      <div class="msg"><p class="error-text">{errorMsg}</p><button class="btn" on:click={loadAll}>Retry</button></div>

    {:else if phase === 'done'}
      <div class="done-card">
        <div class="done-icon">✓</div>
        <h2>Session complete!</h2>
        <p>You've finished all {exercises.length} exercises.</p>
        <button class="btn" on:click={loadAll}>Practice more</button>
      </div>

    {:else if currentEx}
      <!-- Progress bar -->
      <div class="progress-bar-track">
        <div class="progress-bar-fill" style="width:{progressPct}%"></div>
      </div>
      <div class="progress-label">{currentIdx + 1} / {exercises.length}</div>

      <div class="exercise-card">
        <div class="ex-type-badge ex-{currentEx.exercise_type}">{currentEx.exercise_type}</div>
        <div class="target-word">{currentEx.target_word}</div>
        <p class="prompt">{currentEx.prompt}</p>

        {#if phase === 'exercises'}
          <textarea
            class="answer-input"
            placeholder="Type your answer here…"
            bind:value={answer}
            rows="4"
          ></textarea>
          <div class="actions">
            <button class="btn" on:click={submitAnswer} disabled={submitting || !answer.trim()}>
              {submitting ? 'Checking…' : 'Submit'}
            </button>
            <button class="btn-skip" on:click={nextExercise}>Skip</button>
          </div>
        {/if}

        {#if phase === 'reviewing' && feedback}
          <div class="answer-display">{answer}</div>

          <div class="feedback-card">
            <div class="score-circle" style="color:{scoreColor(feedback.score)}">
              {Math.round(feedback.score)}
            </div>
            <p class="feedback-text">{feedback.feedback}</p>
            {#if feedback.feedback_detail?.grammar}
              <div class="detail-row">
                <span class="detail-label">Grammar</span>
                <span>{feedback.feedback_detail.grammar}</span>
              </div>
            {/if}
            {#if feedback.feedback_detail?.vocabulary}
              <div class="detail-row">
                <span class="detail-label">Vocabulary</span>
                <span>{feedback.feedback_detail.vocabulary}</span>
              </div>
            {/if}
            {#if feedback.feedback_detail?.naturalness}
              <div class="detail-row">
                <span class="detail-label">Naturalness</span>
                <span>{feedback.feedback_detail.naturalness}</span>
              </div>
            {/if}
          </div>
          <div class="actions">
            <button class="btn" on:click={nextExercise}>Next exercise →</button>
          </div>
        {/if}
      </div>
    {/if}

  {:else}
    <!-- Shadowing tab -->
    {#if shadowItems.length === 0}
      <div class="msg">
        <p>No shadowing items yet. Mine cards with audio to build your shadowing queue.</p>
      </div>
    {:else}
      <ul class="shadow-list">
        {#each shadowItems as item (item.id)}
          <li class="shadow-card">
            {#if item.card_front}
              <div class="shadow-word">{item.card_front}</div>
            {/if}
            {#if item.transcript}
              <p class="shadow-transcript">{item.transcript}</p>
            {/if}
            <audio controls src={item.audio_url} class="shadow-audio"></audio>
            <div class="shadow-footer">
              <span class="shadow-count">{item.times_shadowed}× shadowed</span>
              <button class="btn-sm" on:click={() => completeShadow(item.id)}>Mark done</button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

<style>
  :global(body) { background: #13151a; color: #e8eaf0; font-family: system-ui, sans-serif; margin: 0; }
  main { max-width: 680px; margin: 0 auto; padding: 1.5rem 1rem; }

  header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1.5rem; border-bottom: 1px solid #2a2d36; padding-bottom: 1rem; }
  header h1 { margin: 0; font-size: 1.4rem; }
  nav { display: flex; gap: 1rem; }
  nav a { color: #9ba8c0; text-decoration: none; font-size: 0.9rem; }
  nav a:hover { color: #e8eaf0; }

  .tabs-row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; }
  .tabs { display: flex; gap: 0.5rem; }
  .tab { background: #1e2128; border: 1px solid #2a2d36; color: #9ba8c0; padding: 0.5rem 1.2rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem; transition: all 0.15s; }
  .tab.active { background: #4caf50; border-color: #4caf50; color: #fff; }

  .lang-select { background: #1e2128; border: 1px solid #3a3d47; color: #9ba8c0; padding: 0.4rem 0.7rem; border-radius: 6px; font-size: 0.85rem; cursor: pointer; }

  .msg { text-align: center; margin-top: 3rem; color: #9ba8c0; }
  .msg a { color: #4caf50; }
  .error-text { color: #ef5350; }

  .progress-bar-track { height: 4px; background: #2a2d36; border-radius: 2px; margin-bottom: 0.3rem; }
  .progress-bar-fill { height: 100%; background: #4caf50; border-radius: 2px; transition: width 0.3s; }
  .progress-label { font-size: 0.75rem; color: #6b7591; margin-bottom: 1rem; }

  .exercise-card { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.5rem; }
  .ex-type-badge { display: inline-block; font-size: 0.68rem; font-weight: 700; padding: 0.1rem 0.5rem; border-radius: 4px; text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 0.75rem; }
  .ex-writing { background: #1a2e40; color: #42a5f5; }
  .ex-cloze   { background: #2e1a40; color: #ba68c8; }
  .ex-shadowing { background: #2e2a1a; color: #ffb74d; }

  .target-word { font-size: 2rem; font-weight: 700; margin-bottom: 0.5rem; color: #4caf50; }
  .prompt { font-size: 0.95rem; color: #9ba8c0; margin: 0 0 1.25rem; line-height: 1.6; }

  .answer-input { width: 100%; box-sizing: border-box; background: #13151a; border: 1px solid #3a3d47; color: #e8eaf0; padding: 0.75rem; border-radius: 7px; font-size: 1rem; resize: vertical; margin-bottom: 1rem; }
  .answer-input:focus { outline: none; border-color: #4caf50; }

  .answer-display { background: #13151a; border-left: 3px solid #4caf50; padding: 0.75rem 1rem; border-radius: 0 6px 6px 0; margin-bottom: 1rem; color: #e8eaf0; font-size: 0.95rem; white-space: pre-wrap; }

  .feedback-card { background: #13151a; border: 1px solid #2a2d36; border-radius: 8px; padding: 1rem; margin-bottom: 1rem; }
  .score-circle { font-size: 2.5rem; font-weight: 700; text-align: center; margin-bottom: 0.5rem; }
  .feedback-text { color: #e8eaf0; font-size: 0.95rem; margin: 0 0 0.75rem; }
  .detail-row { display: flex; gap: 0.75rem; font-size: 0.82rem; color: #9ba8c0; padding: 0.35rem 0; border-top: 1px solid #2a2d36; }
  .detail-label { font-weight: 600; color: #6b7591; min-width: 80px; }

  .actions { display: flex; gap: 0.75rem; align-items: center; }
  .btn { background: #4caf50; color: #fff; border: none; padding: 0.65rem 1.5rem; border-radius: 7px; font-size: 0.95rem; font-weight: 500; cursor: pointer; transition: background 0.15s; }
  .btn:hover:not(:disabled) { background: #43a047; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-skip { background: none; border: 1px solid #3a3d47; color: #9ba8c0; padding: 0.6rem 1.2rem; border-radius: 7px; font-size: 0.88rem; cursor: pointer; }
  .btn-skip:hover { background: #2a2d36; }

  .done-card { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 3rem 2rem; text-align: center; margin-top: 2rem; }
  .done-icon { font-size: 3rem; color: #4caf50; margin-bottom: 1rem; }
  .done-card h2 { margin: 0 0 0.5rem; }
  .done-card p { color: #9ba8c0; margin: 0 0 1.5rem; }

  .shadow-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 1rem; }
  .shadow-card { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.25rem; }
  .shadow-word { font-size: 1.2rem; font-weight: 600; margin-bottom: 0.4rem; }
  .shadow-transcript { font-size: 0.88rem; color: #9ba8c0; margin: 0 0 0.75rem; }
  .shadow-audio { width: 100%; margin-bottom: 0.75rem; }
  .shadow-footer { display: flex; justify-content: space-between; align-items: center; }
  .shadow-count { font-size: 0.78rem; color: #6b7591; }
  .btn-sm { background: #4caf50; color: #fff; border: none; padding: 0.35rem 0.8rem; border-radius: 5px; font-size: 0.82rem; cursor: pointer; }
</style>
