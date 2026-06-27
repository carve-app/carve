# English vocabulary placement test

## Decision

The onboarding placement test is a short diagnostic of **receptive English
vocabulary**. It is not presented as a CEFR, IELTS, or general-proficiency
score.

The first version contains 30 contextualized form–meaning questions sampled
from six frequency ranges covering approximately the first 12,000 English
lemmas. Five observations are taken from every range. The point estimate is the
sum of each range's observed success rate multiplied by that range's width. A
90% Wilson interval is reported because five observations per range do not
justify a precise single-number claim.

Only correctly answered sample words are written to `user_word_knowledge`.
The extrapolated estimate is stored with the attempt but is never expanded into
thousands of assumed-known rows. This keeps reader and discovery highlighting
conservative. Subsequent reading, mining, and review behavior refines exact
word knowledge over time.

## Research basis

- Nation and Beglar's Vocabulary Size Test samples form–meaning knowledge at
  successive 1,000-word-family levels and extrapolates from the sample. The
  full test uses 140 items; that is appropriate for a measurement instrument
  but too long for onboarding. See [Nation & Beglar
  (2007)](https://openaccess.wgtn.ac.nz/articles/journal_contribution/A_vocabulary_size_test/12552197).
- The updated Vocabulary Levels Test validates frequency-banded diagnostic
  testing and emphasizes that the purpose is to identify the frequency level
  on which learning should focus. Its validated forms use substantially more
  items than this onboarding screen. See [Webb, Sasao & Ballance
  (2017)](https://doi.org/10.1075/itl.168.1.02web).
- A computerized adaptive vocabulary test can reach comparable estimates with
  fewer items than its paper counterpart. This first release keeps a fixed,
  auditable 30-item form; item-response calibration and adaptive routing are a
  later step, after real response data exists. See [Choi
  (2016)](https://doi.org/10.1016/j.compedu.2016.02.018).
- Yes/no recognition is fast but is vulnerable to self-report and false-alarm
  bias. Contextualized multiple choice verifies a form–meaning link instead of
  accepting familiarity. The lemma-based Yes/No Vocabulary Size Test remains a
  useful model for principled sampling across frequency bands. See [Masrai
  (2022)](https://doi.org/10.1177/21582440221074355).
- Frequency alone is not identical to word difficulty. Word prevalence predicts
  processing over and above frequency and should be considered when the item
  bank is calibrated. See [Brysbaert et al.
  (2018)](https://doi.org/10.3758/s13428-018-1077-9).
- Vocabulary size is actionable for content choice: widely cited corpus
  estimates put 98% coverage near 6,000–7,000 word families for spoken English
  and 8,000–9,000 for written English, while also warning that lexical coverage
  is not itself a direct comprehension test. See [Nation's results and their
  replication discussion](https://www.cambridge.org/core/journals/language-teaching/article/how-much-vocabulary-is-needed-to-use-english-replication-of-van-zeeland-schmitt-2012-nation-2006-and-cobb-2007/1D217A56A2E0056E67802A6A8360FDDE).

The test's frequency ranks use the same [`wordfreq` Zipf
scale](https://github.com/rspeer/wordfreq) already used by Carve's English
tokenizer. That makes placement bands and later content scoring internally
consistent, but it does not make this short form a validated psychometric
instrument.

## Product references

- Migaku's useful pattern is a persistent word-by-word knowledge state tied to
  native-content reading and card creation, rather than treating placement as a
  permanent label. Carve follows that pattern while retaining exact attempt
  data and conservative known-word writes. See [Migaku's feature
  overview](https://migaku.com/faq/features).
- LingQ initially treats words as new and learns the user's state as they mark
  words known or interact with lessons. Carve similarly treats placement as a
  prior that real reading behavior can correct. See [LingQ's knowledge-base
  explanation](https://forum.lingq.com/t/how-does-lingq-know-which-words-are-new-to-me/6840).
- Atlas demonstrates a product-appropriate target of 30 questions in roughly
  five minutes. Carve borrows the compact cadence, not Atlas's score claims or
  question content. See [Atlas's placement-test
  overview](https://www.atlasvocabulary.com/).

## Result semantics

`Foundation`, `Developing`, `Independent`, `Strong`, and `Advanced` are Carve
vocabulary-band labels. They intentionally do not map to CEFR, because CEFR
describes broader communicative competence across reading, listening, writing,
and speaking.

## Calibration backlog

1. Collect consented, anonymized item responses and completion time.
2. Check item discrimination, distractor performance, differential item
   functioning by first-language group, and irregular response patterns.
3. Replace weak items and add parallel forms to reduce retest memorization.
4. Fit an IRT/Rasch model only after the sample is large enough; then introduce
   multistage or adaptive routing and empirical confidence intervals.
5. Validate the score against a longer established vocabulary instrument and
   actual unknown-word rates in held-out reading samples.
