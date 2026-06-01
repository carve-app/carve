<script lang="ts">
  import { onMount } from 'svelte';
  import Toast from '$lib/design/Toast.svelte';
  import InstallPrompt from '$lib/design/InstallPrompt.svelte';
  import { cssVariables } from '$lib/design/tokens';

  onMount(() => {
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.register('/sw.js').catch(() => {});
    }
  });
</script>

<svelte:head>
  <title>Carve</title>
  <link rel="manifest" href="/manifest.webmanifest" />
  <link rel="icon" type="image/png" sizes="32x32" href="/favicon-32.png" />
  <link rel="icon" type="image/png" sizes="16x16" href="/favicon-16.png" />
  <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png" />
  <meta name="theme-color" content="#4caf50" />
  <meta name="apple-mobile-web-app-capable" content="yes" />
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
  {@html `<style>${cssVariables()}</style>`}
</svelte:head>

<slot />
<Toast />
<InstallPrompt />

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; }

  :global(body) {
    background: var(--c-bg);
    color: var(--c-textHi);
    font-family: system-ui, -apple-system, sans-serif;
    margin: 0;
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
  }

  :global(a) { color: inherit; }
  /* axe link-in-text-block: inline links inside text containers must be
     distinguishable without relying on color alone. */
  :global(p a),
  :global(li a),
  :global(.prose a) { text-decoration: underline; }
  :global(button) { font-family: inherit; }
  :global(:focus-visible) {
    outline: none;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c-green) 40%, transparent);
    border-radius: var(--r-sm);
  }
</style>
