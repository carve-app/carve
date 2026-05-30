<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import {
    fetchFsrsSettings,
    saveFsrsSettings,
    fetchWorkloadPreview,
    apiFetch,
    type FsrsSettings,
    type WorkloadPreview,
  } from '$lib/api';
  import { lang } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';

  let loading = true;
  let saving = false;
  let saved = false;

  let settings: FsrsSettings = {
    language_code: 'ja',
    weights: [],
    target_retention: 0.90,
    leech_threshold: 5,
    daily_new_limit: 20,
    is_customized: false,
  };
  let preview: WorkloadPreview | null = null;
  let previewLoading = false;

  onMount(() => {
    let init = false;
    const unsub = lang.subscribe(l => { if (init) reload(l); });
    reload(get(lang));
    init = true;
    return unsub;
  });

  async function reload(language = get(lang)) {
    loading = true;
    try {
      settings = await fetchFsrsSettings(language);
      settings.language_code = language;
      await loadPreview(language);
    } catch {
      // use defaults already set
    }
    loading = false;
  }

  async function loadPreview(language = get(lang)) {
    previewLoading = true;
    try {
      preview = await fetchWorkloadPreview(language, settings.target_retention);
    } catch {
      preview = null;
    }
    previewLoading = false;
  }

  async function save() {
    saving = true;
    try {
      await saveFsrsSettings({
        language_code: settings.language_code,
        target_retention: settings.target_retention,
        leech_threshold: settings.leech_threshold,
        daily_new_limit: settings.daily_new_limit,
      });
      saved = true;
      setTimeout(() => { saved = false; }, 2500);
      await loadPreview();
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Save failed');
    }
    saving = false;
  }

  function resetToDefaults() {
    settings.target_retention = 0.90;
    settings.leech_threshold = 5;
    settings.daily_new_limit = 20;
    loadPreview();
  }

  function pct(v: number) { return Math.round(v * 100); }

  let optimizing = false;
  let optimizeResult: { optimized: boolean; reason?: string; events_used?: number } | null = null;

  async function runOptimizer() {
    optimizing = true;
    optimizeResult = null;
    try {
      optimizeResult = await apiFetch(`/v1/settings/fsrs/optimize?language=${settings.language_code}`, { method: 'POST' });
      if (optimizeResult?.optimized) {
        settings = await fetchFsrsSettings(settings.language_code);
      }
    } catch (err) {
      optimizeResult = { optimized: false, reason: err instanceof Error ? err.message : 'Optimizer failed' };
    }
    optimizing = false;
  }
</script>

<main>
  {#if loading}
    <p class="msg">Loading…</p>
  {:else}
    <div class="section">
      <h2>SRS Settings</h2>

      <div class="field">
        <label for="retention-slider">
          Target Retention
          <span class="hint">Probability of remembering a card when it's due. Higher = shorter intervals, more reviews.</span>
        </label>
        <div class="slider-row">
          <input
            id="retention-slider"
            type="range"
            min="0.70" max="0.98" step="0.01"
            bind:value={settings.target_retention}
            on:change={() => loadPreview()}
          />
          <span class="pct">{pct(settings.target_retention)}%</span>
        </div>
        {#if previewLoading}
          <p class="preview-text">Calculating…</p>
        {:else if preview}
          <p class="preview-text">
            With {pct(settings.target_retention)}% retention:
            avg interval {preview.avg_interval_days.toFixed(1)} days
            across {preview.total_review_cards} review cards.
          </p>
        {/if}
      </div>

      <div class="field">
        <label for="daily-new-limit">
          Daily New Cards Limit
          <span class="hint">Maximum new cards added to your queue per day.</span>
        </label>
        <input id="daily-new-limit" type="number" min="0" max="500" bind:value={settings.daily_new_limit} class="number-input" />
      </div>

      <div class="field">
        <label for="leech-threshold">
          Leech Threshold
          <span class="hint">Cards suspended after this many lapses.</span>
        </label>
        <input id="leech-threshold" type="number" min="1" max="20" bind:value={settings.leech_threshold} class="number-input" />
      </div>

      <div class="actions">
        <button class="btn" on:click={save} disabled={saving}>
          {saving ? 'Saving…' : saved ? 'Saved!' : 'Save Settings'}
        </button>
        <button class="btn-ghost" on:click={resetToDefaults}>Reset to defaults</button>
      </div>
    </div>

    <div class="section">
      <h2>FSRS-6 Parameters</h2>
      <p class="info-text">
        These 19 weights control the FSRS-6 scheduling algorithm. They are automatically
        optimized based on your review history (optimizer runs after 400+ reviews).
        Current values are {settings.is_customized ? 'customized' : 'default FSRS-6 parameters'}.
      </p>
      {#if settings.weights?.length === 19}
        <div class="weights-grid">
          {#each settings.weights as w, i}
            <div class="weight-cell" title="w[{i}]">
              <span class="weight-index">w[{i}]</span>
              <span class="weight-val">{w.toFixed(4)}</span>
            </div>
          {/each}
        </div>
      {/if}
      <div class="optimizer-row">
        <button class="btn" on:click={runOptimizer} disabled={optimizing}>
          {optimizing ? 'Optimizing…' : 'Optimize my parameters'}
        </button>
        {#if optimizeResult}
          {#if optimizeResult.optimized}
            <span class="opt-success">✓ Optimized using {optimizeResult.events_used} reviews</span>
          {:else}
            <span class="opt-info">{optimizeResult.reason}</span>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</main>

<style>
  main { max-width: 680px; margin: 0 auto; padding: 1.5rem 1rem; }

  .msg { text-align: center; margin-top: 3rem; color: #9ba8c0; }

  .section { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.5rem; margin-bottom: 1.5rem; }
  .section h2 { margin: 0 0 1.25rem; font-size: 1.1rem; }

  .field { margin-bottom: 1.25rem; }
  label { display: block; font-size: 0.95rem; font-weight: 500; margin-bottom: 0.5rem; }
  .hint { display: block; font-size: 0.78rem; color: #6b7591; font-weight: 400; margin-top: 0.15rem; }

  .slider-row { display: flex; align-items: center; gap: 1rem; }
  input[type=range] { flex: 1; accent-color: #4caf50; }
  .pct { font-size: 1rem; font-weight: 600; color: #4caf50; min-width: 3rem; text-align: right; }

  .preview-text { font-size: 0.82rem; color: #9ba8c0; margin: 0.4rem 0 0; }

  .number-input { background: #13151a; border: 1px solid #3a3d47; color: #e8eaf0; padding: 0.45rem 0.75rem; border-radius: 6px; font-size: 1rem; width: 90px; }

  .actions { display: flex; gap: 1rem; align-items: center; margin-top: 1.5rem; }

  .btn { background: #4caf50; color: #fff; border: none; padding: 0.65rem 1.5rem; border-radius: 7px; font-size: 0.95rem; font-weight: 500; cursor: pointer; transition: background 0.15s; }
  .btn:hover:not(:disabled) { background: #43a047; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }

  .btn-ghost { background: transparent; border: 1px solid #3a3d47; color: #9ba8c0; padding: 0.6rem 1.2rem; border-radius: 7px; font-size: 0.9rem; cursor: pointer; }
  .btn-ghost:hover { background: #2a2d36; }

  .info-text { font-size: 0.85rem; color: #9ba8c0; margin: 0 0 1rem; line-height: 1.6; }

  .weights-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(90px, 1fr)); gap: 0.5rem; }

  .optimizer-row { display: flex; align-items: center; gap: 1rem; margin-top: 1rem; flex-wrap: wrap; }
  .opt-success { color: #4caf50; font-size: 0.85rem; }
  .opt-info { color: #9ba8c0; font-size: 0.85rem; }
  .weight-cell { background: #13151a; border: 1px solid #2a2d36; border-radius: 6px; padding: 0.4rem 0.6rem; text-align: center; }
  .weight-index { display: block; font-size: 0.65rem; color: #6b7591; }
  .weight-val { font-size: 0.8rem; font-family: monospace; color: #b0bec5; }
</style>
