// URL fix script - runs before Perfetto loads
// 1. Removes url parameter from hash to prevent "Invalid URL" error
// 2. Mocks Google internal user check to prevent external requests
(function() {
  'use strict';
  
  // Fix 1: Remove url parameter from hash
  // In HTTP RPC mode, we don't need the url parameter as trace is already loaded
  if (window.location.hash.includes('url=')) {
    console.log('[URLFix] Removing url parameter from hash');
    var newHash = window.location.hash
      .replace(/[?&]url=[^&]*/g, '')
      .replace(/\?$/, '')
      .replace(/&$/, '');
    
    if (newHash === '#!' || newHash === '#!/viewer' || newHash === '#!/viewer?') {
      newHash = '';
    }
    
    if (newHash !== window.location.hash) {
      console.log('[URLFix] New hash:', newHash || '(empty)');
      history.replaceState(null, '', window.location.pathname + window.location.search + newHash);
    }
  }
  
  // Fix 2: Mock Google internal user check
  // Perfetto tries to load https://storage.cloud.google.com/perfetto-ui-internal/is_internal_user.js
  // This causes 403 errors in private deployments. We mock it.
  window.PFTUI_IS_INTERNAL_USER = false;
  
  // Intercept fetch requests to Google storage
  var originalFetch = window.fetch;
  window.fetch = function(url, options) {
    if (typeof url === 'string' && url.includes('storage.cloud.google.com')) {
      console.log('[URLFix] Blocking fetch to Google storage:', url);
      return Promise.resolve(new Response('', { status: 200 }));
    }
    return originalFetch.apply(this, arguments);
  };
  
  // Use MutationObserver to block Google scripts instead of redefining properties
  // This is safer and doesn't cause "Cannot redefine property" errors
  var observer = new MutationObserver(function(mutations) {
    mutations.forEach(function(mutation) {
      mutation.addedNodes.forEach(function(node) {
        if (node.tagName === 'SCRIPT' && node.src && node.src.includes('storage.cloud.google.com')) {
          console.log('[URLFix] Removing Google script:', node.src);
          node.remove();
        }
      });
    });
  });
  
  // Start observing when DOM is ready
  if (document.head) {
    observer.observe(document.head, { childList: true });
  }
  if (document.body) {
    observer.observe(document.body, { childList: true });
  }
  
  // Also observe document element for early script additions
  observer.observe(document.documentElement, { childList: true, subtree: true });
  
  console.log('[URLFix] Initialized - URL hash fixed, Google requests blocked');
})();
