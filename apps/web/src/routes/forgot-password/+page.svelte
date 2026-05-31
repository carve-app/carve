<script lang="ts">
  const API_BASE: string = import.meta.env.VITE_API_BASE ?? 'http://localhost:8080';

  let submitted = false;
  let email = '';
  let loading = false;
  let errorMsg = '';

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    errorMsg = '';
    loading = true;
    try {
      await fetch(`${API_BASE}/v1/auth/forgot`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      });
      // Always show success regardless of whether email exists (no enumeration).
      submitted = true;
    } catch {
      errorMsg = 'Could not reach the server. Please try again.';
    } finally {
      loading = false;
    }
  }
</script>

<main>
  <div class="card">
    {#if submitted}
      <h1>Check your email</h1>
      <p class="info">
        If <strong>{email}</strong> has an account, we'll send a reset link shortly.
      </p>
      <a href="/login" class="btn">Back to sign in</a>
    {:else}
      <h1>Reset your password</h1>
      <p class="info">Enter your email and we'll send you a reset link.</p>
      {#if errorMsg}<div class="error-msg">{errorMsg}</div>{/if}
      <form on:submit={handleSubmit}>
        <label for="email">Email</label>
        <input
          id="email"
          type="email"
          bind:value={email}
          placeholder="you@example.com"
          autocomplete="email"
          required
          disabled={loading}
        />
        <button type="submit" disabled={loading}>
          {loading ? 'Sending…' : 'Send reset link'}
        </button>
      </form>
      <p class="footer">
        <a href="/login">← Back to sign in</a>
      </p>
    {/if}
  </div>
</main>

<style>
  main {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    padding: 1rem;
  }

  .card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 2rem;
    width: 100%;
    max-width: 380px;
  }

  h1 {
    margin: 0 0 0.75rem;
    font-size: 1.4rem;
    color: #e8eaf0;
    text-align: center;
  }

  .info {
    color: #9ba8c0;
    font-size: 0.9rem;
    text-align: center;
    margin: 0 0 1.5rem;
    line-height: 1.5;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  label {
    font-size: 0.85rem;
    color: #9ba8c0;
  }

  input {
    padding: 0.65rem 0.75rem;
    background: #13151a;
    border: 1px solid #2a2d36;
    border-radius: 6px;
    color: #e8eaf0;
    font-size: 0.95rem;
    outline: none;
    transition: border-color 0.15s;
  }
  input:focus { border-color: #4caf50; }
  input:disabled { opacity: 0.6; }

  button, .btn {
    display: block;
    width: 100%;
    margin-top: 1rem;
    padding: 0.7rem;
    background: #4caf50;
    color: #fff;
    border: none;
    border-radius: 6px;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
    text-align: center;
    text-decoration: none;
    transition: background 0.15s;
  }
  button:hover:not(:disabled), .btn:hover { background: #43a047; }
  button:disabled { opacity: 0.7; cursor: not-allowed; }

  .footer {
    text-align: center;
    margin: 1.25rem 0 0;
    font-size: 0.85rem;
  }
  .footer a { color: #9ba8c0; text-decoration: none; }
  .footer a:hover { color: #e8eaf0; }

  .error-msg {
    background: rgba(239, 83, 80, 0.12);
    border: 1px solid rgba(239, 83, 80, 0.3);
    color: #ef9a9a;
    border-radius: 6px;
    padding: 0.5rem 0.75rem;
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
  }
</style>
