import { sveltekit } from '@sveltejs/kit/vite';
import { configDefaults, defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [sveltekit()],
	test: {
		exclude: [...configDefaults.exclude, '**/.stryker-tmp/**'],
		coverage: {
			provider: 'v8',
			include: ['src/lib/offline.ts'],
			thresholds: { lines: 80 }
		}
	}
});
