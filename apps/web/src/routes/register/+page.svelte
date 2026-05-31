<script lang="ts">
  import { register, ApiError } from '$lib/api';

  let displayName = '';
  let email = '';
  let password = '';
  let errorMessage = '';
  let loading = false;

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    errorMessage = '';
    loading = true;

    try {
      const res = await register(email, password, displayName);
      localStorage.setItem('carve_access_token', res.access_token);
      window.location.href = '/onboarding';
    } catch (err) {
      if (err instanceof ApiError) {
        errorMessage =
          err.status === 409
            ? 'An account with that email already exists.'
            : err.status === 400
            ? err.message
            : `Registration failed: ${err.message}`;
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
    <h1>Create your account</h1>

    <form on:submit={handleSubmit}>
      <label for="name">Name</label>
      <input
        id="name"
        type="text"
        bind:value={displayName}
        placeholder="Your name"
        autocomplete="name"
        required
        disabled={loading}
      />

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
        placeholder="At least 8 characters"
        autocomplete="new-password"
        minlength="8"
        required
        disabled={loading}
      />

      {#if errorMessage}
        <p class="error">{errorMessage}</p>
      {/if}

      <button type="submit" disabled={loading}>
        {loading ? 'Creating account…' : 'Create account'}
      </button>
    </form>

    <p class="footer">
      Already have an account? <a href="/login">Sign in</a>
    </p>
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

  input:focus { border-color: #4caf50; }
  input:disabled { opacity: 0.6; }

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

  button:hover:not(:disabled) { background: #43a047; }
  button:disabled { opacity: 0.7; cursor: not-allowed; }

  .footer {
    text-align: center;
    margin: 1.25rem 0 0;
    font-size: 0.85rem;
    color: #9ba8c0;
  }

  .footer a { color: #4caf50; text-decoration: none; }
  .footer a:hover { text-decoration: underline; }
</style>
