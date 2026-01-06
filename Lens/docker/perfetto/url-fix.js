// URL fix script - runs before Perfetto loads
// Removes url parameter from hash to prevent "Invalid URL" error
// Our auto-load.js will handle trace loading instead
(function() {
  'use strict';
  
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
})();

