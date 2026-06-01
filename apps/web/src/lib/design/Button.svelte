<script lang="ts">
  /**
   * Button primitive. Variants map to the brand palette in tokens.ts.
   * Always renders a real <button> (or <a> when `href` is set), so keyboard
   * navigation and screen readers get full semantics for free.
   */
  export let variant: 'primary' | 'secondary' | 'ghost' | 'danger' = 'primary';
  export let size: 'sm' | 'md' | 'lg' = 'md';
  export let disabled = false;
  export let loading = false;
  export let href: string | null = null;
  export let type: 'button' | 'submit' | 'reset' = 'button';
  export let fullWidth = false;
  let className = '';
  export { className as class };
</script>

{#if href}
  <a
    {href}
    class="btn {variant} {size} {className}"
    class:fullWidth
    class:loading
    aria-disabled={disabled || loading || undefined}
    on:click
  >
    {#if loading}<span class="spin" aria-hidden="true">◌</span>{/if}
    <slot />
  </a>
{:else}
  <button
    {type}
    class="btn {variant} {size} {className}"
    class:fullWidth
    class:loading
    disabled={disabled || loading}
    on:click
  >
    {#if loading}<span class="spin" aria-hidden="true">◌</span>{/if}
    <slot />
  </button>
{/if}

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    font-family: inherit;
    font-weight: 600;
    border: 1px solid transparent;
    border-radius: var(--r-md);
    cursor: pointer;
    transition: background var(--m-fast) var(--m-easing),
                color      var(--m-fast) var(--m-easing),
                border-color var(--m-fast) var(--m-easing);
    text-decoration: none;
    user-select: none;
  }
  .btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c-green) 40%, transparent);
  }
  .btn:disabled,
  .btn[aria-disabled="true"] { opacity: 0.55; cursor: not-allowed; }

  /* Sizes */
  .sm { padding: 0.35rem 0.7rem;  font-size: var(--type-sm,   0.85rem); }
  .md { padding: 0.5rem  1rem;    font-size: var(--type-base, 0.95rem); }
  .lg { padding: 0.7rem  1.4rem;  font-size: var(--type-md,   1rem); }

  .fullWidth { width: 100%; }

  /* Variants */
  .primary   { background: var(--c-greenBtn);  color: #fff; }
  .primary:hover:not(:disabled)  { background: var(--c-greenBtnHi); }
  .secondary { background: var(--c-bgRaised);  color: var(--c-textHi); border-color: var(--c-border); }
  .secondary:hover:not(:disabled){ border-color: var(--c-green); color: var(--c-textHi); }
  .ghost     { background: transparent; color: var(--c-text); border-color: var(--c-border); }
  .ghost:hover:not(:disabled)    { color: var(--c-textHi); border-color: var(--c-borderHi); }
  .danger    { background: var(--c-dangerBtn);  color: #fff; }
  .danger:hover:not(:disabled)   { background: var(--c-dangerBtnHi); }

  .spin { display: inline-block; animation: rot 0.9s linear infinite; }
  @keyframes rot { to { transform: rotate(360deg); } }
  .loading > .spin + :global(*) { opacity: 0.7; }
</style>
