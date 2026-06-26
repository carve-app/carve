<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';

  const API_BASE: string = import.meta.env.VITE_API_BASE ?? 'http://localhost:8080';

  let token = '';
  let password = '';
  let confirm = '';
  let loading = false;
  let done = false;
  let errorMsg = '';

  onMount(() => {
    token = $page.url.searchParams.get('token') ?? '';
    if (!token) {
      errorMsg = 'Invalid or missing reset token.';
    }
  });

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    errorMsg = '';

    if (password.length < 8) {
      errorMsg = 'Password must be at least 8 characters.';
      return;
    }
    if (password !== confirm) {
      errorMsg = 'Passwords do not match.';
      return;
    }

    loading = true;
    try {
      const res = await fetch(`${API_BASE}/v1/auth/reset`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        errorMsg = (body as { error?: string }).error ?? 'Reset failed. The link may have expired.';
        return;
      }
      done = true;
    } catch {
      errorMsg = 'Could not reach API — is the server running?';
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head>
  <title>Reset password — Carve</title>
</svelte:head>

<main>
  <div class="card">
    {#if done}
      <h1>Password updated</h1>
      <p class="info">Your password has been reset. You can now sign in with your new password.</p>
      <a href="/login" class="btn">Sign in</a>
    {:else}
      <h1>Set new password</h1>
      {#if errorMsg}
        <div class="error-msg">{errorMsg}</div>
      {/if}
      <form on:submit={handleSubmit}>
        <label for="password">New password</label>
        <input
          id="password"
          type="password"
          bind:value={password}
          placeholder="At least 8 characters"
          autocomplete="new-password"
          required
          minlength="8"
          disabled={loading || !token}
        />
        <label for="confirm">Confirm password</label>
        <input
          id="confirm"
          type="password"
          bind:value={confirm}
          placeholder="Repeat password"
          autocomplete="new-password"
          required
          disabled={loading || !token}
        />
        <button type="submit" disabled={loading || !token}>
          {loading ? 'Saving…' : 'Set new password'}
        </button>
      </form>
      <a href="/login" class="back">Back to sign in</a>
    {/if}
  </div>
</main>

<style>
  main {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #0f1116;
    padding: 2rem 1rem;
  }

  .card {
    background: #181c24;
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 12px;
    padding: 2.5rem 2rem;
    width: 100%;
    max-width: 400px;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  h1 {
    font-size: 1.5rem;
    font-weight: 700;
    color: #e8eaf0;
    margin: 0;
  }

  .info {
    color: #9ba8c0;
    font-size: 0.95rem;
    line-height: 1.6;
    margin: 0;
  }

  .error-msg {
    background: rgba(239, 83, 80, 0.12);
    border: 1px solid rgba(239, 83, 80, 0.3);
    color: #ef9a9a;
    border-radius: 6px;
    padding: 0.6rem 0.85rem;
    font-size: 0.875rem;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  label {
    font-size: 0.875rem;
    color: #9ba8c0;
    margin-bottom: -0.4rem;
  }

  input {
    width: 100%;
    padding: 0.65rem 0.85rem;
    background: #0f1116;
    border: 1px solid rgba(255,255,255,0.12);
    border-radius: 7px;
    color: #e8eaf0;
    font-size: 0.95rem;
    box-sizing: border-box;
  }

  input:focus {
    outline: none;
    border-color: #7986cb;
  }

  button {
    margin-top: 0.25rem;
    padding: 0.7rem;
    background: #3949ab;
    color: #fff;
    border: none;
    border-radius: 8px;
    font-size: 0.95rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
  }

  button:disabled { cursor: default; background: #4a5263; color: #c8d0e0; }

  .btn {
    display: block;
    text-align: center;
    padding: 0.7rem;
    background: #3949ab;
    color: #fff;
    border-radius: 8px;
    font-size: 0.95rem;
    font-weight: 600;
    text-decoration: none;
  }

  .back {
    text-align: center;
    color: #9ba8c0;
    font-size: 0.875rem;
    text-decoration: underline;
  }

  .back:hover { color: #e8eaf0; }
</style>
