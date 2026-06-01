/**
 * Design tokens — the single source of truth for color/spacing/type in
 * the web app. Every component imports from here; raw hex codes in
 * component styles are a regression and get flagged by the L14 polish
 * rubric.
 *
 * The tokens are also written into `:root` as CSS custom properties via
 * `apps/web/src/app.html` (one-off load), so unstyled HTML in routes
 * still picks up the palette automatically — useful for raw email/PDF
 * templates and legacy bits not yet converted.
 */

export const color = {
  // Surface
  bg:        '#13151a',
  bgRaised:  '#1e2128',
  bgInput:   '#161a1e',
  border:    '#2a2d36',
  borderHi:  '#3a3f4d',

  // Text
  textHi:    '#e8eaf0',
  text:      '#c8d0e0',
  textLo:    '#9ba8c0',
  textMuted: '#7a8aa6',
  textDim:   '#8a96b3',
  textFaint: '#8a96b3',

  // Brand
  green:     '#4caf50',
  greenDark: '#43a047',
  greenLite: '#81c784',
  greenBg:   '#162316',
  // Solid CTA background — chosen for WCAG AA (≥4.5:1) with white text.
  greenBtn:  '#2e7d32',
  greenBtnHi:'#388e3c',

  // Semantic
  success:   '#4caf50',
  warning:   '#ffa726',
  danger:    '#e57373',
  dangerHi:  '#ef5350',
  info:      '#42a5f5',
  // Solid danger-button background — WCAG AA (≥4.5:1) with white text.
  dangerBtn:   '#c62828',
  dangerBtnHi: '#d84343',

  // FSRS rating palette
  ratingAgain: '#e57373',
  ratingHard:  '#ffb74d',
  ratingGood:  '#4caf50',
  ratingEasy:  '#42a5f5',
} as const;

export const space = {
  '0':  '0',
  '1':  '0.25rem',   // 4px
  '2':  '0.5rem',    // 8px
  '3':  '0.75rem',   // 12px
  '4':  '1rem',      // 16px
  '5':  '1.25rem',   // 20px
  '6':  '1.5rem',    // 24px
  '8':  '2rem',      // 32px
  '10': '2.5rem',    // 40px
  '12': '3rem',      // 48px
  '16': '4rem',      // 64px
  '24': '6rem',      // 96px
} as const;

export const radius = {
  sm: '4px',
  md: '7px',
  lg: '10px',
  xl: '14px',
  full: '999px',
} as const;

export const type = {
  family: 'system-ui, -apple-system, "Inter", sans-serif',
  familyCJK: '"Noto Sans JP", "Noto Sans SC", "Noto Sans KR", system-ui',
  familyMono: 'ui-monospace, SFMono-Regular, Menlo, monospace',

  // Sizes
  xs:   '0.75rem',   // 12px
  sm:   '0.85rem',   // ~14px
  base: '0.95rem',   // ~15px
  md:   '1rem',      // 16px
  lg:   '1.1rem',
  xl:   '1.25rem',
  '2xl':'1.5rem',
  '3xl':'2rem',

  // Weights
  regular: 400,
  medium:  500,
  semi:    600,
  bold:    700,
  heavy:   800,

  // Leading
  tight: 1.2,
  body:  1.5,
  loose: 1.7,
} as const;

export const shadow = {
  sm: '0 1px 2px rgba(0, 0, 0, 0.20)',
  md: '0 4px 12px rgba(0, 0, 0, 0.30)',
  lg: '0 8px 24px rgba(0, 0, 0, 0.35)',
  focusRing: `0 0 0 3px ${color.green}40`,
} as const;

export const motion = {
  fast:   '120ms',
  base:   '180ms',
  slow:   '280ms',
  easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
} as const;

/**
 * The CSS string sent into `<style>` on app boot so every route shares
 * the palette via custom properties. Components prefer `color.*`
 * imports for type safety; raw HTML (e.g. email previews) falls back
 * to `var(--c-bg)` etc.
 */
export function cssVariables(): string {
  const decls: string[] = [];
  for (const [k, v] of Object.entries(color)) decls.push(`--c-${k}: ${v};`);
  for (const [k, v] of Object.entries(space)) decls.push(`--s-${k}: ${v};`);
  for (const [k, v] of Object.entries(radius)) decls.push(`--r-${k}: ${v};`);
  for (const [k, v] of Object.entries(motion)) decls.push(`--m-${k}: ${v};`);
  return `:root {\n  ${decls.join('\n  ')}\n}`;
}
