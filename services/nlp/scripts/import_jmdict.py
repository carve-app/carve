"""
Import JMdict (Japanese-English dictionary) XML into SQLite.

Usage:
    python scripts/import_jmdict.py [--dict data/JMdict_e] [--db data/dictionary.db]

JMdict XML structure (simplified):
  <entry>
    <ent_seq>1000220</ent_seq>
    <k_ele><keb>食べる</keb><ke_pri>ichi1</ke_pri></k_ele>
    <r_ele><reb>たべる</reb><re_pri>ichi1</re_pri></r_ele>
    <sense>
      <pos>&v1;</pos>         <!-- verb, ichidan -->
      <gloss>to eat</gloss>
    </sense>
  </entry>

Priority tags used for frequency ranking:
  news1/news2 → newspaper frequency
  ichi1/ichi2 → Ichimango goi frequency list
  spec1/spec2 → special words
  gai1/gai2   → loanwords

Frequency rank is assigned as:
  rank 1-500:   news1 ∩ ichi1
  rank 501-2000: news1 ∪ ichi1
  rank 2001-5000: news2 ∪ ichi2
  rank 5001+:   everything else
"""

from __future__ import annotations

import argparse
import re
import sqlite3
import sys
import time
import xml.etree.ElementTree as ET
from pathlib import Path
from collections import defaultdict


SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS words (
    id              TEXT PRIMARY KEY,           -- UUID-like: JMdict ent_seq
    language_code   TEXT NOT NULL DEFAULT 'ja',
    lemma           TEXT NOT NULL,
    reading         TEXT,
    frequency_rank  INTEGER,
    jlpt_level      TEXT,
    pos_primary     TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS words_lemma_reading
    ON words(lemma, reading, language_code);
CREATE INDEX IF NOT EXISTS words_lemma ON words(lemma, language_code);
CREATE INDEX IF NOT EXISTS words_frequency ON words(language_code, frequency_rank);

CREATE TABLE IF NOT EXISTS word_definitions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id         TEXT NOT NULL REFERENCES words(id),
    target_language TEXT NOT NULL DEFAULT 'en',
    sense_index     INTEGER NOT NULL DEFAULT 0,
    definition      TEXT NOT NULL,
    part_of_speech  TEXT,
    tags            TEXT,                       -- comma-separated
    source          TEXT NOT NULL DEFAULT 'jmdict',
    confidence      REAL NOT NULL DEFAULT 1.0
);

CREATE INDEX IF NOT EXISTS wdef_word_id ON word_definitions(word_id, target_language);

CREATE TABLE IF NOT EXISTS example_sentences (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id         TEXT NOT NULL REFERENCES words(id),
    text            TEXT NOT NULL,
    translation     TEXT,
    translation_lang TEXT NOT NULL DEFAULT 'en',
    source          TEXT NOT NULL DEFAULT 'jmdict',
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
"""

# JMdict entity expansions (abbreviated)
POS_MAP = {
    "v1":      "verb (ichidan)",
    "v5u":     "verb (godan -u)",
    "v5k":     "verb (godan -ku)",
    "v5g":     "verb (godan -gu)",
    "v5s":     "verb (godan -su)",
    "v5t":     "verb (godan -tsu)",
    "v5n":     "verb (godan -nu)",
    "v5b":     "verb (godan -bu)",
    "v5m":     "verb (godan -mu)",
    "v5r":     "verb (godan -ru)",
    "v5r-i":   "verb (godan -ru, irreg.)",
    "vs":      "verb (suru)",
    "vs-i":    "verb (suru, compound)",
    "vs-s":    "verb (suru, special)",
    "vk":      "verb (kuru)",
    "adj-i":   "i-adjective",
    "adj-na":  "na-adjective",
    "adj-no":  "no-adjective",
    "n":       "noun",
    "n-adv":   "adverbial noun",
    "n-suf":   "noun (suffix)",
    "n-pref":  "noun (prefix)",
    "n-t":     "noun (temporal)",
    "adv":     "adverb",
    "adv-to":  "adverb (to)",
    "prt":     "particle",
    "conj":    "conjunction",
    "int":     "interjection",
    "pref":    "prefix",
    "suf":     "suffix",
    "exp":     "expression",
    "aux-v":   "auxiliary verb",
    "aux-adj": "auxiliary adjective",
    "cop":     "copula",
    "ctr":     "counter",
    "num":     "numeral",
    "pn":      "pronoun",
    "unc":     "unclassified",
}

# Priority tag → approximate frequency tier
HIGH_PRIORITY  = {"news1", "ichi1", "spec1", "gai1"}
MED_PRIORITY   = {"news2", "ichi2", "spec2", "gai2"}


def _parse_pos_entity(raw: str) -> str:
    """Extract POS code from JMdict entity reference like &v1; → v1."""
    m = re.match(r"&([^;]+);", raw)
    if m:
        code = m.group(1)
        return POS_MAP.get(code, code)
    return raw


def _assign_frequency_rank(entries: list[dict]) -> None:
    """
    Assign frequency ranks to entries in-place based on priority tags.
    Rank 1 = most frequent.
    """
    high = [e for e in entries if HIGH_PRIORITY & e["priorities"]]
    med  = [e for e in entries if (MED_PRIORITY & e["priorities"]) and not (HIGH_PRIORITY & e["priorities"])]
    low  = [e for e in entries if not (HIGH_PRIORITY | MED_PRIORITY) & e["priorities"]]

    rank = 1
    for e in high:
        e["frequency_rank"] = rank
        rank += 1
    for e in med:
        e["frequency_rank"] = rank
        rank += 1
    for e in low:
        e["frequency_rank"] = None  # unranked


def parse_jmdict(path: Path) -> list[dict]:
    """
    Parse JMdict XML and return a list of entry dicts.

    Each entry dict:
      id, kanji_forms, kana_forms, priorities, senses, jlpt_level
    """
    print(f"Parsing {path} ...", flush=True)

    # JMdict uses an internal DTD with many entities (&v1;, &n;, etc.).
    # Strategy: read the raw bytes, strip the DOCTYPE block entirely, then
    # replace all remaining &entity; references with [entity] text so the
    # XML parser sees only well-formed content.
    raw = path.read_bytes()
    text = raw.decode("utf-8", errors="replace")

    # Remove the entire DOCTYPE declaration (which spans many lines)
    text = re.sub(r"<!DOCTYPE\s+\w+\s*\[.*?\]>", "", text, flags=re.DOTALL)
    # Replace remaining entity references with bracketed text
    text = re.sub(r"&([A-Za-z0-9_\-]+);", r"[\1]", text)
    # Remove any residual XML declarations that could confuse fromstring
    text = re.sub(r"<\?xml[^?]*\?>", "", text)

    root = ET.fromstring(f"<root>{text}</root>")
    entries = []

    # When wrapped in <root>, we need to find JMdict first if present, else root
    jmdict_el = root.find("JMdict") or root
    for entry in jmdict_el.iter("entry"):
        seq = entry.findtext("ent_seq", "0")

        kanji_forms = [k.text for k in entry.findall("k_ele/keb") if k.text]
        kana_forms  = [r.text for r in entry.findall("r_ele/reb") if r.text]

        # Collect priority tags from both k_ele and r_ele
        priorities: set[str] = set()
        for pri in entry.findall("k_ele/ke_pri"):
            if pri.text:
                priorities.add(pri.text.strip())
        for pri in entry.findall("r_ele/re_pri"):
            if pri.text:
                priorities.add(pri.text.strip())

        senses = []
        for i, sense in enumerate(entry.findall("sense")):
            pos_list = [
                POS_MAP.get(p.text.strip("[]"), p.text.strip("[]"))
                for p in sense.findall("pos") if p.text
            ]
            glosses = [g.text for g in sense.findall("gloss") if g.text]
            misc_tags = [m.text.strip("[]") for m in sense.findall("misc") if m.text]
            field_tags = [f.text.strip("[]") for f in sense.findall("field") if f.text]
            all_tags = misc_tags + field_tags

            if glosses:
                senses.append({
                    "index": i,
                    "pos": "; ".join(pos_list),
                    "definitions": glosses,
                    "tags": all_tags,
                })

        entries.append({
            "id": seq,
            "kanji_forms": kanji_forms,
            "kana_forms": kana_forms,
            "priorities": priorities,
            "senses": senses,
            "jlpt_level": None,
            "frequency_rank": None,
        })

    print(f"Parsed {len(entries)} entries", flush=True)
    return entries


def import_to_sqlite(entries: list[dict], db_path: Path) -> None:
    print(f"Importing to {db_path} ...", flush=True)
    _assign_frequency_rank(entries)

    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_SQL)

    words_batch = []
    defs_batch = []

    for entry in entries:
        # Each kanji form × each kana form = one words row
        # When no kanji form exists, use the kana form as the lemma
        kanji_forms = entry["kanji_forms"] or [""]
        kana_forms  = entry["kana_forms"]  or [""]

        for ki, kanji in enumerate(kanji_forms):
            lemma = kanji if kanji else kana_forms[0]
            reading = kana_forms[0] if kanji else None
            row_id = f"{entry['id']}-{ki}"

            primary_pos = entry["senses"][0]["pos"] if entry["senses"] else None
            words_batch.append((
                row_id,
                lemma,
                reading,
                entry["frequency_rank"],
                entry["jlpt_level"],
                primary_pos,
            ))

            for sense in entry["senses"]:
                for defn in sense["definitions"]:
                    defs_batch.append((
                        row_id,
                        sense["index"],
                        defn,
                        sense["pos"],
                        ",".join(sense["tags"]),
                    ))

        # Also insert kana-only entries for reading-form lookup
        if entry["kanji_forms"]:
            for ki, kana in enumerate(kana_forms):
                row_id = f"{entry['id']}-k{ki}"
                words_batch.append((
                    row_id,
                    kana,           # lemma = kana form
                    None,           # no separate reading
                    entry["frequency_rank"],
                    entry["jlpt_level"],
                    entry["senses"][0]["pos"] if entry["senses"] else None,
                ))
                for sense in entry["senses"]:
                    for defn in sense["definitions"]:
                        defs_batch.append((
                            row_id,
                            sense["index"],
                            defn,
                            sense["pos"],
                            ",".join(sense["tags"]),
                        ))

    # Batch insert
    conn.executemany(
        """
        INSERT OR IGNORE INTO words
            (id, lemma, reading, frequency_rank, jlpt_level, pos_primary)
        VALUES (?, ?, ?, ?, ?, ?)
        """,
        words_batch,
    )
    conn.executemany(
        """
        INSERT INTO word_definitions
            (word_id, sense_index, definition, part_of_speech, tags)
        VALUES (?, ?, ?, ?, ?)
        """,
        defs_batch,
    )
    conn.commit()
    conn.close()

    print(f"Inserted {len(words_batch)} word rows, {len(defs_batch)} definition rows", flush=True)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import JMdict into SQLite")
    parser.add_argument("--dict", default="data/JMdict_e", help="Path to JMdict XML file")
    parser.add_argument("--db",   default="data/dictionary.db", help="Output SQLite path")
    args = parser.parse_args()

    dict_path = Path(args.dict)
    db_path   = Path(args.db)

    if not dict_path.exists():
        print(f"ERROR: {dict_path} not found. Download with:", file=sys.stderr)
        print("  curl -L http://ftp.edrdg.org/pub/Nihongo/JMdict_e.gz | gzip -d > data/JMdict_e", file=sys.stderr)
        sys.exit(1)

    t0 = time.time()
    entries = parse_jmdict(dict_path)
    import_to_sqlite(entries, db_path)
    print(f"Done in {time.time() - t0:.1f}s", flush=True)


if __name__ == "__main__":
    main()
