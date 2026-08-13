const $ = s => document.querySelector(s);
const list = $('#list');
const meta = $('#meta');
const form = $('#f');
const input = $('#q');
const fab = $('#fab');
const selected = new Map();

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

async function copy(text) {
  try { await navigator.clipboard.writeText(text); flash('已复制磁力链'); }
  catch {
    const t = document.createElement('textarea'); t.value = text; document.body.appendChild(t); t.select();
    document.execCommand('copy'); t.remove(); flash('已复制');
  }
}

function flash(msg) {
  meta.innerHTML = `<span style="color:var(--ok)">${esc(msg)}</span>`;
}

const logoutBtn = $('#logout');
if (logoutBtn) {
  logoutBtn.addEventListener('click', async (e) => {
    e.preventDefault();
    await fetch('/api/ui/logout', { method: 'POST' });
    location.replace('/login.html');
  });
}

function renderFab() {
  if (!selected.size) { fab.hidden = true; fab.innerHTML = ''; return; }
  fab.hidden = false;
  fab.innerHTML = `<span>已选 ${selected.size} 条</span>
    <button type="button" id="btnPush">推送到 2dland</button>
    <button type="button" class="ghost" id="btnClear">清空</button>`;
  $('#btnClear').onclick = () => { selected.clear(); renderFab(); list.querySelectorAll('input[type=checkbox]').forEach(c => c.checked = false); };
  $('#btnPush').onclick = doPush;
}

async function doPush() {
  const btn = $('#btnPush');
  if (!btn) return;
  btn.disabled = true;
  const magnets = [...selected.values()].map(r => ({
    name: r.title, magnet: r.magnet, category: r.category || '', title: r.title,
  }));
  try {
    const res = await fetch('/api/push', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ magnets, query: input.value.trim() }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || res.statusText);
    const ok = (data.items || []).filter(i => i.ok).length;
    const fail = (data.items || []).length - ok;
    flash(`已推送 ${ok} 条` + (fail ? `，失败 ${fail}` : '') + '（下载完成后自动 TMDB/AI 整理）');
    selected.clear();
    renderFab();
  } catch (e) {
    flash('推送失败：' + e.message);
  } finally {
    btn.disabled = false;
  }
}

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  const q = input.value.trim();
  if (!q) return;
  list.innerHTML = '';
  selected.clear();
  renderFab();
  meta.textContent = '搜索中（多源并发）…';
  form.querySelector('button').disabled = true;
  try {
    const res = await fetch('/api/search?q=' + encodeURIComponent(q));
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || res.statusText);
    const src = data.sources ? Object.entries(data.sources).map(([k,v]) => `${k}:${v}`).join(' · ') : '';
    const errs = data.errors ? Object.entries(data.errors).map(([k,v]) => `${k}失败`).join(' · ') : '';
    meta.innerHTML = `共 <b>${data.total}</b> 条 · ${data.took_ms}ms` +
      (src ? ` · 源命中 ${esc(src)}` : '') +
      (errs ? ` · <span class="err">${esc(errs)}</span>` : '');
    if (!data.results || !data.results.length) {
      list.innerHTML = '<p class="sub">无结果，换个关键词试试</p>';
      return;
    }
    list.innerHTML = data.results.map((r, i) => `
      <article class="card">
        <label class="pick"><input type="checkbox" data-i="${i}" /> 选</label>
        <h3>${esc(r.title)}</h3>
        <div class="row">
          <span class="tag src">${esc(r.source)}</span>
          ${r.size ? `<span class="tag">${esc(r.size)}</span>` : ''}
          ${r.seeders ? `<span class="tag seed">↑${r.seeders}</span>` : ''}
          ${r.category ? `<span class="tag">${esc(r.category)}</span>` : ''}
          <div class="actions">
            <button type="button" data-magnet="${esc(r.magnet)}">复制磁力</button>
            <button type="button" class="push-one" data-i="${i}">推送</button>
            ${r.page_url ? `<a href="${esc(r.page_url)}" target="_blank" rel="noopener">来源</a>` : ''}
          </div>
        </div>
      </article>`).join('');
    list.querySelectorAll('button[data-magnet]').forEach(btn => {
      btn.addEventListener('click', () => copy(btn.getAttribute('data-magnet')));
    });
    list.querySelectorAll('input[type=checkbox]').forEach(chk => {
      chk.addEventListener('change', () => {
        const i = +chk.dataset.i;
        const r = data.results[i];
        if (chk.checked) selected.set(i, r); else selected.delete(i);
        renderFab();
      });
    });
    list.querySelectorAll('.push-one').forEach(btn => {
      btn.addEventListener('click', async () => {
        const r = data.results[+btn.dataset.i];
        selected.clear();
        selected.set(+btn.dataset.i, r);
        renderFab();
        await doPush();
      });
    });
  } catch (err) {
    meta.innerHTML = `<span class="err">失败：${esc(err.message)}</span>`;
  } finally {
    form.querySelector('button').disabled = false;
  }
});
