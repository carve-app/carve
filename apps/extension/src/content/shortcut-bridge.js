(() => {
  if (window.__carveVideoShortcutBridge) return;
  window.__carveVideoShortcutBridge = true;

  const eventName = 'carve:video-shortcut';

  function isEditableTarget(target) {
    return target instanceof HTMLElement
      && (target instanceof HTMLInputElement
        || target instanceof HTMLTextAreaElement
        || target.isContentEditable);
  }

  function actionFor(event) {
    if (event.key === 'ArrowLeft') return 'prev';
    if (event.key === 'ArrowRight') return 'next';
    if (event.key && event.key.toLowerCase() === 'm' && !event.ctrlKey && !event.metaKey) return 'mine';
    return null;
  }

  function handle(event) {
    if (isEditableTarget(event.target)) return;
    if (!document.getElementById('carve-sub-overlay')) return;

    const action = actionFor(event);
    if (!action) return;

    event.preventDefault();
    event.stopImmediatePropagation();
    if (event.type === 'keydown') {
      window.dispatchEvent(new CustomEvent(eventName, { detail: { action } }));
    }
  }

  window.addEventListener('keydown', handle, true);
  window.addEventListener('keypress', handle, true);
  window.addEventListener('keyup', handle, true);
})();

