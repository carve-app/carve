/*!
 * Carve Japanese WASM Tokenizer
 *
 * A lightweight dictionary-based Japanese tokenizer compiled to WebAssembly
 * for in-browser use. Designed for privacy (no server calls) and speed (<5ms
 * for typical subtitle lines).
 *
 * Architecture:
 *   - Dictionary: compact binary format loaded once from CDN, cached in SW
 *   - Tokenization: maximum matching against dictionary entries
 *   - Readings: stored per-entry in the dictionary
 *   - POS: stored per-entry
 *
 * This is a Phase 0 scaffold. The full dictionary integration (parsing the
 * compact binary dict format built from JMdict) is completed in Phase 1.
 * For Phase 0, the correctness contract is verified by the Python NLP service;
 * the WASM module exposes the same API shape.
 *
 * Production dictionary build pipeline:
 *   python services/nlp/scripts/build_wasm_dict.py → ja_dict.bin (compact)
 *   wasm-pack build --target web --release
 */

use std::sync::OnceLock;
use wasm_bindgen::prelude::*;
use serde::{Deserialize, Serialize};

// Install panic hook for better error messages in browser console
#[cfg(feature = "console_error_panic_hook")]
extern crate console_error_panic_hook;

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct Token {
    /// Text as it appears in the source
    pub surface: String,
    /// Dictionary form (lemma)
    pub lemma: String,
    /// Reading in hiragana
    pub reading_hira: String,
    /// Reading in katakana (canonical form from dictionary)
    pub reading_kata: String,
    /// Part of speech (e.g., "動詞", "名詞")
    pub pos: String,
    /// Whether this is a content word (not a particle/auxiliary)
    pub is_content_word: bool,
    /// Frequency rank (1 = most frequent), None if unknown
    pub frequency_rank: Option<u32>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FuriganaSpan {
    /// Text segment
    pub text: String,
    /// Hiragana reading for this segment (empty string for kana segments)
    pub reading: String,
}

/// Global tokenizer state, initialized once via `init()`.
/// OnceLock is safe in WASM (single-threaded) and avoids `static mut`.
static TOKENIZER: OnceLock<WasmTokenizer> = OnceLock::new();

struct WasmTokenizer {
    /// Loaded dictionary entries: (lemma, reading_kata, pos, frequency_rank)
    /// In Phase 1, this is populated from the compact binary dict.
    /// For Phase 0 scaffold, this is a minimal built-in table of common words.
    entries: Vec<DictEntry>,
}

#[derive(Clone)]
struct DictEntry {
    surface: String,
    lemma: String,
    reading_kata: String,
    pos: String,
    is_content_word: bool,
    frequency_rank: Option<u32>,
}

impl WasmTokenizer {
    /// Build with minimal built-in vocabulary for Phase 0 scaffold.
    /// Phase 1 replaces this with dictionary binary deserialization.
    fn from_builtin() -> Self {
        // Minimal seed entries for the scaffold (verified readings)
        let entries = vec![
            DictEntry::new("食べる",   "食べる",   "タベル",       "動詞", true,  Some(287)),
            DictEntry::new("日本語",   "日本語",   "ニホンゴ",     "名詞", true,  Some(412)),
            DictEntry::new("勉強",     "勉強",     "ベンキョウ",   "名詞", true,  Some(1203)),
            DictEntry::new("入る",     "入る",     "ハイル",       "動詞", true,  Some(180)),
            DictEntry::new("見る",     "見る",     "ミル",         "動詞", true,  Some(143)),
            DictEntry::new("する",     "する",     "スル",         "動詞", true,  Some(3)),
            DictEntry::new("ある",     "ある",     "アル",         "動詞", true,  Some(8)),
            DictEntry::new("いる",     "いる",     "イル",         "動詞", true,  Some(12)),
            DictEntry::new("来る",     "来る",     "クル",         "動詞", true,  Some(156)),
            DictEntry::new("ます",     "ます",     "マス",         "助動詞", false, Some(5)),
            DictEntry::new("ない",     "ない",     "ナイ",         "助動詞", false, Some(9)),
            DictEntry::new("て",       "て",       "テ",           "助詞", false, Some(2)),
            DictEntry::new("を",       "を",       "ヲ",           "助詞", false, Some(6)),
            DictEntry::new("は",       "は",       "ハ",           "助詞", false, Some(1)),
            DictEntry::new("が",       "が",       "ガ",           "助詞", false, Some(4)),
            DictEntry::new("に",       "に",       "ニ",           "助詞", false, Some(7)),
            DictEntry::new("の",       "の",       "ノ",           "助詞", false, Some(10)),
            DictEntry::new("東京都",   "東京都",   "トウキョウト", "名詞", true,  Some(850)),
            DictEntry::new("生き物",   "生き物",   "イキモノ",     "名詞", true,  Some(4200)),
        ];
        WasmTokenizer { entries }
    }

    /// Tokenize `text` using maximum-matching against the dictionary.
    ///
    /// Falls back to single-character tokens for unknown text. This is a
    /// simplified implementation; Phase 1 uses a full lattice-based decoder.
    fn tokenize(&self, text: &str) -> Vec<Token> {
        let chars: Vec<char> = text.chars().collect();
        let mut tokens = Vec::new();
        let mut pos = 0;

        while pos < chars.len() {
            // Try longest match first (greedy maximum matching)
            let mut matched = false;
            let remaining: String = chars[pos..].iter().collect();

            // Try decreasing lengths
            let max_len = remaining.chars().count().min(10);
            for len in (1..=max_len).rev() {
                let candidate: String = remaining.chars().take(len).collect();
                if let Some(entry) = self.lookup_entry(&candidate) {
                    tokens.push(Token {
                        surface: candidate,
                        lemma: entry.lemma.clone(),
                        reading_hira: kata_to_hira(&entry.reading_kata),
                        reading_kata: entry.reading_kata.clone(),
                        pos: entry.pos.clone(),
                        is_content_word: entry.is_content_word,
                        frequency_rank: entry.frequency_rank,
                    });
                    pos += len;
                    matched = true;
                    break;
                }
            }

            if !matched {
                // Unknown character — emit as-is (treated as unknown content)
                let ch: String = chars[pos..pos+1].iter().collect();
                let reading = if is_kana_char(chars[pos]) { ch.clone() } else { ch.clone() };
                tokens.push(Token {
                    surface: ch.clone(),
                    lemma: ch.clone(),
                    reading_hira: kata_to_hira(&reading),
                    reading_kata: hira_to_kata(&reading),
                    pos: "名詞".to_string(),
                    is_content_word: !is_punctuation(chars[pos]),
                    frequency_rank: None,
                });
                pos += 1;
            }
        }

        tokens
    }

    fn lookup_entry(&self, surface: &str) -> Option<&DictEntry> {
        self.entries.iter().find(|e| e.surface == surface)
    }
}

impl DictEntry {
    fn new(
        surface: &str,
        lemma: &str,
        reading_kata: &str,
        pos: &str,
        is_content_word: bool,
        frequency_rank: Option<u32>,
    ) -> Self {
        DictEntry {
            surface: surface.to_string(),
            lemma: lemma.to_string(),
            reading_kata: reading_kata.to_string(),
            pos: pos.to_string(),
            is_content_word,
            frequency_rank,
        }
    }
}

// ── Character utilities ───────────────────────────────────────────────────────

fn is_kana_char(c: char) -> bool {
    let cp = c as u32;
    (0x3040..=0x309F).contains(&cp) ||  // hiragana
    (0x30A0..=0x30FF).contains(&cp) ||  // katakana
    cp == 0x30FC                          // ー (prolonged sound mark)
}

fn is_punctuation(c: char) -> bool {
    let cp = c as u32;
    matches!(cp,
        0x3000..=0x303F |  // CJK punctuation
        0xFF00..=0xFFEF |  // Halfwidth/fullwidth forms
        0x0020..=0x002F |  // ASCII punctuation
        0x003A..=0x0040 |
        0x005B..=0x0060 |
        0x007B..=0x007E
    )
}

/// Convert katakana string to hiragana.
pub fn kata_to_hira(text: &str) -> String {
    text.chars().map(|c| {
        let cp = c as u32;
        if (0x30A1..=0x30F6).contains(&cp) {
            char::from_u32(cp - 0x60).unwrap_or(c)
        } else {
            c
        }
    }).collect()
}

/// Convert hiragana string to katakana.
pub fn hira_to_kata(text: &str) -> String {
    text.chars().map(|c| {
        let cp = c as u32;
        if (0x3041..=0x3096).contains(&cp) {
            char::from_u32(cp + 0x60).unwrap_or(c)
        } else {
            c
        }
    }).collect()
}

// ── WASM-exported API ─────────────────────────────────────────────────────────

/// Initialize the tokenizer with optional dictionary bytes.
///
/// Call once before tokenizing. When `dict_bytes` is empty, falls back to
/// the built-in minimal vocabulary (Phase 0 scaffold mode).
///
/// Phase 1: `dict_bytes` will contain the compact binary dict built from JMdict.
#[wasm_bindgen]
pub fn init(dict_bytes: &[u8]) {
    #[cfg(feature = "console_error_panic_hook")]
    console_error_panic_hook::set_once();

    let tokenizer = if dict_bytes.is_empty() {
        WasmTokenizer::from_builtin()
    } else {
        // Phase 1: deserialize compact dict format from dict_bytes
        // For now, ignore dict_bytes and use builtin
        WasmTokenizer::from_builtin()
    };

    // OnceLock: silently ignore if already initialized (idempotent)
    let _ = TOKENIZER.set(tokenizer);
}

/// Tokenize Japanese text.
///
/// Returns JSON string of `Token[]`.
/// The tokenizer must be initialized with `init()` first.
#[wasm_bindgen]
pub fn tokenize(text: &str) -> String {
    let t = TOKENIZER.get();
    match t {
        None => "[]".to_string(),
        Some(tok) => {
            let tokens = tok.tokenize(text);
            serde_json::to_string(&tokens).unwrap_or_else(|_| "[]".to_string())
        }
    }
}

/// Look up a single word by exact surface form.
///
/// Returns JSON string of `Token | null`.
#[wasm_bindgen]
pub fn lookup(surface: &str) -> String {
    let t = TOKENIZER.get();
    match t {
        None => "null".to_string(),
        Some(tok) => {
            let tokens = tok.tokenize(surface);
            match tokens.first() {
                Some(token) => serde_json::to_string(token).unwrap_or("null".to_string()),
                None => "null".to_string(),
            }
        }
    }
}

/// Returns the full hiragana reading of text (all morphemes concatenated).
/// Convenience function for content scripts.
#[wasm_bindgen]
pub fn reading_of(text: &str) -> String {
    let t = TOKENIZER.get();
    match t {
        None => String::new(),
        Some(tok) => tok.tokenize(text)
            .iter()
            .map(|t| t.reading_hira.as_str())
            .collect(),
    }
}

// ── Unit tests (run with `cargo test`, not in WASM) ──────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn get_tokenizer() -> WasmTokenizer {
        WasmTokenizer::from_builtin()
    }

    #[test]
    fn test_kata_to_hira() {
        assert_eq!(kata_to_hira("タベル"), "たべる");
        assert_eq!(kata_to_hira("ニホンゴ"), "にほんご");
        assert_eq!(kata_to_hira("ゴザイマセン"), "ございません");
        assert_eq!(kata_to_hira("abc"), "abc");
    }

    #[test]
    fn test_hira_to_kata() {
        assert_eq!(hira_to_kata("たべる"), "タベル");
        assert_eq!(hira_to_kata("にほんご"), "ニホンゴ");
    }

    #[test]
    fn test_tokenize_known_word() {
        let tok = get_tokenizer();
        let tokens = tok.tokenize("食べる");
        assert!(!tokens.is_empty());
        assert_eq!(tokens[0].lemma, "食べる");
        assert_eq!(tokens[0].reading_hira, "たべる");
        assert!(tokens[0].is_content_word);
    }

    #[test]
    fn test_particle_not_content_word() {
        let tok = get_tokenizer();
        let tokens = tok.tokenize("は");
        assert!(!tokens.is_empty());
        assert!(!tokens[0].is_content_word);
    }

    #[test]
    fn test_tokyo_to_reading() {
        let tok = get_tokenizer();
        let tokens = tok.tokenize("東京都");
        let reading: String = tokens.iter().map(|t| t.reading_hira.as_str()).collect();
        assert_eq!(reading, "とうきょうと");
    }

    #[test]
    fn test_ikimono_reading() {
        let tok = get_tokenizer();
        let tokens = tok.tokenize("生き物");
        let reading: String = tokens.iter().map(|t| t.reading_hira.as_str()).collect();
        assert_eq!(reading, "いきもの");
    }
}
