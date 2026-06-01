<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { apiFetch, fetchForecast, type ForecastDay } from '$lib/api';
  import { lang, LANG_LABELS, type LangCode } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';
  import Card from '$lib/design/Card.svelte';
  import Chart from '$lib/design/Chart.svelte';
  import Heatmap from '$lib/design/Heatmap.svelte';
  import EmptyState from '$lib/design/EmptyState.svelte';
  import Button from '$lib/design/Button.svelte';
  import Tag from '$lib/design/Tag.svelte';

  type State = 'loading' | 'loaded' | 'error';

  let state: State = 'loading';
  let error = '';
  let stats: any = null;
  let forecast: ForecastDay[] = [];

  onMount(() => {
    let init = false;
    const unsub = lang.subscribe(l => { if (init) load(l); });
    load(get(lang));
    init = true;
    return unsub;
  });

  async function load(language: LangCode = get(lang)) {
    state = 'loading';
    try {
      [stats, { forecast }] = await Promise.all([
        apiFetch(`/v1/stats?language=${language}`),
        fetchForecast(language, 14),
      ]);
      state = 'loaded';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unknown error';
      toasts.add(error);
      state = 'error';
    }
  }

  function pct(v: number) { return Math.round(v * 100); }
  function retentionVariant(r: number): 'success' | 'warning' | 'danger' {
    if (r >= 0.90) return 'success';
    if (r >= 0.80) return 'warning';
    return 'danger';
  }

  $: wordGrowthData = (stats?.word_growth ?? []).map((s: any) => ({ label: s.date, value: s.known_count }));
  $: forecastData   = forecast.map(d => ({ label: d.date, value: d.count }));
  // Heatmap expects [{ date, value }] over last 13 weeks; reuse word_growth as proxy
  // if backend hasn't yet shipped a `reviews_by_day` field.
  $: heatmapData = stats?.reviews_by_day ?? (stats?.word_growth ?? []).map((s: any) => ({ date: s.date, value: s.delta ?? 0 }));
  $: langLabel = LANG_LABELS[$lang as LangCode] ?? $lang;
</script>

<main>
  <header class="page-head">
    <h1>Stats</h1>
    <Tag variant="info" size="md">{langLabel}</Tag>
  </header>

  {#if state === 'loading'}
    <p class="loading">Loading…</p>
  {:else if state === 'error'}
    <EmptyState title="Could not load stats" body={error} icon="!">
      <Button on:click={() => load()}>Retry</Button>
    </EmptyState>
  {:else if stats}
    <div class="kpi-grid">
      <Card padding="sm">
        <div class="kpi-label">Known cards</div>
        <div class="kpi-value green">{stats.known_cards.toLocaleString()}</div>
        <div class="kpi-sub">{stats.learning_cards.toLocaleString()} learning</div>
      </Card>
      <Card padding="sm">
        <div class="kpi-label">Retention (30d)</div>
        <div class="kpi-value">
          <Tag variant={retentionVariant(stats.retention_30d)} size="md">{pct(stats.retention_30d)}%</Tag>
        </div>
        <div class="kpi-sub">{stats.total_reviews.toLocaleString()} reviews</div>
      </Card>
      <Card padding="sm">
        <div class="kpi-label">Streak</div>
        <div class="kpi-value warning">{stats.streak_days}</div>
        <div class="kpi-sub">consecutive days</div>
      </Card>
      <Card padding="sm">
        <div class="kpi-label">Immersion (30d)</div>
        <div class="kpi-value">{(stats.reading_minutes + stats.listening_minutes).toLocaleString()}</div>
        <div class="kpi-sub">{stats.reading_minutes}m reading · {stats.listening_minutes}m listening</div>
      </Card>
    </div>

    <Card padding="md" class="mt">
      <h2>Reviews per day</h2>
      {#if heatmapData.length > 0}
        <Heatmap data={heatmapData} weeks={13} title="Last 13 weeks" />
      {:else}
        <EmptyState title="Not enough history yet" body="The heatmap fills in after your first week of reviews." />
      {/if}
    </Card>

    <Card padding="md" class="mt">
      <h2>Known-words growth</h2>
      {#if wordGrowthData.length > 1}
        <Chart data={wordGrowthData} kind="line" width={720} height={180} yLabel="known cards" />
      {:else}
        <EmptyState title="Coming soon" body="The growth chart appears after a few days of study." />
      {/if}
    </Card>

    <Card padding="md" class="mt">
      <h2>14-day forecast</h2>
      {#if forecastData.length > 0}
        <Chart data={forecastData} kind="bar" width={720} height={180} yLabel="cards due" />
        <p class="forecast-total">Total due: {forecast.reduce((s, d) => s + d.count, 0)}</p>
      {:else}
        <EmptyState title="No upcoming reviews" body="Mine more cards or wait for current cards to mature." />
      {/if}
    </Card>

    <Card padding="md" class="mt">
      <h2>All-time</h2>
      <dl class="kv-list">
        <div class="kv"><dt>Total reviews</dt><dd>{stats.total_ever_reviews.toLocaleString()}</dd></div>
        <div class="kv"><dt>Cards known</dt><dd>{stats.known_cards.toLocaleString()}</dd></div>
        <div class="kv"><dt>Cards learning</dt><dd>{stats.learning_cards.toLocaleString()}</dd></div>
        {#if stats.total_immersion_minutes != null}
          <div class="kv"><dt>Immersion total</dt><dd>{Math.round(stats.total_immersion_minutes / 60)} hours</dd></div>
        {/if}
      </dl>
    </Card>
  {/if}
</main>

<style>
  main { max-width: 820px; margin: 0 auto; padding: var(--s-6) var(--s-4); }
  .page-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--s-5); }
  h1 { margin: 0; font-size: 1.5rem; color: var(--c-textHi); }
  h2 { margin: 0 0 var(--s-4); font-size: 1rem; color: var(--c-textHi); }
  .loading { text-align: center; color: var(--c-textMuted); margin-top: var(--s-12); }

  .kpi-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: var(--s-3);
    margin-bottom: var(--s-4);
  }
  .kpi-label { font-size: 0.72rem; color: var(--c-textMuted); text-transform: uppercase; letter-spacing: 0.06em; }
  .kpi-value { font-size: 1.8rem; font-weight: 700; color: var(--c-textHi); line-height: 1.15; margin-top: var(--s-2); }
  .kpi-value.green   { color: var(--c-green); }
  .kpi-value.warning { color: var(--c-warning); }
  .kpi-sub { font-size: 0.78rem; color: var(--c-textMuted); margin-top: var(--s-1); }

  :global(.mt) { margin-top: var(--s-4); }

  .forecast-total { color: var(--c-textMuted); font-size: 0.85rem; margin: var(--s-3) 0 0; }

  .kv-list { display: flex; flex-direction: column; gap: 0; margin: 0; padding: 0; }
  .kv { display: flex; justify-content: space-between; padding: var(--s-2) 0; border-bottom: 1px solid var(--c-border); }
  .kv:last-child { border-bottom: none; }
  dt { color: var(--c-textMuted); margin: 0; }
  dd { color: var(--c-textHi); font-weight: 600; margin: 0; }
</style>
