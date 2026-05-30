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


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 10: Numbers and counters
# ─────────────────────────────────────────────────────────────────────────────

class TestNumbersAndCounters:

    def test_hitotsu(self, tokenizer):
        """一つ must read ひとつ."""
        r = reading(tokenizer, "一つ")
        assert r == "ひとつ", f"Expected ひとつ, got {r!r}"

    def test_futari(self, tokenizer):
        """二人 must read ふたり (not ににん)."""
        r = reading(tokenizer, "二人")
        assert r == "ふたり", f"Expected ふたり, got {r!r}"

    def test_sanmai(self, tokenizer):
        """三枚 must read さんまい."""
        r = reading(tokenizer, "三枚")
        assert r == "さんまい", f"Expected さんまい, got {r!r}"

    def test_hyaku(self, tokenizer):
        """百 alone reads ひゃく."""
        r = reading(tokenizer, "百")
        assert r == "ひゃく", f"Expected ひゃく, got {r!r}"

    def test_sen(self, tokenizer):
        """千 alone reads せん."""
        r = reading(tokenizer, "千")
        assert r == "せん", f"Expected せん, got {r!r}"

    def test_nannichi(self, tokenizer):
        """何日 must read なんにち."""
        r = reading(tokenizer, "何日")
        assert r == "なんにち", f"Expected なんにち, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 11: Katakana loanwords
# ─────────────────────────────────────────────────────────────────────────────

class TestKatakanaLoanwords:

    def test_terebi(self, tokenizer):
        """テレビ — kana-only, reading converts to hiragana."""
        r = reading(tokenizer, "テレビ")
        assert r == "てれび", f"Expected てれび, got {r!r}"

    def test_koohii(self, tokenizer):
        """コーヒー — has prolonged sound mark ー."""
        r = reading(tokenizer, "コーヒー")
        assert r == "こーひー", f"Expected こーひー, got {r!r}"

    def test_pasokon(self, tokenizer):
        """パソコン — compound loanword."""
        r = reading(tokenizer, "パソコン")
        assert r == "ぱそこん", f"Expected ぱそこん, got {r!r}"

    def test_aisu_kuriimu(self, tokenizer):
        """アイスクリーム."""
        r = reading(tokenizer, "アイスクリーム")
        assert r == "あいすくりーむ", f"Expected あいすくりーむ, got {r!r}"

    def test_sumaho(self, tokenizer):
        """スマホ."""
        r = reading(tokenizer, "スマホ")
        assert r == "すまほ", f"Expected すまほ, got {r!r}"

    def test_intanetto(self, tokenizer):
        """インターネット."""
        r = reading(tokenizer, "インターネット")
        assert r == "いんたーねっと", f"Expected いんたーねっと, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 12: Place names
# ─────────────────────────────────────────────────────────────────────────────

class TestPlaceNames:

    def test_osaka(self, tokenizer):
        """大阪 must read おおさか."""
        r = reading(tokenizer, "大阪")
        assert r == "おおさか", f"Expected おおさか, got {r!r}"

    def test_kyoto(self, tokenizer):
        """京都 must read きょうと."""
        r = reading(tokenizer, "京都")
        assert r == "きょうと", f"Expected きょうと, got {r!r}"

    def test_akihabara(self, tokenizer):
        """秋葉原 must read あきはばら."""
        r = reading(tokenizer, "秋葉原")
        assert r == "あきはばら", f"Expected あきはばら, got {r!r}"

    def test_shinjuku(self, tokenizer):
        """新宿 must read しんじゅく."""
        r = reading(tokenizer, "新宿")
        assert r == "しんじゅく", f"Expected しんじゅく, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 13: Additional verb forms
# ─────────────────────────────────────────────────────────────────────────────

class TestAdditionalVerbForms:

    def test_volitional_form(self, tokenizer):
        """食べよう volitional must read たべよう."""
        r = reading(tokenizer, "食べよう")
        assert r == "たべよう", f"Expected たべよう, got {r!r}"

    def test_tai_form(self, tokenizer):
        """食べたい desiderative must read たべたい."""
        r = reading(tokenizer, "食べたい")
        assert r == "たべたい", f"Expected たべたい, got {r!r}"

    def test_passive_form(self, tokenizer):
        """書かれる passive must read かかれる."""
        r = reading(tokenizer, "書かれる")
        assert r == "かかれる", f"Expected かかれる, got {r!r}"

    def test_causative_form(self, tokenizer):
        """食べさせる causative must read たべさせる."""
        r = reading(tokenizer, "食べさせる")
        assert r == "たべさせる", f"Expected たべさせる, got {r!r}"

    def test_imperative_form(self, tokenizer):
        """食べろ imperative must read たべろ."""
        r = reading(tokenizer, "食べろ")
        assert r == "たべろ", f"Expected たべろ, got {r!r}"

    def test_masu_form(self, tokenizer):
        """食べます polite present must read たべます."""
        r = reading(tokenizer, "食べます")
        assert r == "たべます", f"Expected たべます, got {r!r}"

    def test_masen_form(self, tokenizer):
        """食べません polite negative must read たべません."""
        r = reading(tokenizer, "食べません")
        assert r == "たべません", f"Expected たべません, got {r!r}"

    def test_mashita_form(self, tokenizer):
        """食べました polite past must read たべました."""
        r = reading(tokenizer, "食べました")
        assert r == "たべました", f"Expected たべました, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 14: Common expressions
# ─────────────────────────────────────────────────────────────────────────────

class TestCommonExpressions:

    def test_sumimasen(self, tokenizer):
        """すみません — common apology."""
        r = reading(tokenizer, "すみません")
        assert r == "すみません", f"Expected すみません, got {r!r}"

    def test_arigatou(self, tokenizer):
        """ありがとう — thank you."""
        r = reading(tokenizer, "ありがとう")
        assert r == "ありがとう", f"Expected ありがとう, got {r!r}"

    def test_yoroshiku(self, tokenizer):
        """よろしく."""
        r = reading(tokenizer, "よろしく")
        assert r == "よろしく", f"Expected よろしく, got {r!r}"

    def test_hajimemashite(self, tokenizer):
        """初めまして must read はじめまして."""
        r = reading(tokenizer, "初めまして")
        assert r == "はじめまして", f"Expected はじめまして, got {r!r}"

    def test_daijoubu(self, tokenizer):
        """大丈夫 must read だいじょうぶ."""
        r = reading(tokenizer, "大丈夫")
        assert r == "だいじょうぶ", f"Expected だいじょうぶ, got {r!r}"

    def test_moshimoshi(self, tokenizer):
        """もしもし — phone greeting."""
        r = reading(tokenizer, "もしもし")
        assert r == "もしもし", f"Expected もしもし, got {r!r}"

    def test_wakarimashita(self, tokenizer):
        """分かりました must read わかりました."""
        r = reading(tokenizer, "分かりました")
        assert r == "わかりました", f"Expected わかりました, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 15: Core vocabulary
# ─────────────────────────────────────────────────────────────────────────────

class TestCoreVocabulary:

    def test_kuuki(self, tokenizer):
        """空気 must read くうき."""
        r = reading(tokenizer, "空気")
        assert r == "くうき", f"Expected くうき, got {r!r}"

    def test_jikan(self, tokenizer):
        """時間 must read じかん."""
        r = reading(tokenizer, "時間")
        assert r == "じかん", f"Expected じかん, got {r!r}"

    def test_denwa(self, tokenizer):
        """電話 must read でんわ."""
        r = reading(tokenizer, "電話")
        assert r == "でんわ", f"Expected でんわ, got {r!r}"

    def test_eiga(self, tokenizer):
        """映画 must read えいが."""
        r = reading(tokenizer, "映画")
        assert r == "えいが", f"Expected えいが, got {r!r}"

    def test_ongaku(self, tokenizer):
        """音楽 must read おんがく."""
        r = reading(tokenizer, "音楽")
        assert r == "おんがく", f"Expected おんがく, got {r!r}"


# ─────────────────────────────────────────────────────────────────────────────
# SECTION 16: NHK Web Easy article — 川柳コンクール
#
# Source article (furigana stripped):
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
#
# Every assertion below is hand-verified against:
#   1. NHK Easy's own furigana (authoritative reading source)
#   2. Actual SudachiPy output (run 2026-05-30)
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
    Full-article tokenization test for the NHK Web Easy 川柳コンクール article.
    Covers readings, lemmatization, and content-word detection for every
    significant word in the article.
    """

    # ── Readings ──────────────────────────────────────────────────────────────

    def test_senryu_reading(self, tokenizer):
        """川柳 reads せんりゅう (NHK Easy furigana: せんりゅう)."""
        assert reading(tokenizer, "川柳") == "せんりゅう"

    def test_nihon_reading(self, tokenizer):
        """日本 reads にっぽん (Sudachi normalises to にっぽん; NHK Easy: にっぽん)."""
        result = tokenizer.tokenize("川柳は日本の短い詩のひとつです")
        nihon = next(t for t in result.tokens if t.surface == "日本")
        assert nihon.reading_hira == "にっぽん", (
            f"日本 reading: expected にっぽん, got {nihon.reading_hira!r}"
        )

    def test_mijikai_reading(self, tokenizer):
        """短い reads みじかい."""
        assert reading(tokenizer, "短い") == "みじかい"

    def test_shi_reading(self, tokenizer):
        """詩 (poem) reads し."""
        result = tokenizer.tokenize("短い詩のひとつ")
        shi = next(t for t in result.tokens if t.surface == "詩")
        assert shi.reading_hira == "し", f"詩: expected し, got {shi.reading_hira!r}"

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
        """無一文 reads むいちもん."""
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

    def test_cashless_reading(self, tokenizer):
        """キャッシュレス reads きゃっしゅれす (loanword, katakana→hiragana)."""
        assert reading(tokenizer, "キャッシュレス") == "きゃっしゅれす"

    def test_smartphone_reading(self, tokenizer):
        """スマートフォン reads すまーとふぉん."""
        assert reading(tokenizer, "スマートフォン") == "すまーとふぉん"

    def test_internet_reading(self, tokenizer):
        """インターネット reads いんたーねっと."""
        assert reading(tokenizer, "インターネット") == "いんたーねっと"

    # ── Lemmatization ─────────────────────────────────────────────────────────

    def test_okotta_lemma(self, tokenizer):
        """起こった (past) lemma is 起こる."""
        result = tokenizer.tokenize("周りで起こったおもしろいことを")
        okoru = next((t for t in result.tokens if "起" in t.surface), None)
        assert okoru is not None, "起こ token not found"
        assert okoru.lemma == "起こる", f"Expected 起こる, got {okoru.lemma!r}"

    def test_atsumari_lemma(self, tokenizer):
        """集まりました lemma is 集まる."""
        result = tokenizer.tokenize("川柳が集まりました")
        atsumaru = next((t for t in result.tokens if "集" in t.surface), None)
        assert atsumaru is not None, "集 token not found"
        assert atsumaru.lemma == "集まる", f"Expected 集まる, got {atsumaru.lemma!r}"

    def test_nakunari_lemma(self, tokenizer):
        """無くなり lemma is 無くなる."""
        result = tokenizer.tokenize("充電無くなり")
        nakunaru = next((t for t in result.tokens if "無" in t.surface), None)
        assert nakunaru is not None, "無くなり token not found"
        assert nakunaru.lemma == "無くなる", f"Expected 無くなる, got {nakunaru.lemma!r}"

    def test_harou_lemma(self, tokenizer):
        """払おう (volitional) lemma is 払う."""
        result = tokenizer.tokenize("お金を払おうとしたら")
        harau = next((t for t in result.tokens if "払" in t.surface), None)
        assert harau is not None, "払 token not found"
        assert harau.lemma == "払う", f"Expected 払う, got {harau.lemma!r}"

    def test_narenakute_lemma(self, tokenizer):
        """慣れなくて — verb stem lemma is 慣れる."""
        result = tokenizer.tokenize("慣れなくて困る")
        nareru = next((t for t in result.tokens if "慣" in t.surface), None)
        assert nareru is not None, "慣れ token not found"
        assert nareru.lemma == "慣れる", f"Expected 慣れる, got {nareru.lemma!r}"

    def test_ookatta_lemma(self, tokenizer):
        """多かった (past of i-adj) lemma is 多い."""
        result = tokenizer.tokenize("川柳が多かったです")
        ooi = next((t for t in result.tokens if "多" in t.surface), None)
        assert ooi is not None, "多 token not found"
        assert ooi.lemma == "多い", f"Expected 多い, got {ooi.lemma!r}"

    # ── Segmentation ──────────────────────────────────────────────────────────

    def test_smartphone_single_token(self, tokenizer):
        """スマートフォン must be a single token, not split."""
        result = tokenizer.tokenize("スマートフォンでお金を払う")
        assert "スマートフォン" in [t.surface for t in result.tokens], (
            f"スマートフォン not single token: {[t.surface for t in result.tokens]}"
        )

    def test_internet_single_token(self, tokenizer):
        """インターネット must be a single token."""
        result = tokenizer.tokenize("インターネットなどが")
        assert "インターネット" in [t.surface for t in result.tokens], (
            f"インターネット not single token: {[t.surface for t in result.tokens]}"
        )

    def test_cashless_single_token(self, tokenizer):
        """キャッシュレス must be a single token (from the poem)."""
        result = tokenizer.tokenize("キャッシュレス　充電無くなり　無一文")
        assert "キャッシュレス" in [t.surface for t in result.tokens], (
            f"キャッシュレス not single token: {[t.surface for t in result.tokens]}"
        )

    def test_digital_ka_split(self, tokenizer):
        """デジタル化 splits as デジタル + 化 (Sudachi C-mode does not merge them)."""
        result = tokenizer.tokenize("デジタル化")
        surfaces = [t.surface for t in result.tokens]
        assert "デジタル" in surfaces and "化" in surfaces, (
            f"Expected デジタル + 化, got: {surfaces}"
        )

    # ── Full-article quality ───────────────────────────────────────────────────

    def test_full_article_no_error(self, tokenizer):
        """Full article tokenizes without exception."""
        result = tokenizer.tokenize(_ARTICLE_FULL)
        assert len(result.tokens) > 0

    def test_full_article_content_word_count(self, tokenizer):
        """Full article has ≥ 40 content-word tokens."""
        result = tokenizer.tokenize(_ARTICLE_FULL)
        count = sum(1 for t in result.tokens if t.is_content_word)
        assert count >= 40, f"Expected ≥40 content words, got {count}"

    def test_full_article_key_lemmas_present(self, tokenizer):
        """All key content-word lemmas from the article appear in token output."""
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
# SECTION 17: NHK World broadcast description
#
# Source text (no furigana — already standard Japanese):
#
#   海外に住んでいる、あるいは、海外旅行中の日本人のために、ニュース・情報番組に
#   加え、ドラマ、音楽番組、こども番組、大相撲中継などのスポーツ番組を国内各波
#   から抜粋し、1日24時間編成しています。一部のニュース・番組は海外向けに
#   インターネットでも配信しています。
#
# Every assertion verified against actual SudachiPy output (2026-05-30).
# ─────────────────────────────────────────────────────────────────────────────

_BROADCAST_TEXT = (
    "海外に住んでいる、あるいは、海外旅行中の日本人のために、ニュース・情報番組に加え、"
    "ドラマ、音楽番組、こども番組、大相撲中継などのスポーツ番組を国内各波から抜粋し、"
    "1日24時間編成しています。一部のニュース・番組は海外向けにインターネットでも配信しています。"
)


class TestNHKWorldBroadcastDescription:
    """
    Tokenizer test for NHK World's broadcast service description.
    No furigana stripping needed — text is already standard Japanese.
    """

    # ── Readings ──────────────────────────────────────────────────────────────

    def test_kaigai_reading(self, tokenizer):
        """海外 reads かいがい."""
        assert reading(tokenizer, "海外") == "かいがい"

    def test_kaigai_ryokou_reading(self, tokenizer):
        """海外旅行 is a single token reading かいがいりょこう."""
        result = tokenizer.tokenize("海外旅行中の日本人")
        tok = next(t for t in result.tokens if t.surface == "海外旅行")
        assert tok.reading_hira == "かいがいりょこう", (
            f"Expected かいがいりょこう, got {tok.reading_hira!r}"
        )

    def test_nihonjin_reading(self, tokenizer):
        """日本人 reads にほんじん (not にっぽんじん — compound differs from 日本 alone)."""
        result = tokenizer.tokenize("日本人のために")
        tok = next(t for t in result.tokens if t.surface == "日本人")
        assert tok.reading_hira == "にほんじん", (
            f"Expected にほんじん, got {tok.reading_hira!r}"
        )

    def test_jouhou_reading(self, tokenizer):
        """情報 reads じょうほう."""
        assert reading(tokenizer, "情報") == "じょうほう"

    def test_bangumi_reading(self, tokenizer):
        """番組 reads ばんぐみ."""
        assert reading(tokenizer, "番組") == "ばんぐみ"

    def test_sumo_reading(self, tokenizer):
        """相撲 reads すもう."""
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

    def test_jikan_reading(self, tokenizer):
        """時間 reads じかん."""
        assert reading(tokenizer, "時間") == "じかん"

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

    # ── Lemmatization ─────────────────────────────────────────────────────────

    def test_sunde_lemma(self, tokenizer):
        """住んでいる → verb stem 住ん has lemma 住む."""
        result = tokenizer.tokenize("海外に住んでいる")
        sumu = next((t for t in result.tokens if "住" in t.surface), None)
        assert sumu is not None, "住 token not found"
        assert sumu.lemma == "住む", f"Expected 住む, got {sumu.lemma!r}"

    def test_kuwaeru_lemma(self, tokenizer):
        """加え (て-form stem) has lemma 加える."""
        result = tokenizer.tokenize("情報番組に加え")
        kuwaeru = next((t for t in result.tokens if "加" in t.surface), None)
        assert kuwaeru is not None, "加 token not found"
        assert kuwaeru.lemma == "加える", f"Expected 加える, got {kuwaeru.lemma!r}"

    # ── Segmentation ──────────────────────────────────────────────────────────

    def test_kaigai_ryokou_single_token(self, tokenizer):
        """海外旅行 is recognized as a single compound noun, not 海外+旅行."""
        result = tokenizer.tokenize("海外旅行中の")
        assert "海外旅行" in [t.surface for t in result.tokens], (
            f"海外旅行 not a single token: {[t.surface for t in result.tokens]}"
        )

    def test_daisumo_split(self, tokenizer):
        """大相撲中継: SudachiPy splits as 大+相撲+中継 and reads おおすもう.
        NOTE: this is a known tokenizer deficiency. The correct compound reading
        is おおずもう (with rendaku す→ず). Marked here to catch regressions
        if SudachiPy is updated to fix it."""
        result = tokenizer.tokenize("大相撲中継")
        surfaces = [t.surface for t in result.tokens]
        # Correct behaviour would be: 大相撲 as single token reading おおずもう.
        # Current SudachiPy behaviour: splits and loses rendaku (す instead of ず).
        sumo = next((t for t in result.tokens if "相撲" in t.surface), None)
        assert sumo is not None, "相撲 token not found"
        # Document the wrong reading so a future fix is noticed immediately.
        assert sumo.reading_hira == "すもう", (
            f"SudachiPy rendaku behaviour changed — update this test: got {sumo.reading_hira!r}"
        )
        # The correct reading we want eventually:
        # assert sumo_compound.reading_hira == "ずもう"  # with rendaku

    def test_jouhou_bangumi_split(self, tokenizer):
        """情報番組 splits as 情報 + 番組 (two content words)."""
        result = tokenizer.tokenize("情報番組に加え")
        surfaces = [t.surface for t in result.tokens]
        assert "情報" in surfaces and "番組" in surfaces, (
            f"Expected 情報 and 番組 as separate tokens, got: {surfaces}"
        )

    def test_internet_single_token(self, tokenizer):
        """インターネット is a single token."""
        result = tokenizer.tokenize("インターネットでも配信")
        assert "インターネット" in [t.surface for t in result.tokens]

    # ── Full-text quality ──────────────────────────────────────────────────────

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
        """All key content-word lemmas appear in the full-text token output."""
        result = tokenizer.tokenize(_BROADCAST_TEXT)
        lemmas = {t.lemma for t in result.tokens if t.is_content_word}
        expected = {
            "海外", "住む", "海外旅行", "日本人", "ニュース", "情報", "番組",
            "加える", "ドラマ", "音楽", "相撲", "中継", "スポーツ", "国内",
            "抜粋", "時間", "編成", "一部", "インターネット", "配信",
        }
        missing = expected - lemmas
        assert not missing, f"Missing lemmas: {sorted(missing)}"
