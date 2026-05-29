export interface Token {
  surface: string;
  lemma: string;
  reading: string;       // katakana
  reading_hira: string;  // hiragana
  pos: string;
  is_content_word: boolean;
  user_status: 'known' | 'learning' | 'unknown';
  frequency_rank: number | null;
}

export interface FuriganaSpan {
  text: string;
  reading: string;
}

export interface DictEntry {
  lemma: string;
  reading: string | null;
  frequency_rank: number | null;
  jlpt_level: string | null;
  definitions: Array<{
    definition: string;
    pos: string;
    confidence: number;
  }>;
  furigana: FuriganaSpan[];
  found: boolean;
}

export interface Card {
  id: string;
  lemma: string;
  front_text: string;
  back_text: string;
  sentence: string | null;
  source_url: string | null;
  fsrs_state: string;
  fsrs_due: string | null;
  created_at: string;
}

export type UserStatus = 'known' | 'learning' | 'unknown';
