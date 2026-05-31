/**
 * Cross-browser WebExtension API shim.
 *
 * Firefox 57+  — exposes a Promise-based `browser.*` global natively.
 * Chrome/Edge  — only exposes `chrome.*` (callback-based; Manifest V3
 *               added Promise support for most APIs).
 * Safari 14+   — exposes both `browser.*` and `chrome.*`.
 *
 * We prefer the `browser` global when available; otherwise fall back to
 * `chrome`. Both surfaces have identical method signatures in MV3.
 */

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const _global = globalThis as any;

export const browser: typeof chrome =
  typeof _global.browser !== 'undefined'
    ? (_global.browser as typeof chrome)
    : (typeof _global.chrome !== 'undefined'
        ? (_global.chrome as typeof chrome)
        : ({} as typeof chrome));
