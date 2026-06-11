"""
Unit tests for the SQLite dictionary importers' pure parser functions:
  - import_cedict_sqlite  (CC-CEDICT, zh-cn)
  - import_kde4_sqlite    (Korean curated list + TSV, ko)
  - import_freedict       (FreeDict dictd format, es/de/fr/it/pt)

Each importer also gets a small end-to-end SQLite round-trip test so we know the
rows land under the right language_code and resolve via DictionaryService.
"""
from __future__ import annotations

import sys
import pathlib
import sqlite3

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "scripts"))

import import_cedict_sqlite as cedict
import import_kde4_sqlite as kde4
import import_freedict as freedict
from src.dictionary import DictionaryService, normalize_language_code


# ── CC-CEDICT parser ────────────────────────────────────────────────────────────

class TestCedictParser:
    def test_parses_basic_entry(self):
        line = "你好 你好 [ni3 hao3] /hello/hi/"
        e = cedict.parse_cedict_line(line)
        assert e is not None
        assert e["simplified"] == "你好"
        assert e["traditional"] == "你好"
        assert e["definitions"] == ["hello", "hi"]

    def test_pinyin_to_diacritics(self):
        assert cedict.pinyin_to_diacritics("ni3 hao3") == "nǐ hǎo"
        assert cedict.pinyin_to_diacritics("zhong1 guo2") == "zhōng guó"
        # neutral tone (5) and toneless pass through as plain vowels
        assert cedict.pinyin_to_diacritics("ma5") == "ma"

    def test_skips_comments_and_blanks(self):
        assert cedict.parse_cedict_line("# comment") is None
        assert cedict.parse_cedict_line("") is None
        assert cedict.parse_cedict_line("garbage line") is None

    def test_skips_pure_variant_entries(self):
        line = "麵 面 [mian4] /variant of 麵|面[mian4]/"
        assert cedict.parse_cedict_line(line) is None

    def test_parse_cedict_collects_valid(self):
        text = "\n".join([
            "# CC-CEDICT",
            "你好 你好 [ni3 hao3] /hello/",
            "中国 中国 [zhong1 guo2] /China/",
            "garbage",
        ])
        entries = cedict.parse_cedict(text)
        assert len(entries) == 2
        assert {e["simplified"] for e in entries} == {"你好", "中国"}

    def test_sqlite_roundtrip(self, tmp_path):
        db = tmp_path / "dict.db"
        text = "你好 你好 [ni3 hao3] /hello/hi/\n中国 中国 [zhong1 guo2] /China/"
        cedict.import_to_sqlite(cedict.parse_cedict(text), db)
        svc = DictionaryService(db_path=str(db))
        res = svc.lookup("你好", language="zh-cn", target_lang="en")
        assert res is not None
        assert "hello" in [d.definition for d in res.definitions]
        # zh / zh-tw normalize to zh-cn
        assert svc.lookup("中国", language="zh", target_lang="en") is not None


# ── KDE4 / Korean parser ─────────────────────────────────────────────────────────

class TestKoreanParser:
    def test_parse_tsv(self):
        text = "사랑\tlove\n친구\tfriend\n"
        pairs = kde4.parse_ko_en_tsv(text)
        assert ("사랑", "love") in pairs
        assert ("친구", "friend") in pairs

    def test_skips_non_hangul_and_phrases(self):
        text = "hello\tworld\n사랑\tlove\n" + "가" * 25 + "\ttoolong\n"
        pairs = kde4.parse_ko_en_tsv(text)
        assert pairs == [("사랑", "love")]

    def test_dedupes(self):
        text = "사랑\tlove\n사랑\taffection\n"
        pairs = kde4.parse_ko_en_tsv(text)
        assert len(pairs) == 1

    def test_has_hangul(self):
        assert kde4.has_hangul("사랑")
        assert not kde4.has_hangul("love")

    def test_seed_list_nonempty(self):
        assert len(kde4.SEED_ENTRIES) >= 50

    def test_sqlite_roundtrip_seed(self, tmp_path):
        db = tmp_path / "dict.db"
        kde4.import_to_sqlite(list(kde4.SEED_ENTRIES), db)
        svc = DictionaryService(db_path=str(db))
        res = svc.lookup("사랑", language="ko", target_lang="en")
        assert res is not None
        assert "love" in [d.definition for d in res.definitions]


# ── FreeDict (dictd) parser ──────────────────────────────────────────────────────

class TestFreeDictParser:
    def test_decode_b64_int(self):
        # 'A' = 0, 'B' = 1, 'BA' = 64
        assert freedict.decode_b64_int("A") == 0
        assert freedict.decode_b64_int("B") == 1
        assert freedict.decode_b64_int("BA") == 64

    def test_parse_index_skips_metadata(self):
        index = (
            "00-database-info\tA\tB\n"
            "haus\tBA\tC\n"
        ).encode("utf-8")
        rows = freedict.parse_index(index)
        assert len(rows) == 1
        assert rows[0][0] == "haus"
        assert rows[0][1] == freedict.decode_b64_int("BA")

    def test_extract_glosses_simple(self):
        body = "Buch /bˈuːx/ <neut, n, sg>\nbook <n>\n   Synonym: {Schmöker}"
        assert freedict.extract_glosses(body) == ["book"]

    def test_extract_glosses_multiple_senses(self):
        body = "Haus /hˈaʊs/ <neut, n, sg>\n [adm.] establishment <n>, institution <n>"
        assert freedict.extract_glosses(body) == ["establishment", "institution"]

    def test_extract_glosses_skips_example_lines(self):
        body = (
            "Buch /bˈuːx/ <neut, n, sg>\n"
            "book <n>\n"
            '      "ein Buch lesen"  - read a book\n'
            " see: {Bücher}"
        )
        assert freedict.extract_glosses(body) == ["book"]

    def test_extract_glosses_empty_when_no_translation(self):
        body = "Haus /hˈaʊs/ <neut>\n   Synonym: {Heim}"
        assert freedict.extract_glosses(body) == []

    def test_parse_dictd_roundtrip(self):
        # Build a tiny dictd dict+index in-memory and parse it.
        body1 = "Haus /h/ <n>\nhouse <n>\n"
        body2 = "Liebe /l/ <n>\nlove <n>\n"
        dict_bytes = (body1 + body2).encode("utf-8")
        off2 = len(body1.encode("utf-8"))

        def enc(n: int) -> str:
            if n == 0:
                return "A"
            alpha = freedict._B64_ALPHABET
            out = ""
            while n:
                out = alpha[n % 64] + out
                n //= 64
            return out

        index = (
            f"haus\t{enc(0)}\t{enc(len(body1.encode('utf-8')))}\n"
            f"liebe\t{enc(off2)}\t{enc(len(body2.encode('utf-8')))}\n"
        ).encode("utf-8")
        entries = freedict.parse_dictd(index, dict_bytes)
        by = {e["headword"]: e["glosses"] for e in entries}
        assert by["haus"] == ["house"]
        assert by["liebe"] == ["love"]

    def test_sqlite_roundtrip(self, tmp_path):
        db = tmp_path / "dict.db"
        entries = [
            {"headword": "Haus", "glosses": ["house", "home"]},
            {"headword": "Liebe", "glosses": ["love"]},
        ]
        freedict.import_to_sqlite(entries, "de", db)
        svc = DictionaryService(db_path=str(db))
        # stored lowercased; capitalized lookup resolves via case-insensitive fallback
        res = svc.lookup("Haus", language="de", target_lang="en")
        assert res is not None
        assert "house" in [d.definition for d in res.definitions]
        assert svc.lookup("liebe", language="de", target_lang="en") is not None


class TestLanguageNormalization:
    def test_zh_variants_collapse(self):
        assert normalize_language_code("zh") == "zh-cn"
        assert normalize_language_code("zh-tw") == "zh-cn"
        assert normalize_language_code("zh-cn") == "zh-cn"

    def test_other_langs_passthrough(self):
        for l in ("ja", "ko", "en", "es", "de", "fr", "it", "pt", "vi"):
            assert normalize_language_code(l) == l
