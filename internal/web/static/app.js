'use strict';
(() => {
  const accountButton = document.querySelector('#account-button');
  const loginDialog = document.querySelector('#login-dialog');
  const accountDialog = document.querySelector('#account-dialog');
  const status = document.querySelector('#login-status');

  let user = null;
  let googleScript;
  let signingIn = false;


  async function api(path, options = {}) {
    const response = await fetch(path, { ...options, credentials: 'same-origin', cache: 'no-store' });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || 'Dit is tijdelijk niet beschikbaar. Probeer opnieuw.');
    return body;
  }
  function setUser(value, canEdit = false) {
    window.MCH.user = value;
    window.MCH.canEdit = !!value && canEdit;
    user = value;
    accountButton.querySelector('span').textContent = user ? (user.name?.split(' ')[0] || 'Mijn account') : 'Inloggen';
    document.querySelectorAll('.member-only').forEach(el => el.hidden = !window.MCH.canEdit);
    document.querySelector('#account-permission').textContent = window.MCH.canEdit ? 'Je kunt oefeningen, het clubalbum en de bingo beheren.' : 'Dit account heeft geen beheerrechten.';
    window.dispatchEvent(new CustomEvent('club-auth'));
    if (!user && accountDialog.open) accountDialog.close();
  }
  async function refreshSession() {
    try { const result = await api('/api/auth/me'); setUser(result.user, result.canEdit); }
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
          signingIn = true; window.MCH.signingIn = true;
          status.textContent = 'Bezig met inloggen…';
          try {
            const result = await api('/api/auth/google', {
              method: 'POST', headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ credential }),
            });
            setUser(result.user, result.canEdit);
            loginDialog.close();
            await showAccount();
          } catch (error) {
            // A nonce is single-use. Obtain a new one before offering another attempt.
            await showGoogleButton();
            status.textContent = error.message;
          } finally { signingIn = false; window.MCH.signingIn = false; }
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
      setUser(result.user, result.canEdit);
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
  window.MCH = { api, user: null, canEdit: false, signingIn: false, refreshSession,
    openLogin: async () => { loginDialog.showModal(); await showGoogleButton(); }, showAccount };
  document.addEventListener('visibilitychange', () => { if (!document.hidden) refreshSession(); });
  window.addEventListener('pageshow', refreshSession);
  window.addEventListener('online', refreshSession);
  setInterval(refreshSession, 60 * 60 * 1000);
  refreshSession();
})();
