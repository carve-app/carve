'use strict';

// Runtime proof for the packaged Firefox extension. web-ext installs the
// built bundle into a real Firefox profile; the test passes only when the
// content script executes on a Japanese page and reaches the NLP endpoint.

const http = require('node:http');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { firefox } = require('playwright');

const ROOT = path.resolve(__dirname, '..');
const SOURCE_DIR = path.join(ROOT, 'apps', 'extension', 'dist', 'firefox');

function listen(server, port, host = '127.0.0.1') {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, host, () => resolve(server.address().port));
  });
}

function close(server) {
  return new Promise((resolve) => {
    server.close(resolve);
    server.closeAllConnections?.();
  });
}

async function main() {
  let observedTokenize;
  let pageHits = 0;
  const apiRequests = [];
  const tokenized = new Promise((resolve) => { observedTokenize = resolve; });

  const api = http.createServer((req, res) => {
    apiRequests.push(`${req.method} ${req.url}`);
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Headers', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET,POST,OPTIONS');
    if (req.method === 'OPTIONS') {
      res.writeHead(204).end();
      return;
    }
    if (req.method === 'POST' && req.url === '/v1/nlp/tokenize') {
      observedTokenize();
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        tokens: [{
          surface: '한국어', lemma: '한국어', reading: '한국어', reading_hira: '한국어',
          pos: 'noun', is_content_word: true, user_status: 'unknown',
          frequency_rank: 1000,
        }],
        comprehension_pct: 0,
      }));
      return;
    }
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end('{}');
  });

  const pageServer = http.createServer((_req, res) => {
    pageHits += 1;
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end('<!doctype html><html lang="ko"><head><meta http-equiv="refresh" content="2"></head><body><p>한국어</p></body></html>');
  });

  let child;
  try {
    await listen(api, 8080, '::');
    const pagePort = await listen(pageServer, 0);
    const webExtBin = path.join(__dirname, 'node_modules', 'web-ext', 'bin', 'web-ext.js');
    const webExtArgs = [
      webExtBin,
      'run',
      '--source-dir', SOURCE_DIR,
      '--firefox', process.env.FIREFOX_BINARY || firefox.executablePath(),
      '--start-url', `http://127.0.0.1:${pagePort}/`,
      '--no-reload',
      '--no-input',
      '--verbose',
      '--pref=devtools.console.stdout.content=true',
      '--pref=extensions.logging.enabled=true',
    ];
    if (process.env.FIREFOX_HEADLESS === '1') webExtArgs.push('--args=-headless');
    child = spawn(process.execPath, webExtArgs, { stdio: ['ignore', 'pipe', 'pipe'] });

    let logs = '';
    child.stdout.on('data', (chunk) => { logs += chunk; });
    child.stderr.on('data', (chunk) => { logs += chunk; });
    const exited = new Promise((_, reject) => {
      child.once('exit', (code) => reject(new Error(`Firefox exited before content-script proof (${code})\n${logs}`)));
      child.once('error', reject);
    });
    let timeoutID;
    const timeout = new Promise((_, reject) => {
      timeoutID = setTimeout(() => reject(new Error(`Firefox extension did not call tokenize within 25s (page hits: ${pageHits}; API requests: ${apiRequests.join(', ') || 'none'})\n${logs}`)), 25_000);
    });
    try {
      await Promise.race([tokenized, exited, timeout]);
    } finally {
      clearTimeout(timeoutID);
    }
    console.log('Firefox packaged extension executed its content script.');
  } finally {
    if (child && child.exitCode === null) child.kill('SIGTERM');
    await Promise.all([close(api), close(pageServer)]);
  }
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
