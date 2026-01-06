// httpd-connect.js - Configures Perfetto UI to use the local trace_processor HTTP server
// This enables server-side trace processing for instant viewing without browser-side WASM

(function() {
  'use strict';
  
  // The trace_processor HTTP server is running on the same host via nginx proxy
  // Perfetto UI will connect to /rpc/ which nginx proxies to 127.0.0.1:9001
  const RPC_BASE_URL = window.location.origin + window.location.pathname.replace(/\/$/, '');
  
  console.log('[HTTPD] Configuring Perfetto to use HTTP RPC at:', RPC_BASE_URL);
  
  // Set global configuration for Perfetto
  // Perfetto looks for these globals to configure the HTTP RPC connection
  window.PERFETTO_RPC_URL = RPC_BASE_URL;
  
  // Override the default RPC port check behavior
  // Perfetto normally checks 127.0.0.1:9001, but we proxy through nginx
  window.PERFETTO_HTTP_RPC_OVERRIDE = true;
  
  // Function to wait for Perfetto globals and configure the HTTP RPC
  function configurePerfettoRpc() {
    console.log('[HTTPD] Waiting for Perfetto to initialize...');
    
    let attempts = 0;
    const maxAttempts = 100;
    
    const check = () => {
      attempts++;
      
      // Check if Perfetto's app is initialized
      if (typeof globals !== 'undefined' && globals.httpRpcState !== undefined) {
        console.log('[HTTPD] Perfetto globals found, configuring HTTP RPC...');
        
        // Set the HTTP RPC state to connected
        try {
          if (globals.dispatch) {
            globals.dispatch({
              type: 'SET_HTTP_RPC_STATE',
              httpRpcState: {
                connected: true,
                status: 'CONNECTED'
              }
            });
            console.log('[HTTPD] HTTP RPC state set to connected');
          }
        } catch (e) {
          console.warn('[HTTPD] Could not set HTTP RPC state:', e);
        }
        return;
      }
      
      // Try alternative: check for perfetto app
      if (typeof app !== 'undefined' && app.httpRpc) {
        console.log('[HTTPD] Perfetto app found with httpRpc');
        return;
      }
      
      if (attempts >= maxAttempts) {
        console.log('[HTTPD] Timeout waiting for Perfetto, HTTP RPC may work automatically');
        return;
      }
      
      setTimeout(check, 100);
    };
    
    // Start checking after a short delay
    setTimeout(check, 500);
  }
  
  // Check if trace_processor server is available
  async function checkServerStatus() {
    try {
      const response = await fetch('/status');
      if (response.ok) {
        console.log('[HTTPD] trace_processor server is available');
        return true;
      }
    } catch (e) {
      console.warn('[HTTPD] trace_processor server not responding:', e);
    }
    return false;
  }
  
  // Auto-open trace using HTTP RPC
  async function autoOpenTrace() {
    console.log('[HTTPD] Checking if trace is loaded on server...');
    
    const serverReady = await checkServerStatus();
    if (!serverReady) {
      console.log('[HTTPD] Server not ready, will retry...');
      setTimeout(autoOpenTrace, 2000);
      return;
    }
    
    // Wait for Perfetto app to be ready
    let attempts = 0;
    const maxAttempts = 50;
    
    const tryOpen = () => {
      attempts++;
      
      // Check if app is available and can open trace from HTTP RPC
      if (typeof globals !== 'undefined' && globals.dispatch) {
        console.log('[HTTPD] Opening trace via HTTP RPC...');
        
        try {
          // Dispatch action to connect to HTTP RPC and load trace
          globals.dispatch({
            type: 'OPEN_TRACE_FROM_HTTP_RPC',
          });
          console.log('[HTTPD] Trace open request dispatched');
          return;
        } catch (e) {
          console.warn('[HTTPD] Could not dispatch open trace:', e);
        }
      }
      
      // Alternative: try using app.openTraceFromHttpRpc if available
      if (typeof app !== 'undefined') {
        if (app.openTraceFromHttpRpc) {
          console.log('[HTTPD] Using app.openTraceFromHttpRpc');
          app.openTraceFromHttpRpc();
          return;
        }
        if (app.openTrace) {
          console.log('[HTTPD] Using app.openTrace with httpRpc source');
          app.openTrace({ source: 'HTTP_RPC' });
          return;
        }
      }
      
      if (attempts >= maxAttempts) {
        console.log('[HTTPD] Timeout waiting for Perfetto app');
        console.log('[HTTPD] The trace should load automatically when you click "Use loaded trace"');
        return;
      }
      
      setTimeout(tryOpen, 200);
    };
    
    // Start trying after page load
    if (document.readyState === 'complete') {
      setTimeout(tryOpen, 1000);
    } else {
      window.addEventListener('load', () => setTimeout(tryOpen, 1000));
    }
  }
  
  // Initialize
  function init() {
    console.log('[HTTPD] Initializing HTTP RPC connection...');
    configurePerfettoRpc();
    autoOpenTrace();
  }
  
  // Remove URL hash parameters that might interfere
  if (window.location.hash.includes('url=')) {
    console.log('[HTTPD] Removing url parameter from hash');
    const newHash = window.location.hash
      .replace(/[?&]url=[^&]*/g, '')
      .replace(/\?$/, '')
      .replace(/&$/, '');
    
    if (newHash === '#!' || newHash === '#!/viewer' || newHash === '#!/viewer?') {
      history.replaceState(null, '', window.location.pathname + window.location.search);
    } else if (newHash !== window.location.hash) {
      history.replaceState(null, '', window.location.pathname + window.location.search + newHash);
    }
  }
  
  init();
})();

