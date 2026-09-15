'use strict';
(() => {
  const accountButton = document.querySelector('#account-button');
  const loginDialog = document.querySelector('#login-dialog');
  const accountDialog = document.querySelector('#account-dialog');
  const status = document.querySelector('#login-status');
  const version = document.querySelector('meta[name="app-version"]').content;
  let user = null;
  let googleScript;
  let signingIn = false;
  let checkingVersion = false;
  let reloading = false;

  async function api(path, options = {}) {
    const response = await fetch(path, { ...options, credentials: 'same-origin', cache: 'no-store' });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || 'Dit is tijdelijk niet beschikbaar. Probeer opnieuw.');
    return body;
  }
  function setUser(value) {
    user = value;
    accountButton.textContent = user ? 'Mijn account' : 'Inloggen';
    if (!user && accountDialog.open) accountDialog.close();
  }
  async function refreshSession() {
    try { setUser((await api('/api/auth/me')).user); }
    catch { /* A temporary outage must not turn a signed-in visitor into a logged-out one. */ }
  }
  function loadGoogle() {
    if (!googleScript) {
      googleScript = new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = 'https://accounts.google.com/gsi/client';
        script.async = true;
        script.onload = resolve;
        script.onerror = () => { script.remove(); googleScript = null; reject(new Error('Google kon niet worden geladen. Probeer opnieuw.')); };
        document.head.append(script);
      });
    }
    return googleScript;
  }
  async function showGoogleButton() {
    status.textContent = 'Google-login laden…';
    document.querySelector('#google-button').replaceChildren();
    try {
      const config = await api('/api/auth/config');
      if (!config.enabled) { status.textContent = 'Google-login wordt binnenkort beschikbaar.'; return; }
      await loadGoogle();
      if (!loginDialog.open) return;
      window.google.accounts.id.initialize({
        client_id: config.clientId,
        nonce: config.nonce,
        auto_select: false,
        callback: async ({ credential }) => {
          if (signingIn) return;
          signingIn = true;
          status.textContent = 'Bezig met inloggen…';
          try {
            const result = await api('/api/auth/google', {
              method: 'POST', headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ credential }),
            });
            setUser(result.user);
            loginDialog.close();
            await showAccount();
          } catch (error) {
            // A nonce is single-use. Obtain a new one before offering another attempt.
            await showGoogleButton();
            status.textContent = error.message;
          } finally { signingIn = false; }
        },
      });
      window.google.accounts.id.renderButton(document.querySelector('#google-button'), {
        type: 'standard', theme: 'outline', size: 'large', text: 'signin_with', locale: 'nl',
      });
      status.textContent = '';
    } catch (error) { status.textContent = error.message; }
  }
  async function showAccount() {
    try {
      const result = await api('/api/account');
      setUser(result.user);
      document.querySelector('#account-name').textContent = user.name || 'Google-account';
      document.querySelector('#account-email').textContent = user.email;
      document.querySelector('#account-status').textContent = '';
      if (!accountDialog.open) accountDialog.showModal();
    } catch (error) {
      await refreshSession();
      if (!user) { loginDialog.showModal(); await showGoogleButton(); }
      else { document.querySelector('#account-status').textContent = error.message; accountDialog.showModal(); }
    }
  }
  accountButton.addEventListener('click', async () => {
    if (user) await showAccount();
    else { loginDialog.showModal(); await showGoogleButton(); }
  });
  document.querySelectorAll('.close-dialog').forEach(button => {
    button.addEventListener('click', () => button.closest('dialog').close());
  });
  document.querySelector('#logout-button').addEventListener('click', async event => {
    event.target.disabled = true;
    try {
      await api('/api/auth/logout', { method: 'POST' });
      window.google?.accounts.id.disableAutoSelect();
      setUser(null);
    } catch (error) { document.querySelector('#account-status').textContent = error.message; }
    finally { event.target.disabled = false; }
  });
  async function checkVersion() {
    if (document.hidden || checkingVersion || reloading || signingIn) return;
    checkingVersion = true;
    try {
      const latest = await api('/api/version');
      if (typeof latest.version === 'string' && /^[a-f0-9]{64}$/.test(latest.version) && latest.version !== version) {
        reloading = true;
        const next = new URL(location.href);
        next.searchParams.set('v', latest.version);
        location.replace(next.href);
      }
    } catch { /* Keep the working page during network outages and deployments. */ }
    finally { checkingVersion = false; }
  }
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) { checkVersion(); refreshSession(); }
  });
  window.addEventListener('pageshow', () => { checkVersion(); refreshSession(); });
  window.addEventListener('online', () => { checkVersion(); refreshSession(); });
  setInterval(checkVersion, 3000);
  setInterval(refreshSession, 60 * 60 * 1000);
  refreshSession();
})();
