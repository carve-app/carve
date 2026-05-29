# Research: Migaku Analysis & Second Language Acquisition Science

## Part 1 — Migaku Deep Dive

### What Migaku Is

Migaku is a subscription language learning platform (~$14/month, $99/year) consisting of:

- A Chrome browser extension that overlays any webpage with interactive word lookup
- Integration with streaming services (Netflix, YouTube, Disney+, Viki, Twitter/X, Reddit)
- A built-in spaced repetition system for vocabulary review
- An "Academy" offering structured beginner courses
- A mobile companion app
- Support for 12+ languages (Japanese, Chinese, Korean, Spanish, French, German, Portuguese, Vietnamese, Italian, English, and more)

The core workflow: user watches/reads native content → encounters unknown word → one-click lookup → one-click flashcard creation → reviews cards with built-in SRS.

### Feature Inventory

| Feature | Description | Quality |
|---|---|---|
| Word popup | Definition, pronunciation, images, AI explanation | Good concept, accuracy issues |
| One-click cards | Capture word + sentence + audio + screenshot | Best-in-class UX |
| SRS reviews | Modified SM-2 algorithm | Works, but outdated vs FSRS |
| Comprehension score | % of known words on page | Useful but imprecise |
| Streaming integration | Netflix/YouTube subtitle hooks | Good, best feature |
| Academy courses | Structured vocabulary-first beginner courses | Decent, not differentiated |
| Reader app | Japanese text with furigana and lookup | Parsing bugs (see below) |
| Mobile app | Companion review app | Noticeably worse than desktop |
| Monolingual dictionaries | Target-language definitions | Available but opt-in and clunky |

---

### Community Pain Points & Criticisms

Sources: WaniKani community forum, Trustpilot, independent reviews, Reddit.

#### 1. Parsing & Translation Accuracy (Critical)

This is Migaku's most damaging flaw. Users have documented:

- **Incorrect morpheme segmentation**: e.g., `ございません` being split as `ござい + ません` instead of being recognized as the polite negative form of `あります`. Basic grammar forms are mishandled.
- **Wrong furigana (pitch accent / reading errors)**: Common words like `入って` receive incorrect reading guidance.
- **"A lot, really a lot, of misleading translations"** (direct user quote from WaniKani forum)
- Errors found within 2 minutes of using recommended content — quality control is minimal.

This is catastrophic for a tool whose primary value proposition is teaching you vocabulary in context. If the reading annotation is wrong, learners internalize incorrect pronunciations.

#### 2. Chrome Lock-in

- No Firefox support
- No Safari support
- Users on macOS who prefer Safari or on Linux with Firefox are excluded entirely.

#### 3. Customer Support Failures

- Bugs are acknowledged as "actively investigated" but persist for months.
- Users directed to Discord rather than receiving direct support.
- No SLA on bug fixes; no public roadmap with reliable dates.

#### 4. Pricing Trajectory

- Lifetime subscription: $200 → $400 → announced increase to $500 by early 2025.
- Monthly: $14. Annual: $99.
- Price hikes announced with short notice, pressuring users into purchases.
- No self-hosting option. No offline mode.

#### 5. Mobile Experience

- The mobile app is described as "clunkier" than desktop.
- Performance degrades on older hardware.
- Card creation on mobile is frustrating.

#### 6. Setup Complexity

- Requires understanding of Anki card types, templates, and AnkiConnect to use the advanced features.
- Non-technical users hit a significant wall.
- Multiple configuration steps before the tool is productive.

#### 7. Data Portability & Vendor Lock-in

- Flashcard data lives in Migaku's system; export capabilities are limited.
- If the service shuts down (a real concern for a paid SaaS), learning history is lost.
- No open standard for card export beyond Anki-compatible format.

#### 8. SRS Algorithm

- Uses a "modified" SM-2 variant — the same algorithm from 1987 that Anki popularized.
- FSRS-6 (Free Spaced Repetition Scheduler) has been demonstrated to be significantly more accurate at predicting optimal review timing, reducing reviews needed while maintaining retention.
- Migaku has not adopted FSRS.

#### 9. Output Practice Gap

- The platform is almost entirely input-focused: reading and listening.
- No writing practice, no speaking drills, no production exercises.
- Swain's Output Hypothesis (1985, refined 1995) demonstrates production is essential for acquisition, not optional.

#### 10. No Immersion Tracking

- No built-in way to log reading hours or listening hours.
- No analytics on immersion time vs SRS time.
- Users must use external apps (Toggl, spreadsheets) to track their own progress.

#### 11. Content Discovery

- Comprehension scoring exists but is coarse.
- No recommendation engine to find new content at the right i+1 level.
- Users must manually find appropriate content.

#### 12. Transparency

- Marketing claims ("revolutionize language learning") outpace current capabilities.
- No public accuracy benchmarks for the parser or dictionary.
- Algorithm is a black box — users cannot see why a card is scheduled for a particular date.

---

### Competitive Landscape

| Tool | Strengths | Weaknesses |
|---|---|---|
| **Migaku** | All-in-one, streaming integration, card creation UX | Parsing bugs, Chrome-only, opaque, expensive |
| **Anki** | Free, open, community decks, proven SRS | Ugly UI, no content integration, manual card creation |
| **Yomitan** | Accurate parsing, free, open-source, Firefox+Chrome | Dictionary only, no SRS, no content recommendations |
| **LingQ** | Content library, reading+listening, community | Expensive, word tracking gamey, no real SRS |
| **Duolingo** | Gamified, low friction | Not effective for serious learners, gamification over acquisition |
| **Clozemaster** | Sentence context learning | No content integration, limited languages |
| **Language Reactor** | Netflix/YouTube subtitles, free tier | No SRS, no card creation, limited languages |

---

## Part 2 — Second Language Acquisition Science

This section synthesizes the research that should directly inform Carve's design.

### 2.1 Krashen's Input Hypothesis & i+1

**Theory**: Language is acquired (not learned) through exposure to comprehensible input — messages we understand — at a level slightly above our current competence (i+1). Input at i+2 or higher is incomprehensible and fails to drive acquisition.

**Research support**: Immersion programs demonstrate very high success rates. Learners regularly acquire grammar rules never explicitly taught. Studies confirm that learners with more exposure to target language are more proficient.

**Application for Carve**:
- Track each user's vocabulary knowledge precisely (which words are known at which confidence level)
- Score any piece of content for comprehension % before the user commits time to it
- Recommend content in the 95–98% comprehension range (i+1 zone)
- Surface the unknown words in content before immersion begins so user can pre-study

### 2.2 Schmidt's Noticing Hypothesis

**Theory**: Conscious attention to a linguistic feature is necessary for acquisition. Subconscious exposure alone is insufficient — the learner must notice (register at the conscious level) the target form.

**Application for Carve**:
- Inline word lookup should direct attention to morphology, not just meaning
- Cards should highlight the specific feature being learned (conjugation pattern, pitch accent contour, character component)
- Grammar pop-ups should explain the "why" of a form, not just the translation
- Spaced repetition surfaces words repeatedly in varied contexts to deepen noticing

### 2.3 Swain's Output Hypothesis

**Theory**: Producing language (speaking, writing) drives acquisition in ways input alone cannot. Output forces learners to notice gaps in their knowledge, tests hypotheses, and makes language more automatic.

**Application for Carve**:
- Integrate sentence production exercises tied directly to mined vocabulary
- Shadow listening exercises (output while listening)
- Prompted writing: given a set of recently-mined words, write sentences
- AI-powered error correction with explanations grounded in the target grammar

### 2.4 Spaced Repetition & The Spacing Effect

**Research**: Distributed practice outperforms massed practice by 200–400% for long-term retention. The FSRS-6 algorithm, trained on 100M+ Anki reviews, significantly outperforms the legacy SM-2 algorithm in predicting optimal review timing.

**Key FSRS-6 concepts**:
- **Stability (S)**: How long a memory is expected to persist (days until 90% retention)
- **Difficulty (D)**: Intrinsic difficulty of the item
- **Retrievability (R)**: Current probability of recall (decays exponentially)
- Cards are scheduled to be reviewed when R drops to the desired retention threshold (default 90%)

**Application for Carve**:
- Implement FSRS-6 natively, not SM-2
- Show users S, D, and R for each card — transparency builds trust and understanding
- Allow users to set their desired retention rate (80%, 90%, 95%) with visible workload trade-offs
- Same-day review handling per FSRS-6's improvements

### 2.5 Interleaving

**Research**: Interleaved practice (mixing different card types / subjects) improves discrimination between similar items by ~25% compared to blocked practice. A 2024 study of EFL learners over two semesters confirmed interleaved SRS significantly outperforms control groups across vocabulary dimensions (Meaning, Form, Use).

**Application for Carve**:
- Default review queue mixes card types: meaning → form → use → audio → production
- Avoid reviewing synonyms or near-synonyms back-to-back
- Algorithm-aware interleaving: don't block similar vocabulary

### 2.6 The Role of Frequency

**Research**: The most frequent 1,000–2,000 words account for ~80–90% of spoken language and ~70–80% of written text. Learning high-frequency vocabulary first gives the fastest return on investment.

**Application for Carve**:
- Frequency rank visible on every word card
- Suggest prioritizing high-frequency unknowns when mining content
- Beginners guided to pre-built frequency decks before free immersion
- "Frequency unlock" system: show users what content becomes accessible as their known-word count grows

### 2.7 Morpheme Order & Difficulty Estimation

**Research**: Natural language acquisition follows a roughly consistent order for grammatical morphemes (Brown's morpheme order studies). Difficulty of a piece of content is not just about vocabulary — grammar complexity matters.

**Application for Carve**:
- Content difficulty score should combine: vocabulary coverage (% known) + grammar complexity estimate
- Grammar pattern library: known patterns tracked alongside vocabulary
- i+1 calculation incorporates both dimensions

### 2.8 Comprehensible Input in Practice: The 95–98% Rule

**Research (Paul Nation)**: For incidental vocabulary acquisition from reading, learners need to already know ~98% of the words in a text. Below this threshold, comprehension and acquisition both suffer. For intentional study (with lookups), 90–95% is workable.

**Application for Carve**:
- Two modes: "flow reading" (target 98%+ coverage), "mining reading" (target 90–95%)
- Content is color-coded by comprehension tier in real-time
- Hard gate: never recommend content below 85% to a user unless they explicitly override

### 2.9 Motivation & Habit Formation

**Research**: Intrinsic motivation (genuine interest in content) strongly predicts SLA success. Time-on-task is the single largest predictor of eventual proficiency.

**Application for Carve**:
- Let users study content they genuinely enjoy — no forced content
- Streak tracking and daily review habit tools
- Time-on-task dashboard prominently displayed
- No gamification gimmicks (fake XP, fake leagues) — only real metrics

---

## Summary of Design Implications

| SLA Principle | Carve Feature Required |
|---|---|
| i+1 comprehensible input | Real-time comprehension scoring, content recommendations |
| Noticing hypothesis | Rich morphological annotation, not just translations |
| Output hypothesis | Production drills, shadowing, writing prompts |
| Spaced repetition | FSRS-6 with visible metrics (S, D, R) |
| Interleaving | Mixed-type review queues, anti-similarity scheduling |
| Frequency effect | Frequency rank on all words, guided deck paths |
| 98% coverage rule | Content difficulty gating, two reading modes |
| Motivation & habit | Real immersion time tracking, genuine content freedom |
| Transparency | Algorithm visibility, no black boxes |
