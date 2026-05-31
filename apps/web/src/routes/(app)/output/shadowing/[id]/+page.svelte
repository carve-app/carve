<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { apiFetch, postMultipart } from '$lib/api';
  import { toasts } from '$lib/stores/toast';

  $: id = $page.params.id;

  let reference = '';
  let referenceAudioUrl = '';
  let userAudioUrl = '';
  let recorder: MediaRecorder | null = null;
  let chunks: BlobPart[] = [];
  let recording = false;
  let processing = false;
  let diff: { op: string; ref?: string; hyp?: string }[] = [];
  let wer: number | null = null;
  let transcript = '';

  type SR = typeof window extends { webkitSpeechRecognition: infer T }
    ? T extends new () => any ? InstanceType<T> : any : any;
  let sttFallback: SR | null = null;
  let sttHypothesis = '';

  interface ShadowItem { id: string; audio_url: string; transcript?: string; card_front?: string }
  interface TranscribeResp { transcript: string; diff: { op: string; ref?: string; hyp?: string }[]; wer: number }

  onMount(async () => {
    try {
      const data = await apiFetch<{ items: ShadowItem[] }>('/v1/output/shadowing');
      const item = (data.items ?? []).find(x => x.id === id);
      if (item) {
        reference = item.transcript ?? item.card_front ?? '';
        referenceAudioUrl = item.audio_url ?? '';
      }
    } catch (e) {
      toasts.add(e instanceof Error ? e.message : 'Could not load reference');
    }
  });

  onDestroy(() => {
    recorder?.stop();
  });

  async function startRecording() {
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

      // Try browser SpeechRecognition for an instant local hypothesis.
      const w = window as unknown as { webkitSpeechRecognition?: any; SpeechRecognition?: any };
      const SR = w.SpeechRecognition ?? w.webkitSpeechRecognition;
      if (SR) {
        sttFallback = new SR();
        sttFallback.lang = 'en-US';
        sttFallback.interimResults = false;
        sttFallback.continuous = true;
        sttFallback.onresult = (e: any) => {
          for (const res of Array.from(e.results) as any[]) {
            if (res.isFinal) sttHypothesis += ' ' + res[0].transcript;
          }
        };
        try { sttFallback.start(); } catch { /* may throw if already started */ }
      }
    } catch {
      toasts.add('Microphone permission denied');
    }
  }

  async function stopAndSubmit() {
    if (!recorder) return;
    recording = false;
    processing = true;
    recorder.stop();
    if (sttFallback) try { sttFallback.stop(); } catch { /* */ }

    // Wait one tick so onstop fires and userAudioUrl is set.
    await new Promise(r => setTimeout(r, 80));

    // Build multipart with the blob.
    const blob = new Blob(chunks, { type: 'audio/webm' });
    const form = new FormData();
    form.append('audio', blob, 'utterance.webm');
    form.append('expected', reference);
    form.append('hypothesis', sttHypothesis.trim());
    form.append('language', 'en');

    try {
      const data = await postMultipart<TranscribeResp>('/v1/output/transcribe', form);
      transcript = data.transcript ?? '';
      diff = data.diff ?? [];
      wer = data.wer ?? null;
      await apiFetch(`/v1/output/shadowing/${id}/complete`, { method: 'POST' });
    } catch (e) {
      toasts.add(e instanceof Error ? e.message : 'Transcription failed');
    }
    processing = false;
  }

  function playReference() {
    if (!referenceAudioUrl) return;
    new Audio(referenceAudioUrl).play();
  }

  function colorFor(op: string): string {
    switch (op) {
      case 'keep':   return '#4caf50';
      case 'sub':    return '#ffa726';
      case 'insert': return '#42a5f5';
      case 'delete': return '#e57373';
      default:       return '#9ba8c0';
    }
  }
</script>

<main>
  <div class="head">
    <a href="/output" class="back">← Output</a>
    <h2>Shadow this sentence</h2>
  </div>

  <div class="ref-card">
    <div class="ref-label">Reference</div>
    <p class="ref-text">{reference || '—'}</p>
    {#if referenceAudioUrl}
      <button class="ghost-btn" on:click={playReference}>▶ Play reference</button>
    {/if}
  </div>

  <div class="rec-card">
    {#if !recording && !processing && !transcript}
      <button class="big-btn" on:click={startRecording}>🎙 Start recording</button>
      <div class="tip">Speak the sentence aloud. Stop when done.</div>
    {:else if recording}
      <button class="big-btn rec" on:click={stopAndSubmit}>■ Stop &amp; analyze</button>
      <div class="tip">Recording…</div>
    {:else if processing}
      <div class="big-btn loading">Analyzing…</div>
    {:else}
      <button class="ghost-btn" on:click={() => { transcript = ''; diff = []; wer = null; }}>Try again</button>
    {/if}
  </div>

  {#if transcript}
    <div class="result-card">
      <div class="result-row">
        <span class="result-label">You said</span>
        <span class="result-val">{transcript || '(no hypothesis)'}</span>
      </div>
      {#if wer != null}
        <div class="result-row">
          <span class="result-label">Word error rate</span>
          <span class="result-val" style="color:{wer < 0.15 ? '#4caf50' : wer < 0.35 ? '#ffa726' : '#e57373'}">
            {(wer * 100).toFixed(1)}%
          </span>
        </div>
      {/if}
      {#if diff.length > 0}
        <div class="diff-row">
          {#each diff as e}
            <span class="diff-tok" style="color:{colorFor(e.op)}" title={e.op}>
              {e.op === 'insert' ? `+${e.hyp}` : e.op === 'delete' ? `-${e.ref}` : (e.hyp ?? e.ref)}
            </span>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#if userAudioUrl}
    <div class="playback">
      <audio controls src={userAudioUrl}></audio>
    </div>
  {/if}
</main>

<style>
  main { max-width: 720px; margin: 0 auto; padding: 1.5rem 1rem; }
  .head { display: flex; align-items: baseline; gap: 1rem; margin-bottom: 1.5rem; }
  .back { font-size: 0.85rem; color: #7a8aa6; text-decoration: none; }
  h2 { margin: 0; font-size: 1.2rem; color: #e8eaf0; }

  .ref-card, .rec-card, .result-card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .ref-label { font-size: 0.75rem; color: #6b7591; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.5rem; }
  .ref-text { font-size: 1.1rem; color: #e8eaf0; line-height: 1.5; margin: 0 0 1rem; }

  .rec-card { display: flex; flex-direction: column; align-items: center; gap: 0.5rem; }
  .big-btn {
    background: #4caf50;
    color: #fff;
    border: none;
    border-radius: 999px;
    padding: 0.9rem 1.75rem;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
  }
  .big-btn.rec { background: #e53935; animation: pulse 1.4s ease-in-out infinite; }
  .big-btn.loading { background: #2a2d36; color: #9ba8c0; cursor: default; }
  @keyframes pulse { 50% { transform: scale(1.04); } }

  .ghost-btn {
    background: transparent;
    border: 1px solid #2a2d36;
    color: #c8d0e0;
    padding: 0.45rem 1rem;
    border-radius: 7px;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .tip { font-size: 0.8rem; color: #7a8aa6; }
  .result-label { color: #7a8aa6; font-size: 0.85rem; }
  .result-row { display: flex; justify-content: space-between; padding: 0.4rem 0; }
  .result-val { color: #e8eaf0; }
  .diff-row { margin-top: 0.75rem; display: flex; flex-wrap: wrap; gap: 0.35rem; font-family: monospace; font-size: 0.95rem; }
  .diff-tok { padding: 0 0.15rem; }
  .playback { margin-top: 1rem; }
</style>
