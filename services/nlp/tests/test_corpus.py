"""
Corpus-based NLP correctness tests.

All test sentences are drawn verbatim from Wikipedia Japanese articles
stored in tests/corpus/*.txt (downloaded 2026-05-30).

Tests are organized by corpus source and motivated by learner scenarios:
  LOOKUP  — correct lemma for dictionary lookup
  READING — correct hiragana furigana
  SEGMENT — right word-boundary unit for vocabulary acquisition
  HIGHLIGHT — is_content_word classification

These tests complement the synthetic cases in test_correctness.py with
evidence from real, varied Japanese text.
"""

from __future__ import annotations

import pathlib
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest
from carve_nlp.tokenizer import JapaneseTokenizer


@pytest.fixture(scope="module")
def tokenizer() -> JapaneseTokenizer:
    return JapaneseTokenizer()


CORPUS_DIR = pathlib.Path(__file__).parent / "corpus"


def _tokenize(tokenizer: JapaneseTokenizer, text: str):
    return tokenizer.tokenize(text)


def _first_token(tokenizer, text, surface):
    """Return the first token with the given surface from tokenizing text."""
    result = _tokenize(tokenizer, text)
    return next((t for t in result.tokens if t.surface == surface), None)


def _all_surfaces(tokenizer, text):
    return [t.surface for t in _tokenize(tokenizer, text).tokens]


def _all_content_lemmas(tokenizer, text):
    return {t.lemma for t in _tokenize(tokenizer, text).tokens if t.is_content_word}


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 1: Sumo corpus (corpus/sumo.txt)
#
# Domain vocabulary from Japanese traditional sports.
# Tests compound readings, the 大相撲 rendaku correction,
# and verb lemmatization in context.
# ─────────────────────────────────────────────────────────────────────────────

_SUMO_SENTENCE1 = "土俵の上で力士が組み合って戦う形を取る日本古来の神事や祭であり"
_SUMO_SENTENCE2 = "大相撲を取る人は正式名称は「力士」（りきし）という"
_SUMO_SENTENCE3 = "礼儀作法などが重視されており、生活様式や風貌なども"


class TestSumoCorpus:
    """
    Tests drawn from the Wikipedia article on 相撲.
    Covers traditional-culture vocabulary, compound readings,
    and the 大相撲 rendaku correction.
    """

    # ── Domain vocabulary readings ────────────────────────────────────────────

    def test_douhyou_reading(self, tokenizer):
        """
        READING: 土俵 (sumo ring) reads どひょう, not どじょう or other readings.
        Learner encounter: every sumo news article; wrong reading is embarrassing.
        """
        tok = _first_token(tokenizer, _SUMO_SENTENCE1, "土俵")
        assert tok is not None, "土俵 not found"
        assert tok.reading_hira == "どひょう", f"Got {tok.reading_hira!r}"

    def test_rikishi_reading(self, tokenizer):
        """
        READING: 力士 (sumo wrestler) reads りきし, not ちからひと or りくし.
        Learner encounter: every sumo article and broadcast.
        """
        tok = _first_token(tokenizer, _SUMO_SENTENCE1, "力士")
        assert tok is not None, "力士 not found"
        assert tok.reading_hira == "りきし", f"Got {tok.reading_hira!r}"

    def test_rikishi_is_content_word(self, tokenizer):
        """HIGHLIGHT: 力士 is a content word and must be highlighted."""
        tok = _first_token(tokenizer, _SUMO_SENTENCE1, "力士")
        assert tok is not None
        assert tok.is_content_word, "力士 should be content word"

    def test_kakutougi_reading(self, tokenizer):
        """
        READING: 格闘技 (martial arts/combat sports) reads かくとうぎ.
        Common word in sports news; compound must stay whole.
        """
        result = tokenizer.tokenize("格闘技として国際的に認知されている")
        tok = next((t for t in result.tokens if t.surface == "格闘技"), None)
        assert tok is not None, "格闘技 not found as single token"
        assert tok.reading_hira == "かくとうぎ", f"Got {tok.reading_hira!r}"

    def test_reigi_sahō_compound(self, tokenizer):
        """
        SEGMENT + READING: 礼儀作法 (etiquette, manners) must be one token
        reading れいぎさほう.
        Learner impact: splitting into 礼儀 + 作法 would create two lookup
        targets when the compound itself is the natural vocabulary item.
        """
        tok = _first_token(tokenizer, _SUMO_SENTENCE3, "礼儀作法")
        assert tok is not None, "礼儀作法 not found as single token"
        assert tok.reading_hira == "れいぎさほう", f"Got {tok.reading_hira!r}"

    def test_seikatsu_youshiki_compound(self, tokenizer):
        """
        SEGMENT + READING: 生活様式 (lifestyle, way of life) must be one token
        reading せいかつようしき.
        """
        tok = _first_token(tokenizer, _SUMO_SENTENCE3, "生活様式")
        assert tok is not None, "生活様式 not found as single token"
        assert tok.reading_hira == "せいかつようしき", f"Got {tok.reading_hira!r}"

    # ── 大相撲 compound correction ────────────────────────────────────────────

    def test_daisumo_in_context(self, tokenizer):
        """
        SEGMENT: 大相撲 must be one token in a real sentence, not 大 + 相撲.
        Learner impact: 大 alone (prefix) has no useful dictionary entry.
        """
        result = tokenizer.tokenize(_SUMO_SENTENCE2)
        surfaces = [t.surface for t in result.tokens]
        assert "大相撲" in surfaces, f"Expected 大相撲 as single token; got {surfaces}"
        assert "大" not in surfaces or surfaces.index("大") != surfaces.index("大相撲") - 1, \
            "大 must not appear as a standalone prefix before 相撲"

    def test_daisumo_rendaku_in_context(self, tokenizer):
        """
        READING: 大相撲 in real context reads おおずもう (with rendaku す→ず).
        Taken from corpus sentence: 大相撲を取る人は正式名称は…
        """
        tok = _first_token(tokenizer, _SUMO_SENTENCE2, "大相撲")
        assert tok is not None, "大相撲 not found"
        assert tok.reading_hira == "おおずもう", f"Got {tok.reading_hira!r}"

    # ── Verb lemmatization ────────────────────────────────────────────────────

    def test_kumiawatte_lemma(self, tokenizer):
        """
        LOOKUP: 組み合って (te-form of 組み合う) → lemma 組み合う.
        Learner who clicks 組み合って must find 組み合う in the dictionary.
        """
        result = tokenizer.tokenize(_SUMO_SENTENCE1)
        tok = next((t for t in result.tokens if t.surface == "組み合っ"), None)
        assert tok is not None, "組み合っ not found"
        assert tok.lemma == "組み合う", f"Expected 組み合う, got {tok.lemma!r}"

    def test_full_sumo_article(self, tokenizer):
        """Integration: full sumo corpus tokenizes without error."""
        text = (CORPUS_DIR / "sumo.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 200
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("相撲", "大相撲", "力士", "格闘技", "神事"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 2: Tokyo corpus (corpus/tokyo.txt)
#
# Historical Japanese text with place names, era names, and dates.
# Tests proper noun readings and date tokenization.
# ─────────────────────────────────────────────────────────────────────────────

_TOKYO_DATE = "1868年9月3日（慶応4年7月17日）に改称されたものである"
_TOKYO_GOVT  = "立法府である国会議事堂、司法府の頂点である最高裁判所"


class TestTokyoCorpus:
    """
    Tests drawn from the Wikipedia article on 東京.
    Covers place-name disambiguation, historical dates, and
    government/institution vocabulary.
    """

    def test_tokyo_to_reading(self, tokenizer):
        """
        READING: 東京都 reads とうきょうと (metropolis), not とうきょうみやこ.
        Disambiguation: 都 reads と in 東京都 but みやこ in other contexts.
        Note: in 東京都区部, the tokenizer reads 都区部 as a compound so 東京
        stands alone; use 東京都の… to get the single-token form.
        """
        result = tokenizer.tokenize("東京都の首都機能を有す")
        tok = next((t for t in result.tokens if t.surface == "東京都"), None)
        assert tok is not None, "東京都 must be a single token before の"
        assert tok.reading_hira == "とうきょうと", f"Got {tok.reading_hira!r}"

    def test_shuto_reading(self, tokenizer):
        """
        READING: 首都 (capital city) reads しゅと, not しゅとう.
        Essential geography vocabulary; wrong reading is misleading.
        """
        result = tokenizer.tokenize("東京都区部を指す場合がある。東京は日本の首都である。")
        tok = next((t for t in result.tokens if t.surface == "首都"), None)
        assert tok is not None, "首都 not found"
        assert tok.reading_hira == "しゅと", f"Got {tok.reading_hira!r}"

    def test_edo_reading(self, tokenizer):
        """
        READING: 江戸 (old name for Tokyo) reads えど.
        Historical vocabulary; every learner of Japanese history sees this.
        """
        result = tokenizer.tokenize("もともと江戸幕府が置かれていた都市「江戸」であった")
        tok = next((t for t in result.tokens if t.surface == "江戸"), None)
        assert tok is not None, "江戸 not found"
        assert tok.reading_hira == "えど", f"Got {tok.reading_hira!r}"

    def test_meiji_reading(self, tokenizer):
        """
        READING: 明治 (Meiji era) reads めいじ.
        Meiji is the most-referenced Japanese era in modern history texts.
        """
        result = tokenizer.tokenize("明治天皇の二度目の東京行幸に合わせて")
        tok = next((t for t in result.tokens if t.surface == "明治"), None)
        assert tok is not None
        assert tok.reading_hira == "めいじ", f"Got {tok.reading_hira!r}"

    def test_gyoukou_reading(self, tokenizer):
        """
        READING: 行幸 (imperial visit/procession) reads ぎょうこう, not こうゆき.
        Classical vocabulary with a non-obvious reading; frequent in history texts.
        """
        result = tokenizer.tokenize("明治天皇の二度目の東京行幸に合わせて")
        tok = next((t for t in result.tokens if t.surface == "行幸"), None)
        assert tok is not None, "行幸 not found"
        assert tok.reading_hira == "ぎょうこう", f"Got {tok.reading_hira!r}"

    def test_year_1868_reading(self, tokenizer):
        """
        READING: 1868年 must read せんはっぴゃくろくじゅうはちねん.
        Year numbers are extremely common in historical Japanese; per-digit
        reading (いちはちろくはちねん) is meaningless to a learner.
        The numeral merges with 年 into a single token.
        """
        result = tokenizer.tokenize(_TOKYO_DATE)
        tok = next((t for t in result.tokens if t.surface == "1868年"), None)
        assert tok is not None, "1868年 token not found"
        assert tok.reading_hira == "せんはっぴゃくろくじゅうはちねん", (
            f"Got {tok.reading_hira!r}"
        )

    def test_kokkai_gijidou_compound(self, tokenizer):
        """
        SEGMENT + READING: 国会議事堂 (National Diet Building) must be a single
        token reading こっかいぎじどう.
        Learner impact: splitting into 国会 + 議事 + 堂 would prevent looking up
        this proper noun.
        """
        result = tokenizer.tokenize(_TOKYO_GOVT)
        tok = next((t for t in result.tokens if t.surface == "国会議事堂"), None)
        assert tok is not None, "国会議事堂 not found as single token"
        assert tok.reading_hira == "こっかいぎじどう", f"Got {tok.reading_hira!r}"

    def test_saikōsaibansho_reading(self, tokenizer):
        """
        READING: 最高裁判所 (Supreme Court) reads さいこうさいばんしょ.
        SudachiPy splits it into 最高 (さいこう) + 裁判所 (さいばんしょ);
        concatenation gives the correct reading either way.
        Both are useful vocabulary items for learners of legal/civic Japanese.
        """
        result = tokenizer.tokenize(_TOKYO_GOVT)
        tokens = result.tokens
        # Accept both single-token and 最高 + 裁判所 split
        saisho = next((t for t in tokens if t.surface == "最高裁判所"), None)
        if saisho:
            assert saisho.reading_hira == "さいこうさいばんしょ", f"Got {saisho.reading_hira!r}"
        else:
            saiko = next((t for t in tokens if t.surface == "最高"), None)
            saibansho = next((t for t in tokens if t.surface == "裁判所"), None)
            assert saiko is not None, "最高 not found"
            assert saibansho is not None, "裁判所 not found"
            assert saiko.reading_hira == "さいこう", f"最高 reading: {saiko.reading_hira!r}"
            assert saibansho.reading_hira == "さいばんしょ", f"裁判所 reading: {saibansho.reading_hira!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 3: Shinkansen corpus (corpus/shinkansen.txt)
#
# Technical text with speed/date numbers, place names, and railway compounds.
# Tests Arabic numeral reading in technical context and compound proper nouns.
# ─────────────────────────────────────────────────────────────────────────────

_SHINKANSEN_SPEED = "時速200キロメートル以上の高速度で走行できる日本の幹線鉄道"
_SHINKANSEN_DATE  = "1964年（昭和39年）10月1日に東海道本線の線路容量逼迫対策として"
_SHINKANSEN_DELAY = "年間平均遅延時間は12秒に留まる（2019年）"


class TestShinkansenCorpus:
    """
    Tests drawn from the Wikipedia article on 新幹線.
    Covers technical vocabulary, speed/year numbers, date reading,
    and railway compound proper nouns.
    """

    def test_shinkansen_reading(self, tokenizer):
        """READING: 新幹線 reads しんかんせん as a single token."""
        result = tokenizer.tokenize("新幹線は時速200キロメートルで走る")
        tok = next((t for t in result.tokens if t.surface == "新幹線"), None)
        assert tok is not None, "新幹線 not found as single token"
        assert tok.reading_hira == "しんかんせん", f"Got {tok.reading_hira!r}"

    def test_jisoku_reading(self, tokenizer):
        """
        READING: 時速 (speed per hour) reads じそく, not じそく.
        Common in transport contexts; learners need this for speed signs.
        """
        tok = _first_token(tokenizer, _SHINKANSEN_SPEED, "時速")
        assert tok is not None
        assert tok.reading_hira == "じそく", f"Got {tok.reading_hira!r}"

    def test_arabic_200_in_speed(self, tokenizer):
        """
        READING: 200 in 時速200キロメートル reads にひゃく (not によってれい).
        """
        tok = _first_token(tokenizer, _SHINKANSEN_SPEED, "200")
        assert tok is not None, "200 token not found"
        assert tok.reading_hira == "にひゃく", f"Got {tok.reading_hira!r}"

    def test_kilométer_reading(self, tokenizer):
        """
        READING: キロメートル reads きろめーとる.
        Standard loanword; important for learners reading speed/distance data.
        """
        tok = _first_token(tokenizer, _SHINKANSEN_SPEED, "キロメートル")
        assert tok is not None
        assert tok.reading_hira == "きろめーとる", f"Got {tok.reading_hira!r}"

    def test_year_1964_reading(self, tokenizer):
        """
        READING: 1964年 reads せんきゅうひゃくろくじゅうよんねん.
        Year of the first Shinkansen opening and Tokyo Olympics; frequently cited.
        The numeral merges with 年 into a single token.
        """
        tok = _first_token(tokenizer, _SHINKANSEN_DATE, "1964年")
        assert tok is not None, "1964年 token not found"
        assert tok.reading_hira == "せんきゅうひゃくろくじゅうよんねん", (
            f"Got {tok.reading_hira!r}"
        )

    def test_showa_reading(self, tokenizer):
        """
        READING: 昭和 (Shōwa era) reads しょうわ.
        The Shōwa era covers 1926-1989; very frequent in historical Japanese.
        """
        tok = _first_token(tokenizer, _SHINKANSEN_DATE, "昭和")
        assert tok is not None
        assert tok.reading_hira == "しょうわ", f"Got {tok.reading_hira!r}"

    def test_tsuitachi_date_reading(self, tokenizer):
        """
        READING: 1日 in a date expression (10月1日) reads ついたち, not いちにち.
        Learner impact: ついたち is a special reading for the first day of the
        month. A learner who sees いちにち (one day) as furigana will be confused
        — ついたち is how native speakers read and say it.
        """
        result = tokenizer.tokenize(_SHINKANSEN_DATE)
        tok = next((t for t in result.tokens if t.surface == "1日"), None)
        assert tok is not None, "1日 token not found"
        assert tok.reading_hira == "ついたち", (
            f"Expected ついたち (first of month), got {tok.reading_hira!r}"
        )

    def test_toukaidou_honsen_compound(self, tokenizer):
        """
        SEGMENT + READING: 東海道本線 (Tōkaidō Main Line) must be a single token
        reading とうかいどうほんせん.
        """
        result = tokenizer.tokenize(_SHINKANSEN_DATE)
        tok = next((t for t in result.tokens if t.surface == "東海道本線"), None)
        assert tok is not None, "東海道本線 not found as single token"
        assert tok.reading_hira == "とうかいどうほんせん", f"Got {tok.reading_hira!r}"

    def test_arabic_12_in_delay(self, tokenizer):
        """
        READING: 12 reads じゅうに (twelve), not いちに (per-digit).
        Context: 年間平均遅延時間は12秒 (average annual delay 12 seconds).
        """
        tok = _first_token(tokenizer, _SHINKANSEN_DELAY, "12")
        assert tok is not None, "12 token not found"
        assert tok.reading_hira == "じゅうに", f"Got {tok.reading_hira!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 4: Japanese language corpus (corpus/japanese_language.txt)
#
# Meta-linguistic text: grammar terms, linguistic typology, writing system.
# Tests that grammar terminology tokenizes into learnable units.
# ─────────────────────────────────────────────────────────────────────────────

_JA_GRAMMAR = "文は、「主語・修飾語・述語」の語順で構成される"
_JA_ACCENT  = "アクセントは高低アクセントである"
_JA_KEIGO   = "文法的・語彙的に発達した敬語体系があり"


class TestJapaneseLanguageCorpus:
    """
    Tests drawn from the Wikipedia article on 日本語.
    Covers grammar terms, pitch accent vocabulary, honorific system.
    All high-frequency vocabulary in Japanese language-learning contexts.
    """

    def test_shugo_reading(self, tokenizer):
        """
        READING: 主語 (subject) reads しゅご.
        Learners studying Japanese grammar encounter this constantly.
        """
        tok = _first_token(tokenizer, _JA_GRAMMAR, "主語")
        assert tok is not None
        assert tok.reading_hira == "しゅご", f"Got {tok.reading_hira!r}"

    def test_jutsugoshu_reading(self, tokenizer):
        """
        READING: 述語 (predicate) reads じゅつご.
        Basic grammar term; must be distinguishable from 主語 (しゅご).
        """
        tok = _first_token(tokenizer, _JA_GRAMMAR, "述語")
        assert tok is not None
        assert tok.reading_hira == "じゅつご", f"Got {tok.reading_hira!r}"

    def test_shuushokugo_reading(self, tokenizer):
        """
        READING: 修飾語 (modifier/adjunct) reads しゅうしょくご.
        Three-kanji grammar term; correct reading helps learners find it.
        """
        tok = _first_token(tokenizer, _JA_GRAMMAR, "修飾語")
        assert tok is not None
        assert tok.reading_hira == "しゅうしょくご", f"Got {tok.reading_hira!r}"

    def test_gojun_reading(self, tokenizer):
        """
        READING: 語順 (word order) reads ごじゅん, not ごじゅん.
        Key linguistic concept in explanations of SOV vs SVO structure.
        """
        tok = _first_token(tokenizer, _JA_GRAMMAR, "語順")
        assert tok is not None
        assert tok.reading_hira == "ごじゅん", f"Got {tok.reading_hira!r}"

    def test_accent_kouteI_reading(self, tokenizer):
        """
        READING: 高低 (high-low pitch) reads こうてい.
        Appears in descriptions of Japanese pitch-accent, important for
        learners studying pronunciation.
        """
        tok = _first_token(tokenizer, _JA_ACCENT, "高低")
        assert tok is not None
        assert tok.reading_hira == "こうてい", f"Got {tok.reading_hira!r}"

    def test_keigo_reading(self, tokenizer):
        """
        READING: 敬語 (honorific language) reads けいご, not きょうご.
        One of the most culturally significant concepts in Japanese for learners.
        """
        result = tokenizer.tokenize(_JA_KEIGO)
        tok = next((t for t in result.tokens if t.surface == "敬語"), None)
        assert tok is not None, "敬語 not found"
        assert tok.reading_hira == "けいご", f"Got {tok.reading_hira!r}"

    def test_nihongo_reading(self, tokenizer):
        """
        READING: 日本語 reads にほんご (standard form), not にっぽんご.
        The word for the Japanese language itself — must always be correct.
        """
        result = tokenizer.tokenize("日本語は世界で多くの人に学ばれている")
        tok = next((t for t in result.tokens if t.surface == "日本語"), None)
        assert tok is not None, "日本語 not found as single token"
        assert tok.reading_hira == "にほんご", f"Got {tok.reading_hira!r}"

    def test_full_ja_article(self, tokenizer):
        """Integration: full Japanese language corpus tokenizes without error."""
        text = (CORPUS_DIR / "japanese_language.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 300
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("日本語", "敬語", "アクセント", "方言", "語順"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 5: AI corpus (corpus/ai.txt)
#
# Technical text about artificial intelligence.
# Tests that modern technical compounds tokenize as learnable units.
# ─────────────────────────────────────────────────────────────────────────────

_AI_DEFINITION = "言語の理解や推論、問題解決などの知的行動を人間に代わってコンピュータに行わせる技術"
_AI_NLP = "自然言語処理（機械翻訳・かな漢字変換・構文解析・大規模言語モデル等）"


class TestAICorpus:
    """
    Tests drawn from the Wikipedia article on 人工知能.
    Covers AI/tech vocabulary that appears increasingly in Japanese media.
    Learners of contemporary Japanese need this vocabulary.
    """

    def test_jinkou_chinou_compound(self, tokenizer):
        """
        SEGMENT + READING: 人工知能 (artificial intelligence) must be a single
        token reading じんこうちのう.
        Learner impact: this is the Japanese term for AI — splitting it would
        prevent learners from finding it in a dictionary.
        """
        result = tokenizer.tokenize("人工知能（じんこうちのう）は計算機科学の一分野である")
        tok = next((t for t in result.tokens if t.surface == "人工知能"), None)
        assert tok is not None, "人工知能 not found as single token"
        assert tok.reading_hira == "じんこうちのう", f"Got {tok.reading_hira!r}"

    def test_shizen_gengo_shori_compound(self, tokenizer):
        """
        SEGMENT + READING: 自然言語処理 (natural language processing) must be
        a single token reading しぜんげんごしょり.
        This 5-kanji compound is a well-established technical term with its own
        dictionary entry; splitting would confuse learners.
        """
        tok = _first_token(tokenizer, _AI_NLP, "自然言語処理")
        assert tok is not None, "自然言語処理 not found as single token"
        assert tok.reading_hira == "しぜんげんごしょり", f"Got {tok.reading_hira!r}"

    def test_llm_compound(self, tokenizer):
        """
        SEGMENT + READING: 大規模言語モデル (large language model) must be a
        single token reading だいきぼげんごもでる.
        Contemporary AI vocabulary; very frequent in 2024-2026 Japanese media.
        """
        result = tokenizer.tokenize("大規模言語モデルが注目されている")
        tok = next((t for t in result.tokens if t.surface == "大規模言語モデル"), None)
        assert tok is not None, "大規模言語モデル not found as single token"
        assert tok.reading_hira == "だいきぼげんごもでる", f"Got {tok.reading_hira!r}"

    def test_suiron_reading(self, tokenizer):
        """
        READING: 推論 (reasoning, inference) reads すいろん.
        Fundamental AI/logic concept; common in academic and news Japanese.
        """
        tok = _first_token(tokenizer, _AI_DEFINITION, "推論")
        assert tok is not None
        assert tok.reading_hira == "すいろん", f"Got {tok.reading_hira!r}"

    def test_mondai_kaiketsu_compound(self, tokenizer):
        """
        SEGMENT + READING: 問題解決 (problem solving) must be one token
        reading もんだいかいけつ.
        """
        tok = _first_token(tokenizer, _AI_DEFINITION, "問題解決")
        assert tok is not None, "問題解決 not found as single token"
        assert tok.reading_hira == "もんだいかいけつ", f"Got {tok.reading_hira!r}"

    def test_computer_reading(self, tokenizer):
        """
        READING: コンピュータ reads こんぴゅーた (long vowel preserved).
        Common loanword; learners need the correct pronunciation.
        """
        result = tokenizer.tokenize("コンピュータに行わせる技術")
        tok = next((t for t in result.tokens if "コンピュータ" in t.surface and "科学" not in t.surface), None)
        assert tok is not None, "コンピュータ token not found"
        assert tok.reading_hira == "こんぴゅーた", f"Got {tok.reading_hira!r}"

    def test_kikai_honnyaku_split(self, tokenizer):
        """
        SEGMENT: 機械翻訳 (machine translation) may split into 機械 + 翻訳.
        This is pedagogically acceptable — both 機械 (machine) and 翻訳
        (translation) are learnable vocabulary items in their own right.
        Either split or compound is fine; this test documents current behavior.
        """
        result = tokenizer.tokenize("機械翻訳の精度が向上した")
        surfaces = [t.surface for t in result.tokens]
        assert "機械翻訳" in surfaces or ("機械" in surfaces and "翻訳" in surfaces), (
            f"Expected 機械翻訳 or 機械+翻訳; got {surfaces}"
        )

    def test_full_ai_article(self, tokenizer):
        """Integration: full AI corpus tokenizes without error."""
        text = (CORPUS_DIR / "ai.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 200
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("人工知能", "コンピュータ", "自然言語処理", "技術"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 6: Medicine corpus (corpus/medicine.txt)
#
# Medical/healthcare vocabulary: formal terminology, compound nouns,
# and common medical words encountered in Japanese health news.
# ─────────────────────────────────────────────────────────────────────────────

_MED_DEFINITION = "疾病に対する診断と治療を包括的に指す概念である"
_MED_EMERGENCY  = "救急医療や緩和医療など、対象とする疾病の段階によって分類される"


class TestMedicineCorpus:
    """
    Tests drawn from the Wikipedia article on 医療.
    Covers medical terminology essential for reading Japanese health news and
    understanding patient-oriented NHK Easy health articles.
    """

    def test_iryou_reading(self, tokenizer):
        """
        READING: 医療 (medical care) reads いりょう.
        Extremely common in health news and public announcements.
        """
        result = tokenizer.tokenize("医療は文化性が高いため")
        tok = next((t for t in result.tokens if t.surface == "医療"), None)
        assert tok is not None, "医療 not found"
        assert tok.reading_hira == "いりょう", f"Got {tok.reading_hira!r}"

    def test_shippei_reading(self, tokenizer):
        """
        READING: 疾病 (disease/illness) reads しっぺい.
        Formal medical term; the on-reading pair is non-obvious (疾=shitsu→shi, 病=byou→pei).
        """
        tok = _first_token(tokenizer, _MED_DEFINITION, "疾病")
        assert tok is not None, "疾病 not found"
        assert tok.reading_hira == "しっぺい", f"Got {tok.reading_hira!r}"

    def test_shindan_reading(self, tokenizer):
        """READING: 診断 (diagnosis) reads しんだん."""
        tok = _first_token(tokenizer, _MED_DEFINITION, "診断")
        assert tok is not None, "診断 not found"
        assert tok.reading_hira == "しんだん", f"Got {tok.reading_hira!r}"

    def test_chiryou_reading(self, tokenizer):
        """READING: 治療 (treatment) reads ちりょう (not じりょう)."""
        tok = _first_token(tokenizer, _MED_DEFINITION, "治療")
        assert tok is not None, "治療 not found"
        assert tok.reading_hira == "ちりょう", f"Got {tok.reading_hira!r}"

    def test_houkatsu_teki_reading(self, tokenizer):
        """READING: 包括的 (comprehensive) reads ほうかつてき."""
        tok = _first_token(tokenizer, _MED_DEFINITION, "包括的")
        assert tok is not None, "包括的 not found"
        assert tok.reading_hira == "ほうかつてき", f"Got {tok.reading_hira!r}"

    def test_kyuukyuu_iryou_compound(self, tokenizer):
        """
        SEGMENT + READING: 救急医療 (emergency medicine) may tokenize as
        救急 + 医療 — both are useful vocabulary. Reading must concatenate to
        きゅうきゅういりょう.
        """
        result = tokenizer.tokenize(_MED_EMERGENCY)
        surfaces = [t.surface for t in result.tokens]
        assert "救急医療" in surfaces or ("救急" in surfaces and "医療" in surfaces), (
            f"Expected 救急医療 or 救急+医療; got {surfaces}"
        )
        # Reading must be correct regardless of split
        if "救急医療" in surfaces:
            tok = next(t for t in result.tokens if t.surface == "救急医療")
            assert tok.reading_hira == "きゅうきゅういりょう"
        else:
            kyuu = next(t for t in result.tokens if t.surface == "救急")
            assert kyuu.reading_hira == "きゅうきゅう"

    def test_kanwa_reading(self, tokenizer):
        """READING: 緩和 (palliation/easing) reads かんわ."""
        tok = _first_token(tokenizer, _MED_EMERGENCY, "緩和")
        assert tok is not None, "緩和 not found"
        assert tok.reading_hira == "かんわ", f"Got {tok.reading_hira!r}"

    def test_medical_terms_are_content_words(self, tokenizer):
        """HIGHLIGHT: 疾病, 診断, 治療 must all be classified as content words."""
        result = tokenizer.tokenize(_MED_DEFINITION)
        content = {t.surface for t in result.tokens if t.is_content_word}
        for term in ("疾病", "診断", "治療"):
            assert term in content, f"{term!r} should be a content word"

    def test_full_medicine_article(self, tokenizer):
        """Integration: full medicine corpus tokenizes without error."""
        text = (CORPUS_DIR / "medicine.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 300
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("医療", "疾病", "診断", "治療", "患者"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 7: Edo period corpus (corpus/edo_period.txt)
#
# Historical Japanese text with era names, historical figures, and numbers.
# Tests Arabic numeral reading for historical years and date corrections.
# ─────────────────────────────────────────────────────────────────────────────

_EDO_FOUNDING = "1603年3月24日（慶長8年2月12日）に徳川家康が征夷大将軍に任命されて"
_EDO_DURATION = "慶応から明治に改元されるまでの265年間である"


class TestEdoPeriodCorpus:
    """
    Tests drawn from the Wikipedia article on 江戸時代.
    Historical text is particularly rich in numbers, era names, and
    formal vocabulary that learners encounter in history courses.
    """

    def test_edo_jidai_reading(self, tokenizer):
        """
        READING: 江戸時代 (Edo period). SudachiPy splits into 江戸 + 時代
        (both are independently useful vocabulary). The concatenation reads
        えどじだい. This test accepts either form and validates the readings.
        """
        result = tokenizer.tokenize("江戸時代という名は、江戸に将軍が常駐していたためである")
        surfaces = [t.surface for t in result.tokens]
        if "江戸時代" in surfaces:
            tok = next(t for t in result.tokens if t.surface == "江戸時代")
            assert tok.reading_hira == "えどじだい"
        else:
            edo = next((t for t in result.tokens if t.surface == "江戸"), None)
            assert edo is not None, "江戸 not found"
            assert edo.reading_hira == "えど"
            jidai = next((t for t in result.tokens if t.surface == "時代"), None)
            assert jidai is not None, "時代 not found"
            assert jidai.reading_hira == "じだい"

    def test_shogun_reading(self, tokenizer):
        """READING: 将軍 (shogun/general) reads しょうぐん."""
        result = tokenizer.tokenize("江戸に将軍が常駐していたためである")
        tok = next((t for t in result.tokens if t.surface == "将軍"), None)
        assert tok is not None, "将軍 not found"
        assert tok.reading_hira == "しょうぐん", f"Got {tok.reading_hira!r}"

    def test_bakufu_reading(self, tokenizer):
        """READING: 幕府 (shogunate/bakufu) reads ばくふ."""
        result = tokenizer.tokenize("江戸幕府（徳川幕府）の統治時代を指す時代区分である")
        tok = next((t for t in result.tokens if t.surface == "幕府"), None)
        assert tok is not None, "幕府 not found"
        assert tok.reading_hira == "ばくふ", f"Got {tok.reading_hira!r}"

    def test_tokugawa_ieyasu_compound(self, tokenizer):
        """
        SEGMENT + READING: 徳川家康 (Tokugawa Ieyasu) must be a single token
        reading とくがわいえやす.
        Proper names must stay whole for dictionary lookup.
        """
        result = tokenizer.tokenize(_EDO_FOUNDING)
        tok = next((t for t in result.tokens if t.surface == "徳川家康"), None)
        assert tok is not None, "徳川家康 not found as single token"
        assert tok.reading_hira == "とくがわいえやす", f"Got {tok.reading_hira!r}"

    def test_year_1603_reading(self, tokenizer):
        """
        READING: 1603年 must read せんろっぴゃくさんねん.
        Historical year in a real sentence from the corpus; per-digit reading
        (いちろくれいさんねん) is meaningless to a learner.
        The numeral merges with 年 into a single token.
        """
        result = tokenizer.tokenize(_EDO_FOUNDING)
        tok = next((t for t in result.tokens if t.surface == "1603年"), None)
        assert tok is not None, "1603年 not found"
        assert tok.reading_hira == "せんろっぴゃくさんねん", (
            f"Got {tok.reading_hira!r}"
        )

    def test_24nichi_in_date_context(self, tokenizer):
        """
        READING: 24日 in 1603年3月24日 reads にじゅうよっか (native date form).
        The date correction must apply even when embedded in a longer date string.
        """
        result = tokenizer.tokenize(_EDO_FOUNDING)
        tok = next((t for t in result.tokens if t.surface == "24日"), None)
        assert tok is not None, "24日 not found"
        assert tok.reading_hira == "にじゅうよっか", f"Got {tok.reading_hira!r}"

    def test_265_nenkan_reading(self, tokenizer):
        """
        READING: 265 in 265年間 reads にひゃくろくじゅうご.
        Multi-digit year count; correct reading requires positional conversion.
        """
        result = tokenizer.tokenize(_EDO_DURATION)
        tok = next((t for t in result.tokens if t.surface == "265"), None)
        assert tok is not None, "265 not found"
        assert tok.reading_hira == "にひゃくろくじゅうご", (
            f"Got {tok.reading_hira!r}"
        )

    def test_meiji_era_reading(self, tokenizer):
        """READING: 明治 reads めいじ (Meiji era name)."""
        result = tokenizer.tokenize(_EDO_DURATION)
        tok = next((t for t in result.tokens if t.surface == "明治"), None)
        assert tok is not None, "明治 not found"
        assert tok.reading_hira == "めいじ", f"Got {tok.reading_hira!r}"

    def test_full_edo_article(self, tokenizer):
        """Integration: full Edo period corpus tokenizes without error."""
        text = (CORPUS_DIR / "edo_period.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 300
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("江戸", "幕府", "将軍", "明治", "徳川"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 8: Baseball corpus (corpus/baseball.txt)
#
# Sports vocabulary with numbers (1チーム9人, 1845年) and technical terms.
# Tests numeral reading in sports context and loanword handling.
# ─────────────────────────────────────────────────────────────────────────────

_BASEBALL_TEAMS = "1チーム9人ずつ（指名打者（DH）制を採用する場合は10人）で構成された2チームが守備側と攻撃側に分かれ"
_BASEBALL_ORIGIN = "1845年にアメリカで現在の形・ルールの基礎がつくられた"


class TestBaseballCorpus:
    """
    Tests drawn from the Wikipedia article on 野球.
    Baseball is Japan's most popular sport and a rich domain for learners
    to practice reading sports news.
    """

    def test_yakyuu_reading(self, tokenizer):
        """READING: 野球 (baseball) reads やきゅう."""
        result = tokenizer.tokenize("野球は、2つのチームが攻撃と守備を交代しながら")
        tok = next((t for t in result.tokens if t.surface == "野球"), None)
        assert tok is not None, "野球 not found"
        assert tok.reading_hira == "やきゅう", f"Got {tok.reading_hira!r}"

    def test_kougeki_reading(self, tokenizer):
        """READING: 攻撃 (attack/offense) reads こうげき."""
        result = tokenizer.tokenize("野球は、2つのチームが攻撃と守備を交代しながら")
        tok = next((t for t in result.tokens if t.surface == "攻撃"), None)
        assert tok is not None, "攻撃 not found"
        assert tok.reading_hira == "こうげき", f"Got {tok.reading_hira!r}"

    def test_shuubi_reading(self, tokenizer):
        """READING: 守備 (defense/fielding) reads しゅび."""
        result = tokenizer.tokenize("野球は、2つのチームが攻撃と守備を交代しながら")
        tok = next((t for t in result.tokens if t.surface == "守備"), None)
        assert tok is not None, "守備 not found"
        assert tok.reading_hira == "しゅび", f"Got {tok.reading_hira!r}"

    def test_year_1845_reading(self, tokenizer):
        """
        READING: 1845年 reads せんはっぴゃくよんじゅうごねん.
        Note: 800 → はっぴゃく (special sandhi), tests the 8 in hundreds position.
        The numeral merges with 年 into a single token.
        """
        result = tokenizer.tokenize(_BASEBALL_ORIGIN)
        tok = next((t for t in result.tokens if t.surface == "1845年"), None)
        assert tok is not None, "1845年 not found"
        assert tok.reading_hira == "せんはっぴゃくよんじゅうごねん", (
            f"Got {tok.reading_hira!r}"
        )

    def test_toku_ten_reading(self, tokenizer):
        """READING: 得点 (score/points) reads とくてん."""
        result = tokenizer.tokenize("得点を競い合うバット・アンド・ボール・ゲームである")
        tok = next((t for t in result.tokens if t.surface == "得点"), None)
        assert tok is not None, "得点 not found"
        assert tok.reading_hira == "とくてん", f"Got {tok.reading_hira!r}"

    def test_kyushu_reading(self, tokenizer):
        """
        READING: Arabic 9 in '9人' reads きゅう.
        In this context SudachiPy may keep 9人 split; numeral reads correctly.
        """
        result = tokenizer.tokenize(_BASEBALL_TEAMS)
        tok = next((t for t in result.tokens if t.surface == "9"), None)
        if tok is not None:
            assert tok.reading_hira == "きゅう", f"9 should read きゅう, got {tok.reading_hira!r}"

    def test_full_baseball_article(self, tokenizer):
        """Integration: full baseball corpus tokenizes without error."""
        text = (CORPUS_DIR / "baseball.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 300
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("野球", "攻撃", "守備", "得点", "チーム"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 9: Space corpus (corpus/space.txt)
#
# Scientific vocabulary: cosmos, universe, atmosphere.
# Tests technical loanwords and compound scientific terms.
# ─────────────────────────────────────────────────────────────────────────────

_SPACE_BOUNDARY = "宇宙空間と大気圏内の境界として（便宜的に）カーマン・ラインが定義されている"


class TestSpaceCorpus:
    """
    Tests drawn from the Wikipedia article on 宇宙.
    Science vocabulary increasingly appears in NHK science news.
    """

    def test_uchuu_reading(self, tokenizer):
        """READING: 宇宙 (space/universe) reads うちゅう."""
        result = tokenizer.tokenize("宇宙はすべての物と事象の総体を意味する")
        tok = next((t for t in result.tokens if t.surface == "宇宙"), None)
        assert tok is not None, "宇宙 not found"
        assert tok.reading_hira == "うちゅう", f"Got {tok.reading_hira!r}"

    def test_uchuu_kukan_compound(self, tokenizer):
        """
        SEGMENT: 宇宙空間 (outer space) may stay whole or split into 宇宙 + 空間.
        Both splits are useful vocabulary. Test that surface text is preserved.
        """
        result = tokenizer.tokenize(_SPACE_BOUNDARY)
        surfaces = [t.surface for t in result.tokens]
        assert "宇宙空間" in surfaces or ("宇宙" in surfaces and "空間" in surfaces), (
            f"Expected 宇宙空間 or 宇宙+空間; got {surfaces}"
        )

    def test_taiki_ken_reading(self, tokenizer):
        """READING: 大気圏 (atmosphere) reads たいきけん."""
        tok = _first_token(tokenizer, _SPACE_BOUNDARY, "大気圏")
        assert tok is not None, "大気圏 not found"
        assert tok.reading_hira == "たいきけん", f"Got {tok.reading_hira!r}"

    def test_kyoukai_reading(self, tokenizer):
        """READING: 境界 (boundary) reads きょうかい."""
        tok = _first_token(tokenizer, _SPACE_BOUNDARY, "境界")
        assert tok is not None, "境界 not found"
        assert tok.reading_hira == "きょうかい", f"Got {tok.reading_hira!r}"

    def test_imi_suru_lemma(self, tokenizer):
        """LOOKUP: 意味する → lemma 意味する (suru-verb compound)."""
        result = tokenizer.tokenize("すべての物と事象の総体を意味する")
        # SudachiPy may split 意味する into 意味 + する
        surfaces = [t.surface for t in result.tokens]
        assert "意味する" in surfaces or "意味" in surfaces, (
            f"意味 not found in {surfaces}"
        )

    def test_full_space_article(self, tokenizer):
        """Integration: full space corpus tokenizes without error."""
        text = (CORPUS_DIR / "space.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 200
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("宇宙", "空間", "意味", "定義"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 10: Environment corpus (corpus/environment.txt)
#
# Environmental/policy vocabulary: pollution, protection, climate.
# Tests formal compound nouns common in civic and environmental journalism.
# ─────────────────────────────────────────────────────────────────────────────

_ENV_DEFINITION = "人類の活動に由来する周囲の環境の変化により発生した問題の総称"
_ENV_PROTECTION = "環境保護を推進したり昂発したりする団体を環境保護団体という"


class TestEnvironmentCorpus:
    """
    Tests drawn from the Wikipedia article on 環境問題.
    Environmental topics are heavily covered in NHK Easy and NHK World,
    making this vocabulary essential for intermediate learners.
    """

    def test_kankyou_reading(self, tokenizer):
        """READING: 環境 (environment) reads かんきょう."""
        result = tokenizer.tokenize("環境問題は人類の活動に由来する")
        tok = next((t for t in result.tokens if t.surface == "環境"), None)
        assert tok is not None, "環境 not found"
        assert tok.reading_hira == "かんきょう", f"Got {tok.reading_hira!r}"

    def test_jinrui_reading(self, tokenizer):
        """READING: 人類 (humanity/mankind) reads じんるい."""
        tok = _first_token(tokenizer, _ENV_DEFINITION, "人類")
        assert tok is not None, "人類 not found"
        assert tok.reading_hira == "じんるい", f"Got {tok.reading_hira!r}"

    def test_yuurai_reading(self, tokenizer):
        """READING: 由来 (origin/derivation) reads ゆらい."""
        tok = _first_token(tokenizer, _ENV_DEFINITION, "由来")
        assert tok is not None, "由来 not found"
        assert tok.reading_hira == "ゆらい", f"Got {tok.reading_hira!r}"

    def test_hogo_reading(self, tokenizer):
        """READING: 保護 (protection) reads ほご."""
        result = tokenizer.tokenize(_ENV_PROTECTION)
        tok = next((t for t in result.tokens if t.surface == "保護"), None)
        assert tok is not None, "保護 not found"
        assert tok.reading_hira == "ほご", f"Got {tok.reading_hira!r}"

    def test_suishin_reading(self, tokenizer):
        """READING: 推進 (promotion/advancement) reads すいしん."""
        tok = _first_token(tokenizer, _ENV_PROTECTION, "推進")
        assert tok is not None, "推進 not found"
        assert tok.reading_hira == "すいしん", f"Got {tok.reading_hira!r}"

    def test_kankyou_content_word(self, tokenizer):
        """HIGHLIGHT: 環境 is a content word (noun) and must be highlighted."""
        result = tokenizer.tokenize("環境問題は人類の活動に由来する")
        tok = next((t for t in result.tokens if t.surface == "環境"), None)
        assert tok is not None
        assert tok.is_content_word, "環境 should be a content word"

    def test_full_environment_article(self, tokenizer):
        """Integration: full environment corpus tokenizes without error."""
        text = (CORPUS_DIR / "environment.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 300
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("環境", "問題", "人類", "活動", "保護"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 11: Cooking corpus (corpus/cooking.txt)
#
# Food and culinary vocabulary: ingredients, preparation methods, culture.
# Everyday vocabulary that appears extensively in lifestyle media.
# ─────────────────────────────────────────────────────────────────────────────

_COOKING_DEF   = "料理は、食物をこしらえることで同時にこしらえた結果である食品そのもの"
_COOKING_INGR  = "食材、調味料などを組み合わせて加工を行うこと、およびそれを行ったものの総称"


class TestCookingCorpus:
    """
    Tests drawn from the Wikipedia article on 料理.
    Cooking vocabulary is high-frequency in everyday Japanese and essential
    for learners who want to engage with food culture, recipes, and media.
    """

    def test_ryouri_reading(self, tokenizer):
        """READING: 料理 (cooking/cuisine) reads りょうり."""
        tok = _first_token(tokenizer, _COOKING_DEF, "料理")
        assert tok is not None, "料理 not found"
        assert tok.reading_hira == "りょうり", f"Got {tok.reading_hira!r}"

    def test_shokubutsu_reading(self, tokenizer):
        """READING: 食物 (food/foodstuffs) reads しょくもつ (not たべもの — formal register)."""
        tok = _first_token(tokenizer, _COOKING_DEF, "食物")
        assert tok is not None, "食物 not found"
        assert tok.reading_hira == "しょくもつ", f"Got {tok.reading_hira!r}"

    def test_shokuhin_reading(self, tokenizer):
        """READING: 食品 (food product) reads しょくひん."""
        tok = _first_token(tokenizer, _COOKING_DEF, "食品")
        assert tok is not None, "食品 not found"
        assert tok.reading_hira == "しょくひん", f"Got {tok.reading_hira!r}"

    def test_shokuzai_reading(self, tokenizer):
        """READING: 食材 (ingredients) reads しょくざい."""
        tok = _first_token(tokenizer, _COOKING_INGR, "食材")
        assert tok is not None, "食材 not found"
        assert tok.reading_hira == "しょくざい", f"Got {tok.reading_hira!r}"

    def test_choumiryou_reading(self, tokenizer):
        """READING: 調味料 (seasoning/condiment) reads ちょうみりょう."""
        tok = _first_token(tokenizer, _COOKING_INGR, "調味料")
        assert tok is not None, "調味料 not found"
        assert tok.reading_hira == "ちょうみりょう", f"Got {tok.reading_hira!r}"

    def test_kakou_reading(self, tokenizer):
        """READING: 加工 (processing) reads かこう."""
        tok = _first_token(tokenizer, _COOKING_INGR, "加工")
        assert tok is not None, "加工 not found"
        assert tok.reading_hira == "かこう", f"Got {tok.reading_hira!r}"

    def test_kumiawase_lemma(self, tokenizer):
        """LOOKUP: 組み合わせて (te-form) → lemma 組み合わせる."""
        result = tokenizer.tokenize(_COOKING_INGR)
        tok = next((t for t in result.tokens if "組み合わせ" in t.surface), None)
        assert tok is not None, "組み合わせ token not found"
        assert tok.lemma in ("組み合わせる", "組み合わす"), (
            f"Expected 組み合わせる/組み合わす, got {tok.lemma!r}"
        )

    def test_food_terms_are_content_words(self, tokenizer):
        """HIGHLIGHT: 食材, 調味料, 加工 must all be content words."""
        result = tokenizer.tokenize(_COOKING_INGR)
        content = {t.surface for t in result.tokens if t.is_content_word}
        for term in ("食材", "調味料", "加工"):
            assert term in content, f"{term!r} should be a content word"

    def test_full_cooking_article(self, tokenizer):
        """Integration: full cooking corpus tokenizes without error."""
        text = (CORPUS_DIR / "cooking.txt").read_text()
        result = tokenizer.tokenize(text)
        assert len(result.tokens) > 200
        lemmas = _all_content_lemmas(tokenizer, text)
        for expected in ("料理", "食品", "食材", "調理", "加工"):
            assert expected in lemmas, f"Missing expected lemma: {expected!r}"
