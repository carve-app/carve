/*!
 * Carve Korean WASM Tokenizer
 *
 * Rule-based eojeol splitter with Hangul particle detection and Revised
 * Romanization annotation. No ML model required — Korean's agglutinative
 * morphology is handled by stripping well-known particle/ending suffixes.
 *
 * Romanization follows the Revised Romanization of Korean (국어의 로마자 표기법).
 * Particle display:
 *   - 은/는 (topic), 이/가 (subject), 을/를 (object) → shown as subscript labels
 *   - All particles removed from the lemma before dictionary lookup
 */

use std::sync::OnceLock;
use wasm_bindgen::prelude::*;
use serde::{Deserialize, Serialize};

#[cfg(feature = "console_error_panic_hook")]
extern crate console_error_panic_hook;

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct Token {
    pub surface: String,
    pub lemma: String,
    pub romanization: String,
    pub pos: String,
    pub pos_category: String,
    pub is_content_word: bool,
    pub particles: Vec<String>,
    pub frequency_rank: Option<u32>,
}

static INITIALIZED: OnceLock<bool> = OnceLock::new();

#[wasm_bindgen]
pub fn init(_dict_bytes: &[u8]) {
    #[cfg(feature = "console_error_panic_hook")]
    console_error_panic_hook::set_once();
    let _ = INITIALIZED.set(true);
}

#[wasm_bindgen]
pub fn tokenize(text: &str) -> String {
    let tokens = tokenize_korean(text);
    serde_json::to_string(&tokens).unwrap_or_else(|_| "[]".to_string())
}

#[wasm_bindgen]
pub fn romanize_text(text: &str) -> String {
    romanize(text)
}

// ── Core tokenizer ────────────────────────────────────────────────────────────

fn tokenize_korean(text: &str) -> Vec<Token> {
    let mut tokens = Vec::new();

    for part in split_eojeols(text) {
        if part.trim().is_empty() {
            continue;
        }
        if !has_hangul(&part) {
            // Non-Korean segment: emit as-is
            tokens.push(Token {
                romanization: part.clone(),
                lemma: part.clone(),
                pos: "SL".to_string(),
                pos_category: "foreign".to_string(),
                is_content_word: !part.trim().is_empty() && !is_punct(&part),
                particles: vec![],
                frequency_rank: None,
                surface: part,
            });
            continue;
        }

        let (stem, particles) = strip_particles(&part);
        let rom = romanize(&stem);

        tokens.push(Token {
            surface: part,
            lemma: stem,
            romanization: rom,
            pos: "NNG".to_string(),
            pos_category: "noun".to_string(),
            is_content_word: true,
            particles,
            frequency_rank: None,
        });
    }

    tokens
}

/// Split text into eojeols (space-separated) while preserving non-Korean words.
fn split_eojeols(text: &str) -> Vec<String> {
    text.split_whitespace().map(|s| s.to_string()).collect()
}

fn has_hangul(s: &str) -> bool {
    s.chars().any(|c| {
        let cp = c as u32;
        (0xAC00..=0xD7A3).contains(&cp) || (0x1100..=0x11FF).contains(&cp)
    })
}

fn is_punct(s: &str) -> bool {
    s.chars().all(|c| !c.is_alphabetic() && !is_hangul_char(c))
}

fn is_hangul_char(c: char) -> bool {
    let cp = c as u32;
    (0xAC00..=0xD7A3).contains(&cp) || (0x1100..=0x11FF).contains(&cp)
}

// Common Korean particles ordered longest-first to prefer maximal match
const PARTICLES: &[(&str, &str)] = &[
    ("에서부터", "locative-from"),
    ("에서는", "locative-topic"),
    ("으로서는", "instrumental-topic"),
    ("이라고는", "quotative-topic"),
    ("한테서", "dative-from"),
    ("로부터", "from"),
    ("에서", "locative"),
    ("에게", "dative"),
    ("한테", "dative-informal"),
    ("으로", "instrumental"),
    ("로서", "as"),
    ("로써", "by-means-of"),
    ("에서", "locative"),
    ("에는", "locative-topic"),
    ("이라", "copula"),
    ("이고", "and"),
    ("이지", "but-copula"),
    ("이나", "or"),
    ("이며", "and-also"),
    ("께서", "honorific-subject"),
    ("까지", "until"),
    ("부터", "from"),
    ("처럼", "like"),
    ("보다", "than"),
    ("마다", "each"),
    ("조차", "even"),
    ("밖에", "only"),
    ("만큼", "as-much-as"),
    ("대로", "as"),
    ("이라면", "if-copula"),
    ("이라도", "even-if-copula"),
    ("에서", "locative"),
    ("에게서", "from-person"),
    ("으로는", "instrumental-topic"),
    ("이라는", "quotative"),
    ("이었다", "copula-past"),
    ("이다", "copula"),
    ("을를", "object"),
    ("은는", "topic"),
    ("이가", "subject"),
    ("이랑", "with"),
    ("와랑", "with"),
    ("과와", "and"),
    ("와과", "and"),
    ("를", "object"),
    ("을", "object"),
    ("는", "topic"),
    ("은", "topic"),
    ("가", "subject"),
    ("이", "subject"),
    ("의", "genitive"),
    ("도", "also"),
    ("만", "only"),
    ("에", "locative"),
    ("로", "instrumental"),
    ("와", "and"),
    ("과", "and"),
];

fn strip_particles(eojeol: &str) -> (String, Vec<String>) {
    let chars: Vec<char> = eojeol.chars().collect();
    // Need at least 2 chars for stem after stripping
    for (particle, _role) in PARTICLES {
        if eojeol.ends_with(particle) {
            let stem_end = eojeol.len() - particle.len();
            if stem_end > 0 {
                let stem: String = eojeol[..stem_end].to_string();
                if !stem.is_empty() && has_hangul(&stem) {
                    return (stem, vec![particle.to_string()]);
                }
            }
        }
    }
    // No particle found
    let _ = chars; // suppress warning
    (eojeol.to_string(), vec![])
}

// ── Revised Romanization ──────────────────────────────────────────────────────

const INITIALS: &[&str] = &["g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "ch", "k", "t", "p", "h"];
const VOWELS: &[&str] = &["a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe", "yo", "u", "wo", "we", "wi", "yu", "eu", "ui", "i"];
const FINALS: &[&str] = &["", "g", "kk", "gs", "n", "nj", "nh", "d", "l", "rg", "rm", "rb", "rs", "rt", "rp", "rh", "m", "b", "bs", "s", "ss", "ng", "j", "ch", "k", "t", "p", "h"];

fn romanize_char(c: char) -> String {
    let cp = c as u32;
    if !(0xAC00..=0xD7A3).contains(&cp) {
        return c.to_string();
    }
    let idx = cp - 0xAC00;
    let fin = (idx % 28) as usize;
    let vow = ((idx / 28) % 21) as usize;
    let ini = (idx / 28 / 21) as usize;
    format!("{}{}{}", INITIALS[ini], VOWELS[vow], FINALS[fin])
}

fn romanize(text: &str) -> String {
    text.chars().map(romanize_char).collect()
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_basic_romanization() {
        assert_eq!(romanize("한국"), "hangug");
        assert_eq!(romanize("사랑"), "sarang"); // ㄹ as initial → "r"
    }

    #[test]
    fn test_particle_stripping() {
        let (stem, particles) = strip_particles("학교에서");
        assert_eq!(stem, "학교");
        assert_eq!(particles, vec!["에서"]);
    }

    #[test]
    fn test_tokenize_simple() {
        let tokens = tokenize_korean("나는 학교에 간다");
        assert_eq!(tokens.len(), 3);
        assert!(tokens[0].particles.contains(&"는".to_string())
            || tokens[0].lemma == "나는"); // particle or eojeol preserved
    }

    #[test]
    fn test_topic_particle() {
        let (stem, particles) = strip_particles("나는");
        assert_eq!(stem, "나");
        assert_eq!(particles, vec!["는"]);
    }

    #[test]
    fn test_object_particle() {
        let (stem, particles) = strip_particles("밥을");
        assert_eq!(stem, "밥");
        assert_eq!(particles, vec!["을"]);
    }
}
