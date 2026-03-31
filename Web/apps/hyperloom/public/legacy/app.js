/* ══════════════════════════════════════════════════════
   AMD PRISM — App Controller
   ══════════════════════════════════════════════════════ */

(function initPRISM() {

  /* ──── Phase Tabs (old static — kept for non-analysis pages) ──── */
  document.querySelectorAll('.phase-tab:not([data-phase])').forEach(tab => {
    tab.addEventListener('click', () => {
      tab.parentElement.querySelectorAll('.phase-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
    });
  });

  /* ──── Detail Tabs → Panel Switching ──── */
  document.querySelectorAll('.detail-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      const tabsContainer = tab.parentElement;
      tabsContainer.querySelectorAll('.detail-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      // Find the panels container (next sibling element)
      const panelsContainer = tabsContainer.nextElementSibling;
      if (panelsContainer && panelsContainer.classList.contains('detail-tab-panels')) {
        panelsContainer.querySelectorAll('.detail-panel').forEach(p => p.classList.remove('active'));
        const targetPanel = panelsContainer.querySelector(`[data-panel="${tab.textContent.trim()}"]`);
        if (targetPanel) targetPanel.classList.add('active');
      }
    });
  });

  /* ──── Kernel Chips → Update Detail Content ──── */
  const KERNEL_DETAIL_DATA = {
    'fp4_moe_gemm_kernel': {
      pre: { bw: '2487 GB/s', compute: '1200 TFLOPS', bwUtil: '31.0%', computeUtil: '12.0%', bottleneck: '🔴 Mem Bound' },
      post: { bw: '5134 GB/s', compute: '2487 TFLOPS', bwUtil: '64.2%', bwDelta: '+33.2%', computeUtil: '24.7%', computeDelta: '+12.7%', speedup: '2.4×' },
      roofline: { name: 'fp4_moe_gemm_kernel', peakBW: '8000 GB/s', peakCompute: '10040 TFLOPS', preOI: '2.8', postOI: '4.8', preBW: '31.0%', postBW: '64.2%', preTFLOPS: '1200', postTFLOPS: '2487' },
      trace: { before: '714.8ms', after: '543.3ms', hotspot: 'buffer_load_dword', delta: '-171.5ms (24.0%)' }
    },
    'fp4_dequant_gemm': {
      pre: { bw: '2240 GB/s', compute: '2100 TFLOPS', bwUtil: '28.0%', computeUtil: '20.9%', bottleneck: '🔴 Mem Bound' },
      post: { bw: '4200 GB/s', compute: '4834 TFLOPS', bwUtil: '52.5%', bwDelta: '+24.5%', computeUtil: '48.2%', computeDelta: '+27.3%', speedup: '1.8×' },
      roofline: { name: 'fp4_dequant_gemm', peakBW: '8000 GB/s', peakCompute: '10040 TFLOPS', preOI: '2.2', postOI: '3.5', preBW: '28.0%', postBW: '52.5%', preTFLOPS: '2100', postTFLOPS: '4834' },
      trace: { before: '543.3ms', after: '517.3ms', hotspot: 'shared_mem_load', delta: '-26.0ms (4.8%)' }
    },
    'flash_attn_fwd_v2': {
      pre: { bw: '2800 GB/s', compute: '2600 TFLOPS', bwUtil: '35.0%', computeUtil: '25.9%', bottleneck: '🔴 Mem Bound' },
      post: { bw: '3900 GB/s', compute: '5236 TFLOPS', bwUtil: '48.8%', bwDelta: '+13.8%', computeUtil: '52.1%', computeDelta: '+26.2%', speedup: '1.35×' },
      roofline: { name: 'flash_attn_fwd_v2', peakBW: '8000 GB/s', peakCompute: '10040 TFLOPS', preOI: '3.0', postOI: '4.2', preBW: '35.0%', postBW: '48.8%', preTFLOPS: '2600', postTFLOPS: '5236' },
      trace: { before: '517.3ms', after: '498.2ms', hotspot: 'qk_matmul', delta: '-19.1ms (3.7%)' }
    }
  };

  function updateKernelDetail(kernelName) {
    const data = KERNEL_DETAIL_DATA[kernelName];
    if (!data) return;

    // Update Pre-opt metrics
    const preMetrics = document.querySelectorAll('#page-optimization .metrics-row.compact')[0];
    if (preMetrics) {
      const cards = preMetrics.querySelectorAll('.metric-card');
      if (cards[0]) cards[0].querySelector('.metric-value').textContent = data.pre.bw;
      if (cards[1]) cards[1].querySelector('.metric-value').textContent = data.pre.compute;
      if (cards[2]) cards[2].querySelector('.metric-value').textContent = data.pre.bwUtil;
      if (cards[3]) cards[3].querySelector('.metric-value').textContent = data.pre.computeUtil;
      if (cards[4]) cards[4].querySelector('.metric-value').innerHTML = data.pre.bottleneck;
    }

    // Update Post-opt metrics
    const postMetrics = document.querySelectorAll('#page-optimization .metrics-row.compact')[1];
    if (postMetrics) {
      const cards = postMetrics.querySelectorAll('.metric-card');
      if (cards[0]) cards[0].querySelector('.metric-value').textContent = data.post.bw;
      if (cards[1]) cards[1].querySelector('.metric-value').textContent = data.post.compute;
      if (cards[2]) cards[2].querySelector('.metric-value').innerHTML = `${data.post.bwUtil} <span class="delta-up">${data.post.bwDelta}</span>`;
      if (cards[3]) cards[3].querySelector('.metric-value').innerHTML = `${data.post.computeUtil} <span class="delta-up">${data.post.computeDelta}</span>`;
      if (cards[4]) cards[4].querySelector('.metric-value').textContent = data.post.speedup;
    }

    // Update Roofline SVG texts
    const rooflineSvg = document.querySelector('.roofline-svg');
    if (rooflineSvg) {
      const rf = data.roofline;
      const titleSpan = document.querySelector('.roofline-title');
      if (titleSpan) titleSpan.textContent = `Roofline Model — ${rf.name}`;
      const subSpan = document.querySelector('.roofline-sub');
      if (subSpan) subSpan.textContent = `MI355X · Peak BW: ${rf.peakBW} · Peak Compute: ${rf.peakCompute}`;

      // Update SVG text elements (pre-opt and post-opt labels)
      const textEls = rooflineSvg.querySelectorAll('text');
      textEls.forEach(t => {
        if (t.textContent.includes('Before')) {
          t.textContent = `Before (BW ${rf.preBW})`;
        } else if (t.textContent.includes('After')) {
          t.textContent = `After (BW ${rf.postBW})`;
        } else if (t.textContent.includes('TFLOPS') && t.textContent.includes('OI=') && t.getAttribute('fill') === '#6b7280') {
          if (t.previousElementSibling && t.previousElementSibling.getAttribute('fill') === '#e4002b') {
            t.textContent = `${rf.preTFLOPS} TFLOPS · OI=${rf.preOI}`;
          } else if (t.previousElementSibling && t.previousElementSibling.getAttribute('fill') === '#22c55e') {
            t.textContent = `${rf.postTFLOPS} TFLOPS · OI=${rf.postOI}`;
          }
        }
      });
    }

    // Update TraceDiff
    const traceBefore = document.querySelector('.trace-label.before');
    const traceAfter = document.querySelector('.trace-label.after');
    const traceTimes = document.querySelectorAll('.trace-time');
    if (traceBefore && traceTimes[0]) {
      traceTimes[0].textContent = data.trace.before;
    }
    if (traceAfter && traceTimes[1]) {
      traceTimes[1].textContent = data.trace.after;
    }
    const traceDiffTitle = document.querySelector('.tracediff-title');
    if (traceDiffTitle) {
      traceDiffTitle.textContent = `Trace Diff — ${kernelName}`;
    }
    const savingLegend = document.querySelector('.td-legend-item.saving');
    if (savingLegend) {
      savingLegend.textContent = `Δ ${data.trace.delta}`;
    }
    const hotspotLegend = document.querySelector('.td-legend-item:first-child');
    if (hotspotLegend) {
      const dot = hotspotLegend.querySelector('.td-dot');
      hotspotLegend.textContent = '';
      if (dot) hotspotLegend.appendChild(dot);
      hotspotLegend.append(` Hotspot region (${data.trace.hotspot})`);
    }
  }

  document.querySelectorAll('.kernel-chip').forEach(chip => {
    chip.addEventListener('click', () => {
      chip.parentElement.querySelectorAll('.kernel-chip').forEach(c => c.classList.remove('active'));
      chip.classList.add('active');
      updateKernelDetail(chip.textContent.trim());
    });
  });

  /* ──── Collapsible Sections (GEAK Status) ──── */
  document.querySelectorAll('.collapsible-header').forEach(header => {
    header.addEventListener('click', () => {
      const section = header.closest('.collapsible-section');
      section.classList.toggle('collapsed');
    });
  });

  /* ──── (Strategy cards now inline under each pipeline item — see inline-strategy-toggle-btn) ──── */

  /* ══════ BENCHMARK DATA ══════ */
  // Realistic benchmark data based on InferenceX configs
  // AMD MI355X data as baseline, B200 as reference, plus optimized curves
  const BENCH_DATA = {
    'gpt-oss 120B': {
      arch: { type: 'MoE', attention: 'Sink/Full GQA', size: '120B' },
      '1K / 1K': {
        FP4: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 60, y: 9800 }, { x: 90, y: 7200 }, { x: 120, y: 5100 },
                { x: 150, y: 3500 }, { x: 180, y: 2400 }, { x: 220, y: 1500 },
                { x: 260, y: 900 }, { x: 300, y: 450 }
              ],
              latency: [
                { x: 3.2, y: 450 }, { x: 4.0, y: 900 }, { x: 5.0, y: 1500 },
                { x: 6.5, y: 2400 }, { x: 8.5, y: 3500 }, { x: 11, y: 5100 },
                { x: 14, y: 7200 }, { x: 18, y: 9800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 55, y: 8500 }, { x: 85, y: 6200 }, { x: 115, y: 4400 },
                { x: 145, y: 3000 }, { x: 175, y: 2100 }, { x: 210, y: 1300 },
                { x: 250, y: 750 }, { x: 290, y: 380 }
              ],
              latency: [
                { x: 3.5, y: 380 }, { x: 4.5, y: 750 }, { x: 5.5, y: 1300 },
                { x: 7.0, y: 2100 }, { x: 9.0, y: 3000 }, { x: 12, y: 4400 },
                { x: 15, y: 6200 }, { x: 19, y: 8500 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 65, y: 12800 }, { x: 95, y: 9500 }, { x: 125, y: 6800 },
                { x: 155, y: 4700 }, { x: 185, y: 3200 }, { x: 225, y: 2000 },
                { x: 265, y: 1200 }, { x: 305, y: 600 }
              ],
              latency: [
                { x: 2.8, y: 600 }, { x: 3.5, y: 1200 }, { x: 4.5, y: 2000 },
                { x: 5.8, y: 3200 }, { x: 7.5, y: 4700 }, { x: 10, y: 6800 },
                { x: 12.5, y: 9500 }, { x: 16, y: 12800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 60, y: 11200 }, { x: 90, y: 8300 }, { x: 120, y: 5900 },
                { x: 150, y: 4100 }, { x: 180, y: 2800 }, { x: 215, y: 1750 },
                { x: 255, y: 1050 }, { x: 295, y: 520 }
              ],
              latency: [
                { x: 3.0, y: 520 }, { x: 3.8, y: 1050 }, { x: 4.8, y: 1750 },
                { x: 6.2, y: 2800 }, { x: 8.0, y: 4100 }, { x: 10.5, y: 5900 },
                { x: 13.5, y: 8300 }, { x: 17, y: 11200 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 70, y: 14200 }, { x: 100, y: 10500 }, { x: 130, y: 7500 },
                { x: 160, y: 5200 }, { x: 190, y: 3500 }, { x: 230, y: 2200 },
                { x: 270, y: 1350 }, { x: 310, y: 680 }
              ],
              latency: [
                { x: 2.5, y: 680 }, { x: 3.2, y: 1350 }, { x: 4.0, y: 2200 },
                { x: 5.2, y: 3500 }, { x: 6.8, y: 5200 }, { x: 9.0, y: 7500 },
                { x: 11.5, y: 10500 }, { x: 15, y: 14200 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 65, y: 13000 }, { x: 95, y: 9600 }, { x: 125, y: 6900 },
                { x: 155, y: 4800 }, { x: 185, y: 3200 }, { x: 220, y: 2000 },
                { x: 260, y: 1200 }, { x: 300, y: 600 }
              ],
              latency: [
                { x: 2.7, y: 600 }, { x: 3.5, y: 1200 }, { x: 4.3, y: 2000 },
                { x: 5.5, y: 3200 }, { x: 7.2, y: 4800 }, { x: 9.5, y: 6900 },
                { x: 12, y: 9600 }, { x: 15.5, y: 13000 }
              ]
            },
            TRT_LLM: {
              interactivity: [
                { x: 75, y: 15800 }, { x: 105, y: 11600 }, { x: 135, y: 8300 },
                { x: 165, y: 5700 }, { x: 195, y: 3900 }, { x: 235, y: 2500 },
                { x: 275, y: 1500 }, { x: 315, y: 750 }
              ],
              latency: [
                { x: 2.2, y: 750 }, { x: 2.9, y: 1500 }, { x: 3.6, y: 2500 },
                { x: 4.8, y: 3900 }, { x: 6.2, y: 5700 }, { x: 8.5, y: 8300 },
                { x: 10.5, y: 11600 }, { x: 14, y: 15800 }
              ]
            }
          }
        },
        FP8: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 55, y: 10600 }, { x: 85, y: 7800 }, { x: 115, y: 5500 },
                { x: 145, y: 3800 }, { x: 175, y: 2600 }, { x: 215, y: 1650 },
                { x: 255, y: 980 }, { x: 295, y: 490 }
              ],
              latency: [
                { x: 3.0, y: 490 }, { x: 3.8, y: 980 }, { x: 4.7, y: 1650 },
                { x: 6.0, y: 2600 }, { x: 8.0, y: 3800 }, { x: 10.5, y: 5500 },
                { x: 13.5, y: 7800 }, { x: 17, y: 10600 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 50, y: 9200 }, { x: 80, y: 6700 }, { x: 110, y: 4800 },
                { x: 140, y: 3300 }, { x: 170, y: 2300 }, { x: 205, y: 1420 },
                { x: 245, y: 820 }, { x: 285, y: 410 }
              ],
              latency: [
                { x: 3.3, y: 410 }, { x: 4.2, y: 820 }, { x: 5.2, y: 1420 },
                { x: 6.6, y: 2300 }, { x: 8.5, y: 3300 }, { x: 11.5, y: 4800 },
                { x: 14.5, y: 6700 }, { x: 18, y: 9200 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 60, y: 13800 }, { x: 90, y: 10200 }, { x: 120, y: 7300 },
                { x: 150, y: 5100 }, { x: 180, y: 3500 }, { x: 220, y: 2200 },
                { x: 260, y: 1300 }, { x: 300, y: 650 }
              ],
              latency: [
                { x: 2.6, y: 650 }, { x: 3.3, y: 1300 }, { x: 4.2, y: 2200 },
                { x: 5.4, y: 3500 }, { x: 7.0, y: 5100 }, { x: 9.5, y: 7300 },
                { x: 12, y: 10200 }, { x: 15, y: 13800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 55, y: 12100 }, { x: 85, y: 9000 }, { x: 115, y: 6400 },
                { x: 145, y: 4500 }, { x: 175, y: 3050 }, { x: 210, y: 1900 },
                { x: 250, y: 1140 }, { x: 290, y: 570 }
              ],
              latency: [
                { x: 2.8, y: 570 }, { x: 3.5, y: 1140 }, { x: 4.5, y: 1900 },
                { x: 5.8, y: 3050 }, { x: 7.5, y: 4500 }, { x: 10, y: 6400 },
                { x: 13, y: 9000 }, { x: 16, y: 12100 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 70, y: 14200 }, { x: 100, y: 10500 }, { x: 130, y: 7500 },
                { x: 160, y: 5200 }, { x: 190, y: 3500 }, { x: 230, y: 2200 },
                { x: 270, y: 1350 }, { x: 310, y: 680 }
              ],
              latency: [
                { x: 2.5, y: 680 }, { x: 3.2, y: 1350 }, { x: 4.0, y: 2200 },
                { x: 5.2, y: 3500 }, { x: 6.8, y: 5200 }, { x: 9.0, y: 7500 },
                { x: 11.5, y: 10500 }, { x: 15, y: 14200 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 65, y: 13000 }, { x: 95, y: 9600 }, { x: 125, y: 6900 },
                { x: 155, y: 4800 }, { x: 185, y: 3200 }, { x: 220, y: 2000 },
                { x: 260, y: 1200 }, { x: 300, y: 600 }
              ],
              latency: [
                { x: 2.7, y: 600 }, { x: 3.5, y: 1200 }, { x: 4.3, y: 2000 },
                { x: 5.5, y: 3200 }, { x: 7.2, y: 4800 }, { x: 9.5, y: 6900 },
                { x: 12, y: 9600 }, { x: 15.5, y: 13000 }
              ]
            },
            TRT_LLM: {
              interactivity: [
                { x: 75, y: 15800 }, { x: 105, y: 11600 }, { x: 135, y: 8300 },
                { x: 165, y: 5700 }, { x: 195, y: 3900 }, { x: 235, y: 2500 },
                { x: 275, y: 1500 }, { x: 315, y: 750 }
              ],
              latency: [
                { x: 2.2, y: 750 }, { x: 2.9, y: 1500 }, { x: 3.6, y: 2500 },
                { x: 4.8, y: 3900 }, { x: 6.2, y: 5700 }, { x: 8.5, y: 8300 },
                { x: 10.5, y: 11600 }, { x: 14, y: 15800 }
              ]
            }
          }
        }
      },
      '8K / 1K': {
        FP4: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 40, y: 6200 }, { x: 60, y: 4500 }, { x: 80, y: 3200 },
                { x: 100, y: 2200 }, { x: 130, y: 1500 }, { x: 160, y: 900 },
                { x: 190, y: 500 }, { x: 220, y: 250 }
              ],
              latency: [
                { x: 5.0, y: 250 }, { x: 6.5, y: 500 }, { x: 8.5, y: 900 },
                { x: 11, y: 1500 }, { x: 14, y: 2200 }, { x: 18, y: 3200 },
                { x: 22, y: 4500 }, { x: 28, y: 6200 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 35, y: 5500 }, { x: 55, y: 4000 }, { x: 75, y: 2800 },
                { x: 95, y: 1950 }, { x: 125, y: 1300 }, { x: 155, y: 780 },
                { x: 185, y: 430 }, { x: 215, y: 220 }
              ],
              latency: [
                { x: 5.5, y: 220 }, { x: 7.0, y: 430 }, { x: 9.0, y: 780 },
                { x: 12, y: 1300 }, { x: 15, y: 1950 }, { x: 19, y: 2800 },
                { x: 23, y: 4000 }, { x: 30, y: 5500 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 45, y: 8100 }, { x: 65, y: 5900 }, { x: 85, y: 4200 },
                { x: 105, y: 2900 }, { x: 135, y: 2000 }, { x: 165, y: 1200 },
                { x: 195, y: 680 }, { x: 225, y: 340 }
              ],
              latency: [
                { x: 4.3, y: 340 }, { x: 5.5, y: 680 }, { x: 7.2, y: 1200 },
                { x: 9.5, y: 2000 }, { x: 12, y: 2900 }, { x: 16, y: 4200 },
                { x: 20, y: 5900 }, { x: 25, y: 8100 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 40, y: 7200 }, { x: 60, y: 5200 }, { x: 80, y: 3700 },
                { x: 100, y: 2600 }, { x: 130, y: 1750 }, { x: 160, y: 1050 },
                { x: 190, y: 580 }, { x: 220, y: 290 }
              ],
              latency: [
                { x: 4.8, y: 290 }, { x: 6.0, y: 580 }, { x: 8.0, y: 1050 },
                { x: 10.5, y: 1750 }, { x: 13, y: 2600 }, { x: 17, y: 3700 },
                { x: 21, y: 5200 }, { x: 27, y: 7200 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 50, y: 9000 }, { x: 70, y: 6600 }, { x: 90, y: 4700 },
                { x: 110, y: 3300 }, { x: 140, y: 2200 }, { x: 170, y: 1350 },
                { x: 200, y: 760 }, { x: 230, y: 380 }
              ],
              latency: [
                { x: 3.8, y: 380 }, { x: 5.0, y: 760 }, { x: 6.5, y: 1350 },
                { x: 8.5, y: 2200 }, { x: 11, y: 3300 }, { x: 14, y: 4700 },
                { x: 18, y: 6600 }, { x: 23, y: 9000 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 45, y: 8200 }, { x: 65, y: 6000 }, { x: 85, y: 4300 },
                { x: 105, y: 3000 }, { x: 135, y: 2000 }, { x: 165, y: 1200 },
                { x: 195, y: 680 }, { x: 225, y: 340 }
              ],
              latency: [
                { x: 4.2, y: 340 }, { x: 5.5, y: 680 }, { x: 7.2, y: 1200 },
                { x: 9.0, y: 2000 }, { x: 11.5, y: 3000 }, { x: 15, y: 4300 },
                { x: 19, y: 6000 }, { x: 24, y: 8200 }
              ]
            }
          }
        },
        FP8: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 38, y: 6700 }, { x: 58, y: 4900 }, { x: 78, y: 3500 },
                { x: 98, y: 2400 }, { x: 128, y: 1650 }, { x: 158, y: 990 },
                { x: 188, y: 550 }, { x: 218, y: 275 }
              ],
              latency: [
                { x: 4.7, y: 275 }, { x: 6.0, y: 550 }, { x: 8.0, y: 990 },
                { x: 10.5, y: 1650 }, { x: 13, y: 2400 }, { x: 17, y: 3500 },
                { x: 21, y: 4900 }, { x: 27, y: 6700 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 33, y: 5900 }, { x: 53, y: 4300 }, { x: 73, y: 3050 },
                { x: 93, y: 2100 }, { x: 123, y: 1420 }, { x: 153, y: 860 },
                { x: 183, y: 470 }, { x: 213, y: 240 }
              ],
              latency: [
                { x: 5.2, y: 240 }, { x: 6.5, y: 470 }, { x: 8.5, y: 860 },
                { x: 11, y: 1420 }, { x: 14, y: 2100 }, { x: 18, y: 3050 },
                { x: 22, y: 4300 }, { x: 28, y: 5900 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 42, y: 8800 }, { x: 62, y: 6400 }, { x: 82, y: 4600 },
                { x: 102, y: 3200 }, { x: 132, y: 2200 }, { x: 162, y: 1320 },
                { x: 192, y: 740 }, { x: 222, y: 370 }
              ],
              latency: [
                { x: 4.0, y: 370 }, { x: 5.2, y: 740 }, { x: 6.8, y: 1320 },
                { x: 9.0, y: 2200 }, { x: 11.5, y: 3200 }, { x: 15, y: 4600 },
                { x: 19, y: 6400 }, { x: 24, y: 8800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 38, y: 7800 }, { x: 58, y: 5700 }, { x: 78, y: 4000 },
                { x: 98, y: 2800 }, { x: 128, y: 1900 }, { x: 158, y: 1150 },
                { x: 188, y: 640 }, { x: 218, y: 320 }
              ],
              latency: [
                { x: 4.5, y: 320 }, { x: 5.6, y: 640 }, { x: 7.5, y: 1150 },
                { x: 10, y: 1900 }, { x: 12.5, y: 2800 }, { x: 16, y: 4000 },
                { x: 20, y: 5700 }, { x: 25, y: 7800 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 50, y: 9000 }, { x: 70, y: 6600 }, { x: 90, y: 4700 },
                { x: 110, y: 3300 }, { x: 140, y: 2200 }, { x: 170, y: 1350 },
                { x: 200, y: 760 }, { x: 230, y: 380 }
              ],
              latency: [
                { x: 3.8, y: 380 }, { x: 5.0, y: 760 }, { x: 6.5, y: 1350 },
                { x: 8.5, y: 2200 }, { x: 11, y: 3300 }, { x: 14, y: 4700 },
                { x: 18, y: 6600 }, { x: 23, y: 9000 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 45, y: 8200 }, { x: 65, y: 6000 }, { x: 85, y: 4300 },
                { x: 105, y: 3000 }, { x: 135, y: 2000 }, { x: 165, y: 1200 },
                { x: 195, y: 680 }, { x: 225, y: 340 }
              ],
              latency: [
                { x: 4.2, y: 340 }, { x: 5.5, y: 680 }, { x: 7.2, y: 1200 },
                { x: 9.0, y: 2000 }, { x: 11.5, y: 3000 }, { x: 15, y: 4300 },
                { x: 19, y: 6000 }, { x: 24, y: 8200 }
              ]
            }
          }
        }
      },
      '1K / 8K': {
        FP4: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 15, y: 5500 }, { x: 22, y: 4000 }, { x: 30, y: 2800 },
                { x: 38, y: 1900 }, { x: 48, y: 1200 }, { x: 60, y: 750 },
                { x: 75, y: 400 }, { x: 90, y: 200 }
              ],
              latency: [
                { x: 12, y: 200 }, { x: 16, y: 400 }, { x: 20, y: 750 },
                { x: 26, y: 1200 }, { x: 33, y: 1900 }, { x: 42, y: 2800 },
                { x: 55, y: 4000 }, { x: 70, y: 5500 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 13, y: 4800 }, { x: 20, y: 3500 }, { x: 28, y: 2500 },
                { x: 35, y: 1700 }, { x: 45, y: 1050 }, { x: 55, y: 650 },
                { x: 70, y: 350 }, { x: 85, y: 175 }
              ],
              latency: [
                { x: 13, y: 175 }, { x: 17, y: 350 }, { x: 22, y: 650 },
                { x: 28, y: 1050 }, { x: 35, y: 1700 }, { x: 45, y: 2500 },
                { x: 58, y: 3500 }, { x: 75, y: 4800 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 17, y: 7200 }, { x: 25, y: 5200 }, { x: 33, y: 3700 },
                { x: 42, y: 2500 }, { x: 52, y: 1600 }, { x: 65, y: 1000 },
                { x: 80, y: 550 }, { x: 95, y: 280 }
              ],
              latency: [
                { x: 10, y: 280 }, { x: 14, y: 550 }, { x: 17, y: 1000 },
                { x: 22, y: 1600 }, { x: 29, y: 2500 }, { x: 37, y: 3700 },
                { x: 48, y: 5200 }, { x: 62, y: 7200 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 15, y: 6300 }, { x: 23, y: 4600 }, { x: 30, y: 3200 },
                { x: 38, y: 2200 }, { x: 48, y: 1400 }, { x: 60, y: 880 },
                { x: 75, y: 480 }, { x: 90, y: 240 }
              ],
              latency: [
                { x: 11, y: 240 }, { x: 15, y: 480 }, { x: 19, y: 880 },
                { x: 24, y: 1400 }, { x: 31, y: 2200 }, { x: 40, y: 3200 },
                { x: 52, y: 4600 }, { x: 67, y: 6300 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 18, y: 7800 }, { x: 26, y: 5700 }, { x: 35, y: 4000 },
                { x: 44, y: 2800 }, { x: 55, y: 1800 }, { x: 68, y: 1100 },
                { x: 82, y: 600 }, { x: 98, y: 300 }
              ],
              latency: [
                { x: 9.5, y: 300 }, { x: 13, y: 600 }, { x: 16, y: 1100 },
                { x: 21, y: 1800 }, { x: 27, y: 2800 }, { x: 35, y: 4000 },
                { x: 45, y: 5700 }, { x: 58, y: 7800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 16, y: 7100 }, { x: 24, y: 5200 }, { x: 32, y: 3600 },
                { x: 40, y: 2500 }, { x: 50, y: 1600 }, { x: 63, y: 1000 },
                { x: 78, y: 540 }, { x: 94, y: 270 }
              ],
              latency: [
                { x: 10, y: 270 }, { x: 14, y: 540 }, { x: 17.5, y: 1000 },
                { x: 22.5, y: 1600 }, { x: 29, y: 2500 }, { x: 38, y: 3600 },
                { x: 48, y: 5200 }, { x: 62, y: 7100 }
              ]
            }
          }
        },
        FP8: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 14, y: 5900 }, { x: 21, y: 4300 }, { x: 29, y: 3050 },
                { x: 37, y: 2080 }, { x: 46, y: 1320 }, { x: 58, y: 820 },
                { x: 73, y: 440 }, { x: 88, y: 220 }
              ],
              latency: [
                { x: 11.5, y: 220 }, { x: 15, y: 440 }, { x: 19, y: 820 },
                { x: 25, y: 1320 }, { x: 32, y: 2080 }, { x: 40, y: 3050 },
                { x: 53, y: 4300 }, { x: 68, y: 5900 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 12, y: 5200 }, { x: 19, y: 3800 }, { x: 27, y: 2700 },
                { x: 34, y: 1850 }, { x: 43, y: 1150 }, { x: 54, y: 710 },
                { x: 68, y: 380 }, { x: 83, y: 190 }
              ],
              latency: [
                { x: 12.5, y: 190 }, { x: 16, y: 380 }, { x: 21, y: 710 },
                { x: 27, y: 1150 }, { x: 34, y: 1850 }, { x: 43, y: 2700 },
                { x: 56, y: 3800 }, { x: 72, y: 5200 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 16, y: 7800 }, { x: 24, y: 5700 }, { x: 32, y: 4000 },
                { x: 40, y: 2750 }, { x: 50, y: 1750 }, { x: 63, y: 1100 },
                { x: 78, y: 600 }, { x: 93, y: 300 }
              ],
              latency: [
                { x: 9.5, y: 300 }, { x: 13, y: 600 }, { x: 16.5, y: 1100 },
                { x: 21.5, y: 1750 }, { x: 28, y: 2750 }, { x: 36, y: 4000 },
                { x: 46, y: 5700 }, { x: 60, y: 7800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 14, y: 6800 }, { x: 22, y: 5000 }, { x: 29, y: 3500 },
                { x: 37, y: 2400 }, { x: 46, y: 1520 }, { x: 58, y: 960 },
                { x: 73, y: 520 }, { x: 88, y: 260 }
              ],
              latency: [
                { x: 10.5, y: 260 }, { x: 14, y: 520 }, { x: 18, y: 960 },
                { x: 23, y: 1520 }, { x: 30, y: 2400 }, { x: 39, y: 3500 },
                { x: 50, y: 5000 }, { x: 65, y: 6800 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 18, y: 7800 }, { x: 26, y: 5700 }, { x: 35, y: 4000 },
                { x: 44, y: 2800 }, { x: 55, y: 1800 }, { x: 68, y: 1100 },
                { x: 82, y: 600 }, { x: 98, y: 300 }
              ],
              latency: [
                { x: 9.5, y: 300 }, { x: 13, y: 600 }, { x: 16, y: 1100 },
                { x: 21, y: 1800 }, { x: 27, y: 2800 }, { x: 35, y: 4000 },
                { x: 45, y: 5700 }, { x: 58, y: 7800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 16, y: 7100 }, { x: 24, y: 5200 }, { x: 32, y: 3600 },
                { x: 40, y: 2500 }, { x: 50, y: 1600 }, { x: 63, y: 1000 },
                { x: 78, y: 540 }, { x: 94, y: 270 }
              ],
              latency: [
                { x: 10, y: 270 }, { x: 14, y: 540 }, { x: 17.5, y: 1000 },
                { x: 22.5, y: 1600 }, { x: 29, y: 2500 }, { x: 38, y: 3600 },
                { x: 48, y: 5200 }, { x: 62, y: 7100 }
              ]
            }
          }
        }
      }
    },
    'DeepSeek R1 0528': {
      arch: { type: 'MoE', attention: 'MLA', size: '671B' },
      '1K / 1K': {
        FP4: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 35, y: 5200 }, { x: 55, y: 3800 }, { x: 75, y: 2700 },
                { x: 95, y: 1850 }, { x: 120, y: 1200 }, { x: 150, y: 750 },
                { x: 180, y: 420 }, { x: 210, y: 210 }
              ],
              latency: [
                { x: 5.5, y: 210 }, { x: 7.0, y: 420 }, { x: 9.0, y: 750 },
                { x: 12, y: 1200 }, { x: 15, y: 1850 }, { x: 19, y: 2700 },
                { x: 24, y: 3800 }, { x: 30, y: 5200 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 30, y: 4500 }, { x: 50, y: 3300 }, { x: 70, y: 2350 },
                { x: 90, y: 1600 }, { x: 115, y: 1050 }, { x: 145, y: 650 },
                { x: 175, y: 360 }, { x: 205, y: 180 }
              ],
              latency: [
                { x: 6.0, y: 180 }, { x: 7.5, y: 360 }, { x: 10, y: 650 },
                { x: 13, y: 1050 }, { x: 16, y: 1600 }, { x: 20, y: 2350 },
                { x: 25, y: 3300 }, { x: 32, y: 4500 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 40, y: 6800 }, { x: 60, y: 5000 }, { x: 80, y: 3600 },
                { x: 100, y: 2500 }, { x: 125, y: 1650 }, { x: 155, y: 1000 },
                { x: 185, y: 570 }, { x: 215, y: 285 }
              ],
              latency: [
                { x: 4.8, y: 285 }, { x: 6.0, y: 570 }, { x: 7.8, y: 1000 },
                { x: 10, y: 1650 }, { x: 13, y: 2500 }, { x: 17, y: 3600 },
                { x: 21, y: 5000 }, { x: 27, y: 6800 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 35, y: 5900 }, { x: 55, y: 4300 }, { x: 75, y: 3100 },
                { x: 95, y: 2150 }, { x: 120, y: 1400 }, { x: 150, y: 880 },
                { x: 180, y: 490 }, { x: 210, y: 245 }
              ],
              latency: [
                { x: 5.2, y: 245 }, { x: 6.5, y: 490 }, { x: 8.5, y: 880 },
                { x: 11, y: 1400 }, { x: 14, y: 2150 }, { x: 18, y: 3100 },
                { x: 22.5, y: 4300 }, { x: 29, y: 5900 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 45, y: 7600 }, { x: 65, y: 5500 }, { x: 85, y: 3900 },
                { x: 105, y: 2700 }, { x: 130, y: 1800 }, { x: 160, y: 1100 },
                { x: 190, y: 620 }, { x: 220, y: 310 }
              ],
              latency: [
                { x: 4.2, y: 310 }, { x: 5.5, y: 620 }, { x: 7.0, y: 1100 },
                { x: 9.0, y: 1800 }, { x: 12, y: 2700 }, { x: 15, y: 3900 },
                { x: 19, y: 5500 }, { x: 25, y: 7600 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 40, y: 6800 }, { x: 60, y: 5000 }, { x: 80, y: 3500 },
                { x: 100, y: 2400 }, { x: 125, y: 1600 }, { x: 155, y: 980 },
                { x: 185, y: 550 }, { x: 215, y: 275 }
              ],
              latency: [
                { x: 4.5, y: 275 }, { x: 5.8, y: 550 }, { x: 7.5, y: 980 },
                { x: 10, y: 1600 }, { x: 13, y: 2400 }, { x: 16, y: 3500 },
                { x: 20, y: 5000 }, { x: 26, y: 6800 }
              ]
            }
          }
        },
        FP8: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 30, y: 4200 }, { x: 48, y: 3100 }, { x: 65, y: 2200 },
                { x: 85, y: 1500 }, { x: 110, y: 980 }, { x: 140, y: 600 },
                { x: 170, y: 340 }, { x: 200, y: 170 }
              ],
              latency: [
                { x: 6.5, y: 170 }, { x: 8.5, y: 340 }, { x: 11, y: 600 },
                { x: 14, y: 980 }, { x: 18, y: 1500 }, { x: 22, y: 2200 },
                { x: 28, y: 3100 }, { x: 35, y: 4200 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 27, y: 3800 }, { x: 44, y: 2800 }, { x: 60, y: 2000 },
                { x: 80, y: 1350 }, { x: 105, y: 880 }, { x: 135, y: 540 },
                { x: 165, y: 300 }, { x: 195, y: 150 }
              ],
              latency: [
                { x: 7.0, y: 150 }, { x: 9.0, y: 300 }, { x: 12, y: 540 },
                { x: 15, y: 880 }, { x: 19, y: 1350 }, { x: 24, y: 2000 },
                { x: 30, y: 2800 }, { x: 37, y: 3800 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 35, y: 5500 }, { x: 53, y: 4000 }, { x: 70, y: 2900 },
                { x: 90, y: 2000 }, { x: 115, y: 1300 }, { x: 145, y: 800 },
                { x: 175, y: 450 }, { x: 205, y: 230 }
              ],
              latency: [
                { x: 5.5, y: 230 }, { x: 7.2, y: 450 }, { x: 9.5, y: 800 },
                { x: 12, y: 1300 }, { x: 15.5, y: 2000 }, { x: 19, y: 2900 },
                { x: 24, y: 4000 }, { x: 30, y: 5500 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 32, y: 4900 }, { x: 49, y: 3600 }, { x: 65, y: 2600 },
                { x: 85, y: 1750 }, { x: 110, y: 1150 }, { x: 140, y: 700 },
                { x: 170, y: 390 }, { x: 200, y: 195 }
              ],
              latency: [
                { x: 6.0, y: 195 }, { x: 7.8, y: 390 }, { x: 10, y: 700 },
                { x: 13, y: 1150 }, { x: 16.5, y: 1750 }, { x: 21, y: 2600 },
                { x: 26, y: 3600 }, { x: 33, y: 4900 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 38, y: 6000 }, { x: 58, y: 4400 }, { x: 78, y: 3100 },
                { x: 98, y: 2200 }, { x: 123, y: 1450 }, { x: 153, y: 880 },
                { x: 183, y: 500 }, { x: 213, y: 250 }
              ],
              latency: [
                { x: 5.0, y: 250 }, { x: 6.5, y: 500 }, { x: 8.5, y: 880 },
                { x: 11, y: 1450 }, { x: 14, y: 2200 }, { x: 17.5, y: 3100 },
                { x: 22, y: 4400 }, { x: 28, y: 6000 }
              ]
            },
            vLLM: {
              interactivity: [
                { x: 35, y: 5400 }, { x: 53, y: 4000 }, { x: 72, y: 2800 },
                { x: 92, y: 1950 }, { x: 118, y: 1300 }, { x: 148, y: 790 },
                { x: 178, y: 440 }, { x: 208, y: 220 }
              ],
              latency: [
                { x: 5.5, y: 220 }, { x: 7.0, y: 440 }, { x: 9.0, y: 790 },
                { x: 12, y: 1300 }, { x: 15, y: 1950 }, { x: 19, y: 2800 },
                { x: 24, y: 4000 }, { x: 30, y: 5400 }
              ]
            }
          }
        }
      },
      '8K / 1K': {
        FP4: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 22, y: 3200 }, { x: 35, y: 2300 }, { x: 48, y: 1650 },
                { x: 62, y: 1100 }, { x: 80, y: 720 }, { x: 100, y: 440 },
                { x: 125, y: 250 }, { x: 150, y: 125 }
              ],
              latency: [
                { x: 8.0, y: 125 }, { x: 10.5, y: 250 }, { x: 14, y: 440 },
                { x: 18, y: 720 }, { x: 23, y: 1100 }, { x: 28, y: 1650 },
                { x: 35, y: 2300 }, { x: 45, y: 3200 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 26, y: 4200 }, { x: 40, y: 3000 }, { x: 54, y: 2150 },
                { x: 68, y: 1450 }, { x: 85, y: 950 }, { x: 105, y: 580 },
                { x: 130, y: 330 }, { x: 155, y: 170 }
              ],
              latency: [
                { x: 6.8, y: 170 }, { x: 9.0, y: 330 }, { x: 12, y: 580 },
                { x: 15, y: 950 }, { x: 20, y: 1450 }, { x: 25, y: 2150 },
                { x: 31, y: 3000 }, { x: 40, y: 4200 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 30, y: 4600 }, { x: 45, y: 3400 }, { x: 60, y: 2400 },
                { x: 75, y: 1650 }, { x: 92, y: 1050 }, { x: 112, y: 650 },
                { x: 135, y: 370 }, { x: 160, y: 185 }
              ],
              latency: [
                { x: 6.0, y: 185 }, { x: 8.0, y: 370 }, { x: 10.5, y: 650 },
                { x: 13.5, y: 1050 }, { x: 17, y: 1650 }, { x: 22, y: 2400 },
                { x: 28, y: 3400 }, { x: 36, y: 4600 }
              ]
            }
          }
        },
        FP8: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 18, y: 2600 }, { x: 30, y: 1900 }, { x: 42, y: 1350 },
                { x: 55, y: 900 }, { x: 72, y: 580 }, { x: 90, y: 360 },
                { x: 112, y: 200 }, { x: 135, y: 100 }
              ],
              latency: [
                { x: 9.5, y: 100 }, { x: 12.5, y: 200 }, { x: 16, y: 360 },
                { x: 20, y: 580 }, { x: 26, y: 900 }, { x: 32, y: 1350 },
                { x: 40, y: 1900 }, { x: 50, y: 2600 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 22, y: 3400 }, { x: 35, y: 2500 }, { x: 47, y: 1800 },
                { x: 60, y: 1200 }, { x: 78, y: 780 }, { x: 96, y: 480 },
                { x: 118, y: 270 }, { x: 140, y: 135 }
              ],
              latency: [
                { x: 8.0, y: 135 }, { x: 10.5, y: 270 }, { x: 14, y: 480 },
                { x: 17, y: 780 }, { x: 22, y: 1200 }, { x: 28, y: 1800 },
                { x: 35, y: 2500 }, { x: 45, y: 3400 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 25, y: 3800 }, { x: 38, y: 2800 }, { x: 52, y: 2000 },
                { x: 66, y: 1350 }, { x: 83, y: 880 }, { x: 102, y: 540 },
                { x: 125, y: 300 }, { x: 148, y: 150 }
              ],
              latency: [
                { x: 7.5, y: 150 }, { x: 10, y: 300 }, { x: 13, y: 540 },
                { x: 16.5, y: 880 }, { x: 21, y: 1350 }, { x: 27, y: 2000 },
                { x: 33, y: 2800 }, { x: 42, y: 3800 }
              ]
            }
          }
        }
      },
      '1K / 8K': {
        FP4: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 10, y: 2800 }, { x: 15, y: 2050 }, { x: 20, y: 1450 },
                { x: 26, y: 1000 }, { x: 34, y: 640 }, { x: 42, y: 390 },
                { x: 52, y: 220 }, { x: 62, y: 110 }
              ],
              latency: [
                { x: 18, y: 110 }, { x: 22, y: 220 }, { x: 28, y: 390 },
                { x: 35, y: 640 }, { x: 44, y: 1000 }, { x: 55, y: 1450 },
                { x: 68, y: 2050 }, { x: 85, y: 2800 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 12, y: 3700 }, { x: 18, y: 2700 }, { x: 24, y: 1900 },
                { x: 30, y: 1300 }, { x: 38, y: 850 }, { x: 47, y: 520 },
                { x: 58, y: 290 }, { x: 68, y: 145 }
              ],
              latency: [
                { x: 15, y: 145 }, { x: 19, y: 290 }, { x: 24, y: 520 },
                { x: 30, y: 850 }, { x: 38, y: 1300 }, { x: 48, y: 1900 },
                { x: 60, y: 2700 }, { x: 75, y: 3700 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 14, y: 4100 }, { x: 20, y: 3000 }, { x: 27, y: 2100 },
                { x: 34, y: 1450 }, { x: 42, y: 950 }, { x: 52, y: 580 },
                { x: 64, y: 320 }, { x: 75, y: 160 }
              ],
              latency: [
                { x: 14, y: 160 }, { x: 18, y: 320 }, { x: 22, y: 580 },
                { x: 28, y: 950 }, { x: 35, y: 1450 }, { x: 44, y: 2100 },
                { x: 56, y: 3000 }, { x: 70, y: 4100 }
              ]
            }
          }
        },
        FP8: {
          'MI355X': {
            SGLang: {
              interactivity: [
                { x: 40, y: 250 }, { x: 55, y: 175 }, { x: 68, y: 120 },
                { x: 80, y: 85 }, { x: 92, y: 62 }, { x: 105, y: 48 },
                { x: 118, y: 38 }, { x: 130, y: 30 }
              ],
              latency: [
                { x: 70, y: 30 }, { x: 82, y: 38 }, { x: 95, y: 48 },
                { x: 108, y: 62 }, { x: 120, y: 85 }, { x: 135, y: 120 },
                { x: 150, y: 175 }, { x: 168, y: 250 }
              ]
            }
          },
          'MI355X_OPT': {
            SGLang: {
              interactivity: [
                { x: 44, y: 325 }, { x: 60, y: 228 }, { x: 74, y: 156 },
                { x: 87, y: 110 }, { x: 100, y: 80 }, { x: 112, y: 62 },
                { x: 124, y: 49 }, { x: 136, y: 39 }
              ],
              latency: [
                { x: 60, y: 39 }, { x: 72, y: 49 }, { x: 84, y: 62 },
                { x: 97, y: 80 }, { x: 110, y: 110 }, { x: 125, y: 156 },
                { x: 140, y: 228 }, { x: 158, y: 325 }
              ]
            }
          },
          'B200': {
            SGLang: {
              interactivity: [
                { x: 40, y: 370 }, { x: 55, y: 260 }, { x: 68, y: 180 },
                { x: 80, y: 128 }, { x: 95, y: 88 }, { x: 108, y: 65 },
                { x: 120, y: 48 }, { x: 135, y: 35 }
              ],
              latency: [
                { x: 60, y: 35 }, { x: 70, y: 48 }, { x: 82, y: 65 },
                { x: 95, y: 88 }, { x: 108, y: 128 }, { x: 122, y: 180 },
                { x: 140, y: 260 }, { x: 162, y: 370 }
              ]
            }
          }
        }
      }
    }
  };

  /* ══════ Generate MI355X_ROOFLINE data from B200 reference ══════ */
  // MI355X Roofline = theoretical peak based on hardware roofline model.
  // Slightly better than B200 (~15-20% higher throughput, ~10% better latency/interactivity).
  (function generateMI355XRoofline() {
    const THROUGHPUT_SCALE = 1.18;   // 18% higher throughput (y)
    const INTERACTIVITY_SCALE = 1.05; // 5% better interactivity (higher x)
    const LATENCY_SCALE = 0.88;       // 12% lower latency (lower x = better)

    function scaleData(points, xScale, yScale) {
      return points.map(p => ({
        x: Math.round(p.x * xScale * 10) / 10,
        y: Math.round(p.y * yScale)
      }));
    }

    Object.keys(BENCH_DATA).forEach(model => {
      const modelData = BENCH_DATA[model];
      Object.keys(modelData).forEach(islOsl => {
        if (islOsl === 'arch') return;
        const islData = modelData[islOsl];
        Object.keys(islData).forEach(precision => {
          const precData = islData[precision];
          const b200 = precData['B200'];
          if (!b200) return;

          const rooflineEntry = {};
          Object.keys(b200).forEach(fw => {
            const fwData = b200[fw];
            rooflineEntry[fw] = {};
            if (fwData.interactivity) {
              rooflineEntry[fw].interactivity = scaleData(fwData.interactivity, INTERACTIVITY_SCALE, THROUGHPUT_SCALE);
            }
            if (fwData.latency) {
              rooflineEntry[fw].latency = scaleData(fwData.latency, LATENCY_SCALE, THROUGHPUT_SCALE);
            }
          });

          precData['MI355X_ROOFLINE'] = rooflineEntry;
        });
      });
    });
  })();

  /* ══════ DROPDOWN MENUS ══════ */
  document.querySelectorAll('.dropdown-trigger').forEach(trigger => {
    trigger.addEventListener('click', e => {
      e.stopPropagation();
      const wrap = trigger.closest('.dropdown-wrap');
      // close all other dropdowns
      document.querySelectorAll('.dropdown-wrap.open').forEach(w => {
        if (w !== wrap) w.classList.remove('open');
      });
      wrap.classList.toggle('open');
    });
  });

  // Close dropdowns on outside click
  document.addEventListener('click', () => {
    document.querySelectorAll('.dropdown-wrap.open').forEach(w => w.classList.remove('open'));
  });

  // Dropdown item selection
  document.querySelectorAll('.dropdown-wrap').forEach(wrap => {
    wrap.querySelectorAll('.dd-item').forEach(item => {
      item.addEventListener('click', e => {
        e.stopPropagation();
        const val = item.dataset.val;
        const ddValue = wrap.querySelector('.dd-value');
        // Update active state
        wrap.querySelectorAll('.dd-item').forEach(i => {
          i.classList.remove('active');
          const chk = i.querySelector('.dd-check');
          if (chk) chk.remove();
        });
        item.classList.add('active');
        // Update display value
        if (ddValue) {
          ddValue.textContent = val || item.textContent.trim();
          // Style NV reference "None" differently
          if (wrap.id === 'rpt-dd-nv') {
            ddValue.classList.toggle('nv-none', !val);
          }
        }
        // Close dropdown
        wrap.classList.remove('open');

        // Handle Y-Axis metric change
        if (wrap.id === 'dd-yaxis') {
          handleYAxisChange(val);
        }

        // Handle model change → update architecture diagram + precision + kernels
        if (wrap.id === 'dd-model') {
          updateArchDiagram(val);
          updatePrecisionDropdown(val);
        }

        // Handle precision change → update chip style + kernel charts
        if (wrap.id === 'dd-precision') {
          const chipEl = wrap.querySelector('.precision-chip');
          if (chipEl) chipEl.textContent = val;
        }

        // Update chart subtitle and re-render charts
        updateChartSubtitles();
        updateCharts();
        updateKernelCharts();
      });
    });
  });

  /* ══════ GPU CONFIG CHIP HANDLING ══════ */
  const gpuChipAmd = document.getElementById('gpu-chip-amd');
  const gpuChipNvidia = document.getElementById('gpu-chip-nvidia');

  // Handle GPU config dropdown item clicks (combined HW+FW format)
  function setupGpuConfigDropdown(ddId, chipEl) {
    document.querySelectorAll(`#${ddId} .dd-item`).forEach(item => {
      item.addEventListener('click', e => {
        e.stopPropagation();
        if (!chipEl) return;
        chipEl.dataset.hw = item.dataset.hw;
        chipEl.dataset.fw = item.dataset.fw;
        chipEl.querySelector('.gpu-chip-text').textContent = item.dataset.val;
        // Mark active
        document.querySelectorAll(`#${ddId} .dd-item`).forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        document.getElementById(ddId).classList.remove('open');
        updateChartSubtitles();
        updateCharts();
        updateKernelCharts();
      });
    });
  }

  setupGpuConfigDropdown('dd-amd-gpu', gpuChipAmd);
  setupGpuConfigDropdown('dd-nvidia-gpu', gpuChipNvidia);

  /* ══════ GET CURRENT SELECTIONS ══════ */
  function getSelections() {
    const amdChip = document.getElementById('gpu-chip-amd');
    const nvChip = document.getElementById('gpu-chip-nvidia');
    return {
      model: document.querySelector('#dd-model .dd-value')?.textContent || 'gpt-oss 120B',
      isl_osl: document.querySelector('#dd-isl-osl .dd-value')?.textContent || '1K / 1K',
      precision: document.querySelector('#dd-precision .dd-value')?.textContent || 'FP4',
      yaxis: document.querySelector('#dd-yaxis .dd-value')?.textContent || 'Token Throughput per GPU',
      hardware: amdChip?.dataset.hw || 'MI355X',
      framework: amdChip?.dataset.fw || 'SGLang',
      refHardware: nvChip?.dataset.hw || 'B200',
      refFramework: nvChip?.dataset.fw || 'SGLang'
    };
  }

  /* ══════ DYNAMIC PRECISION DROPDOWN ══════ */
  // Available precisions per model
  const MODEL_PRECISIONS = {
    'gpt-oss 120B': ['FP4', 'FP8'],
    'DeepSeek R1 0528': ['FP4', 'FP8']
  };

  function updatePrecisionDropdown(model) {
    const precisions = MODEL_PRECISIONS[model] || ['FP4'];
    const ddPrecision = document.querySelector('#dd-precision');
    if (!ddPrecision) return;

    const menu = ddPrecision.querySelector('.dropdown-menu');
    if (!menu) return;

    // Rebuild dropdown items
    menu.innerHTML = '';
    precisions.forEach((p, i) => {
      const item = document.createElement('div');
      item.className = 'dd-item' + (i === 0 ? ' active' : '');
      item.dataset.val = p;
      const icon = p === 'FP4' ? '●' : '■';
      item.textContent = icon + ' ' + p;

      item.addEventListener('click', e => {
        e.stopPropagation();
        menu.querySelectorAll('.dd-item').forEach(it => it.classList.remove('active'));
        item.classList.add('active');
        const ddValue = ddPrecision.querySelector('.dd-value');
        if (ddValue) ddValue.textContent = p;
        const chipEl = ddPrecision.querySelector('.precision-chip');
        if (chipEl) chipEl.textContent = p;
        ddPrecision.classList.remove('open');
        updateChartSubtitles();
        updateCharts();
        updateKernelCharts();
      });

      menu.appendChild(item);
    });

    // Reset to first precision
    const ddValue = ddPrecision.querySelector('.dd-value');
    if (ddValue) ddValue.textContent = precisions[0];
    const chipEl = ddPrecision.querySelector('.precision-chip');
    if (chipEl) chipEl.textContent = precisions[0];
  }

  /* ══════ KERNEL DATA PER MODEL/PRECISION ══════ */
  // KERNEL_DATA: model → precision → ISL/OSL
  const KERNEL_DATA = {
    'gpt-oss 120B': {
      FP4: {
        '1K / 1K': {
          subtitle: 'gpt-oss 120B · FP4 · ISL 1K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [34,  48,  28,  34],
            GEMM_MOE: [314, 519, 295, 385],
            AR_NORM:  [44,  72,  38,  52],
            QUANT:    [10,  26,   9,  17],
            TOPK:     [8,   14,   7,  10],
            ACT:      [6,   0,    0,   0],
            CACHE:    [6,   0,    0,   0],
            other:    [35,  83,  30,  42]
          },
          sideBySide: {
            labels: ['ATTN', 'GEMM/MOE', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [12, 158, 38, 28, 12, 8,  34, 40],
            roofline:  [ 7, 108, 17,  7,  5, 3,   8, 10],
            after:     [10, 145, 28, 18,  8, 6,  24, 28],
            reference: [20, 120, 20,  8,  6, 4,  10, 12]
          }
        },
        '8K / 1K': {
          subtitle: 'gpt-oss 120B · FP4 · ISL 8K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [120, 185, 105, 130],
            GEMM_MOE: [280, 470, 260, 350],
            AR_NORM:  [38,  62,  34,  45],
            QUANT:    [14,  32,  12,  22],
            TOPK:     [7,   12,   6,   9],
            ACT:      [5,   0,    0,   0],
            CACHE:    [18,  0,    0,   0],
            other:    [30,  72,  26,  38]
          },
          sideBySide: {
            labels: ['ATTN', 'GEMM/MOE', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [52, 142, 32, 30, 10, 7,  28, 35],
            roofline:  [32,  96, 15,  9,  4, 3,  11,  8],
            after:     [38, 128, 24, 20,  7, 5,  20, 24],
            reference: [42, 108, 18, 10,  5, 4,  14, 10]
          }
        },
        '1K / 8K': {
          subtitle: 'gpt-oss 120B · FP4 · ISL 1K / OSL 8K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [42,  65,  36,  45],
            GEMM_MOE: [340, 560, 310, 420],
            AR_NORM:  [55,  90,  48,  65],
            QUANT:    [12,  28,  10,  18],
            TOPK:     [9,   16,   8,  11],
            ACT:      [7,   0,    0,   0],
            CACHE:    [22,  0,    0,   0],
            other:    [45, 100,  38,  52]
          },
          sideBySide: {
            labels: ['ATTN', 'GEMM/MOE', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [18, 170, 44, 30, 14, 9,  38, 45],
            roofline:  [10, 115, 18,  7,  5, 4,  13, 11],
            after:     [14, 155, 32, 20, 10, 7,  28, 32],
            reference: [22, 130, 22,  9,  7, 5,  16, 14]
          }
        }
      },
      FP8: {
        '1K / 1K': {
          subtitle: 'gpt-oss 120B · FP8 · ISL 1K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [30,  44,  26,  32],
            GEMM_MOE: [290, 480, 270, 355],
            AR_NORM:  [40,  66,  35,  48],
            QUANT:    [8,   20,   7,  13],
            TOPK:     [7,   12,   6,   9],
            ACT:      [5,   0,    0,   0],
            CACHE:    [5,   0,    0,   0],
            other:    [32,  76,  27,  38]
          },
          sideBySide: {
            labels: ['ATTN', 'GEMM/MOE', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [10, 146, 34, 22, 10, 7,  30, 36],
            roofline:  [ 6, 100, 15,  6,  4, 3,   7,  9],
            after:     [ 8, 132, 26, 14,  7, 5,  20, 25],
            reference: [18, 110, 18,  7,  5, 4,   9, 11]
          }
        },
        '8K / 1K': {
          subtitle: 'gpt-oss 120B · FP8 · ISL 8K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [110, 170,  96, 120],
            GEMM_MOE: [260, 435, 240, 325],
            AR_NORM:  [35,  57,  30,  42],
            QUANT:    [12,  28,  10,  19],
            TOPK:     [6,   11,   5,   8],
            ACT:      [4,   0,    0,   0],
            CACHE:    [16,  0,    0,   0],
            other:    [28,  66,  24,  35]
          },
          sideBySide: {
            labels: ['ATTN', 'GEMM/MOE', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [48, 132, 30, 26, 9,  6,  26, 32],
            roofline:  [29,  88, 13,  8, 3,  3,  10,  7],
            after:     [35, 118, 22, 17, 6,  4,  18, 22],
            reference: [39, 100, 16,  9, 4,  3,  12,  9]
          }
        },
        '1K / 8K': {
          subtitle: 'gpt-oss 120B · FP8 · ISL 1K / OSL 8K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [38,  60,  33,  42],
            GEMM_MOE: [315, 520, 285, 390],
            AR_NORM:  [50,  82,  44,  60],
            QUANT:    [10,  24,   8,  15],
            TOPK:     [8,   14,   7,  10],
            ACT:      [6,   0,    0,   0],
            CACHE:    [20,  0,    0,   0],
            other:    [40,  92,  34,  48]
          },
          sideBySide: {
            labels: ['ATTN', 'GEMM/MOE', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [16, 158, 40, 26, 12, 8,  34, 42],
            roofline:  [ 9, 106, 16,  6,  4, 3,  11, 10],
            after:     [12, 142, 30, 17, 9,  6,  25, 29],
            reference: [20, 120, 20,  8, 6,  5,  14, 13]
          }
        }
      }
    },
    'DeepSeek R1 0528': {
      FP4: {
        '1K / 1K': {
          subtitle: 'DeepSeek R1 0528 · FP4 · ISL 1K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [28,  42,  24,  30],
            GEMM_MOE: [380, 620, 350, 460],
            AR_NORM:  [52,  85,  45,  62],
            QUANT:    [8,   22,   7,  14],
            TOPK:     [10,  18,   8,  12],
            ACT:      [5,   0,   0,   0],
            CACHE:    [8,   0,   0,   0],
            other:    [40,  95,  34,  50]
          },
          sideBySide: {
            labels: ['MLA', 'MOE GEMM', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [18, 185, 42, 24, 14, 10, 38, 45],
            roofline:  [10, 125, 20,  8,  6,  4, 10, 12],
            after:     [14, 165, 32, 16, 10,  8, 28, 32],
            reference: [24, 140, 24, 10,  8,  5, 12, 15]
          }
        },
        '8K / 1K': {
          subtitle: 'DeepSeek R1 0528 · FP4 · ISL 8K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [95,  155,  82, 105],
            GEMM_MOE: [340, 560, 310, 415],
            AR_NORM:  [45,  74,  38,  54],
            QUANT:    [10,  26,   9,  16],
            TOPK:     [9,   15,   7,  11],
            ACT:      [5,   0,    0,   0],
            CACHE:    [20,  0,    0,   0],
            other:    [35,  82,  30,  44]
          },
          sideBySide: {
            labels: ['MLA', 'MOE GEMM', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [45, 168, 38, 26, 12, 9,  32, 40],
            roofline:  [26, 112, 16,  8,  5, 4,  11,  9],
            after:     [32, 150, 28, 17,  9, 7,  24, 28],
            reference: [38, 125, 20, 10,  7, 5,  14, 12]
          }
        },
        '1K / 8K': {
          subtitle: 'DeepSeek R1 0528 · FP4 · ISL 1K / OSL 8K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [35,  55,  30,  38],
            GEMM_MOE: [410, 670, 375, 500],
            AR_NORM:  [65, 105,  56,  76],
            QUANT:    [10,  25,   8,  16],
            TOPK:     [11,  20,   9,  14],
            ACT:      [6,   0,    0,   0],
            CACHE:    [25,  0,    0,   0],
            other:    [48, 110,  40,  58]
          },
          sideBySide: {
            labels: ['MLA', 'MOE GEMM', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [22, 200, 50, 28, 16, 11, 42, 50],
            roofline:  [12, 135, 22,  9,  7,  4, 14, 12],
            after:     [16, 180, 38, 18, 12,  9, 32, 36],
            reference: [26, 152, 28, 11,  9,  6, 18, 16]
          }
        }
      },
      FP8: {
        '1K / 1K': {
          subtitle: 'DeepSeek R1 0528 · FP8 · ISL 1K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [32,  48,  28,  35],
            GEMM_MOE: [350, 580, 320, 430],
            AR_NORM:  [48,  78,  42,  56],
            QUANT:    [12,  30,  10,  20],
            TOPK:     [9,   16,   8,  11],
            ACT:      [6,   0,   0,   0],
            CACHE:    [7,   0,   0,   0],
            other:    [38,  88,  32,  48]
          },
          sideBySide: {
            labels: ['MLA', 'MOE GEMM', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [20, 175, 40, 28, 16, 10, 36, 42],
            roofline:  [12, 118, 18,  8,  6,  4,  9, 11],
            after:     [16, 155, 30, 18, 12,  8, 26, 30],
            reference: [26, 132, 22, 10,  8,  5, 11, 14]
          }
        },
        '8K / 1K': {
          subtitle: 'DeepSeek R1 0528 · FP8 · ISL 8K / OSL 1K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [105, 168,  92, 115],
            GEMM_MOE: [315, 520, 290, 390],
            AR_NORM:  [42,  68,  36,  50],
            QUANT:    [14,  32,  12,  22],
            TOPK:     [8,   14,   7,  10],
            ACT:      [5,   0,    0,   0],
            CACHE:    [18,  0,    0,   0],
            other:    [34,  78,  29,  42]
          },
          sideBySide: {
            labels: ['MLA', 'MOE GEMM', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [48, 158, 36, 30, 14, 9,  30, 38],
            roofline:  [28, 105, 14, 10,  5, 4,  10,  9],
            after:     [34, 140, 26, 20, 10, 7,  22, 26],
            reference: [40, 118, 18, 12,  7, 5,  13, 12]
          }
        },
        '1K / 8K': {
          subtitle: 'DeepSeek R1 0528 · FP8 · ISL 1K / OSL 8K',
          stack: {
            labels: ['B200\n(Reference)', 'MI355X\n(Before)', 'MI355X\n(Roofline)', 'MI355X\n(After Opt)'],
            ATTN:     [40,  62,  34,  44],
            GEMM_MOE: [380, 625, 345, 465],
            AR_NORM:  [58,  95,  50,  68],
            QUANT:    [14,  34,  12,  22],
            TOPK:     [10,  18,   8,  13],
            ACT:      [6,   0,    0,   0],
            CACHE:    [22,  0,    0,   0],
            other:    [42,  98,  36,  52]
          },
          sideBySide: {
            labels: ['MLA', 'MOE GEMM', 'AR/NORM', 'QUANT', 'TOPK', 'ACT', 'CACHE', 'other'],
            before:    [24, 190, 46, 32, 18, 11, 40, 46],
            roofline:  [14, 126, 20, 10,  7,  4, 12, 11],
            after:     [18, 170, 34, 22, 13,  9, 30, 33],
            reference: [28, 142, 25, 12,  9,  6, 16, 15]
          }
        }
      }
    }
  };

  /* ══════ ARCHITECTURE DIAGRAM DATA ══════ */
  const ARCH_DATA = {
    'gpt-oss 120B': {
      org: 'OpenAI',
      params: '120B total · 5B active · 131K context',
      embedSub: 'd = 2,880 · vocab = 201,088',
      block1Title: 'Sliding Attention + Sink',
      block1Sub: '×18 layers · Top-4/128 MoE',
      showAlternating: true,
      alternatingText: '↕ alternating every layer',
      block2Title: 'Causal Grouped Query Attention',
      block2Sub: '×18 layers · Top-4/128 MoE',
      outputSub: 'vocab = 201,088',
      chipType: 'MoE', chipAttn: 'Sink/Full GQA', chipSize: '120B',
      sumType: 'MoE', sumLayers: '36', sumAttn: 'Sink/Full GQA', sumCtx: '131K', sumExperts: '4/128',
      features: ['Alternating Sliding/Full Attention', 'Attention Sink Tokens', 'YaRN RoPE (factor=32)', 'MXFP4 Quantization'],
      release: 'Released by OpenAI on Jun 13, 2025'
    },
    'DeepSeek R1 0528': {
      org: 'DeepSeek',
      params: '671B total · 37B active · 128K context',
      embedSub: 'd = 7,168 · vocab = 129,280',
      block1Title: 'Dense Transformer Block',
      block1Sub: '×3 dense layers · FFN = 18,432',
      showAlternating: false,
      alternatingText: '',
      block2Title: 'MoE Transformer Block',
      block2Sub: '×58 MoE layers · Multi-head Latent Attention · Top-8/257',
      outputSub: 'vocab = 129,280',
      chipType: 'MoE', chipAttn: 'MLA', chipSize: '671B',
      sumType: 'MoE', sumLayers: '3D + 58M', sumAttn: 'MLA', sumCtx: '128K', sumExperts: '8/257',
      features: ['Multi-head Latent Attention', 'Auxiliary-loss-free Load Balancing', 'Multi-Token Prediction'],
      release: 'Released by DeepSeek on May 28, 2025'
    }
  };

  /* ══════ ARCHITECTURE DIAGRAM UPDATE ══════ */
  function updateArchDiagram(model) {
    const data = ARCH_DATA[model];
    if (!data) return;

    // Update chips in header
    const chipType = document.getElementById('arch-chip-type');
    const chipAttn = document.getElementById('arch-chip-attn');
    const chipSize = document.getElementById('arch-chip-size');
    if (chipType) chipType.textContent = data.chipType;
    if (chipAttn) chipAttn.textContent = data.chipAttn;
    if (chipSize) chipSize.textContent = data.chipSize;

    // Update diagram content
    const setTextById = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
    setTextById('arch-org', data.org);
    setTextById('arch-params', data.params);
    setTextById('arch-embed-sub', data.embedSub);
    setTextById('arch-block-1-title', data.block1Title);
    setTextById('arch-block-1-sub', data.block1Sub);
    setTextById('arch-block-2-title', data.block2Title);
    setTextById('arch-block-2-sub', data.block2Sub);
    setTextById('arch-output-sub', data.outputSub);

    // Alternating label
    const altEl = document.getElementById('arch-alternating');
    if (altEl) {
      altEl.textContent = data.alternatingText;
      altEl.style.display = data.showAlternating ? '' : 'none';
    }

    // Summary table
    setTextById('as-type', data.sumType);
    setTextById('as-layers', data.sumLayers);
    setTextById('as-attn', data.sumAttn);
    setTextById('as-ctx', data.sumCtx);
    setTextById('as-experts', data.sumExperts);

    // Features
    const featureIds = ['af-1', 'af-2', 'af-3', 'af-4'];
    featureIds.forEach((id, i) => {
      const el = document.getElementById(id);
      if (el) {
        if (i < data.features.length) {
          el.textContent = data.features[i];
          el.style.display = '';
        } else {
          el.style.display = 'none';
        }
      }
    });

    // Release
    setTextById('arch-release', data.release);
  }

  /* ══════ ARCH SECTION TOGGLE ══════ */
  const archHeader = document.getElementById('arch-header');
  const archSection = document.getElementById('arch-section');
  if (archHeader && archSection) {
    archHeader.addEventListener('click', () => {
      archSection.classList.toggle('collapsed');
    });
  }

  /* ══════ Y-AXIS METRIC — "Building" ══════ */
  function handleYAxisChange(metric) {
    const isDefault = (metric === 'Token Throughput per GPU');
    document.querySelectorAll('.chart-card').forEach(card => {
      const chartContainer = card.querySelector('.chart-container');
      const building = card.querySelector('.chart-building');
      if (!chartContainer || !building) return;
      if (isDefault) {
        chartContainer.style.display = '';
        building.style.display = 'none';
      } else {
        chartContainer.style.display = 'none';
        building.style.display = '';
        building.querySelector('.building-title').textContent = 'Building…';
        building.querySelector('.building-sub').textContent = `"${metric}" — data will be available soon.`;
      }
    });
  }

  /* ══════ CHART SUBTITLES UPDATE ══════ */
  function updateChartSubtitles() {
    const sel = getSelections();
    const sub = `${sel.model} · ${sel.precision} · ${sel.isl_osl} · Source: SemiAnalysis InferenceX™ · Updated: 03/04/2026`;
    document.querySelectorAll('.chart-subtitle-text').forEach(el => {
      el.textContent = sub;
    });
  }

  /* ══════ CHART ZOOM (EXPAND / COLLAPSE) ══════ */
  document.querySelectorAll('.chart-zoom-btn').forEach(btn => {
    btn.addEventListener('click', e => {
      e.stopPropagation();
      const targetId = btn.dataset.target;
      const card = document.getElementById(targetId);
      if (!card) return;

      const isMaximized = card.classList.contains('maximized');

      document.querySelectorAll('.chart-card.maximized').forEach(c => {
        c.classList.remove('maximized');
      });

      if (!isMaximized) {
        card.classList.add('maximized');
        btn.textContent = '⤡';
        const row = card.closest('.card-row');
        if (row) {
          row.querySelectorAll('.chart-card').forEach(sibling => {
            if (sibling !== card) sibling.style.display = 'none';
          });
        }
      } else {
        btn.textContent = '⤢';
        const row = card.closest('.card-row');
        if (row) {
          row.querySelectorAll('.chart-card').forEach(sibling => {
            sibling.style.display = '';
          });
        }
      }

      setTimeout(() => {
        window.dispatchEvent(new Event('resize'));
      }, 100);
    });
  });

  /* ──── Bridge Plan Table Row Toggle ──── */
  document.querySelectorAll('.bt-row').forEach(row => {
    row.addEventListener('click', (e) => {
      // Don't toggle if clicking the checkbox
      if (e.target.classList.contains('bt-check')) return;
      const group = row.closest('.bt-row-group');
      group.classList.toggle('expanded');
      const toggle = row.querySelector('.bt-toggle');
      toggle.textContent = group.classList.contains('expanded') ? '▾' : '▸';
    });
  });

  /* ──── Bridge Plan Checkbox Toggle ──── */
  document.querySelectorAll('.bt-check').forEach(check => {
    check.addEventListener('click', (e) => {
      e.stopPropagation();
      const row = check.closest('.bt-row');
      const isSelected = row.classList.contains('selected');
      row.classList.toggle('selected', !isSelected);
      check.textContent = isSelected ? '☐' : '☑';
      check.classList.toggle('unchecked', isSelected);

      // Also update kernel selections inside
      const group = row.closest('.bt-row-group');
      group.querySelectorAll('.kernel-row').forEach(kr => {
        kr.classList.toggle('selected', !isSelected);
        const kc = kr.querySelector('.kernel-check');
        kc.textContent = isSelected ? '☐' : '☑';
        kc.classList.toggle('unchecked', isSelected);
        kr.querySelector('.kernel-name').classList.toggle('muted', isSelected);
      });

      updateBridgePlanSummary();
    });
  });

  /* ──── Kernel Row Checkbox Toggle ──── */
  document.querySelectorAll('.bt-kernels .kernel-row').forEach(kr => {
    kr.addEventListener('click', () => {
      const isSelected = kr.classList.contains('selected');
      kr.classList.toggle('selected', !isSelected);
      const kc = kr.querySelector('.kernel-check');
      kc.textContent = isSelected ? '☐' : '☑';
      kc.classList.toggle('unchecked', isSelected);
      kr.querySelector('.kernel-name').classList.toggle('muted', isSelected);
      updateBridgePlanSummary();
    });
  });

  /* ──── Saving data per plan ──── */
  const PLAN_SAVING = { p1: 171.5, p2: 26.0, p3: 19.1, p4: 10.6, p5: 4.7 };
  const E2E_BASELINE = 714.8;

  function updateBridgePlanSummary() {
    const selectedKernels = [];
    let totalSaving = 0;
    const seenPlans = new Set();
    document.querySelectorAll('.bt-kernels .kernel-row.selected').forEach(kr => {
      const name = kr.querySelector('.kernel-name').textContent.trim();
      selectedKernels.push(name);
      const plan = kr.closest('.bt-row-group').dataset.plan;
      if (!seenPlans.has(plan)) {
        seenPlans.add(plan);
        totalSaving += PLAN_SAVING[plan] || 0;
      }
    });
    const planCount = seenPlans.size;
    const pctSaving = ((totalSaving / E2E_BASELINE) * 100).toFixed(1);
    const afterTime = (E2E_BASELINE - totalSaving).toFixed(1);

    const summaryLine = document.querySelector('.plan-summary .summary-line');
    const summaryDetail = document.querySelector('.plan-summary .summary-detail');
    if (summaryLine) {
      summaryLine.textContent = `💡 ${selectedKernels.length} kernels selected from ${planCount} plans · Total est. saving: -${totalSaving.toFixed(1)}ms (${pctSaving}%)`;
    }
    if (summaryDetail) {
      summaryDetail.textContent = selectedKernels.length > 0
        ? `Selected → ${selectedKernels.join(', ')}`
        : 'No kernels selected';
    }

    // Update E2E comparison row
    const beforeEl = document.getElementById('e2e-before-val');
    const afterEl = document.getElementById('e2e-after-val');
    const deltaEl = document.getElementById('e2e-delta-val');
    if (beforeEl) beforeEl.textContent = `${E2E_BASELINE}ms`;
    if (afterEl) afterEl.textContent = `${afterTime}ms`;
    if (deltaEl) deltaEl.textContent = `-${totalSaving.toFixed(1)}ms (${pctSaving}%)`;

    // Update kernel dropdown options to match selected kernels
    updateKernelDropdownOptions(selectedKernels);
  }

  function updateKernelDropdownOptions(selectedKernels) {
    const menu = document.getElementById('opt-kernel-menu');
    const valSpan = document.querySelector('#opt-dd-kernel .dd-value');
    if (!menu || !valSpan) return;

    if (selectedKernels.length === 0) {
      menu.innerHTML = '<div class="dd-item active" data-val="">No kernels selected</div>';
      valSpan.textContent = 'No kernels selected';
      return;
    }

    menu.innerHTML = selectedKernels.map((name, i) =>
      `<div class="dd-item${i === 0 ? ' active' : ''}" data-val="${name}">${name}</div>`
    ).join('');

    // If current value not in list, select first
    if (!selectedKernels.includes(valSpan.textContent.trim())) {
      valSpan.textContent = selectedKernels[0];
      updateKernelDetail(selectedKernels[0]);
    }

    // Rebind click events
    menu.querySelectorAll('.dd-item').forEach(item => {
      item.addEventListener('click', () => {
        menu.querySelectorAll('.dd-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        valSpan.textContent = item.dataset.val;
        menu.classList.remove('open');
        updateKernelDetail(item.dataset.val);
      });
    });
  }

  /* ──── Start GEAK Agent Button (opens settings first) ──── */
  const btnGeakFull = document.querySelector('#opt-bridge-plan-card .btn-full, #page-optimization .btn-full');
  const geakOverlay = document.getElementById('geak-settings-overlay');
  const geakApplyBtn = document.getElementById('geak-settings-apply');
  const geakCancelBtn = document.getElementById('geak-settings-cancel');
  const geakCloseBtn = document.getElementById('geak-settings-close');

  function closeGeakSettings() {
    if (geakOverlay) geakOverlay.style.display = 'none';
  }

  if (btnGeakFull && geakOverlay) {
    btnGeakFull.addEventListener('click', () => {
      // Open settings modal instead of directly starting
      geakOverlay.style.display = '';
    });
  }

  // Cancel / Close
  if (geakCancelBtn) geakCancelBtn.addEventListener('click', closeGeakSettings);
  if (geakCloseBtn) geakCloseBtn.addEventListener('click', closeGeakSettings);
  if (geakOverlay) {
    geakOverlay.addEventListener('click', e => {
      if (e.target === geakOverlay) closeGeakSettings();
    });
  }

  /* ──── GEAK Model Dropdown ──── */
  const geakModelTrigger = document.getElementById('geak-model-trigger');
  const geakModelDropdown = document.getElementById('geak-model-dropdown');
  const geakModelValue = document.getElementById('geak-model-value');
  if (geakModelTrigger && geakModelDropdown) {

    geakModelTrigger.addEventListener('click', e => {
      e.stopPropagation();
      const isOpen = geakModelDropdown.classList.toggle('open');
      geakModelTrigger.classList.toggle('open', isOpen);
    });

    geakModelDropdown.addEventListener('click', e => {
      e.stopPropagation();
      const opt = e.target.closest('.geak-model-option');
      if (!opt) return;
      geakModelDropdown.querySelectorAll('.geak-model-option').forEach(o => o.classList.remove('active'));
      opt.classList.add('active');
      if (geakModelValue) geakModelValue.textContent = opt.dataset.model;
      geakModelDropdown.classList.remove('open');
      geakModelTrigger.classList.remove('open');
    });

    document.addEventListener('click', e => {
      if (geakModelTrigger.contains(e.target) || geakModelDropdown.contains(e.target)) return;
      geakModelDropdown.classList.remove('open');
      geakModelTrigger.classList.remove('open');
    });
  }

  // Apply & Start
  if (geakApplyBtn) {
    geakApplyBtn.addEventListener('click', () => {
      closeGeakSettings();

      // Read parallelism setting
      const parallelVal = parseInt(document.getElementById('geak-set-parallel')?.value || '1', 10);

      // Show all GEAK agent cards
      document.querySelectorAll('.geak-agent-card').forEach(card => {
        card.classList.remove('opt-hidden');
        card.classList.add('opt-visible');
      });

      // Apply parallelism-dependent state to kernel 2 (fp4_dequant_gemm)
      applyParallelismState(parallelVal);

      // Update status items to match selected kernels
      updateStatusVisibility();

      // Visual feedback
      if (btnGeakFull) {
        btnGeakFull.textContent = '✅ GEAK Agent Started!';
        btnGeakFull.classList.add('btn-success');
        setTimeout(() => {
          btnGeakFull.textContent = '🚀 Start GEAK Agent';
          btnGeakFull.classList.remove('btn-success');
        }, 2000);
      }
    });
  }

  /* ──── Parallelism: control kernel 2 state ──── */
  function applyParallelismState(parallelVal) {
    const geakItems = document.querySelectorAll('#geak-merged-status-body .geak-expand-item');
    const item2 = geakItems[1]; // fp4_dequant_gemm
    if (!item2) return;

    const statusLabel = item2.querySelector('.geak-status-label');
    const subSpan = item2.querySelector('.geak-item-sub');
    const detailChips = item2.querySelector('.geak-expand-detail');
    const pipeSteps = item2.querySelectorAll('.pipe-step');
    const pipeConnectors = item2.querySelectorAll('.pipe-step-connector');

    // Header subtitle for GEAK Strategies card
    const cardSub = document.querySelector('#geak-strategies-merged .card-subtitle');

    if (parallelVal >= 2) {
      // ── Parallelism ≥ 2: fp4_dequant_gemm → Validated ──
      item2.classList.remove('processing');
      item2.classList.add('validated');

      if (statusLabel) {
        statusLabel.className = 'geak-status-label validated';
        statusLabel.textContent = 'Validated ✅';
      }
      if (subSpan) subSpan.textContent = 'MoE GEMM Opt. · 2m 08s';

      // Pipeline: Ready ✓ → Sent ✓ → Processing ✓ → Validated ✓ → Merged ○
      const stepData = [
        { state: 'done', icon: '✓', name: 'Ready', time: '00:00' },
        { state: 'done', icon: '✓', name: 'Sent', time: '00:08' },
        { state: 'done', icon: '✓', name: 'Processing', time: '01:20' },
        { state: 'done', icon: '✓', name: 'Validated', time: '02:08' },
        { state: 'pending', icon: '○', name: 'Merged', time: '—' }
      ];
      pipeSteps.forEach((step, i) => {
        if (!stepData[i]) return;
        const d = stepData[i];
        step.className = `pipe-step ${d.state}`;
        const iconEl = step.querySelector('.pipe-step-icon');
        if (iconEl) {
          iconEl.className = `pipe-step-icon ${d.state}`;
          iconEl.classList.remove('pulsing');
          iconEl.textContent = d.icon;
        }
        const nameEl = step.querySelector('.pipe-step-name');
        if (nameEl) nameEl.textContent = d.name;
        const timeEl = step.querySelector('.pipe-step-time');
        if (timeEl) timeEl.textContent = d.time;
      });
      pipeConnectors.forEach((conn, i) => {
        conn.className = i < 3 ? 'pipe-step-connector done' : 'pipe-step-connector pending';
      });

      // Detail chips: show validated results
      if (detailChips) {
        detailChips.innerHTML = `
          <span class="detail-chip">BW: 28% (pre-opt)</span>
          <span class="detail-chip">Est. -26.0ms</span>
          <span class="detail-chip green-chip">Speedup: 1.8×</span>
          <span class="detail-chip e2e-chip">E2E: 7.2% ▲</span>
        `;
      }

      // Update card subtitle
      if (cardSub) cardSub.textContent = '3 kernels · 2 validated · 0 processing · 1 sent';

    } else {
      // ── Parallelism = 1: fp4_dequant_gemm stays Processing ──
      item2.classList.remove('validated');
      item2.classList.add('processing');

      if (statusLabel) {
        statusLabel.className = 'geak-status-label processing';
        statusLabel.textContent = 'Processing 🔄';
      }
      if (subSpan) subSpan.textContent = 'MoE GEMM Opt. · 1m 12s';

      // Pipeline: Ready ✓ → Sent ✓ → Processing ⟳ → Validated ○ → Merged ○
      const stepData = [
        { state: 'done', icon: '✓', name: 'Ready', time: '00:00' },
        { state: 'done', icon: '✓', name: 'Sent', time: '00:08' },
        { state: 'active', icon: '⟳', name: 'Processing', time: '01:12…', pulsing: true },
        { state: 'pending', icon: '○', name: 'Validated', time: '—' },
        { state: 'pending', icon: '○', name: 'Merged', time: '—' }
      ];
      pipeSteps.forEach((step, i) => {
        if (!stepData[i]) return;
        const d = stepData[i];
        step.className = `pipe-step ${d.state}`;
        const iconEl = step.querySelector('.pipe-step-icon');
        if (iconEl) {
          iconEl.className = `pipe-step-icon ${d.state}`;
          if (d.pulsing) iconEl.classList.add('pulsing');
          iconEl.textContent = d.icon;
        }
        const nameEl = step.querySelector('.pipe-step-name');
        if (nameEl) nameEl.textContent = d.name;
        const timeEl = step.querySelector('.pipe-step-time');
        if (timeEl) timeEl.textContent = d.time;
      });
      pipeConnectors.forEach((conn, i) => {
        if (i < 1) conn.className = 'pipe-step-connector done';
        else if (i === 1) conn.className = 'pipe-step-connector active';
        else conn.className = 'pipe-step-connector pending';
      });

      // Detail chips: show in-progress state
      if (detailChips) {
        detailChips.innerHTML = `
          <span class="detail-chip">Fuse dequant + GEMM</span>
          <span class="detail-chip">Est. -26.0ms</span>
          <span class="detail-chip blue-chip">Iter 3/8</span>
        `;
      }

      // Update card subtitle
      if (cardSub) cardSub.textContent = '3 kernels · 1 validated · 1 processing · 1 sent';
    }
  }

  function updateStatusVisibility() {
    const selectedKernels = [];
    document.querySelectorAll('.bt-kernels .kernel-row.selected').forEach(kr => {
      selectedKernels.push(kr.querySelector('.kernel-name').textContent.trim());
    });

    // Show/hide GEAK status items based on selection
    document.querySelectorAll('.geak-expand-item').forEach(item => {
      const name = item.querySelector('.geak-item-name').textContent.trim();
      if (selectedKernels.includes(name)) {
        item.style.display = '';
      } else {
        item.style.display = 'none';
      }
    });

    // Update kernel dropdown
    updateKernelDropdownOptions(selectedKernels);
  }

  /* ──── Optimization Model Search Dropdown ──── */
  const optModelDD = document.querySelector('#opt-dd-model');
  if (optModelDD) {
    const trigger = optModelDD.querySelector('.dropdown-trigger');
    const menu = optModelDD.querySelector('.dropdown-menu');
    const searchInput = menu ? menu.querySelector('.search-dropdown-input') : null;
    const searchList = menu ? menu.querySelector('.search-dropdown-list') : null;

    if (trigger && menu) {
      trigger.addEventListener('click', e => {
        e.stopPropagation();
        menu.classList.toggle('open');
        if (menu.classList.contains('open') && searchInput) {
          searchInput.focus();
          searchInput.value = '';
          // Show all items
          if (searchList) searchList.querySelectorAll('.dd-item').forEach(i => i.style.display = '');
        }
      });

      // Search filtering
      if (searchInput && searchList) {
        searchInput.addEventListener('input', () => {
          const query = searchInput.value.toLowerCase();
          searchList.querySelectorAll('.dd-item').forEach(item => {
            const text = item.textContent.toLowerCase();
            item.style.display = text.includes(query) ? '' : 'none';
          });
        });
        searchInput.addEventListener('click', e => e.stopPropagation());
      }

      // Item selection
      if (searchList) {
        searchList.querySelectorAll('.dd-item').forEach(item => {
        item.addEventListener('click', () => {
            searchList.querySelectorAll('.dd-item').forEach(i => i.classList.remove('active'));
          item.classList.add('active');
            const ddValue = optModelDD.querySelector('.dd-value');
            if (ddValue) {
              ddValue.textContent = item.dataset.val;
              ddValue.classList.remove('dd-placeholder');
            }
          menu.classList.remove('open');

            // Show version info
            const versionEl = document.getElementById('opt-bridge-version');
            if (versionEl) {
              const fw = item.dataset.fw || 'SGLang';
              const hw = item.dataset.hw || 'MI355X';
              const prec = item.dataset.prec || 'FP4';
              versionEl.textContent = `${fw} · ${hw} · ${prec} — 2026-03-04`;
              versionEl.style.display = '';
            }

            // Update Bridge Plan title and show content
            const bpTitle = document.getElementById('opt-bridge-plan-title');
            if (bpTitle) bpTitle.textContent = `Optimization Plan — ${item.dataset.val}`;
            const bpSub = document.getElementById('opt-bridge-plan-sub');
            const bpEmpty = document.getElementById('bridge-plan-empty');
            if (bpEmpty) bpEmpty.style.display = 'none';
            const bpContent = document.getElementById('bridge-plan-content');
            const bpBuilding = document.getElementById('bridge-plan-building');

            const isGptOss = item.dataset.val.toLowerCase().includes('gpt-oss');
            if (isGptOss) {
              // Show building state for gpt-oss
              if (bpContent) bpContent.style.display = 'none';
              if (bpBuilding) bpBuilding.style.display = '';
              if (bpSub) bpSub.textContent = 'Plan is being built — kernel profiling in progress';
              // Hide GEAK agent cards since plan is building
              document.querySelectorAll('.geak-agent-card').forEach(card => {
                card.classList.add('opt-hidden');
                card.classList.remove('opt-visible');
              });
            } else {
              // Show normal plan content
              if (bpContent) bpContent.style.display = '';
              if (bpBuilding) bpBuilding.style.display = 'none';
              if (bpSub) bpSub.textContent = '5 optimization items · Click row to expand kernel details';
            }

            // Update E2E card subtitle to match selected model
            const e2eSub = document.getElementById('opt-e2e-subtitle');
            if (e2eSub) {
              const fw = item.dataset.fw || 'SGLang';
              const hw = item.dataset.hw || 'MI355X';
              const prec = item.dataset.prec || 'FP4';
              e2eSub.textContent = `${item.dataset.val} · ${fw} · ${hw} · ${prec} · 1k/1k · conc=4`;
            }
        });
      });
      }
    }
  }

  /* ──── Kernel Detail Dropdown ──── */
  const optKernelDD = document.querySelector('#opt-dd-kernel');
  if (optKernelDD) {
    const trigger = optKernelDD.querySelector('.dropdown-trigger');
    const menu = optKernelDD.querySelector('.dropdown-menu');
    if (trigger && menu) {
      trigger.addEventListener('click', e => {
        e.stopPropagation();
        menu.classList.toggle('open');
      });
      menu.querySelectorAll('.dd-item').forEach(item => {
        item.addEventListener('click', () => {
          menu.querySelectorAll('.dd-item').forEach(i => i.classList.remove('active'));
          item.classList.add('active');
          optKernelDD.querySelector('.dd-value').textContent = item.dataset.val;
          menu.classList.remove('open');
          updateKernelDetail(item.dataset.val);
        });
      });
    }
  }

  /* Close opt dropdowns on outside click */
  document.addEventListener('click', () => {
    document.querySelectorAll('#opt-dd-model .dropdown-menu.open, #opt-dd-kernel .dropdown-menu.open').forEach(m => m.classList.remove('open'));
  });

  /* ──── GEAK Status Expand/Collapse ──── */
  document.querySelectorAll('.geak-expand-header').forEach(header => {
    header.addEventListener('click', () => {
      const item = header.closest('.geak-expand-item');
      const isExpanded = item.classList.contains('expanded');
      item.classList.toggle('expanded', !isExpanded);
      const toggle = header.querySelector('.geak-expand-toggle');
      toggle.textContent = isExpanded ? '▸' : '▾';
    });
  });

  /* ──── Inline Strategy Toggle (under each pipeline item) ──── */
  document.querySelectorAll('.inline-strategy-toggle-btn').forEach(btn => {
    btn.addEventListener('click', e => {
      e.stopPropagation();
      const targetId = btn.dataset.target;
      const body = document.getElementById(targetId);
      const icon = btn.querySelector('.inline-strategy-toggle-icon');
      if (body) {
        const isHidden = body.style.display === 'none';
        body.style.display = isHidden ? '' : 'none';
        if (icon) icon.textContent = isHidden ? '▾' : '▸';
      }
    });
  });

  /* ──── Report Model Expand/Collapse ──── */
  document.querySelectorAll('.report-model-header').forEach(header => {
    header.addEventListener('click', () => {
      const model = header.closest('.report-model');
      const isExpanded = model.classList.contains('expanded');
      model.classList.toggle('expanded', !isExpanded);
      model.classList.toggle('collapsed', isExpanded);
      header.classList.toggle('active', !isExpanded);
      const toggle = header.querySelector('.report-toggle');
      toggle.textContent = model.classList.contains('expanded') ? '▾' : '▸';
      const body = model.querySelector('.report-model-body');
      if (body) body.style.display = model.classList.contains('expanded') ? '' : 'none';
    });
  });

  /* ──── Report Model Checkbox (batch select) ──── */
  function updateBatchCount() {
    const checked = document.querySelectorAll('.rpt-model-cb:checked');
    const batchActions = document.getElementById('report-batch-actions');
    const batchCount = document.getElementById('batch-count');
    if (batchActions) {
      batchActions.style.display = checked.length > 0 ? '' : 'none';
    }
    if (batchCount) {
      batchCount.textContent = `${checked.length} selected`;
    }
  }

  document.querySelectorAll('.rpt-model-cb').forEach(cb => {
    cb.addEventListener('change', updateBatchCount);
  });

  const btnBatchExport = document.getElementById('btn-batch-export');
  if (btnBatchExport) {
    btnBatchExport.addEventListener('click', () => {
      const checked = document.querySelectorAll('.rpt-model-cb:checked');
      const names = [];
      checked.forEach(cb => {
        const header = cb.closest('.report-model-header');
        const nameEl = header ? header.querySelector('.report-model-name') : null;
        if (nameEl) names.push(nameEl.textContent.trim());
      });
      if (names.length === 0) return;
      // Visual feedback
      btnBatchExport.textContent = `✅ Exporting ${names.length} report(s)…`;
      btnBatchExport.disabled = true;
      setTimeout(() => {
        alert('Batch export initiated for:\\n\\n' + names.join('\\n'));
        btnBatchExport.textContent = '📄 Export Selected Reports (PDF)';
        btnBatchExport.disabled = false;
      }, 800);
    });
  }

  /* ──── Report Subsection Expand/Collapse (Level 2) ──── */
  document.querySelectorAll('.report-subsection-header[data-sub-toggle]').forEach(header => {
    header.addEventListener('click', e => {
      e.stopPropagation();
      const sub = header.closest('.report-subsection');
      const isExpanded = sub.classList.contains('expanded');
      sub.classList.toggle('expanded', !isExpanded);
      sub.classList.toggle('collapsed', isExpanded);
      const toggle = header.querySelector('.report-sub-toggle');
      if (toggle) toggle.textContent = sub.classList.contains('expanded') ? '▾' : '▸';
      const body = sub.querySelector('.report-subsection-body');
      if (body) body.style.display = sub.classList.contains('expanded') ? '' : 'none';
    });
  });

  /* ══════ REPORT PAGE — EXPOSE LOGIC ══════ */

  // Data availability registry: which model+precision+ISL/OSL combos have data
  const REPORT_DATA_REGISTRY = {
    'gpt-oss 120B': {
      FP4: {
        '1K / 1K': { gap: '32%→~27% gap', gapPct: 68, rooflineGap: '19%→15%', rooflinePct: 81, status: 'in_progress',
          profiling: 'Top3: Attn FP8 32.7%, GEMM 18%, Norm 12%',
          kernelOpt: 'Attn +15%, 62.7% WL covered',
          expectedE2E: '~8% E2E uplift',
          e2ePerf: '', color: 'red' },
        '8K / 1K': { gap: '34%→~31% gap', gapPct: 66, rooflineGap: '22%', rooflinePct: 78, status: 'in_progress',
          profiling: 'Top3: Attn FP8 35.1%, GEMM 16%, KV Cache 11%',
          kernelOpt: 'Attn +12%, 62.1% WL covered',
          expectedE2E: '~5% estimated',
          e2ePerf: '', color: 'red' },
        '1K / 8K': { gap: '35% gap', gapPct: 65, rooflineGap: '26%', rooflinePct: 74, status: 'in_progress',
          profiling: 'Top3: Attn FP8 30.4%, GEMM 19%, Decode 14%',
          kernelOpt: '2/5 kernels, 30.4% WL',
          expectedE2E: '—',
          e2ePerf: '', color: 'red' }
      },
      FP8: {
        '1K / 1K': { gap: '26%→~17% gap', gapPct: 74, rooflineGap: '13%→9%', rooflinePct: 87, status: 'done',
          profiling: 'Top3: Attn FP8 34.2%, GEMM 17%, Norm 11%',
          kernelOpt: 'Attn +12%, 62.2% WL covered',
          expectedE2E: '~12% E2E uplift',
          e2ePerf: '~12%', color: 'yellow' },
        '8K / 1K': { gap: '28%→~22% gap', gapPct: 72, rooflineGap: '17%', rooflinePct: 83, status: 'in_progress',
          profiling: 'Top3: Attn FP8 36.0%, GEMM 15%, KV Cache 10%',
          kernelOpt: 'Attn +10%, 61.0% WL covered',
          expectedE2E: '~8% estimated',
          e2ePerf: '', color: 'red' },
        '1K / 8K': { gap: '29% gap', gapPct: 71, rooflineGap: '21%', rooflinePct: 79, status: 'in_progress',
          profiling: 'Top3: Attn FP8 31.8%, GEMM 18%, Decode 13%',
          kernelOpt: '2/5 kernels, 31.8% WL',
          expectedE2E: '—',
          e2ePerf: '', color: 'red' }
      }
    },
    'DeepSeek R1 0528': {
      FP4: {
        '1K / 1K': { gap: '22%→19% gap', gapPct: 78, rooflineGap: '22%→18%', rooflinePct: 78, status: 'done',
          profiling: 'Top3: MoE 31%, MLA Attn 13%, Attn GEMMs 13%',
          kernelOpt: 'MoE +66%, MLA Attn, 5 items, 64.9% WL',
          expectedE2E: '+3.2% validated',
          e2ePerf: '+3.2%', color: 'yellow' },
        '8K / 1K': { gap: '27% gap', gapPct: 73, rooflineGap: '26%', rooflinePct: 74, status: 'in_progress',
          profiling: 'Top3: MoE 28%, MLA Attn 18%, Attn GEMMs 15%',
          kernelOpt: '2/5 items, MoE+MLA, 46% WL',
          expectedE2E: '~2% estimated',
          e2ePerf: '', color: 'red' },
        '1K / 8K': { gap: '25% gap', gapPct: 75, rooflineGap: '29%', rooflinePct: 71, status: 'in_progress',
          profiling: 'Top3: MoE 34%, MLA Attn 22%, Comm 15%',
          kernelOpt: '1/5 items, MoE, 34% WL',
          expectedE2E: '—',
          e2ePerf: '', color: 'red' }
      },
      FP8: {
        '1K / 1K': { gap: '17% gap', gapPct: 83, rooflineGap: '17%', rooflinePct: 83, status: 'in_progress',
          profiling: 'Top3: MoE 28%, MLA Attn 14%, Comm 18%',
          kernelOpt: '0/5 items, paused',
          expectedE2E: '~2% estimated',
          e2ePerf: '', color: 'green' },
        '8K / 1K': { gap: '23% gap', gapPct: 77, rooflineGap: '20%', rooflinePct: 80, status: 'in_progress',
          profiling: 'Top3: MoE 29%, MLA Attn 16%, Attn GEMMs 14%',
          kernelOpt: '1/5 items, MoE, 29% WL',
          expectedE2E: '—',
          e2ePerf: '', color: 'yellow' },
        '1K / 8K': { gap: '21% gap', gapPct: 79, rooflineGap: '23%', rooflinePct: 77, status: 'in_progress',
          profiling: 'Top3: MoE 32%, MLA Attn 20%, Comm 17%',
          kernelOpt: '1/5 items, MoE, 32% WL',
          expectedE2E: '—',
          e2ePerf: '', color: 'yellow' }
      }
    }
  };

  // NV reference data availability
  const NV_REF_DATA = {
    'gpt-oss 120B': {
      FP4: { '1K / 1K': true, '8K / 1K': true, '1K / 8K': true },
      FP8: { '1K / 1K': true, '8K / 1K': true, '1K / 8K': true }
    },
    'DeepSeek R1 0528': {
      FP4: { '1K / 1K': true, '8K / 1K': true, '1K / 8K': true },
      FP8: { '1K / 1K': true, '8K / 1K': true, '1K / 8K': true }
    }
  };

  let exposeHistory = [];
  let exposeIdCounter = 0;

  // Report dropdown: update precision based on model (like overview page)
  const rptModelDD = document.querySelector('#rpt-dd-model');
  const rptPrecDD = document.querySelector('#rpt-dd-precision');

  function updateReportPrecision() {
    if (!rptModelDD || !rptPrecDD) return;
    const model = rptModelDD.querySelector('.dd-value').textContent.trim();
    const menu = rptPrecDD.querySelector('.dropdown-menu');
    const valSpan = rptPrecDD.querySelector('.dd-value');

    // Both models support FP4 and FP8
      menu.innerHTML = '<div class="dd-item active" data-val="FP4">● FP4</div><div class="dd-item" data-val="FP8">■ FP8</div>';
      if (valSpan.textContent !== 'FP4' && valSpan.textContent !== 'FP8') {
        valSpan.textContent = 'FP4';
    }
    // Re-bind click events for new items
    menu.querySelectorAll('.dd-item').forEach(item => {
      item.addEventListener('click', () => {
        menu.querySelectorAll('.dd-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        valSpan.textContent = item.dataset.val;
        rptPrecDD.querySelector('.dropdown-menu').style.display = 'none';
      });
    });
  }

  if (rptModelDD) {
    rptModelDD.querySelector('.dropdown-menu').addEventListener('click', () => {
      setTimeout(updateReportPrecision, 50);
    });
    updateReportPrecision();
  }

  function getPhaseChip(value) {
    // Legacy status codes
    const legacyMap = {
      done: '<span class="phase-chip done">✅ Done</span>',
      wip: '<span class="phase-chip wip">🔄 WIP</span>',
      paused: '<span class="phase-chip paused">⏸ Paused</span>',
      na: '<span class="phase-chip na">—</span>'
    };
    if (legacyMap[value]) return legacyMap[value];
    // Numeric / descriptive string — highlight key numbers
    if (!value || value === '—') return '<span class="phase-chip na">—</span>';
    // Bold numbers/percentages and key terms for readability
    const formatted = value
      .replace(/(\d+\.?\d*%)/g, '<strong>$1</strong>')
      .replace(/(\d+\.?\d*×)/g, '<strong>$1</strong>')
      .replace(/(\+\d+\.?\d*%)/g, '<strong class="dc-green">$1</strong>')
      .replace(/(Top3:)/g, '<span class="dc-label">$1</span>')
      .replace(/(\d+\/\d+ kernels)/g, '<strong>$1</strong>')
      .replace(/(validated|uplift|covered|estimated)/gi, '<em>$1</em>');
    return `<span class="phase-chip data-chip" title="${value}">${formatted}</span>`;
  }

  function getE2eChip(value) {
    if (!value || value === '—') return '<span class="phase-chip na">—</span>';
    // Positive % results → green pill
    if (/^[+~]?\d/.test(value) || value.includes('%')) {
      return `<span class="e2e-pill green">${value}</span>`;
    }
    // In Progress / Started → amber pill
    if (/progress|started/i.test(value)) {
      return `<span class="e2e-pill amber">${value}</span>`;
    }
    // Not started → gray
    if (/not started/i.test(value)) {
      return `<span class="e2e-pill gray">${value}</span>`;
    }
    return `<span class="e2e-pill">${value}</span>`;
  }

  function getGapColor(pct) {
    if (pct >= 100) return 'green';
    if (pct >= 75) return 'yellow';
    return 'red';
  }

  // Track which NV ref columns are visible
  let hasAnyNvRef = false;

  function updateNvRefColumns() {
    // Check if any exposed item has NV ref
    hasAnyNvRef = exposeHistory.some(e => e.nvRef && e.nvRef !== '— None —' && e.nvRef !== '');
    const nvHeader = document.getElementById('st-nvref-header');
    const gapHeader = document.getElementById('st-gap-header');
    if (nvHeader) nvHeader.style.display = hasAnyNvRef ? '' : 'none';
    if (gapHeader) gapHeader.style.display = hasAnyNvRef ? '' : 'none';

    // Also show/hide per-row NV ref + gap cells
    document.querySelectorAll('.st-col.st-nvref').forEach(el => {
      el.style.display = hasAnyNvRef ? '' : 'none';
    });
    document.querySelectorAll('.st-col.st-gap').forEach(el => {
      el.style.display = hasAnyNvRef ? '' : 'none';
    });
  }

  function addStatusRow(entry) {
    const body = document.getElementById('status-body');
    const empty = document.getElementById('status-empty');
    if (empty) empty.style.display = 'none';

    const idx = body.querySelectorAll('.status-row').length;
    const altClass = idx % 2 === 0 ? ' alt' : '';
    const color = entry.color || getGapColor(entry.gapPct || 0);

    const row = document.createElement('div');
    row.className = `status-row${altClass}`;
    row.dataset.exposeId = entry.id;

    // NV Reference cell (beside AMD data)
    const hasNv = entry.nvRef && entry.nvRef !== '— None —' && entry.nvRef !== '';
    const nvRefCell = hasNv
      ? (entry.nvHasData
        ? `<div class="st-col st-nvref"><span class="st-nvref-name">${entry.nvRef}</span><span class="st-nvref-sub">Reference</span></div>`
        : `<div class="st-col st-nvref"><span class="st-nvref-notfound">${entry.nvRef}</span><span class="st-nvref-notfound">Not Found</span></div>`)
      : `<div class="st-col st-nvref"><span class="st-nvref-notfound">—</span></div>`;

    // Gap cell (only meaningful when NV ref exists)
    const gapContent = entry.notFound
      ? '<span class="gap-value red">Not Found</span>'
      : entry.gapPct >= 100
        ? `<span class="gap-value ${color}">${entry.gap}</span>`
        : `<span class="gap-value ${color}">${entry.gap}</span><div class="gap-bar"><div class="gap-fill ${color}" style="width:${entry.gapPct}%"></div></div>`;

    // Roofline gap cell
    const rooflineColor = entry.rooflinePct >= 90 ? 'green' : entry.rooflinePct >= 75 ? 'yellow' : 'red';
    const rooflineContent = entry.notFound
      ? '<span class="gap-value red">—</span>'
      : `<span class="gap-value ${rooflineColor}">${entry.rooflineGap}</span><div class="gap-bar"><div class="gap-fill ${rooflineColor}" style="width:${entry.rooflinePct}%"></div></div>`;

    // Conditional: in_progress → show Expected E2E, hide E2E Perf; done → show E2E Perf, hide Expected E2E
    const isDone = entry.status === 'done';
    const expectedE2ECell = entry.notFound
      ? '<span class="phase-chip na">—</span>'
      : isDone
        ? '<span class="phase-chip na">—</span>'
        : getPhaseChip(entry.expectedE2E);
    const e2ePerfCell = entry.notFound
      ? '<span class="phase-chip na">Not Found</span>'
      : isDone
        ? getE2eChip(entry.e2ePerf)
        : '<span class="phase-chip na">—</span>';

    row.innerHTML = `
      <div class="st-col st-model">
        <div class="st-accent ${color}"></div>
        <div>
          <div class="st-model-name">${entry.modelName}</div>
          <div class="st-model-sub">${entry.configSub}</div>
        </div>
      </div>
      ${nvRefCell}
      <div class="st-col st-gap">${gapContent}</div>
      <div class="st-col st-roofline">${rooflineContent}</div>
      <div class="st-col st-phase">${entry.notFound ? '<span class="phase-chip na">—</span>' : getPhaseChip(entry.profiling)}</div>
      <div class="st-col st-phase">${entry.notFound ? '<span class="phase-chip na">—</span>' : getPhaseChip(entry.kernelOpt)}</div>
      <div class="st-col st-phase">${expectedE2ECell}</div>
      <div class="st-col st-e2e">${e2ePerfCell}</div>
    `;

    body.appendChild(row);
    updateNvRefColumns();
  }

  function addExposeHistoryItem(entry) {
    const list = document.getElementById('expose-history-list');
    const empty = document.getElementById('expose-empty');
    if (empty) empty.style.display = 'none';

    const statusClass = entry.notFound ? 'not-found' : 'found';
    const statusText = entry.notFound ? 'Not Found' : 'Data Found';
    const nvLabel = (entry.nvRef && entry.nvRef !== '— None —' && entry.nvRef !== '')
      ? ` | NV: ${entry.nvRef}` : '';

    const item = document.createElement('div');
    item.className = 'expose-item';
    item.dataset.exposeId = entry.id;
    item.innerHTML = `
      <span class="ei-model">${entry.modelName}</span>
      <span class="ei-config">${entry.configSub}${nvLabel}</span>
      <span class="ei-status ${statusClass}">${statusText}</span>
      <span class="ei-remove" data-expose-id="${entry.id}" title="Remove">✕</span>
    `;

    item.querySelector('.ei-remove').addEventListener('click', () => {
      removeExposeEntry(entry.id);
    });

    list.appendChild(item);
  }

  function removeExposeEntry(id) {
    exposeHistory = exposeHistory.filter(e => e.id !== id);

    const histItem = document.querySelector(`.expose-item[data-expose-id="${id}"]`);
    if (histItem) histItem.remove();

    const statusRow = document.querySelector(`.status-row[data-expose-id="${id}"]`);
    if (statusRow) statusRow.remove();

    if (exposeHistory.length === 0) {
      const empty = document.getElementById('expose-empty');
      if (empty) empty.style.display = '';
      const statusEmpty = document.getElementById('status-empty');
      if (statusEmpty) statusEmpty.style.display = '';
      // Hide detail reports
      const detailCard = document.getElementById('detail-reports-card');
      if (detailCard) detailCard.style.display = 'none';
    }

    updateNvRefColumns();
    updateExposeCounts();
    updateBatchCount();
  }

  function updateExposeCounts() {
    const countEl = document.getElementById('expose-count');
    if (countEl) countEl.textContent = `${exposeHistory.length} items`;

    const statusSub = document.getElementById('status-overview-sub');
    if (statusSub) {
      statusSub.textContent = exposeHistory.length > 0
        ? `${exposeHistory.length} items exposed · Gap = perf gap vs Nvidia B200 (lower is better)`
        : 'No items exposed · Click Expose above to add entries';
    }
  }

  // Expose button handler
  const btnExpose = document.getElementById('btn-expose');
  if (btnExpose) {
    btnExpose.addEventListener('click', () => {
      const model = document.querySelector('#rpt-dd-model .dd-value')?.textContent?.trim() || 'gpt-oss 120B';
      const precision = document.querySelector('#rpt-dd-precision .dd-value')?.textContent?.trim() || 'FP4';
      const islOsl = document.querySelector('#rpt-dd-isl-osl .dd-value')?.textContent?.trim() || '1K / 1K';
      const amdConfig = document.querySelector('#rpt-dd-amd .dd-value')?.textContent?.trim() || 'MI355X (SGLang)';
      const nvRefRaw = document.querySelector('#rpt-dd-nv .dd-value')?.textContent?.trim() || '';
      const nvRef = (nvRefRaw === '— None —' || nvRefRaw === '') ? '' : nvRefRaw;

      // Look up data
      const registry = REPORT_DATA_REGISTRY[model];
      const precData = registry?.[precision];
      const islData = precData?.[islOsl];

      const id = ++exposeIdCounter;
      const modelName = `${model} (${precision})`;
      const configSub = `${islOsl} · ${amdConfig}`;

      // Check NV ref data availability
      const nvHasData = nvRef ? (NV_REF_DATA[model]?.[precision]?.[islOsl] || false) : false;

      const entry = {
        id,
        modelName,
        configSub,
        nvRef: nvRef,
        nvHasData: nvHasData,
        notFound: !islData,
        gap: islData?.gap || 'Not Found',
        gapPct: islData?.gapPct || 0,
        rooflineGap: islData?.rooflineGap || '—',
        rooflinePct: islData?.rooflinePct || 0,
        status: islData?.status || 'in_progress',
        profiling: islData?.profiling || 'na',
        kernelOpt: islData?.kernelOpt || 'na',
        expectedE2E: islData?.expectedE2E || 'na',
        e2ePerf: islData?.e2ePerf || '',
        color: islData?.color || 'red'
      };

      exposeHistory.push(entry);
      addExposeHistoryItem(entry);
      addStatusRow(entry);
      updateExposeCounts();

      // Show the matching Per-Model detail section if it exists
      const reportModelKey = `${model}||${precision}`;
      const reportModelEl = document.querySelector(`.report-model[data-report-model="${reportModelKey}"]`);
      if (reportModelEl) {
        reportModelEl.style.display = '';
      }

      // Show detail reports card if any Per-Model section is now visible
      const detailCard = document.getElementById('detail-reports-card');
      if (detailCard) {
        const anyVisible = detailCard.querySelector('.report-model[style=""]') ||
                           detailCard.querySelector('.report-model:not([style*="display:none"])') ||
                           reportModelEl;
        if (anyVisible) detailCard.style.display = '';
      }

      // Flash the expose button
      btnExpose.textContent = '✅ Exposed!';
      btnExpose.style.background = 'var(--green)';
      setTimeout(() => {
        btnExpose.textContent = '🚀 Expose';
        btnExpose.style.background = '';
      }, 1200);
    });
  }

  /* ══════════════════════════════════════════════════════
     CHARTS (Chart.js)
     ══════════════════════════════════════════════════════ */

  const AMD_RED = '#e4002b';
  const AMD_RED_LIGHT = 'rgba(228,0,43,0.35)';
  const GREEN = '#22c55e';
  const BLUE = '#3b82f6';
  const GRID_COLOR = 'rgba(243,244,246,1)';
  const TICK_COLOR = '#9ca3af';

  const commonScaleOpts = {
    grid: { color: GRID_COLOR, drawBorder: true, borderColor: '#e5e7eb' },
    ticks: { color: TICK_COLOR, font: { size: 10 } }
  };

  // Store chart instances for later update
  let chart1Instance = null;
  let chart2Instance = null;

  function getDataForSelection() {
    const sel = getSelections();
    const modelData = BENCH_DATA[sel.model];
    if (!modelData) return null;

    const islOslData = modelData[sel.isl_osl];
    if (!islOslData) return null;

    const precisionData = islOslData[sel.precision];
    if (!precisionData) return null;

    // Normalize framework name (TRT-LLM → TRT_LLM)
    const fwKey = sel.framework.replace('-', '_');
    const refFwKey = sel.refFramework.replace('-', '_');

    // Get baseline (AMD), optimized, roofline, and reference data
    const hwData = precisionData[sel.hardware];
    const hwOptData = precisionData[sel.hardware + '_OPT'];
    const hwRooflineData = precisionData[sel.hardware + '_ROOFLINE'];
    const refData = precisionData[sel.refHardware];

    const baseline = hwData?.[fwKey] || hwData?.[sel.framework] || null;
    const optimized = hwOptData?.[fwKey] || hwOptData?.[sel.framework] || null;
    const roofline = hwRooflineData?.[fwKey] || hwRooflineData?.[sel.framework] || null;
    const reference = refData?.[refFwKey] || refData?.[sel.refFramework] || null;

    return { baseline, optimized, roofline, reference, sel };
  }

  const CYAN = '#06b6d4';

  function buildChartDatasets(chartType) {
    const result = getDataForSelection();
    if (!result) return [];

    const { baseline, optimized, roofline, reference, sel } = result;
    const datasets = [];

    // Baseline (AMD, dashed line)
    if (baseline) {
      datasets.push({
        label: `${sel.hardware} (${sel.framework}) — Baseline`,
        data: chartType === 'interactivity' ? baseline.interactivity : baseline.latency,
        borderColor: AMD_RED_LIGHT,
        backgroundColor: 'transparent',
        borderWidth: 2,
        borderDash: [6, 4],
        pointRadius: 4,
        pointBackgroundColor: '#fff',
        pointBorderColor: AMD_RED_LIGHT,
        pointBorderWidth: 1.5,
        showLine: true,
        tension: 0.35
      });
    }

    // MI355X Roofline (cyan, dash-dot line — theoretical peak)
    if (roofline) {
      datasets.push({
        label: `${sel.hardware} (Roofline)`,
        data: chartType === 'interactivity' ? roofline.interactivity : roofline.latency,
        borderColor: CYAN,
        backgroundColor: 'transparent',
        borderWidth: 2,
        borderDash: [8, 3, 2, 3],
        pointRadius: 3.5,
        pointBackgroundColor: '#fff',
        pointBorderColor: CYAN,
        pointBorderWidth: 1.5,
        pointStyle: 'triangle',
        showLine: true,
        tension: 0.35
      });
    }

    // Optimized AMD (solid red)
    if (optimized) {
      datasets.push({
        label: `${sel.hardware} (${sel.framework}) — Optimized`,
        data: chartType === 'interactivity' ? optimized.interactivity : optimized.latency,
        borderColor: AMD_RED,
        backgroundColor: 'transparent',
        borderWidth: 2.5,
        pointRadius: 5,
        pointBackgroundColor: '#fff',
        pointBorderColor: AMD_RED,
        pointBorderWidth: 2,
        showLine: true,
        tension: 0.35
      });
    }

    // Reference (NVIDIA, green solid)
    if (reference) {
      datasets.push({
        label: `${sel.refHardware} (${sel.refFramework})`,
        data: chartType === 'interactivity' ? reference.interactivity : reference.latency,
        borderColor: GREEN,
        backgroundColor: 'transparent',
        borderWidth: 2.5,
        pointRadius: 5,
        pointBackgroundColor: '#fff',
        pointBorderColor: GREEN,
        pointBorderWidth: 2,
        showLine: true,
        tension: 0.35
      });
    }

    return datasets;
  }

  function getYMax(datasets) {
    let max = 0;
    datasets.forEach(ds => {
      ds.data.forEach(pt => {
        if (pt.y > max) max = pt.y;
      });
    });
    return Math.ceil(max / 2000) * 2000 + 2000;
  }

  function getXMax(datasets) {
    let max = 0;
    datasets.forEach(ds => {
      ds.data.forEach(pt => {
        if (pt.x > max) max = pt.x;
      });
    });
    return Math.ceil(max / 50) * 50 + 50;
  }

  function updateLegendBox(legendBox, sel, hasBaseline, hasOptimized, hasReference, hasRoofline) {
    if (!legendBox) return;
    const items = legendBox.querySelector('.legend-items');
    if (!items) return;
    items.innerHTML = '';

    if (hasReference) {
      items.innerHTML += `<span class="legend-item"><span class="legend-line green solid"></span><strong>${sel.refHardware} (${sel.refFramework})</strong></span>`;
    }
    if (hasRoofline) {
      items.innerHTML += `<span class="legend-item"><span class="legend-line cyan dashdot"></span><strong>${sel.hardware} (Roofline)</strong></span>`;
    }
    if (hasOptimized) {
      items.innerHTML += `<span class="legend-item"><span class="legend-line red solid"></span><strong>${sel.hardware} (${sel.framework}) — Optimized</strong></span>`;
    }
    if (hasBaseline) {
      items.innerHTML += `<span class="legend-item"><span class="legend-line red dashed"></span>${sel.hardware} (${sel.framework}) — Baseline</span>`;
    }
  }

  function createOrUpdateChart1() {
    const ctx1 = document.getElementById('chart-throughput-interactivity');
    if (!ctx1) return;

    const datasets = buildChartDatasets('interactivity');
    const sel = getSelections();
    const yMax = datasets.length > 0 ? getYMax(datasets) : 24000;
    const xMax = datasets.length > 0 ? getXMax(datasets) : 400;

    const config = {
      type: 'scatter',
      data: { datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor: 'rgba(26,26,46,0.9)',
            titleFont: { size: 12 },
            bodyFont: { size: 11 },
            callbacks: {
              label: ctx => `${ctx.dataset.label}: ${ctx.parsed.y.toLocaleString()} tok/s/gpu @ ${ctx.parsed.x} tok/s/user`
            }
          }
        },
        scales: {
          x: {
            ...commonScaleOpts,
            title: { display: true, text: 'Interactivity (tok/s/user)', color: '#6b7280', font: { size: 11 } },
            min: 0,
            max: xMax,
            ticks: { ...commonScaleOpts.ticks, stepSize: 50 }
          },
          y: {
            ...commonScaleOpts,
            title: { display: true, text: 'Token Throughput per GPU (tok/s/gpu)', color: '#6b7280', font: { size: 11 } },
            min: 0,
            max: yMax,
            ticks: {
              ...commonScaleOpts.ticks,
              stepSize: 2000,
              callback: v => v >= 1000 ? (v / 1000).toFixed(0) + 'k' : v
            }
          }
        }
      }
    };

    if (chart1Instance) {
      chart1Instance.destroy();
    }
    chart1Instance = new Chart(ctx1, config);

    // Update legend
    const result = getDataForSelection();
    const legendBox = document.querySelector('#card-chart1 .chart-legend-box');
    updateLegendBox(legendBox, sel, !!result?.baseline, !!result?.optimized, !!result?.reference, !!result?.roofline);
  }

  function createOrUpdateChart2() {
    const ctx2 = document.getElementById('chart-throughput-latency');
    if (!ctx2) return;

    const datasets = buildChartDatasets('latency');
    const sel = getSelections();
    const yMax = datasets.length > 0 ? getYMax(datasets) : 20000;

    // Compute x max for latency
    let latXMax = 0;
    datasets.forEach(ds => {
      ds.data.forEach(pt => {
        if (pt.x > latXMax) latXMax = pt.x;
      });
    });
    latXMax = Math.ceil(latXMax / 5) * 5 + 5;

    const config = {
      type: 'scatter',
      data: { datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor: 'rgba(26,26,46,0.9)',
            callbacks: {
              label: ctx => `${ctx.dataset.label}: ${ctx.parsed.y.toLocaleString()} tok/s/gpu @ ${ctx.parsed.x}s latency`
            }
          }
        },
        scales: {
          x: {
            ...commonScaleOpts,
            title: { display: true, text: 'End-to-end Latency (s)', color: '#6b7280', font: { size: 11 } },
            min: 0,
            max: latXMax,
            ticks: { ...commonScaleOpts.ticks, stepSize: 2 }
          },
          y: {
            ...commonScaleOpts,
            title: { display: true, text: 'Token Throughput per GPU (tok/s/gpu)', color: '#6b7280', font: { size: 11 } },
            min: 0,
            max: yMax,
            ticks: {
              ...commonScaleOpts.ticks,
              stepSize: 2000,
              callback: v => v >= 1000 ? (v / 1000).toFixed(0) + 'k' : v
            }
          }
        }
      }
    };

    if (chart2Instance) {
      chart2Instance.destroy();
    }
    chart2Instance = new Chart(ctx2, config);

    // Update legend
    const result = getDataForSelection();
    const legendBox = document.querySelector('#card-chart2 .chart-legend-box');
    updateLegendBox(legendBox, sel, !!result?.baseline, !!result?.optimized, !!result?.reference, !!result?.roofline);
  }

  function updateCharts() {
    createOrUpdateChart1();
    createOrUpdateChart2();
  }

  /* ──── Chart 3 & 4: Dynamic Kernel Charts ──── */
  let chart3Instance = null;
  let chart4Instance = null;

  function updateKernelCharts() {
    const sel = getSelections();
    const modelKernel = KERNEL_DATA[sel.model];
    if (!modelKernel) return;
    const precKernel = modelKernel[sel.precision] || modelKernel['FP4'];
    if (!precKernel) return;
    // Lookup by ISL/OSL, fallback to first available
    const kd = precKernel[sel.isl_osl] || precKernel[Object.keys(precKernel)[0]];
    if (!kd) return;

    // Update kernel card subtitle
    const kernelSubtitle = document.querySelector('#card-chart3 .card-subtitle');
    if (kernelSubtitle) kernelSubtitle.textContent = kd.subtitle;

    // Chart 3: Kernel Stack (Horizontal Stacked Bar)
    const ctx3 = document.getElementById('chart-kernel-stack');
    if (ctx3) {
      if (chart3Instance) chart3Instance.destroy();
      chart3Instance = new Chart(ctx3, {
        type: 'bar',
        data: {
          labels: kd.stack.labels,
          datasets: [
            { label: 'ATTN',     data: kd.stack.ATTN,     backgroundColor: '#3b82f6' },
            { label: 'GEMM/MOE', data: kd.stack.GEMM_MOE, backgroundColor: '#f97316' },
            { label: 'AR/NORM',  data: kd.stack.AR_NORM,  backgroundColor: '#22c55e' },
            { label: 'QUANT',    data: kd.stack.QUANT,    backgroundColor: '#ef4444' },
            { label: 'TOPK',     data: kd.stack.TOPK,     backgroundColor: '#8b5cf6' },
            { label: 'ACT',      data: kd.stack.ACT,      backgroundColor: '#92400e' },
            { label: 'CACHE',    data: kd.stack.CACHE,    backgroundColor: '#f9a8d4' },
            { label: 'other',    data: kd.stack.other,    backgroundColor: '#9ca3af' }
          ]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          indexAxis: 'y',
          plugins: {
            legend: { display: false },
            tooltip: {
              callbacks: {
                label: ctx => `${ctx.dataset.label}: ${ctx.parsed.x}k μs`
              }
            }
          },
          scales: {
            x: {
              stacked: true,
              ...commonScaleOpts,
              title: { display: true, text: 'Duration (μs × 1000)', color: '#6b7280', font: { size: 10 } },
              ticks: {
                ...commonScaleOpts.ticks,
                callback: v => v + 'k'
              }
            },
            y: {
              stacked: true,
              ...commonScaleOpts,
              ticks: {
                ...commonScaleOpts.ticks,
                font: { size: 10, weight: 'bold' }
              }
            }
          }
        }
      });
    }

    // Chart 4: Kernel Side-by-Side (Grouped Vertical Bar)
    const ctx4 = document.getElementById('chart-kernel-sidebyside');
    if (ctx4) {
      if (chart4Instance) chart4Instance.destroy();
      chart4Instance = new Chart(ctx4, {
        type: 'bar',
        data: {
          labels: kd.sideBySide.labels,
          datasets: [
            {
              label: 'MI355X (Before)',
              data: kd.sideBySide.before,
              backgroundColor: 'rgba(228,0,43,0.4)'
            },
            {
              label: 'MI355X (Roofline)',
              data: kd.sideBySide.roofline,
              backgroundColor: '#06b6d4',
              borderColor: '#0891b2',
              borderWidth: 1,
              borderDash: [4, 2]
            },
            {
              label: 'MI355X (After Opt)',
              data: kd.sideBySide.after,
              backgroundColor: '#e4002b'
            },
            {
              label: 'B200 (Reference)',
              data: kd.sideBySide.reference,
              backgroundColor: '#22c55e'
            }
          ]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: { display: false },
            tooltip: {
              callbacks: {
                label: ctx => `${ctx.dataset.label}: ${ctx.parsed.y}k μs`
              }
            }
          },
          scales: {
            x: {
              ...commonScaleOpts,
              title: { display: true, text: 'Kernel Categories', color: '#6b7280', font: { size: 10 } },
              ticks: { ...commonScaleOpts.ticks, maxRotation: 35, minRotation: 35 }
            },
            y: {
              ...commonScaleOpts,
              title: { display: true, text: 'Duration (μs × 1000)', color: '#6b7280', font: { size: 10 } },
              ticks: {
                ...commonScaleOpts.ticks,
                callback: v => v + 'k'
              }
            }
          }
        }
      });
    }
  }

  /* ──── Initial Charts ──── */
  createOrUpdateChart1();
  createOrUpdateChart2();
  updateKernelCharts();


  /* ══════════════════════════════════════════════════════════════
     ANALYSIS PAGE — Dynamic Data + Dropdowns
     ══════════════════════════════════════════════════════════════ */

  /* ---------- Simulated ANALYSIS_DATA ---------- */
  const ANALYSIS_DATA = {
    'DeepSeek R1 0528': {
      'FP4': {
        '1K / 1K': {
          '4': {
            e2e: { input: '50.65', output: '50.16', ttft: '90.69ms', itl: '9.53ms', latency: '8,940ms', gap: '22% gap', rooflineGap: '22%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 13, ms: 11.8, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 13, ms: 11.8, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 31, ms: 28.1, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 16.3, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 10.9, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 13, ms: 11.8, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 18, ms: 1.72, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 1.43, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 28, ms: 2.67, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 1.52, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.14, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 11, ms: 1.05, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'MoE expert dispatch severely fragmented — 256-expert top-8 launches up to 256 rocBLAS Cijk kernels per layer × 61 layers per forward pass. BW utilization only 31% of 8 TB/s peak due to small per-expert M dimension and FP4 dequant overhead (~15µs/call).', source: 'Trace' },
              { sev: 'high', text: 'flash_attn_fwd_v2 memory-bound at 38% HBM peak. Compressed KV (d_c=512) saves cache capacity but adds 61 absorb_attn + 61 output_proj = 122 MLA projection kernel launches per forward — latent projection overhead dominates attention time.', source: 'Roofline' },
              { sev: 'med', text: 'RMSNorm unfused: each instance decomposes into 8 separate kernels (cast→pow→mean→add→rsqrt→mul→cast→mul) × 123 instances = 984 kernel launches. Hidden state tensor read/written up to 8× instead of once per normalization.', source: 'Trace' },
              { sev: 'tip', text: 'SwiGLU activation unfused — silu and elementwise mul execute as 2 separate passes over intermediate tensors. Each pass reads/writes the full activation buffer, causing redundant HBM traffic per expert per layer.', source: 'Trace' },
              { sev: 'na', text: 'CUDA Graph not applicable on ROCm. ~3000+ individual kernel launches contribute to ~10% idle time from CPU dispatch overhead between launches.', source: 'Not Support' }
            ]
          },
          '128': {
            e2e: { input: '506.5', output: '458.0', ttft: '499ms', itl: '11.95ms', latency: '11,579ms', gap: '20% gap', rooflineGap: '22%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 11, ms: 54.9, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 14, ms: 69.8, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 33, ms: 164.7, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 99.8, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 49.9, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 12, ms: 59.9, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 16, ms: 1.91, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 16, ms: 1.91, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 30, ms: 3.59, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 2.15, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 1.20, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 1.20, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Expert dispatch scales poorly at conc=128 — M=131K causes tile waste in rocBLAS Cijk kernels. BW utilization only 38% peak despite larger batches. 256-expert synchronization barriers add ~50µs/layer.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on 512-dim latent vectors across 8×TP dominated by synchronization barriers. Ring-AllReduce achieves only 72% Infinity Fabric peak — significant compute-comm serialization observed.', source: 'Trace' },
              { sev: 'med', text: 'QKV absorb projections (M=131K, N=512, K=7168) at 62% peak TFLOPS. N=512 aligns cleanly with tile MT256×128×64 (4 tiles, zero waste) — remaining 38% gap from compute pipeline latency and memory access patterns.', source: 'Roofline' },
              { sev: 'tip', text: 'RMSNorm unfused across 123 instances (8 kernels each = 984 launches). Combined with unfused SwiGLU (2 kernels × 123 instances), Activ.&Norm category contributes 10% of compute with high launch overhead.', source: 'Trace' }
            ]
          }
        },
        '1K / 8K': {
          '4': {
            e2e: { input: '42.90', output: '357.6', ttft: '77.0ms', itl: '11.40ms', latency: '85,044ms', gap: '25% gap', rooflineGap: '29%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 10, ms: 7.7, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 12, ms: 9.2, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 34, ms: 26.2, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 15.4, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 8.5, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 13, ms: 10.0, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 22, ms: 2.51, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 16, ms: 1.82, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 26, ms: 2.96, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 15, ms: 1.71, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 1.25, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 1.14, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Decode-bound regime: flash_attn_fwd reads 61-layer KV-cache per step at 3800 GB/s (48% peak). Compressed KV (d_c=512) helps but 8K output length amplifies cache reads 8× vs 1K — HBM bandwidth is the primary bottleneck.', source: 'Trace' },
              { sev: 'high', text: 'Expert dispatch per decode step: BW utilization only 35% peak. FP4 dequant adds ~12µs/expert vs native FP8 — up to 256 active experts per layer × 61 layers of dequant ops per step, totaling significant overhead.', source: 'Roofline' },
              { sev: 'med', text: 'KV-cache paging at OSL=8K: page fault rate 4.2× higher than 1K. Current block size 256 causes severe fragmentation at long output sequences, increasing memory management overhead.', source: 'Trace' },
              { sev: 'tip', text: 'Unfused RoPE: each Q/K rotation decomposes into 5 kernels (neg+cat+mul+mul+add) × 2(Q,K) × 61 layers = 610 launches/step. The cat (rotate_half) kernel at 18µs/call is 3× costlier than the elementwise ops due to non-contiguous data reorganization.', source: 'Trace' }
            ]
          },
          '128': {
            e2e: { input: '429.0', output: '3,141', ttft: '455ms', itl: '13.78ms', latency: '103,128ms', gap: '23% gap', rooflineGap: '29%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 9, ms: 41.0, color: 'rgba(239,68,68,0.6)', sev: '' },
              { label: 'Attn GEMMs', pct: 13, ms: 59.2, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 32, ms: 145.6, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 22, ms: 100.1, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 12, ms: 54.6, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 12, ms: 54.6, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 20, ms: 2.76, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 2.07, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 27, ms: 3.72, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 2.34, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 1.52, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 1.38, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Expert dispatch overhead ~45µs/layer in decode. GEMM BW utilization drops to 33% peak at conc=128 due to batch fragmentation across 256 experts — effective per-expert batch too small for efficient tile utilization.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on (128K, 512) latent vectors: ring bandwidth only 68% peak. 8×TP barrier sync adds ~180µs per AllReduce — compute and communication fully serialized with no overlap.', source: 'Trace' },
              { sev: 'med', text: 'KV-cache L2 thrashing at conc=128 × OSL=8K — 128 concurrent sequences exceed L2 capacity. HBM re-read rate at 43% peak, indicating severe cache miss pressure from competing KV-cache accesses.', source: 'Roofline' },
              { sev: 'tip', text: 'RMSNorm + SwiGLU together unfused across 61 layers: 123 RMSNorm instances (8 kernels each) + 122 SwiGLU instances (2 kernels each) = 1228 kernel launches in the Activ.&Norm category alone.', source: 'Trace' }
            ]
          }
        },
        '8K / 1K': {
          '4': {
            e2e: { input: '281.6', output: '42.89', ttft: '291.5ms', itl: '8.34ms', latency: '8,029ms', gap: '27% gap', rooflineGap: '26%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 18, ms: 52.5, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 43.7, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 28, ms: 81.6, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 49.6, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 35.0, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 29.2, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 15, ms: 1.25, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.17, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 30, ms: 2.50, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 1.50, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.00, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 11, ms: 0.92, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Prefill-bound at ISL=8K — flash_attn on (4, H, 8192, d_c=512): BW only 45% peak. KV-cache write amplification at 61 layers × 8K sequence generates massive HBM traffic, saturating the memory subsystem.', source: 'Trace' },
              { sev: 'high', text: '256-expert top-8 at M=32768: BW utilization only 32% peak. FP4 dequant adds ~15µs per expert GEMM — total 3.7ms dequant overhead per layer accumulates to significant fraction across 61 layers.', source: 'Roofline' },
              { sev: 'med', text: 'QKV absorb projections at (M=32K, N=512, K=7168): 68% peak TFLOPS. M tiles cleanly but FP4 dequant in the compute path reduces effective throughput by ~8%, creating a dequant pipeline stall.', source: 'Trace' },
              { sev: 'tip', text: 'RMSNorm unfused: 123 instances × 8 kernels = 984 launches. At ISL=8K the hidden state tensor is larger (32K×7168), amplifying the redundant data movement from each unfused kernel pass.', source: 'Trace' }
            ]
          },
          '128': {
            e2e: { input: '2,255', output: '370.7', ttft: '1,086ms', itl: '10.55ms', latency: '10,810ms', gap: '24% gap', rooflineGap: '26%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 20, ms: 217.1, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 152.0, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 27, ms: 293.1, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 206.3, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 10, ms: 108.6, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 108.6, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 17, ms: 1.79, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 1.58, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 29, ms: 3.06, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 1.90, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 1.16, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 1.06, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Memory subsystem near saturation at 90% peak (7200 GB/s) — M=1M GEMM tiles are well-utilized but HBM bandwidth becomes the primary bottleneck. FP4 dequant compounds memory pressure with additional read traffic.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on (1M, 512) tensors: ring bandwidth drops to 65% peak under heavy memory pressure. Compute and communication fully serialized — no overlap observed in trace.', source: 'Trace' },
              { sev: 'med', text: 'TTFT 680ms exceeds interactive threshold (200ms). Prefill processes entire 8K×128 batch sequentially with no chunking — full sequence materialized before first token output.', source: 'Roofline' },
              { sev: 'tip', text: '~3000+ individual kernel launches observed, contributing to ~10% idle time from CPU dispatch overhead. Single-GPU parallelism (TP=8) leaves inter-launch gaps between sequential kernel submissions.', source: 'Trace' }
            ]
          }
        }
      },
      'FP8': {
        '1K / 1K': {
          '4': {
            e2e: { input: '59.11', output: '58.89', ttft: '77.0ms', itl: '8.34ms', latency: '7,834ms', gap: '17% gap', rooflineGap: '17%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 14, ms: 10.8, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 12, ms: 9.2, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 28, ms: 21.6, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 13.9, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 14, ms: 10.8, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 14, ms: 10.8, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 16, ms: 1.33, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.17, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 26, ms: 2.17, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 1.50, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 14, ms: 1.17, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 12, ms: 1.00, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Native FP8 GEMM eliminates dequant overhead — BW utilization improves to 45% peak (vs 31% at FP4). However, 256-expert top-8 dispatch still launches up to 256 expert GEMMs per layer × 61 layers per forward pass, maintaining high launch overhead.', source: 'Trace' },
              { sev: 'med', text: 'flash_attn_fwd at 58% HBM peak — improved over FP4 (53%) but still memory-bound. Compressed KV (d_c=512) keeps cache efficient, yet 61 absorb_attn + 61 output_proj = 122 MLA projection launches per forward remain the dominant overhead.', source: 'Roofline' },
              { sev: 'med', text: 'RMSNorm share rises to 14% of compute at FP8 — compute ops speed up with native tensor cores but normalization stays unfused. 984 kernel launches (123 instances × 8 kernels) become proportionally more significant.', source: 'Trace' },
              { sev: 'tip', text: 'GEMMs achieve 72% peak TFLOPS with native FP8 tensor core utilization — no dequant pipeline stalls. Gap vs B200 narrows from 25% (FP4) to 20%, confirming dequant overhead as a key FP4 penalty.', source: 'Roofline' }
            ]
          },
          '128': {
            e2e: { input: '591.1', output: '538.3', ttft: '444ms', itl: '10.55ms', latency: '10,226ms', gap: '15% gap', rooflineGap: '17%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 12, ms: 53.3, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 13, ms: 57.8, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 30, ms: 133.3, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 21, ms: 93.3, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 12, ms: 53.3, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 12, ms: 53.3, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 14, ms: 1.48, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 15, ms: 1.58, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 28, ms: 2.95, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 2.11, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.27, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 11, ms: 1.16, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Expert GEMM at M=131K tiles well but dispatch barrier adds ~50µs/layer sync cost. BW utilization 42% peak — limited by 256-expert scatter pattern causing non-coalesced memory access.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on (131K, 512): TP activation payload remains in BF16 regardless of weight precision — sync overhead dominates. 65ms spent in synchronization barriers with no compute-comm overlap observed.', source: 'Trace' },
              { sev: 'med', text: 'Absorb projections at 70% peak TFLOPS — FP8 native path avoids dequant penalty. Tile MT256×128×64 aligns well with N=512 but 70% utilization indicates remaining inefficiency in the compute pipeline.', source: 'Roofline' },
              { sev: 'tip', text: 'Idle time ~9% from kernel launches. ~2500+ individual kernel submissions observed — CPU dispatch overhead between kernels leaves GPU idle during inter-launch gaps.', source: 'Trace' }
            ]
          }
        },
        '1K / 8K': {
          '4': {
            e2e: { input: '50.07', output: '418.7', ttft: '67.1ms', itl: '10.00ms', latency: '74,599ms', gap: '21% gap', rooflineGap: '23%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 11, ms: 7.4, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 11, ms: 7.4, color: 'rgba(59,130,246,0.6)', sev: '' },
              { label: 'MoE (Fused SGLang)', pct: 32, ms: 21.5, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 13.4, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 13, ms: 8.7, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 13, ms: 8.7, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 20, ms: 2.00, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.40, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 27, ms: 2.70, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 1.70, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.20, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 1.00, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Decode-bound: flash_attn reads 8K KV-cache per step at 50% HBM peak across 61 layers — massive read traffic even with compressed KV (d_c=512). Memory bandwidth is the primary bottleneck limiting decode throughput.', source: 'Trace' },
              { sev: 'high', text: 'FP8 expert GEMMs at 68% peak TFLOPS. Each decode token routes to 8 experts — at BS=4 up to 256 active experts/layer × 61 layers per step. Dispatch overhead ~35µs/layer limits throughput due to frequent small-kernel launches.', source: 'Roofline' },
              { sev: 'med', text: 'Unfused RoPE: each Q/K rotation decomposes into 5 kernels (neg+cat+mul+mul+add) × 2(Q,K) × 61 layers = 610 launches/step. The cat (rotate_half) kernel at 22µs/call performs non-contiguous data reorganization, 3× costlier than the elementwise ops.', source: 'Trace' },
              { sev: 'tip', text: 'FP8 narrows gap from 25% (FP4) to 21% — confirming dequant overhead as significant FP4 penalty. Paged attention KV-cache currently occupies majority of HBM at 8K output length.', source: 'Roofline' }
            ]
          },
          '128': {
            e2e: { input: '500.7', output: '3,665', ttft: '403ms', itl: '12.17ms', latency: '91,080ms', gap: '19% gap', rooflineGap: '23%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 10, ms: 40.3, color: 'rgba(239,68,68,0.6)', sev: '' },
              { label: 'Attn GEMMs', pct: 12, ms: 48.3, color: 'rgba(59,130,246,0.6)', sev: '' },
              { label: 'MoE (Fused SGLang)', pct: 31, ms: 124.8, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 22, ms: 88.6, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 13, ms: 52.3, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 12, ms: 48.3, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 18, ms: 2.19, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.70, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 28, ms: 3.41, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 2.31, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 1.34, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 1.22, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Expert GEMMs at M=131K achieve 94% tile utilization but only 66% peak TFLOPS — limited by 256-expert dispatch synchronization at batch=128. Dispatch barriers serialize expert execution despite good tile fill.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce latency scales superlinearly above conc=64. TP activation payload remains in BF16 (weight precision does not affect AllReduce size) and sync barriers dominate at 8×TP — AllReduce time grows disproportionately with concurrency, indicating synchronization-bound scaling.', source: 'Trace' },
              { sev: 'med', text: '128 concurrent KV-caches × 61 layers cause severe L2 thrashing — effective BW drops to 40% peak. Competing cache line evictions from parallel sequence attention cause repeated HBM re-reads.', source: 'Roofline' },
              { sev: 'tip', text: 'Unfused RMSNorm (123 instances × 8 kernels = 984 launches) and SwiGLU (122 instances × 2 kernels) contribute disproportionate launch overhead at high concurrency, as kernel dispatch cost remains constant while per-kernel work shrinks.', source: 'Trace' }
            ]
          }
        },
        '8K / 1K': {
          '4': {
            e2e: { input: '330.0', output: '50.17', ttft: '249.2ms', itl: '7.32ms', latency: '7,041ms', gap: '23% gap', rooflineGap: '20%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 16, ms: 39.9, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 34.9, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 29, ms: 72.3, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 44.9, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 29.9, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 11, ms: 27.4, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 14, ms: 1.02, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 13, ms: 0.95, color: 'rgba(59,130,246,0.6)', sev: '' },
              { label: 'MoE (Fused SGLang)', pct: 30, ms: 2.20, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 1.32, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 13, ms: 0.95, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 12, ms: 0.88, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Prefill-bound at ISL=8K — flash_attn at 53% HBM peak. Despite FP8 native GEMMs achieving 71% peak TFLOPS, the attention memory bandwidth remains the dominant bottleneck at long sequence lengths.', source: 'Trace' },
              { sev: 'med', text: 'Absorb projections at M=32K: rocBLAS achieves 69% peak. Tile MT256×128×64 leaves ~3% waste. M=32768 is already power-of-2 aligned, indicating the remaining 31% gap comes from compute pipeline inefficiency rather than tile waste.', source: 'Roofline' },
              { sev: 'med', text: 'Unfused elementwise patterns: RMSNorm (8 kernels × 123 instances = 984) + SwiGLU (2 kernels × 61 instances = 122) = ~1,106 kernel launches. Each kernel reads/writes the full hidden state tensor, causing up to 10× redundant HBM traffic.', source: 'Trace' },
              { sev: 'tip', text: '26% gap vs B200 — 4pp better than FP4, confirming dequant overhead contribution. At ISL=8K, per-GPU attention memory pressure limits batch scaling — single-GPU holds all 61 layers\' KV-cache.', source: 'Roofline' }
            ]
          },
          '128': {
            e2e: { input: '2,638', output: '429.0', ttft: '935ms', itl: '9.19ms', latency: '9,406ms', gap: '20% gap', rooflineGap: '20%' },
            prefill: [
              { label: 'MLA (Attn)', pct: 18, ms: 168.2, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 13, ms: 121.5, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 28, ms: 261.7, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 186.9, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 11, ms: 102.8, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 93.5, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'MLA (Attn)', pct: 15, ms: 1.38, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.29, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (Fused SGLang)', pct: 30, ms: 2.76, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 1.75, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.10, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 10, ms: 0.92, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'M=1M expert GEMMs at 67% peak TFLOPS despite good tile fill — memory subsystem contention limits throughput. flash_attn at 55% HBM peak confirms near-saturation of the memory subsystem at this scale.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on (1M, 512): ring-AllReduce at only 64% Infinity Fabric peak. Compute and communication fully serialized — ~100ms spent in synchronization with GPU idle during AllReduce.', source: 'Trace' },
              { sev: 'med', text: 'TTFT 585ms exceeds interactive threshold (200ms). Full 8K×128 prefill processed as single contiguous batch — no chunking observed, delaying first token output until entire prefill completes.', source: 'Roofline' },
              { sev: 'tip', text: '23% gap vs B200 — 4pp better than FP4, confirming dequant elimination as significant win. However, remaining gap is dominated by memory bandwidth saturation and communication overhead at this scale.', source: 'Trace' }
            ]
          }
        }
      }
    },
    'gpt-oss 120B': {
      'FP4': {
        '1K / 1K': {
          '4': {
            e2e: { input: '67.52', output: '66.26', ttft: '67.6ms', itl: '7.96ms', latency: '7,472ms', gap: '32% gap', rooflineGap: '19%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 22, ms: 14.9, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 20, ms: 13.5, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 33, ms: 22.3, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 13, ms: 8.8, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 7, ms: 4.7, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 5, ms: 3.4, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 20, ms: 1.59, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 16, ms: 1.27, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 30, ms: 2.39, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 1.27, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 0.80, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.64, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: '128-expert top-4: each expert GEMM launches rocBLAS Cijk with MXFP4 dequant. BW utilization only 31% peak. Dequant adds ~18µs/call — 2.3ms overhead per layer accumulates significantly across 36 layers.', source: 'Trace' },
              { sev: 'high', text: 'Sliding(128)/full alternating attention pattern: flash_attn BW only 53% peak. GQA 8:1 ratio keeps KV-cache compact (2MB/layer) but alternating sliding window introduces branch divergence in the attention kernel.', source: 'Roofline' },
              { sev: 'med', text: 'QKV projections at 66% peak TFLOPS. GQA KV projection (N=360) has poor tile fill for tile MT256×128×64 — N=360 leaves fractional tiles, wasting ~12% of compute resources on padding.', source: 'Trace' },
              { sev: 'tip', text: 'RMSNorm unfused: 73 instances × 8 kernels = 584 launches. SwiGLU unfused: 36 instances × 2 kernels = 72 launches. Combined 656 Activ.&Norm kernel launches with redundant HBM passes per instance.', source: 'Trace' },
              { sev: 'na', text: 'CUDA Graph not applicable on ROCm. ~2000+ individual kernel launches contribute to 8-10% idle time from CPU dispatch overhead between launches.', source: 'Not Support' }
            ]
          },
          '128': {
            e2e: { input: '675.2', output: '605.1', ttft: '392ms', itl: '9.52ms', latency: '9,356ms', gap: '30% gap', rooflineGap: '19%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 20, ms: 78.4, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 19, ms: 74.5, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 30, ms: 117.6, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 66.6, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 8, ms: 31.4, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 6, ms: 23.5, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 18, ms: 1.71, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 1.43, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 28, ms: 2.67, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 1.81, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.14, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.76, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'M=128K expert GEMMs: tile MT128×224×64 at only 64% peak. MXFP4 dequant amortizes better at large M but dispatch barrier still ~40µs/layer — 128-expert synchronization limits scaling despite good per-expert tile fill.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on (128K, 2880): batch payload amplified to significant size. Ring-AllReduce at 74% Infinity Fabric peak — compute and communication not overlapped, with ~45ms spent in synchronization barriers.', source: 'Trace' },
              { sev: 'med', text: 'flash_attn at 55% HBM peak, memory-bound. QKV proj at M=128K achieves 68% TFLOPS but GQA KV projection (N=360) wastes tiles — N not aligned to tile MT256×128×64 boundaries, causing fractional tile inefficiency.', source: 'Roofline' },
              { sev: 'tip', text: 'RMSNorm unfused (73 instances × 8 kernels) + SwiGLU unfused (36 instances × 2 kernels) = 656 kernel launches in Activ.&Norm. At conc=128, per-kernel work is small but launch overhead remains constant.', source: 'Trace' }
            ]
          }
        },
        '1K / 8K': {
          '4': {
            e2e: { input: '50.56', output: '538.0', ttft: '57.3ms', itl: '9.52ms', latency: '71,044ms', gap: '35% gap', rooflineGap: '26%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 21, ms: 12.0, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 17, ms: 9.7, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 30, ms: 17.2, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 9.2, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 9, ms: 5.2, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 4.0, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 28, ms: 2.67, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.33, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 24, ms: 2.28, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 1.52, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 0.95, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.76, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Decode-bound: flash_attn reads 32MB KV/layer × 36 layers per step. BW only 45% peak with severe L2 thrashing — 8K output length saturates the cache hierarchy, forcing repeated HBM re-reads.', source: 'Trace' },
              { sev: 'high', text: '512 expert GEMM calls per decode step. MXFP4 dequant adds ~18µs/call — total 9.2ms dequant overhead per step. This dequant penalty is a constant per-step tax independent of expert computation.', source: 'Roofline' },
              { sev: 'med', text: 'sliding_window=128 causes page eviction thrashing at OSL=8K. Page fault rate 4.2× higher than at 1K — current block size 256 creates fragmentation as attention window slides across long sequences.', source: 'Trace' },
              { sev: 'tip', text: 'Unfused RoPE: each Q/K rotation decomposes into 5 kernels (neg+cat+mul+mul+add) × 2(Q,K) × 36 layers = 360 launches/step. The cat (rotate_half) kernel at 28µs/call is 3× costlier than elementwise ops due to non-contiguous data reorganization.', source: 'Trace' }
            ]
          },
          '128': {
            e2e: { input: '505.6', output: '4,692', ttft: '363ms', itl: '11.65ms', latency: '87,190ms', gap: '33% gap', rooflineGap: '26%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 19, ms: 68.9, color: 'rgba(239,68,68,0.6)', sev: 'med' },
              { label: 'Attn GEMMs', pct: 16, ms: 58.1, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 28, ms: 101.6, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 72.6, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 10, ms: 36.3, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 25.4, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 25, ms: 2.91, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.63, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 25, ms: 2.91, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 1.98, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 11, ms: 1.28, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.93, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: '128 concurrent KV-caches at OSL=8K consume ~25GB HBM — L2 miss rate ~85%. BW drops to 40% peak due to severe cache thrashing at conc=128 × OSL=8K. Competing cache line evictions dominate memory access latency.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce payload 737MB per call. Ring-AllReduce at 70% Infinity Fabric peak — compute and communication fully serialized with ~40ms spent waiting on synchronization barriers.', source: 'Trace' },
              { sev: 'med', text: 'Per-expert GEMM M~4 on average (128 tokens × top-4 / 128 experts) at high concurrency — only 35% peak TFLOPS. Small-M GEMMs severely underutilize the compute units. MXFP4 dequant adds constant ~18µs overhead per expert regardless of M size.', source: 'Roofline' },
              { sev: 'tip', text: 'KV-cache occupies majority of HBM at conc=128 × OSL=8K. Current FP16 KV storage doubles memory footprint compared to lower-precision alternatives — limiting maximum concurrent sequence capacity.', source: 'Trace' }
            ]
          }
        },
        '8K / 1K': {
          '4': {
            e2e: { input: '351.6', output: '56.67', ttft: '217.3ms', itl: '6.96ms', latency: '6,704ms', gap: '34% gap', rooflineGap: '22%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 26, ms: 56.5, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 19, ms: 41.3, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 26, ms: 56.5, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 14, ms: 30.4, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 9, ms: 19.6, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 6, ms: 13.0, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 20, ms: 1.39, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 1.04, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 32, ms: 2.23, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 1.11, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 0.70, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 0.49, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Sliding window achieves 60% BW peak but full attention drops to 48% at 8K sequence length. Memory-bound: FLOPS/Byte=12, well below ridge point ~134 — arithmetic intensity too low for compute utilization.', source: 'Trace' },
              { sev: 'high', text: 'M=32K expert GEMMs at only 62% peak. MXFP4 dequant: 128 experts × 4 active × 36 layers = 18432 dequant ops per forward pass. Total dequant overhead ~4.8ms — a significant fixed cost at ISL=8K.', source: 'Roofline' },
              { sev: 'med', text: 'QKV projections at 70% peak TFLOPS. GQA KV projection (N=360) wastes tiles — N not aligned to tile MT256×128×64, causing ~8% fractional tile fill loss. Separate Q and KV projections double the kernel launch count.', source: 'Trace' },
              { sev: 'tip', text: 'Sliding window layers (18 of 36) process only a 128-token window but still launch full-sequence-length kernels. Attention computation at these layers is bounded by the window size, yet kernel launch overhead is proportional to full sequence.', source: 'Trace' }
            ]
          },
          '128': {
            e2e: { input: '2,813', output: '489.2', ttft: '808ms', itl: '8.35ms', latency: '8,116ms', gap: '31% gap', rooflineGap: '22%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 24, ms: 193.9, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 18, ms: 145.4, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 24, ms: 193.9, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 145.4, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 9, ms: 72.7, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 56.6, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 18, ms: 1.50, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.17, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (MXFP4)', pct: 30, ms: 2.51, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 1.59, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 12, ms: 1.00, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 0.59, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'flash_attn on (128, 8KV, 8K, 128): BW only 43% peak. M=1M QKV proj at 72% peak TFLOPS — attention memory bandwidth dominates at large ISL × high concurrency due to massive KV-cache read traffic.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce payload 5.5GB per call (1M×2880×2B). Ring bandwidth at 68% peak — compute and communication fully serialized with ~80ms spent in synchronization barriers per AllReduce.', source: 'Trace' },
              { sev: 'med', text: 'Expert GEMMs at M=1M: good tile fill (72% peak) but MXFP4 dequant adds 5.2ms total. Dispatch barrier: 40µs × 36 layers = 1.44ms — dequant overhead exceeds dispatch barrier by 3.6×.', source: 'Roofline' },
              { sev: 'tip', text: 'TTFT 625ms exceeds interactive threshold (200ms). Full 8K×128 prefill processed as single contiguous batch across all 36 layers before first token output — no chunking or pipelining observed.', source: 'Trace' }
            ]
          }
        }
      },
      'FP8': {
        '1K / 1K': {
          '4': {
            e2e: { input: '78.06', output: '76.61', ttft: '58.8ms', itl: '6.91ms', latency: '6,512ms', gap: '26% gap', rooflineGap: '13%→9%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 24, ms: 14.1, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 18, ms: 10.6, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 30, ms: 17.6, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 14, ms: 8.2, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 8, ms: 4.7, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 6, ms: 3.5, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 22, ms: 1.52, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 1.04, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 28, ms: 1.93, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 1.17, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 0.69, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.55, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Native FP8 eliminates MXFP4 dequant — BW utilization jumps to 48% peak (vs 31% at MXFP4). However, 128-expert top-4 dispatch still launches up to 128 expert GEMMs per layer × 36 layers per forward pass, maintaining high launch overhead.', source: 'Trace' },
              { sev: 'high', text: 'Sliding(128)/full alternating: flash_attn at 60% HBM peak. FP8 KV-cache reduces per-layer storage by 2× vs BF16 but attention kernel remains memory-bound — bandwidth, not compute, limits performance.', source: 'Roofline' },
              { sev: 'med', text: 'QKV projections at FP8: 72% peak TFLOPS — 6pp improvement vs MXFP4 from eliminated dequant pipeline stalls. Remaining 28% gap indicates non-dequant inefficiencies in tile selection or memory access patterns.', source: 'Trace' },
              { sev: 'tip', text: 'RMSNorm unfused: 73 instances × 8 kernels = 584 launches with redundant HBM passes. At FP8, compute ops speed up but normalization kernel time remains unchanged, increasing its relative share.', source: 'Trace' },
              { sev: 'na', text: 'CUDA Graph not applicable on ROCm. ~1800+ individual kernel launches contribute to 6-8% idle time from CPU dispatch overhead between launches.', source: 'Not Support' }
            ]
          },
          '128': {
            e2e: { input: '780.6', output: '699.6', ttft: '339ms', itl: '8.29ms', latency: '8,178ms', gap: '24% gap', rooflineGap: '13%→9%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 22, ms: 74.6, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 17, ms: 57.6, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 28, ms: 94.9, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 61.0, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 9, ms: 30.5, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 6, ms: 20.3, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 20, ms: 1.66, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 1.16, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 27, ms: 2.24, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 1.66, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 11, ms: 0.91, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.66, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Expert GEMMs at M=128K: 70% peak TFLOPS. FP8 saves ~3ms/layer vs MXFP4 dequant. Dispatch barrier 35µs/layer still present — expert dispatch synchronization and next-layer prefetch not overlapped.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce on (128K, 2880): TP activation payload remains in BF16 regardless of weight precision. Ring BW 76% Infinity Fabric peak — near optimal, yet compute and communication remain fully serialized.', source: 'Trace' },
              { sev: 'med', text: 'FP8 flash_attn at 63% HBM peak — best efficiency among all gpt-oss configs but still memory-bound. QKV proj at 74% peak TFLOPS from native FP8 tensor cores, confirming dequant was the primary FP4 bottleneck.', source: 'Roofline' },
              { sev: 'tip', text: 'RMSNorm + SwiGLU unfused across 36 layers: 584 RMSNorm + 72 SwiGLU = 656 Activ.&Norm kernel launches. Kernel dispatch overhead becomes proportionally significant at high concurrency.', source: 'Trace' }
            ]
          }
        },
        '1K / 8K': {
          '4': {
            e2e: { input: '58.24', output: '621.0', ttft: '49.8ms', itl: '8.27ms', latency: '61,762ms', gap: '29% gap', rooflineGap: '21%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 23, ms: 11.5, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 16, ms: 8.0, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 28, ms: 13.9, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 17, ms: 8.5, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 9, ms: 4.5, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 3.5, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 30, ms: 2.48, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 13, ms: 1.08, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 22, ms: 1.82, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 1.32, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 0.91, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.66, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Decode-bound: 32MB KV read/layer × 36 layers = 1.15GB per step. BW only 48% peak — HBM bandwidth is the primary bottleneck, not compute. Each decode step re-reads the entire KV-cache across all layers.', source: 'Trace' },
              { sev: 'high', text: 'FP8 expert GEMMs 15% better than MXFP4 but per-expert GEMM at (M=4, N=d_ff, K=2880) is severely small-M bound. Up to 128 expert GEMMs/layer × 36 layers per step — tile utilization < 5% for small-M shapes.', source: 'Roofline' },
              { sev: 'med', text: 'Unfused RoPE: each Q/K rotation decomposes into 5 kernels (neg+cat+mul+mul+add) × 2(Q,K) × 36 layers = 360 launches/step. The cat (rotate_half) at 24µs/call is 3× costlier than elementwise ops due to non-contiguous data reorganization.', source: 'Trace' },
              { sev: 'tip', text: '29% gap vs B200 — 6pp better than MXFP4, confirming dequant overhead as major FP4 penalty. Paged KV-cache at 8K output currently occupies majority of HBM, limiting concurrent batch capacity.', source: 'Roofline' }
            ]
          },
          '128': {
            e2e: { input: '582.4', output: '5,422', ttft: '316ms', itl: '10.11ms', latency: '75,687ms', gap: '27% gap', rooflineGap: '21%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 21, ms: 66.4, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 15, ms: 47.4, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 27, ms: 85.3, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 20, ms: 63.2, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 10, ms: 31.6, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 22.1, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 27, ms: 2.73, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 13, ms: 1.31, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 23, ms: 2.33, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 18, ms: 1.82, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 11, ms: 1.11, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 8, ms: 0.81, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: '128 concurrent KV-caches at OSL=8K consume ~25GB HBM — L2 miss rate ~82%. flash_attn at only 39% peak due to severe cache thrashing. Competing cache line evictions from parallel sequence attention cause repeated HBM re-reads.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce payload 737MB per call. Ring-AllReduce at 71% Infinity Fabric peak — compute and expert dispatch not overlapped with communication, leaving ~35ms of idle compute during synchronization.', source: 'Trace' },
              { sev: 'med', text: 'FP8 native GEMMs at 65% peak (vs 52% MXFP4). At conc=128 decode, each expert gets M~4 on average (128 tokens × top-4 / 128 experts) — small-M GEMM efficiency only ~40%, severely underutilizing compute units for per-token expert dispatch.', source: 'Roofline' },
              { sev: 'tip', text: '27% gap vs B200 — 6pp better than MXFP4. KV-cache at FP16 storage occupies majority of HBM at 128 concurrent × 8K output sequences, limiting further batch scaling.', source: 'Trace' }
            ]
          }
        },
        '8K / 1K': {
          '4': {
            e2e: { input: '406.6', output: '65.12', ttft: '188.5ms', itl: '6.05ms', latency: '5,839ms', gap: '28% gap', rooflineGap: '17%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 28, ms: 52.8, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 17, ms: 32.0, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 24, ms: 45.2, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 15, ms: 28.3, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 10, ms: 18.9, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 6, ms: 11.3, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 22, ms: 1.33, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 14, ms: 0.85, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 30, ms: 1.82, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 16, ms: 0.97, color: 'rgba(139,92,246,0.6)', sev: '' },
              { label: 'Activ. & Norm', pct: 11, ms: 0.67, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 0.42, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'Full-attention layers BW 53% peak, sliding layers 60% — 8K sequence length creates a bimodal performance split. Alternating attention pattern prevents uniform kernel optimization across layers.', source: 'Trace' },
              { sev: 'high', text: 'M=32K expert GEMMs at 71% peak TFLOPS — well-amortized at large M. Native FP8 eliminates 4.8ms dequant overhead vs MXFP4, but expert dispatch barrier (40µs/layer) still serializes expert execution.', source: 'Roofline' },
              { sev: 'med', text: 'QKV projections at 73% peak TFLOPS. Tile MT256×128×64 well-utilized for Q projection but GQA KV projection (N=360) still wastes ~8% of tiles due to N not aligned to tile boundaries.', source: 'Trace' },
              { sev: 'tip', text: 'Sliding window layers (18 of 36) process only 128-token windows but launch full-sequence kernels. Attention compute is bounded by window size, yet kernel overhead scales with full 8K sequence length.', source: 'Trace' }
            ]
          },
          '128': {
            e2e: { input: '3,253', output: '564.4', ttft: '701ms', itl: '7.26ms', latency: '7,029ms', gap: '25% gap', rooflineGap: '17%' },
            prefill: [
              { label: 'GQA (Attn)', pct: 26, ms: 182.3, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 16, ms: 112.2, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 23, ms: 161.2, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 133.2, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 9, ms: 63.1, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 49.1, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            decode: [
              { label: 'GQA (Attn)', pct: 20, ms: 1.45, color: 'rgba(239,68,68,0.6)', sev: 'hot' },
              { label: 'Attn GEMMs', pct: 13, ms: 0.94, color: 'rgba(59,130,246,0.6)', sev: 'med' },
              { label: 'MoE (FP8)', pct: 29, ms: 2.11, color: 'rgba(245,158,11,0.6)', sev: 'hot' },
              { label: 'Communication', pct: 19, ms: 1.38, color: 'rgba(139,92,246,0.6)', sev: 'med' },
              { label: 'Activ. & Norm', pct: 12, ms: 0.87, color: 'rgba(20,184,166,0.6)', sev: '' },
              { label: 'Memory Ops', pct: 7, ms: 0.51, color: 'rgba(107,114,128,0.6)', sev: '' }
            ],
            insights: [
              { sev: 'high', text: 'flash_attn at only 45% HBM peak. M=1M QKV proj at 74% peak TFLOPS — compute-efficient but memory-bound. Despite FP8 native tensors maximizing hardware utilization, HBM bandwidth remains the ceiling.', source: 'Trace' },
              { sev: 'high', text: 'AllReduce payload 5.5GB (1M×2880×2B). Ring-AllReduce at only 66% Infinity Fabric peak — synchronization overhead grows superlinearly at large batch × sequence sizes. Compute and communication fully serialized.', source: 'Trace' },
              { sev: 'med', text: 'Expert GEMMs at M=1M: 72% peak TFLOPS. FP8 eliminates 5.2ms dequant overhead vs MXFP4 but remaining 28% gap from tile selection and memory access patterns persists at this scale.', source: 'Roofline' },
              { sev: 'tip', text: 'TTFT 540ms exceeds interactive threshold (200ms). All 36 layers processed sequentially on single TP=8 group — no pipeline parallelism observed to distribute prefill compute across stages.', source: 'Trace' }
            ]
          }
        }
      }
    }
  };

  /* ---------- Analysis Dropdown Handlers (Searchable Combobox) ---------- */
  const anaDropdowns = document.querySelectorAll('.analysis-filter-bar .dropdown-wrap');
  anaDropdowns.forEach(wrap => {
    const trigger = wrap.querySelector('.dropdown-trigger');
    const menu = wrap.querySelector('.dropdown-menu');
    const comboInput = wrap.querySelector('.search-combo-input');
    if (!trigger || !menu) return;

    // Click on trigger or caret opens dropdown
    trigger.addEventListener('click', e => {
      // Don't toggle if clicking inside the input
      if (e.target.classList.contains('search-combo-input')) {
        // Show menu
        document.querySelectorAll('.analysis-filter-bar .dropdown-menu.open').forEach(m => {
          if (m !== menu) m.classList.remove('open');
        });
        if (!menu.classList.contains('open')) menu.classList.add('open');
        return;
      }
      e.stopPropagation();
      document.querySelectorAll('.analysis-filter-bar .dropdown-menu.open').forEach(m => {
        if (m !== menu) m.classList.remove('open');
      });
      menu.classList.toggle('open');
      if (menu.classList.contains('open') && comboInput) {
        comboInput.focus();
        comboInput.select();
      }
    });

    // Combobox search filtering
    if (comboInput) {
      comboInput.addEventListener('input', () => {
        const query = comboInput.value.toLowerCase();
        let hasMatch = false;
        menu.querySelectorAll('.dd-item').forEach(item => {
          const text = (item.dataset.val || item.textContent).toLowerCase();
          const show = text.includes(query);
          item.style.display = show ? '' : 'none';
          if (show) hasMatch = true;
        });
        // Show "no match" or allow custom input
        let noMatchEl = menu.querySelector('.dd-no-match');
        if (!hasMatch && query.length > 0) {
          if (!noMatchEl) {
            noMatchEl = document.createElement('div');
            noMatchEl.className = 'dd-item dd-no-match';
            menu.appendChild(noMatchEl);
          }
          noMatchEl.textContent = `Use "${comboInput.value}" (custom)`;
          noMatchEl.dataset.val = comboInput.value;
          noMatchEl.style.display = '';
        } else if (noMatchEl) {
          noMatchEl.style.display = 'none';
        }
        if (!menu.classList.contains('open')) menu.classList.add('open');
      });

      comboInput.addEventListener('click', e => {
        e.stopPropagation();
        if (!menu.classList.contains('open')) menu.classList.add('open');
      });

      // Allow Enter to confirm custom value
      comboInput.addEventListener('keydown', e => {
        if (e.key === 'Enter') {
          menu.classList.remove('open');
        }
      });
    }

    // Item selection — only updates the UI selection, data refreshes on Start Analysis
    menu.querySelectorAll('.dd-item').forEach(item => {
      item.addEventListener('click', () => {
        menu.querySelectorAll('.dd-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        if (comboInput) {
          comboInput.value = item.dataset.val;
        } else {
          const ddValue = wrap.querySelector('.dd-value');
          if (ddValue) ddValue.textContent = item.dataset.val;
        }
        menu.classList.remove('open');
        // Update precision dropdown options when model changes
        const sel = getAnalysisSelections();
        updateAnalysisPrecision(sel.model);
      });
    });
  });
  document.addEventListener('click', () => {
    document.querySelectorAll('.analysis-filter-bar .dropdown-menu.open').forEach(m => m.classList.remove('open'));
  });

  /* ---------- Read current analysis selections ---------- */
  function getAnalysisSelections() {
    const v = id => {
      // Try combobox input first, then dd-value span
      const input = document.querySelector(`#${id} .search-combo-input`);
      if (input) return input.value.trim();
      const el = document.querySelector(`#${id} .dd-value`);
      return el ? el.textContent.trim() : '';
    };
    return {
      model: v('ana-dd-model'),
      framework: v('ana-dd-framework'),
      gpu: v('ana-dd-gpu'),
      precision: v('ana-dd-precision'),
      isl_osl: v('ana-dd-isl-osl'),
      conc: v('ana-dd-conc'),
      trace: v('ana-dd-trace')
    };
  }

  /* ---------- Update precision options based on model ---------- */
  function updateAnalysisPrecision(model) {
    const menu = document.getElementById('ana-precision-menu');
    if (!menu) return;
    // Both models support FP4 and FP8
      menu.innerHTML = '<div class="dd-item active" data-val="FP4">● FP4</div><div class="dd-item" data-val="FP8">■ FP8</div>';
    // re-bind clicks — only updates UI, data refreshes on Start Analysis
    menu.querySelectorAll('.dd-item').forEach(item => {
      item.addEventListener('click', () => {
        menu.querySelectorAll('.dd-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        document.querySelector('#ana-dd-precision .dd-value').textContent = item.dataset.val;
        menu.classList.remove('open');
      });
    });
  }

  /* ---------- Render Step Rows (sorted by pct descending) ---------- */
  function renderStepRows(steps) {
    const body = document.getElementById('ana-step-body');
    if (!body) return;
    const sorted = [...steps].sort((a, b) => b.pct - a.pct);
    body.innerHTML = sorted.map(s => {
      return `<div class="step-row">
        <span class="step-label">${s.label}</span>
        <div class="step-bar-wrap">
          <div class="step-bar-bg">
            <div class="step-bar-fill" style="width:${s.pct}%; background:${s.color};"></div>
          </div>
          <span class="step-value">${s.pct}% · ${s.ms}ms <span class="avg-times-badge">avg</span></span>
        </div>
      </div>`;
    }).join('');
  }

  /* ---------- Render Insights ---------- */
  function renderInsights(insights) {
    const body = document.getElementById('ana-insights-body');
    if (!body) return;
    const subEl = document.getElementById('ana-insights-subtitle');
    if (subEl) subEl.textContent = `Key Findings · ${insights.length} insights`;
    body.innerHTML = insights.map(ins => {
      const sevClass = ins.sev;
      const sevLabel = ins.sev === 'high' ? 'HIGH' : ins.sev === 'med' ? 'MED' : ins.sev === 'tip' ? 'TIP' : 'N/A';
      const sourceTag = ins.source
        ? `<span class="insight-source ${sevClass}">${ins.source}</span>`
        : '';
      return `<div class="insight-row ${sevClass}">
        <span class="insight-severity ${sevClass}">${sevLabel}</span>
        <span class="insight-text">${ins.text}</span>
        ${sourceTag}
      </div>`;
    }).join('');
  }

  /* ---------- Track whether analysis has been started ---------- */
  let analysisStarted = false;

  /* ---------- Main Update Function ---------- */
  function onAnalysisSelectionChange() {
    const sel = getAnalysisSelections();
    // update precision options when model changes
    updateAnalysisPrecision(sel.model);

    // lookup data
    const modelData = ANALYSIS_DATA[sel.model];
    if (!modelData) return;
    const precData = modelData[sel.precision];
    if (!precData) {
      // fallback to first available precision
      const firstPrec = Object.keys(modelData)[0];
      document.querySelector('#ana-dd-precision .dd-value').textContent = firstPrec;
      sel.precision = firstPrec;
    }
    const islData = (modelData[sel.precision] || {})[sel.isl_osl];
    if (!islData) return;
    const concData = islData[sel.conc];
    if (!concData) return;

    // Update E2E subtitle
    const subtitleEl = document.getElementById('ana-e2e-subtitle');
    const MODEL_TP = { 'DeepSeek R1 0528': 8, 'gpt-oss 120B': 8 };
    const tp = MODEL_TP[sel.model] || 8;
    if (subtitleEl) subtitleEl.textContent = `${sel.model} · ${sel.framework} · ${sel.gpu} · ${sel.precision} · ${sel.isl_osl.replace(/ /g,'')} · conc=${sel.conc} · TP=${tp}`;

    // Update E2E metrics — restore color classes and set values
    const e = concData.e2e;
    const setM = (id, v, colorClass) => {
      const el = document.getElementById(id);
      if (el) {
        el.textContent = v;
        el.classList.remove('muted');
        if (colorClass) el.className = `metric-value ${colorClass}`;
      }
    };
    setM('ana-m-input', e.input + ' tok/s/GPU', 'blue');
    setM('ana-m-output', e.output + ' tok/s/GPU', 'blue');
    setM('ana-m-ttft', e.ttft, 'yellow');
    setM('ana-m-itl', e.itl, 'yellow');
    setM('ana-m-latency', e.latency, 'red');
    setM('ana-m-gap', e.gap, 'red');
    setM('ana-m-roofline-gap', e.rooflineGap || '—', 'yellow');

    // Update Model Profiling subtitle
    const stepSubEl = document.getElementById('ana-step-subtitle');
    if (stepSubEl) stepSubEl.textContent = `Phase Breakdown: Prefill → Decode · ${sel.model} · ${sel.precision}`;

    // Update Step-by-Step (use active phase tab)
    const activePhase = document.querySelector('#ana-phase-tabs .phase-tab.active');
    const phase = activePhase ? activePhase.dataset.phase : 'prefill';
    renderStepRows(concData[phase] || concData.prefill);

    // Update Insights
    const insightsSubEl = document.getElementById('ana-insights-subtitle');
    if (insightsSubEl && concData.insights) insightsSubEl.textContent = `Key Findings · ${concData.insights.length} insights`;
    renderInsights(concData.insights || []);
  }

  /* ---------- Phase tab clicks (only work after Start) ---------- */
  document.querySelectorAll('#ana-phase-tabs .phase-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      if (!analysisStarted) return;
      document.querySelectorAll('#ana-phase-tabs .phase-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      onAnalysisSelectionChange();
    });
  });

  /* ---------- Start Analysis button ---------- */
  const btnAnalysis = document.getElementById('btn-start-analysis');
  if (btnAnalysis) {
    btnAnalysis.addEventListener('click', () => {
      analysisStarted = true;
      onAnalysisSelectionChange();
      // visual feedback
      btnAnalysis.textContent = '✅ Analysis Complete';
      btnAnalysis.classList.add('btn-success');
      setTimeout(() => {
        btnAnalysis.textContent = '🔬 Start Analysis';
        btnAnalysis.classList.remove('btn-success');
      }, 2000);
    });
  }

})();
