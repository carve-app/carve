<svelte:head>
  <title>Pricing — Carve</title>
</svelte:head>

<main>
  <div class="container">
    <h1>Simple, honest pricing</h1>
    <p class="subtitle">Start free. Upgrade when you're ready.</p>

    <div class="plans">
      <!-- Free -->
      <div class="plan">
        <h2 class="plan-name">Free</h2>
        <div class="plan-price"><span class="amount">$0</span><span class="period">/month</span></div>
        <p class="plan-desc">Everything you need to get started with immersion learning.</p>
        <ul class="features">
          <li>Up to 200 cards</li>
          <li>Japanese, Chinese, Korean</li>
          <li>Full FSRS-6 scheduling</li>
          <li>Chrome, Firefox &amp; Safari extension</li>
          <li>Furigana &amp; pitch accent</li>
          <li>Web reader</li>
        </ul>
        <a href="/register" class="btn btn-outline">Get started free</a>
      </div>

      <!-- Learner -->
      <div class="plan plan-featured">
        <div class="plan-badge">Most popular</div>
        <h2 class="plan-name">Learner</h2>
        <div class="plan-price"><span class="amount">$9</span><span class="period">/month</span></div>
        <p class="plan-desc">For serious learners who want unlimited cards and rich media.</p>
        <ul class="features">
          <li><strong>Unlimited cards</strong></li>
          <li>Everything in Free</li>
          <li>Audio + frame capture from video</li>
          <li>Anki / Migaku import</li>
          <li>EPUB &amp; SRT reader</li>
          <li>Comprehension stats</li>
          <li>Priority support</li>
        </ul>
        <a href="/register" class="btn btn-primary">Start 7-day trial</a>
      </div>

      <!-- Pro -->
      <div class="plan">
        <h2 class="plan-name">Pro</h2>
        <div class="plan-price"><span class="amount">$19</span><span class="period">/month</span></div>
        <p class="plan-desc">For advanced learners and power users who want every feature.</p>
        <ul class="features">
          <li>Everything in Learner</li>
          <li>Output practice suite</li>
          <li>Shadowing &amp; speaking exercises</li>
          <li>AnkiConnect sync</li>
          <li>API access</li>
          <li>Early access to new features</li>
        </ul>
        <a href="/register" class="btn btn-outline">Start 7-day trial</a>
      </div>
    </div>

    <p class="faq-note">
      Have questions? See the <a href="/privacy">privacy policy</a> or email
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

  .period {
    color: #9ba8c0;
    font-size: 0.875rem;
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

  .btn-outline {
    background: transparent;
    color: #c5cae9;
    border: 1px solid rgba(255,255,255,0.18);
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
