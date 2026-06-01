<script lang="ts">
  import { triggerExport } from '$lib/api';
  import { toasts } from '$lib/stores/toast';

  let exporting = false;
  let done = false;

  async function doExport() {
    exporting = true;
    done = false;
    try {
      await triggerExport();
      done = true;
      setTimeout(() => { done = false; }, 3000);
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Export failed');
    }
    exporting = false;
  }
</script>

<main>
  <div class="section">
    <h2>Download your data</h2>
    <p class="info">
      Export all your cards, review history, and immersion sessions as a single JSON file.
      Your data belongs to you — export any time, no restrictions.
    </p>

    <div class="format-block">
      <h3>What's included</h3>
      <ul>
        <li>All cards (front, back, FSRS state, stability, difficulty)</li>
        <li>Full review event log (every rating, timestamp, time taken)</li>
        <li>Immersion sessions (reading and listening time)</li>
        <li>User profile</li>
      </ul>
    </div>

    {#if done}
      <p class="success">Download started!</p>
    {/if}

    <button class="btn" on:click={doExport} disabled={exporting}>
      {exporting ? 'Preparing export…' : 'Download JSON export'}
    </button>
  </div>
</main>

<style>
  main { max-width: 620px; margin: 0 auto; padding: 1.5rem 1rem; }

  .section { background: #1e2128; border: 1px solid #2a2d36; border-radius: 10px; padding: 1.5rem; }
  .section h2 { margin: 0 0 0.75rem; font-size: 1.1rem; }
  .info { color: #9ba8c0; font-size: 0.92rem; line-height: 1.6; margin: 0 0 1.25rem; }

  .format-block { background: #13151a; border: 1px solid #2a2d36; border-radius: 8px; padding: 1rem 1.25rem; margin-bottom: 1.5rem; }
  .format-block h3 { margin: 0 0 0.6rem; font-size: 0.92rem; color: #9ba8c0; }
  ul { margin: 0; padding-left: 1.4rem; }
  li { font-size: 0.88rem; color: #b0bec5; margin-bottom: 0.3rem; }

  .btn { background: #2e7d32; color: #fff; border: none; padding: 0.7rem 1.75rem; border-radius: 7px; font-size: 1rem; font-weight: 500; cursor: pointer; transition: background 0.15s; }
  .btn:hover:not(:disabled) { background: #43a047; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }

  .success { color: #4caf50; font-size: 0.9rem; margin-bottom: 0.75rem; }
</style>
