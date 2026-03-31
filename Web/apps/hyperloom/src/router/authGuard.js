import { ssoLoginRaw, getUserSelf } from '@/services/auth.js';

const PUBLIC_PATHS = new Set(['/login']);
const ENTRY_KEY = 'hl-sso.redirect';
const STATE_KEY = 'hl-oauth_state';

function isPublicRoute(to) {
  const p = (to.path || '/').replace(/\/+$/, '') || '/';
  return PUBLIC_PATHS.has(p);
}

export default function setupAuthGuard(router) {
  router.beforeEach(async (to, _from, next) => {
    const code = to.query.code;

    if (code) {
      const state = to.query.state;

      let stateRandom = state;
      if (typeof state === 'string' && state.startsWith('hyperloom:')) {
        const parts = state.split(':');
        stateRandom = parts[1];
      }

      const saved = sessionStorage.getItem(STATE_KEY);
      if (saved && stateRandom && saved !== stateRandom) {
        history.replaceState(null, '', to.path);
        return next({ path: '/login', query: { error: 'state_mismatch' } });
      }

      try {
        const resp = await ssoLoginRaw(code, state);
        const loginOk = resp.status >= 200 && resp.status < 300 && resp.data?.id;

        if (loginOk) {
          sessionStorage.removeItem(STATE_KEY);

          localStorage.setItem(
            'hl-user',
            JSON.stringify({
              id: resp.data.id,
              name: resp.data.name,
              email: resp.data.email,
              session: 'authenticated',
            }),
          );

          const target = sessionStorage.getItem(ENTRY_KEY) || '/overview';
          sessionStorage.removeItem(ENTRY_KEY);
          history.replaceState(null, '', to.path);
          return next(target);
        }

        const errMsg = resp.data?.errorMessage || resp.data?.message || '';
        history.replaceState(null, '', to.path);
        return next({ path: '/login', query: { error: 'sso_failed', detail: errMsg } });
      } catch (err) {
        console.error('[HyperLoom Auth] SSO callback error:', err);
        history.replaceState(null, '', to.path);
        return next({ path: '/login', query: { error: 'sso_failed' } });
      }
    }

    if (to.query.error && to.path !== '/login') {
      history.replaceState(null, '', to.path);
      return next({
        path: '/login',
        query: { error: to.query.error, error_description: to.query.error_description },
      });
    }

    const stored = localStorage.getItem('hl-user');
    if (stored) {
      try {
        const user = JSON.parse(stored);
        if (user?.session === 'authenticated') {
          if (isPublicRoute(to)) {
            return next(to.query.redirect || '/overview');
          }
          return next();
        }
      } catch {
        localStorage.removeItem('hl-user');
      }
    }

    // Dev mode: try to probe existing session via proxy before forcing login.
    // If the user already authenticated on the SaFE domain and we have a valid
    // cookie (e.g. via manual cookie bridge), this will succeed silently.
    if (!isPublicRoute(to)) {
      try {
        const profile = await getUserSelf();
        if (profile?.id) {
          localStorage.setItem(
            'hl-user',
            JSON.stringify({
              id: profile.id,
              name: profile.name,
              email: profile.email,
              session: 'authenticated',
            }),
          );
          return next();
        }
      } catch {
        // Not authenticated — fall through to login redirect
      }

      sessionStorage.setItem(ENTRY_KEY, to.fullPath);
      return next({ path: '/login', query: { redirect: to.fullPath } });
    }

    return next();
  });
}
