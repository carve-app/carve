<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    fetchPlacementTest,
    markKnownWords,
    submitPlacementTest,
    subscribeStarterDeck,
    type PlacementResult,
    type PlacementTest,
  } from '$lib/api';

  let step = 1;
  let language = 'ja';
  let knownGroups = new Set<number>();
  let loading = false;
  let errorMsg = '';
  let starterDeckStatus: 'subscribed' | 'unavailable' | null = null;
  let placementPhase: 'intro' | 'testing' | 'result' = 'intro';
  let placementTest: PlacementTest | null = null;
  let placementResult: PlacementResult | null = null;
  let placementIndex = 0;
  let placementAnswers: Record<string, number> = {};

  const TOTAL_STEPS = 5;

  const LANG_OPTIONS = [
    { code: 'ja',    flag: '🇯🇵', name: 'Japanese',              native: '日本語' },
    { code: 'zh-cn', flag: '🇨🇳', name: 'Chinese (Simplified)', native: '中文'   },
    { code: 'ko',    flag: '🇰🇷', name: 'Korean',               native: '한국어' },
    { code: 'es',    flag: '🇪🇸', name: 'Spanish',              native: 'Español'   },
    { code: 'de',    flag: '🇩🇪', name: 'German',               native: 'Deutsch'   },
    { code: 'fr',    flag: '🇫🇷', name: 'French',               native: 'Français'  },
    { code: 'it',    flag: '🇮🇹', name: 'Italian',              native: 'Italiano'  },
    { code: 'pt',    flag: '🇵🇹', name: 'Portuguese',           native: 'Português' },
    { code: 'en',    flag: '🇬🇧', name: 'English (intermediate+)', native: 'English' },
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
    es: [
      {
        label: 'Beginner — A1/A2',
        words: ['hola', 'gracias', 'casa', 'agua', 'comer', 'beber', 'ir', 'tener',
                'hacer', 'querer', 'grande', 'pequeño', 'bueno', 'hoy', 'mañana',
                'amigo', 'trabajo', 'tiempo', 'hablar', 'vivir'],
      },
      {
        label: 'Intermediate — B1/B2',
        words: ['conseguir', 'aunque', 'desarrollo', 'sociedad', 'aumentar', 'sin embargo',
                'experiencia', 'mejorar', 'reconocer', 'cultura', 'medio ambiente',
                'situación', 'necesario', 'difícil', 'realizar', 'suponer', 'establecer',
                'demostrar', 'consecuencia', 'tendencia'],
      },
    ],
    de: [
      {
        label: 'Beginner — A1/A2',
        words: ['hallo', 'danke', 'Haus', 'Wasser', 'essen', 'trinken', 'gehen', 'haben',
                'machen', 'wollen', 'groß', 'klein', 'gut', 'heute', 'morgen',
                'Freund', 'Arbeit', 'Zeit', 'sprechen', 'leben'],
      },
      {
        label: 'Intermediate — B1/B2',
        words: ['erreichen', 'obwohl', 'Entwicklung', 'Gesellschaft', 'erhöhen', 'jedoch',
                'Erfahrung', 'verbessern', 'erkennen', 'Kultur', 'Umwelt', 'Situation',
                'notwendig', 'schwierig', 'verwirklichen', 'annehmen', 'feststellen',
                'beweisen', 'Folge', 'Tendenz'],
      },
    ],
    fr: [
      {
        label: 'Beginner — A1/A2',
        words: ['bonjour', 'merci', 'maison', 'eau', 'manger', 'boire', 'aller', 'avoir',
                'faire', 'vouloir', 'grand', 'petit', 'bon', "aujourd'hui", 'demain',
                'ami', 'travail', 'temps', 'parler', 'vivre'],
      },
      {
        label: 'Intermediate — B1/B2',
        words: ['atteindre', 'bien que', 'développement', 'société', 'augmenter', 'cependant',
                'expérience', 'améliorer', 'reconnaître', 'culture', 'environnement',
                'situation', 'nécessaire', 'difficile', 'réaliser', 'supposer', 'établir',
                'démontrer', 'conséquence', 'tendance'],
      },
    ],
    it: [
      {
        label: 'Beginner — A1/A2',
        words: ['ciao', 'grazie', 'casa', 'acqua', 'mangiare', 'bere', 'andare', 'avere',
                'fare', 'volere', 'grande', 'piccolo', 'buono', 'oggi', 'domani',
                'amico', 'lavoro', 'tempo', 'parlare', 'vivere'],
      },
      {
        label: 'Intermediate — B1/B2',
        words: ['raggiungere', 'sebbene', 'sviluppo', 'società', 'aumentare', 'tuttavia',
                'esperienza', 'migliorare', 'riconoscere', 'cultura', 'ambiente', 'situazione',
                'necessario', 'difficile', 'realizzare', 'supporre', 'stabilire',
                'dimostrare', 'conseguenza', 'tendenza'],
      },
    ],
    pt: [
      {
        label: 'Beginner — A1/A2',
        words: ['olá', 'obrigado', 'casa', 'água', 'comer', 'beber', 'ir', 'ter',
                'fazer', 'querer', 'grande', 'pequeno', 'bom', 'hoje', 'amanhã',
                'amigo', 'trabalho', 'tempo', 'falar', 'viver'],
      },
      {
        label: 'Intermediate — B1/B2',
        words: ['alcançar', 'embora', 'desenvolvimento', 'sociedade', 'aumentar', 'no entanto',
                'experiência', 'melhorar', 'reconhecer', 'cultura', 'meio ambiente',
                'situação', 'necessário', 'difícil', 'realizar', 'supor', 'estabelecer',
                'demonstrar', 'consequência', 'tendência'],
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

  function selectLanguage(code: string) {
    language = code;
    starterDeckStatus = null;
    knownGroups = new Set();
    resetPlacement();
  }

  function resetPlacement() {
    placementPhase = 'intro';
    placementTest = null;
    placementResult = null;
    placementIndex = 0;
    placementAnswers = {};
    errorMsg = '';
  }

  async function startPlacement() {
    loading = true;
    errorMsg = '';
    try {
      placementTest = await fetchPlacementTest('en');
      placementIndex = 0;
      placementAnswers = {};
      placementPhase = 'testing';
    } catch (error) {
      errorMsg = error instanceof Error ? error.message : 'Could not load the placement test. Please try again.';
    } finally {
      loading = false;
    }
  }

  function choosePlacementAnswer(selectedIndex: number) {
    const item = placementTest?.items[placementIndex];
    if (!item) return;
    placementAnswers = { ...placementAnswers, [item.id]: selectedIndex };
  }

  async function advancePlacement() {
    if (!placementTest) return;
    const item = placementTest.items[placementIndex];
    if (!item || placementAnswers[item.id] === undefined) return;

    if (placementIndex < placementTest.items.length - 1) {
      placementIndex += 1;
      return;
    }

    loading = true;
    errorMsg = '';
    try {
      const answers = placementTest.items.map((testItem) => ({
        item_id: testItem.id,
        selected_index: placementAnswers[testItem.id],
      }));
      placementResult = await submitPlacementTest('en', placementTest.version, answers);
      placementPhase = 'result';
    } catch (error) {
      errorMsg = error instanceof Error ? error.message : 'Could not save your placement result. Please try again.';
    } finally {
      loading = false;
    }
  }

  async function primaryAction() {
    if (step === 2 && language === 'en') {
      if (placementPhase === 'intro') {
        await startPlacement();
      } else if (placementPhase === 'testing') {
        await advancePlacement();
      } else {
        step += 1;
      }
      return;
    }
    await next();
  }

  function back() {
    errorMsg = '';
    if (step === 2 && language === 'en' && placementPhase === 'testing') {
      if (placementIndex > 0) {
        placementIndex -= 1;
      } else {
        placementPhase = 'intro';
      }
      return;
    }
    step -= 1;
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

    if (step === 2 && language !== 'en') {
      const groups = WORD_GROUPS[language] ?? [];
      const lemmas: string[] = [];
      knownGroups.forEach(idx => {
        lemmas.push(...(groups[idx]?.words ?? []));
      });
      if (lemmas.length > 0) {
        loading = true;
        try {
          await markKnownWords(language, lemmas);
        } catch (error) {
          errorMsg = error instanceof Error ? error.message : 'Could not save your known words. Please try again.';
          loading = false;
          return;
        }
        loading = false;
      }
    }

    if (step === 3) {
      loading = true;
      try {
        const result = await subscribeStarterDeck(language);
        starterDeckStatus = result.status === 'subscribed' ? 'subscribed' : 'unavailable';
      } catch (error) {
        errorMsg = error instanceof Error ? error.message : 'Could not subscribe to the starter deck. Please try again.';
        loading = false;
        return;
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
    localStorage.setItem('carve_lang', language);
    goto('/cards');
  }

  function skip() {
    localStorage.setItem('carve_onboarding_done', '1');
    localStorage.setItem('carve_lang', language);
    goto('/cards');
  }

  onMount(() => {
    if (!localStorage.getItem('carve_access_token')) {
      goto('/login');
    }
  });

  $: currentGroups = WORD_GROUPS[language] ?? [];
  $: browser = typeof navigator !== 'undefined' ? detectBrowser() : 'chrome';
  $: placementItem = placementTest?.items[placementIndex] ?? null;
  $: placementSelection = placementItem ? placementAnswers[placementItem.id] : undefined;
  $: placementProgress = placementTest ? ((placementIndex + 1) / placementTest.items.length) * 100 : 0;
  $: primaryDisabled = loading || (
    step === 2 && language === 'en' && placementPhase === 'testing' && placementSelection === undefined
  );
  $: primaryLabel = loading
    ? (placementPhase === 'testing' ? 'Calculating…' : 'Loading…')
    : step === TOTAL_STEPS
      ? 'Go to my cards →'
      : step === 2 && language === 'en'
        ? placementPhase === 'intro'
          ? 'Start the test →'
          : placementPhase === 'testing'
            ? placementTest && placementIndex === placementTest.items.length - 1
              ? 'See my result →'
              : 'Next question →'
            : 'Continue →'
        : 'Continue →';
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
              on:click={() => selectLanguage(opt.code)}
            >
              <span class="lang-flag">{opt.flag}</span>
              <span class="lang-name">{opt.name}</span>
              <span class="lang-native">{opt.native}</span>
            </button>
          {/each}
          <div class="lang-card disabled" aria-disabled="true" title="Coming next">
            <span class="lang-flag" aria-hidden="true">🌐</span>
            <span class="lang-name">More soon</span>
            <span class="lang-native">ES, FR, DE…</span>
          </div>
        </div>

      {:else if step === 2}
        {#if language === 'en'}
          {#if placementPhase === 'intro'}
            <span class="eyebrow">English placement</span>
            <h2>Find your vocabulary starting point</h2>
            <p class="sub">
              Choose the closest meaning of 30 words in context. The questions move from
              frequent everyday words to less common vocabulary.
            </p>
            <div class="placement-intro">
              <div class="placement-stat"><strong>30</strong><span>questions</span></div>
              <div class="placement-stat"><strong>~4</strong><span>minutes</span></div>
              <div class="placement-stat"><strong>1</strong><span>honest estimate</span></div>
            </div>
            <p class="method-note">
              This measures receptive vocabulary, not overall English proficiency. Choose
              “I don’t know” instead of guessing; Carve will keep refining the result as you read.
            </p>
          {:else if placementPhase === 'testing' && placementItem && placementTest}
            <div class="question-meta">
              <span>Question {placementIndex + 1} of {placementTest.items.length}</span>
              <span>{Math.round(placementProgress)}%</span>
            </div>
            <div class="question-progress" aria-hidden="true">
              <div class="question-progress-fill" style={`width: ${placementProgress}%`}></div>
            </div>
            <h2>What does <span class="tested-word">“{placementItem.word}”</span> mean here?</h2>
            <p class="question-prompt">{placementItem.prompt}</p>
            <div class="answer-list" role="radiogroup" aria-label={`Meaning of ${placementItem.word}`}>
              {#each placementItem.options as option, i}
                <button
                  class="answer-option"
                  class:selected={placementSelection === i}
                  role="radio"
                  aria-checked={placementSelection === i}
                  on:click={() => choosePlacementAnswer(i)}
                >
                  <span class="answer-letter">{String.fromCharCode(65 + i)}</span>
                  <span>{option}</span>
                </button>
              {/each}
              <button
                class="answer-option unknown-option"
                class:selected={placementSelection === -1}
                role="radio"
                aria-checked={placementSelection === -1}
                on:click={() => choosePlacementAnswer(-1)}
              >
                <span class="answer-letter">?</span>
                <span>I don’t know this word</span>
              </button>
            </div>
          {:else if placementPhase === 'result' && placementResult}
            <span class="eyebrow">Your starting point</span>
            <h2>{placementResult.result_label} receptive vocabulary</h2>
            <div class="placement-result">
              <div class="estimate-label">Estimated vocabulary</div>
              <div class="estimate-value">~{placementResult.estimated_known.toLocaleString()}</div>
              <div class="estimate-unit">high-frequency English words</div>
              <div class="estimate-range">
                Likely range {placementResult.estimate_lower.toLocaleString()}–{placementResult.estimate_upper.toLocaleString()}
              </div>
            </div>
            <div class="result-details">
              <div>
                <strong>{placementResult.correct}/{placementResult.total}</strong>
                <span>meanings recognized</span>
              </div>
              <div>
                <strong>{placementResult.verified_known}</strong>
                <span>verified words saved</span>
              </div>
            </div>
            <p class="method-note">
              The estimate helps choose suitable content. Only answers you demonstrated were
              marked known, so unfamiliar words stay visible instead of disappearing on an assumption.
            </p>
            <button class="retake-btn" on:click={resetPlacement}>Retake placement test</button>
          {/if}
        {:else}
          <h2>Which of these words do you already know?</h2>
          <p class="sub">
            Select the groups you're comfortable with. A scored placement test is currently available for English;
            other languages will follow.
          </p>
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
        {/if}

      {:else if step === 3}
        <h2>Start with a curated deck</h2>
        <p class="sub">
          If an official starter deck is available for your language, we'll add its
          high-frequency words to your collection. You can continue without one.
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
          {:else if language === 'en'}
            <div class="deck-card">
              <div class="deck-icon">🇬🇧</div>
              <div class="deck-info">
                <div class="deck-name">CEFR B2 Academic Word List</div>
                <div class="deck-desc">High-utility vocabulary for advanced learners · Official deck</div>
              </div>
            </div>
          {:else}
            <div class="deck-card">
              <div class="deck-icon">📚</div>
              <div class="deck-info">
                <div class="deck-name">No official starter deck yet</div>
                <div class="deck-desc">Continue now and mine or import your own vocabulary.</div>
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
          {starterDeckStatus === 'subscribed'
            ? 'Your first deck is ready to review.'
            : 'Your language is configured and ready for cards you mine or import.'}
          Use the extension to mine words from any content you read or watch.
        </p>
        <div class="done-art">🎉</div>
        <div class="done-steps">
          <div class="done-item">✓ Language selected</div>
          <div class="done-item">✓ Known words marked</div>
          <div class="done-item">
            {starterDeckStatus === 'subscribed' ? '✓ Starter deck subscribed' : '✓ Starter deck availability checked'}
          </div>
        </div>
      {/if}

      {#if errorMsg}
        <p class="save-error" role="alert">{errorMsg}</p>
      {/if}

    </div>

    <div class="wizard-footer">
      {#if step > 1}
        <button class="btn-ghost" on:click={back} disabled={loading}>Back</button>
      {:else}
        <div></div>
      {/if}
      <button class="btn-primary" on:click={primaryAction} disabled={primaryDisabled}>
        {primaryLabel}
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

  .dot.done { background: #2e7d32; }
  .dot.active { background: #81c784; width: 18px; border-radius: 4px; }

  .skip-btn {
    background: none;
    border: none;
    color: #8a96b3;
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

  .save-error {
    color: #ef9a9a;
    background: #2a1719;
    border: 1px solid #6d2c31;
    border-radius: 8px;
    padding: 0.75rem;
    margin: 1rem 0 0;
    font-size: 0.85rem;
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
  .lang-card.disabled { cursor: default; border-color: #2a2d36; background: #161922; }
  .lang-card.disabled .lang-name { color: #a8b2c8; }
  .lang-card.disabled .lang-native { color: #8a96b3; }

  .lang-flag { font-size: 1.75rem; }
  .lang-name { font-size: 0.9rem; font-weight: 600; color: #c8d0e0; }
  .lang-native { font-size: 0.8rem; color: #8a96b3; }

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

  .word-sample { font-size: 0.78rem; color: #8a96b3; line-height: 1.7; }
  .more { color: #8a96b3; margin-left: 0.25rem; }

  /* ── English placement test ── */

  .eyebrow {
    display: block;
    color: #81c784;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    margin-bottom: 0.5rem;
  }

  .placement-intro {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0.6rem;
    margin: 0.25rem 0 1.25rem;
  }

  .placement-stat {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
    padding: 0.9rem 0.5rem;
    background: #13151a;
    border: 1px solid #2a2d36;
    border-radius: 9px;
    text-align: center;
  }
  .placement-stat strong { color: #e8eaf0; font-size: 1.2rem; }
  .placement-stat span { color: #8a96b3; font-size: 0.7rem; }

  .method-note {
    margin: 0;
    padding: 0.8rem 0.9rem;
    color: #8a96b3;
    background: #171a20;
    border-left: 3px solid #3b6f3e;
    border-radius: 0 7px 7px 0;
    font-size: 0.78rem;
    line-height: 1.55;
  }

  .question-meta {
    display: flex;
    justify-content: space-between;
    color: #8a96b3;
    font-size: 0.75rem;
    margin-bottom: 0.45rem;
  }

  .question-progress {
    height: 4px;
    overflow: hidden;
    background: #2a2d36;
    border-radius: 4px;
    margin-bottom: 1.5rem;
  }

  .question-progress-fill {
    height: 100%;
    background: #4caf50;
    border-radius: inherit;
    transition: width 0.2s ease;
  }

  .tested-word { color: #81c784; }

  .question-prompt {
    margin: 0.85rem 0 1.15rem;
    padding: 0.85rem 1rem;
    color: #c8d0e0;
    background: #13151a;
    border-radius: 8px;
    font-size: 0.92rem;
    font-style: italic;
    line-height: 1.5;
  }

  .answer-list { display: flex; flex-direction: column; gap: 0.55rem; }

  .answer-option {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.65rem 0.75rem;
    color: #b8c1d3;
    background: #17191f;
    border: 1px solid #2f333d;
    border-radius: 8px;
    font-size: 0.84rem;
    text-align: left;
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s, color 0.15s;
  }
  .answer-option:hover { border-color: #4f7652; background: #1a201c; }
  .answer-option.selected { border-color: #4caf50; background: #162316; color: #e8eaf0; }

  .answer-letter {
    display: grid;
    place-items: center;
    flex: 0 0 1.55rem;
    height: 1.55rem;
    color: #8a96b3;
    background: #242832;
    border-radius: 5px;
    font-size: 0.72rem;
    font-weight: 700;
  }
  .answer-option.selected .answer-letter { color: #d8f3da; background: #2e6b32; }
  .unknown-option { color: #8a96b3; border-style: dashed; }

  .placement-result {
    margin: 1.1rem 0 0.75rem;
    padding: 1.25rem;
    background: linear-gradient(145deg, #142016, #13151a);
    border: 1px solid #315234;
    border-radius: 11px;
    text-align: center;
  }
  .estimate-label { color: #8a96b3; font-size: 0.72rem; text-transform: uppercase; letter-spacing: 0.08em; }
  .estimate-value { color: #a5d6a7; font-size: 2.25rem; font-weight: 800; line-height: 1.2; margin-top: 0.25rem; }
  .estimate-unit { color: #b8c1d3; font-size: 0.8rem; }
  .estimate-range { color: #8a96b3; font-size: 0.72rem; margin-top: 0.65rem; }

  .result-details {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.65rem;
    margin-bottom: 0.9rem;
  }
  .result-details div {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0.65rem;
    background: #17191f;
    border-radius: 8px;
    text-align: center;
  }
  .result-details strong { color: #e8eaf0; font-size: 0.95rem; }
  .result-details span { color: #8a96b3; font-size: 0.7rem; }

  .retake-btn {
    display: block;
    margin: 0.8rem auto 0;
    padding: 0.3rem;
    color: #8a96b3;
    background: transparent;
    border: 0;
    font-size: 0.78rem;
    text-decoration: underline;
    cursor: pointer;
  }
  .retake-btn:hover { color: #c8d0e0; }

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
  .deck-desc { font-size: 0.8rem; color: #8a96b3; margin-top: 0.15rem; }

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
  .ext-note { font-size: 0.8rem; color: #8a96b3; }

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
    background: #2e7d32;
    color: #fff;
    border: none;
    padding: 0.65rem 1.5rem;
    border-radius: 7px;
    font-size: 0.95rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-primary:hover:not(:disabled) { background: #2f7d34; }
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
  .btn-ghost:hover:not(:disabled) { border-color: #8a96b3; color: #9ba8c0; }
  .btn-ghost:disabled { opacity: 0.5; cursor: not-allowed; }

  @media (max-width: 480px) {
    .wizard-body { padding: 1.5rem 1rem 1.25rem; }
    .wizard-footer { padding: 0.9rem 1rem 1.1rem; }
    .placement-intro { gap: 0.4rem; }
    .placement-stat { padding: 0.7rem 0.25rem; }
    .answer-option { padding: 0.6rem; }
  }
</style>
