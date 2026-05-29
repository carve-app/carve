/// <reference types="chrome" />

async function init(): Promise<void> {
  const dueEl = document.getElementById('due-count')!;
  const statusEl = document.getElementById('status')!;
  const linkEl = document.getElementById('review-link') as HTMLAnchorElement;

  try {
    const response = await chrome.runtime.sendMessage({ type: 'GET_DUE_COUNT' });
    const count: number = response?.count ?? 0;

    dueEl.textContent = String(count);
    if (count === 0) {
      dueEl.classList.add('zero');
      statusEl.textContent = 'All caught up!';
      linkEl.classList.add('disabled');
      linkEl.removeAttribute('href');
    } else {
      statusEl.textContent = 'Ready to review';
    }
  } catch {
    dueEl.textContent = '—';
    statusEl.textContent = 'Not connected — is the API running?';
    statusEl.classList.add('error');
    linkEl.classList.add('disabled');
    linkEl.removeAttribute('href');
  }
}

init();
