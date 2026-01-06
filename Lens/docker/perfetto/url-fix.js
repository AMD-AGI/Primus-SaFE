// URL fix script - runs before Perfetto loads
// 1. Removes url parameter from hash to prevent "Invalid URL" error
// 2. Mocks Google internal user check to prevent external requests
(function() {
  'use strict';
  
  // Fix 1: Remove url parameter from hash
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
  // This causes 403 errors in private deployments. We mock it by defining the expected global.
  window.PFTUI_IS_INTERNAL_USER = false;
  
  // Also intercept fetch/script requests to Google storage
  var originalFetch = window.fetch;
  window.fetch = function(url, options) {
    if (typeof url === 'string' && url.includes('storage.cloud.google.com')) {
      console.log('[URLFix] Blocking request to Google storage:', url);
      return Promise.resolve(new Response('', { status: 200 }));
    }
    return originalFetch.apply(this, arguments);
  };
  
  // Intercept dynamic script loading
  var originalCreateElement = document.createElement.bind(document);
  document.createElement = function(tagName) {
    var element = originalCreateElement(tagName);
    if (tagName.toLowerCase() === 'script') {
      var originalSetAttribute = element.setAttribute.bind(element);
      element.setAttribute = function(name, value) {
        if (name === 'src' && typeof value === 'string' && value.includes('storage.cloud.google.com')) {
          console.log('[URLFix] Blocking script load from Google storage:', value);
          return; // Don't set the src
        }
        return originalSetAttribute(name, value);
      };
      
      // Also intercept direct src assignment
      Object.defineProperty(element, 'src', {
        set: function(value) {
          if (typeof value === 'string' && value.includes('storage.cloud.google.com')) {
            console.log('[URLFix] Blocking script src from Google storage:', value);
            return;
          }
          originalSetAttribute('src', value);
        },
        get: function() {
          return element.getAttribute('src');
        }
      });
    }
    return element;
  };
  
  console.log('[URLFix] Initialized - URL hash fixed, Google requests blocked');
})();

