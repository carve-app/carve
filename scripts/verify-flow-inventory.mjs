#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const inventoryPath = path.join(root, 'docs', '15-flow-inventory.md');
const inventory = fs.readFileSync(inventoryPath, 'utf8');
const refs = [...inventory.matchAll(/`([^`]+\.(?:go|js|mjs|py|ts))`/g)].map((match) => match[1]);
const missing = [...new Set(refs)].filter((ref) => !fs.existsSync(path.join(root, ref)));

if (missing.length > 0) {
  console.error('Flow inventory references missing evidence:');
  for (const ref of missing) console.error(`  - ${ref}`);
  process.exit(1);
}
if (refs.length === 0) {
  console.error('Flow inventory contains no enforceable evidence references');
  process.exit(1);
}
console.log(`Flow inventory verified: ${new Set(refs).size} evidence files exist.`);
