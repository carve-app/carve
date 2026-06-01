<svelte:head>
  <title>Privacy Policy — Carve</title>
</svelte:head>

<main>
  <div class="container">
    <h1>Privacy Policy</h1>
    <p class="updated">Last updated: May 31, 2026</p>

    <section>
      <h2>What we collect</h2>
      <p>Carve collects only what is necessary to provide the service:</p>
      <ul>
        <li><strong>Account data:</strong> email address and hashed password used to authenticate your account.</li>
        <li><strong>Flashcard data:</strong> words, sentences, and source URLs you explicitly mine while browsing.</li>
        <li><strong>Review history:</strong> ratings and timestamps for your spaced-repetition sessions.</li>
        <li><strong>Immersion logs:</strong> aggregate reading/listening session duration per site, used to display your stats dashboard. Individual page content is never transmitted.</li>
        <li><strong>Extension settings:</strong> stored locally in your browser via <code>chrome.storage.local</code> — never sent to our servers.</li>
      </ul>
    </section>

    <section>
      <h2>What we do not collect</h2>
      <ul>
        <li>Full browsing history or page content outside of explicitly mined cards.</li>
        <li>Advertising identifiers or third-party tracking cookies.</li>
        <li>Any data from pages where you have disabled the Carve extension.</li>
      </ul>
    </section>

    <section>
      <h2>How we use your data</h2>
      <p>Your data is used solely to operate Carve — to schedule your reviews, display your progress, and sync your cards across devices. We do not sell, share, or use your data for advertising.</p>
    </section>

    <section>
      <h2>Data storage</h2>
      <p>Data is stored on servers located in the European Union. All data in transit is encrypted via HTTPS/TLS.</p>
    </section>

    <section>
      <h2>Your rights</h2>
      <p>You may request export or deletion of your account data at any time by emailing <a href="mailto:privacy@carve-app.com">privacy@carve-app.com</a>. Account deletion removes all associated cards, reviews, and logs within 30 days.</p>
    </section>

    <section>
      <h2>Changes</h2>
      <p>We will post any material changes to this policy on this page with an updated date. Continued use of Carve after changes constitutes acceptance.</p>
    </section>

    <section>
      <h2>Contact</h2>
      <p>Questions? Email <a href="mailto:privacy@carve-app.com">privacy@carve-app.com</a>.</p>
    </section>
  </div>
</main>

<style>
  main {
    min-height: 100vh;
    background: var(--bg, #0f1116);
    color: var(--text, #e8eaf0);
    padding: 3rem 1rem;
  }

  .container {
    max-width: 720px;
    margin: 0 auto;
  }

  h1 {
    font-size: 2rem;
    font-weight: 700;
    margin-bottom: 0.25rem;
  }

  .updated {
    color: #9ba8c0;
    font-size: 0.875rem;
    margin-bottom: 2.5rem;
  }

  h2 {
    font-size: 1.125rem;
    font-weight: 600;
    margin: 2rem 0 0.75rem;
    color: #c5cae9;
  }

  p, li {
    line-height: 1.7;
    color: #9ba8c0;
  }

  ul {
    padding-left: 1.5rem;
    margin: 0.5rem 0;
  }

  li {
    margin-bottom: 0.35rem;
  }

  a {
    color: #c5cae9;
    text-decoration: underline;
  }

  a:hover {
    color: #fff;
  }

  code {
    font-family: monospace;
    background: rgba(255,255,255,0.07);
    padding: 0.1em 0.35em;
    border-radius: 3px;
    font-size: 0.9em;
  }

  section {
    border-bottom: 1px solid rgba(255,255,255,0.06);
    padding-bottom: 1.5rem;
  }

  section:last-child {
    border-bottom: none;
  }
</style>
