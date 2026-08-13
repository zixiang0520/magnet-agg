const form = document.getElementById('login');
const pw = document.getElementById('pw');
const err = document.getElementById('err');
const hint = document.getElementById('hint');

async function session() {
  const res = await fetch('/api/ui/session');
  return res.json();
}

session().then(s => {
  if (s.logged_in) {
    location.replace('/');
    return;
  }
  if (s.setup_needed) {
    hint.textContent = '首次使用：设置访问密码（至少 4 位）';
    form.dataset.setup = '1';
  }
}).catch(() => {});

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  err.textContent = '';
  const password = pw.value.trim();
  if (password.length < 4) {
    err.textContent = '密码至少 4 位';
    return;
  }
  const setup = form.dataset.setup === '1';
  const url = setup ? '/api/ui/setup' : '/api/ui/login';
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || res.statusText);
    location.replace('/');
  } catch (e2) {
    err.textContent = e2.message;
  }
});
