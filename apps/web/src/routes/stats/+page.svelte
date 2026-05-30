<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch, fetchForecast, ApiError, type ForecastDay } from '$lib/api';

  type State = 'loading' | 'loaded' | 'unauthenticated' | 'error';

  let state: State = 'loading';
  let error = '';
  let stats: any = null;
  let forecast: ForecastDay[] = [];

  onMount(async () => {
    if (!localStorage.getItem('carve_access_token')) {
      state = 'unauthenticated';
      return;
    }
    try {
      [stats, { forecast }] = await Promise.all([
        apiFetch('/v1/stats?language=ja'),
        fetchForecast('ja', 14),
      ]);
      state = 'loaded';
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        state = 'unauthenticated';
      } else {
        error = err instanceof Error ? err.message : 'Unknown error';
        state = 'error';
      }
    }
  });

  function pct(v: number) { return Math.round(v * 100); }
  function retentionColor(r: number): string {
    if (r >= 0.90) return '#4caf50';
    if (r >= 0.80) return '#ffa726';
    return '#ef5350';
  }

  function chartBars(snapshots: any[]): string {
    if (!snapshots?.length) return '';
    const maxVal = Math.max(...snapshots.map((s: any) => s.known_count), 1);
    return snapshots.map((s: any) => {
      const h = Math.max(4, Math.round((s.known_count / maxVal) * 80));
      return `<div class="bar" style="height:${h}px" title="${s.date}: ${s.known_count} known"></div>`;
    }).join('');
  }

  function forecastBars(days: ForecastDay[]): string {
    if (!days?.length) return '';
    const maxVal = Math.max(...days.map(d => d.count), 1);
    return days.map(d => {
      const h = Math.max(2, Math.round((d.count / maxVal) * 80));
      const color = d.count > 100 ? '#ef5350' : d.count > 50 ? '#ffa726' : '#4caf50';
      return `<div class="bar" style="height:${h}px;background:${color}" title="${d.date}: ${d.count} due"></div>`;
    }).join('');
  }
</script>

<main>
  <header>
    <h1>Stats</h1>
    <nav>
      <a href="/">Home</a>
      <a href="/review">Review</a>
      <a href="/library">Library</a>
    </nav>
  </header>

  {#if state === 'loading'}
    <p class="msg">Loading…</p>

  {:else if state === 'unauthenticated'}
    <div class="msg"><p>Please <a href="/login">log in</a> to see your stats.</p></div>

  {:else if state === 'error'}
    <div class="msg error"><p>{error}</p></div>

  {:else if stats}
    <div class="grid">
      <div class="card">
        <div class="card-label">Known Cards</div>
        <div class="card-value" style="color:#4caf50">{stats.known_cards}</div>
        <div class="card-sub">{stats.learning_cards} learning</div>
      </div>
      <div class="card">
        <div class="card-label">Retention (30d)</div>
        <div class="card-value" style="color:{retentionColor(stats.retention_30d)}">
          {pct(stats.retention_30d)}%
        </div>
        <div class="card-sub">{stats.total_reviews} reviews</div>
      </div>
      <div class="card">
        <div class="card-label">Streak</div>
        <div class="card-value" style="color:#ffa726">{stats.streak_days}</div>
        <div class="card-sub">consecutive days</div>
      </div>
      <div class="card">
        <div class="card-label">Immersion (30d)</div>
        <div class="card-value">{stats.reading_minutes + stats.listening_minutes}</div>
        <div class="card-sub">{stats.reading_minutes}m reading · {stats.listening_minutes}m listening</div>
      </div>
    </div>

    {#if stats.word_growth?.length > 1}
      <div class="section">
        <h2>Known Words Growth</h2>
        <div class="chart">
          {@html chartBars(stats.word_growth)}
        </div>
        <div class="chart-range">
          <span>{stats.word_growth[0]?.date}</span>
          <span>{stats.word_growth[stats.word_growth.length - 1]?.date}</span>
        </div>
      </div>
    {:else}
      <div class="section empty">
        <p>Word growth chart will appear after a few days of study.</p>
      </div>
    {/if}

    <div class="section">
      <h2>14-Day Review Forecast</h2>
      {#if forecast.length > 0}
        <div class="chart">
          {@html forecastBars(forecast)}
        </div>
        <div class="chart-range">
          <span>{forecast[0]?.date}</span>
          <span class="forecast-total">{forecast.reduce((s, d) => s + d.count, 0)} total due</span>
          <span>{forecast[forecast.length - 1]?.date}</span>
        </div>
      {:else}
        <p class="empty-msg">No upcoming reviews.</p>
      {/if}
    </div>

    <div class="section">
      <h2>All-time</h2>
      <div class="stat-row">
        <span class="stat-label">Total reviews</span>
        <span class="stat-val">{stats.total_ever_reviews.toLocaleString()}</span>
      </div>
      <div class="stat-row">
        <span class="stat-label">Cards known</span>
        <span class="stat-val">{stats.known_cards.toLocaleString()}</span>
      </div>
      <div class="stat-row">
        <span class="stat-label">Cards learning</span>
        <span class="stat-val">{stats.learning_cards.toLocaleString()}</span>
      </div>
    </div>
  {/if}
</main>

<style>
  :global(body) { background: #13151a; color: #e8eaf0; font-family: system-ui, sans-serif; margin: 0; }
  main { max-width: 760px; margin: 0 auto; padding: 1.5rem 1rem; }

  header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1.5rem; border-bottom: 1px solid #2a2d36; padding-bottom: 1rem; }
  header h1 { margin: 0; font-size: 1.4rem; }
  nav { display: flex; gap: 1rem; }
  nav a { color: #9ba8c0; text-decoration: none; font-size: 0.9rem; }
  nav a:hover { color: #e8eaf0; }

  .msg { text-align: center; margin-top: 3rem; color: #9ba8c0; }
  .msg.error { color: #ef5350; }
  .msg a { color: #4caf50; }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  .card {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1.25rem 1rem;
    text-align: center;
  }
  .card-label { font-size: 0.78rem; color: #6b7591; text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 0.5rem; }
  .card-value { font-size: 2.2rem; font-weight: 700; line-height: 1.1; }
  .card-sub { font-size: 0.78rem; color: #6b7591; margin-top: 0.4rem; }

  .section {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 10px;
    padding: 1.25rem 1.5rem;
    margin-bottom: 1rem;
  }
  .section h2 { margin: 0 0 1rem; font-size: 1rem; }
  .section.empty { color: #6b7591; font-size: 0.88rem; text-align: center; padding: 2rem; }

  .chart {
    display: flex;
    align-items: flex-end;
    gap: 2px;
    height: 88px;
    border-bottom: 1px solid #2a2d36;
    margin-bottom: 0.4rem;
    overflow: hidden;
  }
  :global(.bar) {
    flex: 1;
    min-width: 2px;
    background: #4caf50;
    border-radius: 2px 2px 0 0;
    opacity: 0.85;
  }
  .chart-range {
    display: flex;
    justify-content: space-between;
    font-size: 0.72rem;
    color: #6b7591;
  }
  .forecast-total { font-weight: 600; color: #9ba8c0; }
  .empty-msg { color: #6b7591; font-size: 0.85rem; margin: 0; }

  .stat-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.5rem 0;
    border-bottom: 1px solid #2a2d36;
  }
  .stat-row:last-child { border-bottom: none; }
  .stat-label { font-size: 0.9rem; color: #9ba8c0; }
  .stat-val { font-size: 0.9rem; font-weight: 600; }
</style>
