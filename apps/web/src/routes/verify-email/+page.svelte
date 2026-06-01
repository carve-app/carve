<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import Card from '$lib/design/Card.svelte';
  import Button from '$lib/design/Button.svelte';

  let state: 'idle' | 'verifying' | 'verified' | 'error' = 'idle';
  let message = '';

  onMount(async () => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');
    if (!token) {
      state = 'error';
      message = 'No verification token in the URL. Check your inbox for the latest email.';
      return;
    }
    state = 'verifying';
    try {
      await apiFetch<{ verified: boolean }>('/v1/auth/verify', {
        method: 'POST',
        body: JSON.stringify({ token }),
      });
      state = 'verified';
    } catch (e) {
      state = 'error';
      message = e instanceof Error ? e.message : 'Verification failed';
    }
  });

  let resendEmail = '';
  let resending = false;
  let resent = false;
  async function resend() {
    resending = true; resent = false;
    try {
      await apiFetch('/v1/auth/verify/resend', {
        method: 'POST',
        body: JSON.stringify({ email: resendEmail }),
      });
      resent = true;
    } catch {
      // Anti-enumeration: always succeed silently.
      resent = true;
    }
    resending = false;
  }
</script>

<main>
  <Card padding="lg" class="hero">
    {#if state === 'verifying'}
      <h1>Verifying your email…</h1>
      <p class="muted">One moment.</p>
    {:else if state === 'verified'}
      <h1>You're all set</h1>
      <p class="muted">Your email is verified. Sign in to continue.</p>
      <Button href="/login">Go to login</Button>
    {:else}
      <h1>We couldn't verify that link</h1>
      <p class="muted">{message}</p>
      <form on:submit|preventDefault={resend}>
        <input
          type="email" required class="text-input"
          placeholder="email@example.com"
          aria-label="Email to resend verification to"
          bind:value={resendEmail}
        />
        <Button type="submit" loading={resending}>Send a new link</Button>
      </form>
      {#if resent}
        <p class="ok">If that email is on file, we just sent a fresh verification link.</p>
      {/if}
    {/if}
  </Card>
</main>

<style>
  main { max-width: 480px; margin: 4rem auto; padding: var(--s-4); }
  h1 { font-size: 1.3rem; margin: 0 0 var(--s-3); color: var(--c-textHi); }
  .muted { color: var(--c-textMuted); margin: 0 0 var(--s-4); }
  form { display: flex; gap: var(--s-2); }
  .text-input {
    background: var(--c-bg);
    color: var(--c-textHi);
    border: 1px solid var(--c-border);
    border-radius: var(--r-md);
    padding: 0.5rem 0.75rem;
    flex: 1;
  }
  .ok { color: var(--c-green); margin-top: var(--s-3); font-size: 0.85rem; }
</style>
