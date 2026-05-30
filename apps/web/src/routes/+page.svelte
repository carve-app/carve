<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  onMount(() => {
    if (localStorage.getItem('carve_access_token')) {
      goto('/review');
    }
  });
</script>

<main>
  <div class="hero">
    <h1>Carve</h1>
    <p>Science-grounded language learning.</p>
    <a href="/login" class="cta">Get started</a>
  </div>
</main>

<style>
  main {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
  }

  .hero {
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
  }

  h1 {
    font-size: 3.5rem;
    font-weight: 800;
    color: #4caf50;
    margin: 0;
    letter-spacing: -0.04em;
  }

  p {
    color: #9ba8c0;
    margin: 0;
    font-size: 1.1rem;
  }

  .cta {
    display: inline-block;
    background: #4caf50;
    color: #fff;
    padding: 0.75rem 2rem;
    border-radius: 8px;
    text-decoration: none;
    font-weight: 600;
    font-size: 1rem;
    margin-top: 0.5rem;
    transition: background 0.15s;
  }
  .cta:hover { background: #43a047; }
</style>
