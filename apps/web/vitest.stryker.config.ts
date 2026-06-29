import { configDefaults, defineConfig } from 'vitest/config';

// Mutation tests exercise framework-independent IndexedDB helpers. Loading the
// full SvelteKit Vite plugin in every Stryker sandbox makes Vitest run Vite's
// dependency optimizer unnecessarily and can fail inside Rolldown before any
// mutant is tested. Keep this runner deliberately minimal; the regular Vitest
// suite continues to use vite.config.ts and the real SvelteKit integration.
export default defineConfig({
	test: {
		environment: 'node',
		exclude: [...configDefaults.exclude, '**/.stryker-tmp/**']
	}
});
