<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import {
    getGrammarCatalog,
    getKnownPatterns,
    markPattern,
    unmarkPattern,
    ApiError,
    type GrammarPattern,
  } from '$lib/api';
  import { lang, LANG_LABELS, type LangCode } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';
  import Card from '$lib/design/Card.svelte';
  import Tag from '$lib/design/Tag.svelte';
  import Button from '$lib/design/Button.svelte';
  import EmptyState from '$lib/design/EmptyState.svelte';

  type State = 'loading' | 'loaded' | 'unsupported' | 'error';

  let state: State = 'loading';
  let error = '';
  let currentLang: LangCode = get(lang);

  let patterns: GrammarPattern[] = [];
  // Set of known pattern ids; reassigned (not mutated) so Svelte reactivity fires.
  let known = new Set<string>();
  // Per-pattern in-flight guard so rapid double-clicks don't race.
  let pending = new Set<string>();

  // JLPT levels in descending difficulty so beginners see N5 first.
  const LEVEL_ORDER = ['N5', 'N4', 'N3', 'N2', 'N1'];

  onMount(() => {
    let init = false;
    const unsub = lang.subscribe((l) => {
      currentLang = l;
      if (init) load(l);
    });
    load(get(lang));
    init = true;
    return unsub;
  });

  async function load(language: LangCode) {
    state = 'loading';
    error = '';
    try {
      const [catalog, knownRes] = await Promise.all([
        getGrammarCatalog(language),
        getKnownPatterns(language),
      ]);
      patterns = catalog.patterns ?? [];
      known = new Set(knownRes.pattern_ids ?? []);
      state = 'loaded';
    } catch (err) {
      // The NLP catalog returns 422 for languages without grammar detection
      // (only Japanese is supported today). Treat that as "unsupported", not an error.
      if (err instanceof ApiError && err.status === 422) {
        patterns = [];
        known = new Set();
        state = 'unsupported';
        return;
      }
      error = err instanceof Error ? err.message : 'Unknown error';
      toasts.add(error);
      state = 'error';
    }
  }

  async function toggle(p: GrammarPattern) {
    if (pending.has(p.id)) return;
    pending = new Set(pending).add(p.id);

    const wasKnown = known.has(p.id);
    // Optimistic update.
    const next = new Set(known);
    if (wasKnown) next.delete(p.id);
    else next.add(p.id);
    known = next;

    try {
      if (wasKnown) await unmarkPattern(currentLang, p.id);
      else await markPattern(currentLang, p.id);
    } catch (err) {
      // Roll back on failure.
      const revert = new Set(known);
      if (wasKnown) revert.add(p.id);
      else revert.delete(p.id);
      known = revert;
      toasts.add(err instanceof Error ? err.message : 'Could not update grammar');
    } finally {
      const p2 = new Set(pending);
      p2.delete(p.id);
      pending = p2;
    }
  }

  function levelVariant(level: string): 'success' | 'warning' | 'danger' | 'info' | 'default' {
    switch (level) {
      case 'N5': return 'success';
      case 'N4': return 'info';
      case 'N3': return 'warning';
      default: return 'default';
    }
  }

  // Group patterns by JLPT level, preserving catalog order within a level.
  $: grouped = LEVEL_ORDER
    .map((level) => ({ level, items: patterns.filter((p) => p.jlpt === level) }))
    .filter((g) => g.items.length > 0);
  $: knownCount = patterns.filter((p) => known.has(p.id)).length;
  $: totalCount = patterns.length;
  $: langLabel = LANG_LABELS[currentLang] ?? currentLang;
</script>

<main>
  <header class="page-head">
    <h1>Grammar</h1>
    <Tag variant="info" size="md">{langLabel}</Tag>
  </header>

  {#if state === 'loading'}
    <p class="loading">Loading…</p>
  {:else if state === 'unsupported'}
    <EmptyState
      title="Grammar tracking is Japanese-only for now"
      body="The grammar detector currently recognizes Japanese JLPT patterns. Switch the language to 日本語 to mark patterns you already know."
      icon="あ"
    />
  {:else if state === 'error'}
    <EmptyState title="Could not load grammar" body={error} icon="!">
      <Button on:click={() => load(currentLang)}>Retry</Button>
    </EmptyState>
  {:else if totalCount === 0}
    <EmptyState title="No grammar patterns available" body="The grammar catalog is empty for this language." icon="あ" />
  {:else}
    <Card padding="md" class="summary">
      <div class="summary-label">Grammar known</div>
      <div class="summary-value">
        <span class="green">{knownCount}</span> / {totalCount}
      </div>
      <div class="summary-sub">
        Mark the patterns you already understand. This drives your grammar comprehension on mined sentences.
      </div>
    </Card>

    {#each grouped as group (group.level)}
      <Card padding="md" class="mt">
        <div class="level-head">
          <h2>{group.level}</h2>
          <Tag variant={levelVariant(group.level)} size="sm">
            {group.items.filter((p) => known.has(p.id)).length} / {group.items.length}
          </Tag>
        </div>
        <ul class="pattern-list">
          {#each group.items as p (p.id)}
            <li class="pattern-row" class:is-known={known.has(p.id)}>
              <div class="pattern-info">
                <span class="pattern-name">{p.name}</span>
                <span class="pattern-desc">{p.description}</span>
              </div>
              <button
                class="toggle"
                class:on={known.has(p.id)}
                disabled={pending.has(p.id)}
                aria-pressed={known.has(p.id)}
                aria-label={known.has(p.id) ? `Mark ${p.name} as not known` : `Mark ${p.name} as known`}
                on:click={() => toggle(p)}
              >
                {known.has(p.id) ? 'Known' : 'Mark known'}
              </button>
            </li>
          {/each}
        </ul>
      </Card>
    {/each}
  {/if}
</main>

<style>
  main { max-width: 820px; margin: 0 auto; padding: var(--s-6) var(--s-4); }
  .page-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--s-5); }
  h1 { margin: 0; font-size: 1.5rem; color: var(--c-textHi); }
  h2 { margin: 0; font-size: 1rem; color: var(--c-textHi); }
  .loading { text-align: center; color: var(--c-textMuted); margin-top: var(--s-12); }

  :global(.mt) { margin-top: var(--s-4); }
  :global(.summary) { margin-bottom: var(--s-4); }

  .summary-label { font-size: 0.72rem; color: var(--c-textMuted); text-transform: uppercase; letter-spacing: 0.06em; }
  .summary-value { font-size: 1.8rem; font-weight: 700; color: var(--c-textHi); line-height: 1.15; margin-top: var(--s-2); }
  .summary-value .green { color: var(--c-green); }
  .summary-sub { font-size: 0.82rem; color: var(--c-textMuted); margin-top: var(--s-2); line-height: 1.5; }

  .level-head { display: flex; align-items: center; gap: var(--s-3); margin-bottom: var(--s-3); }

  .pattern-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }

  .pattern-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-3) 0;
    border-bottom: 1px solid var(--c-border);
  }
  .pattern-row:last-child { border-bottom: none; }

  .pattern-info { display: flex; flex-direction: column; gap: 0.15rem; min-width: 0; }
  .pattern-name { font-size: 1rem; color: var(--c-textHi); font-weight: 600; }
  .pattern-desc { font-size: 0.82rem; color: var(--c-textMuted); }

  .toggle {
    flex-shrink: 0;
    font-family: inherit;
    font-size: 0.82rem;
    font-weight: 600;
    padding: 0.35rem 0.8rem;
    border-radius: var(--r-md);
    border: 1px solid var(--c-border);
    background: var(--c-bgRaised);
    color: var(--c-text);
    cursor: pointer;
    transition: background var(--m-fast) var(--m-easing),
                color var(--m-fast) var(--m-easing),
                border-color var(--m-fast) var(--m-easing);
  }
  .toggle:hover:not(:disabled) { border-color: var(--c-green); color: var(--c-textHi); }
  .toggle:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c-green) 40%, transparent);
  }
  .toggle:disabled { opacity: 0.55; cursor: not-allowed; }
  .toggle.on {
    background: var(--c-greenBtn);
    border-color: var(--c-greenBtn);
    color: #fff;
  }
  .toggle.on:hover:not(:disabled) { background: var(--c-greenBtnHi); border-color: var(--c-greenBtnHi); }
</style>
