<script lang="ts">
  export let padding: 'sm' | 'md' | 'lg' = 'md';
  export let interactive = false;
  let className = '';
  export { className as class };
</script>

{#if interactive}
  <button type="button" class="card {padding} interactive {className}" on:click>
    <slot />
  </button>
{:else}
  <div class="card {padding} {className}">
    <slot />
  </div>
{/if}

<style>
  .card {
    background: var(--c-bgRaised);
    border: 1px solid var(--c-border);
    border-radius: var(--r-lg);
    color: inherit;
    text-align: left;
    width: 100%;
    font: inherit;
  }
  button.card { display: block; }
  .sm { padding: var(--s-3); }
  .md { padding: var(--s-5); }
  .lg { padding: var(--s-6) var(--s-6); }

  .interactive {
    cursor: pointer;
    transition: border-color var(--m-fast) var(--m-easing),
                background   var(--m-fast) var(--m-easing);
  }
  .interactive:hover { border-color: var(--c-green); background: color-mix(in srgb, var(--c-bgRaised) 95%, var(--c-green) 5%); }
  .interactive:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c-green) 40%, transparent);
  }
</style>
