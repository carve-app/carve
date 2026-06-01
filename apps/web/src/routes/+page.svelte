<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  onMount(() => {
    if (localStorage.getItem('carve_access_token')) {
      goto('/review');
    }
  });

  const FEATURES = [
    {
      icon: '🎯',
      title: 'Mine any page',
      body: 'Hover any word on any website or video to see its definition. One click adds it to your deck.',
    },
    {
      icon: '🧠',
      title: 'FSRS-6 scheduling',
      body: 'Reviews scheduled by the most accurate spaced-repetition algorithm, personalised to your memory.',
    },
    {
      icon: '📊',
      title: 'Know your level',
      body: 'See your comprehension percentage for any content you add. Track your known-word count over time.',
    },
  ];
</script>

<div class="page">
  <nav class="nav">
    <a href="/" class="nav-brand">Carve</a>
    <div class="nav-links">
      <a href="/login" class="nav-link">Sign in</a>
      <a href="/register" class="cta-small">Get started free</a>
    </div>
  </nav>

  <section class="hero">
    <div class="hero-inner">
      <div class="hero-badge">Open beta · Free to use</div>
      <h1>Learn languages<br/>the way your brain works</h1>
      <p class="hero-sub">
        Carve is a vocabulary mining and spaced-repetition app built for serious language learners.
        Mine words from anything you read or watch, review them with FSRS-6, and track your progress.
      </p>
      <div class="hero-actions">
        <a href="/register" class="btn-primary">Start for free →</a>
        <a href="/login" class="btn-ghost">Sign in</a>
      </div>
    </div>
  </section>

  <section class="features">
    <div class="features-inner">
      {#each FEATURES as f}
        <div class="feature-card">
          <div class="feature-icon">{f.icon}</div>
          <h3>{f.title}</h3>
          <p>{f.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <section class="install">
    <div class="install-inner">
      <h2>Mine words from anywhere</h2>
      <p>Install the browser extension to start mining vocabulary from any webpage or video subtitle.</p>
      <div class="install-buttons">
        <a href="/register" class="btn-primary">Create free account</a>
      </div>
    </div>
  </section>

  <footer class="footer">
    <span class="footer-brand">Carve</span>
    <span class="footer-copy">© {new Date().getFullYear()}</span>
  </footer>
</div>

<style>
  .page {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }

  /* ── Nav ── */

  .nav {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 2rem;
    border-bottom: 1px solid #1e2128;
    position: sticky;
    top: 0;
    background: #13151a;
    z-index: 10;
  }

  .nav-brand {
    font-size: 1.15rem;
    font-weight: 800;
    color: #4caf50;
    text-decoration: none;
    letter-spacing: -0.03em;
  }

  .nav-links {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .nav-link {
    color: #9ba8c0;
    text-decoration: none;
    font-size: 0.9rem;
    transition: color 0.1s;
  }
  .nav-link:hover { color: #e8eaf0; }

  .cta-small {
    background: #2e7d32;
    color: #fff;
    padding: 0.45rem 1rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 600;
    text-decoration: none;
    transition: background 0.15s;
  }
  .cta-small:hover { background: #43a047; }

  /* ── Hero ── */

  .hero {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 5rem 2rem 4rem;
  }

  .hero-inner {
    max-width: 640px;
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.25rem;
  }

  .hero-badge {
    display: inline-block;
    background: #162316;
    color: #4caf50;
    border: 1px solid #2a3d2a;
    padding: 0.3rem 0.8rem;
    border-radius: 20px;
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.02em;
  }

  h1 {
    font-size: clamp(2.2rem, 6vw, 3.5rem);
    font-weight: 800;
    color: #e8eaf0;
    line-height: 1.12;
    letter-spacing: -0.04em;
    margin: 0;
  }

  .hero-sub {
    color: #7a8aa6;
    font-size: 1.05rem;
    line-height: 1.65;
    max-width: 520px;
    margin: 0;
  }

  .hero-actions {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
    justify-content: center;
    margin-top: 0.5rem;
  }

  /* ── Shared buttons ── */

  .btn-primary {
    display: inline-block;
    background: #2e7d32;
    color: #fff;
    padding: 0.75rem 1.75rem;
    border-radius: 8px;
    text-decoration: none;
    font-weight: 600;
    font-size: 1rem;
    transition: background 0.15s;
  }
  .btn-primary:hover { background: #43a047; }

  .btn-ghost {
    display: inline-block;
    background: transparent;
    color: #9ba8c0;
    border: 1px solid #2a2d36;
    padding: 0.75rem 1.75rem;
    border-radius: 8px;
    text-decoration: none;
    font-size: 1rem;
    transition: border-color 0.15s, color 0.15s;
  }
  .btn-ghost:hover { border-color: #8a96b3; color: #c8d0e0; }

  /* ── Features ── */

  .features {
    padding: 4rem 2rem;
    background: #0f1116;
    border-top: 1px solid #1e2128;
    border-bottom: 1px solid #1e2128;
  }

  .features-inner {
    max-width: 900px;
    margin: 0 auto;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    gap: 1.5rem;
  }

  .feature-card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1.5rem;
  }

  .feature-icon { font-size: 2rem; margin-bottom: 0.75rem; }

  .feature-card h3 {
    margin: 0 0 0.5rem;
    font-size: 1rem;
    font-weight: 600;
    color: #e8eaf0;
  }

  .feature-card p {
    margin: 0;
    font-size: 0.88rem;
    color: #7a8aa6;
    line-height: 1.6;
  }

  /* ── Install CTA ── */

  .install {
    padding: 5rem 2rem;
    text-align: center;
  }

  .install-inner {
    max-width: 500px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
  }

  .install h2 {
    margin: 0;
    font-size: 1.8rem;
    font-weight: 700;
    color: #e8eaf0;
    letter-spacing: -0.03em;
  }

  .install p {
    margin: 0;
    color: #7a8aa6;
    font-size: 0.95rem;
    line-height: 1.6;
  }

  .install-buttons { margin-top: 0.5rem; }

  /* ── Footer ── */

  .footer {
    padding: 1.5rem 2rem;
    border-top: 1px solid #1e2128;
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.82rem;
  }

  .footer-brand { font-weight: 700; color: #4caf50; }
  .footer-copy { color: #8a96b3; }
</style>
