// httpd-connect.js - Intercept HTTP RPC and WebSocket requests to 127.0.0.1:9001
// Redirects to nginx proxy for server-side trace_processor
(function() {
  'use strict';
  
  // Build proxy URLs based on current location
  var basePath = window.location.pathname.replace(/\/$/, '');
  var RPC_PROXY_BASE = window.location.origin + basePath + '/rpc';
  
  // WebSocket URL: wss:// for https://, ws:// for http://
  var wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  var WS_PROXY_BASE = wsProtocol + '//' + window.location.host + basePath + '/rpc';
  
  console.log('[HTTPD] RPC proxy base:', RPC_PROXY_BASE);
  console.log('[HTTPD] WebSocket proxy base:', WS_PROXY_BASE);
  
  // Pattern to match trace_processor HTTP server
  var TP_PATTERN = /^https?:\/\/(127\.0\.0\.1|localhost):9001/;
  var TP_WS_PATTERN = /^wss?:\/\/(127\.0\.0\.1|localhost):9001/;
  
  // Intercept fetch requests
  var originalFetch = window.fetch;
  window.fetch = function(input, init) {
    var url = typeof input === 'string' ? input : (input instanceof Request ? input.url : String(input));
    
    if (TP_PATTERN.test(url)) {
      var urlObj = new URL(url);
      var newUrl = RPC_PROXY_BASE + urlObj.pathname + urlObj.search;
      console.log('[HTTPD] Redirecting fetch:', url, '->', newUrl);
      
      var newInit = init ? Object.assign({}, init) : {};
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
  var originalXHROpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
    if (typeof url === 'string' && TP_PATTERN.test(url)) {
      var urlObj = new URL(url);
      var newUrl = RPC_PROXY_BASE + urlObj.pathname + urlObj.search;
      console.log('[HTTPD] Redirecting XHR:', url, '->', newUrl);
      return originalXHROpen.call(this, method, newUrl, async !== false, user, password);
    }
    return originalXHROpen.apply(this, arguments);
  };
  
  // Intercept WebSocket connections
  var OriginalWebSocket = window.WebSocket;
  window.WebSocket = function(url, protocols) {
    var newUrl = url;
    
    if (typeof url === 'string' && TP_WS_PATTERN.test(url)) {
      // Parse the original URL and redirect to our proxy
      var urlObj = new URL(url);
      newUrl = WS_PROXY_BASE + urlObj.pathname + urlObj.search;
      console.log('[HTTPD] Redirecting WebSocket:', url, '->', newUrl);
    }
    
    if (protocols !== undefined) {
      return new OriginalWebSocket(newUrl, protocols);
    } else {
      return new OriginalWebSocket(newUrl);
    }
  };
  
  // Copy static properties and prototype
  window.WebSocket.prototype = OriginalWebSocket.prototype;
  window.WebSocket.CONNECTING = OriginalWebSocket.CONNECTING;
  window.WebSocket.OPEN = OriginalWebSocket.OPEN;
  window.WebSocket.CLOSING = OriginalWebSocket.CLOSING;
  window.WebSocket.CLOSED = OriginalWebSocket.CLOSED;
  
  // Check if RPC proxy is available
  function checkRpcStatus() {
    originalFetch.call(window, RPC_PROXY_BASE + '/status', {
      method: 'GET',
      mode: 'cors'
    }).then(function(response) {
      if (response.ok) {
        return response.text();
      }
      throw new Error('Status check failed');
    }).then(function(text) {
      console.log('[HTTPD] RPC proxy status:', text);
    }).catch(function(e) {
      console.warn('[HTTPD] RPC proxy not available:', e.message);
    });
  }
  
  // Check status after page load
  window.addEventListener('load', function() {
    setTimeout(checkRpcStatus, 1000);
  });
  
  console.log('[HTTPD] HTTP/WebSocket RPC interceptor installed');
})();
