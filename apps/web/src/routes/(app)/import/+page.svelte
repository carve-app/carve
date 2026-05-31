<script lang="ts">
  import { get } from 'svelte/store';
  import { lang } from '$lib/stores/lang';
  import { toasts } from '$lib/stores/toast';
  import {
    importLibraryFile,
    importAnki,
    importMigakuCSV,
    importYomitan,
    importJPDBCSV,
    type ImportResult,
  } from '$lib/api';
  import { goto } from '$app/navigation';

  type Tab = 'file' | 'vocab';
  let activeTab: Tab = 'file';

  // File reader (txt/srt)
  let fileReaderFile: File | null = null;
  let fileReaderLanguage = get(lang);
  let fileReaderLoading = false;

  // Vocab/deck import
  type ImportKind = 'anki' | 'migaku' | 'yomitan' | 'jpdb';
  let vocabKind: ImportKind = 'anki';
  let vocabFile: File | null = null;
  let vocabLanguage = get(lang);
  let vocabLoading = false;
  let vocabResult: ImportResult | null = null;

  const LANGUAGES = [
    { code: 'ja', label: 'Japanese' },
    { code: 'zh', label: 'Chinese' },
    { code: 'ko', label: 'Korean' },
  ];

  const IMPORT_KINDS: { id: ImportKind; label: string; accept: string; description: string }[] = [
    {
      id: 'anki',
      label: 'Anki deck (.apkg)',
      accept: '.apkg',
      description: 'Imports cards from an Anki package. Maps the first three fields to front text, back text, and sentence.',
    },
    {
      id: 'migaku',
      label: 'Migaku CSV',
      accept: '.csv',
      description: 'Exports from Migaku: word, reading, definition, sentence columns.',
    },
    {
      id: 'yomitan',
      label: 'Yomitan vocab (.zip)',
      accept: '.zip',
      description: 'Marks words as known in your vocabulary list. No flashcards are created.',
    },
    {
      id: 'jpdb',
      label: 'JPDB known-words (.csv)',
      accept: '.csv',
      description: 'Imports your JPDB vocabulary list as known/learning words.',
    },
  ];

  function handleFileReaderDrop(e: DragEvent) {
    const file = e.dataTransfer?.files[0];
    if (file) fileReaderFile = file;
  }

  function handleVocabDrop(e: DragEvent) {
    const file = e.dataTransfer?.files[0];
    if (file) vocabFile = file;
  }

  async function importFileReader() {
    if (!fileReaderFile) return;
    fileReaderLoading = true;
    try {
      const res = await importLibraryFile(fileReaderFile, fileReaderLanguage);
      toasts.add(`"${res.title}" added to library`);
      goto(`/library/${res.id}`);
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Import failed');
    }
    fileReaderLoading = false;
  }

  async function importVocab() {
    if (!vocabFile) return;
    vocabLoading = true;
    vocabResult = null;
    try {
      let res: ImportResult;
      switch (vocabKind) {
        case 'anki':   res = await importAnki(vocabFile, vocabLanguage); break;
        case 'migaku': res = await importMigakuCSV(vocabFile, vocabLanguage); break;
        case 'yomitan':res = await importYomitan(vocabFile, vocabLanguage); break;
        case 'jpdb':   res = await importJPDBCSV(vocabFile, vocabLanguage); break;
      }
      vocabResult = res;
    } catch (err) {
      toasts.add(err instanceof Error ? err.message : 'Import failed');
    }
    vocabLoading = false;
  }

  const kindInfo = (id: ImportKind) => IMPORT_KINDS.find(k => k.id === id)!;

  function onFileReaderChange(e: Event) {
    fileReaderFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
  }

  function onVocabFileChange(e: Event) {
    vocabFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
    vocabResult = null;
  }
</script>

<main>
  <div class="page-header">
    <a href="/library" class="back">← Library</a>
    <h1>Import</h1>
  </div>

  <!-- Tabs -->
  <div class="tabs">
    <button class="tab" class:active={activeTab === 'file'} on:click={() => activeTab = 'file'}>
      Read a file
    </button>
    <button class="tab" class:active={activeTab === 'vocab'} on:click={() => activeTab = 'vocab'}>
      Import cards / vocab
    </button>
  </div>

  <!-- Tab: File reader import -->
  {#if activeTab === 'file'}
    <div class="section">
      <p class="desc">Upload a plain text or subtitle file to read it in the annotated reader.</p>

      <div class="field">
        <label>Language</label>
        <select bind:value={fileReaderLanguage}>
          {#each LANGUAGES as l}
            <option value={l.code}>{l.label}</option>
          {/each}
        </select>
      </div>

      <div
        class="drop-zone"
        class:has-file={!!fileReaderFile}
        on:dragover|preventDefault
        on:drop|preventDefault={handleFileReaderDrop}
      >
        {#if fileReaderFile}
          <div class="file-name">{fileReaderFile.name}</div>
          <button class="clear-btn" on:click={() => fileReaderFile = null}>✕</button>
        {:else}
          <p>Drop a <strong>.txt</strong> or <strong>.srt</strong> file here</p>
          <label class="file-label">
            Browse
            <input
              type="file"
              accept=".txt,.srt"
              on:change={onFileReaderChange}
            />
          </label>
        {/if}
      </div>

      <button
        class="import-btn"
        disabled={!fileReaderFile || fileReaderLoading}
        on:click={importFileReader}
      >
        {fileReaderLoading ? 'Importing…' : 'Import and open reader'}
      </button>
    </div>

  <!-- Tab: Cards/vocab import -->
  {:else}
    <div class="section">
      <p class="desc">Import flashcards from Anki or vocabulary lists from Migaku, Yomitan, or JPDB.</p>

      <div class="kind-grid">
        {#each IMPORT_KINDS as k}
          <button
            class="kind-card"
            class:active={vocabKind === k.id}
            on:click={() => { vocabKind = k.id; vocabFile = null; vocabResult = null; }}
          >
            <div class="kind-label">{k.label}</div>
            <div class="kind-desc">{k.description}</div>
          </button>
        {/each}
      </div>

      <div class="field">
        <label>Language</label>
        <select bind:value={vocabLanguage}>
          {#each LANGUAGES as l}
            <option value={l.code}>{l.label}</option>
          {/each}
        </select>
      </div>

      <div
        class="drop-zone"
        class:has-file={!!vocabFile}
        on:dragover|preventDefault
        on:drop|preventDefault={handleVocabDrop}
      >
        {#if vocabFile}
          <div class="file-name">{vocabFile.name}</div>
          <button class="clear-btn" on:click={() => { vocabFile = null; vocabResult = null; }}>✕</button>
        {:else}
          <p>Drop a <strong>{kindInfo(vocabKind).accept}</strong> file here</p>
          <label class="file-label">
            Browse
            <input
              type="file"
              accept={kindInfo(vocabKind).accept}
              on:change={onVocabFileChange}
            />
          </label>
        {/if}
      </div>

      {#if vocabResult}
        <div class="result-box">
          <div class="result-row">
            <span class="result-label">Imported</span>
            <span class="result-val success">{vocabResult.imported}</span>
          </div>
          <div class="result-row">
            <span class="result-label">Skipped</span>
            <span class="result-val muted">{vocabResult.skipped}</span>
          </div>
          {#if vocabResult.type === 'known_words'}
            <p class="result-note">Words added to your vocabulary — no flashcards created. They will be tracked as known during comprehension scoring.</p>
          {:else}
            <p class="result-note">Cards are ready for review in your deck.</p>
          {/if}
        </div>
      {/if}

      <button
        class="import-btn"
        disabled={!vocabFile || vocabLoading}
        on:click={importVocab}
      >
        {vocabLoading ? 'Importing…' : `Import ${kindInfo(vocabKind).label}`}
      </button>
    </div>
  {/if}
</main>

<style>
  main { max-width: 680px; margin: 0 auto; padding: 1.5rem 1rem 3rem; }

  .page-header { display: flex; align-items: baseline; gap: 1.25rem; margin-bottom: 1.5rem; }
  .back { color: #6b7591; text-decoration: none; font-size: 0.85rem; }
  .back:hover { color: #e8eaf0; }
  h1 { margin: 0; font-size: 1.25rem; }

  .tabs { display: flex; gap: 0; border-bottom: 1px solid #2a2d36; margin-bottom: 1.5rem; }
  .tab {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: #6b7591;
    padding: 0.55rem 1.25rem;
    font-size: 0.9rem;
    cursor: pointer;
    margin-bottom: -1px;
    transition: color 0.15s, border-color 0.15s;
  }
  .tab.active { color: #e8eaf0; border-bottom-color: #4caf50; }

  .section { display: flex; flex-direction: column; gap: 1.25rem; }
  .desc { color: #9ba8c0; font-size: 0.9rem; margin: 0; }

  .field { display: flex; flex-direction: column; gap: 0.35rem; }
  .field label { font-size: 0.78rem; color: #6b7591; text-transform: uppercase; letter-spacing: 0.05em; }

  select {
    background: #13151a;
    border: 1px solid #2a2d36;
    border-radius: 7px;
    color: #e8eaf0;
    padding: 0.45rem 0.75rem;
    font-size: 0.9rem;
    width: 160px;
  }

  .kind-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 0.75rem; }

  .kind-card {
    background: #1e2128;
    border: 1.5px solid #2a2d36;
    border-radius: 9px;
    padding: 0.75rem 1rem;
    text-align: left;
    cursor: pointer;
    transition: border-color 0.15s;
  }
  .kind-card.active { border-color: #4caf50; background: #1a2a1a; }
  .kind-label { font-size: 0.88rem; font-weight: 500; color: #e8eaf0; margin-bottom: 0.25rem; }
  .kind-desc { font-size: 0.74rem; color: #6b7591; line-height: 1.4; }

  .drop-zone {
    border: 2px dashed #2a2d36;
    border-radius: 10px;
    padding: 2rem 1.5rem;
    text-align: center;
    color: #6b7591;
    font-size: 0.9rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.75rem;
    transition: border-color 0.15s;
  }
  .drop-zone:hover { border-color: #4a5568; }
  .drop-zone.has-file { border-color: #4caf50; border-style: solid; }
  .drop-zone p { margin: 0; }

  .file-name { color: #c8d0e0; font-size: 0.92rem; word-break: break-all; }

  .file-label {
    background: #1e2128;
    border: 1px solid #2a2d36;
    color: #9ba8c0;
    padding: 0.35rem 1rem;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
  }
  .file-label input { display: none; }

  .clear-btn { background: none; border: none; color: #6b7591; cursor: pointer; font-size: 1rem; }

  .import-btn {
    background: #4caf50;
    color: #fff;
    border: none;
    padding: 0.65rem 1.5rem;
    border-radius: 7px;
    font-size: 0.95rem;
    font-weight: 500;
    cursor: pointer;
    transition: opacity 0.15s;
    align-self: flex-start;
  }
  .import-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .result-box {
    background: #1e2128;
    border: 1px solid #2a3d2a;
    border-radius: 9px;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .result-row { display: flex; justify-content: space-between; align-items: center; }
  .result-label { font-size: 0.82rem; color: #9ba8c0; }
  .result-val { font-size: 1rem; font-weight: 600; }
  .result-val.success { color: #4caf50; }
  .result-val.muted { color: #6b7591; }
  .result-note { font-size: 0.8rem; color: #6b7591; margin: 0.25rem 0 0; }
</style>
