// Auto-load trace file script for Perfetto UI
// This script is injected into the page to automatically load the trace file

(function() {
  'use strict';

  const TRACE_URL = '/trace.perfetto';
  const MAX_RETRIES = 3;
  const RETRY_DELAY = 1000;

  async function fetchWithRetry(url, retries = MAX_RETRIES) {
    for (let i = 0; i < retries; i++) {
      try {
        const response = await fetch(url);
        if (response.ok) {
          return response;
        }
        console.warn(`Fetch attempt ${i + 1} failed: ${response.status}`);
      } catch (err) {
        console.warn(`Fetch attempt ${i + 1} error:`, err);
      }
      if (i < retries - 1) {
        await new Promise(r => setTimeout(r, RETRY_DELAY));
      }
    }
    throw new Error(`Failed to fetch ${url} after ${retries} attempts`);
  }

  async function loadTrace() {
    console.log('[AutoLoad] Fetching trace file...');
    
    try {
      const response = await fetchWithRetry(TRACE_URL);
      const blob = await response.blob();
      const arrayBuffer = await blob.arrayBuffer();
      
      console.log(`[AutoLoad] Trace loaded: ${arrayBuffer.byteLength} bytes`);
      
      // Wait for Perfetto app to be ready
      await waitForPerfetto();
      
      // Use Perfetto's internal API to open the trace
      if (window.app && window.app.openTraceFromBuffer) {
        console.log('[AutoLoad] Opening trace via app.openTraceFromBuffer...');
        window.app.openTraceFromBuffer({
          buffer: arrayBuffer,
          title: 'Trace',
          fileName: 'trace.perfetto'
        });
      } else if (window.globals && window.globals.dispatch) {
        // Alternative method using globals.dispatch
        console.log('[AutoLoad] Opening trace via globals.dispatch...');
        const file = new File([arrayBuffer], 'trace.perfetto');
        window.globals.dispatch({
          type: 'OPEN_TRACE_FROM_FILE',
          file: file
        });
      } else {
        console.error('[AutoLoad] Perfetto API not available');
      }
    } catch (err) {
      console.error('[AutoLoad] Failed to load trace:', err);
    }
  }

  function waitForPerfetto() {
    return new Promise((resolve) => {
      const check = () => {
        // Check if Perfetto app is initialized
        if ((window.app && window.app.openTraceFromBuffer) || 
            (window.globals && window.globals.dispatch)) {
          resolve();
        } else {
          setTimeout(check, 100);
        }
      };
      
      // Start checking after a short delay
      setTimeout(check, 500);
      
      // Timeout after 10 seconds
      setTimeout(resolve, 10000);
    });
  }

  // Check if we should auto-load (only if trace file exists)
  async function init() {
    try {
      const response = await fetch(TRACE_URL, { method: 'HEAD' });
      if (response.ok) {
        console.log('[AutoLoad] Trace file detected, will auto-load');
        // Wait for page to fully load
        if (document.readyState === 'complete') {
          loadTrace();
        } else {
          window.addEventListener('load', loadTrace);
        }
      } else {
        console.log('[AutoLoad] No trace file found, skipping auto-load');
      }
    } catch (err) {
      console.log('[AutoLoad] Could not check trace file:', err);
    }
  }

  init();
})();

