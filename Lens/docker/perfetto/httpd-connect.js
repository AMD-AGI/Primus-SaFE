// httpd-connect.js - Intercept HTTP RPC requests to 127.0.0.1:9001 and redirect to nginx proxy
// This allows Perfetto UI to use server-side trace_processor via our proxy
(function() {
  'use strict';
  
  // Build proxy URL based on current location
  const basePath = window.location.pathname.replace(/\/$/, '');
  const RPC_PROXY_BASE = window.location.origin + basePath + '/rpc';
  
  console.log('[HTTPD] Installing RPC interceptor, proxy base:', RPC_PROXY_BASE);
  
  // Pattern to match trace_processor HTTP server
  const TP_PATTERN = /^https?:\/\/(127\.0\.0\.1|localhost):9001/;
  
  // Intercept fetch requests
  const originalFetch = window.fetch;
  window.fetch = function(input, init) {
    let url = typeof input === 'string' ? input : (input instanceof Request ? input.url : String(input));
    
    if (TP_PATTERN.test(url)) {
      // Redirect to our proxy
      const urlObj = new URL(url);
      const newUrl = RPC_PROXY_BASE + urlObj.pathname + urlObj.search;
      console.log('[HTTPD] Redirecting fetch:', url, '->', newUrl);
      
      // Clone init and ensure CORS mode
      const newInit = init ? { ...init } : {};
      newInit.mode = 'cors';
      
      if (typeof input === 'string') {
        return originalFetch.call(this, newUrl, newInit);
      } else if (input instanceof Request) {
        return originalFetch.call(this, new Request(newUrl, input), newInit);
      }
    }
    
    return originalFetch.apply(this, arguments);
  };
  
  // Intercept XMLHttpRequest
  const originalXHROpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
    if (typeof url === 'string' && TP_PATTERN.test(url)) {
      const urlObj = new URL(url);
      const newUrl = RPC_PROXY_BASE + urlObj.pathname + urlObj.search;
      console.log('[HTTPD] Redirecting XHR:', url, '->', newUrl);
      return originalXHROpen.call(this, method, newUrl, async !== false, user, password);
    }
    return originalXHROpen.apply(this, arguments);
  };
  
  // Intercept WebSocket connections (Perfetto might use WebSocket for RPC)
  const originalWebSocket = window.WebSocket;
  window.WebSocket = function(url, protocols) {
    if (typeof url === 'string' && TP_PATTERN.test(url)) {
      // WebSocket proxy requires different handling
      // For now, just log - we'd need wss proxy setup for this
      console.log('[HTTPD] WebSocket to trace_processor detected:', url);
    }
    return new originalWebSocket(url, protocols);
  };
  window.WebSocket.prototype = originalWebSocket.prototype;
  window.WebSocket.CONNECTING = originalWebSocket.CONNECTING;
  window.WebSocket.OPEN = originalWebSocket.OPEN;
  window.WebSocket.CLOSING = originalWebSocket.CLOSING;
  window.WebSocket.CLOSED = originalWebSocket.CLOSED;
  
  // Check if RPC proxy is available and log status
  async function checkRpcStatus() {
    try {
      const response = await originalFetch.call(window, RPC_PROXY_BASE + '/status', {
        method: 'GET',
        mode: 'cors'
      });
      if (response.ok) {
        const text = await response.text();
        console.log('[HTTPD] RPC proxy status:', text);
        return true;
      }
    } catch (e) {
      console.warn('[HTTPD] RPC proxy not available:', e.message);
    }
    return false;
  }
  
  // Trigger Perfetto to check for HTTP RPC by simulating availability
  // Perfetto calls fetch('http://127.0.0.1:9001/status') to check
  // Our interceptor will redirect this to /rpc/status
  
  // Wait for page load and check RPC status
  window.addEventListener('load', function() {
    setTimeout(function() {
      checkRpcStatus().then(available => {
        if (available) {
          console.log('[HTTPD] trace_processor HTTP RPC is available via proxy');
          console.log('[HTTPD] Perfetto should auto-detect and offer "Use loaded trace" option');
        }
      });
    }, 1000);
  });
  
  console.log('[HTTPD] RPC interceptor installed successfully');
})();
