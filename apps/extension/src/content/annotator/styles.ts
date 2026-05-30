export function injectStyles(): void {
  if (document.getElementById('carve-styles')) return;

  const style = document.createElement('style');
  style.id = 'carve-styles';
  style.textContent = `
    /* Token colorization */
    [data-carve="token"][data-status="unknown"] {
      cursor: pointer;
    }
    [data-carve="token"][data-status="learning"] {
      background: rgba(100, 160, 255, 0.2);
      border-radius: 2px;
      cursor: pointer;
    }
    [data-carve="token"][data-status="known"] {
      cursor: pointer;
    }
    /* Frequency-band underlines for unknown content words */
    [data-carve="token"][data-status="unknown"][data-band="green"] {
      border-bottom: 2px solid #1db954;
    }
    [data-carve="token"][data-status="unknown"][data-band="yellow"] {
      border-bottom: 2px solid #ffa726;
    }
    [data-carve="token"][data-status="unknown"][data-band="red"] {
      border-bottom: 2px solid #ef5350;
    }
    /* Popup container */
    #carve-popup {
      position: fixed;
      z-index: 2147483647;
      background: #1e2128;
      color: #e8eaf0;
      border-radius: 8px;
      box-shadow: 0 4px 24px rgba(0,0,0,0.5);
      padding: 12px 16px;
      max-width: 340px;
      min-width: 240px;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 14px;
      line-height: 1.5;
    }
    #carve-popup .carve-word {
      font-size: 22px;
      font-weight: 600;
      margin-bottom: 2px;
    }
    #carve-popup .carve-reading {
      font-size: 13px;
      color: #9ba8c0;
      margin-bottom: 8px;
    }
    #carve-popup .carve-furigana {
      display: flex;
      flex-wrap: wrap;
      gap: 0;
      font-size: 22px;
      margin-bottom: 6px;
    }
    #carve-popup .carve-furigana ruby {
      display: inline-flex;
      flex-direction: column;
      align-items: center;
    }
    #carve-popup .carve-furigana rt {
      font-size: 9px;
      color: #9ba8c0;
      line-height: 1.2;
    }
    #carve-popup .carve-defs {
      margin: 8px 0;
    }
    #carve-popup .carve-def {
      margin: 3px 0;
      font-size: 13px;
    }
    #carve-popup .carve-pos {
      font-size: 11px;
      color: #6b7a99;
      font-style: italic;
    }
    #carve-popup .carve-status {
      display: inline-block;
      padding: 1px 6px;
      border-radius: 12px;
      font-size: 11px;
      font-weight: 600;
      margin-bottom: 8px;
    }
    #carve-popup .carve-status.unknown { background: rgba(200,60,60,0.3); color: #ff8888; }
    #carve-popup .carve-status.learning { background: rgba(100,160,255,0.3); color: #88aaff; }
    #carve-popup .carve-status.known { background: rgba(80,200,120,0.3); color: #88cc88; }
    #carve-popup .carve-actions {
      display: flex;
      gap: 8px;
      margin-top: 10px;
    }
    #carve-popup button {
      flex: 1;
      padding: 6px 10px;
      border: none;
      border-radius: 6px;
      cursor: pointer;
      font-size: 12px;
      font-weight: 600;
      transition: opacity 0.15s;
    }
    #carve-popup button:hover { opacity: 0.85; }
    #carve-popup .btn-mine { background: #4CAF50; color: white; }
    #carve-popup .btn-ignore { background: #37404e; color: #9ba8c0; }
    #carve-popup .carve-sentence {
      font-size: 11px;
      color: #6b7a99;
      margin-top: 6px;
      padding-top: 6px;
      border-top: 1px solid #2d3344;
      line-height: 1.4;
    }
    #carve-popup .carve-jlpt {
      font-size: 11px;
      color: #9ba8c0;
      margin-left: 4px;
    }
    #carve-popup .carve-pitch {
      font-size: 11px;
      color: #b8a0d0;
      margin-left: 6px;
      font-family: monospace;
    }
    /* Comprehension score overlay */
    #carve-overlay {
      position: fixed;
      bottom: 20px;
      right: 20px;
      z-index: 2147483646;
      background: rgba(30, 33, 40, 0.92);
      border: 1px solid #2a2d36;
      border-radius: 10px;
      padding: 10px 14px;
      font-family: system-ui, sans-serif;
      font-size: 13px;
      color: #e8eaf0;
      min-width: 130px;
      text-align: center;
      box-shadow: 0 4px 16px rgba(0,0,0,0.4);
      backdrop-filter: blur(4px);
      cursor: default;
      user-select: none;
    }
    #carve-overlay .overlay-label {
      font-size: 10px;
      color: #6b7a99;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      margin-bottom: 4px;
    }
    #carve-overlay .overlay-pct {
      font-size: 26px;
      font-weight: 700;
    }
    #carve-overlay .overlay-sub {
      font-size: 10px;
      color: #6b7a99;
      margin-top: 2px;
    }
    #carve-overlay .overlay-close {
      position: absolute;
      top: 4px;
      right: 7px;
      font-size: 14px;
      color: #6b7a99;
      cursor: pointer;
      line-height: 1;
    }
    #carve-overlay .overlay-close:hover { color: #e8eaf0; }
  `;
  document.head.appendChild(style);
}
