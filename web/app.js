const $ = s => document.querySelector(s);
const list = $('#list');
const meta = $('#meta');
const form = $('#f');
const input = $('#q');

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

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  const q = input.value.trim();
  if (!q) return;
  list.innerHTML = '';
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
    list.innerHTML = data.results.map(r => `
      <article class="card">
        <h3>${esc(r.title)}</h3>
        <div class="row">
          <span class="tag src">${esc(r.source)}</span>
          ${r.size ? `<span class="tag">${esc(r.size)}</span>` : ''}
          ${r.seeders ? `<span class="tag seed">↑${r.seeders}</span>` : ''}
          ${r.category ? `<span class="tag">${esc(r.category)}</span>` : ''}
          <div class="actions">
            <button type="button" data-magnet="${esc(r.magnet)}">复制磁力</button>
            ${r.page_url ? `<a href="${esc(r.page_url)}" target="_blank" rel="noopener">来源</a>` : ''}
          </div>
        </div>
      </article>`).join('');
    list.querySelectorAll('button[data-magnet]').forEach(btn => {
      btn.addEventListener('click', () => copy(btn.getAttribute('data-magnet')));
    });
  } catch (err) {
    meta.innerHTML = `<span class="err">失败：${esc(err.message)}</span>`;
  } finally {
    form.querySelector('button').disabled = false;
  }
});
