/// <reference types="chrome" />

const API_DEFAULT = 'http://localhost:8080';

async function getApiBase(): Promise<string> {
  const r = await chrome.storage.local.get('apiBaseUrl');
  return (r['apiBaseUrl'] as string | undefined) ?? API_DEFAULT;
}

async function getToken(): Promise<string | null> {
  const r = await chrome.storage.local.get('accessToken');
  return (r['accessToken'] as string | undefined) ?? null;
}

async function setToken(token: string): Promise<void> {
  await chrome.storage.local.set({ accessToken: token });
}

async function clearToken(): Promise<void> {
  await chrome.storage.local.remove('accessToken');
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
    } catch (err) {
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

  app.innerHTML = `
    <div class="section">
      <div class="label">Cards due today</div>
      <div class="due-count" id="due-count">—</div>
      <div class="status" id="status">Checking…</div>
    </div>
    <a href="http://localhost:5173/cards" target="_blank" class="review-link" id="review-link">
      Open review queue →
    </a>
    <button id="logout-btn">Sign out</button>
  `;

  document.getElementById('logout-btn')!.addEventListener('click', async () => {
    await clearToken();
    showLogin();
  });

  try {
    const base = await getApiBase();
    const res = await fetch(`${base}/v1/review/due-count?language=ja`, {
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
