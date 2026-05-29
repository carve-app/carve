# Product Design

## Core Philosophy

**Carve is built on three commitments:**

1. **Science over magic** — every feature is grounded in SLA research. No fake gamification. No misleading metrics. No promises that can't be kept.

2. **You own your data** — user vocabulary lists, card decks, review history, and immersion logs are always exportable in open formats. The core is open-source and self-hostable. No vendor lock-in.

3. **Accuracy above velocity** — we will not ship a parser that produces wrong furigana. We will not ship a dictionary that gives misleading translations. Quality of annotation is non-negotiable; release cadence is secondary.

---

## Target Users

### Primary: The Serious Self-Study Learner

- Learning Japanese, Chinese, Korean, or a European language independently
- Already knows Anki or has tried immersion methods (AJATT, Krashen-inspired)
- Frustrated by the fragmentation: Yomitan + Anki + Language Reactor + manual tracking
- Willing to pay for a polished, integrated experience if it actually works
- Values accuracy and transparency over flashy design

### Secondary: The Language Learning Hobbyist

- Watches anime, K-dramas, or foreign films with subtitles
- Wants to understand without subtitles someday but doesn't know how to start
- Needs a gentler on-ramp than "install Anki, configure 15 add-ons"
- Responds to clear progress metrics

### Out of Scope (for v1)

- Children learning as a primary education tool
- Corporate language training
- Students needing classroom assignments / LMS integration

---

## The Five Pillars of Carve

### Pillar 1: Precision Annotation

Every word in every piece of content gets:
- Correct morphological segmentation
- Part of speech
- Dictionary form (lemma)
- Reading / pronunciation (with pitch accent for Japanese)
- Frequency rank in the target language
- Confidence score for the annotation itself (low confidence = flagged for user)

This is the foundation. Without it, everything else is noise.

### Pillar 2: Smart Immersion

The browser extension and mobile reader hook into native content and make it navigable without interrupting flow:
- Hover/tap to lookup (never blocking the subtitle/text by default)
- One gesture to mine a word or sentence to the card deck
- Comprehension score overlay per paragraph/subtitle
- Color coding: known words (neutral), known-but-fragile (faint highlight), unknown (color-coded by frequency rank)

### Pillar 3: Science-Based Review

Cards are reviewed using FSRS-6 with full transparency:
- Each card shows current Retrievability %, Stability (days), and Difficulty
- Review session shows workload forecast for the next 7 days
- Interleaved queue: meaning, reading, production, audio
- Retention target is user-configurable (80%, 90%, 95%)

### Pillar 4: Immersion Tracking

A real activity log — not a streak counter:
- Minutes of reading per day (extension tracks open-tab time on target-language content)
- Minutes of listening per day
- Review minutes per day
- Known-word count over time
- Content comprehension score over time (shows that your level is actually improving)
- Weekly and monthly summaries

### Pillar 5: Output Practice

Vocabulary becomes fluency only when you produce it:
- **Sentence writing**: given recently-mined words, write sentences; AI provides feedback
- **Listening shadow**: play audio, transcribe what you hear; system compares to subtitle text
- **Speaking drill**: speak a sentence; speech-to-text evaluates pronunciation
- **Fill-in-the-blank**: cloze deletions from mined sentences

---

## Feature Map

### Browser Extension (cross-browser: Chrome, Firefox, Safari)

| Feature | Description |
|---|---|
| Word popup | Tap/hover for definition, reading, frequency, examples |
| Sentence lookup | Select a sentence for full grammatical breakdown |
| Card mining | One-click capture: word + sentence + audio + screenshot |
| Comprehension overlay | Toggle color-coded annotation on any page |
| Subtitle hook | Intercept Netflix/YouTube/Viki/Disney+ subtitles |
| Subtitle word timing | Word-level timing on YouTube auto-subtitles |
| i+1 meter | Shows comprehension % of current content in toolbar |
| Reading mode | Clean reader with dictionary overlay for web articles |
| Time tracking | Auto-log reading minutes when on target-language pages |

### Web App

| Section | Features |
|---|---|
| Review | Daily card review (FSRS-6), session report, retention graph |
| Library | Saved content (articles, videos, books), sorted by comprehension % |
| Vocabulary | Full word list with S/D/R metrics, search, filter, bulk actions |
| Decks | Pre-built frequency decks, shared community decks, custom decks |
| Stats | Immersion time, known words, retention rate, comprehension trend |
| Output | Writing prompts, cloze exercises, shadowing queue |
| Settings | FSRS parameters, retention target, language config, dictionary selection |

### Mobile App (iOS + Android — PWA first, native later)

| Feature | Description |
|---|---|
| Daily review | Full SRS review session with audio |
| Offline review | Cards synced for offline access |
| Reader | Read imported text/EPUBs with dictionary overlay |
| Time logger | Manual log for listening / watching without Carve open |
| Streak & stats | Daily habit view |

### Backend / Platform

| Feature | Description |
|---|---|
| Multi-language NLP | Accurate tokenization per language (see NLP Pipeline doc) |
| Dictionary system | JMdict, CEDICT, Wiktionary data + community corrections |
| Content indexing | User-submitted URLs indexed with vocabulary profile |
| Deck sharing | Public deck registry with download counts, ratings |
| Data export | Full export: JSON (vocabulary, cards, review history, immersion log) |
| Self-hosting | Docker Compose setup with full documentation |

---

## UX Principles

### Progressive Disclosure

Default state is simple: hover a word, see a popup. No settings visible until you need them. Advanced features (pitch accent, morphological breakdown, FSRS parameters) live one level deeper.

### Real Information Over Gamification

No fake XP. No fake leagues. No unlockable avatars. The only metrics shown are real ones: words known, hours immersed, retention rate, comprehension percentage. These are motivating because they are true.

### Never Break Flow

The immersion experience must not be interrupted. The popup is non-blocking. Card mining is a single gesture. The extension tracks time automatically — you never need to "clock in." Reviews are designed for short sessions (10–15 min) that fit around immersion.

### Transparent Algorithms

Every SRS decision is explainable. A user can click on any card and see: "This card has Stability=12 days, Retrievability=87%, Difficulty=6.2. It's due today because your target retention is 90% and it's been 10 days since your last review." No black boxes.

### Designed for Failure

Missing a day does not wipe your streak (it just doesn't add to it). A missed review doesn't punish — it just adjusts the schedule. The system is designed to be picked back up after breaks. Guilt is not a learning tool.

---

## Monetization Strategy

### Open Core Model

| Tier | Price | What's Included |
|---|---|---|
| **Free** | $0 | Extension + dictionary + 200 cards + 7-day review history |
| **Learner** | $8/month or $72/year | Unlimited cards, all languages, full stats, mobile app |
| **Pro** | $16/month or $144/year | Learner + AI output feedback, priority support, API access |
| **Self-Hosted** | Free (AGPL) | Full platform, self-managed, no support SLA |

### Principles

- Pricing published openly; no surprise hikes
- No lifetime tier (unsustainable business model)
- Cancellation is one click; data export available immediately after cancel
- The open-source core can never be taken away

---

## Supported Languages (Priority Order)

| Priority | Language | Complexity | Notes |
|---|---|---|---|
| 1 | Japanese | Very high | Morpheme segmentation, pitch accent, kanji, furigana |
| 2 | Mandarin Chinese | High | Segmentation, tone marks, traditional/simplified |
| 3 | Korean | High | Morpheme agglutination, particle analysis |
| 4 | Spanish | Medium | Well-studied, good dictionaries |
| 5 | German | Medium | Compound splitting, case tracking |
| 6 | French | Medium | Liaison, elision rules |
| 7 | Portuguese | Medium | Brazilian vs European variant |
| 8 | Italian | Medium | — |
| 9 | Vietnamese | High | Tone marks, segmentation |
| 10+ | Others | Varies | Community-driven language packs |
