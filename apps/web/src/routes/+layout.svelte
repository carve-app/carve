<script lang="ts">
  import { onMount } from 'svelte';
  import Toast from '$lib/design/Toast.svelte';
  import InstallPrompt from '$lib/design/InstallPrompt.svelte';

  onMount(() => {
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.register('/sw.js').catch(() => {});
    }
  });
</script>

<svelte:head>
  <link rel="manifest" href="/manifest.webmanifest" />
  <meta name="theme-color" content="#4caf50" />
  <meta name="apple-mobile-web-app-capable" content="yes" />
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
</svelte:head>

<slot />
<Toast />
<InstallPrompt />

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
