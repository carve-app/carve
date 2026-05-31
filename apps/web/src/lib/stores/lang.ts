import { writable } from 'svelte/store';

export type LangCode = 'ja' | 'zh-cn' | 'ko' | 'en';

export const LANG_LABELS: Record<LangCode, string> = {
  'ja': '日本語',
  'zh-cn': '中文',
  'ko': '한국어',
  'en': 'English',
};

function createLang() {
  const initial: LangCode =
    typeof localStorage !== 'undefined'
      ? (localStorage.getItem('carve_lang') as LangCode) || 'ja'
      : 'ja';

  const { subscribe, set: _set } = writable<LangCode>(initial);

  function set(code: LangCode) {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('carve_lang', code);
    }
    _set(code);
  }

  return { subscribe, set };
}

export const lang = createLang();
