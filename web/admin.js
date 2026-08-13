const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

async function api(path, opt) {
  const res = await fetch(path, opt);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function pluginRow(p) {
  return `<div class="src-row" data-name="${esc(p.name)}">
    <label class="chk"><input type="checkbox" ${p.enabled ? 'checked' : ''} /> ${esc(p.name)}</label>
    <input class="base" placeholder="站点/API 根" value="${esc(p.base || '')}" />
    <input class="proxy" placeholder="${p.name === '6v520' ? '不要填代理' : '代理 http://...'}" value="${esc(p.proxy || '')}" ${p.name === '6v520' ? 'disabled' : ''} />
    <button type="button" class="ghost test">测连通</button>
    <span class="muted test-out">${p.live ? '已启用' : '未启用'}</span>
  </div>`;
}

async function loadSettings() {
  const s = await api('/api/settings');
  const box = $('#plugins');
  const names = ['6v520', 'apibay', 'torrents-csv', 'yts'];
  const plugins = s.plugins || {};
  box.innerHTML = names.map(n => {
    const p = plugins[n] || {};
    return pluginRow({ name: n, enabled: !!p.enabled, base: p.base || '', proxy: p.proxy || '', live: !!p.enabled });
  }).join('');
  $('#cid').value = s.client_id || '';
  $('#basedir').value = s.base_dir || '';
  $('#tmdbProxy').value = s.tmdb_proxy || '';
  $('#tmdbLang').value = s.tmdb_language || 'zh-CN';
  $('#aiBase').value = s.ai_base_url || '';
  $('#aiModel').value = s.ai_model || '';
  $('#landMeta').textContent = [
    s.has_client_secret ? '已存 secret' : '未填 secret',
    s.has_tmdb_api_key ? 'TMDB 已配' : 'TMDB 未配',
    s.has_ai_api_key ? 'AI 已配' : 'AI 未配',
  ].join(' · ');
  box.querySelectorAll('.src-row').forEach(row => bindRow(row));
  refreshLand();
}

function collectPlugins() {
  const out = {};
  document.querySelectorAll('.src-row').forEach(row => {
    const name = row.dataset.name;
    out[name] = {
      enabled: row.querySelector('input[type=checkbox]').checked,
      base: row.querySelector('.base').value.trim(),
      proxy: name === '6v520' ? '' : row.querySelector('.proxy').value.trim(),
    };
  });
  return out;
}

function bindRow(row) {
  const name = row.dataset.name;
  const save = async () => {
    try {
      await api('/api/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plugins: collectPlugins() }),
      });
      row.querySelector('.test-out').textContent = '已保存';
    } catch (e) {
      row.querySelector('.test-out').textContent = e.message;
    }
  };
  row.querySelector('input[type=checkbox]').addEventListener('change', save);
  row.querySelector('.test').addEventListener('click', async () => {
    const out = row.querySelector('.test-out');
    out.textContent = '测试中…';
    try {
      const data = await api('/api/plugins/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name,
          base: row.querySelector('.base').value.trim(),
          proxy: row.querySelector('.proxy').value.trim(),
        }),
      });
      out.textContent = data.ok ? `通 · ${data.hits} 条 · ${data.took_ms}ms` : (`失败 · ${data.error || ''}`);
    } catch (e) {
      out.textContent = e.message;
    }
  });
}

async function refreshLand() {
  try {
    const s = await api('/api/2dland/status');
    $('#landMeta').textContent = s.logged_in ? '已登录 2dland' : (s.has_credentials ? '已填凭证，未登录' : '未绑定');
  } catch (e) {
    $('#landMeta').textContent = e.message;
  }
}

$('#saveLand').onclick = async () => {
  const body = {
    client_id: $('#cid').value.trim(),
    base_dir: $('#basedir').value.trim(),
  };
  const sec = $('#csec').value.trim();
  if (sec) body.client_secret = sec;
  try {
    await api('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    $('#csec').value = '';
    flash('#landMeta', '凭证已保存');
    refreshLand();
  } catch (e) {
    flash('#landMeta', e.message);
  }
};

$('#saveMeta').onclick = async () => {
  const body = {
    tmdb_proxy: $('#tmdbProxy').value.trim(),
    tmdb_language: $('#tmdbLang').value.trim() || 'zh-CN',
    ai_base_url: $('#aiBase').value.trim(),
    ai_model: $('#aiModel').value.trim(),
  };
  const tk = $('#tmdb').value.trim();
  const ak = $('#aiKey').value.trim();
  if (tk) body.tmdb_api_key = tk;
  if (ak) body.ai_api_key = ak;
  try {
    await api('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    $('#tmdb').value = '';
    $('#aiKey').value = '';
    flash('#tmdbMeta', '整理设置已保存');
  } catch (e) {
    flash('#tmdbMeta', e.message);
  }
};

$('#testTmdb').onclick = async () => {
  flash('#tmdbMeta', '测试中…');
  try {
    const r = await api('/api/tmdb/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        tmdb_proxy: $('#tmdbProxy').value.trim(),
        tmdb_language: $('#tmdbLang').value.trim() || 'zh-CN',
        tmdb_api_key: $('#tmdb').value.trim(),
      }),
    });
    flash('#tmdbMeta', r.ok ? `通 · ${r.title || ''} (${r.took_ms}ms)` : (`失败 · ${r.error || ''}`));
  } catch (e) {
    flash('#tmdbMeta', e.message);
  }
};

$('#savePw').onclick = async () => {
  const p = $('#uiPw').value.trim();
  if (p.length < 4) { flash('#pwMeta', '密码至少 4 位'); return; }
  try {
    await api('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ access_password: p }) });
    $('#uiPw').value = '';
    flash('#pwMeta', '访问密码已更新');
  } catch (e) {
    flash('#pwMeta', e.message);
  }
};

const logoutA = $('#logout');
if (logoutA) {
  logoutA.onclick = async (e) => {
    e.preventDefault();
    await fetch('/api/ui/logout', { method: 'POST' });
    location.replace('/login.html');
  };
}

let pollTimer = null;
$('#loginLand').onclick = async () => {
  const box = $('#loginBox');
  box.hidden = false;
  box.textContent = '发起登录…';
  try {
    const data = await api('/api/2dland/login', { method: 'POST' });
    box.innerHTML = `请打开 <a href="${esc(data.verification_uri)}" target="_blank" rel="noopener">${esc(data.verification_uri)}</a>
      输入代码 <b>${esc(data.user_code)}</b>，完成后自动检测。`;
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(async () => {
      try {
        const st = await api('/api/2dland/poll');
        if (st.logged_in || st.status === 'AUTHORIZATION_SUCCESS') {
          clearInterval(pollTimer);
          box.textContent = '登录成功';
          refreshLand();
        }
      } catch {}
    }, Math.max(2000, (data.interval || 3) * 1000));
  } catch (e) {
    box.textContent = e.message;
  }
};

$('#logoutLand').onclick = async () => {
  await api('/api/2dland/logout', { method: 'POST' });
  refreshLand();
};

$('#refreshTasks').onclick = loadTasks;

async function loadTasks() {
  const box = $('#tasks');
  $('#taskMeta').textContent = '加载中…';
  try {
    const tasks = await api('/api/tasks');
    $('#taskMeta').textContent = `共 ${tasks.length || 0} 条`;
    if (!tasks.length) { box.innerHTML = '<p class="muted">暂无任务</p>'; return; }
    box.innerHTML = tasks.map(t => `<div class="task">
      <div><b>${esc(t.name || t.Name || t.identity || '')}</b>
        <span class="tag">${esc(t.status ?? t.Status ?? '')}</span>
        <span class="muted">${esc(t.save_path || t.SavePath || '')}</span></div>
      <div class="row">
        <button type="button" class="ghost org" data-path="${esc(t.save_path || t.SavePath || '')}">整理</button>
      </div>
    </div>`).join('');
    box.querySelectorAll('.org').forEach(btn => {
      btn.onclick = async () => {
        const p = btn.getAttribute('data-path');
        if (!p) return;
        btn.disabled = true;
        try {
          const r = await api('/api/tasks/organize', {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ save_path: p }),
          });
          flash('#taskMeta', `整理完成 改名${(r.renamed||[]).length} 删杂项${(r.deleted||[]).length}`);
        } catch (e) {
          flash('#taskMeta', e.message);
        } finally { btn.disabled = false; }
      };
    });
  } catch (e) {
    $('#taskMeta').textContent = e.message;
    box.innerHTML = '';
  }
}

function flash(sel, msg) { $(sel).textContent = msg; }

loadSettings().catch(e => { $('#landMeta').textContent = e.message; });
