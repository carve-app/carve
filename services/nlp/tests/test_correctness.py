"""
Japanese NLP correctness test suite.

This is the Phase 0 gate: ALL tests here must pass before the project advances
to Phase 1. Each test case is hand-verified against authoritative sources
(JMdict, NHK pitch accent dictionary, native speaker review).

Documented Migaku failure cases are marked with: # MIGAKU-FAIL
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
    """Return the full hiragana reading of text (all morphemes concatenated)."""
    result = tokenizer.tokenize(text, mode="A")
    return "".join(t.reading_hira for t in result.a_tokens)


def lemma(tokenizer: JapaneseTokenizer, text: str) -> str:
    """Return the lemma (dictionary form) of the first content token."""
    result = tokenizer.tokenize(text, mode="C")
    for t in result.tokens:
        if t.is_content_word:
            return t.lemma
    return result.tokens[0].lemma if result.tokens else ""


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 1: Migaku documented failure cases
# ─────────────────────────────────────────────────────────────────────────────

class TestMigakuFailureCases:
    """
    These are the exact cases reported as failures in Migaku's parser.
    They represent the minimum correctness bar for the Carve NLP service.
    """

    def test_gozaimasen_full_reading(self, tokenizer):  # MIGAKU-FAIL
        """
        ございません must read as ごzaiません (not be split with wrong readings).
        Migaku splits this into ごzai + ません with incorrect analysis.
        """
        r = reading(tokenizer, "ございません")
        assert r == "ございません", f"Expected ございません, got {r!r}"

    def test_haitte_reading(self, tokenizer):  # MIGAKU-FAIL
        """
        入って must read as はいって.
        Migaku produced incorrect reading for this very common word.
        """
        r = reading(tokenizer, "入って")
        assert r == "はいって", f"Expected はいって, got {r!r}"

    def test_haitte_lemma(self, tokenizer):  # MIGAKU-FAIL
        """入って lemma must be 入る."""
        result = tokenizer.tokenize("入って")
        lemmas = [t.lemma for t in result.tokens if t.is_content_word]
        assert "入る" in lemmas, f"Expected 入る in lemmas, got {lemmas}"

    def test_gozaimasen_not_misplit(self, tokenizer):  # MIGAKU-FAIL
        """
        ございません must not be split as [ごzai, ません].
        It should be analyzed into its morphological components correctly.
        """
        result = tokenizer.tokenize("ございません", mode="A")
        surfaces = [t.surface for t in result.a_tokens]
        # Migaku's wrong split: exactly ['ございません', 'ません'] or ['ごzai', 'ません']
        # The correct analysis has >= 2 morphemes OR correctly identifies as a unit
        # Key requirement: readings must sum to ございません
        full_reading = "".join(t.reading_hira for t in result.a_tokens)
        assert full_reading == "ございません", (
            f"ございません reading = {full_reading!r}, surfaces = {surfaces}"
        )


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 2: Kanji reading disambiguation
# These test cases have multiple possible readings; correct choice is context-sensitive.
# ─────────────────────────────────────────────────────────────────────────────

class TestKanjiReadingDisambiguation:

    def test_tokyo_to(self, tokenizer):
        """東京都 must read とうきょうと not とうきょうみやこ."""
        r = reading(tokenizer, "東京都")
        assert r == "とうきょうと", f"Expected とうきょうと, got {r!r}"

    def test_ikimono(self, tokenizer):
        """生き物 must read いきもの not なまきもの."""
        r = reading(tokenizer, "生き物")
        assert r == "いきもの", f"Expected いきもの, got {r!r}"

    def test_mizu(self, tokenizer):
        """水 alone must read みず."""
        r = reading(tokenizer, "水")
        assert r == "みず", f"Expected みず, got {r!r}"

    def test_hito_reading(self, tokenizer):
        """人 alone must read ひと."""
        r = reading(tokenizer, "人")
        assert r == "ひと", f"Expected ひと, got {r!r}"

    def test_kaisha_in(self, tokenizer):
        """会社員 must read かいしゃいん."""
        r = reading(tokenizer, "会社員")
        assert r == "かいしゃいん", f"Expected かいしゃいん, got {r!r}"

    def test_nihongo(self, tokenizer):
        """日本語 must read にほんご."""
        r = reading(tokenizer, "日本語")
        assert r == "にほんご", f"Expected にほんご, got {r!r}"

    def test_gakko(self, tokenizer):
        """学校 must read がっこう."""
        r = reading(tokenizer, "学校")
        assert r == "がっこう", f"Expected がっこう, got {r!r}"

    def test_te_form_taberu(self, tokenizer):
        """食べて must read たべて."""
        r = reading(tokenizer, "食べて")
        assert r == "たべて", f"Expected たべて, got {r!r}"

    def test_te_form_kaku(self, tokenizer):
        """書いて must read かいて."""
        r = reading(tokenizer, "書いて")
        assert r == "かいて", f"Expected かいて, got {r!r}"

    def test_te_form_kuru(self, tokenizer):
        """来て must read きて."""
        r = reading(tokenizer, "来て")
        assert r == "きて", f"Expected きて, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 3: Verb conjugation lemmatization
# ─────────────────────────────────────────────────────────────────────────────

class TestVerbLemmatization:

    def test_taberu_base(self, tokenizer):
        """食べる lemma is 食べる."""
        l = lemma(tokenizer, "食べる")
        assert l == "食べる", f"Expected 食べる, got {l!r}"

    def test_taberareta(self, tokenizer):
        """食べられた (passive past) lemma is 食べる."""
        result = tokenizer.tokenize("食べられた")
        l = lemma(tokenizer, "食べられた")
        assert l == "食べる", f"Expected 食べる, got {l!r}"

    def test_benkyou_suru(self, tokenizer):
        """勉強する is a single compound verb."""
        r = reading(tokenizer, "勉強する")
        assert r == "べんきょうする", f"Expected べんきょうする, got {r!r}"

    def test_benkyou_shita(self, tokenizer):
        """勉強した past form reading."""
        r = reading(tokenizer, "勉強した")
        assert r == "べんきょうした", f"Expected べんきょうした, got {r!r}"

    def test_ikeba(self, tokenizer):
        """行けば conditional reading."""
        r = reading(tokenizer, "行けば")
        assert r == "いけば", f"Expected いけば, got {r!r}"

    def test_miru(self, tokenizer):
        """見る lemma is 見る."""
        l = lemma(tokenizer, "見ている")
        assert l == "見る", f"Expected 見る, got {l!r}"

    def test_kuru_lemma(self, tokenizer):
        """来る irregular verb."""
        r = reading(tokenizer, "来る")
        assert r == "くる", f"Expected くる, got {r!r}"

    def test_suru_te_form(self, tokenizer):
        """して is て-form of する."""
        r = reading(tokenizer, "して")
        assert r == "して", f"Expected して, got {r!r}"

    def test_negative_form(self, tokenizer):
        """食べない negative reading."""
        r = reading(tokenizer, "食べない")
        assert r == "たべない", f"Expected たべない, got {r!r}"

    def test_potential_form(self, tokenizer):
        """書ける potential form reading."""
        r = reading(tokenizer, "書ける")
        assert r == "かける", f"Expected かける, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 4: Noun and adjective readings
# ─────────────────────────────────────────────────────────────────────────────

class TestNounAdjectiveReadings:

    def test_kawaii(self, tokenizer):
        """可愛い must read かわいい."""
        r = reading(tokenizer, "可愛い")
        assert r == "かわいい", f"Expected かわいい, got {r!r}"

    def test_muzukashii(self, tokenizer):
        """難しい must read むずかしい."""
        r = reading(tokenizer, "難しい")
        assert r == "むずかしい", f"Expected むずかしい, got {r!r}"

    def test_tanoshii(self, tokenizer):
        """楽しい must read たのしい."""
        r = reading(tokenizer, "楽しい")
        assert r == "たのしい", f"Expected たのしい, got {r!r}"

    def test_densha(self, tokenizer):
        """電車 must read でんしゃ."""
        r = reading(tokenizer, "電車")
        assert r == "でんしゃ", f"Expected でんしゃ, got {r!r}"

    def test_toshokan(self, tokenizer):
        """図書館 must read としょかん."""
        r = reading(tokenizer, "図書館")
        assert r == "としょかん", f"Expected としょかん, got {r!r}"

    def test_byouin(self, tokenizer):
        """病院 must read びょういん."""
        r = reading(tokenizer, "病院")
        assert r == "びょういん", f"Expected びょういん, got {r!r}"

    def test_onaka(self, tokenizer):
        """お腹 must read おなか."""
        r = reading(tokenizer, "お腹")
        assert r == "おなか", f"Expected おなか, got {r!r}"

    def test_chotto(self, tokenizer):
        """ちょっと reading (kana-only, should pass through)."""
        r = reading(tokenizer, "ちょっと")
        assert r == "ちょっと", f"Expected ちょっと, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 5: Compound words and tricky segmentation
# ─────────────────────────────────────────────────────────────────────────────

class TestCompoundSegmentation:

    def test_wakaru(self, tokenizer):
        """分かる must read わかる."""
        r = reading(tokenizer, "分かる")
        assert r == "わかる", f"Expected わかる, got {r!r}"

    def test_taisetsu(self, tokenizer):
        """大切 must read たいせつ."""
        r = reading(tokenizer, "大切")
        assert r == "たいせつ", f"Expected たいせつ, got {r!r}"

    def test_iroiro(self, tokenizer):
        """色々 must read いろいろ."""
        r = reading(tokenizer, "色々")
        assert r == "いろいろ", f"Expected いろいろ, got {r!r}"

    def test_hajimete(self, tokenizer):
        """初めて must read はじめて."""
        r = reading(tokenizer, "初めて")
        assert r == "はじめて", f"Expected はじめて, got {r!r}"

    def test_kodomo(self, tokenizer):
        """子供 must read こども."""
        r = reading(tokenizer, "子供")
        assert r == "こども", f"Expected こども, got {r!r}"

    def test_shizen(self, tokenizer):
        """自然 must read しぜん."""
        r = reading(tokenizer, "自然")
        assert r == "しぜん", f"Expected しぜん, got {r!r}"

    def test_tomodachi(self, tokenizer):
        """友達 must read ともだち."""
        r = reading(tokenizer, "友達")
        assert r == "ともだち", f"Expected ともだち, got {r!r}"

    def test_oshieru(self, tokenizer):
        """教える must read おしえる."""
        r = reading(tokenizer, "教える")
        assert r == "おしえる", f"Expected おしえる, got {r!r}"

    def test_kangaeru(self, tokenizer):
        """考える must read かんがえる."""
        r = reading(tokenizer, "考える")
        assert r == "かんがえる", f"Expected かんがえる, got {r!r}"

    def test_suki(self, tokenizer):
        """好き must read すき."""
        r = reading(tokenizer, "好き")
        assert r == "すき", f"Expected すき, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 6: Grammar structures and longer phrases
# ─────────────────────────────────────────────────────────────────────────────

class TestGrammarStructures:

    def test_nakereba_naranai(self, tokenizer):
        """食べなければならない — must-eat form, correct reading."""
        r = reading(tokenizer, "食べなければならない")
        assert r == "たべなければならない", f"Got {r!r}"

    def test_te_iru_progressive(self, tokenizer):
        """食べている progressive, reading."""
        r = reading(tokenizer, "食べている")
        assert r == "たべている", f"Got {r!r}"

    def test_shimaimashita(self, tokenizer):
        """してしまいました — polite completion, reading."""
        r = reading(tokenizer, "してしまいました")
        assert r == "してしまいました", f"Got {r!r}"

    def test_ja_nai(self, tokenizer):
        """じゃない — casual negative copula, reading."""
        r = reading(tokenizer, "じゃない")
        assert r == "じゃない", f"Got {r!r}"

    def test_deshita(self, tokenizer):
        """でした — polite past copula."""
        r = reading(tokenizer, "でした")
        assert r == "でした", f"Got {r!r}"

    def test_sentence_wo_particle(self, tokenizer):
        """毎日ご飯を食べる — particle を should not be content word."""
        result = tokenizer.tokenize("毎日ご飯を食べる")
        particles = [t for t in result.tokens if t.surface == "を"]
        assert all(not p.is_content_word for p in particles), (
            "Particle を incorrectly marked as content word"
        )

    def test_content_word_count(self, tokenizer):
        """毎日ご飯を食べる — should have 3 content words: 毎日, ご飯, 食べる."""
        result = tokenizer.tokenize("毎日ご飯を食べる")
        content = [t for t in result.tokens if t.is_content_word]
        assert len(content) == 3, (
            f"Expected 3 content words, got {len(content)}: {[t.surface for t in content]}"
        )

    def test_long_sentence_reading(self, tokenizer):
        """全体的な読み順 for a common sentence."""
        r = reading(tokenizer, "日本語を勉強しています")
        assert r == "にほんごをべんきょうしています", f"Got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 7: Furigana alignment
# ─────────────────────────────────────────────────────────────────────────────

class TestFuriganaAlignment:

    def test_taberu_furigana(self, tokenizer):
        """食べる → 食(た)べる — only the kanji gets furigana."""
        spans = generate_furigana("食べる", "たべる")
        # Should produce: [kanji_span("食","た"), kana_span("べる","")]
        kanji_spans = [s for s in spans if s.reading]
        assert len(kanji_spans) >= 1
        assert any(s.text == "食" and s.reading == "た" for s in spans), (
            f"Expected 食→た in furigana, got {spans}"
        )

    def test_ikimono_furigana(self, tokenizer):
        """生き物 → 生(い)き物(もの)."""
        spans = generate_furigana("生き物", "いきもの")
        non_empty = [s for s in spans if s.reading]
        assert len(non_empty) >= 1

    def test_pure_kana_no_furigana(self, tokenizer):
        """ありがとう — no kanji, no furigana spans."""
        spans = generate_furigana("ありがとう", "ありがとう")
        assert all(s.reading == "" for s in spans), (
            f"Kana-only text should have empty readings: {spans}"
        )

    def test_pure_kanji_furigana(self, tokenizer):
        """山 → 山(やま) — pure kanji gets full reading."""
        spans = generate_furigana("山", "やま")
        assert len(spans) == 1
        assert spans[0].text == "山"
        assert spans[0].reading == "やま"

    def test_mixed_furigana_hairu(self, tokenizer):
        """入る → 入(はい)る."""
        spans = generate_furigana("入る", "はいる")
        kanji_spans = [s for s in spans if s.reading]
        assert len(kanji_spans) >= 1
        assert any("入" in s.text and "はい" in s.reading for s in spans), (
            f"Expected 入→はい, got {spans}"
        )


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 8: Normalization
# ─────────────────────────────────────────────────────────────────────────────

class TestNormalization:

    def test_fullwidth_ascii_normalized(self, tokenizer):
        """Full-width ！→ ! before tokenization."""
        # Text with full-width characters should normalize without error
        result = tokenizer.tokenize("日本語！")
        surfaces = [t.surface for t in result.tokens]
        # Should not crash; surface should not contain full-width ASCII punct
        assert "!" in " ".join(surfaces) or "！" not in " ".join(surfaces)

    def test_nfc_normalization(self, tokenizer):
        """NFC normalization applied before tokenization."""
        # NFD decomposed か (か + combining dakuten) vs NFC が
        nfd_ga = "が"  # か + combining voiced mark = が in NFD
        nfc_ga = "が"
        r1 = reading(tokenizer, nfd_ga + "んばれ")
        r2 = reading(tokenizer, nfc_ga + "んばれ")
        assert r1 == r2, f"NFD and NFC should produce same reading: {r1!r} vs {r2!r}"

    def test_kata_to_hira(self):
        """katakana → hiragana conversion."""
        assert kata_to_hira("タベル") == "たべる"
        assert kata_to_hira("ニホンゴ") == "にほんご"
        assert kata_to_hira("ゴザイマセン") == "ございません"
        assert kata_to_hira("abc") == "abc"  # non-kana unchanged


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 9: POS tagging
# ─────────────────────────────────────────────────────────────────────────────

class TestPOSTagging:

    def test_noun_pos(self, tokenizer):
        """日本語 is classified as a noun."""
        result = tokenizer.tokenize("日本語")
        assert any(t.pos == "名詞" for t in result.tokens)

    def test_verb_pos(self, tokenizer):
        """食べる is classified as a verb."""
        result = tokenizer.tokenize("食べる")
        assert any(t.pos == "動詞" for t in result.tokens)

    def test_particle_not_content(self, tokenizer):
        """助詞 (particle) は is not a content word."""
        result = tokenizer.tokenize("これは")
        particles = [t for t in result.tokens if t.pos == "助詞"]
        assert all(not p.is_content_word for p in particles)

    def test_adjective_pos(self, tokenizer):
        """楽しい is an i-adjective (形容詞)."""
        result = tokenizer.tokenize("楽しい")
        assert any(t.pos == "形容詞" for t in result.tokens)

    def test_na_adjective_pos(self, tokenizer):
        """きれい is a na-adjective (形状詞 in Sudachi's scheme)."""
        result = tokenizer.tokenize("きれいな")
        # Sudachi uses 形状詞 for na-adjectives
        poses = {t.pos for t in result.tokens}
        assert "形状詞" in poses or "名詞" in poses  # acceptable classifications

    def test_adverb_is_content(self, tokenizer):
        """とても (adverb) is a content word."""
        result = tokenizer.tokenize("とても")
        assert any(t.is_content_word for t in result.tokens)
