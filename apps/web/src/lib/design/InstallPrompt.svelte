<script lang="ts">
  import { onMount } from 'svelte';

  let deferredPrompt: any = null;
  let visible = false;

  onMount(() => {
    if (typeof window === 'undefined') return;
    if (localStorage.getItem('carve_install_dismissed') === '1') return;
    if (window.matchMedia('(display-mode: standalone)').matches) return;

    const onBefore = (e: Event) => {
      e.preventDefault();
      deferredPrompt = e;
      const reviewedOnce = localStorage.getItem('carve_reviewed_once') === '1';
      if (reviewedOnce) visible = true;
    };
    window.addEventListener('beforeinstallprompt', onBefore);
    return () => window.removeEventListener('beforeinstallprompt', onBefore);
  });

  async function install() {
    if (!deferredPrompt) { visible = false; return; }
    deferredPrompt.prompt();
    await deferredPrompt.userChoice.catch(() => {});
    deferredPrompt = null;
    visible = false;
  }

  function dismiss() {
    localStorage.setItem('carve_install_dismissed', '1');
    visible = false;
  }
</script>

{#if visible}
  <div class="install-banner" role="dialog" aria-label="Install Carve">
    <div class="install-text">
      <div class="install-title">Install Carve</div>
      <div class="install-sub">Add to your home screen for one-tap review and offline support.</div>
    </div>
    <div class="install-actions">
      <button class="install-ghost" on:click={dismiss}>Not now</button>
      <button class="install-primary" on:click={install}>Install</button>
    </div>
  </div>
{/if}

<style>
  .install-banner {
    position: fixed;
    left: 1rem;
    right: 1rem;
    bottom: 1rem;
    max-width: 480px;
    margin: 0 auto;
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 12px;
    padding: 0.85rem 1rem;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    z-index: 50;
  }
  .install-text { flex: 1; min-width: 0; }
  .install-title { font-weight: 600; color: #e8eaf0; font-size: 0.9rem; }
  .install-sub { color: #7a8aa6; font-size: 0.78rem; margin-top: 0.15rem; }
  .install-actions { display: flex; gap: 0.4rem; }
  .install-ghost {
    background: transparent; border: 1px solid #2a2d36; color: #9ba8c0;
    padding: 0.4rem 0.7rem; border-radius: 6px; font-size: 0.78rem; cursor: pointer;
  }
  .install-primary {
    background: #2e7d32; border: none; color: #fff;
    padding: 0.4rem 0.9rem; border-radius: 6px; font-size: 0.78rem;
    font-weight: 600; cursor: pointer;
  }
</style>
