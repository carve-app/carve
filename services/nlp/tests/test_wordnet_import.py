"""
Tests for the WordNet English-dictionary importer (monolingual en→en lookup,
for intermediate+ English learners).
"""
from __future__ import annotations

import os
import sqlite3
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from scripts.import_wordnet import parse_wordnet_line, import_wordnet  # noqa: E402
from carve_nlp.dictionary import DictionaryService  # noqa: E402


class TestParseWordnetLine:
    def test_noun_synset(self):
        line = "00001740 03 n 01 entity 0 003 ~ 00001930 n 0000 | that which is perceived"
        lemmas, pos, definition = parse_wordnet_line(line)
        assert lemmas == ["entity"]
        assert pos == "noun"
        assert definition == "that which is perceived"

    def test_strips_quoted_example(self):
        line = '02084071 05 n 01 dog 0 000 | a domesticated mammal  "the dog barked"'
        _, _, definition = parse_wordnet_line(line)
        assert definition == "a domesticated mammal"
        assert '"' not in definition

    def test_multiword_lemma_underscores_to_spaces(self):
        line = "00002098 00 a 02 ab_oral 0 hot_dog 0 000 | a definition here"
        lemmas, pos, _ = parse_wordnet_line(line)
        assert lemmas == ["ab oral", "hot dog"]
        assert pos == "adj"

    def test_verb_pos(self):
        line = "02614387 33 v 01 run 0 000 | move fast"
        _, pos, _ = parse_wordnet_line(line)
        assert pos == "verb"

    def test_licence_header_skipped(self):
        assert parse_wordnet_line("  1 This software and database ...") is None

    def test_no_gloss_skipped(self):
        assert parse_wordnet_line("00001740 03 n 01 entity 0 003") is None

    def test_empty_line(self):
        assert parse_wordnet_line("") is None


class TestEnglishDictionaryEndToEnd:
    def _build_db(self, tmp_path):
        wndir = tmp_path / "wn"
        wndir.mkdir()
        (wndir / "data.noun").write_text(
            "00001740 03 n 01 entity 0 000 | that which has distinct existence\n"
            '02084071 05 n 01 dog 0 000 | a domesticated carnivorous mammal  "the dog barked"\n',
            encoding="utf-8",
        )
        (wndir / "data.verb").write_text(
            "02614387 33 v 01 run 0 000 | move fast by using legs\n",
            encoding="utf-8",
        )
        db = tmp_path / "dictionary.db"
        sqlite3.connect(str(db)).close()
        import_wordnet(db, wndir)
        return db

    def test_lookup_english_word(self, tmp_path):
        svc = DictionaryService(db_path=str(self._build_db(tmp_path)))
        dog = svc.lookup("dog", language="en")
        assert dog is not None
        assert dog.definitions[0].definition.startswith("a domesticated")

    def test_pos_preserved(self, tmp_path):
        svc = DictionaryService(db_path=str(self._build_db(tmp_path)))
        run = svc.lookup("run", language="en")
        assert run is not None
        assert run.definitions[0].part_of_speech == "verb"

    def test_unknown_word_returns_none(self, tmp_path):
        svc = DictionaryService(db_path=str(self._build_db(tmp_path)))
        assert svc.lookup("zxqweqwe", language="en") is None

    def test_english_does_not_match_japanese_query(self, tmp_path):
        # An english word must not resolve when queried as ja (language isolation).
        svc = DictionaryService(db_path=str(self._build_db(tmp_path)))
        assert svc.lookup("dog", language="ja") is None
