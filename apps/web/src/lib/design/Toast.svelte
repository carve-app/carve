<script lang="ts">
  import { toasts } from '$lib/stores/toast';
</script>

<div class="toast-region" aria-live="polite" aria-atomic="false">
  {#each $toasts as toast (toast.id)}
    <div class="toast toast-{toast.type}" role="alert">
      <span class="toast-msg">{toast.message}</span>
      <button class="toast-close" on:click={() => toasts.remove(toast.id)} aria-label="Dismiss">✕</button>
    </div>
  {/each}
</div>

<style>
  .toast-region {
    position: fixed;
    bottom: 1.5rem;
    right: 1.5rem;
    z-index: 9000;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    max-width: 360px;
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 0.7rem 1rem;
    border-radius: 8px;
    font-size: 0.88rem;
    line-height: 1.45;
    pointer-events: all;
    animation: slide-in 0.18s ease;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.5);
  }

  .toast-msg { flex: 1; }

  .toast-close {
    background: none;
    border: none;
    cursor: pointer;
    opacity: 0.55;
    font-size: 0.85rem;
    padding: 0;
    line-height: 1;
    flex-shrink: 0;
    margin-top: 1px;
    transition: opacity 0.1s;
  }
  .toast-close:hover { opacity: 1; }

  .toast-error  { background: #2d1515; border: 1px solid #c62828; color: #ef9a9a; }
  .toast-error .toast-close { color: #ef9a9a; }

  .toast-success { background: #1a2e1a; border: 1px solid #388e3c; color: #a5d6a7; }
  .toast-success .toast-close { color: #a5d6a7; }

  .toast-info   { background: #1a2240; border: 1px solid #1565c0; color: #90caf9; }
  .toast-info .toast-close { color: #90caf9; }

  @keyframes slide-in {
    from { transform: translateX(110%); opacity: 0; }
    to   { transform: translateX(0);    opacity: 1; }
  }
</style>
