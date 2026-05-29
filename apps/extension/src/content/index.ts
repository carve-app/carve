/// <reference types="chrome" />

function init(): void {
	const lang = document.documentElement.lang?.slice(0, 2);
	if (lang !== 'ja') return;

	console.log('[Carve content] Japanese page detected, NLP ready');
}

if (document.readyState === 'loading') {
	document.addEventListener('DOMContentLoaded', init);
} else {
	init();
}
