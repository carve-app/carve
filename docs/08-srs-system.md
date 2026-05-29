# SRS System: FSRS-6 Implementation

Carve uses FSRS-6 (Free Spaced Repetition Scheduler, version 6) as its scheduling algorithm. This replaces the SM-2 algorithm that Anki and Migaku use, which was designed in 1987 and has not been updated to reflect modern memory research.

---

## Why FSRS-6 Over SM-2

| Criterion | SM-2 | FSRS-6 |
|---|---|---|
| Theoretical basis | Ad-hoc empirical (Wozniak 1987) | DSR memory model (Difficulty, Stability, Retrievability) |
| Trained on data | No | Yes (100M+ real Anki review events) |
| Handles same-day reviews | Poorly | Yes (FSRS-6 specific improvement) |
| Predicts forgetting curve | Constant intervals | Exponential decay with individual item tracking |
| Personalizes per item | Ease factor (blunt) | Difficulty + Stability (fine-grained) |
| Optimizable | No | Yes (parameters fit to user's own review history) |
| Retention accuracy | ~70% | ~90%+ on held-out data |
| New card intervals | Fixed (steps) | Mathematically derived |

---

## FSRS-6 Memory Model

Three variables describe the state of each memory:

| Variable | Symbol | Range | Meaning |
|---|---|---|---|
| **Stability** | S | > 0 (days) | Days until retrievability drops to 90% |
| **Difficulty** | D | 1–10 | Intrinsic difficulty of the item (1=easy, 10=hard) |
| **Retrievability** | R | 0–1 | Current probability of successful recall |

**Forgetting curve:**
```
R(t) = 0.9^(t/S)
```
When `t = S` days have passed, `R = 0.9` (90% chance of recall — exactly at the review threshold).

---

## Algorithm: Review Scheduling

### Rating Scale

| Rating | Label | Meaning |
|---|---|---|
| 1 | Again | Complete blackout — couldn't recall |
| 2 | Hard | Recalled with significant difficulty |
| 3 | Good | Recalled correctly after some hesitation |
| 4 | Easy | Recalled instantly, effortlessly |

### State Machine

```
           rate 3/4              rate 3/4
new ────────────────→ learning ──────────────→ review
         rate 1/2 ↗              rate 1/2 ↘
                                           relearning
                                           rate 3/4 ↗
                                           (back to review)
```

- **new**: Never reviewed. Scheduled immediately.
- **learning**: Recently introduced, still in short-interval phase.
- **review**: Graduated to long-term review scheduling.
- **relearning**: Lapsed (rated 1 in review state), now re-stabilizing.

### Core Equations (FSRS-6)

Parameters `w[0]..w[21]` are global defaults (derived from training data). Users can optimize their own parameters via the FSRS optimizer after accumulating review history (>= 400 reviews recommended).

```python
# Stability after first successful review
def stability_after_first_learning(D: float, R: float, rating: int) -> float:
    return (
        w[0] * (11 - D) *
        (D ** -w[1]) *
        (math.e ** (w[2] * (1 - R)) - 1) *
        (w[15] if rating == 2 else  # Hard
         w[16] if rating == 4 else  # Easy
         1.0)                       # Good
    )

# Stability after successful recall in review state
def stability_after_recall(D: float, S: float, R: float, rating: int) -> float:
    hard_penalty = w[15] if rating == 2 else 1
    easy_bonus  = w[16] if rating == 4 else 1
    return S * (
        math.e ** (w[8]) *
        (11 - D) *
        (S ** -w[9]) *
        (math.e ** (w[10] * (1 - R)) - 1) *
        hard_penalty *
        easy_bonus +
        1
    )

# Stability after forgetting (lapse)
def stability_after_forgetting(D: float, S: float, R: float) -> float:
    return (
        w[11] *
        (D ** -w[12]) *
        ((S + 1) ** w[13]) *
        math.e ** (w[14] * (1 - R)) *
        (math.e ** (w[17]) - 1)  # same-day review correction (FSRS-6)
    )

# Difficulty update after each review
def update_difficulty(D: float, rating: int) -> float:
    delta_D = -w[6] * (rating - 3)  # rating 3=Good is neutral
    new_D = D + delta_D * mean_reversion_factor(D)
    return clamp(new_D, 1, 10)

def mean_reversion_factor(D: float) -> float:
    # Pull extreme difficulties back toward mean
    return w[7] * (w[4] - D)

# Next review date
def next_due(S: float, target_retention: float = 0.9) -> timedelta:
    interval = S * math.log(target_retention) / math.log(0.9)
    # Apply fuzz to prevent review clustering
    fuzzed = apply_fuzz(interval)
    return timedelta(days=max(1, round(fuzzed)))

def apply_fuzz(interval: float) -> float:
    if interval < 2.5:
        return interval
    fuzz_range = max(1, round(interval * 0.05))
    return interval + random.uniform(-fuzz_range, fuzz_range)
```

Default parameters `w` (FSRS-6 trained defaults):
```python
W_DEFAULTS = [
    0.40072, 1.18947, 3.17278, 15.6948, 7.1932,
    0.56812, 1.05756, 0.0, 1.37095, 0.10537,
    0.98883, 1.89527, 0.11402, 0.29739, 2.29898,
    0.51655, 2.46243, 0.00, 0.0, 0.0,
    0.0, 0.0
]
```

---

## Interleaved Queue Construction

The daily review queue is not simply "all due cards in random order." Carve constructs an interleaved queue to maximize discrimination between similar items.

### Queue Building Algorithm

```python
def build_review_queue(
    due_cards: list[Card],
    new_cards: list[Card],
    daily_new_limit: int,
    target_session_size: int,
) -> list[Card]:
    # 1. Separate by card type
    recognition = [c for c in due_cards if c.card_type == 'recognition']
    production  = [c for c in due_cards if c.card_type == 'production']
    audio       = [c for c in due_cards if c.card_type == 'audio']
    cloze       = [c for c in due_cards if c.card_type == 'cloze']

    # 2. Sort each bucket by due date (most overdue first)
    for bucket in [recognition, production, audio, cloze]:
        bucket.sort(key=lambda c: c.fsrs_due)

    # 3. Interleave: round-robin across types
    queue = []
    iterators = [iter(b) for b in [recognition, audio, production, cloze] if b]
    while iterators:
        next_iterators = []
        for it in iterators:
            card = next(it, None)
            if card:
                queue.append(card)
                next_iterators.append(it)
        iterators = next_iterators

    # 4. Anti-similarity: ensure no two consecutive cards share the same lemma
    queue = desimilarize(queue)

    # 5. Prepend new cards (limited, spaced within queue)
    new_cards = new_cards[:daily_new_limit]
    queue = inject_new_cards(queue, new_cards, injection_interval=5)

    return queue[:target_session_size]


def desimilarize(cards: list[Card]) -> list[Card]:
    # Swap cards when consecutive cards have the same word_id or
    # similar readings (e.g., 聞く vs 効く)
    for i in range(1, len(cards)):
        if cards[i].word_id == cards[i-1].word_id:
            # Find next card with different word_id and swap
            for j in range(i+1, min(i+6, len(cards))):
                if cards[j].word_id != cards[i-1].word_id:
                    cards[i], cards[j] = cards[j], cards[i]
                    break
    return cards
```

---

## Server-Side vs Client-Side Scheduling

Scheduling runs **server-side** to ensure consistency across multiple devices (phone + desktop + web app). The client submits review events; the server updates the card's FSRS state and returns the new `fsrs_due`.

This means:
- No divergence between devices
- Review history is the authoritative record (events are immutable)
- Card state can always be recomputed from the event log (useful for FSRS optimizer)
- Client displays optimistic next-due (computed locally) while waiting for server confirmation

### Sync Protocol

```
Client: POST /review/events [event1, event2, ...]
Server: 1. Validate events (timestamp, card ownership)
        2. For each event: compute new FSRS state
        3. Write immutable review_event records
        4. Update cards table (fsrs_stability, fsrs_difficulty, fsrs_due, fsrs_state)
        5. Update user_word_knowledge (status = 'mature' if stability > 30 days)
        6. Return updated card states
Client: Update local IndexedDB cache with returned states
```

---

## FSRS Parameter Optimizer

After a user accumulates ~400+ reviews, their personal FSRS parameters can be optimized to fit their individual memory patterns. This is done as a background job.

```python
import scipy.optimize

def optimize_fsrs_parameters(review_history: list[ReviewEvent]) -> list[float]:
    """
    Fit FSRS parameters to minimize log-loss on held-out review events.
    Uses gradient-free optimization (Nelder-Mead) since the scheduling
    function isn't differentiable everywhere.
    """
    # Split history: 80% train, 20% validation
    train, val = split(review_history, 0.8)

    def objective(w: list[float]) -> float:
        total_loss = 0
        for event in val:
            predicted_R = compute_retrievability(
                event.card_history, w, event.reviewed_at
            )
            actual = 1 if event.rating >= 2 else 0
            # Binary cross-entropy
            total_loss -= (
                actual * math.log(predicted_R + 1e-7) +
                (1 - actual) * math.log(1 - predicted_R + 1e-7)
            )
        return total_loss / len(val)

    result = scipy.optimize.minimize(
        objective,
        x0=W_DEFAULTS,
        method='Nelder-Mead',
        options={'maxiter': 1000, 'xatol': 1e-4}
    )
    return result.x.tolist()
```

Optimization runs as a background Kubernetes CronJob, triggered when:
- User has >= 400 reviews AND
- >= 30 days since last optimization AND
- >= 50 new reviews since last optimization

---

## Retention & Workload Trade-offs

Users can configure their target retention rate. Carve displays the workload implication before they commit:

```
Target retention: 90% (default)
  → Estimated reviews/day: 35 (for 2,000 mature cards)
  → Forgetting rate: ~10% of cards each review

Target retention: 95%
  → Estimated reviews/day: 58 (+66%)
  → Forgetting rate: ~5% of cards each review

Target retention: 85%
  → Estimated reviews/day: 22 (-37%)
  → Forgetting rate: ~15% of cards each review
```

This transparency helps users make an informed choice rather than blindly trusting a default.

---

## Card Transparency UI

Every card shows its FSRS metrics in the review interface (collapsible, shown by default after first 100 reviews):

```
┌─────────────────────────────────────────────────────┐
│  食べる                                              │
│  ─────────────────────────────────────────────────  │
│  [Front card displayed here]                        │
│                                                     │
│  Card stats:                                        │
│  Retrievability: 88%  Stability: 14d  Difficulty: 4.2│
│  Due: today (was due yesterday)                     │
│  Reviews: 8  Lapses: 1                              │
└─────────────────────────────────────────────────────┘
     [Again]      [Hard]      [Good]      [Easy]
        ↓           ↓           ↓           ↓
     [1d]         [4d]        [18d]       [35d]
```

The interval preview under each button is shown before the user rates, so they understand the consequence of each rating.

---

## Leech Detection

A card is a "leech" if it has been reset (rated Again) too many times relative to its review count. Leeches are flagged for user review — they may indicate the card needs to be reformulated, the vocabulary is too advanced, or a deeper misunderstanding exists.

```python
def is_leech(card: Card) -> bool:
    if card.fsrs_lapses == 0:
        return False
    # Standard Anki threshold: lapses >= 8
    # Carve uses a more forgiving relative threshold
    lapse_ratio = card.fsrs_lapses / max(card.fsrs_reps, 1)
    return card.fsrs_lapses >= 6 and lapse_ratio > 0.25

def handle_leech(card: Card):
    card.suspended = True
    # Notify user with actionable suggestion
    create_notification(card.user_id, {
        "type": "leech",
        "card_id": card.id,
        "message": f"'{card.front_text}' has been suspended after {card.fsrs_lapses} resets. "
                   "Consider rewriting this card or breaking it into simpler parts.",
        "suggestions": ["simplify_card", "add_mnemonic", "find_example", "delete_card"]
    })
```
