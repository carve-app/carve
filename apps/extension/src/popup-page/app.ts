import { browser } from '../shared/browser';

declare const __WEB_BASE__: string;

const API_DEFAULT = 'http://localhost:8080';

async function getApiBase(): Promise<string> {
  const r = await browser.storage.local.get('apiBaseUrl');
  return (r['apiBaseUrl'] as string | undefined) ?? API_DEFAULT;
}

async function getToken(): Promise<string | null> {
  const r = await browser.storage.local.get('accessToken');
  return (r['accessToken'] as string | undefined) ?? null;
}

async function setToken(token: string): Promise<void> {
  await browser.storage.local.set({ accessToken: token });
}

async function clearToken(): Promise<void> {
  await browser.storage.local.remove('accessToken');
}

// ── Site toggle helpers ───────────────────────────────────────────────────────

async function getDisabledDomains(): Promise<string[]> {
  const r = await browser.storage.local.get('disabledDomains');
  return (r['disabledDomains'] as string[] | undefined) ?? [];
}

async function isSiteEnabled(hostname: string): Promise<boolean> {
  const disabled = await getDisabledDomains();
  return !disabled.includes(hostname);
}

async function setSiteEnabled(hostname: string, enabled: boolean): Promise<void> {
  const disabled = await getDisabledDomains();
  const next = enabled
    ? disabled.filter(d => d !== hostname)
    : [...new Set([...disabled, hostname])];
  await browser.storage.local.set({ disabledDomains: next });

  // Tell the active tab's content script immediately
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (tab?.id) {
    browser.tabs.sendMessage(tab.id, { type: 'SET_SITE_ENABLED', enabled }).catch(() => {
      // Content script may not be present on this page — ignore
    });
  }
}

// ── Views ─────────────────────────────────────────────────────────────────────

function showLogin(errorMsg?: string): void {
  const app = document.getElementById('app')!;
  app.innerHTML = `
    <div class="section">
      <div class="label">Sign in to Carve</div>
      ${errorMsg ? `<div class="error-msg">${escHtml(errorMsg)}</div>` : ''}
      <form id="login-form">
        <input id="email" type="email" placeholder="Email" autocomplete="email" required />
        <input id="password" type="password" placeholder="Password" autocomplete="current-password" required />
        <button type="submit" id="submit-btn">Sign in</button>
      </form>
    </div>
  `;

  document.getElementById('login-form')!.addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = document.getElementById('submit-btn') as HTMLButtonElement;
    btn.disabled = true;
    btn.textContent = 'Signing in…';

    const email = (document.getElementById('email') as HTMLInputElement).value;
    const password = (document.getElementById('password') as HTMLInputElement).value;

    try {
      const base = await getApiBase();
      const res = await fetch(`${base}/v1/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        const msg = (body as { error?: string }).error ?? `HTTP ${res.status}`;
        showLogin(msg);
        return;
      }

      const data = await res.json() as { access_token: string };
      await setToken(data.access_token);
      await showDueCount();
    } catch {
      showLogin('Could not reach API — is Docker running?');
    }
  });
}

async function showDueCount(): Promise<void> {
  const app = document.getElementById('app')!;

  const token = await getToken();
  if (!token) {
    showLogin();
    return;
  }

  // Get current tab hostname for the site toggle
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  const hostname = tab?.url ? new URL(tab.url).hostname : null;
  const enabled = hostname ? await isSiteEnabled(hostname) : true;

  const siteToggleHTML = hostname ? `
    <div class="site-row">
      <div class="site-row-label">
        Enable on this site
        <span>${escHtml(hostname)}</span>
      </div>
      <label class="toggle" title="${enabled ? 'Disable on this site' : 'Enable on this site'}">
        <input type="checkbox" id="site-toggle" ${enabled ? 'checked' : ''} />
        <div class="toggle-track"></div>
      </label>
    </div>
  ` : '';

  // Comprehension overlay state
  let overlayOn = false;
  browser.storage.local.get('overlayEnabled').then(r => {
    overlayOn = !!r['overlayEnabled'];
    const cb = document.getElementById('overlay-toggle') as HTMLInputElement | null;
    if (cb) cb.checked = overlayOn;
    updateOverlayBadge();
  });

  const langState = await browser.storage.local.get(['targetLanguage', 'annotateLatinSites']);
  const currentLang = (langState['targetLanguage'] as string | undefined) ?? 'ja';
  const annotateLatin = !!langState['annotateLatinSites'];

  app.innerHTML = `
    ${siteToggleHTML}
    <div class="overlay-row">
      <div class="overlay-row-label">Target language</div>
      <select id="lang-select" class="lang-select">
        <option value="ja"   ${currentLang === 'ja'    ? 'selected' : ''}>Japanese 日本語</option>
        <option value="zh-cn"${currentLang === 'zh-cn' ? 'selected' : ''}>Chinese 中文</option>
        <option value="ko"   ${currentLang === 'ko'    ? 'selected' : ''}>Korean 한국어</option>
        <option value="en"   ${currentLang === 'en'    ? 'selected' : ''}>English</option>
      </select>
    </div>
    ${currentLang === 'en' ? `
    <div class="overlay-row">
      <div class="overlay-row-label" title="When on, Carve annotates English-language pages. Otherwise it stays inert on Latin-script sites.">
        Annotate English pages
      </div>
      <label class="toggle">
        <input type="checkbox" id="latin-toggle" ${annotateLatin ? 'checked' : ''} />
        <div class="toggle-track"></div>
      </label>
    </div>
    ` : ''}
    <div class="overlay-row">
      <div class="overlay-row-label">
        Comprehension overlay
        <span class="overlay-pct-badge" id="overlay-pct"></span>
      </div>
      <label class="toggle" title="Show comprehension score on page">
        <input type="checkbox" id="overlay-toggle" />
        <div class="toggle-track"></div>
      </label>
    </div>
    <div class="section">
      <div class="label">Cards due today</div>
      <div class="due-count" id="due-count">—</div>
      <div class="status" id="status">Checking…</div>
    </div>
    <a href="${__WEB_BASE__}/cards" target="_blank" class="review-link" id="review-link">
      Open review queue →
    </a>
    <button id="logout-btn">Sign out</button>
  `;

  document.getElementById('lang-select')!.addEventListener('change', async (e) => {
    const v = (e.target as HTMLSelectElement).value;
    await browser.storage.local.set({ targetLanguage: v });
    location.reload();  // simplest: rebuild the popup with the new selection
  });

  document.getElementById('latin-toggle')?.addEventListener('change', async (e) => {
    const checked = (e.target as HTMLInputElement).checked;
    await browser.storage.local.set({ annotateLatinSites: checked });
  });

  async function updateOverlayBadge(): Promise<void> {
    const badge = document.getElementById('overlay-pct');
    if (!badge) return;
    try {
      const [activeTab] = await browser.tabs.query({ active: true, currentWindow: true });
      if (!activeTab?.id) return;
      const response = await browser.tabs.sendMessage(activeTab.id, { type: 'GET_COMPREHENSION' }) as { pct: number | null } | undefined;
      badge.textContent = response?.pct != null ? `${response.pct}%` : '';
    } catch {
      badge.textContent = '';
    }
  }

  document.getElementById('overlay-toggle')!.addEventListener('change', async (e) => {
    const checked = (e.target as HTMLInputElement).checked;
    await browser.storage.local.set({ overlayEnabled: checked });
    const [activeTab] = await browser.tabs.query({ active: true, currentWindow: true });
    if (activeTab?.id) {
      try {
        await browser.tabs.sendMessage(activeTab.id, { type: 'SET_OVERLAY', enabled: checked });
      } catch { /* tab may not have content script */ }
    }
    if (checked) updateOverlayBadge();
  });

  if (hostname) {
    document.getElementById('site-toggle')!.addEventListener('change', async (e) => {
      const checked = (e.target as HTMLInputElement).checked;
      await setSiteEnabled(hostname, checked);
    });
  }

  document.getElementById('logout-btn')!.addEventListener('click', async () => {
    await clearToken();
    showLogin();
  });

  try {
    const base = await getApiBase();
    const res = await fetch(`${base}/v1/review/due-count?language=${encodeURIComponent(currentLang)}`, {
      headers: { Authorization: `Bearer ${token}` },
    });

    if (res.status === 401) {
      await clearToken();
      showLogin('Session expired — please sign in again');
      return;
    }

    const data = await res.json() as { due_count: number };
    const count = data.due_count ?? 0;
    const dueEl = document.getElementById('due-count')!;
    const statusEl = document.getElementById('status')!;
    const linkEl = document.getElementById('review-link') as HTMLAnchorElement;

    dueEl.textContent = String(count);
    dueEl.className = 'due-count' + (count === 0 ? ' zero' : '');

    if (count === 0) {
      statusEl.textContent = 'All caught up!';
      linkEl.classList.add('disabled');
      linkEl.removeAttribute('href');
    } else {
      statusEl.textContent = 'Ready to review';
    }
  } catch {
    const statusEl = document.getElementById('status')!;
    statusEl.textContent = 'Could not reach API — is Docker running?';
    statusEl.classList.add('error');
  }
}

function escHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// ── Boot ──────────────────────────────────────────────────────────────────────

(async () => {
  const token = await getToken();
  if (token) {
    await showDueCount();
  } else {
    showLogin();
  }
})();
