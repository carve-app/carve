import { defineConfig } from 'vite';
import { viteStaticCopy } from 'vite-plugin-static-copy';
import { resolve } from 'path';

type Target = 'background' | 'content' | 'popup';
type Browser = 'chrome' | 'firefox' | 'safari';

const TARGET = (process.env.BUILD_TARGET ?? 'background') as Target;
const BROWSER = (process.env.BROWSER ?? 'chrome') as Browser;

const entries: Record<Target, { input: string; outFile: string }> = {
  background: {
    input: resolve(__dirname, 'src/background/index.ts'),
    outFile: 'background/index.js',
  },
  content: {
    input: resolve(__dirname, 'src/content/index.ts'),
    outFile: 'content/index.js',
  },
  popup: {
    input: resolve(__dirname, 'src/popup-page/app.ts'),
    outFile: 'popup/app.js',
  },
};

const { input, outFile } = entries[TARGET];
const outDir = `dist/${BROWSER}`;

export default defineConfig({
  define: {
    __WEB_BASE__: JSON.stringify(process.env.WEB_BASE ?? 'http://localhost:5173'),
  },
  build: {
    outDir,
    emptyOutDir: TARGET === 'background',
    rollupOptions: {
      input,
      output: {
        entryFileNames: outFile,
        format: 'iife',
        inlineDynamicImports: true,
      },
    },
    target: 'es2022',
    minify: false,
  },
  plugins: [
    ...(TARGET === 'background'
      ? [
          viteStaticCopy({
            targets: [
              { src: `src/manifest.${BROWSER}.json`, dest: '.', rename: 'manifest.json' },
              { src: 'src/content/shortcut-bridge.js', dest: 'content' },
              { src: 'src/icons/*', dest: 'icons' },
              { src: 'src/popup-page/popup.html', dest: 'popup', rename: 'index.html' },
              { src: 'src/wasm/ja_tokenizer_bg.wasm', dest: 'wasm' },
              { src: 'src/wasm/ja_tokenizer.js', dest: 'wasm' },
            ],
          }),
        ]
      : []),
  ],
});
