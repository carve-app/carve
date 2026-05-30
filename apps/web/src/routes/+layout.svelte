<script lang="ts">
  import Toast from '$lib/design/Toast.svelte';
</script>

<slot />
<Toast />

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; }

  :global(body) {
    background: #13151a;
    color: #e8eaf0;
    font-family: system-ui, -apple-system, sans-serif;
    margin: 0;
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
  }

  :global(a) { color: inherit; }

  :global(button) { font-family: inherit; }
</style>
