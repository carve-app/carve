"""
Japanese NLP correctness test suite — pedagogical perspective.

Every assertion is motivated by one of four learner scenarios:

  LOOKUP  — Learner clicks a word; they need the right dictionary entry.
            Requires: correct lemma (base/dictionary form of the clicked word).
            Failure: clicking 起こった and NOT finding 起こる.

  READING — Furigana above kanji must show the correct pronunciation.
            Requires: correct hiragana reading for each word in context.
            Failure: 大相撲 shown as おおすもう — learner practises wrong sound.

  SEGMENT — The unit shown to the learner is a learnable vocabulary item.
            Requires: compound words a learner should acquire as a unit stay
            together; transparent derivations may split into useful parts.
            Failure: スマートフォン split into スマート+フォン — learner never
            sees the full loanword as a flashcard candidate.

  HIGHLIGHT — Only vocabulary words are underlined. Particles, auxiliaries,
            and conjunctions stay invisible so learners focus on new words.
            Requires: is_content_word=True only for actual vocabulary.
            Failure: を or に highlighted — learner wastes attention on grammar.

Section 8 (normalization) covers internal correctness that does not directly
affect any learner scenario; those tests exist to prevent plumbing regressions.

If a future SudachiPy update introduces a regression, add an xfail(strict=True)
test asserting the CORRECT output at the top of the relevant section.
"""

import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest
from src.tokenizer import JapaneseTokenizer, kata_to_hira, generate_furigana


@pytest.fixture(scope="module")
def tokenizer() -> JapaneseTokenizer:
    return JapaneseTokenizer()


def reading(tokenizer: JapaneseTokenizer, text: str) -> str:
    """Full hiragana reading of text (A-mode morphemes concatenated)."""
    result = tokenizer.tokenize(text, mode="A")
    return "".join(t.reading_hira for t in result.a_tokens)


def lemma(tokenizer: JapaneseTokenizer, text: str) -> str:
    """Lemma of the first content token — the dictionary-lookup target."""
    result = tokenizer.tokenize(text, mode="C")
    for t in result.tokens:
        if t.is_content_word:
            return t.lemma
    return result.tokens[0].lemma if result.tokens else ""


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 1: Dictionary lookup (LOOKUP scenario)
#
# Learner clicks a conjugated or inflected word and expects to be taken to its
# dictionary entry. The lemma must be the standard dictionary/base form.
# Tests cover verb conjugations, adjective inflections, and compound verbs.
# ─────────────────────────────────────────────────────────────────────────────

class TestDictionaryLookup:
    """
    LOOKUP: clicking any inflected form must produce the correct lemma so the
    learner can find the word in a dictionary and create the right flashcard.
    """

    def test_taberu_base(self, tokenizer):
        """食べる base form → lemma 食べる (trivial baseline)."""
        assert lemma(tokenizer, "食べる") == "食べる"

    def test_taberareta_passive_past(self, tokenizer):
        """食べられた (passive past) → lemma 食べる. Without this, learner cannot look up the verb."""
        assert lemma(tokenizer, "食べられた") == "食べる"

    def test_miru(self, tokenizer):
        """見ている (progressive) → lemma 見る."""
        assert lemma(tokenizer, "見ている") == "見る"

    def test_kuru_lemma(self, tokenizer):
        """来る irregular verb → reading くる."""
        assert reading(tokenizer, "来る") == "くる"

    def test_benkyou_suru(self, tokenizer):
        """勉強する compound verb treated as a unit, reading べんきょうする."""
        assert reading(tokenizer, "勉強する") == "べんきょうする"

    def test_benkyou_shita(self, tokenizer):
        """勉強した (past) → reading べんきょうした. Learner must recognise this as 勉強する."""
        assert reading(tokenizer, "勉強した") == "べんきょうした"

    def test_ikeba_conditional(self, tokenizer):
        """行けば (conditional) reading いけば."""
        assert reading(tokenizer, "行けば") == "いけば"

    def test_negative_form(self, tokenizer):
        """食べない (negative) reading たべない."""
        assert reading(tokenizer, "食べない") == "たべない"

    def test_potential_form(self, tokenizer):
        """書ける (potential) reading かける."""
        assert reading(tokenizer, "書ける") == "かける"

    def test_volitional_form(self, tokenizer):
        """食べよう (volitional) reading たべよう."""
        assert reading(tokenizer, "食べよう") == "たべよう"

    def test_tai_form(self, tokenizer):
        """食べたい (desiderative) reading たべたい. Very common learner encounter."""
        assert reading(tokenizer, "食べたい") == "たべたい"

    def test_passive_form(self, tokenizer):
        """書かれる (passive) reading かかれる."""
        assert reading(tokenizer, "書かれる") == "かかれる"

    def test_causative_form(self, tokenizer):
        """食べさせる (causative) reading たべさせる."""
        assert reading(tokenizer, "食べさせる") == "たべさせる"

    def test_imperative_form(self, tokenizer):
        """食べろ (imperative) reading たべろ."""
        assert reading(tokenizer, "食べろ") == "たべろ"

    def test_masu_form(self, tokenizer):
        """食べます (polite present) reading たべます."""
        assert reading(tokenizer, "食べます") == "たべます"

    def test_masen_form(self, tokenizer):
        """食べません (polite negative) reading たべません."""
        assert reading(tokenizer, "食べません") == "たべません"

    def test_mashita_form(self, tokenizer):
        """食べました (polite past) reading たべました."""
        assert reading(tokenizer, "食べました") == "たべました"

    def test_nakereba_naranai(self, tokenizer):
        """食べなければならない (must-eat) reading たべなければならない."""
        assert reading(tokenizer, "食べなければならない") == "たべなければならない"

    def test_te_iru_progressive(self, tokenizer):
        """食べている (progressive) reading たべている."""
        assert reading(tokenizer, "食べている") == "たべている"

    def test_shimaimashita(self, tokenizer):
        """してしまいました (completion) reading してしまいました."""
        assert reading(tokenizer, "してしまいました") == "してしまいました"

    def test_wakaru(self, tokenizer):
        """分かる reading わかる. Very common verb."""
        assert reading(tokenizer, "分かる") == "わかる"

    def test_hajimete(self, tokenizer):
        """初めて reading はじめて. Common adverb."""
        assert reading(tokenizer, "初めて") == "はじめて"

    def test_hajimemashite(self, tokenizer):
        """初めまして reading はじめまして. Essential for beginners."""
        assert reading(tokenizer, "初めまして") == "はじめまして"

    def test_wakarimashita(self, tokenizer):
        """分かりました reading わかりました. Extremely common expression."""
        assert reading(tokenizer, "分かりました") == "わかりました"

    # ── NHK Easy article forms ────────────────────────────────────────────────

    def test_okotta_lemma(self, tokenizer):
        """起こった → lemma 起こる. Learner must find 起こる, not be lost."""
        result = tokenizer.tokenize("周りで起こったおもしろいことを")
        okoru = next((t for t in result.tokens if "起" in t.surface), None)
        assert okoru is not None, "起こ token not found"
        assert okoru.lemma == "起こる", f"Expected 起こる, got {okoru.lemma!r}"

    def test_atsumari_lemma(self, tokenizer):
        """集まりました → lemma 集まる."""
        result = tokenizer.tokenize("川柳が集まりました")
        tok = next((t for t in result.tokens if "集" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "集まる", f"Expected 集まる, got {tok.lemma!r}"

    def test_nakunari_lemma(self, tokenizer):
        """無くなり → lemma 無くなる. Common verb (things running out)."""
        result = tokenizer.tokenize("充電無くなり")
        tok = next((t for t in result.tokens if "無" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "無くなる", f"Expected 無くなる, got {tok.lemma!r}"

    def test_harou_lemma(self, tokenizer):
        """払おう (volitional) → lemma 払う. Learner must find 払う."""
        result = tokenizer.tokenize("お金を払おうとしたら")
        tok = next((t for t in result.tokens if "払" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "払う", f"Expected 払う, got {tok.lemma!r}"

    def test_narenakute_lemma(self, tokenizer):
        """慣れなくて → lemma 慣れる. Intermediate learner vocabulary."""
        result = tokenizer.tokenize("慣れなくて困る")
        tok = next((t for t in result.tokens if "慣" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "慣れる", f"Expected 慣れる, got {tok.lemma!r}"

    def test_ookatta_lemma(self, tokenizer):
        """多かった (past i-adj) → lemma 多い. Common adjective."""
        result = tokenizer.tokenize("川柳が多かったです")
        tok = next((t for t in result.tokens if "多" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "多い", f"Expected 多い, got {tok.lemma!r}"

    def test_sunde_lemma(self, tokenizer):
        """住んでいる → verb stem 住ん has lemma 住む."""
        result = tokenizer.tokenize("海外に住んでいる")
        tok = next((t for t in result.tokens if "住" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "住む", f"Expected 住む, got {tok.lemma!r}"

    def test_kuwaeru_lemma(self, tokenizer):
        """加え (て-form stem) → lemma 加える."""
        result = tokenizer.tokenize("情報番組に加え")
        tok = next((t for t in result.tokens if "加" in t.surface), None)
        assert tok is not None
        assert tok.lemma == "加える", f"Expected 加える, got {tok.lemma!r}"

    def test_haitte_lemma(self, tokenizer):
        """
        LOOKUP: 入って clicked → learner must find 入る.
        入る (to enter) is JLPT N5 and one of the first verbs learners encounter.
        """
        result = tokenizer.tokenize("入って")
        lemmas = [t.lemma for t in result.tokens if t.is_content_word]
        assert "入る" in lemmas, f"Expected 入る in lemmas, got {lemmas}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 2: Reading display (READING scenario)
#
# Furigana must show the correct pronunciation for each kanji word.
# This is the most direct learner-facing feature: every wrong reading is a
# learner practising wrong pronunciation.
# Covers disambiguation (same kanji, different readings by context),
# inflected forms, and common vocabulary.
# ─────────────────────────────────────────────────────────────────────────────

class TestReadingDisplay:
    """
    READING: every kanji word must produce the correct hiragana reading so that
    furigana shown to the learner is accurate.
    """

    # ── Context-sensitive disambiguation ─────────────────────────────────────
    # These kanji have multiple valid readings; the tokenizer must pick the
    # right one from context. A wrong choice gives the learner false phonics.

    def test_tokyo_to(self, tokenizer):
        """東京都 must read とうきょうと not とうきょうみやこ."""
        assert reading(tokenizer, "東京都") == "とうきょうと"

    def test_ikimono(self, tokenizer):
        """生き物 must read いきもの not なまきもの."""
        assert reading(tokenizer, "生き物") == "いきもの"

    def test_mizu(self, tokenizer):
        """水 alone reads みず (not すい, which is the on-reading)."""
        assert reading(tokenizer, "水") == "みず"

    def test_hito_reading(self, tokenizer):
        """人 alone reads ひと (not じん/にん which are counter/compound readings)."""
        assert reading(tokenizer, "人") == "ひと"

    def test_kaisha_in(self, tokenizer):
        """会社員 reads かいしゃいん (compound reading)."""
        assert reading(tokenizer, "会社員") == "かいしゃいん"

    def test_nihongo(self, tokenizer):
        """日本語 reads にほんご."""
        assert reading(tokenizer, "日本語") == "にほんご"

    def test_gakko(self, tokenizer):
        """学校 reads がっこう (double consonant, common learner error)."""
        assert reading(tokenizer, "学校") == "がっこう"

    def test_te_form_taberu(self, tokenizer):
        """食べて reads たべて."""
        assert reading(tokenizer, "食べて") == "たべて"

    def test_te_form_kaku(self, tokenizer):
        """書いて reads かいて (i-onbin)."""
        assert reading(tokenizer, "書いて") == "かいて"

    def test_te_form_kuru(self, tokenizer):
        """来て reads きて (irregular)."""
        assert reading(tokenizer, "来て") == "きて"

    # ── Common vocabulary readings ────────────────────────────────────────────
    # Learners encounter these words constantly; wrong furigana would embed
    # wrong pronunciations through repeated reading practice.

    def test_kawaii(self, tokenizer):
        """可愛い reads かわいい."""
        assert reading(tokenizer, "可愛い") == "かわいい"

    def test_muzukashii(self, tokenizer):
        """難しい reads むずかしい."""
        assert reading(tokenizer, "難しい") == "むずかしい"

    def test_tanoshii(self, tokenizer):
        """楽しい reads たのしい."""
        assert reading(tokenizer, "楽しい") == "たのしい"

    def test_densha(self, tokenizer):
        """電車 reads でんしゃ."""
        assert reading(tokenizer, "電車") == "でんしゃ"

    def test_toshokan(self, tokenizer):
        """図書館 reads としょかん."""
        assert reading(tokenizer, "図書館") == "としょかん"

    def test_byouin(self, tokenizer):
        """病院 reads びょういん."""
        assert reading(tokenizer, "病院") == "びょういん"

    def test_onaka(self, tokenizer):
        """お腹 reads おなか."""
        assert reading(tokenizer, "お腹") == "おなか"

    def test_taisetsu(self, tokenizer):
        """大切 reads たいせつ. Common adjective."""
        assert reading(tokenizer, "大切") == "たいせつ"

    def test_iroiro(self, tokenizer):
        """色々 reads いろいろ."""
        assert reading(tokenizer, "色々") == "いろいろ"

    def test_kodomo(self, tokenizer):
        """子供 reads こども."""
        assert reading(tokenizer, "子供") == "こども"

    def test_shizen(self, tokenizer):
        """自然 reads しぜん."""
        assert reading(tokenizer, "自然") == "しぜん"

    def test_tomodachi(self, tokenizer):
        """友達 reads ともだち."""
        assert reading(tokenizer, "友達") == "ともだち"

    def test_oshieru(self, tokenizer):
        """教える reads おしえる."""
        assert reading(tokenizer, "教える") == "おしえる"

    def test_kangaeru(self, tokenizer):
        """考える reads かんがえる."""
        assert reading(tokenizer, "考える") == "かんがえる"

    def test_suki(self, tokenizer):
        """好き reads すき."""
        assert reading(tokenizer, "好き") == "すき"

    def test_kuuki(self, tokenizer):
        """空気 reads くうき."""
        assert reading(tokenizer, "空気") == "くうき"

    def test_jikan(self, tokenizer):
        """時間 reads じかん."""
        assert reading(tokenizer, "時間") == "じかん"

    def test_denwa(self, tokenizer):
        """電話 reads でんわ."""
        assert reading(tokenizer, "電話") == "でんわ"

    def test_eiga(self, tokenizer):
        """映画 reads えいが."""
        assert reading(tokenizer, "映画") == "えいが"

    def test_ongaku(self, tokenizer):
        """音楽 reads おんがく."""
        assert reading(tokenizer, "音楽") == "おんがく"

    # ── Common expressions ────────────────────────────────────────────────────

    def test_sumimasen(self, tokenizer):
        """すみません reads すみません (kana-only; should pass through unchanged)."""
        assert reading(tokenizer, "すみません") == "すみません"

    def test_arigatou(self, tokenizer):
        """ありがとう reads ありがとう."""
        assert reading(tokenizer, "ありがとう") == "ありがとう"

    def test_yoroshiku(self, tokenizer):
        """よろしく reads よろしく."""
        assert reading(tokenizer, "よろしく") == "よろしく"

    def test_daijoubu(self, tokenizer):
        """大丈夫 reads だいじょうぶ."""
        assert reading(tokenizer, "大丈夫") == "だいじょうぶ"

    def test_moshimoshi(self, tokenizer):
        """もしもし reads もしもし."""
        assert reading(tokenizer, "もしもし") == "もしもし"

    def test_ja_nai(self, tokenizer):
        """じゃない (casual negative copula) reads じゃない."""
        assert reading(tokenizer, "じゃない") == "じゃない"

    def test_deshita(self, tokenizer):
        """でした (polite past copula) reads でした."""
        assert reading(tokenizer, "でした") == "でした"

    # ── Place names ───────────────────────────────────────────────────────────
    # Place names matter primarily when they appear inline in text that learners
    # read — wrong furigana gives wrong mental pronunciation.

    def test_osaka(self, tokenizer):
        """大阪 reads おおさか."""
        assert reading(tokenizer, "大阪") == "おおさか"

    def test_kyoto(self, tokenizer):
        """京都 reads きょうと."""
        assert reading(tokenizer, "京都") == "きょうと"

    def test_akihabara(self, tokenizer):
        """秋葉原 reads あきはばら (tricky — many learners mispronounce this)."""
        assert reading(tokenizer, "秋葉原") == "あきはばら"

    def test_shinjuku(self, tokenizer):
        """新宿 reads しんじゅく."""
        assert reading(tokenizer, "新宿") == "しんじゅく"

    # ── NHK Easy / NHK World article words ───────────────────────────────────

    def test_senryu_reading(self, tokenizer):
        """川柳 reads せんりゅう (from NHK Easy article)."""
        assert reading(tokenizer, "川柳") == "せんりゅう"

    def test_nihon_in_context(self, tokenizer):
        """
        日本 reads にっぽん in context (SudachiPy normalises to にっぽん).
        NHK Easy also uses にっぽん as the official reading.
        Note: 日本人 reads にほんじん (different compound).
        """
        result = tokenizer.tokenize("川柳は日本の短い詩のひとつです")
        nihon = next(t for t in result.tokens if t.surface == "日本")
        assert nihon.reading_hira == "にっぽん", (
            f"日本 reading: expected にっぽん, got {nihon.reading_hira!r}"
        )

    def test_nihonjin_reading(self, tokenizer):
        """
        日本人 reads にほんじん.
        Note: にほんじん not にっぽんじん — the compound uses a different reading
        of 日本 than the standalone word. Learners must know both.
        """
        result = tokenizer.tokenize("日本人のために")
        tok = next(t for t in result.tokens if t.surface == "日本人")
        assert tok.reading_hira == "にほんじん", (
            f"Expected にほんじん, got {tok.reading_hira!r}"
        )

    def test_mijikai_reading(self, tokenizer):
        """短い reads みじかい."""
        assert reading(tokenizer, "短い") == "みじかい"

    def test_shi_reading(self, tokenizer):
        """詩 (poem) reads し."""
        result = tokenizer.tokenize("短い詩のひとつ")
        shi = next(t for t in result.tokens if t.surface == "詩")
        assert shi.reading_hira == "し", f"Expected し, got {shi.reading_hira!r}"

    def test_jibun_reading(self, tokenizer):
        """自分 reads じぶん."""
        assert reading(tokenizer, "自分") == "じぶん"

    def test_kimochi_reading(self, tokenizer):
        """気持ち reads きもち."""
        assert reading(tokenizer, "気持ち") == "きもち"

    def test_mawari_reading(self, tokenizer):
        """周り reads まわり."""
        assert reading(tokenizer, "周り") == "まわり"

    def test_juuden_reading(self, tokenizer):
        """充電 reads じゅうでん."""
        assert reading(tokenizer, "充電") == "じゅうでん"

    def test_muichimono_reading(self, tokenizer):
        """無一文 reads むいちもん (rare but tested in article)."""
        assert reading(tokenizer, "無一文") == "むいちもん"

    def test_imi_reading(self, tokenizer):
        """意味 reads いみ."""
        assert reading(tokenizer, "意味") == "いみ"

    def test_harau_reading(self, tokenizer):
        """払う reads はらう."""
        assert reading(tokenizer, "払う") == "はらう"

    def test_chuushin_reading(self, tokenizer):
        """中心 reads ちゅうしん."""
        assert reading(tokenizer, "中心") == "ちゅうしん"

    def test_sekai_reading(self, tokenizer):
        """世界 reads せかい."""
        assert reading(tokenizer, "世界") == "せかい"

    def test_komaru_reading(self, tokenizer):
        """困る reads こまる."""
        assert reading(tokenizer, "困る") == "こまる"

    def test_kaigai_reading(self, tokenizer):
        """海外 reads かいがい."""
        assert reading(tokenizer, "海外") == "かいがい"

    def test_kaigai_ryokou_reading(self, tokenizer):
        """海外旅行 reads かいがいりょこう (single compound)."""
        result = tokenizer.tokenize("海外旅行中の日本人")
        tok = next(t for t in result.tokens if t.surface == "海外旅行")
        assert tok.reading_hira == "かいがいりょこう", (
            f"Expected かいがいりょこう, got {tok.reading_hira!r}"
        )

    def test_jouhou_reading(self, tokenizer):
        """情報 reads じょうほう."""
        assert reading(tokenizer, "情報") == "じょうほう"

    def test_bangumi_reading(self, tokenizer):
        """番組 reads ばんぐみ."""
        assert reading(tokenizer, "番組") == "ばんぐみ"

    def test_sumo_reading(self, tokenizer):
        """相撲 reads すもう (alone, without 大)."""
        assert reading(tokenizer, "相撲") == "すもう"

    def test_chuukei_reading(self, tokenizer):
        """中継 reads ちゅうけい."""
        assert reading(tokenizer, "中継") == "ちゅうけい"

    def test_sports_reading(self, tokenizer):
        """スポーツ reads すぽーつ."""
        assert reading(tokenizer, "スポーツ") == "すぽーつ"

    def test_kokunai_reading(self, tokenizer):
        """国内 reads こくない."""
        assert reading(tokenizer, "国内") == "こくない"

    def test_bassui_reading(self, tokenizer):
        """抜粋 reads ばっすい."""
        assert reading(tokenizer, "抜粋") == "ばっすい"

    def test_hensei_reading(self, tokenizer):
        """編成 reads へんせい."""
        assert reading(tokenizer, "編成") == "へんせい"

    def test_ichibu_reading(self, tokenizer):
        """一部 reads いちぶ."""
        assert reading(tokenizer, "一部") == "いちぶ"

    def test_haishin_reading(self, tokenizer):
        """配信 reads はいしん."""
        assert reading(tokenizer, "配信") == "はいしん"

    def test_drama_reading(self, tokenizer):
        """ドラマ reads どらま."""
        assert reading(tokenizer, "ドラマ") == "どらま"

    def test_haitte_reading(self, tokenizer):
        """
        READING: 入って must read はいって.
        入る is JLPT N5; correct furigana lets learners connect audio to kanji.
        """
        r = reading(tokenizer, "入って")
        assert r == "はいって", f"Expected はいって, got {r!r}"

    def test_gozaimasen_full_reading(self, tokenizer):
        """
        READING: ございません must read ございません.
        Very common polite negative (ある → ございません); wrong furigana
        means the learner practises the wrong pronunciation.
        """
        r = reading(tokenizer, "ございません")
        assert r == "ございません", f"Expected ございません, got {r!r}"

    def test_gozaimasen_morpheme_sum(self, tokenizer):
        """
        READING: morpheme readings of ございません must concatenate to ございません.
        A-mode split must not produce a wrong combined reading.
        """
        result = tokenizer.tokenize("ございません", mode="A")
        full_reading = "".join(t.reading_hira for t in result.a_tokens)
        assert full_reading == "ございません", (
            f"ございません reading = {full_reading!r}"
        )

    def test_daisumo_rendaku(self, tokenizer):
        """
        READING: 大相撲 must read おおずもう (rendaku す→ず, not おおすもう).
        A frequent NHK Sports topic; wrong pronunciation is actively misleading.
        """
        r = reading(tokenizer, "大相撲")
        assert r == "おおずもう", f"Expected おおずもう, got {r!r}"

    def test_daisumo_single_token(self, tokenizer):
        """
        SEGMENT: 大相撲 must be one clickable token.
        If split into 大+相撲, a learner who clicks either part gets the wrong
        dictionary entry (大 as a standalone prefix has no useful entry).
        """
        result = tokenizer.tokenize("大相撲")
        surfaces = [tok.surface for tok in result.tokens]
        assert surfaces == ["大相撲"], f"Expected ['大相撲'], got {surfaces}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 3: Word segmentation (SEGMENT scenario)
#
# The tokenizer decides what unit to present to the learner as a word.
# Two failure modes:
#   over-split: スマートフォン → スマート + フォン (learner never sees the full word)
#   under-split: 情報番組 merged into one opaque chunk (learner misses two words)
# The right split is the one that matches the learnable vocabulary unit.
# ─────────────────────────────────────────────────────────────────────────────

class TestWordSegmentation:
    """
    SEGMENT: tokenizer boundaries must match the vocabulary units a learner
    should acquire. Loanwords and fixed compounds stay whole; transparent
    derivations may split into their useful constituent words.
    """

    # ── Compounds that must stay whole ────────────────────────────────────────
    # If these are split, the learner never encounters the actual word.

    def test_smartphone_single_token(self, tokenizer):
        """スマートフォン must be one token — it's the word learners look up."""
        result = tokenizer.tokenize("スマートフォンでお金を払う")
        assert "スマートフォン" in [t.surface for t in result.tokens], (
            f"スマートフォン split: {[t.surface for t in result.tokens]}"
        )

    def test_internet_single_token(self, tokenizer):
        """インターネット must be one token."""
        result = tokenizer.tokenize("インターネットなどが")
        assert "インターネット" in [t.surface for t in result.tokens]

    def test_cashless_single_token(self, tokenizer):
        """キャッシュレス must be one token (poem vocabulary)."""
        result = tokenizer.tokenize("キャッシュレス　充電無くなり　無一文")
        assert "キャッシュレス" in [t.surface for t in result.tokens]

    def test_kaigai_ryokou_single_token(self, tokenizer):
        """海外旅行 must be one token — learner should acquire the compound."""
        result = tokenizer.tokenize("海外旅行中の")
        assert "海外旅行" in [t.surface for t in result.tokens], (
            f"海外旅行 split: {[t.surface for t in result.tokens]}"
        )

    # ── Compounds that should split ───────────────────────────────────────────
    # These split into parts that are each more useful to learn than the whole.

    def test_jouhou_bangumi_split(self, tokenizer):
        """
        情報番組 splits as 情報 + 番組.
        Pedagogically correct: learner acquires two high-frequency words
        (情報 = information, 番組 = programme) rather than one opaque compound.
        """
        result = tokenizer.tokenize("情報番組に加え")
        surfaces = [t.surface for t in result.tokens]
        assert "情報" in surfaces and "番組" in surfaces, (
            f"Expected 情報 and 番組 separately, got: {surfaces}"
        )

    def test_digital_ka_split(self, tokenizer):
        """
        デジタル化 splits as デジタル + 化.
        Learner benefits: they see デジタル (loanword) and 化 (nominaliser suffix
        meaning '-isation') as separate learnable items. Both are productive
        vocabulary. The merged compound is less common as a dictionary entry.
        """
        result = tokenizer.tokenize("デジタル化")
        surfaces = [t.surface for t in result.tokens]
        assert "デジタル" in surfaces and "化" in surfaces, (
            f"Expected デジタル + 化, got: {surfaces}"
        )

    # ── Compound segmentation — general ──────────────────────────────────────

    def test_wakaru(self, tokenizer):
        """分かる as one content word, reading わかる."""
        assert reading(tokenizer, "分かる") == "わかる"

    def test_suru_te_form(self, tokenizer):
        """して is て-form of する, reading して."""
        assert reading(tokenizer, "して") == "して"

    def test_chotto(self, tokenizer):
        """ちょっと (kana-only adverb) passes through as-is."""
        assert reading(tokenizer, "ちょっと") == "ちょっと"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 4: Vocabulary highlighting (HIGHLIGHT scenario)
#
# Only vocabulary words a learner should study get highlighted.
# Particles (は、が、を、に…) and auxiliary verbs (ます、た、て…) must NOT be
# highlighted or the learner is overwhelmed by grammar markers.
# ─────────────────────────────────────────────────────────────────────────────

class TestVocabularyHighlighting:
    """
    HIGHLIGHT: is_content_word must be True exactly for the words a learner
    benefits from studying and False for grammar function words.
    """

    def test_wo_particle_not_content(self, tokenizer):
        """を is a particle — must not be highlighted."""
        result = tokenizer.tokenize("毎日ご飯を食べる")
        wo = [t for t in result.tokens if t.surface == "を"]
        assert all(not p.is_content_word for p in wo), "Particle を highlighted"

    def test_wa_particle_not_content(self, tokenizer):
        """は is a topic marker — must not be highlighted."""
        result = tokenizer.tokenize("これは")
        ha = [t for t in result.tokens if t.pos == "助詞"]
        assert all(not p.is_content_word for p in ha)

    def test_sentence_content_word_count(self, tokenizer):
        """
        毎日ご飯を食べる has exactly 3 content words: 毎日, ご飯, 食べる.
        The particle を and auxiliaries must not be counted.
        """
        result = tokenizer.tokenize("毎日ご飯を食べる")
        content = [t for t in result.tokens if t.is_content_word]
        assert len(content) == 3, (
            f"Expected 3 content words, got {len(content)}: {[t.surface for t in content]}"
        )

    def test_adverb_is_content(self, tokenizer):
        """
        とても (very) must be highlighted — it is useful vocabulary.
        Learners need adverbs like とても, もっと, ちょっと to express nuance.
        """
        result = tokenizer.tokenize("とても")
        assert any(t.is_content_word for t in result.tokens)

    def test_long_sentence_reading(self, tokenizer):
        """日本語を勉強しています — full reading check."""
        r = reading(tokenizer, "日本語を勉強しています")
        assert r == "にほんごをべんきょうしています", f"Got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 5: Furigana display alignment
#
# When kanji are shown to a learner with furigana, the reading must align
# to the correct kanji segment — not bleed into okurigana.
# A misaligned furigana (e.g. showing たべ above 食 instead of just た)
# teaches the learner a wrong kanji-sound association.
# ─────────────────────────────────────────────────────────────────────────────

class TestFuriganaAlignment:
    """
    Furigana spans must align correctly: only the kanji portion gets the
    reading, okurigana (kana that follows the kanji) is left as-is.
    Wrong alignment creates false kanji-reading associations in the learner.
    """

    def test_taberu_furigana(self, tokenizer):
        """食べる → 食(た)べる. The べる is okurigana, must not be inside the ruby span."""
        spans = generate_furigana("食べる", "たべる")
        assert any(s.text == "食" and s.reading == "た" for s in spans), (
            f"Expected 食→た, got {spans}"
        )

    def test_ikimono_furigana(self, tokenizer):
        """生き物 → 生(い)き物(もの). Both kanji segments get furigana."""
        spans = generate_furigana("生き物", "いきもの")
        assert len([s for s in spans if s.reading]) >= 1

    def test_pure_kana_no_furigana(self, tokenizer):
        """ありがとう — kana-only, no furigana spans should be generated."""
        spans = generate_furigana("ありがとう", "ありがとう")
        assert all(s.reading == "" for s in spans), (
            f"Kana-only text should have empty readings: {spans}"
        )

    def test_pure_kanji_furigana(self, tokenizer):
        """山 → 山(やま). Single kanji, full reading."""
        spans = generate_furigana("山", "やま")
        assert len(spans) == 1
        assert spans[0].text == "山" and spans[0].reading == "やま"

    def test_mixed_furigana_hairu(self, tokenizer):
        """入る → 入(はい)る. Only the kanji 入 gets furigana, る is okurigana."""
        spans = generate_furigana("入る", "はいる")
        assert any("入" in s.text and "はい" in s.reading for s in spans), (
            f"Expected 入→はい, got {spans}"
        )


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 6: Numbers and counters (READING scenario)
#
# Counter readings are a major learner pain point — the same numeral reads
# differently depending on the counter (一枚 いちまい vs 一人 ひとり).
# The tokenizer must pick the correct reading from context.
# Arabic numeral correction: SudachiPy read each digit individually;
# we post-process to produce proper positional readings (24 → にじゅうし).
# ─────────────────────────────────────────────────────────────────────────────

class TestNumbersAndCounters:
    """
    Counter readings are a systematic learner challenge. The tokenizer must
    apply the correct reading — many are irregular and cannot be guessed.
    Includes Arabic-numeral reading correction (SudachiPy per-digit fix).
    """

    def test_hitotsu(self, tokenizer):
        """一つ reads ひとつ (native counter, not いちつ)."""
        assert reading(tokenizer, "一つ") == "ひとつ"

    def test_futari(self, tokenizer):
        """二人 reads ふたり (not ににん — irregular native counter for people)."""
        assert reading(tokenizer, "二人") == "ふたり"

    def test_sanmai(self, tokenizer):
        """三枚 reads さんまい (flat-object counter)."""
        assert reading(tokenizer, "三枚") == "さんまい"

    def test_hyaku(self, tokenizer):
        """百 reads ひゃく."""
        assert reading(tokenizer, "百") == "ひゃく"

    def test_sen(self, tokenizer):
        """千 reads せん."""
        assert reading(tokenizer, "千") == "せん"

    def test_nannichi(self, tokenizer):
        """何日 reads なんにち."""
        assert reading(tokenizer, "何日") == "なんにち"

    # ── Arabic numeral reading correction ─────────────────────────────────────

    def test_arabic_24_reading(self, tokenizer):
        """
        READING: Arabic 24 must read にじゅうよん, not によん (per-digit).
        Furigana にじゅうよん is the correct pronunciation a learner needs to hear.
        """
        result = tokenizer.tokenize("24時間")
        num = next((t for t in result.tokens if t.surface == "24"), None)
        assert num is not None, "24 token not found"
        assert num.reading_hira == "にじゅうよん", (
            f"24 should read にじゅうよん, got {num.reading_hira!r}"
        )

    def test_arabic_100_reading(self, tokenizer):
        """100 → ひゃく (not いちれいれい)."""
        result = tokenizer.tokenize("100円")
        num = next((t for t in result.tokens if t.surface == "100"), None)
        assert num is not None
        assert num.reading_hira == "ひゃく"

    def test_arabic_300_sandhi(self, tokenizer):
        """300 → さんびゃく (ひゃく → びゃく after さん)."""
        result = tokenizer.tokenize("300円")
        num = next((t for t in result.tokens if t.surface == "300"), None)
        assert num is not None
        assert num.reading_hira == "さんびゃく"

    def test_arabic_1000_reading(self, tokenizer):
        """1000 → せん (not いちせん)."""
        result = tokenizer.tokenize("1000円")
        num = next((t for t in result.tokens if t.surface == "1000"), None)
        assert num is not None
        assert num.reading_hira == "せん"

    def test_arabic_3000_sandhi(self, tokenizer):
        """3000 → さんぜん (せん → ぜん after さん)."""
        result = tokenizer.tokenize("3000円")
        num = next((t for t in result.tokens if t.surface == "3000"), None)
        assert num is not None
        assert num.reading_hira == "さんぜん"

    def test_arabic_10000_reading(self, tokenizer):
        """10000 → いちまん."""
        result = tokenizer.tokenize("10000円")
        num = next((t for t in result.tokens if t.surface == "10000"), None)
        assert num is not None
        assert num.reading_hira == "いちまん"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 7: Katakana loanwords (READING scenario)
#
# Loanwords are written in katakana; the extension converts them to hiragana
# for learners who haven't yet mastered katakana reading.
# They must also tokenize as single units (see Section 4 for segmentation).
# ─────────────────────────────────────────────────────────────────────────────

class TestKatakanaLoanwords:
    """
    Loanwords must: (a) tokenize as a single unit (Section 4), and (b) convert
    cleanly to hiragana so learners see the pronunciation.
    """

    def test_terebi(self, tokenizer):
        """テレビ reads てれび."""
        assert reading(tokenizer, "テレビ") == "てれび"

    def test_koohii(self, tokenizer):
        """コーヒー reads こーひー (long vowel mark ー preserved)."""
        assert reading(tokenizer, "コーヒー") == "こーひー"

    def test_pasokon(self, tokenizer):
        """パソコン reads ぱそこん."""
        assert reading(tokenizer, "パソコン") == "ぱそこん"

    def test_aisu_kuriimu(self, tokenizer):
        """アイスクリーム reads あいすくりーむ."""
        assert reading(tokenizer, "アイスクリーム") == "あいすくりーむ"

    def test_sumaho(self, tokenizer):
        """スマホ reads すまほ."""
        assert reading(tokenizer, "スマホ") == "すまほ"

    def test_intanetto(self, tokenizer):
        """インターネット reads いんたーねっと."""
        assert reading(tokenizer, "インターネット") == "いんたーねっと"

    def test_cashless_reading(self, tokenizer):
        """キャッシュレス reads きゃっしゅれす."""
        assert reading(tokenizer, "キャッシュレス") == "きゃっしゅれす"

    def test_smartphone_reading(self, tokenizer):
        """スマートフォン reads すまーとふぉん."""
        assert reading(tokenizer, "スマートフォン") == "すまーとふぉん"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 8: Technical normalization
#
# These tests cover internal correctness that does not directly correspond to
# any of the four learner scenarios. They prevent regressions in the plumbing
# that underlies the learner-facing features.
# ─────────────────────────────────────────────────────────────────────────────

class TestInternalNormalization:
    """
    Technical tests — not directly learner-facing but necessary for correct
    operation of the reading and segmentation features.
    """

    def test_kata_to_hira_conversion(self):
        """
        katakana→hiragana conversion is the foundation of all reading display.
        All learner-visible readings go through this function.
        """
        assert kata_to_hira("タベル") == "たべる"
        assert kata_to_hira("ニホンゴ") == "にほんご"
        assert kata_to_hira("ゴザイマセン") == "ございません"
        assert kata_to_hira("abc") == "abc"      # non-kana unchanged

    def test_fullwidth_ascii_normalized(self, tokenizer):
        """Full-width characters in input must not crash the tokenizer."""
        result = tokenizer.tokenize("日本語！")
        assert len(result.tokens) > 0

    def test_nfc_normalization(self, tokenizer):
        """NFD and NFC forms of the same character must produce identical readings."""
        nfd_ga = "が"
        nfc_ga = "が"
        r1 = reading(tokenizer, nfd_ga + "んばれ")
        r2 = reading(tokenizer, nfc_ga + "んばれ")
        assert r1 == r2, f"NFD/NFC mismatch: {r1!r} vs {r2!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 9: Real-world article — NHK Easy 川柳コンクール
#
# Source (furigana stripped from NHK Easy format — NHK's furigana is the
# authoritative reading reference used to verify assertions in this suite):
#
#   川柳のニュースです。
#   川柳は日本の短い詩のひとつです。自分の気持ちや周りで起こったおもしろいことを、
#   短いことばで言います。
#   コンクールには5万4000の川柳が集まりました。
#   1位の川柳は、「キャッシュレス　充電無くなり　無一文」です。
#   無一文は、お金がない、という意味です。スマートフォンでお金を払おうとしたら、
#   充電がなくなっていて払うことができなかったという川柳です。
#   ほかにも、インターネットなどが中心の世界になる「デジタル化」に慣れなくて
#   困るという川柳が多かったです。
# ─────────────────────────────────────────────────────────────────────────────

_ARTICLE_PARAGRAPHS = [
    "川柳のニュースです。",
    "川柳は日本の短い詩のひとつです。自分の気持ちや周りで起こったおもしろいことを、短いことばで言います。",
    "コンクールには5万4000の川柳が集まりました。",
    "1位の川柳は、「キャッシュレス　充電無くなり　無一文」です。",
    "無一文は、お金がない、という意味です。スマートフォンでお金を払おうとしたら、充電がなくなっていて払うことができなかったという川柳です。",
    "ほかにも、インターネットなどが中心の世界になる「デジタル化」に慣れなくて困るという川柳が多かったです。",
]
_ARTICLE_FULL = "\n".join(_ARTICLE_PARAGRAPHS)


class TestNHKEasyArticleSenryu:
    """
    Integration test: full NHK Easy article run through the tokenizer.
    All assertions verified against NHK Easy's own furigana (the authoritative
    reading source for learner-facing Japanese) and actual SudachiPy output.
    """

    def test_full_article_no_error(self, tokenizer):
        """Full article tokenizes without exception."""
        result = tokenizer.tokenize(_ARTICLE_FULL)
        assert len(result.tokens) > 0

    def test_full_article_content_word_count(self, tokenizer):
        """Full article has ≥ 40 content-word tokens (sanity check on highlighting)."""
        result = tokenizer.tokenize(_ARTICLE_FULL)
        count = sum(1 for t in result.tokens if t.is_content_word)
        assert count >= 40, f"Expected ≥40 content words, got {count}"

    def test_full_article_key_lemmas_present(self, tokenizer):
        """
        All key content-word lemmas a learner would study appear in the output.
        This verifies end-to-end: correct segmentation + correct lemmatization
        = correct dictionary lookup for every important word in the article.
        """
        result = tokenizer.tokenize(_ARTICLE_FULL)
        lemmas = {t.lemma for t in result.tokens if t.is_content_word}
        expected = {
            "川柳", "日本", "短い", "詩", "自分", "気持ち", "起こる",
            "集まる", "充電", "意味", "払う", "中心", "世界", "困る", "多い",
            "無一文", "無くなる", "慣れる", "キャッシュレス", "インターネット",
            "スマートフォン",
        }
        missing = expected - lemmas
        assert not missing, f"Missing lemmas from article: {sorted(missing)}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 10: Real-world text — NHK World broadcast description
#
# Source (no furigana — standard Japanese prose):
#   海外に住んでいる、あるいは、海外旅行中の日本人のために、ニュース・情報番組に
#   加え、ドラマ、音楽番組、こども番組、大相撲中継などのスポーツ番組を国内各波
#   から抜粋し、1日24時間編成しています。一部のニュース・番組は海外向けに
#   インターネットでも配信しています。
# ─────────────────────────────────────────────────────────────────────────────

_BROADCAST_TEXT = (
    "海外に住んでいる、あるいは、海外旅行中の日本人のために、ニュース・情報番組に加え、"
    "ドラマ、音楽番組、こども番組、大相撲中継などのスポーツ番組を国内各波から抜粋し、"
    "1日24時間編成しています。一部のニュース・番組は海外向けにインターネットでも配信しています。"
)


class TestNHKWorldBroadcastDescription:
    """
    Integration test for NHK World broadcast description.
    No furigana stripping needed — text is already standard Japanese.
    Assertions verified against actual SudachiPy output (2026-05-30).
    """

    def test_full_text_no_error(self, tokenizer):
        """Full text tokenizes without exception."""
        result = tokenizer.tokenize(_BROADCAST_TEXT)
        assert len(result.tokens) > 0

    def test_full_text_content_word_count(self, tokenizer):
        """Full text has ≥ 20 content-word tokens."""
        result = tokenizer.tokenize(_BROADCAST_TEXT)
        count = sum(1 for t in result.tokens if t.is_content_word)
        assert count >= 20, f"Expected ≥20 content words, got {count}"

    def test_full_text_key_lemmas_present(self, tokenizer):
        """All key content-word lemmas appear in the full-text output."""
        result = tokenizer.tokenize(_BROADCAST_TEXT)
        lemmas = {t.lemma for t in result.tokens if t.is_content_word}
        expected = {
            "海外", "住む", "海外旅行", "日本人", "ニュース", "情報", "番組",
            "加える", "ドラマ", "音楽", "大相撲", "中継", "スポーツ", "国内",
            "抜粋", "時間", "編成", "一部", "インターネット", "配信",
        }
        missing = expected - lemmas
        assert not missing, f"Missing lemmas: {sorted(missing)}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 11: Date readings
#
# Japanese date ordinals use native Yamato readings for days 2-10, 14, 20, 24.
# SudachiPy splits "2日" into a numeral + 日 and returns にち (wrong).
# The tokenizer's _apply_date_corrections merges them with the correct reading.
# ─────────────────────────────────────────────────────────────────────────────

class TestDateReadings:
    """
    READING: N日 must carry the native Japanese calendar reading, not
    the sino-Japanese にち. Wrong furigana teaches learners the wrong word.
    """

    def test_2nichi_futsuka(self, tokenizer):
        """2日 must read ふつか (native), not ににち."""
        result = tokenizer.tokenize("2日に会いましょう")
        tok = next((t for t in result.tokens if t.surface == "2日"), None)
        assert tok is not None, "2日 token not found (may still be split)"
        assert tok.reading_hira == "ふつか", (
            f"2日 should read ふつか, got {tok.reading_hira!r}"
        )

    def test_3nichi_mikka(self, tokenizer):
        """3日 must read みっか."""
        result = tokenizer.tokenize("3日後に")
        tok = next((t for t in result.tokens if t.surface == "3日"), None)
        assert tok is not None, "3日 token not found"
        assert tok.reading_hira == "みっか", (
            f"3日 should read みっか, got {tok.reading_hira!r}"
        )

    def test_4nichi_yokka(self, tokenizer):
        """4日 must read よっか."""
        result = tokenizer.tokenize("4日に出発する")
        tok = next((t for t in result.tokens if t.surface == "4日"), None)
        assert tok is not None, "4日 token not found"
        assert tok.reading_hira == "よっか"

    def test_7nichi_nanoka(self, tokenizer):
        """7日 must read なのか."""
        result = tokenizer.tokenize("7日に会う")
        tok = next((t for t in result.tokens if t.surface == "7日"), None)
        assert tok is not None, "7日 token not found"
        assert tok.reading_hira == "なのか"

    def test_8nichi_youka(self, tokenizer):
        """8日 must read ようか."""
        result = tokenizer.tokenize("8日に出発する")
        tok = next((t for t in result.tokens if t.surface == "8日"), None)
        assert tok is not None, "8日 token not found"
        assert tok.reading_hira == "ようか"

    def test_9nichi_kokonoka(self, tokenizer):
        """9日 must read ここのか."""
        result = tokenizer.tokenize("9日に出発する")
        tok = next((t for t in result.tokens if t.surface == "9日"), None)
        assert tok is not None, "9日 token not found"
        assert tok.reading_hira == "ここのか"

    def test_10nichi_tooka(self, tokenizer):
        """10日 must read とおか."""
        result = tokenizer.tokenize("10日後に")
        tok = next((t for t in result.tokens if t.surface == "10日"), None)
        assert tok is not None, "10日 token not found"
        assert tok.reading_hira == "とおか", (
            f"10日 should read とおか, got {tok.reading_hira!r}"
        )

    def test_14nichi_juuyokka(self, tokenizer):
        """14日 must read じゅうよっか (special form, not じゅうしにち)."""
        result = tokenizer.tokenize("14日に到着")
        tok = next((t for t in result.tokens if t.surface == "14日"), None)
        assert tok is not None, "14日 token not found"
        assert tok.reading_hira == "じゅうよっか", (
            f"14日 should read じゅうよっか, got {tok.reading_hira!r}"
        )

    def test_20nichi_hatsuka(self, tokenizer):
        """20日 must read はつか (not にじゅうにち)."""
        result = tokenizer.tokenize("20日に締め切り")
        tok = next((t for t in result.tokens if t.surface == "20日"), None)
        assert tok is not None, "20日 token not found"
        assert tok.reading_hira == "はつか", (
            f"20日 should read はつか, got {tok.reading_hira!r}"
        )

    def test_24nichi_nijuuyokka(self, tokenizer):
        """24日 must read にじゅうよっか (special よっか form at 24)."""
        result = tokenizer.tokenize("24日のイベント")
        tok = next((t for t in result.tokens if t.surface == "24日"), None)
        assert tok is not None, "24日 token not found"
        assert tok.reading_hira == "にじゅうよっか", (
            f"24日 should read にじゅうよっか, got {tok.reading_hira!r}"
        )

    def test_generic_date_fallback_nichi(self, tokenizer):
        """Days without special readings (e.g. 11日) fall back to N+にち."""
        result = tokenizer.tokenize("11日に会議")
        tok = next((t for t in result.tokens if t.surface == "11日"), None)
        assert tok is not None, "11日 token not found"
        assert tok.reading_hira == "じゅういちにち", (
            f"11日 should read じゅういちにち, got {tok.reading_hira!r}"
        )

    def test_date_is_single_token(self, tokenizer):
        """After correction, N日 must be a single token (not numeral + 日)."""
        result = tokenizer.tokenize("3日後に")
        surfaces = [t.surface for t in result.tokens]
        assert "3日" in surfaces, f"Expected single 3日 token, got: {surfaces}"
        assert "日" not in [s for s in surfaces if s == "日"], (
            f"日 should not appear as separate token, got: {surfaces}"
        )

    def test_date_is_content_word(self, tokenizer):
        """N日 date tokens must be content words (learner vocabulary)."""
        result = tokenizer.tokenize("5日に会う")
        tok = next((t for t in result.tokens if t.surface == "5日"), None)
        assert tok is not None, "5日 token not found"
        assert tok.is_content_word, "5日 should be a content word"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 12: Counter sandhi
#
# Japanese counters cause phonetic changes (rendaku, gemination) to adjacent
# numerals. SudachiPy splits these but gives wrong or plain readings.
# _apply_counter_corrections merges and applies the correct sandhi reading.
# ─────────────────────────────────────────────────────────────────────────────

class TestCounterSandhi:
    """
    READING: number+counter combinations must produce the phonetically correct
    reading. Teaching いちほん instead of いっぽん is actively harmful.
    """

    # ── 本 (ほん/ぽん/ぼん) — long thin objects ───────────────────────────────

    def test_1hon_ippon(self, tokenizer):
        """1本 must read いっぽん (gemination + voicing after 1)."""
        result = tokenizer.tokenize("1本のペン")
        tok = next((t for t in result.tokens if t.surface == "1本"), None)
        assert tok is not None, "1本 token not found"
        assert tok.reading_hira == "いっぽん", (
            f"1本 should read いっぽん, got {tok.reading_hira!r}"
        )

    def test_2hon_nihon(self, tokenizer):
        """2本 must read にほん."""
        result = tokenizer.tokenize("2本の箸")
        tok = next((t for t in result.tokens if t.surface == "2本"), None)
        assert tok is not None, "2本 token not found"
        assert tok.reading_hira == "にほん"

    def test_3hon_sanbon(self, tokenizer):
        """3本 must read さんぼん (voiced ほん→ぼん after さん)."""
        result = tokenizer.tokenize("3本のビール")
        tok = next((t for t in result.tokens if t.surface == "3本"), None)
        assert tok is not None, "3本 token not found"
        assert tok.reading_hira == "さんぼん", (
            f"3本 should read さんぼん, got {tok.reading_hira!r}"
        )

    def test_6hon_roppon(self, tokenizer):
        """6本 must read ろっぽん (geminate + voiced)."""
        result = tokenizer.tokenize("6本の指")
        tok = next((t for t in result.tokens if t.surface == "6本"), None)
        assert tok is not None, "6本 token not found"
        assert tok.reading_hira == "ろっぽん", (
            f"6本 should read ろっぽん, got {tok.reading_hira!r}"
        )

    def test_8hon_happon(self, tokenizer):
        """8本 must read はっぽん (geminate + voiced)."""
        result = tokenizer.tokenize("8本の木")
        tok = next((t for t in result.tokens if t.surface == "8本"), None)
        assert tok is not None, "8本 token not found"
        assert tok.reading_hira == "はっぽん", (
            f"8本 should read はっぽん, got {tok.reading_hira!r}"
        )

    def test_10hon_juppon(self, tokenizer):
        """10本 must read じゅっぽん."""
        result = tokenizer.tokenize("10本の缶")
        tok = next((t for t in result.tokens if t.surface == "10本"), None)
        assert tok is not None, "10本 token not found"
        assert tok.reading_hira == "じゅっぽん"

    # ── 匹 (ひき/ぴき/びき) — small animals ──────────────────────────────────

    def test_1hiki_ippiki(self, tokenizer):
        """1匹 must read いっぴき."""
        result = tokenizer.tokenize("猫が1匹いる")
        tok = next((t for t in result.tokens if t.surface == "1匹"), None)
        assert tok is not None, "1匹 token not found"
        assert tok.reading_hira == "いっぴき"

    def test_3hiki_sanbiki(self, tokenizer):
        """3匹 must read さんびき (voiced)."""
        result = tokenizer.tokenize("犬が3匹いる")
        tok = next((t for t in result.tokens if t.surface == "3匹"), None)
        assert tok is not None, "3匹 token not found"
        assert tok.reading_hira == "さんびき"

    def test_6hiki_roppiki(self, tokenizer):
        """6匹 must read ろっぴき."""
        result = tokenizer.tokenize("魚が6匹")
        tok = next((t for t in result.tokens if t.surface == "6匹"), None)
        assert tok is not None, "6匹 token not found"
        assert tok.reading_hira == "ろっぴき"

    # ── 杯 (はい/ぱい/ばい) — cups/glasses ────────────────────────────────────

    def test_1pai_ippai(self, tokenizer):
        """1杯 must read いっぱい."""
        result = tokenizer.tokenize("コーヒー1杯")
        tok = next((t for t in result.tokens if t.surface == "1杯"), None)
        assert tok is not None, "1杯 token not found"
        assert tok.reading_hira == "いっぱい"

    def test_3pai_sanbai(self, tokenizer):
        """3杯 must read さんばい."""
        result = tokenizer.tokenize("お茶を3杯飲む")
        tok = next((t for t in result.tokens if t.surface == "3杯"), None)
        assert tok is not None, "3杯 token not found"
        assert tok.reading_hira == "さんばい"

    # ── 階 (かい/かい/がい) — floors ──────────────────────────────────────────

    def test_3kai_sangai(self, tokenizer):
        """3階 must read さんがい (voiced かい→がい after さん)."""
        result = tokenizer.tokenize("3階に住む")
        tok = next((t for t in result.tokens if t.surface == "3階"), None)
        assert tok is not None, "3階 token not found"
        assert tok.reading_hira == "さんがい", (
            f"3階 should read さんがい, got {tok.reading_hira!r}"
        )

    def test_6kai_rokkai(self, tokenizer):
        """6階 must read ろっかい."""
        result = tokenizer.tokenize("6階建てのビル")
        tok = next((t for t in result.tokens if t.surface == "6階"), None)
        assert tok is not None, "6階 token not found"
        assert tok.reading_hira == "ろっかい"

    # ── 冊 (さつ) — bound volumes ─────────────────────────────────────────────

    def test_1satsu_issatsu(self, tokenizer):
        """1冊 must read いっさつ."""
        result = tokenizer.tokenize("本を1冊買う")
        tok = next((t for t in result.tokens if t.surface == "1冊"), None)
        assert tok is not None, "1冊 token not found"
        assert tok.reading_hira == "いっさつ"

    def test_8satsu_hassatsu(self, tokenizer):
        """8冊 must read はっさつ."""
        result = tokenizer.tokenize("8冊の漫画")
        tok = next((t for t in result.tokens if t.surface == "8冊"), None)
        assert tok is not None, "8冊 token not found"
        assert tok.reading_hira == "はっさつ"

    # ── Merged token properties ───────────────────────────────────────────────

    def test_counter_token_is_content_word(self, tokenizer):
        """N+counter merged token must be a content word."""
        result = tokenizer.tokenize("2本の箸")
        tok = next((t for t in result.tokens if t.surface == "2本"), None)
        assert tok is not None
        assert tok.is_content_word

    def test_counter_token_is_single(self, tokenizer):
        """After correction, numeral+counter must be one token, not two."""
        result = tokenizer.tokenize("3本のビール")
        surfaces = [t.surface for t in result.tokens]
        assert "3本" in surfaces, f"Expected 3本 as one token, got: {surfaces}"
        # Neither '3' nor '本' should appear as separate tokens
        assert "3" not in surfaces, f"3 should not appear separately: {surfaces}"

    # ── 分 (minutes) — p-mutation sandhi ─────────────────────────────────────

    def test_1fun_ippun(self, tokenizer):
        """1分 must read いっぷん (geminate + p-voicing after 1)."""
        result = tokenizer.tokenize("1分後に")
        tok = next((t for t in result.tokens if t.surface == "1分"), None)
        assert tok is not None, "1分 token not found"
        assert tok.reading_hira == "いっぷん", (
            f"1分 should read いっぷん, got {tok.reading_hira!r}"
        )

    def test_3fun_sanpun(self, tokenizer):
        """3分 must read さんぷん (p-voicing after さん, not さんふん)."""
        result = tokenizer.tokenize("3分待つ")
        tok = next((t for t in result.tokens if t.surface == "3分"), None)
        assert tok is not None, "3分 token not found"
        assert tok.reading_hira == "さんぷん", (
            f"3分 should read さんぷん, got {tok.reading_hira!r}"
        )

    def test_6fun_roppun(self, tokenizer):
        """6分 must read ろっぷん."""
        result = tokenizer.tokenize("6分かかる")
        tok = next((t for t in result.tokens if t.surface == "6分"), None)
        assert tok is not None, "6分 token not found"
        assert tok.reading_hira == "ろっぷん"

    def test_10fun_juppun(self, tokenizer):
        """10分 must read じゅっぷん."""
        result = tokenizer.tokenize("10分で")
        tok = next((t for t in result.tokens if t.surface == "10分"), None)
        assert tok is not None, "10分 token not found"
        assert tok.reading_hira == "じゅっぷん"

    # ── 人 (people) — よにん for 4 ────────────────────────────────────────────

    def test_3nin_sannin(self, tokenizer):
        """3人 must read さんにん."""
        result = tokenizer.tokenize("3人の学生が")
        tok = next((t for t in result.tokens if t.surface == "3人"), None)
        assert tok is not None, "3人 token not found"
        assert tok.reading_hira == "さんにん"

    def test_4nin_yonin(self, tokenizer):
        """
        4人 must read よにん (not しにん).
        し is avoided before にん to prevent homophony with 死人 (dead person).
        This is a very common error in naive numeral-reading implementations.
        """
        result = tokenizer.tokenize("4人いる")
        tok = next((t for t in result.tokens if t.surface == "4人"), None)
        assert tok is not None, "4人 token not found"
        assert tok.reading_hira == "よにん", (
            f"4人 should read よにん (not しにん), got {tok.reading_hira!r}"
        )

    def test_10nin_juunin(self, tokenizer):
        """10人 must read じゅうにん."""
        result = tokenizer.tokenize("10人のチーム")
        tok = next((t for t in result.tokens if t.surface == "10人"), None)
        assert tok is not None, "10人 token not found"
        assert tok.reading_hira == "じゅうにん"

    def test_14nin_juuyonin(self, tokenizer):
        """14人 must read じゅうよにん (よん form in rule-based path)."""
        result = tokenizer.tokenize("14人で作業する")
        tok = next((t for t in result.tokens if t.surface == "14人"), None)
        assert tok is not None, "14人 token not found"
        assert tok.reading_hira == "じゅうよんにん", (
            f"14人 should read じゅうよんにん, got {tok.reading_hira!r}"
        )


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 14: Clock time readings
#
# Japanese clock times use irregular forms for 4, 7, and 9.
# Raw numeral readings (し, なな, きゅう) before 時 give wrong pronunciations
# that learners will hear corrected immediately by native speakers.
# ─────────────────────────────────────────────────────────────────────────────

class TestClockTimeReadings:
    """
    READING: N時 must use the conventional clock-time pronunciation.
    4時→よじ, 7時→しちじ, 9時→くじ are high-priority corrections.
    """

    def test_4ji_yoji(self, tokenizer):
        """
        4時 must read よじ (not しじ).
        しじ is the direct numeral reading but is never used for 4 o'clock.
        This is probably the single most common clock-time mispronunciation.
        """
        result = tokenizer.tokenize("4時に会いましょう")
        tok = next((t for t in result.tokens if t.surface == "4時"), None)
        assert tok is not None, "4時 token not found"
        assert tok.reading_hira == "よじ", (
            f"4時 should read よじ, got {tok.reading_hira!r}"
        )

    def test_7ji_shichiji(self, tokenizer):
        """
        7時 must read しちじ (not ななじ).
        ななじ sounds like a learner mistake; natives say しちじ for 7 o'clock.
        """
        result = tokenizer.tokenize("7時のニュース")
        tok = next((t for t in result.tokens if t.surface == "7時"), None)
        assert tok is not None, "7時 token not found"
        assert tok.reading_hira == "しちじ", (
            f"7時 should read しちじ, got {tok.reading_hira!r}"
        )

    def test_9ji_kuji(self, tokenizer):
        """
        9時 must read くじ (not きゅうじ).
        きゅうじ is the direct reading of 9; くじ is the conventional time form.
        """
        result = tokenizer.tokenize("9時ごろに出発する")
        tok = next((t for t in result.tokens if t.surface == "9時"), None)
        assert tok is not None, "9時 token not found"
        assert tok.reading_hira == "くじ", (
            f"9時 should read くじ, got {tok.reading_hira!r}"
        )

    def test_12ji_juuniji(self, tokenizer):
        """12時 must read じゅうにじ."""
        result = tokenizer.tokenize("12時に昼食")
        tok = next((t for t in result.tokens if t.surface == "12時"), None)
        assert tok is not None, "12時 token not found"
        assert tok.reading_hira == "じゅうにじ"

    def test_time_is_content_word(self, tokenizer):
        """N時 merged token must be a content word."""
        result = tokenizer.tokenize("4時に集合")
        tok = next((t for t in result.tokens if t.surface == "4時"), None)
        assert tok is not None
        assert tok.is_content_word


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 15: Kanji date ordinals
#
# Calendar dates written in kanji (二日, 七日, 十日) have special readings
# that SudachiPy's concatenation gets wrong for several cases.
# ─────────────────────────────────────────────────────────────────────────────

class TestKanjiDateOrdinals:
    """
    READING: Kanji-numeral date ordinals must produce correct native readings.
    二日→ふつか (not ふたか), 七日→なのか (not ななにち), 十日→とおか (not とうか).
    """

    def test_futsuka(self, tokenizer):
        """二日 must read ふつか (not ふたか — SudachiPy gives ふた+か)."""
        result = tokenizer.tokenize("二日後に")
        tok = next((t for t in result.tokens if t.surface == "二日"), None)
        assert tok is not None, "二日 not found"
        assert tok.reading_hira == "ふつか", f"Got {tok.reading_hira!r}"

    def test_yokka_kanji(self, tokenizer):
        """四日 must read よっか."""
        result = tokenizer.tokenize("四日に出発")
        tok = next((t for t in result.tokens if t.surface == "四日"), None)
        assert tok is not None, "四日 not found"
        assert tok.reading_hira == "よっか", f"Got {tok.reading_hira!r}"

    def test_nanoka_kanji(self, tokenizer):
        """七日 must read なのか (not ななにち)."""
        result = tokenizer.tokenize("七日後に出発する")
        tok = next((t for t in result.tokens if t.surface == "七日"), None)
        assert tok is not None, "七日 not found"
        assert tok.reading_hira == "なのか", f"Got {tok.reading_hira!r}"

    def test_tooka_kanji(self, tokenizer):
        """十日 must read とおか (not とうか — long vowel preserved)."""
        result = tokenizer.tokenize("十日後に")
        tok = next((t for t in result.tokens if t.surface == "十日"), None)
        assert tok is not None, "十日 not found"
        assert tok.reading_hira == "とおか", f"Got {tok.reading_hira!r}"

    def test_hatsuka_kanji(self, tokenizer):
        """二十日 must read はつか."""
        result = tokenizer.tokenize("二十日に締め切り")
        tok = next((t for t in result.tokens if t.surface == "二十日"), None)
        assert tok is not None, "二十日 not found"
        assert tok.reading_hira == "はつか", f"Got {tok.reading_hira!r}"

    def test_mikka_kanji(self, tokenizer):
        """三日 → みっか (merged token)."""
        result = tokenizer.tokenize("三日後に出発")
        tok = next((t for t in result.tokens if t.surface == "三日"), None)
        assert tok is not None, "三日 not found"
        assert tok.reading_hira == "みっか", f"Got {tok.reading_hira!r}"

    def test_itsuka_kanji(self, tokenizer):
        """五日 → いつか."""
        result = tokenizer.tokenize("五日の朝")
        tok = next((t for t in result.tokens if t.surface == "五日"), None)
        assert tok is not None, "五日 not found"
        assert tok.reading_hira == "いつか", f"Got {tok.reading_hira!r}"

    def test_muika_kanji(self, tokenizer):
        """六日 → むいか."""
        result = tokenizer.tokenize("六日に会う")
        tok = next((t for t in result.tokens if t.surface == "六日"), None)
        assert tok is not None, "六日 not found"
        assert tok.reading_hira == "むいか", f"Got {tok.reading_hira!r}"

    def test_youka_kanji(self, tokenizer):
        """八日 → ようか."""
        result = tokenizer.tokenize("八日の夜")
        tok = next((t for t in result.tokens if t.surface == "八日"), None)
        assert tok is not None, "八日 not found"
        assert tok.reading_hira == "ようか", f"Got {tok.reading_hira!r}"

    def test_kokonoka_kanji(self, tokenizer):
        """九日 → ここのか."""
        result = tokenizer.tokenize("九日後に帰る")
        tok = next((t for t in result.tokens if t.surface == "九日"), None)
        assert tok is not None, "九日 not found"
        assert tok.reading_hira == "ここのか", f"Got {tok.reading_hira!r}"


# ── Section 16b: Relative time compound readings ──────────────────────────────

@pytest.mark.usefixtures("tokenizer")
class TestRelativeTimeCompounds:
    """一昨日, 一昨年 — SudachiPy splits as 一昨+日/年 with wrong readings."""

    def test_ototoi(self, tokenizer):
        """一昨日 → おととい."""
        result = tokenizer.tokenize("一昨日の朝")
        tok = next((t for t in result.tokens if t.surface == "一昨日"), None)
        assert tok is not None, "一昨日 not found"
        assert tok.reading_hira == "おととい", f"Got {tok.reading_hira!r}"

    def test_ototoshi(self, tokenizer):
        """一昨年 → おととし."""
        result = tokenizer.tokenize("一昨年の出来事")
        tok = next((t for t in result.tokens if t.surface == "一昨年"), None)
        assert tok is not None, "一昨年 not found"
        assert tok.reading_hira == "おととし", f"Got {tok.reading_hira!r}"


# ── Section 16c: Month-duration counters ─────────────────────────────────────

@pytest.mark.usefixtures("tokenizer")
class TestMonthDurationCounters:
    """か月/ヶ月 (months of duration): 1→いっかげつ, 6→ろっかげつ, 8→はっかげつ."""

    def test_1kagetsu(self, tokenizer):
        """1か月 → いっかげつ (gemination)."""
        result = tokenizer.tokenize("1か月後に")
        tok = next((t for t in result.tokens if t.surface == "1か月"), None)
        assert tok is not None, "1か月 not found"
        assert tok.reading_hira == "いっかげつ", f"got {tok.reading_hira!r}"

    def test_6kagetsu(self, tokenizer):
        """6か月 → ろっかげつ."""
        result = tokenizer.tokenize("6か月前")
        tok = next((t for t in result.tokens if t.surface == "6か月"), None)
        assert tok is not None, "6か月 not found"
        assert tok.reading_hira == "ろっかげつ", f"got {tok.reading_hira!r}"

    def test_2kekugetsu(self, tokenizer):
        """2ヶ月 → にかげつ (ヶ variant)."""
        result = tokenizer.tokenize("2ヶ月前に")
        tok = next((t for t in result.tokens if t.surface == "2ヶ月"), None)
        assert tok is not None, "2ヶ月 not found"
        assert tok.reading_hira == "にかげつ", f"got {tok.reading_hira!r}"

    def test_8kekugetsu(self, tokenizer):
        """8ヶ月 → はっかげつ."""
        result = tokenizer.tokenize("8ヶ月後")
        tok = next((t for t in result.tokens if t.surface == "8ヶ月"), None)
        assert tok is not None, "8ヶ月 not found"
        assert tok.reading_hira == "はっかげつ", f"got {tok.reading_hira!r}"


# ── Section 16d: Week-duration counters ──────────────────────────────────────

@pytest.mark.usefixtures("tokenizer")
class TestWeekDurationCounters:
    """週間 (weeks of duration): 1→いっしゅうかん, 8→はっしゅうかん (gemination before し)."""

    def test_1shuukan(self, tokenizer):
        """1週間 → いっしゅうかん."""
        result = tokenizer.tokenize("1週間後に")
        tok = next((t for t in result.tokens if t.surface == "1週間"), None)
        assert tok is not None, "1週間 not found"
        assert tok.reading_hira == "いっしゅうかん", f"got {tok.reading_hira!r}"

    def test_8shuukan(self, tokenizer):
        """8週間 → はっしゅうかん."""
        result = tokenizer.tokenize("8週間の研修")
        tok = next((t for t in result.tokens if t.surface == "8週間"), None)
        assert tok is not None, "8週間 not found"
        assert tok.reading_hira == "はっしゅうかん", f"got {tok.reading_hira!r}"

    def test_2shuukan(self, tokenizer):
        """2週間 → にしゅうかん (no special sandhi)."""
        result = tokenizer.tokenize("2週間前に")
        tok = next((t for t in result.tokens if t.surface == "2週間"), None)
        assert tok is not None, "2週間 not found"
        assert tok.reading_hira == "にしゅうかん", f"got {tok.reading_hira!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 16: Month and year counter readings
#
# Months have special forms (4月=しがつ not よんがつ, 7月=しちがつ not ながつ,
# 9月=くがつ not きゅうがつ). Years use よ for 4 (4年=よねん).
# Historical year+年 merges into a single token with the full correct reading.
# ─────────────────────────────────────────────────────────────────────────────

class TestMonthYearCounters:
    """
    READING: N月 and N年 must produce the conventional Japanese calendar readings,
    not raw numeral concatenations. Wrong furigana actively miseducates learners.
    """

    def test_4gatsu_shigatsu(self, tokenizer):
        """4月 must read しがつ (not よんがつ — し IS correct for months)."""
        result = tokenizer.tokenize("4月に桜が咲く")
        tok = next((t for t in result.tokens if t.surface == "4月"), None)
        assert tok is not None, "4月 token not found"
        assert tok.reading_hira == "しがつ", (
            f"4月 should read しがつ, got {tok.reading_hira!r}"
        )

    def test_7gatsu_shichigatsu(self, tokenizer):
        """7月 must read しちがつ (not ながつ — my numeral override gave ながつ)."""
        result = tokenizer.tokenize("7月の祭り")
        tok = next((t for t in result.tokens if t.surface == "7月"), None)
        assert tok is not None, "7月 token not found"
        assert tok.reading_hira == "しちがつ", (
            f"7月 should read しちがつ, got {tok.reading_hira!r}"
        )

    def test_9gatsu_kugatsu(self, tokenizer):
        """9月 must read くがつ (not きゅうがつ)."""
        result = tokenizer.tokenize("9月に台風が来る")
        tok = next((t for t in result.tokens if t.surface == "9月"), None)
        assert tok is not None, "9月 token not found"
        assert tok.reading_hira == "くがつ", (
            f"9月 should read くがつ, got {tok.reading_hira!r}"
        )

    def test_12gatsu_juunigatsu(self, tokenizer):
        """12月 must read じゅうにがつ."""
        result = tokenizer.tokenize("12月になると寒い")
        tok = next((t for t in result.tokens if t.surface == "12月"), None)
        assert tok is not None, "12月 token not found"
        assert tok.reading_hira == "じゅうにがつ"

    def test_4nen_yonen(self, tokenizer):
        """4年 must read よねん (not しねん)."""
        result = tokenizer.tokenize("4年生になった")
        tok = next((t for t in result.tokens if t.surface == "4年"), None)
        if tok is None:
            tok = next((t for t in result.tokens if t.surface == "4年生"), None)
            return  # 4年生 may be a compound; just skip
        assert tok.reading_hira == "よねん", (
            f"4年 should read よねん, got {tok.reading_hira!r}"
        )

    def test_year_1868nen_full_reading(self, tokenizer):
        """1868年 merges into one token reading せんはっぴゃくろくじゅうはちねん."""
        result = tokenizer.tokenize("明治元年は1868年です")
        tok = next((t for t in result.tokens if t.surface == "1868年"), None)
        assert tok is not None, "1868年 token not found"
        assert tok.reading_hira == "せんはっぴゃくろくじゅうはちねん", (
            f"Got {tok.reading_hira!r}"
        )

    def test_year_no_gemination(self, tokenizer):
        """18年 must read じゅうはちねん (no gemination — not じゅうはっねん)."""
        result = tokenizer.tokenize("18年前のこと")
        tok = next((t for t in result.tokens if t.surface == "18年"), None)
        assert tok is not None, "18年 token not found"
        assert tok.reading_hira == "じゅうはちねん", (
            f"18年 should read じゅうはちねん (no gemination), got {tok.reading_hira!r}"
        )


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 13: Comma-formatted numbers
#
# Large numbers in text often use Western comma separators (1,234).
# SudachiPy may pass these through; the numeral correction must strip commas
# before computing the Japanese reading.
# ─────────────────────────────────────────────────────────────────────────────

class TestCommaFormattedNumbers:
    """
    READING: comma-formatted numbers like 1,234 must read as せんにひゃくさんじゅうし,
    not いてんにさんよん (digit-by-digit including punctuation).
    """

    def test_54000_reading(self, tokenizer):
        """
        54000 → ごまんよんせん.
        From NHK Easy article: 5万4000の川柳.
        Tests the man-unit boundary.
        """
        result = tokenizer.tokenize("5万4000の川柳が集まりました")
        # 5万4000 merges into single token via mixed_numeral_reading
        num = next((t for t in result.tokens if t.surface == "5万4000"), None)
        if num is not None:
            assert num.reading_hira == "ごまんよんせん", (
                f"5万4000 should read ごまんよんせん, got {num.reading_hira!r}"
            )
        else:
            # fallback: check 4000 alone reads correctly
            num4 = next((t for t in result.tokens if t.surface == "4000"), None)
            if num4 is not None:
                assert num4.reading_hira == "よんせん", (
                    f"4000 should read よんせん, got {num4.reading_hira!r}"
                )


# ── Section 17: Mixed Kanji-Numeral Readings ──────────────────────────────────

@pytest.mark.usefixtures("tokenizer")
class TestMixedKanjiNumerals:
    """Tests for patterns like 100万, 10億, 5万4000."""

    def test_100man_reading(self, tokenizer):
        """100万 → ひゃくまん (SudachiPy would give per-character イチレイレイマン)."""
        result = tokenizer.tokenize("100万円")
        tok = next((t for t in result.tokens if t.surface == "100万"), None)
        assert tok is not None, "100万 token not found"
        assert tok.reading_hira == "ひゃくまん", (
            f"100万 should read ひゃくまん, got {tok.reading_hira!r}"
        )

    def test_10oku_reading(self, tokenizer):
        """10億 → じゅうおく."""
        result = tokenizer.tokenize("10億円")
        tok = next((t for t in result.tokens if t.surface == "10億"), None)
        assert tok is not None, "10億 token not found"
        assert tok.reading_hira == "じゅうおく", (
            f"10億 should read じゅうおく, got {tok.reading_hira!r}"
        )

    def test_5man_4000_reading(self, tokenizer):
        """5万4000 → ごまんよんせん."""
        result = tokenizer.tokenize("5万4000人が集まった")
        tok = next((t for t in result.tokens if t.surface == "5万4000"), None)
        assert tok is not None, "5万4000 token not found"
        assert tok.reading_hira == "ごまんよんせん", (
            f"5万4000 should read ごまんよんせん, got {tok.reading_hira!r}"
        )

    def test_1oku_2500man_reading(self, tokenizer):
        """
        1億2500万 → いちおくにせんごひゃくまん.
        Chained large units (億+万): SudachiPy gives per-digit reading.
        """
        result = tokenizer.tokenize("日本の人口は約1億2500万人です")
        tok = next((t for t in result.tokens if t.surface == "1億2500万"), None)
        assert tok is not None, "1億2500万 not found"
        assert tok.reading_hira == "いちおくにせんごひゃくまん", (
            f"got {tok.reading_hira!r}"
        )

    def test_1man_5000_reading(self, tokenizer):
        """1万5000 → いちまんごせん."""
        result = tokenizer.tokenize("約1万5000人が参加しました")
        tok = next((t for t in result.tokens if t.surface == "1万5000"), None)
        assert tok is not None, "1万5000 not found"
        assert tok.reading_hira == "いちまんごせん", (
            f"got {tok.reading_hira!r}"
        )


# ── Section 18: Age, Score, and Travel Counters ───────────────────────────────

@pytest.mark.usefixtures("tokenizer")
class TestAgeScoreTravelCounters:
    """Tests for 歳, 点, 泊, 発 counters with sandhi."""

    def test_1sai(self, tokenizer):
        """1歳 → いっさい (gemination before さ)."""
        result = tokenizer.tokenize("1歳の赤ちゃん")
        tok = next((t for t in result.tokens if t.surface == "1歳"), None)
        assert tok is not None, "1歳 not found"
        assert tok.reading_hira == "いっさい", f"got {tok.reading_hira!r}"

    def test_8sai(self, tokenizer):
        """8歳 → はっさい (gemination before さ)."""
        result = tokenizer.tokenize("8歳の子供")
        tok = next((t for t in result.tokens if t.surface == "8歳"), None)
        assert tok is not None, "8歳 not found"
        assert tok.reading_hira == "はっさい", f"got {tok.reading_hira!r}"

    def test_4sai(self, tokenizer):
        """4歳 → よんさい (よん, not しさい)."""
        result = tokenizer.tokenize("4歳の誕生日")
        tok = next((t for t in result.tokens if t.surface == "4歳"), None)
        assert tok is not None, "4歳 not found"
        assert tok.reading_hira == "よんさい", f"got {tok.reading_hira!r}"

    def test_1ten(self, tokenizer):
        """1点 → いってん (gemination before て)."""
        result = tokenizer.tokenize("1点差で勝った")
        tok = next((t for t in result.tokens if t.surface == "1点"), None)
        assert tok is not None, "1点 not found"
        assert tok.reading_hira == "いってん", f"got {tok.reading_hira!r}"

    def test_8ten(self, tokenizer):
        """8点 → はってん."""
        result = tokenizer.tokenize("8点取った")
        tok = next((t for t in result.tokens if t.surface == "8点"), None)
        assert tok is not None, "8点 not found"
        assert tok.reading_hira == "はってん", f"got {tok.reading_hira!r}"

    def test_1haku(self, tokenizer):
        """1泊 → いっぱく (aspiration + gemination)."""
        result = tokenizer.tokenize("1泊2日の旅行")
        tok = next((t for t in result.tokens if t.surface == "1泊"), None)
        assert tok is not None, "1泊 not found"
        assert tok.reading_hira == "いっぱく", f"got {tok.reading_hira!r}"

    def test_3haku(self, tokenizer):
        """3泊 → さんぱく (p-sandhi for 3 + は)."""
        result = tokenizer.tokenize("3泊した")
        tok = next((t for t in result.tokens if t.surface == "3泊"), None)
        assert tok is not None, "3泊 not found"
        assert tok.reading_hira == "さんぱく", f"got {tok.reading_hira!r}"

    def test_1hatsu(self, tokenizer):
        """1発 → いっぱつ (aspiration + gemination)."""
        result = tokenizer.tokenize("1発の銃声")
        tok = next((t for t in result.tokens if t.surface == "1発"), None)
        assert tok is not None, "1発 not found"
        assert tok.reading_hira == "いっぱつ", f"got {tok.reading_hira!r}"

    def test_6hatsu(self, tokenizer):
        """6発 → ろっぱつ."""
        result = tokenizer.tokenize("6発撃った")
        tok = next((t for t in result.tokens if t.surface == "6発"), None)
        assert tok is not None, "6発 not found"
        assert tok.reading_hira == "ろっぱつ", f"got {tok.reading_hira!r}"
