<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';

  export let open = false;
  export let title: string | null = null;
  export let dismissible = true;

  const dispatch = createEventDispatcher();

  let modalEl: HTMLDivElement | null = null;
  let prevFocus: HTMLElement | null = null;

  function close() {
    if (!dismissible) return;
    open = false;
    dispatch('close');
  }

  function trap(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') { close(); return; }
    if (e.key !== 'Tab' || !modalEl) return;
    const focusables = modalEl.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
    if (focusables.length === 0) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (e.shiftKey && document.activeElement === first) { last.focus(); e.preventDefault(); }
    else if (!e.shiftKey && document.activeElement === last) { first.focus(); e.preventDefault(); }
  }

  $: if (open && typeof document !== 'undefined') {
    prevFocus = document.activeElement as HTMLElement;
    setTimeout(() => modalEl?.querySelector<HTMLElement>('button, [href], input, [tabindex]')?.focus(), 10);
  }
  $: if (!open && prevFocus) { prevFocus.focus(); prevFocus = null; }

  onMount(() => window.addEventListener('keydown', trap));
  onDestroy(() => typeof window !== 'undefined' && window.removeEventListener('keydown', trap));
</script>

{#if open}
  <div class="overlay" role="presentation">
    <button class="scrim" aria-label="Close dialog" tabindex="-1" on:click={close}></button>
    <div
      class="modal"
      bind:this={modalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby={title ? 'modal-title' : undefined}
    >
      {#if title}
        <div class="head">
          <h2 id="modal-title">{title}</h2>
          {#if dismissible}
            <button class="x" on:click={close} aria-label="Close dialog">✕</button>
          {/if}
        </div>
      {/if}
      <div class="body"><slot /></div>
      {#if $$slots.footer}
        <div class="footer"><slot name="footer" /></div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed; inset: 0;
    display: flex; align-items: center; justify-content: center;
    z-index: 100;
    padding: var(--s-4);
  }
  .scrim {
    position: absolute; inset: 0;
    background: rgba(0,0,0,0.6);
    border: none; cursor: default;
  }
  .modal {
    background: var(--c-bgRaised);
    border: 1px solid var(--c-border);
    border-radius: var(--r-xl);
    max-width: 560px; width: 100%;
    max-height: 90vh; overflow: auto;
    display: flex; flex-direction: column;
  }
  .head {
    display: flex; align-items: center; justify-content: space-between;
    padding: var(--s-5) var(--s-6) var(--s-3);
    border-bottom: 1px solid var(--c-border);
  }
  h2 { margin: 0; font-size: 1.05rem; color: var(--c-textHi); }
  .x {
    background: transparent; border: none; color: var(--c-textMuted);
    cursor: pointer; font-size: 1rem; padding: var(--s-1) var(--s-2); border-radius: var(--r-sm);
  }
  .x:hover { color: var(--c-textHi); background: var(--c-border); }
  .body   { padding: var(--s-6); }
  .footer { padding: var(--s-3) var(--s-6) var(--s-5); border-top: 1px solid var(--c-border); display: flex; justify-content: flex-end; gap: var(--s-2); }
</style>
