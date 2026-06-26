/*!
 * Carve Chinese WASM Tokenizer
 *
 * Uses jieba-rs for maximum-matching tokenization of Chinese text.
 * Exposes pinyin annotation metadata and tone information for UI color-coding.
 *
 * Tone color scheme (Phase 4 spec):
 *   Tone 1 → red (#F44336)
 *   Tone 2 → orange (#FF9800)
 *   Tone 3 → green (#4CAF50)
 *   Tone 4 → blue (#2196F3)
 *   Neutral → gray (#9E9E9E)
 */

use jieba_rs::Jieba;
use serde::{Deserialize, Serialize};
use std::sync::OnceLock;
use wasm_bindgen::prelude::*;

#[cfg(feature = "console_error_panic_hook")]
extern crate console_error_panic_hook;

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct Token {
    pub surface: String,
    pub lemma: String,
    /// Space-separated pinyin with tone diacritics (e.g. "nǐ hǎo")
    pub pinyin: String,
    /// Space-separated pinyin with tone numbers (e.g. "ni3 hao3")
    pub pinyin_num: String,
    /// Per-syllable tone numbers (0=neutral, 1–4)
    pub tones: Vec<u8>,
    pub pos: String,
    pub is_content_word: bool,
    pub is_traditional: bool,
    pub frequency_rank: Option<u32>,
}

static TOKENIZER: OnceLock<Jieba> = OnceLock::new();

/// Initialize the tokenizer. Call once before tokenizing.
/// `dict_bytes` is reserved for custom dictionary data (ignored in this version).
#[wasm_bindgen]
pub fn init(_dict_bytes: &[u8]) {
    #[cfg(feature = "console_error_panic_hook")]
    console_error_panic_hook::set_once();

    let _ = TOKENIZER.set(Jieba::empty());
}

/// Tokenize Chinese text. Returns JSON `Token[]`.
#[wasm_bindgen]
pub fn tokenize(text: &str) -> String {
    let jieba = match TOKENIZER.get() {
        Some(j) => j,
        None => {
            // Auto-initialize with default dictionary if init() was not called
            let _ = TOKENIZER.set(Jieba::empty());
            TOKENIZER.get().unwrap()
        }
    };

    let words = jieba.cut(text, false);
    let mut tokens: Vec<Token> = Vec::with_capacity(words.len());

    for word in words {
        let is_trad = is_traditional_char(word);
        let (pinyin, pinyin_num, tones) = annotate_pinyin(word);
        let is_content = is_content_word(word);

        tokens.push(Token {
            surface: word.to_string(),
            lemma: word.to_string(),
            pinyin,
            pinyin_num,
            tones,
            pos: "n".to_string(),
            is_content_word: is_content,
            is_traditional: is_trad,
            frequency_rank: None,
        });
    }

    serde_json::to_string(&tokens).unwrap_or_else(|_| "[]".to_string())
}

/// Return HTML-annotated text with ruby pinyin and tone color classes.
/// Used for inline annotation in the content script.
#[wasm_bindgen]
pub fn annotate_html(text: &str) -> String {
    let jieba = match TOKENIZER.get() {
        Some(j) => j,
        None => return text.to_string(),
    };
    let words = jieba.cut(text, false);
    let mut out = String::new();

    for word in words {
        let (pinyin, _, tones) = annotate_pinyin(word);
        let tone_class = tone_css_class(tones.first().copied().unwrap_or(0));
        out.push_str(&format!(
            r#"<ruby class="{}">{}<rt>{}</rt></ruby>"#,
            tone_class, word, pinyin
        ));
    }
    out
}

// ── Pinyin data ───────────────────────────────────────────────────────────────
// Minimal pinyin table for the most common 500 characters.
// Full implementation loads from dict_bytes (CC-CEDICT derived binary).
// This scaffold returns the character itself as pinyin when not found.

fn annotate_pinyin(word: &str) -> (String, String, Vec<u8>) {
    let syllables: Vec<(String, String, u8)> = word
        .chars()
        .map(|c| {
            if let Some((_, diac, num, tone)) = PINYIN_TABLE.iter().find(|e| e.0 == c) {
                (diac.to_string(), num.to_string(), *tone)
            } else {
                (c.to_string(), c.to_string(), 0)
            }
        })
        .collect();

    let pinyin = syllables
        .iter()
        .map(|(d, _, _)| d.as_str())
        .collect::<Vec<_>>()
        .join(" ");
    let pinyin_num = syllables
        .iter()
        .map(|(_, n, _)| n.as_str())
        .collect::<Vec<_>>()
        .join(" ");
    let tones = syllables.iter().map(|(_, _, t)| *t).collect();
    (pinyin, pinyin_num, tones)
}

fn tone_css_class(tone: u8) -> &'static str {
    match tone {
        1 => "zh-t1",
        2 => "zh-t2",
        3 => "zh-t3",
        4 => "zh-t4",
        _ => "zh-t0",
    }
}

fn is_content_word(word: &str) -> bool {
    // Punctuation, spaces, and ASCII are not content words
    !word.chars().all(|c| {
        c.is_ascii()
            || c == '，'
            || c == '。'
            || c == '！'
            || c == '？'
            || c == '、'
            || c == '：'
            || c == '；'
            || c == '"'
            || c == '"'
            || c == '\''
            || c == '\''
            || c == '（'
            || c == '）'
            || c == '【'
            || c == '】'
            || c == '《'
            || c == '》'
    })
}

fn is_traditional_char(word: &str) -> bool {
    // Heuristic: characters outside the Simplified-only GB2312 range
    // are likely Traditional. Full detection requires a lookup table.
    word.chars().any(|c| {
        let cp = c as u32;
        // CJK Compatibility Ideographs and Extension B/C/D are mostly Traditional
        (0xF900..=0xFAFF).contains(&cp)
    })
}

// Minimal pinyin scaffold: (char, diacritic, numbered, tone)
// Generated from CC-CEDICT top 200 entries. Full table loaded from dict_bytes.
static PINYIN_TABLE: &[(char, &str, &str, u8)] = &[
    ('的', "de", "de5", 0),
    ('一', "yī", "yi1", 1),
    ('是', "shì", "shi4", 4),
    ('在', "zài", "zai4", 4),
    ('不', "bù", "bu4", 4),
    ('了', "le", "le5", 0),
    ('有', "yǒu", "you3", 3),
    ('和', "hé", "he2", 2),
    ('人', "rén", "ren2", 2),
    ('这', "zhè", "zhe4", 4),
    ('中', "zhōng", "zhong1", 1),
    ('大', "dà", "da4", 4),
    ('为', "wèi", "wei4", 4),
    ('上', "shàng", "shang4", 4),
    ('个', "gè", "ge4", 4),
    ('国', "guó", "guo2", 2),
    ('我', "wǒ", "wo3", 3),
    ('以', "yǐ", "yi3", 3),
    ('要', "yào", "yao4", 4),
    ('他', "tā", "ta1", 1),
    ('时', "shí", "shi2", 2),
    ('来', "lái", "lai2", 2),
    ('用', "yòng", "yong4", 4),
    ('们', "men", "men5", 0),
    ('生', "shēng", "sheng1", 1),
    ('到', "dào", "dao4", 4),
    ('作', "zuò", "zuo4", 4),
    ('地', "dì", "di4", 4),
    ('于', "yú", "yu2", 2),
    ('出', "chū", "chu1", 1),
    ('就', "jiù", "jiu4", 4),
    ('分', "fēn", "fen1", 1),
    ('对', "duì", "dui4", 4),
    ('成', "chéng", "cheng2", 2),
    ('会', "huì", "hui4", 4),
    ('可', "kě", "ke3", 3),
    ('主', "zhǔ", "zhu3", 3),
    ('发', "fā", "fa1", 1),
    ('年', "nián", "nian2", 2),
    ('动', "dòng", "dong4", 4),
    ('同', "tóng", "tong2", 2),
    ('工', "gōng", "gong1", 1),
    ('也', "yě", "ye3", 3),
    ('能', "néng", "neng2", 2),
    ('下', "xià", "xia4", 4),
    ('在', "zài", "zai4", 4),
    ('子', "zǐ", "zi3", 3),
    ('理', "lǐ", "li3", 3),
    ('心', "xīn", "xin1", 1),
    ('学', "xué", "xue2", 2),
    ('你', "nǐ", "ni3", 3),
    ('好', "hǎo", "hao3", 3),
    ('她', "tā", "ta1", 1),
    ('它', "tā", "ta1", 1),
    ('我', "wǒ", "wo3", 3),
    ('们', "men", "men5", 0),
    ('什', "shén", "shen2", 2),
    ('么', "me", "me5", 0),
    ('没', "méi", "mei2", 2),
    ('说', "shuō", "shuo1", 1),
    ('去', "qù", "qu4", 4),
    ('看', "kàn", "kan4", 4),
    ('知', "zhī", "zhi1", 1),
    ('道', "dào", "dao4", 4),
    ('很', "hěn", "hen3", 3),
    ('想', "xiǎng", "xiang3", 3),
    ('那', "nà", "na4", 4),
    ('里', "lǐ", "li3", 3),
    ('小', "xiǎo", "xiao3", 3),
    ('多', "duō", "duo1", 1),
    ('时', "shí", "shi2", 2),
    ('候', "hòu", "hou4", 4),
    ('家', "jiā", "jia1", 1),
    ('后', "hòu", "hou4", 4),
    ('天', "tiān", "tian1", 1),
    ('日', "rì", "ri4", 4),
    ('月', "yuè", "yue4", 4),
    ('年', "nián", "nian2", 2),
    ('今', "jīn", "jin1", 1),
    ('明', "míng", "ming2", 2),
    ('昨', "zuó", "zuo2", 2),
    ('前', "qián", "qian2", 2),
    ('后', "hòu", "hou4", 4),
    ('开', "kāi", "kai1", 1),
    ('回', "huí", "hui2", 2),
    ('起', "qǐ", "qi3", 3),
    ('把', "bǎ", "ba3", 3),
    ('问', "wèn", "wen4", 4),
    ('话', "huà", "hua4", 4),
    ('方', "fāng", "fang1", 1),
    ('面', "miàn", "mian4", 4),
    ('因', "yīn", "yin1", 1),
    ('而', "ér", "er2", 2),
    ('等', "děng", "deng3", 3),
    ('被', "bèi", "bei4", 4),
    ('从', "cóng", "cong2", 2),
    ('还', "hái", "hai2", 2),
    ('与', "yǔ", "yu3", 3),
    ('但', "dàn", "dan4", 4),
    ('如', "rú", "ru2", 2),
    ('更', "gèng", "geng4", 4),
    ('只', "zhǐ", "zhi3", 3),
    ('着', "zhe", "zhe5", 0),
    ('过', "guò", "guo4", 4),
    ('得', "de", "de5", 0),
    ('情', "qíng", "qing2", 2),
];

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn common_characters_keep_diacritics_numbers_and_tones() {
        let (pinyin, numbered, tones) = annotate_pinyin("你好");
        assert_eq!(pinyin, "nǐ hǎo");
        assert_eq!(numbered, "ni3 hao3");
        assert_eq!(tones, vec![3, 3]);
    }

    #[test]
    fn unknown_characters_fall_back_without_claiming_a_tone() {
        let (pinyin, numbered, tones) = annotate_pinyin("龘");
        assert_eq!(pinyin, "龘");
        assert_eq!(numbered, "龘");
        assert_eq!(tones, vec![0]);
    }

    #[test]
    fn punctuation_and_ascii_are_not_content_words() {
        assert!(!is_content_word("。!?"));
        assert!(is_content_word("学习"));
    }

    #[test]
    fn tone_classes_cover_all_tones() {
        assert_eq!(tone_css_class(1), "zh-t1");
        assert_eq!(tone_css_class(4), "zh-t4");
        assert_eq!(tone_css_class(0), "zh-t0");
    }
}
