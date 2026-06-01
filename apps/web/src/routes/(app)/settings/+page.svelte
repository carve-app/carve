<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import {
    fetchFsrsSettings,
    saveFsrsSettings,
    fetchWorkloadPreview,
    deleteAccount,
    apiFetch,
    type FsrsSettings,
    type WorkloadPreview,
  } from '$lib/api';
  import { lang, LANG_LABELS, type LangCode } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';
  import { currentUser } from '$lib/stores/user';
  import Button from '$lib/design/Button.svelte';
  import Card from '$lib/design/Card.svelte';
  import EmptyState from '$lib/design/EmptyState.svelte';

  type TabId = 'account' | 'display' | 'review' | 'mining' | 'sites' | 'sync' | 'billing';

  const TABS: { id: TabId; label: string; icon: string }[] = [
    { id: 'account', label: 'Account',  icon: '👤' },
    { id: 'display', label: 'Display',  icon: '🎨' },
    { id: 'review',  label: 'Review',   icon: '🃏' },
    { id: 'mining',  label: 'Mining',   icon: '⛏️' },
    { id: 'sites',   label: 'Sites',    icon: '🌐' },
    { id: 'sync',    label: 'Sync',     icon: '🔄' },
    { id: 'billing', label: 'Billing',  icon: '💳' },
  ];

  function initialTab(): TabId {
    if (typeof window === 'undefined') return 'account';
    const hash = window.location.hash.slice(1) as TabId;
    return TABS.find(t => t.id === hash) ? hash : 'account';
  }
  let activeTab: TabId = initialTab();

  function selectTab(t: TabId) {
    activeTab = t;
    if (typeof window !== 'undefined') history.replaceState(null, '', `#${t}`);
  }

  // ── Account ───────────────────────────────────────────────────────────────
  let deleteConfirm = '';
  let deleting = false;
  async function handleDeleteAccount() {
    if (deleteConfirm !== 'delete my account') return;
    deleting = true;
    try {
      await deleteAccount();
      localStorage.removeItem('carve_access_token');
      currentUser.set(null);
      window.location.href = '/login';
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Account deletion failed');
      deleting = false;
    }
  }
  async function downloadData() {
    try {
      const r = await apiFetch<{ url?: string }>('/v1/users/export', { method: 'POST' });
      if (r?.url) window.open(r.url, '_blank');
      else toasts.add('Export queued — you\'ll get an email when it\'s ready.');
    } catch (e) {
      toasts.add(e instanceof Error ? e.message : 'Could not start export');
    }
  }

  // ── Display ───────────────────────────────────────────────────────────────
  type Theme = 'system' | 'dark' | 'light';
  type Furigana = 'always' | 'unknown-only' | 'kanji-only' | 'off';
  type FontSize = 'sm' | 'md' | 'lg';
  let theme: Theme = (typeof localStorage !== 'undefined' && (localStorage.getItem('carve_theme') as Theme)) || 'system';
  let furigana: Furigana = (typeof localStorage !== 'undefined' && (localStorage.getItem('carve_furigana') as Furigana)) || 'unknown-only';
  let pitchColor: boolean = typeof localStorage !== 'undefined' ? localStorage.getItem('carve_pitch_color') !== '0' : true;
  let fontSize: FontSize = (typeof localStorage !== 'undefined' && (localStorage.getItem('carve_font_size') as FontSize)) || 'md';
  let uiLang: string = (typeof localStorage !== 'undefined' && localStorage.getItem('carve_ui_lang')) || 'en';

  function persistDisplay() {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem('carve_theme', theme);
    localStorage.setItem('carve_furigana', furigana);
    localStorage.setItem('carve_pitch_color', pitchColor ? '1' : '0');
    localStorage.setItem('carve_font_size', fontSize);
    localStorage.setItem('carve_ui_lang', uiLang);
    toasts.add('Display preferences saved');
  }

  // ── Review (FSRS) ─────────────────────────────────────────────────────────
  let loading = true;
  let saving = false;
  let settings: FsrsSettings = {
    language_code: 'ja', weights: [],
    target_retention: 0.90, leech_threshold: 5, daily_new_limit: 20,
    is_customized: false,
  };
  let preview: WorkloadPreview | null = null;
  let previewLoading = false;

  onMount(() => {
    let init = false;
    const unsub = lang.subscribe(l => { if (init) reload(l); });
    reload(get(lang));
    init = true;
    return unsub;
  });

  async function reload(language = get(lang)) {
    loading = true;
    try {
      settings = await fetchFsrsSettings(language);
      settings.language_code = language;
      await loadPreview(language);
    } catch { /* defaults already set */ }
    loading = false;
  }

  async function loadPreview(language = get(lang)) {
    previewLoading = true;
    try { preview = await fetchWorkloadPreview(language, settings.target_retention); }
    catch { preview = null; }
    previewLoading = false;
  }

  async function saveFsrs() {
    saving = true;
    try {
      await saveFsrsSettings({
        language_code: settings.language_code,
        target_retention: settings.target_retention,
        leech_threshold: settings.leech_threshold,
        daily_new_limit: settings.daily_new_limit,
      });
      toasts.add('FSRS settings saved');
      await loadPreview();
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Save failed');
    }
    saving = false;
  }

  let optimizing = false;
  let optimizeResult: { optimized: boolean; reason?: string; events_used?: number } | null = null;
  async function runOptimizer() {
    optimizing = true; optimizeResult = null;
    try {
      optimizeResult = await apiFetch(`/v1/settings/fsrs/optimize?language=${settings.language_code}`, { method: 'POST' });
      if (optimizeResult?.optimized) settings = await fetchFsrsSettings(settings.language_code);
    } catch (err) {
      optimizeResult = { optimized: false, reason: err instanceof Error ? err.message : 'Optimizer failed' };
    }
    optimizing = false;
  }

  function pct(v: number) { return Math.round(v * 100); }
  $: langLabel = LANG_LABELS[settings.language_code as LangCode] ?? settings.language_code;

  // ── Mining ────────────────────────────────────────────────────────────────
  let defaultDeck: Record<LangCode, string> = JSON.parse(
    (typeof localStorage !== 'undefined' && localStorage.getItem('carve_default_decks')) || '{"ja":"","zh-cn":"","ko":"","en":""}',
  );
  let screenshotQuality = parseInt(
    (typeof localStorage !== 'undefined' && localStorage.getItem('carve_screenshot_q')) || '80',
  );
  let autoPause = typeof localStorage !== 'undefined' ? localStorage.getItem('carve_auto_pause') === '1' : false;

  function persistMining() {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem('carve_default_decks', JSON.stringify(defaultDeck));
    localStorage.setItem('carve_screenshot_q', String(screenshotQuality));
    localStorage.setItem('carve_auto_pause', autoPause ? '1' : '0');
    toasts.add('Mining preferences saved');
  }

  // ── Sites ─────────────────────────────────────────────────────────────────
  let disabledDomains: string[] = JSON.parse(
    (typeof localStorage !== 'undefined' && localStorage.getItem('carve_disabled_domains')) || '[]',
  );
  let newDomain = '';
  function addDomain() {
    const d = newDomain.trim().toLowerCase().replace(/^https?:\/\//, '').replace(/\/$/, '');
    if (!d || disabledDomains.includes(d)) return;
    disabledDomains = [...disabledDomains, d];
    localStorage.setItem('carve_disabled_domains', JSON.stringify(disabledDomains));
    newDomain = '';
  }
  function removeDomain(d: string) {
    disabledDomains = disabledDomains.filter(x => x !== d);
    localStorage.setItem('carve_disabled_domains', JSON.stringify(disabledDomains));
  }

  // ── Sync (AnkiConnect) ────────────────────────────────────────────────────
  let ankiConnectURL = (typeof localStorage !== 'undefined' && localStorage.getItem('carve_ankiconnect_url')) || 'http://localhost:8765';
  let lastSyncAt: string | null = (typeof localStorage !== 'undefined' && localStorage.getItem('carve_last_sync')) || null;
  let syncing = false;
  let syncStatus = '';
  async function testAnkiConnect() {
    syncing = true; syncStatus = 'Connecting…';
    try {
      const res = await fetch(ankiConnectURL, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'deckNames', version: 6 }),
      });
      const data = await res.json();
      syncStatus = `✓ Connected — ${data.result?.length ?? 0} decks`;
      localStorage.setItem('carve_ankiconnect_url', ankiConnectURL);
    } catch (e) {
      syncStatus = `✗ ${e instanceof Error ? e.message : 'connection failed'}`;
    }
    syncing = false;
  }
  async function syncNow() {
    syncing = true; syncStatus = 'Syncing…';
    try {
      const r = await apiFetch<{ pushed: number; pulled: number }>(`/v1/sync/anki-connect`, {
        method: 'POST',
        body: JSON.stringify({ url: ankiConnectURL }),
      });
      syncStatus = `✓ Pushed ${r.pushed} new, pulled ${r.pulled} review events`;
      lastSyncAt = new Date().toISOString();
      localStorage.setItem('carve_last_sync', lastSyncAt);
    } catch (e) {
      syncStatus = `✗ ${e instanceof Error ? e.message : 'sync failed'}`;
    }
    syncing = false;
  }

  // ── Billing ───────────────────────────────────────────────────────────────
  type Plan = { tier: 'free' | 'pro' | 'team'; renews_at?: string | null };
  let plan: Plan = { tier: 'free' };
  async function loadPlan() {
    try { plan = await apiFetch<Plan>('/v1/billing/plan'); } catch { /* free */ }
  }
  async function openPortal() {
    try {
      const r = await apiFetch<{ url: string }>('/v1/billing/portal', { method: 'POST' });
      window.location.href = r.url;
    } catch (e) {
      toasts.add(e instanceof Error ? e.message : 'Could not open billing portal');
    }
  }
  onMount(loadPlan);
</script>

<main>
  <h1 class="page-title">Settings</h1>

  <div class="tabs" role="tablist" aria-label="Settings sections">
    {#each TABS as tab}
      <button
        role="tab"
        id="tab-{tab.id}"
        aria-controls="panel-{tab.id}"
        aria-selected={activeTab === tab.id}
        tabindex={activeTab === tab.id ? 0 : -1}
        class="tab"
        class:active={activeTab === tab.id}
        on:click={() => selectTab(tab.id)}
      >
        <span class="tab-icon" aria-hidden="true">{tab.icon}</span>
        <span class="tab-label">{tab.label}</span>
      </button>
    {/each}
  </div>

  <!-- Account ─────────────────────────────────────────────────────────────-->
  {#if activeTab === 'account'}
    <div role="tabpanel" id="panel-account" aria-labelledby="tab-account">
      <Card padding="md">
        <h2>Profile</h2>
        {#if $currentUser}
          <div class="kv"><span class="k">Email</span><span class="v">{$currentUser.email}</span></div>
          <div class="kv"><span class="k">Name</span> <span class="v">{$currentUser.display_name}</span></div>
        {:else}
          <p class="muted">Not signed in.</p>
        {/if}
      </Card>

      <Card padding="md" class="mt-4">
        <h2>Data export</h2>
        <p class="muted">Download your cards, review history, and immersion logs as JSON. Carve is fully exportable — you own your data.</p>
        <Button variant="secondary" on:click={downloadData}>Request data export</Button>
      </Card>

      <Card padding="md" class="mt-4 danger-zone">
        <h2 class="danger-title">Danger zone</h2>
        <p class="muted">Deleting your account permanently removes all cards, history, and settings. This cannot be undone.</p>
        <p class="muted">Type <strong>delete my account</strong> to confirm.</p>
        <input
          aria-label="Type 'delete my account' to confirm"
          type="text"
          class="confirm-input"
          placeholder="delete my account"
          bind:value={deleteConfirm}
        />
        <Button variant="danger" disabled={deleteConfirm !== 'delete my account' || deleting} on:click={handleDeleteAccount}>
          {deleting ? 'Deleting…' : 'Delete my account'}
        </Button>
      </Card>
    </div>
  {/if}

  <!-- Display ─────────────────────────────────────────────────────────────-->
  {#if activeTab === 'display'}
    <div role="tabpanel" id="panel-display" aria-labelledby="tab-display">
      <Card padding="md">
        <h2>Appearance</h2>
        <div class="field">
          <label for="theme-sel">Theme</label>
          <select id="theme-sel" bind:value={theme} class="select">
            <option value="system">System default</option>
            <option value="dark">Dark</option>
            <option value="light">Light</option>
          </select>
        </div>
        <div class="field">
          <label for="font-size">Font size</label>
          <select id="font-size" bind:value={fontSize} class="select">
            <option value="sm">Small</option>
            <option value="md">Medium</option>
            <option value="lg">Large</option>
          </select>
        </div>
        <div class="field">
          <label for="ui-lang">UI language</label>
          <select id="ui-lang" bind:value={uiLang} class="select">
            <option value="en">English</option>
            <option value="ja">日本語</option>
            <option value="zh-cn">中文</option>
            <option value="ko">한국어</option>
          </select>
        </div>
      </Card>

      <Card padding="md" class="mt-4">
        <h2>Reading aids</h2>
        <div class="field">
          <label for="furigana-mode">Furigana / ruby</label>
          <select id="furigana-mode" bind:value={furigana} class="select">
            <option value="always">Always show</option>
            <option value="unknown-only">Unknown words only</option>
            <option value="kanji-only">Kanji only</option>
            <option value="off">Off</option>
          </select>
        </div>
        <div class="field row">
          <label for="pitch-color" class="row-label">Pitch-accent color overlay</label>
          <input id="pitch-color" type="checkbox" bind:checked={pitchColor} />
        </div>
      </Card>

      <div class="footer-row">
        <Button on:click={persistDisplay}>Save display preferences</Button>
      </div>
    </div>
  {/if}

  <!-- Review (FSRS) ──────────────────────────────────────────────────────-->
  {#if activeTab === 'review'}
    <div role="tabpanel" id="panel-review" aria-labelledby="tab-review">
      <Card padding="md">
        <h2>SRS Settings ({langLabel})</h2>
        {#if loading}
          <p class="muted">Loading…</p>
        {:else}
          <div class="field">
            <label for="retention-slider">
              Target Retention
              <span class="hint">Probability of remembering when a card is due. Higher = shorter intervals.</span>
            </label>
            <div class="slider-row">
              <input id="retention-slider" type="range" min="0.70" max="0.98" step="0.01"
                     bind:value={settings.target_retention} on:change={() => loadPreview()} />
              <span class="pct">{pct(settings.target_retention)}%</span>
            </div>
            {#if previewLoading}
              <p class="hint">Calculating…</p>
            {:else if preview}
              <p class="hint">
                With {pct(settings.target_retention)}% retention: avg interval
                {preview.avg_interval_days.toFixed(1)} days across {preview.total_review_cards} cards.
              </p>
            {/if}
          </div>

          <div class="field">
            <label for="daily-new">Daily new-card limit <span class="hint">Per language.</span></label>
            <input id="daily-new" type="number" min="0" max="500" bind:value={settings.daily_new_limit} class="number-input" />
          </div>

          <div class="field">
            <label for="leech-th">Leech threshold <span class="hint">Lapses before card is suspended.</span></label>
            <input id="leech-th" type="number" min="1" max="20" bind:value={settings.leech_threshold} class="number-input" />
          </div>

          <div class="actions">
            <Button on:click={saveFsrs} disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
            <Button variant="ghost" on:click={() => { settings.target_retention=0.90; settings.leech_threshold=5; settings.daily_new_limit=20; loadPreview(); }}>Reset to defaults</Button>
          </div>
        {/if}
      </Card>

      {#if settings.weights?.length === 19}
        <Card padding="md" class="mt-4">
          <h2>FSRS-6 Parameters</h2>
          <p class="muted">
            These 19 weights control the scheduling algorithm. They optimize automatically after 400+ reviews.
            Current state: {settings.is_customized ? 'customized' : 'default'}.
          </p>
          <div class="weights-grid">
            {#each settings.weights as w, i}
              <div class="weight-cell"><span class="weight-index">w[{i}]</span><span class="weight-val">{w.toFixed(4)}</span></div>
            {/each}
          </div>
          <div class="actions mt-3">
            <Button variant="secondary" on:click={runOptimizer} disabled={optimizing}>
              {optimizing ? 'Optimizing…' : 'Optimize from my history'}
            </Button>
            {#if optimizeResult}
              <span class="status" class:ok={optimizeResult.optimized} class:warn={!optimizeResult.optimized}>
                {optimizeResult.optimized ? `✓ Optimized using ${optimizeResult.events_used} reviews` : optimizeResult.reason}
              </span>
            {/if}
          </div>
        </Card>
      {/if}
    </div>
  {/if}

  <!-- Mining ─────────────────────────────────────────────────────────────-->
  {#if activeTab === 'mining'}
    <div role="tabpanel" id="panel-mining" aria-labelledby="tab-mining">
      <Card padding="md">
        <h2>Mining defaults</h2>
        <p class="muted">Carve auto-fills these when you mine a new card. You can override per card.</p>
        <div class="field">
          <label for="default-deck-ja">Default deck — Japanese</label>
          <input id="default-deck-ja" class="text-input" bind:value={defaultDeck.ja} placeholder="e.g. JLPT N3 Mining" />
        </div>
        <div class="field">
          <label for="default-deck-zh">Default deck — Chinese</label>
          <input id="default-deck-zh" class="text-input" bind:value={defaultDeck['zh-cn']} placeholder="e.g. HSK 4 Mining" />
        </div>
        <div class="field">
          <label for="default-deck-ko">Default deck — Korean</label>
          <input id="default-deck-ko" class="text-input" bind:value={defaultDeck.ko} placeholder="e.g. TOPIK II Mining" />
        </div>
        <div class="field">
          <label for="default-deck-en">Default deck — English</label>
          <input id="default-deck-en" class="text-input" bind:value={defaultDeck.en} placeholder="e.g. CEFR C1 Mining" />
        </div>
      </Card>

      <Card padding="md" class="mt-4">
        <h2>Capture</h2>
        <div class="field">
          <label for="ssq">Screenshot quality ({screenshotQuality})</label>
          <input id="ssq" type="range" min="40" max="100" bind:value={screenshotQuality} />
        </div>
        <div class="field row">
          <label for="auto-pause" class="row-label">Auto-pause video on new subtitle cue</label>
          <input id="auto-pause" type="checkbox" bind:checked={autoPause} />
        </div>
      </Card>

      <div class="footer-row">
        <Button on:click={persistMining}>Save mining preferences</Button>
      </div>
    </div>
  {/if}

  <!-- Sites ──────────────────────────────────────────────────────────────-->
  {#if activeTab === 'sites'}
    <div role="tabpanel" id="panel-sites" aria-labelledby="tab-sites">
      <Card padding="md">
        <h2>Disabled domains</h2>
        <p class="muted">Carve stays inert on these domains. Useful for sites that conflict with our tokenizer or where you don't want to study.</p>

        <div class="add-domain">
          <input
            type="text"
            class="text-input"
            placeholder="example.com"
            bind:value={newDomain}
            on:keydown={(e) => { if (e.key === 'Enter') addDomain(); }}
            aria-label="Domain to disable"
          />
          <Button variant="secondary" on:click={addDomain}>Add</Button>
        </div>

        {#if disabledDomains.length === 0}
          <EmptyState title="No disabled domains" body="Carve is active on every page where the target language is detected." />
        {:else}
          <ul class="domain-list">
            {#each disabledDomains as d}
              <li>
                <span class="domain">{d}</span>
                <Button size="sm" variant="ghost" on:click={() => removeDomain(d)}>Remove</Button>
              </li>
            {/each}
          </ul>
        {/if}
      </Card>
    </div>
  {/if}

  <!-- Sync ───────────────────────────────────────────────────────────────-->
  {#if activeTab === 'sync'}
    <div role="tabpanel" id="panel-sync" aria-labelledby="tab-sync">
      <Card padding="md">
        <h2>AnkiConnect</h2>
        <p class="muted">
          Coexist with Anki: new Carve cards push to AnkiConnect, and review events flow back into Carve.
          Requires the
          <a href="https://ankiweb.net/shared/info/2055492159" target="_blank" rel="noopener">AnkiConnect plugin</a>
          in your local Anki installation.
        </p>
        <div class="field">
          <label for="anki-url">AnkiConnect URL</label>
          <input id="anki-url" type="text" class="text-input" bind:value={ankiConnectURL} />
        </div>
        <div class="actions">
          <Button variant="secondary" on:click={testAnkiConnect} disabled={syncing}>Test connection</Button>
          <Button on:click={syncNow} disabled={syncing}>{syncing ? 'Syncing…' : 'Sync now'}</Button>
        </div>
        {#if syncStatus}
          <p class="status-line" class:ok={syncStatus.startsWith('✓')} class:err={syncStatus.startsWith('✗')}>{syncStatus}</p>
        {/if}
        {#if lastSyncAt}
          <p class="muted small">Last sync: {new Date(lastSyncAt).toLocaleString()}</p>
        {/if}
      </Card>
    </div>
  {/if}

  <!-- Billing ────────────────────────────────────────────────────────────-->
  {#if activeTab === 'billing'}
    <div role="tabpanel" id="panel-billing" aria-labelledby="tab-billing">
      <Card padding="md">
        <h2>Plan</h2>
        <div class="kv"><span class="k">Current plan</span><span class="v plan-{plan.tier}">{plan.tier.toUpperCase()}</span></div>
        {#if plan.renews_at}
          <div class="kv"><span class="k">Renews</span> <span class="v">{new Date(plan.renews_at).toLocaleDateString()}</span></div>
        {/if}
        <div class="actions">
          <Button on:click={openPortal}>Manage subscription</Button>
          <Button variant="ghost" href="/pricing">View plans</Button>
        </div>
      </Card>
    </div>
  {/if}
</main>

<style>
  main {
    max-width: 760px;
    margin: 0 auto;
    padding: var(--s-6) var(--s-4);
  }
  .page-title {
    font-size: 1.5rem;
    margin: 0 0 var(--s-6);
    color: var(--c-textHi);
  }
  h2 { margin: 0 0 var(--s-4); font-size: 1.05rem; color: var(--c-textHi); }

  .tabs {
    display: flex;
    gap: var(--s-1);
    border-bottom: 1px solid var(--c-border);
    margin-bottom: var(--s-6);
    overflow-x: auto;
    padding-bottom: var(--s-1);
  }
  .tab {
    display: inline-flex; align-items: center; gap: var(--s-2);
    background: transparent; border: none;
    color: var(--c-textMuted);
    font: inherit; font-weight: 500;
    padding: var(--s-3) var(--s-4);
    border-radius: var(--r-md) var(--r-md) 0 0;
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    white-space: nowrap;
  }
  .tab:hover  { color: var(--c-text); }
  .tab.active { color: var(--c-textHi); border-bottom-color: var(--c-green); }
  .tab-icon   { font-size: 1.05rem; }

  .field { margin-bottom: var(--s-4); }
  .field.row { display: flex; align-items: center; justify-content: space-between; }
  label { display: block; font-size: 0.92rem; font-weight: 500; color: var(--c-text); margin-bottom: var(--s-1); }
  .row-label { margin: 0; }
  .hint  { display: block; font-size: 0.78rem; color: var(--c-textMuted); font-weight: 400; margin-top: 0.15rem; }
  .muted { color: var(--c-textMuted); font-size: 0.88rem; line-height: 1.5; margin: 0 0 var(--s-3); }
  .muted.small { font-size: 0.78rem; }

  .slider-row { display: flex; align-items: center; gap: var(--s-4); }
  input[type=range] { flex: 1; accent-color: var(--c-green); }
  .pct { font-size: 1rem; font-weight: 600; color: var(--c-green); min-width: 3rem; text-align: right; }

  .number-input, .text-input, .select, .confirm-input {
    background: var(--c-bg);
    color: var(--c-textHi);
    border: 1px solid var(--c-border);
    border-radius: var(--r-md);
    padding: 0.5rem 0.75rem;
    font: inherit;
    font-size: 0.95rem;
  }
  .number-input { width: 110px; }
  .text-input   { width: 100%; }
  .select       { width: 220px; }
  .confirm-input { width: 100%; max-width: 280px; margin-bottom: var(--s-3); }
  .text-input:focus, .select:focus, .number-input:focus, .confirm-input:focus {
    outline: none; border-color: var(--c-green);
  }

  .kv { display: flex; justify-content: space-between; padding: var(--s-2) 0; border-bottom: 1px solid var(--c-border); }
  .kv:last-child { border-bottom: none; }
  .k { color: var(--c-textMuted); }
  .v { color: var(--c-textHi); }

  .actions { display: flex; flex-wrap: wrap; gap: var(--s-3); align-items: center; margin-top: var(--s-3); }
  .footer-row { margin-top: var(--s-5); }

  .mt-3 { margin-top: var(--s-3); }
  .mt-4 { margin-top: var(--s-4); }

  .weights-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(100px, 1fr)); gap: var(--s-2); }
  .weight-cell { background: var(--c-bg); border: 1px solid var(--c-border); border-radius: var(--r-sm); padding: var(--s-2) var(--s-3); text-align: center; }
  .weight-index { display: block; font-size: 0.65rem; color: var(--c-textMuted); }
  .weight-val   { font-size: 0.8rem; font-family: ui-monospace, monospace; color: var(--c-text); }

  .status { font-size: 0.85rem; }
  .status.ok  { color: var(--c-green); }
  .status.warn { color: var(--c-warning); }
  .status-line { font-size: 0.85rem; margin-top: var(--s-3); }
  .status-line.ok  { color: var(--c-green); }
  .status-line.err { color: var(--c-danger); }

  .danger-zone { border-color: color-mix(in srgb, var(--c-danger) 35%, var(--c-border)); }
  .danger-title { color: var(--c-danger); }

  .add-domain { display: flex; gap: var(--s-2); margin-bottom: var(--s-4); }
  .add-domain .text-input { flex: 1; }
  .domain-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: var(--s-1); }
  .domain-list li { display: flex; justify-content: space-between; align-items: center; padding: var(--s-2) var(--s-3); background: var(--c-bg); border: 1px solid var(--c-border); border-radius: var(--r-sm); }
  .domain { font-family: ui-monospace, monospace; font-size: 0.88rem; color: var(--c-text); }

  .plan-free { color: var(--c-textMuted); }
  .plan-pro  { color: var(--c-green); }
  .plan-team { color: var(--c-info); }
</style>
