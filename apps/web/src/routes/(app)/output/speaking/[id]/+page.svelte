<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { apiFetch, postMultipart } from '$lib/api';
  import { toasts } from '$lib/stores/toast';

  $: id = $page.params.id;

  let prompt = '';
  let targetWord = '';
  let recorder: MediaRecorder | null = null;
  let chunks: BlobPart[] = [];
  let recording = false;
  let processing = false;
  let userAudioUrl = '';
  let transcript = '';
  let usedTarget = false;
  let sttFallback: any = null;
  let sttHypothesis = '';

  interface Exercise { id: string; prompt: string; target_word: string }
  interface TranscribeResp { transcript: string; diff: unknown[]; wer: number }
  interface FeedbackDetail { grammar: string; vocabulary: string; naturalness: string; }
  interface Feedback { score: number; feedback: string; feedback_detail: FeedbackDetail; }

  let feedback: Feedback | null = null;

  function scoreColor(score: number): string {
    if (score >= 80) return '#4caf50';
    if (score >= 50) return '#ffa726';
    return '#ef5350';
  }

  onMount(async () => {
    try {
      const data = await apiFetch<{ exercises: Exercise[] }>('/v1/output/exercises');
      const ex = (data.exercises ?? []).find(x => x.id === id);
      if (ex) {
        prompt = ex.prompt;
        targetWord = ex.target_word;
      }
    } catch (e) {
      toasts.add(e instanceof Error ? e.message : 'Could not load exercise');
    }
  });

  onDestroy(() => recorder?.stop());

  async function start() {
    chunks = [];
    sttHypothesis = '';
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      recorder = new MediaRecorder(stream);
      recorder.ondataavailable = (e) => chunks.push(e.data);
      recorder.onstop = () => {
        const blob = new Blob(chunks, { type: 'audio/webm' });
        userAudioUrl = URL.createObjectURL(blob);
        stream.getTracks().forEach(t => t.stop());
      };
      recorder.start();
      recording = true;

      const w = window as unknown as { webkitSpeechRecognition?: any; SpeechRecognition?: any };
      const SR = w.SpeechRecognition ?? w.webkitSpeechRecognition;
      if (SR) {
        sttFallback = new SR();
        sttFallback.lang = 'en-US';
        sttFallback.continuous = true;
        sttFallback.interimResults = false;
        sttFallback.onresult = (e: any) => {
          for (const res of Array.from(e.results) as any[]) {
            if (res.isFinal) sttHypothesis += ' ' + res[0].transcript;
          }
        };
        try { sttFallback.start(); } catch { /* */ }
      }
    } catch {
      toasts.add('Microphone permission denied');
    }
  }

  async function stop() {
    if (!recorder) return;
    recording = false;
    processing = true;
    recorder.stop();
    if (sttFallback) try { sttFallback.stop(); } catch { /* */ }
    await new Promise(r => setTimeout(r, 80));

    const blob = new Blob(chunks, { type: 'audio/webm' });
    const form = new FormData();
    form.append('audio', blob, 'speech.webm');
    form.append('hypothesis', sttHypothesis.trim());
    form.append('language', 'en');

    try {
      const data = await postMultipart<TranscribeResp>('/v1/output/transcribe', form);
      transcript = data.transcript ?? '';
      usedTarget = transcript.toLowerCase().includes(targetWord.toLowerCase());

      // Submit the transcript for AI evaluation. The backend field is
      // `answer_text` (not `response_text`) — sending the wrong key produced an
      // empty answer and discarded feedback. Capture + render the result so the
      // user actually gets pronunciation/usage feedback on what they said.
      feedback = await apiFetch<Feedback>('/v1/output/submit', {
        method: 'POST',
        body: JSON.stringify({
          exercise_id: id,
          answer_text: transcript,
        }),
      });
    } catch (e) {
      toasts.add(e instanceof Error ? e.message : 'Could not analyze speech');
    }
    processing = false;
  }

  function reset() {
    transcript = '';
    userAudioUrl = '';
    usedTarget = false;
    feedback = null;
  }
</script>

<main>
  <div class="head">
    <a href="/output" class="back">← Output</a>
    <h2>Speaking practice</h2>
  </div>

  <div class="prompt-card">
    <div class="label">Prompt</div>
    <p class="prompt-text">{prompt || '—'}</p>
    {#if targetWord}
      <div class="target">Try to use: <code>{targetWord}</code></div>
    {/if}
  </div>

  <div class="rec-card">
    {#if !recording && !processing && !transcript}
      <button class="big-btn" on:click={start}>🎙 Start speaking</button>
    {:else if recording}
      <button class="big-btn rec" on:click={stop}>■ Stop</button>
      <div class="tip">Recording…</div>
    {:else if processing}
      <div class="big-btn loading">Analyzing…</div>
    {:else}
      <button class="ghost-btn" on:click={reset}>Try again</button>
    {/if}
  </div>

  {#if transcript}
    <div class="result-card">
      <div class="row">
        <span class="label">You said</span>
        <p class="trans">{transcript}</p>
      </div>
      <div class="row">
        <span class="label">Target word used</span>
        <span class={usedTarget ? 'yes' : 'no'}>{usedTarget ? '✓ Yes' : '✗ No'}</span>
      </div>
    </div>
  {/if}

  {#if feedback}
    <div class="feedback-card">
      <div class="score-circle" style="color:{scoreColor(feedback.score)}">{Math.round(feedback.score)}</div>
      <p class="feedback-text">{feedback.feedback}</p>
      {#if feedback.feedback_detail?.grammar}
        <div class="detail-row"><span class="detail-label">Grammar</span><span>{feedback.feedback_detail.grammar}</span></div>
      {/if}
      {#if feedback.feedback_detail?.vocabulary}
        <div class="detail-row"><span class="detail-label">Vocabulary</span><span>{feedback.feedback_detail.vocabulary}</span></div>
      {/if}
      {#if feedback.feedback_detail?.naturalness}
        <div class="detail-row"><span class="detail-label">Naturalness</span><span>{feedback.feedback_detail.naturalness}</span></div>
      {/if}
    </div>
  {/if}

  {#if userAudioUrl}
    <div class="playback"><audio controls src={userAudioUrl}></audio></div>
  {/if}
</main>

<style>
  main { max-width: 720px; margin: 0 auto; padding: 1.5rem 1rem; }
  .head { display: flex; align-items: baseline; gap: 1rem; margin-bottom: 1.5rem; }
  .back { font-size: 0.85rem; color: #7a8aa6; text-decoration: none; }
  h2 { margin: 0; font-size: 1.2rem; color: #e8eaf0; }

  .prompt-card, .rec-card, .result-card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .feedback-card { background: #13151a; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.25rem; margin-bottom: 1rem; }
  .score-circle { font-size: 2.5rem; font-weight: 700; text-align: center; margin-bottom: 0.5rem; }
  .feedback-text { color: #e8eaf0; font-size: 0.95rem; margin: 0 0 0.75rem; }
  .detail-row { display: flex; gap: 0.75rem; font-size: 0.85rem; color: #b0bec5; padding: 0.3rem 0; border-top: 1px solid #2a2d36; }
  .detail-label { color: #8a96b3; min-width: 90px; text-transform: uppercase; font-size: 0.7rem; letter-spacing: 0.05em; padding-top: 0.15rem; }
  .label { font-size: 0.75rem; color: #8a96b3; text-transform: uppercase; letter-spacing: 0.05em; }
  .prompt-text { color: #e8eaf0; font-size: 1.05rem; line-height: 1.5; margin: 0.5rem 0 0.75rem; }
  .target { color: #9ba8c0; font-size: 0.85rem; }
  .target code {
    background: #13151a;
    padding: 0.1rem 0.4rem;
    border-radius: 4px;
    color: #4caf50;
  }

  .rec-card { display: flex; flex-direction: column; align-items: center; gap: 0.5rem; }
  .big-btn {
    background: #2e7d32; color: #fff; border: none; border-radius: 999px;
    padding: 0.9rem 1.75rem; font-size: 1rem; font-weight: 600; cursor: pointer;
  }
  .big-btn.rec { background: #e53935; animation: pulse 1.4s ease-in-out infinite; }
  .big-btn.loading { background: #2a2d36; color: #9ba8c0; cursor: default; }
  @keyframes pulse { 50% { transform: scale(1.04); } }
  .ghost-btn {
    background: transparent; border: 1px solid #2a2d36; color: #c8d0e0;
    padding: 0.45rem 1rem; border-radius: 7px; font-size: 0.85rem; cursor: pointer;
  }
  .tip { font-size: 0.8rem; color: #7a8aa6; }
  .row { padding: 0.5rem 0; display: flex; flex-direction: column; gap: 0.2rem; }
  .trans { color: #e8eaf0; margin: 0.25rem 0 0; }
  .yes { color: #4caf50; }
  .no { color: #e57373; }
  .playback { margin-top: 1rem; }
</style>
