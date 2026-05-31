import { browser } from '../shared/browser';
import initWasm, { init, tokenize as wasmTokenizeText } from '../wasm/ja_tokenizer.js';
import type { Token } from '../shared/types';

interface WasmToken {
  surface: string;
  lemma: string;
  reading_hira: string;
  reading_kata: string;
  pos: string;
  is_content_word: boolean;
  frequency_rank: number | null;
}

let _initPromise: Promise<void> | null = null;

function ensureInit(): Promise<void> {
  if (!_initPromise) {
    _initPromise = (async () => {
      const wasmUrl = browser.runtime.getURL('wasm/ja_tokenizer_bg.wasm');
      await initWasm(fetch(wasmUrl));
      init(new Uint8Array());
    })();
  }
  return _initPromise;
}

export async function wasmTokenize(text: string): Promise<Token[]> {
  await ensureInit();
  const raw: WasmToken[] = JSON.parse(wasmTokenizeText(text));
  return raw.map((t) => ({
    surface: t.surface,
    lemma: t.lemma,
    reading: t.reading_kata,
    reading_hira: t.reading_hira,
    pos: t.pos,
    is_content_word: t.is_content_word,
    frequency_rank: t.frequency_rank ?? null,
    // user_status is always recomputed from VocabCache in PageAnnotator.buildFragment;
    // setting 'unknown' here is a safe default that is never directly rendered.
    user_status: 'unknown' as const,
  }));
}
