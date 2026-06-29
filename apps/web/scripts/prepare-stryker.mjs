import { copyFileSync } from 'node:fs';
import { URL } from 'node:url';

copyFileSync(
	new URL('../tsconfig.stryker.json', import.meta.url),
	new URL('../tsconfig.json', import.meta.url)
);
