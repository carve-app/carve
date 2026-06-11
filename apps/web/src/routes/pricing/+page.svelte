<svelte:head>
  <title>Pricing — Carve</title>
</svelte:head>

<main>
  <div class="container">
    <h1>Free while we're in alpha</h1>
    <p class="subtitle">Carve is in open alpha. Every feature is free — no card limits, no trials, no card on file.</p>

    <div class="plans">
      <!-- Free alpha -->
      <div class="plan plan-featured">
        <div class="plan-badge">Open alpha</div>
        <h2 class="plan-name">Free</h2>
        <div class="plan-price"><span class="amount">$0</span></div>
        <p class="plan-desc">Everything Carve does, free for everyone during the alpha.</p>
        <ul class="features">
          <li><strong>Unlimited cards</strong></li>
          <li>Mine words from any page or video subtitle</li>
          <li>Audio + frame capture from video</li>
          <li>Full FSRS-6 scheduling</li>
          <li>Furigana &amp; pitch accent (Japanese)</li>
          <li>Web reader + comprehension stats</li>
          <li>Anki / Migaku / Yomitan / jpdb import</li>
        </ul>
        <a href="/register" class="btn btn-primary">Get started free</a>
      </div>
    </div>

    <p class="faq-note">
      Carve is early software under active development — expect rough edges, and please send feedback.
      See the <a href="/privacy">privacy policy</a> or email
      <a href="mailto:support@carve-app.com">support@carve-app.com</a>.
    </p>
  </div>
</main>

<style>
  main {
    min-height: 100vh;
    background: var(--bg, #0f1116);
    color: var(--text, #e8eaf0);
    padding: 4rem 1rem 3rem;
  }

  .container {
    max-width: 900px;
    margin: 0 auto;
    text-align: center;
  }

  h1 {
    font-size: 2.25rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
  }

  .subtitle {
    color: #9ba8c0;
    font-size: 1.1rem;
    margin-bottom: 3rem;
  }

  .plans {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 1.5rem;
    margin-bottom: 2.5rem;
    max-width: 380px;
    margin-left: auto;
    margin-right: auto;
  }

  .plan {
    background: #181c24;
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 12px;
    padding: 2rem 1.5rem;
    position: relative;
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .plan-featured {
    border-color: #7986cb;
    box-shadow: 0 0 0 1px #7986cb;
  }

  .plan-badge {
    position: absolute;
    top: -12px;
    left: 50%;
    transform: translateX(-50%);
    background: #3949ab;
    color: #fff;
    font-size: 0.72rem;
    font-weight: 600;
    padding: 0.2rem 0.75rem;
    border-radius: 20px;
    white-space: nowrap;
  }

  .plan-name {
    font-size: 1.125rem;
    font-weight: 700;
    color: #c5cae9;
  }

  .plan-price {
    display: flex;
    align-items: baseline;
    gap: 0.25rem;
  }

  .amount {
    font-size: 2rem;
    font-weight: 700;
  }

  .plan-desc {
    color: #9ba8c0;
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .features {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    flex: 1;
  }

  .features li {
    font-size: 0.9rem;
    color: #c5cae9;
    padding-left: 1.4rem;
    position: relative;
  }

  .features li::before {
    content: '✓';
    position: absolute;
    left: 0;
    color: #4caf50;
    font-weight: 700;
  }

  .btn {
    display: block;
    text-align: center;
    padding: 0.65rem 1rem;
    border-radius: 8px;
    font-size: 0.95rem;
    font-weight: 600;
    text-decoration: none;
    transition: opacity 0.15s;
    margin-top: auto;
  }

  .btn:hover { opacity: 0.85; }

  .btn-primary {
    background: #3949ab;
    color: #fff;
    border: none;
  }

  .faq-note {
    color: #9ba8c0;
    font-size: 0.875rem;
  }

  .faq-note a {
    color: #c5cae9;
    text-decoration: underline;
  }

  .faq-note a:hover { color: #fff; }
</style>
