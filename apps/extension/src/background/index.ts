/// <reference types="chrome" />

chrome.runtime.onInstalled.addListener(() => {
	console.log('[Carve] Extension installed');
});

chrome.runtime.onMessage.addListener(
	(message: unknown, _sender, sendResponse) => {
		console.log('[Carve background] message received', message);
		sendResponse({ ok: true });
		return true;
	}
);
