<script lang="ts">
  import { login, ApiError } from '$lib/api';

  let email = '';
  let password = '';
  let errorMessage = '';
  let loading = false;

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    errorMessage = '';
    loading = true;

    try {
      const res = await login(email, password);
      localStorage.setItem('carve_access_token', res.access_token);
      window.location.href = '/cards';
    } catch (err) {
      if (err instanceof ApiError) {
        errorMessage =
          err.status === 401
            ? 'Invalid email or password.'
            : `Login failed: ${err.message}`;
      } else {
        errorMessage = 'Could not reach the server. Is it running?';
      }
    } finally {
      loading = false;
    }
  }
</script>

<main>
  <div class="card">
    <h1>Log in to Carve</h1>

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

      <label for="password">Password</label>
      <input
        id="password"
        type="password"
        bind:value={password}
        placeholder="••••••••"
        autocomplete="current-password"
        required
        disabled={loading}
      />

      {#if errorMessage}
        <p class="error">{errorMessage}</p>
      {/if}

      <button type="submit" disabled={loading}>
        {loading ? 'Logging in…' : 'Log in'}
      </button>
    </form>

    <p class="footer">
      <a href="/">Back to home</a>
    </p>
  </div>
</main>

<style>
  :global(body) {
    background: #13151a;
    color: #e8eaf0;
    font-family: system-ui, sans-serif;
    margin: 0;
  }

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
    margin: 0 0 1.5rem;
    font-size: 1.4rem;
    color: #e8eaf0;
    text-align: center;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  label {
    font-size: 0.85rem;
    color: #9ba8c0;
    margin-top: 0.5rem;
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

  input:focus {
    border-color: #4caf50;
  }

  input:disabled {
    opacity: 0.6;
  }

  .error {
    color: #ef9a9a;
    font-size: 0.85rem;
    margin: 0.25rem 0;
  }

  button {
    margin-top: 1rem;
    padding: 0.7rem;
    background: #4caf50;
    color: #fff;
    border: none;
    border-radius: 6px;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }

  button:hover:not(:disabled) {
    background: #43a047;
  }

  button:disabled {
    opacity: 0.7;
    cursor: not-allowed;
  }

  .footer {
    text-align: center;
    margin: 1.25rem 0 0;
    font-size: 0.85rem;
  }

  .footer a {
    color: #9ba8c0;
    text-decoration: none;
  }

  .footer a:hover {
    color: #e8eaf0;
  }
</style>
