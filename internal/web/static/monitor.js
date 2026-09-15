'use strict';
(() => {
  const version = document.querySelector('meta[name="app-version"]').content;
  let checking = false, reloading = false;
  async function checkVersion() {
    // Preserve form input, login flows and a playing video until the user closes them.
    if (document.hidden || checking || reloading || window.MCH?.signingIn || document.querySelector('dialog[open]')) return;
    checking = true;
    try {
      const response = await fetch('/api/version', {cache:'no-store', credentials:'same-origin'});
      if (!response.ok) return;
      const latest = await response.json();
      if (typeof latest.version === 'string' && /^[a-f0-9]{64}$/.test(latest.version) && latest.version !== version) {
        reloading = true;
        const next = new URL(location.href); next.searchParams.set('v', latest.version); location.replace(next.href);
      } else if (Number.isSafeInteger(latest.revision) && latest.revision !== window.MCH?.revision) {
        await window.MCH?.refreshLibrary?.();
      }
    } catch { /* Keep the working page through network outages and deployments. */ }
    finally { checking = false; }
  }
  document.addEventListener('visibilitychange', checkVersion);
  window.addEventListener('pageshow', checkVersion);
  window.addEventListener('online', checkVersion);
  setInterval(checkVersion, 3000);
})();
