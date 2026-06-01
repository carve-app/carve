# Design Rubric (v1)

> **Purpose:** the load-bearing artifact behind the L14 LLM-as-judge layer
> of the testing strategy. Claude reviews each route's screenshot against
> the criteria below and produces a 1–5 score per criterion. The aggregate
> per-route score is tracked over time; a regression of ≥ 0.5 vs. main
> blocks the next release.
>
> Rubric changes are versioned in `docs/14-rubric-changelog.md` and
> treated like API changes.

---

## Scoring scale

| Score | Meaning                                                                    |
|-------|----------------------------------------------------------------------------|
| 5     | Indistinguishable from a senior-designer-led product (Linear, Cron, Raycast). |
| 4     | Strong; one or two minor polish opportunities a reviewer might note.        |
| 3     | Acceptable; obvious that the team owns design, but the rhythm is uneven.   |
| 2     | Visibly amateur or AI-generated; would warrant a redesign.                 |
| 1     | Broken-looking, mis-aligned, or wholly inconsistent with the rest of the app. |

Below 3 on any single criterion fails the route. Aggregate < 3.5 across all
criteria also fails the route.

---

## Criteria

### 1. Design tokens used (not inline hex)
Look for repeated `#13151a`, `#1e2128`, `#4caf50`, `#2a2d36`, etc. in the
HTML — they're the dark-theme palette duplicated across components and
should be replaced by CSS custom properties or `$lib/design` tokens. If
the rendered page would look broken in light theme because the colors are
hardcoded, score 2 or lower.

### 2. Spacing rhythm (4/8/12/16/24/32 px scale)
Inspect vertical rhythm between sibling elements. Random gaps (13px, 19px,
22px) score lower. Containers should breathe consistently from section to
section.

### 3. Typography hierarchy
Exactly one logical H1 per page. Body text should be one consistent size,
labels another, headings another — not five distinct sizes scattered
around. Line-heights should be consistent (1.4–1.6 for body, 1.1–1.3 for
display).

### 4. Empty states
Every list or table view should have a deliberate empty state with a
single sentence of context and one call-to-action. "No cards" with
nothing around it scores 2.

### 5. No generic AI aesthetic
Generic purple→blue gradient hero. Stock "rocket" or "lightbulb" icons.
"AI-Powered Language Learning" superlative copy. Generic glassmorphism.
If the page looks like any other AI-generated landing page, score 2 or
lower.

### 6. Contrast (WCAG AA)
Body text ≥ 4.5:1, large text ≥ 3:1, focus rings ≥ 3:1. The dark theme's
muted grays for secondary text (`#6b7591` on `#13151a`) should be
inspected carefully — these often fail AA.

### 7. Interactive affordances visible
Buttons look pressable. Hover states defined. Focus rings visible without
hover. Disabled states distinct from enabled. A user finding the page via
keyboard navigation must be able to see where the focus is at all times.

### 8. Mobile responsive
Open the screenshot at iPhone 14 viewport (390×844). Does horizontal
scroll appear? Are tap targets ≥ 44 px? Does the bottom nav (if any) sit
above the safe area?

### 9. Loading and skeleton states
A blank page during data fetch scores 2. A spinner alone scores 3. A
skeleton matching the eventual layout scores 4–5.

### 10. Information density
Per-route appropriate density: marketing landing should be airy;
`/cards` should be scan-able with high information density. Either
extreme misapplied scores lower.

---

## Output schema (what Claude returns)

For each route the judge returns:

```json
{
  "route": "/cards",
  "screenshot_hash": "sha256:...",
  "scores": {
    "design_tokens": 4,
    "spacing_rhythm": 4,
    "typography": 5,
    "empty_states": 3,
    "no_ai_aesthetic": 5,
    "contrast": 4,
    "affordances": 4,
    "mobile_responsive": 4,
    "loading_states": 3,
    "information_density": 4
  },
  "aggregate": 4.0,
  "blockers": [],
  "improvements": [
    "Empty state for the /cards list shows no CTA — add a 'Mine your first card' link.",
    "Loading state is a spinner; replace with a card-skeleton list."
  ]
}
```

The `blockers` array is non-empty when any individual criterion < 3. The
`improvements` array is advisory.

---

## How CI uses this

`scripts/polish-review.mjs`:

1. Boots the SvelteKit web app + mock API (same fixtures as L5).
2. Captures one full-page screenshot per route at 1280×720 and 390×844
   (mobile).
3. Posts each screenshot pair to the Claude API along with this rubric.
4. Parses the JSON output above.
5. Writes `reports/polish/polish-scores.json`.
6. Compares against `reports/polish/baseline.json` (committed). A drop of
   ≥ 0.5 on any route's aggregate fails the job.

Updating baselines: when an intentional redesign lands, the PR includes
the new `reports/polish/baseline.json` alongside the code change.
