<script lang="ts">
  export let title: string;
  export let body: string | null = null;
  export let icon: string | null = null;          // optional emoji or text mark
</script>

<div class="empty" role="status" aria-live="polite">
  {#if icon}<div class="icon" aria-hidden="true">{icon}</div>{/if}
  <h3>{title}</h3>
  {#if body}<p>{body}</p>{/if}
  {#if $$slots.default}
    <div class="action"><slot /></div>
  {/if}
</div>

<style>
  .empty {
    display: flex; flex-direction: column;
    align-items: center; justify-content: center;
    padding: var(--s-10) var(--s-6);
    text-align: center;
    color: var(--c-textMuted);
    gap: var(--s-3);
  }
  .icon { font-size: 2.5rem; opacity: 0.6; }
  h3 {
    margin: 0; font-size: 1.05rem; font-weight: 600;
    color: var(--c-text);
  }
  p {
    margin: 0; font-size: 0.88rem; max-width: 40ch;
    color: var(--c-textMuted); line-height: 1.5;
  }
  .action { margin-top: var(--s-2); }
</style>
