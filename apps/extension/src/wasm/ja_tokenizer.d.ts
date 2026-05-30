/* tslint:disable */
/* eslint-disable */

/**
 * Initialize the tokenizer with optional dictionary bytes.
 *
 * Call once before tokenizing. When `dict_bytes` is empty, falls back to
 * the built-in minimal vocabulary (Phase 0 scaffold mode).
 *
 * Phase 1: `dict_bytes` will contain the compact binary dict built from JMdict.
 */
export function init(dict_bytes: Uint8Array): void;

/**
 * Look up a single word by exact surface form.
 *
 * Returns JSON string of `Token | null`.
 */
export function lookup(surface: string): string;

/**
 * Returns the full hiragana reading of text (all morphemes concatenated).
 * Convenience function for content scripts.
 */
export function reading_of(text: string): string;

/**
 * Tokenize Japanese text.
 *
 * Returns JSON string of `Token[]`.
 * The tokenizer must be initialized with `init()` first.
 */
export function tokenize(text: string): string;

export type InitInput = RequestInfo | URL | Response | BufferSource | WebAssembly.Module;

export interface InitOutput {
    readonly memory: WebAssembly.Memory;
    readonly init: (a: number, b: number) => void;
    readonly lookup: (a: number, b: number) => [number, number];
    readonly reading_of: (a: number, b: number) => [number, number];
    readonly tokenize: (a: number, b: number) => [number, number];
    readonly __wbindgen_free: (a: number, b: number, c: number) => void;
    readonly __wbindgen_malloc: (a: number, b: number) => number;
    readonly __wbindgen_realloc: (a: number, b: number, c: number, d: number) => number;
    readonly __wbindgen_externrefs: WebAssembly.Table;
    readonly __wbindgen_start: () => void;
}

export type SyncInitInput = BufferSource | WebAssembly.Module;

/**
 * Instantiates the given `module`, which can either be bytes or
 * a precompiled `WebAssembly.Module`.
 *
 * @param {{ module: SyncInitInput }} module - Passing `SyncInitInput` directly is deprecated.
 *
 * @returns {InitOutput}
 */
export function initSync(module: { module: SyncInitInput } | SyncInitInput): InitOutput;

/**
 * If `module_or_path` is {RequestInfo} or {URL}, makes a request and
 * for everything else, calls `WebAssembly.instantiate` directly.
 *
 * @param {{ module_or_path: InitInput | Promise<InitInput> }} module_or_path - Passing `InitInput` directly is deprecated.
 *
 * @returns {Promise<InitOutput>}
 */
export default function __wbg_init (module_or_path?: { module_or_path: InitInput | Promise<InitInput> } | InitInput | Promise<InitInput>): Promise<InitOutput>;
