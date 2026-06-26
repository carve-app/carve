import js from '@eslint/js';
import globals from 'globals';
import svelte from 'eslint-plugin-svelte';
import tseslint from 'typescript-eslint';

export default [
  {
    ignores: [
      '**/node_modules/**',
      '**/dist/**',
      '**/build/**',
      '**/.svelte-kit/**',
      '**/coverage/**',
      '**/playwright-report/**',
      '**/test-results/**',
      '**/wasm-src/**/pkg/**',
      '**/wasm-src/**/target/**',
      'apps/extension/src/wasm/ja_tokenizer.js',
      'apps/extension/src/wasm/ja_tokenizer.d.ts',
    ],
  },
  {
    linterOptions: { reportUnusedDisableDirectives: false },
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...svelte.configs['flat/recommended'],
  {
    files: ['**/*.svelte'],
    languageOptions: {
      globals: { ...globals.browser },
      parserOptions: {
        parser: tseslint.parser,
        extraFileExtensions: ['.svelte'],
      },
    },
    rules: {
      'no-undef': 'off',
    },
  },
  {
    files: ['**/*.ts'],
    rules: {
      'no-undef': 'off',
    },
  },
  {
    files: ['apps/web/src/**/*.{js,ts,svelte}', 'apps/extension/src/**/*.{js,ts}'],
    languageOptions: {
      globals: { ...globals.browser, ...globals.webextensions },
    },
  },
  {
    files: ['e2e/**/*.ts'],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'no-empty': ['error', { allowEmptyCatch: true }],
      'no-useless-assignment': 'off',
      'svelte/no-at-html-tags': 'off',
      'svelte/no-immutable-reactive-statements': 'off',
      'svelte/no-navigation-without-resolve': 'off',
      'svelte/no-reactive-literals': 'off',
      'svelte/prefer-svelte-reactivity': 'off',
      'svelte/require-each-key': 'off',
    },
  },
];
