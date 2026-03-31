package api

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Email Relay Service</title>
<style>
  :root {
    --bg: #0f1117; --surface: #1a1d27; --surface2: #242836;
    --border: #2e3348; --text: #e4e6f0; --text2: #8b8fa8;
    --accent: #e67e22; --accent2: #d35400;
    --green: #27ae60; --red: #e74c3c; --blue: #3498db;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: var(--bg); color: var(--text); font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; }

  .header {
    background: linear-gradient(135deg, var(--accent), var(--accent2));
    padding: 20px 32px; display: flex; align-items: center; justify-content: space-between;
  }
  .header h1 { font-size: 18px; font-weight: 600; color: #fff; letter-spacing: -0.5px; }
  .header .tag { background: rgba(255,255,255,0.2); padding: 4px 10px; border-radius: 4px; font-size: 11px; color: #fff; }

  .container { max-width: 1200px; margin: 0 auto; padding: 24px; }

  .section { margin-bottom: 24px; }
  .section-title {
    font-size: 11px; text-transform: uppercase; letter-spacing: 1.5px;
    color: var(--text2); margin-bottom: 12px; font-weight: 600;
  }

  .clusters { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
  .cluster-card {
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 16px;
    transition: border-color 0.2s;
  }
  .cluster-card:hover { border-color: var(--accent); }
  .cluster-name { font-size: 15px; font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }
  .dot.on { background: var(--green); box-shadow: 0 0 6px var(--green); }
  .dot.off { background: var(--red); }
  .cluster-url { color: var(--text2); font-size: 11px; word-break: break-all; margin-bottom: 10px; }
  .cluster-stats { display: flex; gap: 16px; font-size: 12px; }
  .cluster-stats .stat { display: flex; align-items: center; gap: 4px; }
  .stat-val { font-weight: 700; }
  .stat-val.sent { color: var(--green); }
  .stat-val.fail { color: var(--red); }

  .test-panel {
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 20px;
    display: grid; grid-template-columns: 1fr 1fr 1fr auto; gap: 10px; align-items: end;
  }
  .field label { display: block; font-size: 10px; text-transform: uppercase; letter-spacing: 1px; color: var(--text2); margin-bottom: 6px; }
  .field input, .field select {
    width: 100%; background: var(--surface2); border: 1px solid var(--border); border-radius: 6px;
    padding: 8px 12px; color: var(--text); font-family: inherit; font-size: 13px; outline: none;
  }
  .field input:focus, .field select:focus { border-color: var(--accent); }
  .btn {
    background: linear-gradient(135deg, var(--accent), var(--accent2));
    color: #fff; border: none; border-radius: 6px; padding: 9px 20px;
    font-family: inherit; font-size: 13px; font-weight: 600; cursor: pointer;
    transition: opacity 0.2s;
  }
  .btn:hover { opacity: 0.85; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .history-table { width: 100%; border-collapse: collapse; }
  .history-table th {
    text-align: left; font-size: 10px; text-transform: uppercase; letter-spacing: 1px;
    color: var(--text2); padding: 8px 12px; border-bottom: 1px solid var(--border);
  }
  .history-table td {
    padding: 10px 12px; border-bottom: 1px solid var(--border); font-size: 12px; vertical-align: top;
  }
  .history-table tr:hover td { background: var(--surface2); }
  .badge {
    display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600;
  }
  .badge.sent { background: rgba(39,174,96,0.15); color: var(--green); }
  .badge.failed { background: rgba(231,76,60,0.15); color: var(--red); }
  .badge.cluster-tag { background: rgba(52,152,219,0.15); color: var(--blue); }
  .recipients { color: var(--text2); font-size: 11px; }
  .error-text { color: var(--red); font-size: 11px; margin-top: 2px; }
  .time-text { color: var(--text2); font-size: 11px; }

  .filter-bar { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; }
  .filter-bar select {
    background: var(--surface2); border: 1px solid var(--border); border-radius: 6px;
    padding: 6px 12px; color: var(--text); font-family: inherit; font-size: 12px; outline: none;
  }
  .auto-refresh { color: var(--text2); font-size: 11px; margin-left: auto; }

  .toast {
    position: fixed; bottom: 20px; right: 20px; padding: 12px 20px; border-radius: 8px;
    font-size: 13px; font-weight: 500; z-index: 999; transition: opacity 0.3s;
    box-shadow: 0 4px 12px rgba(0,0,0,0.4);
  }
  .toast.success { background: var(--green); color: #fff; }
  .toast.error { background: var(--red); color: #fff; }
  .toast.hidden { opacity: 0; pointer-events: none; }

  @media (max-width: 700px) {
    .test-panel { grid-template-columns: 1fr; }
  }
</style>
</head>
<body>

<div class="header">
  <h1>Email Relay Service</h1>
  <span class="tag">primus-safe@amd.com</span>
</div>

<div class="container">
  <div class="section">
    <div class="section-title">Clusters</div>
    <div class="clusters" id="clusters"></div>
  </div>

  <div class="section">
    <div class="section-title">Test Send</div>
    <div class="test-panel">
      <div class="field">
        <label>Cluster</label>
        <input type="text" id="testCluster" value="test" placeholder="cluster name">
      </div>
      <div class="field">
        <label>To</label>
        <input type="text" id="testTo" placeholder="user@amd.com">
      </div>
      <div class="field">
        <label>Subject</label>
        <input type="text" id="testSubject" placeholder="Test Email" value="Email Relay Test">
      </div>
      <button class="btn" id="sendBtn" onclick="doTestSend()">Send</button>
    </div>
  </div>

  <div class="section">
    <div class="section-title">Send History</div>
    <div class="filter-bar">
      <select id="filterCluster" onchange="loadHistory()">
        <option value="">All Clusters</option>
      </select>
      <span class="auto-refresh">Auto-refresh: 5s</span>
    </div>
    <div style="background:var(--surface);border:1px solid var(--border);border-radius:8px;overflow:hidden;">
      <table class="history-table">
        <thead>
          <tr>
            <th>Status</th>
            <th>Cluster</th>
            <th>Subject</th>
            <th>Recipients</th>
            <th>Source</th>
            <th>Time</th>
          </tr>
        </thead>
        <tbody id="historyBody"></tbody>
      </table>
      <div id="emptyState" style="padding:40px;text-align:center;color:var(--text2);display:none;">No emails sent yet</div>
    </div>
  </div>
</div>

<div class="toast hidden" id="toast"></div>

<script>
const BASE = window.location.pathname.replace(/\/*$/, '') + '/';

function showToast(msg, type) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.className = 'toast ' + type;
  setTimeout(() => t.className = 'toast hidden', 3000);
}

function relTime(iso) {
  const d = new Date(iso);
  const s = Math.floor((Date.now() - d) / 1000);
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s / 60) + 'm ago';
  if (s < 86400) return Math.floor(s / 3600) + 'h ago';
  return d.toLocaleDateString();
}

async function loadClusters() {
  try {
    const res = await fetch(BASE + 'api/clusters');
    const data = await res.json();
    const el = document.getElementById('clusters');
    const filter = document.getElementById('filterCluster');
    const currentFilter = filter.value;

    el.innerHTML = data.map(c => {
      const connClass = c.connected ? 'on' : 'off';
      const connText = c.connected ? 'Connected' : 'Disconnected';
      const lastEvt = c.last_event && c.last_event !== '0001-01-01T00:00:00Z' ? relTime(c.last_event) : 'never';
      return '<div class="cluster-card">' +
        '<div class="cluster-name"><span class="dot ' + connClass + '"></span>' + esc(c.name) + '</div>' +
        '<div class="cluster-url">' + esc(c.base_url) + ' &middot; ' + connText + '</div>' +
        '<div class="cluster-stats">' +
          '<div class="stat">Sent: <span class="stat-val sent">' + c.sent_count + '</span></div>' +
          '<div class="stat">Failed: <span class="stat-val fail">' + c.fail_count + '</span></div>' +
          '<div class="stat">Last: ' + lastEvt + '</div>' +
        '</div></div>';
    }).join('');

    // Update filter dropdown
    const opts = ['<option value="">All Clusters</option>'].concat(
      data.map(c => '<option value="' + esc(c.name) + '"' + (c.name === currentFilter ? ' selected' : '') + '>' + esc(c.name) + '</option>')
    );
    filter.innerHTML = opts.join('');
  } catch (e) { console.error('loadClusters', e); }
}

async function loadHistory() {
  try {
    const cluster = document.getElementById('filterCluster').value;
    const url = BASE + 'api/history?limit=100' + (cluster ? '&cluster=' + encodeURIComponent(cluster) : '');
    const res = await fetch(url);
    const data = await res.json();
    const tbody = document.getElementById('historyBody');
    const empty = document.getElementById('emptyState');

    if (!data || data.length === 0) {
      tbody.innerHTML = '';
      empty.style.display = 'block';
      return;
    }
    empty.style.display = 'none';

    tbody.innerHTML = data.map(r => {
      const badge = r.status === 'sent'
        ? '<span class="badge sent">SENT</span>'
        : '<span class="badge failed">FAILED</span>';
      const errLine = r.error ? '<div class="error-text">' + esc(r.error) + '</div>' : '';
      return '<tr>' +
        '<td>' + badge + '</td>' +
        '<td><span class="badge cluster-tag">' + esc(r.cluster) + '</span></td>' +
        '<td>' + esc(r.subject) + errLine + '</td>' +
        '<td class="recipients">' + (r.recipients || []).map(esc).join(', ') + '</td>' +
        '<td>' + esc(r.source) + '</td>' +
        '<td class="time-text">' + relTime(r.sent_at) + '</td>' +
        '</tr>';
    }).join('');
  } catch (e) { console.error('loadHistory', e); }
}

async function doTestSend() {
  const btn = document.getElementById('sendBtn');
  btn.disabled = true;
  try {
    const to = document.getElementById('testTo').value.trim();
    if (!to) { showToast('Recipient is required', 'error'); return; }
    const res = await fetch(BASE + 'api/test-send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        cluster: document.getElementById('testCluster').value.trim() || 'test',
        to: to.split(',').map(s => s.trim()).filter(Boolean),
        subject: document.getElementById('testSubject').value.trim() || 'Email Relay Test'
      })
    });
    const data = await res.json();
    if (data.status === 'sent') {
      showToast('Email sent successfully', 'success');
      setTimeout(() => { loadHistory(); loadClusters(); }, 500);
    } else {
      showToast('Send failed: ' + (data.error || 'unknown'), 'error');
    }
  } catch (e) {
    showToast('Error: ' + e.message, 'error');
  } finally {
    btn.disabled = false;
  }
}

function esc(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

loadClusters();
loadHistory();
setInterval(() => { loadClusters(); loadHistory(); }, 5000);
</script>
</body>
</html>`
