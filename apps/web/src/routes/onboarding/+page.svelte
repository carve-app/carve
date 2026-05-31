<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { markKnownWords, subscribeStarterDeck, ApiError } from '$lib/api';

  let step = 1;
  let language = 'ja';
  let knownGroups = new Set<number>();
  let loading = false;
  let errorMsg = '';

  const TOTAL_STEPS = 5;

  const LANG_OPTIONS = [
    { code: 'ja',    flag: '🇯🇵', name: 'Japanese',              native: '日本語' },
    { code: 'zh-cn', flag: '🇨🇳', name: 'Chinese (Simplified)', native: '中文'   },
    { code: 'ko',    flag: '🇰🇷', name: 'Korean',               native: '한국어' },
    { code: 'en',    flag: '🇬🇧', name: 'English',              native: 'English' },
  ];

  const WORD_GROUPS: Record<string, { label: string; words: string[] }[]> = {
    ja: [
      {
        label: 'Beginner — JLPT N5',
        words: ['私', 'あなた', '今日', '明日', '昨日', '水', '食べる', '飲む', '行く', '来る',
                '見る', '聞く', '話す', '読む', '書く', '大きい', '小さい', '好き', '嫌い', '電車'],
      },
      {
        label: 'Elementary — JLPT N4',
        words: ['働く', '始める', '終わる', '考える', '覚える', '忘れる', '難しい', '楽しい',
                '嬉しい', '悲しい', '便利', '有名', '旅行', '料理', '天気', '春', '夏', '秋', '冬', '言葉'],
      },
      {
        label: 'Intermediate — JLPT N3',
        words: ['経験', '成功', '失敗', '目的', '方法', '理由', '結果', '影響', '変化', '発展',
                '社会', '文化', '政治', '経済', '環境', '技術', '医療', '教育', '研究', '情報'],
      },
    ],
    'zh-cn': [
      {
        label: 'Beginner — HSK 1',
        words: ['你', '我', '他', '是', '有', '不', '在', '人', '这', '中',
                '大', '上', '国', '的', '和', '来', '学', '生', '好', '日'],
      },
      {
        label: 'Elementary — HSK 2',
        words: ['因为', '所以', '但是', '虽然', '已经', '还是', '就是', '可以', '应该', '知道',
                '觉得', '喜欢', '需要', '希望', '重要', '发现', '问题', '方法', '时间', '地方'],
      },
      {
        label: 'Intermediate — HSK 3',
        words: ['经济', '发展', '社会', '文化', '科技', '教育', '医疗', '环境', '政治', '历史',
                '影响', '变化', '研究', '结果', '目的', '方向', '情况', '关系', '意义', '作用'],
      },
    ],
    ko: [
      {
        label: 'Beginner — TOPIK 1',
        words: ['나', '저', '이', '그', '학교', '집', '음식', '물', '친구', '선생님',
                '오늘', '내일', '어제', '좋다', '싫다', '크다', '작다', '가다', '오다', '보다'],
      },
      {
        label: 'Elementary — TOPIK 2',
        words: ['공부하다', '일하다', '시작하다', '끝나다', '생각하다', '알다', '모르다',
                '중요하다', '어렵다', '재미있다', '시간', '방법', '이유', '결과', '문화',
                '사회', '여행', '음악', '영화', '역사'],
      },
    ],
    en: [
      {
        label: 'Intermediate — CEFR B1',
        words: ['achieve', 'attempt', 'consider', 'mention', 'realize', 'suggest',
                'manage', 'increase', 'improve', 'recognize', 'experience', 'opportunity',
                'situation', 'culture', 'society', 'environment', 'common', 'difficult',
                'necessary', 'recently'],
      },
      {
        label: 'Upper-intermediate — CEFR B2',
        words: ['assume', 'demonstrate', 'establish', 'evaluate', 'illustrate', 'imply',
                'pursue', 'reveal', 'undertake', 'witness', 'consequence', 'criterion',
                'phenomenon', 'tendency', 'sustainable', 'thorough', 'inevitable',
                'controversial', 'inherent', 'arguably'],
      },
      {
        label: 'Advanced — CEFR C1/C2',
        words: ['acquiesce', 'circumvent', 'corroborate', 'delineate', 'eschew',
                'exacerbate', 'mitigate', 'obfuscate', 'reconcile', 'underscore',
                'aberration', 'caveat', 'discrepancy', 'hegemony', 'paradigm',
                'predicament', 'quintessential', 'salient', 'tantamount', 'ubiquitous'],
      },
    ],
  };

  function toggleGroup(idx: number) {
    if (knownGroups.has(idx)) {
      knownGroups.delete(idx);
    } else {
      knownGroups.add(idx);
    }
    knownGroups = new Set(knownGroups);
  }

  function detectBrowser(): 'chrome' | 'firefox' | 'safari' | 'other' {
    const ua = navigator.userAgent;
    if (ua.includes('Firefox')) return 'firefox';
    if (ua.includes('Chrome') && !ua.includes('Edg')) return 'chrome';
    if (ua.includes('Safari') && !ua.includes('Chrome')) return 'safari';
    return 'other';
  }

  async function next() {
    errorMsg = '';

    if (step === 2) {
      const groups = WORD_GROUPS[language] ?? [];
      const lemmas: string[] = [];
      knownGroups.forEach(idx => {
        lemmas.push(...(groups[idx]?.words ?? []));
      });
      if (lemmas.length > 0) {
        loading = true;
        try {
          await markKnownWords(language, lemmas);
        } catch {
          // non-fatal
        }
        loading = false;
      }
    }

    if (step === 3) {
      loading = true;
      try {
        await subscribeStarterDeck(language);
      } catch {
        // non-fatal
      }
      loading = false;
    }

    if (step === TOTAL_STEPS) {
      finish();
      return;
    }
    step++;
  }

  function finish() {
    localStorage.setItem('carve_onboarding_done', '1');
    goto('/cards');
  }

  function skip() {
    localStorage.setItem('carve_onboarding_done', '1');
    goto('/cards');
  }

  onMount(() => {
    if (!localStorage.getItem('carve_access_token')) {
      goto('/login');
    }
  });

  $: currentGroups = WORD_GROUPS[language] ?? [];
  $: browser = typeof navigator !== 'undefined' ? detectBrowser() : 'chrome';
</script>

<main>
  <div class="wizard">
    <div class="wizard-header">
      <a href="/" class="brand">Carve</a>
      <div class="progress">
        {#each Array(TOTAL_STEPS) as _, i}
          <div class="dot" class:active={i + 1 === step} class:done={i + 1 < step}></div>
        {/each}
      </div>
      <button class="skip-btn" on:click={skip}>Skip setup</button>
    </div>

    <div class="wizard-body">

      {#if step === 1}
        <h2>What language are you learning?</h2>
        <p class="sub">You can add more languages later in Settings.</p>
        <div class="lang-grid">
          {#each LANG_OPTIONS as opt}
            <button
              class="lang-card"
              class:selected={language === opt.code}
              on:click={() => language = opt.code}
            >
              <span class="lang-flag">{opt.flag}</span>
              <span class="lang-name">{opt.name}</span>
              <span class="lang-native">{opt.native}</span>
            </button>
          {/each}
          <div class="lang-card disabled" title="Coming next">
            <span class="lang-flag">🌐</span>
            <span class="lang-name">More soon</span>
            <span class="lang-native">ES, FR, DE…</span>
          </div>
        </div>

      {:else if step === 2}
        <h2>Which of these words do you already know?</h2>
        <p class="sub">Select the groups you're comfortable with. This helps us skip cards you don't need.</p>
        <div class="group-list">
          {#each currentGroups as group, i}
            <button
              class="group-card"
              class:selected={knownGroups.has(i)}
              on:click={() => toggleGroup(i)}
            >
              <div class="group-header">
                <span class="group-label">{group.label}</span>
                <span class="group-check">{knownGroups.has(i) ? '✓' : ''}</span>
              </div>
              <div class="word-sample">
                {group.words.slice(0, 8).join('　')}
                {#if group.words.length > 8}<span class="more">+{group.words.length - 8} more</span>{/if}
              </div>
            </button>
          {/each}
        </div>

      {:else if step === 3}
        <h2>Start with a curated deck</h2>
        <p class="sub">
          We'll subscribe you to the official starter deck for your language — a set of
          high-frequency words to build your foundation.
        </p>
        <div class="deck-preview">
          {#if language === 'ja'}
            <div class="deck-card">
              <div class="deck-icon">🎌</div>
              <div class="deck-info">
                <div class="deck-name">JLPT N5 Core Vocabulary</div>
                <div class="deck-desc">50 essential words · Official deck</div>
              </div>
            </div>
          {:else if language === 'zh-cn'}
            <div class="deck-card">
              <div class="deck-icon">🇨🇳</div>
              <div class="deck-info">
                <div class="deck-name">HSK 1 Core Vocabulary</div>
                <div class="deck-desc">High-frequency words · Official deck</div>
              </div>
            </div>
          {:else if language === 'ko'}
            <div class="deck-card">
              <div class="deck-icon">🇰🇷</div>
              <div class="deck-info">
                <div class="deck-name">TOPIK 1 Core Vocabulary</div>
                <div class="deck-desc">Essential vocabulary · Official deck</div>
              </div>
            </div>
          {:else if language === 'en'}
            <div class="deck-card">
              <div class="deck-icon">🇬🇧</div>
              <div class="deck-info">
                <div class="deck-name">CEFR B2 Academic Word List</div>
                <div class="deck-desc">High-utility vocabulary for advanced learners · Official deck</div>
              </div>
            </div>
          {/if}
        </div>

      {:else if step === 4}
        <h2>Install the browser extension</h2>
        <p class="sub">
          The Carve extension lets you mine vocabulary directly from any webpage or video with a single click.
        </p>
        <div class="ext-options">
          <a
            href="https://chrome.google.com/webstore"
            target="_blank"
            rel="noopener"
            class="ext-btn"
            class:highlighted={browser === 'chrome'}
          >
            <span class="ext-icon">🟡</span>
            Chrome / Brave
          </a>
          <a
            href="https://addons.mozilla.org"
            target="_blank"
            rel="noopener"
            class="ext-btn"
            class:highlighted={browser === 'firefox'}
          >
            <span class="ext-icon">🦊</span>
            Firefox
          </a>
        </div>
        <p class="ext-note">You can skip this and install it later — the extension isn't required to use Carve.</p>

      {:else if step === 5}
        <h2>You're all set!</h2>
        <p class="sub">
          Your first deck is ready to review. Start building your vocabulary, and use the extension
          to mine words from any content you read or watch.
        </p>
        <div class="done-art">🎉</div>
        <div class="done-steps">
          <div class="done-item">✓ Language selected</div>
          <div class="done-item">✓ Known words marked</div>
          <div class="done-item">✓ Starter deck subscribed</div>
        </div>
      {/if}

    </div>

    <div class="wizard-footer">
      {#if step > 1}
        <button class="btn-ghost" on:click={() => step--} disabled={loading}>Back</button>
      {:else}
        <div></div>
      {/if}
      <button class="btn-primary" on:click={next} disabled={loading}>
        {loading ? 'Saving…' : step === TOTAL_STEPS ? 'Go to my cards →' : 'Continue →'}
      </button>
    </div>
  </div>
</main>

<style>
  main {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
  }

  .wizard {
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 14px;
    width: 100%;
    max-width: 540px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  /* ── Header ── */

  .wizard-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.5rem;
    border-bottom: 1px solid #2a2d36;
  }

  .brand {
    font-size: 1.1rem;
    font-weight: 800;
    color: #4caf50;
    text-decoration: none;
    letter-spacing: -0.03em;
  }

  .progress {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #2a2d36;
    transition: background 0.2s;
  }

  .dot.done { background: #4caf50; }
  .dot.active { background: #81c784; width: 18px; border-radius: 4px; }

  .skip-btn {
    background: none;
    border: none;
    color: #6b7591;
    font-size: 0.8rem;
    cursor: pointer;
    padding: 0.2rem 0.4rem;
  }
  .skip-btn:hover { color: #9ba8c0; }

  /* ── Body ── */

  .wizard-body {
    padding: 2rem 1.75rem 1.5rem;
    flex: 1;
  }

  h2 {
    margin: 0 0 0.5rem;
    font-size: 1.25rem;
    color: #e8eaf0;
    line-height: 1.3;
  }

  .sub {
    color: #7a8aa6;
    font-size: 0.9rem;
    margin: 0 0 1.5rem;
    line-height: 1.5;
  }

  /* ── Language grid ── */

  .lang-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }

  .lang-card {
    background: #13151a;
    border: 2px solid #2a2d36;
    border-radius: 10px;
    padding: 1rem;
    cursor: pointer;
    text-align: center;
    transition: border-color 0.15s, background 0.15s;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
  }

  .lang-card:hover:not(.disabled) { border-color: #4caf50; background: #161a1e; }
  .lang-card.selected { border-color: #4caf50; background: #162316; }
  .lang-card.disabled { opacity: 0.4; cursor: default; }

  .lang-flag { font-size: 1.75rem; }
  .lang-name { font-size: 0.9rem; font-weight: 600; color: #c8d0e0; }
  .lang-native { font-size: 0.8rem; color: #6b7591; }

  /* ── Word groups ── */

  .group-list { display: flex; flex-direction: column; gap: 0.75rem; }

  .group-card {
    background: #13151a;
    border: 2px solid #2a2d36;
    border-radius: 9px;
    padding: 0.9rem 1rem;
    text-align: left;
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s;
    width: 100%;
  }

  .group-card:hover { border-color: #4caf50; background: #161a1e; }
  .group-card.selected { border-color: #4caf50; background: #162316; }

  .group-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.4rem;
  }

  .group-label { font-size: 0.88rem; font-weight: 600; color: #c8d0e0; }
  .group-check { color: #4caf50; font-weight: 700; width: 1rem; text-align: right; }

  .word-sample { font-size: 0.78rem; color: #6b7591; line-height: 1.7; }
  .more { color: #4a5568; margin-left: 0.25rem; }

  /* ── Deck preview ── */

  .deck-preview { margin-top: 0.5rem; }

  .deck-card {
    display: flex;
    align-items: center;
    gap: 1rem;
    background: #13151a;
    border: 1px solid #2a2d36;
    border-radius: 9px;
    padding: 1rem 1.25rem;
  }

  .deck-icon { font-size: 2rem; }
  .deck-name { font-size: 0.95rem; font-weight: 600; color: #c8d0e0; }
  .deck-desc { font-size: 0.8rem; color: #6b7591; margin-top: 0.15rem; }

  /* ── Extension ── */

  .ext-options {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .ext-btn {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.9rem 1.25rem;
    background: #13151a;
    border: 2px solid #2a2d36;
    border-radius: 9px;
    color: #9ba8c0;
    text-decoration: none;
    font-size: 0.95rem;
    font-weight: 500;
    transition: border-color 0.15s, color 0.15s;
  }

  .ext-btn:hover { border-color: #4caf50; color: #c8d0e0; }
  .ext-btn.highlighted { border-color: #4caf50; color: #e8eaf0; }

  .ext-icon { font-size: 1.4rem; }
  .ext-note { font-size: 0.8rem; color: #4a5568; }

  /* ── Done ── */

  .done-art { font-size: 4rem; text-align: center; margin: 0.5rem 0 1.5rem; }

  .done-steps { display: flex; flex-direction: column; gap: 0.5rem; }

  .done-item {
    font-size: 0.9rem;
    color: #4caf50;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  /* ── Footer ── */

  .wizard-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem 1.75rem 1.5rem;
    border-top: 1px solid #2a2d36;
  }

  .btn-primary {
    background: #4caf50;
    color: #fff;
    border: none;
    padding: 0.65rem 1.5rem;
    border-radius: 7px;
    font-size: 0.95rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-primary:hover:not(:disabled) { background: #43a047; }
  .btn-primary:disabled { opacity: 0.6; cursor: not-allowed; }

  .btn-ghost {
    background: transparent;
    border: 1px solid #2a2d36;
    color: #7a8aa6;
    padding: 0.6rem 1.2rem;
    border-radius: 7px;
    font-size: 0.9rem;
    cursor: pointer;
    transition: border-color 0.15s, color 0.15s;
  }
  .btn-ghost:hover:not(:disabled) { border-color: #4a5568; color: #9ba8c0; }
  .btn-ghost:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
