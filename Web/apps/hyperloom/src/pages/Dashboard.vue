<template>
  <div class="legacy-dashboard-wrapper">
    <div id="page-overview" class="page active">

      <!-- Filter Bar: All in one row -->
      <div class="filter-bar single-row">
        <div class="filter-group dropdown-wrap" id="dd-model">
          <label class="filter-label">Model <span class="info-icon">ⓘ</span></label>
          <div class="filter-select dropdown-trigger">
            <span class="dd-value">gpt-oss 120B</span>
            <span class="caret">▾</span>
          </div>
          <div class="dropdown-menu">
            <div class="dd-item active" data-val="gpt-oss 120B">gpt-oss 120B <span class="dd-sub">MoE · 120B</span></div>
            <div class="dd-item" data-val="DeepSeek R1 0528">DeepSeek R1 0528 <span class="dd-sub">MoE · 671B</span></div>
          </div>
        </div>
        <div class="filter-group dropdown-wrap" id="dd-isl-osl">
          <label class="filter-label">ISL / OSL <span class="info-icon">ⓘ</span></label>
          <div class="filter-select dropdown-trigger">
            <span class="dd-value">1K / 1K</span>
            <span class="caret">▾</span>
          </div>
          <div class="dropdown-menu">
            <div class="dd-item active" data-val="1K / 1K">1K / 1K</div>
            <div class="dd-item" data-val="1K / 8K">1K / 8K</div>
            <div class="dd-item" data-val="8K / 1K">8K / 1K</div>
          </div>
        </div>
        <div class="filter-group dropdown-wrap" id="dd-precision">
          <label class="filter-label">Precision <span class="info-icon">ⓘ</span></label>
          <div class="filter-select dropdown-trigger">
            <span class="precision-chip active dd-value">FP4</span>
            <span class="caret">▾</span>
          </div>
          <div class="dropdown-menu">
            <div class="dd-item active" data-val="FP4">● FP4</div>
            <div class="dd-item" data-val="FP8">■ FP8</div>
          </div>
        </div>
        <div class="filter-group dropdown-wrap" id="dd-yaxis">
          <label class="filter-label">Y-Axis Metric <span class="info-icon">ⓘ</span></label>
          <div class="filter-select wide dropdown-trigger">
            <span class="dd-value">Token Throughput per GPU</span>
            <span class="caret">▾</span>
          </div>
          <div class="dropdown-menu dropdown-menu-wide">
            <div class="dd-group-label">Throughput</div>
            <div class="dd-item active" data-val="Token Throughput per GPU">Token Throughput per GPU <span class="dd-check">✓</span></div>
            <div class="dd-item" data-val="Input Token Throughput per GPU">Input Token Throughput per GPU</div>
            <div class="dd-item" data-val="Output Token Throughput per GPU">Output Token Throughput per GPU</div>
            <div class="dd-item" data-val="Token Throughput per All in Utility MW">Token Throughput per All in Utility MW</div>
            <div class="dd-group-label">Cost per Million Total Tokens</div>
            <div class="dd-item" data-val="Cost/M Total (Owning – Hyperscaler)">Cost per Million Total Tokens (Owning – Hyperscaler)</div>
            <div class="dd-item" data-val="Cost/M Total (Owning – Neocloud Giant)">Cost per Million Total Tokens (Owning – Neocloud Giant)</div>
            <div class="dd-item" data-val="Cost/M Total (3 Year Rental)">Cost per Million Total Tokens (3 Year Rental)</div>
            <div class="dd-group-label">Cost per Million Output Tokens</div>
            <div class="dd-item" data-val="Cost/M Output (Owning – Hyperscaler)">Cost per Million Output Tokens (Owning – Hyperscaler)</div>
            <div class="dd-item" data-val="Cost/M Output (Owning – Neocloud Giant)">Cost per Million Output Tokens (Owning – Neocloud Giant)</div>
            <div class="dd-item" data-val="Cost/M Output (3 Year Rental)">Cost per Million Output Tokens (3 Year Rental)</div>
            <div class="dd-group-label">Cost per Million Input Tokens</div>
            <div class="dd-item" data-val="Cost/M Input (Owning – Hyperscaler)">Cost per Million Input Tokens (Owning – Hyperscaler)</div>
            <div class="dd-item" data-val="Cost/M Input (Owning – Neocloud Giant)">Cost per Million Input Tokens (Owning – Neocloud Giant)</div>
            <div class="dd-item" data-val="Cost/M Input (3 Year Rental)">Cost per Million Input Tokens (3 Year Rental)</div>
            <div class="dd-group-label">All-in Provisioned Energy per Token</div>
            <div class="dd-item" data-val="All-in Provisioned Joules per Total Token">All-in Provisioned Joules per Total Token</div>
            <div class="dd-item" data-val="All-in Provisioned Joules per Output Token">All-in Provisioned Joules per Output Token</div>
          </div>
        </div>
        <div class="gpu-config-group">
          <label class="filter-label">AMD GPU Config <span class="info-icon">ⓘ</span></label>
          <div class="gpu-config-chips">
            <span class="gpu-chip amd" id="gpu-chip-amd" data-hw="MI355X" data-fw="SGLang">
              <span class="gpu-chip-text">MI355X (SGLang)</span>
              <span class="gpu-chip-close">✕</span>
            </span>
            <div class="gpu-chip-add dropdown-wrap" id="dd-amd-gpu">
              <span class="filter-add dropdown-trigger">▾</span>
              <div class="dropdown-menu">
                <div class="dd-item active" data-val="MI355X (SGLang)" data-hw="MI355X" data-fw="SGLang">MI355X (SGLang)</div>
                <div class="dd-item" data-val="MI355X (vLLM)" data-hw="MI355X" data-fw="vLLM">MI355X (vLLM)</div>
              </div>
            </div>
          </div>
        </div>
        <div class="gpu-config-group">
          <label class="filter-label">Nvidia GPU Config <span class="info-icon">ⓘ</span></label>
          <div class="gpu-config-chips">
            <span class="gpu-chip nvidia" id="gpu-chip-nvidia" data-hw="B200" data-fw="SGLang">
              <span class="gpu-chip-text">B200 (SGLang)</span>
              <span class="gpu-chip-close">✕</span>
            </span>
            <div class="gpu-chip-add dropdown-wrap" id="dd-nvidia-gpu">
              <span class="filter-add dropdown-trigger">▾</span>
              <div class="dropdown-menu">
                <div class="dd-item active" data-val="B200 (SGLang)" data-hw="B200" data-fw="SGLang">B200 (SGLang)</div>
                <div class="dd-item" data-val="B200 (vLLM)" data-hw="B200" data-fw="vLLM">B200 (vLLM)</div>
                <div class="dd-item" data-val="B200 (TRT-LLM)" data-hw="B200" data-fw="TRT_LLM">B200 (TRT-LLM)</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Model Architecture (collapsible) -->
      <div class="arch-section collapsed" id="arch-section">
        <div class="arch-header" id="arch-header">
          <span class="arch-icon">📦</span>
          <span class="arch-label">Model Architecture</span>
          <span class="arch-chip" id="arch-chip-type">MoE</span>
          <span class="arch-chip" id="arch-chip-attn">Sink/Full GQA</span>
          <span class="arch-chip" id="arch-chip-size">120B</span>
          <span class="arch-toggle" id="arch-toggle">▾</span>
        </div>
        <div class="arch-body" id="arch-body">
          <!-- Architecture Diagram — gpt-oss 120B (default) -->
          <div class="arch-diagram" id="arch-diagram">
            <div class="arch-model-title" id="arch-org">OpenAI</div>
            <div class="arch-model-sub" id="arch-params">120B total · 5B active · 131K context</div>

            <div class="arch-flow">
              <div class="arch-block primary">
                <div class="arch-block-title">Token Embedding</div>
                <div class="arch-block-sub" id="arch-embed-sub">d = 2,880 · vocab = 201,088</div>
              </div>
              <div class="arch-arrow">↓</div>
              <div class="arch-block dashed" id="arch-block-1">
                <div class="arch-block-title" id="arch-block-1-title">Sliding Attention + Sink</div>
                <div class="arch-block-sub" id="arch-block-1-sub">×18 layers · Top-4/128 MoE</div>
                <span class="arch-block-expand">＋</span>
              </div>
              <div class="arch-alternating" id="arch-alternating">↕ alternating every layer</div>
              <div class="arch-block dashed" id="arch-block-2">
                <div class="arch-block-title" id="arch-block-2-title">Causal Grouped Query Attention</div>
                <div class="arch-block-sub" id="arch-block-2-sub">×18 layers · Top-4/128 MoE</div>
                <span class="arch-block-expand">＋</span>
              </div>
              <div class="arch-arrow">↓</div>
              <div class="arch-block solid">
                <div class="arch-block-title">RMSNorm</div>
              </div>
              <div class="arch-arrow">↓</div>
              <div class="arch-block output">
                <div class="arch-block-title">Output Head (LM Head)</div>
                <div class="arch-block-sub" id="arch-output-sub">vocab = 201,088</div>
              </div>
            </div>

            <!-- Summary Table -->
            <div class="arch-summary-table" id="arch-summary-table">
              <div class="arch-sum-cell"><div class="arch-sum-label">Type</div><div class="arch-sum-value" id="as-type">MoE</div></div>
              <div class="arch-sum-cell"><div class="arch-sum-label">Layers</div><div class="arch-sum-value" id="as-layers">36</div></div>
              <div class="arch-sum-cell"><div class="arch-sum-label">Attention</div><div class="arch-sum-value" id="as-attn">Sink/Full GQA</div></div>
              <div class="arch-sum-cell"><div class="arch-sum-label">Context</div><div class="arch-sum-value" id="as-ctx">131K</div></div>
              <div class="arch-sum-cell"><div class="arch-sum-label">Experts</div><div class="arch-sum-value" id="as-experts">4/128</div></div>
            </div>

            <!-- Features -->
            <div class="arch-features-bar">
              <span class="arch-features-label">Features:</span>
              <span class="arch-feature-tag" id="af-1">Alternating Sliding/Full Attention</span>
              <span class="arch-feature-tag" id="af-2">Attention Sink Tokens</span>
              <span class="arch-feature-tag" id="af-3">YaRN RoPE (factor=32)</span>
              <span class="arch-feature-tag" id="af-4">MXFP4 Quantization</span>
            </div>
            <div class="arch-release" id="arch-release">Released by OpenAI on Jun 13, 2025</div>
          </div>
        </div>
      </div>

      <!-- Run Date Navigation -->
      <div class="run-date-bar">
        <span class="rdb-link">Latest</span>
        <span class="rdb-nav">‹</span>
        <span class="rdb-icon">📅</span>
        <span class="rdb-text"><strong>Run Date:</strong> Mar 04, 2026</span>
        <span class="rdb-nav">›</span>
        <span class="rdb-sep"></span>
        <span class="rdb-nav">‹</span>
        <span class="rdb-run-badge">Run 1/1 <span class="rdb-ext">↗</span></span>
        <span class="rdb-nav">›</span>
        <span class="rdb-sep"></span>
        <span class="rdb-link">Changelog <span class="caret">▾</span></span>
      </div>

      <!-- ① Token Throughput vs Interactivity Chart -->
      <div class="card chart-card" id="card-chart1">
        <div class="card-header">
          <h2 class="chart-title-text">Token Throughput per GPU vs. Interactivity</h2>
          <span class="card-subtitle chart-subtitle-text">gpt-oss 120B · FP4 · 1K / 1K · Source: SemiAnalysis InferenceX™ · Updated: 02/26/2026</span>
          <div class="chart-tools">
            <button class="chart-tool-btn" title="Download">⬇</button>
            <button class="chart-tool-btn chart-zoom-btn" title="Expand / Collapse" data-target="card-chart1">⤢</button>
          </div>
        </div>
        <div class="chart-body">
          <div class="chart-container" style="height: 320px;">
            <canvas id="chart-throughput-interactivity"></canvas>
          </div>
          <!-- Building placeholder (hidden by default) -->
          <div class="chart-building" style="display:none;">
            <div class="building-icon">🏗️</div>
            <div class="building-title">Building…</div>
            <div class="building-sub">This metric view is under construction. Data will be available soon.</div>
          </div>
        </div>
        <div class="chart-legend-box">
          <span class="legend-title">Curve Legend</span>
          <div class="legend-items">
            <span class="legend-item"><span class="legend-dot" style="background:#22c55e"></span><strong>B200 (vLLM)</strong></span>
            <span class="legend-item"><span class="legend-dot" style="background:#e4002b"></span><strong>MI355X (vLLM)</strong></span>
          </div>
        </div>
      </div>

      <!-- ①-b Token Throughput vs E2E Latency Chart -->
      <div class="card chart-card" id="card-chart2">
        <div class="card-header">
          <h2 class="chart-title-text">Token Throughput per GPU vs. End-to-end Latency</h2>
          <span class="card-subtitle chart-subtitle-text">gpt-oss 120B · FP4 · 1K / 1K · Source: SemiAnalysis InferenceX™ · Updated: 02/26/2026</span>
          <div class="chart-tools">
            <button class="chart-tool-btn" title="Download">⬇</button>
            <button class="chart-tool-btn chart-zoom-btn" title="Expand / Collapse" data-target="card-chart2">⤢</button>
          </div>
        </div>
        <div class="chart-body">
          <div class="chart-container" style="height: 320px;">
            <canvas id="chart-throughput-latency"></canvas>
          </div>
          <div class="chart-building" style="display:none;">
            <div class="building-icon">🏗️</div>
            <div class="building-title">Building…</div>
            <div class="building-sub">This metric view is under construction. Data will be available soon.</div>
          </div>
        </div>
        <div class="chart-legend-box">
          <span class="legend-title">Curve Legend</span>
          <div class="legend-items">
            <span class="legend-item"><span class="legend-dot" style="background:#22c55e"></span><strong>B200 (vLLM)</strong></span>
            <span class="legend-item"><span class="legend-dot" style="background:#e4002b"></span><strong>MI355X (vLLM)</strong></span>
          </div>
        </div>
      </div>

      <!-- Bottom Row: Kernel Stack + Side-by-Side -->
      <div class="card-row" id="kernel-row">
        <!-- ② Kernel Stack -->
        <div class="card half chart-card" id="card-chart3">
          <div class="card-header">
            <h2>Kernel Stack</h2>
            <span class="card-subtitle">gpt-oss 120B · FP4 (conc=c128)</span>
            <div class="chart-tools">
              <button class="chart-tool-btn chart-zoom-btn" title="Expand / Collapse" data-target="card-chart3">⤢</button>
            </div>
          </div>
          <div class="warning-banner">⚠ Kernel comparison does not include inter-kernel bubbles and latency saving from parallel kernel execution.</div>
          <div class="chart-body">
            <div class="chart-container" style="height: 240px;">
              <canvas id="chart-kernel-stack"></canvas>
            </div>
          </div>
          <!-- Kernel Legend (2 rows, inside Kernel Stack card) -->
          <div class="kernel-legend-grid">
            <div class="kernel-legend-row">
              <span class="legend-bar-item"><span class="swatch" style="background:#f97316"></span>GEMM/MOE</span>
              <span class="legend-bar-item"><span class="swatch" style="background:#3b82f6"></span>ATTN</span>
              <span class="legend-bar-item"><span class="swatch" style="background:#22c55e"></span>AR/NORM</span>
              <span class="legend-bar-item"><span class="swatch" style="background:#ef4444"></span>QUANT</span>
            </div>
            <div class="kernel-legend-row">
              <span class="legend-bar-item"><span class="swatch" style="background:#8b5cf6"></span>TOPK</span>
              <span class="legend-bar-item"><span class="swatch" style="background:#92400e"></span>ACT</span>
              <span class="legend-bar-item"><span class="swatch" style="background:#f9a8d4"></span>CACHE</span>
              <span class="legend-bar-item"><span class="swatch" style="background:#9ca3af"></span>other</span>
            </div>
          </div>
        </div>

        <!-- ③ Kernel Side-by-Side -->
        <div class="card half chart-card" id="card-chart4">
          <div class="card-header">
            <h2>Kernel Side-by-Side Comparison</h2>
            <div class="chart-tools">
              <button class="chart-tool-btn chart-zoom-btn" title="Expand / Collapse" data-target="card-chart4">⤢</button>
            </div>
          </div>
          <div class="hw-legend">
            <span class="legend-title">Status &amp; Hardware</span>
            <div class="hw-legend-items">
              <span class="hw-legend-item"><span class="hw-swatch red-light"></span>MI355X (Before)</span>
              <span class="hw-legend-item"><span class="hw-swatch cyan"></span>MI355X (Roofline)</span>
              <span class="hw-legend-item"><span class="hw-swatch red"></span>MI355X (After Opt)</span>
              <span class="hw-legend-item"><span class="hw-swatch green"></span>B200 (Reference)</span>
            </div>
          </div>
          <div class="chart-body">
            <div class="chart-container" style="height: 220px;">
              <canvas id="chart-kernel-sidebyside"></canvas>
            </div>
          </div>
        </div>
      </div>

      <!-- (Kernel Legend moved into Kernel Stack card above) -->

      <!-- Bottom Action Bar -->
      <div class="action-bar">
        <button class="btn btn-analysis" data-page="analysis">🔬 New Analysis</button>
        <button class="btn btn-optimization" data-page="optimization">⚙️ Optimization & Results</button>
        <button class="btn btn-report btn-primary" data-page="report">📄 Report</button>
      </div>

    </div><!-- end page-overview -->

    <!-- ╔══════════════════════════════════════════════════════╗ -->
    <!-- ║          PAGE 1: ANALYSIS                           ║ -->
    <!-- ╚══════════════════════════════════════════════════════╝ -->
    <div id="page-analysis" class="page">

      <!-- ① Task Setup (with searchable comboboxes) -->
      <div class="card">
        <div class="card-header">
          <h2>Task Setup</h2>
        </div>
        <div class="task-setup-grid">
          <div class="analysis-filter-bar">
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-model">
              <label class="filter-label">Model <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="DeepSeek R1 0528" placeholder="Search model…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu">
                <div class="dd-item" data-val="gpt-oss 120B">gpt-oss 120B <span class="dd-sub">MoE · 120B</span></div>
                <div class="dd-item active" data-val="DeepSeek R1 0528">DeepSeek R1 0528 <span class="dd-sub">MoE · 671B</span></div>
              </div>
            </div>
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-framework">
              <label class="filter-label">Framework <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="SGLang" placeholder="Search framework…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu">
                <div class="dd-item active" data-val="SGLang">SGLang</div>
                <div class="dd-item" data-val="vLLM">vLLM</div>
              </div>
            </div>
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-gpu">
              <label class="filter-label">GPU <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="MI355X" placeholder="Search GPU…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu">
                <div class="dd-item active" data-val="MI355X">MI355X</div>
              </div>
            </div>
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-precision">
              <label class="filter-label">Precision <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="FP4" placeholder="Search precision…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu" id="ana-precision-menu">
                <div class="dd-item active" data-val="FP4">● FP4</div>
                <div class="dd-item" data-val="FP8">■ FP8</div>
              </div>
            </div>
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-isl-osl">
              <label class="filter-label">ISL / OSL <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="1K / 1K" placeholder="Search ISL/OSL…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu">
                <div class="dd-item active" data-val="1K / 1K">1K / 1K</div>
                <div class="dd-item" data-val="1K / 8K">1K / 8K</div>
                <div class="dd-item" data-val="8K / 1K">8K / 1K</div>
              </div>
            </div>
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-conc">
              <label class="filter-label">Concurrency <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="4" placeholder="Search…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu">
                <div class="dd-item active" data-val="4">4</div>
                <div class="dd-item" data-val="128">128</div>
              </div>
            </div>
            <div class="filter-group dropdown-wrap search-combo" id="ana-dd-trace">
              <label class="filter-label">Trace <span class="info-icon">ⓘ</span></label>
              <div class="filter-select dropdown-trigger search-combo-trigger">
                <input type="text" class="search-combo-input" value="PyTorch" placeholder="Search trace…" autocomplete="off" />
                <span class="caret">▾</span>
              </div>
              <div class="dropdown-menu search-combo-menu">
                <div class="dd-item active" data-val="PyTorch">PyTorch</div>
                <div class="dd-item" data-val="ROCm">ROCm</div>
              </div>
            </div>
            <div class="filter-group form-group-action">
              <label class="filter-label">&nbsp;</label>
              <button class="btn btn-primary btn-analysis" id="btn-start-analysis">🔬 Start Analysis</button>
            </div>
          </div>
        </div>
      </div>

      <!-- ② E2E Performance Metrics (dynamic — empty until Start Analysis) -->
      <div class="card">
        <div class="card-header">
          <h2>E2E Performance Metrics</h2>
          <span class="card-subtitle" id="ana-e2e-subtitle">Select configuration above and click Start Analysis</span>
        </div>
        <div class="metrics-row" id="ana-e2e-metrics">
          <div class="metric-card blue">
            <span class="metric-label">Input Throughput/GPU <span class="avg-times-badge">×avg</span></span>
            <span class="metric-value muted" id="ana-m-input">—</span>
          </div>
          <div class="metric-card blue">
            <span class="metric-label">Output Throughput/GPU <span class="avg-times-badge">×avg</span></span>
            <span class="metric-value muted" id="ana-m-output">—</span>
          </div>
          <div class="metric-card yellow">
            <span class="metric-label">TTFT <span class="avg-times-badge">×avg</span></span>
            <span class="metric-value muted" id="ana-m-ttft">—</span>
          </div>
          <div class="metric-card yellow">
            <span class="metric-label">ITL <span class="avg-times-badge">×avg</span></span>
            <span class="metric-value muted" id="ana-m-itl">—</span>
          </div>
          <div class="metric-card red">
            <span class="metric-label">Total Latency <span class="avg-times-badge">×avg</span></span>
            <span class="metric-value muted" id="ana-m-latency">—</span>
          </div>
          <div class="metric-card red-strong">
            <span class="metric-label">Gap vs B200</span>
            <span class="metric-value muted" id="ana-m-gap">—</span>
          </div>
          <div class="metric-card yellow">
            <span class="metric-label">Gap vs Roofline</span>
            <span class="metric-value muted" id="ana-m-roofline-gap">—</span>
          </div>
        </div>
      </div>

      <!-- ③ Model Profiling (dynamic — empty until Start Analysis) -->
      <div class="card">
        <div class="card-header">
          <h2>Model Profiling</h2>
          <span class="card-subtitle" id="ana-step-subtitle">Click Start Analysis to view phase breakdown</span>
        </div>
        <div class="phase-tabs" id="ana-phase-tabs">
          <button class="phase-tab active" data-phase="prefill">Prefill</button>
          <button class="phase-tab" data-phase="decode">Decode</button>
        </div>
        <div class="step-analysis" id="ana-step-body">
          <div class="ana-empty-state" id="ana-step-empty">
            <span class="empty-icon">📊</span>
            <span class="empty-text">No profiling data. Configure parameters above and click <strong>Start Analysis</strong>.</span>
          </div>
        </div>
      </div>

      <!-- ④ Agent Insights (collapsible — empty until Start Analysis) -->
      <div class="card collapsible-section" id="agent-insights-section">
        <div class="card-header collapsible-header">
          <h2>Agent Insights</h2>
          <span class="card-subtitle" id="ana-insights-subtitle">Click Start Analysis to generate insights</span>
          <span class="section-toggle-icon">▾</span>
        </div>
        <div class="collapsible-body">
          <div class="insights-list" id="ana-insights-body">
            <div class="ana-empty-state" id="ana-insights-empty">
              <span class="empty-icon">💡</span>
              <span class="empty-text">No insights yet. Run analysis to see key findings.</span>
            </div>
          </div>
        </div>
      </div>

      <!-- ⑤ Bridge Plan -->
      <div class="card">
        <div class="card-header">
          <h2>Bridge Plan</h2>
          <span class="card-subtitle">5 actionable items identified · Ready for optimization</span>
        </div>
        <div class="bridge-plan-action">
          <button class="btn btn-primary btn-lg" data-page="optimization">⚙️ Go to Optimization & Results →</button>
        </div>
      </div>

    </div><!-- end page-analysis -->

    <!-- ╔══════════════════════════════════════════════════════╗ -->
    <!-- ║          PAGE 2: OPTIMIZATION                       ║ -->
    <!-- ╚══════════════════════════════════════════════════════╝ -->
    <div id="page-optimization" class="page">

      <!-- Bridge Plan Selector (Search Model) -->
      <div class="card compact">
        <div class="bridge-selector">
          <span class="bridge-label">Optimization Plan:</span>
          <div class="filter-group dropdown-wrap search-dropdown" id="opt-dd-model">
            <div class="filter-select dropdown-trigger">
              <span class="dd-value dd-placeholder">🔍 Search Model…</span>
              <span class="dd-version-info" id="opt-bridge-version" style="display:none;"></span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu search-dropdown-menu">
              <input type="text" class="search-dropdown-input" placeholder="Type to search model…" autocomplete="off" />
              <div class="search-dropdown-list">
                <div class="dd-item" data-val="gpt-oss 120B" data-fw="SGLang" data-hw="MI355X" data-prec="FP4">gpt-oss 120B <span class="dd-sub">MoE · 120B</span></div>
                <div class="dd-item" data-val="DeepSeek R1 0528" data-fw="SGLang" data-hw="MI355X" data-prec="FP4">DeepSeek R1 0528 <span class="dd-sub">MoE · 671B</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Optimization Plan (default empty — populated on model selection) -->
      <div class="card" id="opt-bridge-plan-card">
        <div class="card-header">
          <h2 id="opt-bridge-plan-title">Optimization Plan</h2>
          <span class="card-subtitle" id="opt-bridge-plan-sub">Select a model above to load optimization items</span>
        </div>
        <div class="bridge-plan-empty" id="bridge-plan-empty">
          <div class="empty-state">
            <span class="empty-icon">📋</span>
            <span class="empty-text">No model selected. Choose a model from the dropdown above to view the optimization plan.</span>
          </div>
        </div>
        <!-- Building state for gpt-oss -->
        <div id="bridge-plan-building" class="bridge-plan-empty" style="display:none;">
          <div class="empty-state building-state">
            <span class="empty-icon">🔨</span>
            <span class="empty-text building-text">Optimization plan for <strong>gpt-oss 120B</strong> is currently being built…</span>
            <span class="building-sub-text">Kernel profiling and strategy generation in progress. Check back soon.</span>
          </div>
        </div>

        <div id="bridge-plan-content" style="display:none;">

        <div class="bridge-table">
          <!-- Table Header -->
          <div class="bt-header">
            <span class="bt-col bt-sel"></span>
            <span class="bt-col bt-pri">PRI</span>
            <span class="bt-col bt-name">Optimization</span>
            <span class="bt-col bt-save">Est. Saving</span>
            <span class="bt-col bt-e2e">E2E Impact</span>
            <span class="bt-col bt-exp"></span>
          </div>

          <!-- P1: MoE FP4 GEMM Opt. -->
          <div class="bt-row-group expanded" data-plan="p1">
            <div class="bt-row selected">
              <span class="bt-col bt-sel"><span class="bt-check">☑</span></span>
              <span class="bt-col bt-pri"><span class="pri-badge p1">P1</span></span>
              <span class="bt-col bt-name"><strong>MoE FP4 GEMM Opt.</strong> <span class="bt-bw-info">BW 59% · 756 ops</span></span>
              <span class="bt-col bt-save"><span class="save-value">-171.5ms</span></span>
              <span class="bt-col bt-e2e"><span class="e2e-pct green-text">24.0%</span> <span class="e2e-detail">714.8→543.3ms</span></span>
              <span class="bt-col bt-exp"><span class="bt-toggle">▾</span></span>
            </div>
            <div class="bt-kernels">
              <div class="kernel-row selected">
                <span class="kernel-check">☑</span>
                <span class="kernel-name">fp4_moe_gemm_kernel</span>
                <span class="kernel-metrics">BW: 2487 GB/s · Compute: 1200 TFLOPS</span>
              </div>
              <div class="kernel-row selected">
                <span class="kernel-check">☑</span>
                <span class="kernel-name">fp4_dequant_gemm</span>
                <span class="kernel-metrics">BW: 2240 GB/s · Compute: 2100 TFLOPS</span>
              </div>
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">moe_gate_topk</span>
                <span class="kernel-metrics">BW: 3200 GB/s · Compute: 2800 TFLOPS</span>
              </div>
            </div>
          </div>

          <!-- P2: MLA Flash Attention Opt. -->
          <div class="bt-row-group expanded" data-plan="p2">
            <div class="bt-row selected">
              <span class="bt-col bt-sel"><span class="bt-check">☑</span></span>
              <span class="bt-col bt-pri"><span class="pri-badge p2">P2</span></span>
              <span class="bt-col bt-name"><strong>MLA Flash Attention Opt.</strong> <span class="bt-bw-info">BW 53% · latent projection overhead</span></span>
              <span class="bt-col bt-save"><span class="save-value">-26.0ms</span></span>
              <span class="bt-col bt-e2e"><span class="e2e-pct blue-text">3.6%</span> <span class="e2e-detail">543.3→517.3ms</span></span>
              <span class="bt-col bt-exp"><span class="bt-toggle">▾</span></span>
            </div>
            <div class="bt-kernels">
              <div class="kernel-row selected">
                <span class="kernel-check">☑</span>
                <span class="kernel-name">flash_attn_fwd_v2</span>
                <span class="kernel-metrics">BW: 2800 GB/s · Compute: 2600 TFLOPS</span>
              </div>
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">mla_absorb_proj</span>
                <span class="kernel-metrics">BW: 2480 GB/s · Compute: 2200 TFLOPS</span>
              </div>
            </div>
          </div>

          <!-- P3: MLA QKV Absorb Projection -->
          <div class="bt-row-group" data-plan="p3">
            <div class="bt-row">
              <span class="bt-col bt-sel"><span class="bt-check unchecked">☐</span></span>
              <span class="bt-col bt-pri"><span class="pri-badge p3">P3</span></span>
              <span class="bt-col bt-name">MLA QKV Absorb Projection <span class="bt-bw-info">suboptimal N=512 tiling · 244 ops</span></span>
              <span class="bt-col bt-save"><span class="save-value">-15.8ms</span></span>
              <span class="bt-col bt-e2e"><span class="e2e-pct teal-text">2.2%</span> <span class="e2e-detail">517.3→501.5ms</span></span>
              <span class="bt-col bt-exp"><span class="bt-toggle">▸</span></span>
            </div>
            <div class="bt-kernels">
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">absorb_attn_qkv</span>
                <span class="kernel-metrics">BW: 2240 GB/s · Compute: 1960 TFLOPS</span>
              </div>
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">absorb_output_proj</span>
                <span class="kernel-metrics">BW: 4100 GB/s · Compute: 2100 TFLOPS</span>
              </div>
            </div>
          </div>

          <!-- P4: SwiGLU + Elementwise Fusion -->
          <div class="bt-row-group" data-plan="p4">
            <div class="bt-row">
              <span class="bt-col bt-sel"><span class="bt-check unchecked">☐</span></span>
              <span class="bt-col bt-pri"><span class="pri-badge p4">P4</span></span>
              <span class="bt-col bt-name">SwiGLU + Elementwise Fusion <span class="bt-bw-info">unfused silu + mul · 122 ops</span></span>
              <span class="bt-col bt-save"><span class="save-value">-10.2ms</span></span>
              <span class="bt-col bt-e2e"><span class="e2e-pct">1.4%</span> <span class="e2e-detail">501.5→491.3ms</span></span>
              <span class="bt-col bt-exp"><span class="bt-toggle">▸</span></span>
            </div>
            <div class="bt-kernels">
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">silu_activation</span>
                <span class="kernel-metrics">BW: 3840 GB/s · Compute: 180 TFLOPS</span>
              </div>
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">elementwise_mul</span>
                <span class="kernel-metrics">BW: 4480 GB/s · Compute: 160 TFLOPS</span>
              </div>
            </div>
          </div>

          <!-- P5: RMSNorm Triton Fusion -->
          <div class="bt-row-group" data-plan="p5">
            <div class="bt-row">
              <span class="bt-col bt-sel"><span class="bt-check unchecked">☐</span></span>
              <span class="bt-col bt-pri"><span class="pri-badge p5">P5</span></span>
              <span class="bt-col bt-name">RMSNorm Triton Fusion <span class="bt-bw-info">8-kernel decomposition · 984 ops</span></span>
              <span class="bt-col bt-save"><span class="save-value">-4.7ms</span></span>
              <span class="bt-col bt-e2e"><span class="e2e-pct">0.7%</span> <span class="e2e-detail">491.3→486.6ms</span></span>
              <span class="bt-col bt-exp"><span class="bt-toggle">▸</span></span>
            </div>
            <div class="bt-kernels">
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">rms_norm_triton</span>
                <span class="kernel-metrics">BW: 3840 GB/s · Compute: 312 TFLOPS</span>
              </div>
              <div class="kernel-row">
                <span class="kernel-check unchecked">☐</span>
                <span class="kernel-name muted">residual_add_kernel</span>
                <span class="kernel-metrics">BW: 3600 GB/s · Compute: 280 TFLOPS</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Start GEAK Agent -->
        <button class="btn btn-primary btn-full">🚀 Start GEAK Agent</button>
        </div><!-- end bridge-plan-content -->
      </div>

      <!-- GEAK Strategies (merged with Jarvis ↔ GEAK Status) — hidden until Start GEAK Agent -->
      <div class="card collapsible-section opt-hidden geak-agent-card" id="geak-strategies-merged">
        <div class="card-header collapsible-header" id="geak-strategies-toggle">
          <h2>GEAK Strategies</h2>
          <span class="card-subtitle">3 kernels · 1 validated · 1 processing · 1 sent</span>
          <span class="exec-flow-tooltip-wrap" title="Execution Flow: a) Click kernel in Strategies panel → b) Set configuration (LLM API, model, steps, parallelism) → c) Apply Optimization Strategies for kernel. Strategies → Configure → Apply">
            <span class="exec-flow-info-icon">⚠</span>
          </span>
          <span class="section-toggle-icon">▾</span>
        </div>

        <!-- Status + inline strategy details -->
        <div class="collapsible-body" id="geak-merged-status-body">
          <div class="geak-status-list">
            <!-- Item 1: Validated -->
            <div class="geak-expand-item expanded validated">
              <div class="geak-expand-header">
                <span class="geak-expand-toggle">▾</span>
                <span class="geak-item-name">fp4_moe_gemm_kernel</span>
                <span class="geak-item-sub">MoE GEMM Opt. · 2m 34s</span>
                <span class="geak-status-label validated">Validated ✅</span>
              </div>
              <div class="geak-expand-body">
                <div class="pipeline-steps">
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Ready</span><span class="pipe-step-time">00:00</span></div></div>
                  <div class="pipe-step-connector done"></div>
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Sent</span><span class="pipe-step-time">00:12</span></div></div>
                  <div class="pipe-step-connector done"></div>
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Processing</span><span class="pipe-step-time">01:45</span></div></div>
                  <div class="pipe-step-connector done"></div>
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Validated</span><span class="pipe-step-time">02:34</span></div></div>
                  <div class="pipe-step-connector pending"></div>
                  <div class="pipe-step pending"><span class="pipe-step-icon pending">○</span><div class="pipe-step-info"><span class="pipe-step-name">Merged</span><span class="pipe-step-time">—</span></div></div>
                </div>
                <div class="geak-expand-detail">
                  <span class="detail-chip">BW: 31% (pre-opt)</span>
                  <span class="detail-chip">Est. -171.5ms</span>
                  <span class="detail-chip green-chip">Speedup: 2.4×</span>
                  <span class="detail-chip e2e-chip">E2E: 24.0% ▲</span>
                </div>
                <!-- Inline strategy expand -->
                <div class="inline-strategy-section">
                  <div class="inline-strategy-toggle-btn" data-target="inline-strat-1">
                    <span class="inline-strategy-toggle-icon">▸</span>
                    <span>Strategy Details</span>
                    <span class="strategy-est-badge">Est. 2.4×</span>
                  </div>
                  <div class="inline-strategy-body" id="inline-strat-1" style="display:none;">
                    <div class="strategy-desc">Optimize BW utilization 31% → 75% · Memory layout + tiling</div>
                    <div class="strategy-actions">
                      <div class="strat-action-row">
                        <span class="strat-action-label">Strategy:</span>
                        <span class="strat-action-value">Fuse memory layout + loop tiling + vectorized load</span>
                      </div>
                      <div class="strat-action-row">
                        <span class="strat-action-label">Target:</span>
                        <span class="strat-action-value">BW util 31% → 75%, reduce memory round-trips by 60%</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <!-- Item 2: Processing -->
            <div class="geak-expand-item expanded processing">
              <div class="geak-expand-header">
                <span class="geak-expand-toggle">▾</span>
                <span class="geak-item-name">fp4_dequant_gemm</span>
                <span class="geak-item-sub">MoE GEMM Opt. · 1m 12s</span>
                <span class="geak-status-label processing">Processing 🔄</span>
              </div>
              <div class="geak-expand-body">
                <div class="pipeline-steps">
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Ready</span><span class="pipe-step-time">00:00</span></div></div>
                  <div class="pipe-step-connector done"></div>
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Sent</span><span class="pipe-step-time">00:08</span></div></div>
                  <div class="pipe-step-connector active"></div>
                  <div class="pipe-step active"><span class="pipe-step-icon active pulsing">⟳</span><div class="pipe-step-info"><span class="pipe-step-name">Processing</span><span class="pipe-step-time">01:12…</span></div></div>
                  <div class="pipe-step-connector pending"></div>
                  <div class="pipe-step pending"><span class="pipe-step-icon pending">○</span><div class="pipe-step-info"><span class="pipe-step-name">Validated</span><span class="pipe-step-time">—</span></div></div>
                  <div class="pipe-step-connector pending"></div>
                  <div class="pipe-step pending"><span class="pipe-step-icon pending">○</span><div class="pipe-step-info"><span class="pipe-step-name">Merged</span><span class="pipe-step-time">—</span></div></div>
                </div>
                <div class="geak-expand-detail">
                  <span class="detail-chip">Fuse dequant + GEMM</span>
                  <span class="detail-chip">Est. -26.0ms</span>
                  <span class="detail-chip blue-chip">Iter 3/8</span>
                </div>
                <!-- Inline strategy expand -->
                <div class="inline-strategy-section">
                  <div class="inline-strategy-toggle-btn" data-target="inline-strat-2">
                    <span class="inline-strategy-toggle-icon">▸</span>
                    <span>Strategy Details</span>
                    <span class="strategy-est-badge">Est. 1.8×</span>
                  </div>
                  <div class="inline-strategy-body" id="inline-strat-2" style="display:none;">
                    <div class="strategy-desc">Fuse dequantize + GEMM · Reduce memory round-trips</div>
                    <div class="strategy-actions">
                      <div class="strat-action-row">
                        <span class="strat-action-label">Strategy:</span>
                        <span class="strat-action-value">Kernel fusion (dequant → GEMM) + shared memory staging</span>
                      </div>
                      <div class="strat-action-row">
                        <span class="strat-action-label">Target:</span>
                        <span class="strat-action-value">Eliminate intermediate buffer write, reduce BW 40%</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <!-- Item 3: Sent -->
            <div class="geak-expand-item sent">
              <div class="geak-expand-header">
                <span class="geak-expand-toggle">▸</span>
                <span class="geak-item-name">flash_attn_fwd_v2</span>
                <span class="geak-item-sub">MLA Attn Opt. · 0m 45s</span>
                <span class="geak-status-label sent">Sent →</span>
              </div>
              <div class="geak-expand-body">
                <div class="pipeline-steps">
                  <div class="pipe-step done"><span class="pipe-step-icon done">✓</span><div class="pipe-step-info"><span class="pipe-step-name">Ready</span><span class="pipe-step-time">00:00</span></div></div>
                  <div class="pipe-step-connector active"></div>
                  <div class="pipe-step active"><span class="pipe-step-icon active">⟳</span><div class="pipe-step-info"><span class="pipe-step-name">Sent</span><span class="pipe-step-time">00:45…</span></div></div>
                  <div class="pipe-step-connector pending"></div>
                  <div class="pipe-step pending"><span class="pipe-step-icon pending">○</span><div class="pipe-step-info"><span class="pipe-step-name">Processing</span><span class="pipe-step-time">—</span></div></div>
                  <div class="pipe-step-connector pending"></div>
                  <div class="pipe-step pending"><span class="pipe-step-icon pending">○</span><div class="pipe-step-info"><span class="pipe-step-name">Validated</span><span class="pipe-step-time">—</span></div></div>
                  <div class="pipe-step-connector pending"></div>
                  <div class="pipe-step pending"><span class="pipe-step-icon pending">○</span><div class="pipe-step-info"><span class="pipe-step-name">Merged</span><span class="pipe-step-time">—</span></div></div>
                </div>
                <div class="geak-expand-detail">
                  <span class="detail-chip">MLA latent proj + flash attn</span>
                  <span class="detail-chip">Est. -26.0ms</span>
                </div>
                <!-- Inline strategy expand -->
                <div class="inline-strategy-section">
                  <div class="inline-strategy-toggle-btn" data-target="inline-strat-3">
                    <span class="inline-strategy-toggle-icon">▸</span>
                    <span>Strategy Details</span>
                    <span class="strategy-est-badge">Est. 1.35×</span>
                  </div>
                  <div class="inline-strategy-body" id="inline-strat-3" style="display:none;">
                    <div class="strategy-desc">Optimize MLA compressed KV path · Reduce latent projection overhead</div>
                    <div class="strategy-actions">
                      <div class="strat-action-row">
                        <span class="strat-action-label">Strategy:</span>
                        <span class="strat-action-value">Fuse absorb_attn + flash_attn_fwd · Swizzled KV layout for d_c=512</span>
                      </div>
                      <div class="strat-action-row">
                        <span class="strat-action-label">Target:</span>
                        <span class="strat-action-value">BW util 53% → 72%, eliminate 122 absorb_proj kernel launches</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Total saving summary -->
          <div class="total-saving">
            <span>Est. Total Saving: -228.2ms (31.9%)</span>
            <span class="saving-detail">714.8 → 486.6ms</span>
          </div>
        </div>
      </div>

      <!-- Kernel Details (hidden until Start GEAK Agent) -->
      <div class="card opt-hidden geak-agent-card" id="opt-kernel-detail-card">
        <div class="card-header">
          <h2>Kernel Details</h2>
          <div class="filter-group dropdown-wrap" id="opt-dd-kernel">
            <div class="filter-select dropdown-trigger">
              <span class="dd-value">fp4_moe_gemm_kernel</span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu" id="opt-kernel-menu">
              <div class="dd-item active" data-val="fp4_moe_gemm_kernel">fp4_moe_gemm_kernel</div>
              <div class="dd-item" data-val="fp4_dequant_gemm">fp4_dequant_gemm</div>
              <div class="dd-item" data-val="flash_attn_fwd_v2">flash_attn_fwd_v2</div>
              <div class="dd-item" data-val="mla_absorb_proj">mla_absorb_proj</div>
              <div class="dd-item" data-val="flash_attn_fwd_v2">flash_attn_fwd_v2</div>
            </div>
          </div>
        </div>

        <!-- Pre-optimization Metrics -->
        <div class="detail-section-label">Pre-optimization (Baseline)</div>
        <div class="metrics-row compact">
          <div class="metric-card blue">
            <span class="metric-label">Bandwidth</span>
            <span class="metric-value blue">2487 GB/s</span>
          </div>
          <div class="metric-card yellow">
            <span class="metric-label">Compute</span>
            <span class="metric-value yellow">1200 TFLOPS</span>
          </div>
          <div class="metric-card orange">
            <span class="metric-label">BW Util</span>
            <span class="metric-value orange">31.0%</span>
          </div>
          <div class="metric-card green-card">
            <span class="metric-label">Compute Util</span>
            <span class="metric-value red">12.0%</span>
          </div>
          <div class="metric-card red">
            <span class="metric-label">Bottleneck</span>
            <span class="metric-value red">🔴 Mem Bound</span>
          </div>
        </div>

        <!-- Post-optimization Metrics -->
        <div class="detail-section-label green-label">Post-optimization (GEAK Result)</div>
        <div class="metrics-row compact">
          <div class="metric-card green-card">
            <span class="metric-label">Bandwidth</span>
            <span class="metric-value green">5134 GB/s</span>
          </div>
          <div class="metric-card green-card">
            <span class="metric-label">Compute</span>
            <span class="metric-value green">2487 TFLOPS</span>
          </div>
          <div class="metric-card green-card">
            <span class="metric-label">BW Util</span>
            <span class="metric-value green">64.2% <span class="delta-up">+33.2%</span></span>
          </div>
          <div class="metric-card green-card">
            <span class="metric-label">Compute Util</span>
            <span class="metric-value green">24.7% <span class="delta-up">+12.7%</span></span>
          </div>
          <div class="metric-card green-card">
            <span class="metric-label">Speedup <span class="avg-times-badge">×</span></span>
            <span class="metric-value green">2.4×</span>
          </div>
        </div>

        <!-- Deep-dive tabs (commented out) -->
        <!--
        <div class="detail-tabs">
          <button class="detail-tab active">Roofline</button>
          <button class="detail-tab">Assembly</button>
          <button class="detail-tab">TraceDiff</button>
          <button class="detail-tab">Trace Viewer</button>
        </div>

        <div class="detail-tab-panels">
          <div class="detail-panel active" data-panel="Roofline">
            <div class="roofline-chart">
              <div class="roofline-header">
                <span class="roofline-title">Roofline Model — fp4_moe_gemm_kernel</span>
                <span class="roofline-sub">MI355X · Peak BW: 8000 GB/s · Peak Compute: 10040 TFLOPS</span>
              </div>
              <div class="roofline-diagram">
                <svg viewBox="0 0 600 320" xmlns="http://www.w3.org/2000/svg" class="roofline-svg">
                  <defs>
                    <pattern id="grid" width="60" height="40" patternUnits="userSpaceOnUse">
                      <path d="M 60 0 L 0 0 0 40" fill="none" stroke="#e5e7eb" stroke-width="0.5"/>
                    </pattern>
                  </defs>
                  <rect x="60" y="20" width="520" height="260" fill="url(#grid)"/>
                  <line x1="60" y1="280" x2="580" y2="280" stroke="#4b5563" stroke-width="1.5"/>
                  <line x1="60" y1="20" x2="60" y2="280" stroke="#4b5563" stroke-width="1.5"/>
                  <text x="320" y="310" text-anchor="middle" font-size="11" fill="#6b7280">Operational Intensity (FLOPS/Byte)</text>
                  <line x1="60" y1="260" x2="350" y2="60" stroke="#3b82f6" stroke-width="2" stroke-dasharray="6 3"/>
                  <text x="180" y="140" font-size="10" fill="#3b82f6" transform="rotate(-42 180 140)">Peak BW: 8000 GB/s</text>
                  <line x1="350" y1="60" x2="580" y2="60" stroke="#ef4444" stroke-width="2" stroke-dasharray="6 3"/>
                  <text x="470" y="52" font-size="10" fill="#ef4444">Peak: 10040 TFLOPS</text>
                  <circle cx="170" cy="230" r="8" fill="#e4002b" stroke="white" stroke-width="2"/>
                  <circle cx="260" cy="170" r="8" fill="#22c55e" stroke="white" stroke-width="2"/>
                  <circle cx="350" cy="60" r="4" fill="#4b5563"/>
                </svg>
              </div>
            </div>
          </div>
          <div class="detail-panel" data-panel="Assembly">
            <div class="assembly-panel">
              <div class="asm-line comment">; Disassembly of fp4_moe_gemm_kernel</div>
            </div>
          </div>
          <div class="detail-panel" data-panel="TraceDiff">
            <div class="tracediff-panel">
              <div class="tracediff-header">
                <span class="tracediff-title">Trace Diff — fp4_moe_gemm_kernel</span>
              </div>
            </div>
          </div>
          <div class="detail-panel" data-panel="Trace Viewer">
            <div class="traceviewer-placeholder">
              <div class="building-icon">🔍</div>
              <div class="building-title">Trace Viewer</div>
            </div>
          </div>
        </div>
        -->
      </div>

      <!-- E2E Perf w/ Optimized Kernels (hidden until Start GEAK Agent) -->
      <div class="card opt-hidden geak-agent-card" id="opt-e2e-card">
        <div class="card-header">
          <h2>E2E Performance — Optimized Metrics</h2>
          <span class="card-subtitle" id="opt-e2e-subtitle">DeepSeek R1 0528 · SGLang · MI355X · FP4 · 1K/1K · conc=4 · TP=8</span>
        </div>
        <div class="metrics-row e2e-opt-metrics" id="opt-e2e-metrics">
          <div class="metric-card green-card e2e-metric-delta">
            <span class="metric-label">INPUT THROUGHPUT/GPU</span>
            <span class="metric-value green" id="opt-m-input">52.27 tok/s/GPU</span>
            <span class="metric-sub-was">was 50.65 <span class="metric-delta-badge up">▲ 3.2%</span></span>
          </div>
          <div class="metric-card green-card e2e-metric-delta">
            <span class="metric-label">OUTPUT THROUGHPUT/GPU</span>
            <span class="metric-value green" id="opt-m-output">51.76 tok/s/GPU</span>
            <span class="metric-sub-was">was 50.16 <span class="metric-delta-badge up">▲ 3.2%</span></span>
          </div>
          <div class="metric-card green-card e2e-metric-delta">
            <span class="metric-label">TTFT <span class="metric-label-hint">(Time to First Token)</span></span>
            <span class="metric-value green" id="opt-m-ttft">88.15 ms</span>
            <span class="metric-sub-was">was 90.69ms <span class="metric-delta-badge down-good">▼ 2.8%</span></span>
          </div>
          <div class="metric-card green-card e2e-metric-delta">
            <span class="metric-label">ITL <span class="metric-label-hint">(Inter-Token Latency)</span></span>
            <span class="metric-value green" id="opt-m-itl">9.23 ms</span>
            <span class="metric-sub-was">was 9.53ms <span class="metric-delta-badge down-good">▼ 3.1%</span></span>
          </div>
          <div class="metric-card green-card e2e-metric-delta">
            <span class="metric-label">TOTAL E2E UPLIFT</span>
            <span class="metric-value green" id="opt-m-uplift">+3.2%</span>
            <span class="metric-sub-was">Dtype +1.7% | MoE fused +1.5%</span>
          </div>
          <div class="metric-card e2e-metric-delta gap-metric-card">
            <span class="metric-label">GAP vs B200</span>
            <span class="metric-value" id="opt-m-gap-val"><span class="gap-from">22%</span> → <span class="gap-to green">~19%</span></span>
            <span class="metric-sub-was">gap vs B200 (conc=4) <span class="metric-delta-badge down-good">▼ 3pp</span></span>
          </div>
        </div>

        <!-- E2E integration issues -->
        <div class="e2e-integration-notes" id="opt-e2e-notes">
          <div class="e2e-note yellow"><span class="note-icon">⚠️</span> MoE Sort: sort kernel 1.6× uplift but MoE kernel 3× slower — under investigation</div>
          <div class="e2e-note yellow"><span class="note-icon">⚠️</span> BS4 MLA: integrates correctly but 2.2× slowdown vs UT speedup — investigating</div>
          <div class="e2e-note red"><span class="note-icon">❌</span> Meta OOB: Fused MoE — not showing perf improvement</div>
        </div>
      </div>

      <!-- GEAK Agent Settings Modal -->
      <div class="geak-settings-overlay" id="geak-settings-overlay" style="display:none;">
        <div class="geak-settings-modal">
          <div class="geak-settings-header">
            <h3>⚙️ GEAK Agent Configuration</h3>
            <button class="geak-settings-close" id="geak-settings-close">✕</button>
          </div>
          <div class="geak-settings-body">
            <div class="geak-setting-group">
              <label class="geak-setting-label">LLM API Key <span class="geak-setting-hint">API key for the LLM provider</span></label>
              <input type="text" class="geak-setting-input" id="geak-set-api" value="sk-a3f8c1d9e7b24f60a1c5d8e3f9b07a42" placeholder="sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" />
            </div>
            <div class="geak-setting-group">
              <label class="geak-setting-label">Model <span class="geak-setting-hint">Select the LLM model for Agent</span></label>
              <div class="geak-model-select" id="geak-model-select">
                <div class="geak-model-trigger" id="geak-model-trigger">
                  <span class="geak-model-value" id="geak-model-value">Claude Sonnet 4.5</span>
                  <span class="caret">▾</span>
                </div>
                <div class="geak-model-collapse" id="geak-model-dropdown">
                  <div class="geak-model-option active" data-model="Claude Sonnet 4.5">Claude Sonnet 4.5</div>
                  <div class="geak-model-option" data-model="Claude Opus 4.5">Claude Opus 4.5</div>
                  <div class="geak-model-option" data-model="GPT-5">GPT-5</div>
                  <div class="geak-model-option" data-model="GPT-5.1">GPT-5.1</div>
                  <div class="geak-model-option" data-model="GPT-5 Codex">GPT-5 Codex</div>
                  <div class="geak-model-option" data-model="Gemini 3 Pro">Gemini 3 Pro</div>
                </div>
              </div>
            </div>
            <div class="geak-setting-group">
              <label class="geak-setting-label">Max Steps per Task <span class="geak-setting-hint">Number of optimization iterations per kernel</span></label>
              <input type="number" class="geak-setting-input" id="geak-set-steps" value="8" min="1" max="50" />
            </div>
            <div class="geak-setting-group">
              <label class="geak-setting-label">Parallelism <span class="geak-setting-hint">Number of kernels to optimize concurrently</span></label>
              <input type="number" class="geak-setting-input" id="geak-set-parallel" value="1" min="1" max="8" />
            </div>
          </div>
          <div class="geak-settings-footer">
            <button class="btn btn-secondary" id="geak-settings-cancel">Cancel</button>
            <button class="btn btn-primary" id="geak-settings-apply">✅ Apply & Start Agent</button>
          </div>
        </div>
      </div>

    </div><!-- end page-optimization -->

    <!-- ╔══════════════════════════════════════════════════════╗ -->
    <!-- ║          PAGE 4: REPORT                             ║ -->
    <!-- ╚══════════════════════════════════════════════════════╝ -->
    <div id="page-report" class="page">

      <!-- ① Expose Configuration -->
      <div class="card">
        <div class="card-header">
          <h2>Expose Configuration</h2>
          <span class="card-subtitle">Select model + parameters → Expose to Status Overview</span>
        </div>

        <!-- Filter Row: All in one line -->
        <div class="report-filter-bar single-row">
          <div class="filter-group dropdown-wrap" id="rpt-dd-model">
            <label class="filter-label">Model <span class="info-icon">ⓘ</span></label>
            <div class="filter-select dropdown-trigger">
              <span class="dd-value">gpt-oss 120B</span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu rpt-dd-scroll">
              <div class="dd-item active" data-val="gpt-oss 120B">gpt-oss 120B <span class="dd-sub">MoE · 120B</span></div>
              <div class="dd-item" data-val="DeepSeek R1 0528">DeepSeek R1 0528 <span class="dd-sub">MoE · 671B</span></div>
            </div>
          </div>
          <div class="filter-group dropdown-wrap" id="rpt-dd-precision">
            <label class="filter-label">Precision <span class="info-icon">ⓘ</span></label>
            <div class="filter-select dropdown-trigger">
              <span class="dd-value">FP4</span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu rpt-dd-scroll">
              <div class="dd-item active" data-val="FP4">● FP4</div>
              <div class="dd-item" data-val="FP8">■ FP8</div>
            </div>
          </div>
          <div class="filter-group dropdown-wrap" id="rpt-dd-isl-osl">
            <label class="filter-label">ISL / OSL <span class="info-icon">ⓘ</span></label>
            <div class="filter-select dropdown-trigger">
              <span class="dd-value">1K / 1K</span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu rpt-dd-scroll">
              <div class="dd-item active" data-val="1K / 1K">1K / 1K</div>
              <div class="dd-item" data-val="1K / 8K">1K / 8K</div>
              <div class="dd-item" data-val="8K / 1K">8K / 1K</div>
            </div>
          </div>
          <div class="filter-group dropdown-wrap" id="rpt-dd-amd">
            <label class="filter-label">AMD GPU Config <span class="info-icon">ⓘ</span></label>
            <div class="filter-select dropdown-trigger">
              <span class="dd-value">MI355X (SGLang)</span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu rpt-dd-scroll">
              <div class="dd-item active" data-val="MI355X (SGLang)">MI355X (SGLang)</div>
              <div class="dd-item" data-val="MI355X (vLLM)">MI355X (vLLM)</div>
            </div>
          </div>
          <div class="filter-group dropdown-wrap" id="rpt-dd-nv">
            <label class="filter-label">NV Ref <span class="dd-optional">(opt.)</span></label>
            <div class="filter-select dropdown-trigger">
              <span class="dd-value nv-none">— None —</span>
              <span class="caret">▾</span>
            </div>
            <div class="dropdown-menu rpt-dd-scroll">
              <div class="dd-item active" data-val="">— None —</div>
              <div class="dd-item" data-val="B200 (SGLang)">B200 (SGLang)</div>
              <div class="dd-item" data-val="B200 (vLLM)">B200 (vLLM)</div>
            </div>
          </div>
          <div class="filter-group form-group-action">
            <label class="filter-label">&nbsp;</label>
            <button class="btn btn-primary" id="btn-expose">🚀 Expose</button>
          </div>
        </div>

        <!-- Expose History (scrollable list of exposed items) -->
        <div class="expose-history-wrap">
          <label class="filter-label expose-history-label">Exposed History <span class="expose-count" id="expose-count">0 items</span></label>
          <div class="expose-history-list" id="expose-history-list">
            <div class="expose-empty" id="expose-empty">No items exposed yet. Select parameters above and click <strong>Expose</strong>.</div>
          </div>
        </div>
      </div>

      <!-- ② Status Overview (dynamic — populated by Expose) -->
      <div class="card">
        <div class="card-header">
          <h2>Status Overview</h2>
          <span class="card-subtitle" id="status-overview-sub">No items exposed · Click Expose above to add entries</span>
        </div>

        <div class="status-table" id="status-table">
          <div class="status-body-scroll" id="status-body">
            <div class="status-header" id="status-header">
              <span class="st-col st-model">Model (AMD)</span>
              <span class="st-col st-nvref" id="st-nvref-header" style="display:none;">NV Reference</span>
              <span class="st-col st-gap" id="st-gap-header" style="display:none;">Gap vs B200</span>
              <span class="st-col st-roofline">Gap vs Roofline</span>
              <span class="st-col st-phase">Profiling (Top Kernels)</span>
              <span class="st-col st-phase">Kernel Opt (Coverage)</span>
              <span class="st-col st-phase">Expected E2E</span>
              <span class="st-col st-e2e">E2E Perf</span>
            </div>
            <div class="status-empty" id="status-empty">
              <span class="status-empty-icon">📋</span>
              <span>No models exposed yet. Use the configuration above to add entries.</span>
            </div>
          </div>
        </div>
      </div>

      <!-- ④ Per-Model Detail Reports (hidden until Expose) -->
      <div class="card" id="detail-reports-card" style="display:none;">
        <div class="card-header">
          <h2>Per-Model Detail Reports</h2>
          <span class="card-subtitle" id="detail-reports-sub">Model & Configuration · Kernel-Level Optimization · E2E Integration</span>
          <div class="report-batch-actions" id="report-batch-actions" style="display:none;">
            <span class="batch-count" id="batch-count">0 selected</span>
            <button class="btn btn-primary btn-sm" id="btn-batch-export">📄 Export Selected Reports (PDF)</button>
          </div>
        </div>

        <!-- ─── Model A: Deepseek R1 0528 (FP4) ─── -->
        <div class="report-model collapsed" data-report-model="DeepSeek R1 0528||FP4" style="display:none;">
          <div class="report-model-header">
            <label class="report-model-check" onclick="event.stopPropagation()"><input type="checkbox" class="rpt-model-cb" /><span class="rpt-cb-mark"></span></label>
            <span class="report-toggle">▸</span>
            <span class="report-model-name">DeepSeek R1 0528 (FP4) — SemiAnalysis · Meta OOB</span>
            <span class="report-model-config">Config: 1k1k conc=4</span>
          </div>
          <div class="report-model-body" style="display:none;">

            <!-- ── Level 1: Model & Configuration — E2E Profiling ── -->
            <div class="report-subsection collapsed">
              <div class="report-subsection-header" data-sub-toggle>
                <span class="report-sub-toggle">▸</span>
                <span class="report-sub-title">📊 Model & Configuration — E2E Profiling</span>
                <span class="report-sub-status">Gap: 22% → ~19% vs B200</span>
              </div>
              <div class="report-subsection-body" style="display:none;">
                <!-- E2E Performance Metrics (aligned with Optimization page) -->
                <div class="metrics-row e2e-opt-metrics rpt-e2e-metrics">
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">INPUT THROUGHPUT/GPU</span>
                    <span class="metric-value green">52.27 tok/s/GPU</span>
                    <span class="metric-sub-was">was 50.65 <span class="metric-delta-badge up">▲ 3.2%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">OUTPUT THROUGHPUT/GPU</span>
                    <span class="metric-value green">51.76 tok/s/GPU</span>
                    <span class="metric-sub-was">was 50.16 <span class="metric-delta-badge up">▲ 3.2%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">TTFT <span class="metric-label-hint">(Time to First Token)</span></span>
                    <span class="metric-value green">88.15 ms</span>
                    <span class="metric-sub-was">was 90.69ms <span class="metric-delta-badge down-good">▼ 2.8%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">ITL <span class="metric-label-hint">(Inter-Token Latency)</span></span>
                    <span class="metric-value green">9.23 ms</span>
                    <span class="metric-sub-was">was 9.53ms <span class="metric-delta-badge down-good">▼ 3.1%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">TOTAL E2E UPLIFT</span>
                    <span class="metric-value green">+3.2%</span>
                    <span class="metric-sub-was">Dtype +1.7% | MoE fused +1.5%</span>
                  </div>
                </div>

                <!-- Gap Improvement -->
                <div class="rpt-gap-improvement">
                  <div class="rpt-gap-row">
                    <span class="rpt-gap-label">Gap vs B200 (conc=4, TP=8):</span>
                    <span class="rpt-gap-from red">22%</span>
                    <span class="rpt-gap-arrow">→</span>
                    <span class="rpt-gap-to green">~19%</span>
                    <span class="metric-delta-badge down-good">▼ 3pp</span>
                  </div>
                  <div class="rpt-gap-bar-wrap">
                    <div class="gap-bar wide"><div class="gap-fill yellow" style="width:78%"></div><div class="gap-fill-overlay green" style="width:81%"></div></div>
                  </div>
                </div>

                <!-- Model Profile Summary -->
                <div class="rpt-profile-summary">
                  <div class="report-section-header compact">
                    <span>🔬 Model Profiling: Complete</span>
                  </div>
                  <div class="report-kernels-row">
                    <div class="report-kernel-card hot">
                      <span class="rk-name">MoE  31%</span>
                      <span class="rk-desc">of workload</span>
                    </div>
                    <div class="report-kernel-card hot">
                      <span class="rk-name">Fused SGLang MoE  20.9%</span>
                    </div>
                    <div class="report-kernel-card warm">
                      <span class="rk-name">MLA  13%</span>
                      <span class="rk-desc">of workload</span>
                    </div>
                    <div class="report-kernel-card warm">
                      <span class="rk-name">Attn GEMMs  13%</span>
                      <span class="rk-desc">of workload</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- ── Level 2: Kernel-Level Optimization Results ── -->
            <div class="report-subsection collapsed">
              <div class="report-subsection-header" data-sub-toggle>
                <span class="report-sub-toggle">▸</span>
                <span class="report-sub-title">🔧 Kernel-Level Optimization Results</span>
                <span class="report-sub-status">5 optimizations · MoE +66% · MLA Attn · SwiGLU · RMSNorm</span>
              </div>
              <div class="report-subsection-body" style="display:none;">
                <!-- P1: MoE FP4 GEMM Opt. -->
                <div class="rko-card">
                  <div class="rko-header">
                    <span class="rko-name">fp4_moe_gemm_kernel</span>
                    <span class="rko-tag hot">MoE GEMM · 31% of WL</span>
                  </div>
                  <div class="rko-body">
                    <div class="rko-metrics-row">
                      <div class="rko-metric">
                        <span class="rko-label">Throughput:</span>
                        <span class="rko-val">229 TFLOPS → 377 TFLOPS</span>
                        <span class="rko-uplift green">+66%</span>
                      </div>
                      <div class="rko-metric">
                        <span class="rko-label">Latency:</span>
                        <span class="rko-val">9.44 ms → 5.75 ms</span>
                        <span class="rko-uplift green">1.64× faster</span>
                      </div>
                    </div>
                    <div class="rko-roofline">
                      <span class="rko-rf-label">vs Roofline:</span>
                      BW Util: 31% → 64.2% (+33.2%) · Compute Util: 12% → 24.7% (+12.7%) · Speedup: 2.4×
                    </div>
                  </div>
                </div>

                <!-- P1b: Fused SGLang MoE -->
                <div class="rko-card">
                  <div class="rko-header">
                    <span class="rko-name">fp4_dequant_gemm</span>
                    <span class="rko-tag hot">Fused Dequant+GEMM · 20.9% of WL</span>
                  </div>
                  <div class="rko-body">
                    <div class="rko-metrics-row">
                      <div class="rko-metric">
                        <span class="rko-label">Optimization:</span>
                        <span class="rko-val">Fused dequantize + GEMM · Shared memory staging</span>
                      </div>
                      <div class="rko-metric">
                        <span class="rko-label">E2E Uplift:</span>
                        <span class="rko-val">Eliminated intermediate buffer writes</span>
                        <span class="rko-uplift green">+1.5%</span>
                      </div>
                    </div>
                    <div class="rko-roofline">
                      <span class="rko-rf-label">vs Roofline:</span>
                      BW Util: 28% → 58% (+30%) · Reduced BW pressure 40%
                    </div>
                  </div>
                </div>

                <!-- P2: MLA Flash Attention -->
                <div class="rko-card">
                  <div class="rko-header">
                    <span class="rko-name">flash_attn_fwd_v2 + mla_absorb_proj</span>
                    <span class="rko-tag med">MLA Attention · 13% of WL</span>
                  </div>
                  <div class="rko-body">
                    <div class="rko-metrics-row">
                      <div class="rko-metric">
                        <span class="rko-label">Optimization:</span>
                        <span class="rko-val">Fuse absorb_attn + flash_attn · Swizzled KV layout (d_c=512)</span>
                      </div>
                      <div class="rko-metric">
                        <span class="rko-label">BW Improvement:</span>
                        <span class="rko-val">53% → 72% HBM peak</span>
                        <span class="rko-uplift green">-26.0ms</span>
                      </div>
                    </div>
                    <div class="rko-roofline">
                      <span class="rko-rf-label">vs Roofline:</span>
                      Eliminate 122 absorb_proj kernel launches · Reduce latent projection overhead · E2E: 3.6%
                    </div>
                  </div>
                </div>

                <!-- P4: SwiGLU Fusion -->
                <div class="rko-card">
                  <div class="rko-header">
                    <span class="rko-name">silu_activation + elementwise_mul</span>
                    <span class="rko-tag">SwiGLU Fusion · Activ.&Norm</span>
                  </div>
                  <div class="rko-body">
                    <div class="rko-metrics-row">
                      <div class="rko-metric">
                        <span class="rko-label">Optimization:</span>
                        <span class="rko-val">Fuse silu + mul into single kernel per expert per layer</span>
                      </div>
                      <div class="rko-metric">
                        <span class="rko-label">Saving:</span>
                        <span class="rko-val">Eliminate one HBM round-trip per expert</span>
                        <span class="rko-uplift green">-10.2ms</span>
                      </div>
                    </div>
                    <div class="rko-roofline">
                      <span class="rko-rf-label">vs Roofline:</span>
                      BW Util: 52% pre-opt · 122 ops across 61 layers · E2E: 1.4%
                    </div>
                  </div>
                </div>

                <!-- P5: RMSNorm Fusion -->
                <div class="rko-card">
                  <div class="rko-header">
                    <span class="rko-name">rms_norm_triton + residual_add</span>
                    <span class="rko-tag">RMSNorm Fusion · 984 ops</span>
                  </div>
                  <div class="rko-body">
                    <div class="rko-metrics-row">
                      <div class="rko-metric">
                        <span class="rko-label">Optimization:</span>
                        <span class="rko-val">Fuse 8-kernel decomposition (cast→pow→mean→…) into single Triton kernel</span>
                      </div>
                      <div class="rko-metric">
                        <span class="rko-label">Saving:</span>
                        <span class="rko-val">123 instances × 8 kernels → 123 fused ops</span>
                        <span class="rko-uplift green">-4.7ms</span>
                      </div>
                    </div>
                    <div class="rko-roofline">
                      <span class="rko-rf-label">vs Roofline:</span>
                      BW Util: 93% pre-opt (near BW ceiling) · Single-pass read+normalize+write · E2E: 0.7%
                    </div>
                  </div>
                </div>

              </div>
            </div>

            <!-- ── Level 2: E2E Integration Results ── -->
            <div class="report-subsection collapsed">
              <div class="report-subsection-header" data-sub-toggle>
                <span class="report-sub-toggle">▸</span>
                <span class="report-sub-title">📈 E2E Integration Results</span>
                <span class="report-sub-status">SemiAnalysis +1.7% · MoE Fused +1.5%</span>
              </div>
              <div class="report-subsection-body" style="display:none;">
                <div class="report-detail-row green">
                  <span class="rd-accent green"></span>
                  <strong>SemiAnalysis:</strong>&nbsp; Dtype change (non-AI optimized) <span class="rd-value green">+1.7%</span> uplift &nbsp;|&nbsp; MoE fused <span class="rd-value green">+1.5%</span> uplift
                </div>
                <div class="report-detail-row yellow">
                  <span class="rd-accent yellow"></span>
                  <strong>⚠️ MoE Sort:</strong>&nbsp; sort kernel 1.6× uplift but MoE kernel 3× slower — regression under investigation
                </div>
                <div class="report-detail-row yellow">
                  <span class="rd-accent yellow"></span>
                  <strong>⚠️ BS4 MLA:</strong>&nbsp; Integrates correctly but 2.2× slowdown vs UT speedup — investigating kernel behavior differences
                </div>
                <div class="report-detail-row red">
                  <span class="rd-accent red"></span>
                  <strong>Meta OOB:</strong>&nbsp; Fused MoE — <span class="rd-value red">not showing perf improvement</span>
                </div>
              </div>
            </div>

          </div>
        </div>

        <!-- ─── Model B: gpt-oss 120B (FP4) ─── -->
        <div class="report-model collapsed" data-report-model="gpt-oss 120B||FP4" style="display:none;">
          <div class="report-model-header">
            <label class="report-model-check" onclick="event.stopPropagation()"><input type="checkbox" class="rpt-model-cb" /><span class="rpt-cb-mark"></span></label>
            <span class="report-toggle">▸</span>
            <span class="report-model-name">gpt-oss 120B (FP4) — MLPerf · SemiAnalysis · Meta OOB</span>
            <span class="report-model-summary">Attn FP8 32.7% WL | +10-15% kernel | Gap: 32%→~27%</span>
          </div>
          <div class="report-model-body" style="display:none;">

            <!-- Level 1: Model & Configuration -->
            <div class="report-subsection collapsed">
              <div class="report-subsection-header" data-sub-toggle>
                <span class="report-sub-toggle">▸</span>
                <span class="report-sub-title">📊 Model & Configuration — E2E Profiling</span>
                <span class="report-sub-status">Gap: 32% → ~27% vs B200</span>
              </div>
              <div class="report-subsection-body" style="display:none;">
                <!-- E2E Performance Metrics -->
                <div class="metrics-row e2e-opt-metrics rpt-e2e-metrics">
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">INPUT THROUGHPUT/GPU</span>
                    <span class="metric-value green">72.92 tok/s/GPU</span>
                    <span class="metric-sub-was">was 67.52 <span class="metric-delta-badge up">▲ 8.0%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">OUTPUT THROUGHPUT/GPU</span>
                    <span class="metric-value green">71.56 tok/s/GPU</span>
                    <span class="metric-sub-was">was 66.26 <span class="metric-delta-badge up">▲ 8.0%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">TTFT <span class="metric-label-hint">(Time to First Token)</span></span>
                    <span class="metric-value green">62.6 ms</span>
                    <span class="metric-sub-was">was 67.6ms <span class="metric-delta-badge down-good">▼ 7.4%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">ITL <span class="metric-label-hint">(Inter-Token Latency)</span></span>
                    <span class="metric-value green">7.37 ms</span>
                    <span class="metric-sub-was">was 7.96ms <span class="metric-delta-badge down-good">▼ 7.4%</span></span>
                  </div>
                  <div class="metric-card green-card e2e-metric-delta">
                    <span class="metric-label">TOTAL E2E UPLIFT</span>
                    <span class="metric-value green">+8.0%</span>
                    <span class="metric-sub-was">Attn FP8 +15% | 62.7% WL covered</span>
                  </div>
                </div>

                <!-- Gap -->
                <div class="rpt-gap-improvement">
                  <div class="rpt-gap-row">
                    <span class="rpt-gap-label">Gap vs B200 (conc=4, TP=8):</span>
                    <span class="rpt-gap-from red">32%</span>
                    <span class="rpt-gap-arrow">→</span>
                    <span class="rpt-gap-to yellow">~27%</span>
                    <span class="metric-delta-badge down-good">▼ 5pp</span>
                  </div>
                  <div class="rpt-gap-bar-wrap">
                    <div class="gap-bar wide"><div class="gap-fill red" style="width:68%"></div><div class="gap-fill-overlay yellow" style="width:73%"></div></div>
                  </div>
                </div>

                <!-- Profiling Summary -->
                <div class="rpt-profile-summary">
                  <div class="report-section-header compact">
                    <span>🔬 Model Profiling: Complete</span>
                  </div>
                  <div class="report-kernels-row">
                    <div class="report-kernel-card hot">
                      <span class="rk-name">Attention FP8  32.7%</span>
                      <span class="rk-desc">of workload</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Level 2: Kernel-Level Optimization Results -->
            <div class="report-subsection collapsed">
              <div class="report-subsection-header" data-sub-toggle>
                <span class="report-sub-toggle">▸</span>
                <span class="report-sub-title">🔧 Kernel-Level Optimization Results</span>
                <span class="report-sub-status">Attention FP8 · +10-15% over BF16</span>
              </div>
              <div class="report-subsection-body" style="display:none;">
                <div class="rko-card">
                  <div class="rko-header">
                    <span class="rko-name">Attention Kernel (FP8)</span>
                    <span class="rko-tag hot">32.7% of workload</span>
                  </div>
                  <div class="rko-body">
                    <div class="rko-metrics-row">
                      <div class="rko-metric">
                        <span class="rko-label">Optimization:</span>
                        <span class="rko-val">FP8 delivers 10–15% uplift over BF16</span>
                        <span class="rko-uplift green">+10-15%</span>
                      </div>
                      <div class="rko-metric">
                        <span class="rko-label">Root Cause:</span>
                        <span class="rko-val">Identified FP8 kernel memory fault root cause</span>
                      </div>
                    </div>
                    <div class="rko-roofline">
                      <span class="rko-rf-label">Notes:</span>
                      Triton already bandwidth-bound · Minimal further optimization possible · Delivered for integration
                    </div>
                  </div>
                </div>

              </div>
            </div>

            <!-- ── Level 2: E2E Integration Results ── -->
            <div class="report-subsection collapsed">
              <div class="report-subsection-header" data-sub-toggle>
                <span class="report-sub-toggle">▸</span>
                <span class="report-sub-title">📈 E2E Integration Results</span>
                <span class="report-sub-status">MLPerf +8% · Meta OOB +2-2.5%</span>
              </div>
              <div class="report-subsection-body" style="display:none;">
                <div class="report-detail-row green">
                  <span class="rd-accent green"></span>
                  <strong>MLPerf:</strong>&nbsp; ~8% end-to-end uplift in Offline and Server modes
                </div>
                <div class="report-detail-row green">
                  <span class="rd-accent green"></span>
                  <strong>Meta OOB:</strong>&nbsp; ~2-2.5% overall E2E geomean improvement
                </div>
                <div class="report-detail-row yellow">
                  <span class="rd-accent yellow"></span>
                  <strong>SemiAnalysis:</strong>&nbsp; In Progress — ETA: 3/10
                </div>
              </div>
            </div>

          </div>
        </div>

      </div>

      <!-- ⑤ Export Action Bar -->
      <div class="action-bar">
        <button class="btn btn-export-csv">📊 Export CSV</button>
        <button class="btn btn-share">🔗 Share Link</button>
      </div>
      <div class="export-hint">Per Model: each model → separate file &nbsp;|&nbsp; CSV: raw data export</div>

    </div><!-- end page-report -->
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, watch, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';

defineOptions({ name: 'HyperLoomDashboard' });

const route = useRoute();
const router = useRouter();

const PAGE_MAP = {
  overview: 'page-overview',
  analysis: 'page-analysis',
  optimization: 'page-optimization',
  report: 'page-report',
};

function switchPage(routeName) {
  const targetId = PAGE_MAP[routeName];
  if (!targetId) return;
  document.querySelectorAll('.legacy-dashboard-wrapper .page').forEach(p => {
    p.classList.toggle('active', p.id === targetId);
  });
}

let scriptEl = null;

function handleDataPageClick(e) {
  const btn = e.target.closest('[data-page]');
  if (!btn) return;
  const page = btn.dataset.page;
  if (page && PAGE_MAP[page]) {
    e.preventDefault();
    router.push({ name: page });
  }
}

onMounted(async () => {
  await nextTick();

  scriptEl = document.createElement('script');
  scriptEl.src = '/hyperloom/legacy/app.js?v=' + Date.now();
  document.body.appendChild(scriptEl);

  document.querySelector('.legacy-dashboard-wrapper')
    ?.addEventListener('click', handleDataPageClick);

  setTimeout(() => {
    switchPage(route.name);
  }, 150);
});

onUnmounted(() => {
  if (scriptEl && scriptEl.parentNode) {
    scriptEl.parentNode.removeChild(scriptEl);
  }
  document.querySelector('.legacy-dashboard-wrapper')
    ?.removeEventListener('click', handleDataPageClick);
});

watch(() => route.name, (newName) => {
  if (PAGE_MAP[newName]) {
    switchPage(newName);
  }
});
</script>
