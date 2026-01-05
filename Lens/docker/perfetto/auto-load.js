// Auto-load trace file script for Perfetto UI v49+
// Uses Perfetto's openTrace API with ArrayBuffer

(function() {
  'use strict';

  const TRACE_URL = '/trace.perfetto';

  async function loadTrace() {
    console.log('[AutoLoad] Starting trace load...');
    
    try {
      // Fetch the trace file
      console.log('[AutoLoad] Fetching trace from:', TRACE_URL);
      const response = await fetch(TRACE_URL);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }
      
      const arrayBuffer = await response.arrayBuffer();
      console.log(`[AutoLoad] Trace fetched: ${arrayBuffer.byteLength} bytes`);
      
      // Wait for Perfetto to be ready
      await waitForPerfettoReady();
      
      // Open trace using Perfetto's API
      openTraceInPerfetto(arrayBuffer);
      
    } catch (err) {
      console.error('[AutoLoad] Failed to load trace:', err);
    }
  }

  function waitForPerfettoReady() {
    return new Promise((resolve) => {
      let attempts = 0;
      const maxAttempts = 100; // 10 seconds max
      
      const check = () => {
        attempts++;
        
        // Check various Perfetto global objects
        const ready = (
          (typeof globals !== 'undefined' && globals.dispatch) ||
          (typeof perfetto !== 'undefined' && perfetto.openTrace) ||
          (window.perfetto && window.perfetto.openTrace) ||
          (window.app && window.app.openTraceFromBuffer)
        );
        
        if (ready) {
          console.log('[AutoLoad] Perfetto ready after', attempts * 100, 'ms');
          resolve();
        } else if (attempts >= maxAttempts) {
          console.log('[AutoLoad] Timeout waiting for Perfetto, proceeding anyway');
          resolve();
        } else {
          setTimeout(check, 100);
        }
      };
      
      // Start checking after initial delay
      setTimeout(check, 500);
    });
  }

  function openTraceInPerfetto(arrayBuffer) {
    console.log('[AutoLoad] Attempting to open trace...');
    
    // Method 1: Perfetto v49+ uses globals.dispatch with openTraceFromBuffer action
    if (typeof globals !== 'undefined' && globals.dispatch) {
      console.log('[AutoLoad] Using globals.dispatch method');
      try {
        // Create a File object from the ArrayBuffer
        const file = new File([arrayBuffer], 'trace.perfetto', { type: 'application/octet-stream' });
        globals.dispatch({ type: 'OPEN_TRACE_FROM_FILE', file: file });
        return;
      } catch (e) {
        console.warn('[AutoLoad] globals.dispatch failed:', e);
      }
    }
    
    // Method 2: Try perfetto.openTrace if available
    if (typeof perfetto !== 'undefined' && perfetto.openTrace) {
      console.log('[AutoLoad] Using perfetto.openTrace method');
      try {
        perfetto.openTrace(arrayBuffer, 'trace.perfetto');
        return;
      } catch (e) {
        console.warn('[AutoLoad] perfetto.openTrace failed:', e);
      }
    }
    
    // Method 3: Try window.app.openTraceFromBuffer
    if (window.app && window.app.openTraceFromBuffer) {
      console.log('[AutoLoad] Using app.openTraceFromBuffer method');
      try {
        window.app.openTraceFromBuffer({
          buffer: arrayBuffer,
          title: 'Trace',
          fileName: 'trace.perfetto'
        });
        return;
      } catch (e) {
        console.warn('[AutoLoad] app.openTraceFromBuffer failed:', e);
      }
    }
    
    // Method 4: Simulate file drop as last resort
    console.log('[AutoLoad] Trying drag-drop simulation...');
    try {
      const file = new File([arrayBuffer], 'trace.perfetto', { type: 'application/octet-stream' });
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(file);
      
      const dropEvent = new DragEvent('drop', {
        bubbles: true,
        cancelable: true,
        dataTransfer: dataTransfer
      });
      
      // Find drop target
      const dropTarget = document.querySelector('.drop-zone') || 
                         document.querySelector('[class*="drop"]') ||
                         document.body;
      dropTarget.dispatchEvent(dropEvent);
    } catch (e) {
      console.error('[AutoLoad] All methods failed:', e);
    }
  }

  // Initialize
  function init() {
    // Check if trace file exists before attempting to load
    fetch(TRACE_URL, { method: 'HEAD' })
      .then(response => {
        if (response.ok) {
          console.log('[AutoLoad] Trace file found, initiating load...');
          // Wait for page to be ready
          if (document.readyState === 'complete') {
            setTimeout(loadTrace, 1000); // Give Perfetto time to initialize
          } else {
            window.addEventListener('load', () => setTimeout(loadTrace, 1000));
          }
        } else {
          console.log('[AutoLoad] No trace file found (status:', response.status, ')');
        }
      })
      .catch(err => {
        console.log('[AutoLoad] Could not check trace file:', err);
      });
  }

  init();
})();
