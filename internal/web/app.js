'use strict';
const $ = id => document.getElementById(id);
const adminPage = document.body.dataset.page === 'admin';
function on(id, event, action) { $(id)?.addEventListener(event, action); }
let config = {}, me = null, refreshBusy = false;
let proxyNodes = [];
function show(id, visible = true) { const e = $(id); if (e) e.hidden = !visible; }
function el(tag, text, cls) { const e = document.createElement(tag); if (text !== undefined) e.textContent = text; if (cls) e.className = cls; return e; }
function notice(message, error = false) { const target = $('notice'); target.textContent = message; target.className = error ? 'error' : ''; target.hidden = false; }
function button(text, action, cls = 'secondary') { const b = el('button', text, cls); b.type = 'button'; b.addEventListener('click', async () => { b.disabled = true; try { await action(); } catch (e) { notice(e.message, true); } finally { b.disabled = false; } }); return b; }
async function api(path, method = 'GET', body) {
  const r = await fetch(path, { method, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-PortBridge-Request': '1' }, body: body === undefined ? undefined : JSON.stringify(body) });
  let data; try { data = await r.json(); } catch { throw Error('服务响应异常，请检查连接'); }
  if (!r.ok) { const e = Error(data.error || '请求失败'); e.status = r.status; throw e; } return data;
}
function form(id, fn) {
  if (!$(id)) return;
  $(id).addEventListener('submit', async e => {
    e.preventDefault(); const controls = [...e.target.querySelectorAll('button')]; controls.forEach(b => b.disabled = true);
    try { await fn(new FormData(e.target), e.target); } catch (err) { notice(err.message, true); } finally { controls.forEach(b => b.disabled = false); }
  });
}
async function copy(value) {
  try { await navigator.clipboard.writeText(value); notice('已复制'); } catch { notice('复制失败，请手动复制', true); }
}
function date(value) { return value ? new Date(value).toLocaleString() : '—'; }

async function boot() {
  config = await api('/api/config'); show('loading', false);
  if (config.mode === 'node') { if (adminPage) { location.replace('/'); return; } show('node-page'); await refreshNode(); return; }
  if ($('allow-register')) $('allow-register').checked = config.allow_register;
  show('register-button', config.allow_register);
  try { me = await api('/api/me'); await dashboard(); } catch (e) { if (e.status === 401) { show('auth'); } else { throw e; } }
}
async function dashboard() {
  if (adminPage && !me.admin) { location.replace('/'); return; }
  show('auth', false); show('dashboard', !adminPage); show('admin-panel', adminPage);
  const toolbar = $('toolbar'); toolbar.replaceChildren(el('span', `${me.username}${me.admin ? ' · 管理员' : ''}`));
  if (me.admin) {
    const link = el('a', adminPage ? '转发服务' : '管理页面', 'toolbar-link'); link.href = adminPage ? '/' : '/admin'; toolbar.append(link);
  }
  toolbar.append(button('修改密码', changePassword), button('退出', logout));
  if (adminPage) { activateTab(location.hash.slice(1)); await Promise.all([refreshAdminNodes(), refreshUsers(), refreshAudit()]); } else { show('proxy-pane', me.admin); show('readonly-nodes', !me.admin); if (me.admin) await refreshProxies(); else await refreshNodes(); }
}
async function logout() { await api('/api/logout', 'POST', {}); me = null; location.reload(); }
function listFocus(content) {
  const active = document.activeElement, row = active.closest('[data-node-id], [data-proxy-id]');
  if (!row || !content.contains(active)) return null;
  const key = row.dataset.proxyId ? 'proxyId' : 'nodeId';
  return { key, id: row.dataset[key], index: [...row.querySelectorAll('button, summary, a')].indexOf(active) };
}
function restoreListFocus(content, focus) {
  if (!focus || document.activeElement !== document.body) return;
  const row = [...content.querySelectorAll('[data-node-id], [data-proxy-id]')].find(row => row.dataset[focus.key] === focus.id);
  row?.querySelectorAll('button, summary, a')[focus.index]?.focus({ preventScroll: true });
}
async function renderNodes(id) {
  const saved = await api('/api/nodes'), content = $(id), focus = listFocus(content); content.replaceChildren();
  if ($('node-summary')) $('node-summary').textContent = `${saved.filter(n => n.online).length} / ${saved.length} 在线`;
  if (!saved.length) content.append(el('p', me.admin ? '还没有节点。点击“接入服务器”，生成配对链接。' : '还没有节点。', 'empty-state muted'));
  for (const n of saved) {
    const row = el('article', undefined, 'admin-node-row'), info = el('div', undefined, 'node-info'), title = el('div', undefined, 'node-title'); row.dataset.nodeId = n.id;
    title.append(el('h3', n.name), el('span', n.online ? '在线' : '离线', `badge ${n.online ? 'good' : ''}`));
    const ports = n.status?.tcp_ports || [];
    info.append(title, el('p', `${ports.length} 个授权端口${n.status?.tcp_paused ? ' · 本机转发已暂停' : ''}`, 'muted')); row.append(info);
    if (me.admin) row.append(button('移除节点', async () => {
      if (prompt(`输入「${n.name}」确认移除。该节点的全部转发将删除，对应连接将断开。`) !== n.name) return;
      await api(`/api/nodes/${encodeURIComponent(n.id)}`, 'DELETE', {}); notice('节点已移除'); await refreshAdminNodes();
    }, 'secondary danger destructive-action'));
    content.append(row);
  }
  restoreListFocus(content, focus);
}
async function refreshNodes() { await renderNodes('nodes'); }
async function refreshAdminNodes() { await renderNodes('admin-nodes'); }
async function refreshNode() {
  const s = await api('/api/node'); const content = $('local-status'); content.replaceChildren();
  content.append(el('span', s.online ? '已连接' : s.paired ? '正在连接控制台' : '等待配对', `badge ${s.online ? 'good' : ''}`));
  if (s.hub_url) content.append(el('p', s.hub_url, 'muted'));
  if (s.error) content.append(el('p', s.error, 'muted'));
  show('pair-section', !s.paired);
  const ports = $('tcp-ports'); ports.replaceChildren();
  if (!(s.tcp_ports || []).length) ports.append(el('p', '尚未授权 TCP 服务端口。', 'muted'));
  for (const port of s.tcp_ports || []) {
    const row = el('div', undefined, 'allowed-port-row');
    row.append(el('code', `127.0.0.1:${port}`), button('撤销授权', async () => {
      if (!confirm(`撤销端口 ${port} 的授权？该端口的转发连接将断开。`)) return;
      await api(`/api/tcp-ports/${port}`, 'DELETE', {}); notice('端口授权已撤销'); await refreshNode();
    }, 'secondary danger')); ports.append(row);
  }
  forwardingControl('local-tcp-forwarding', s.tcp_paused, '/api/tcp-forwarding', '这台节点', refreshNode, 'TCP');
}
async function toggleForwarding(path, paused, scope, kind = 'TCP') {
  if (paused && !confirm(`暂停${scope}的 ${kind} 转发？对应的连接和文件传输将断开。`)) return;
  await api(path, 'PUT', { paused });
  notice(paused ? `${kind} 转发已暂停` : '已恢复此处的转发设置');
}
function forwardingControl(id, paused, path, scope, refresh, kind = 'TCP') {
  $(id).replaceChildren(el('span', paused ? `${kind} 转发已暂停` : `${kind} 转发已启用`, `badge ${paused ? '' : 'good'}`), button(paused ? `恢复 ${kind} 转发` : `暂停 ${kind} 转发`, async () => { await toggleForwarding(path, !paused, scope, kind); await refresh(); }));
}
function proxyTargetOptions() {
  const form = $('proxy-form'), target = form.elements.target_port, selected = target.value;
  const node = proxyNodes.find(n => n.id === form.elements.node.value);
  const ports = node?.status?.tcp_ports || [];
  target.replaceChildren();
  if (!ports.length) { const option = el('option', '请先在节点授权端口'); option.value = ''; target.append(option); }
  for (const p of ports) {
    const option = el('option', `${p.port}${p.ready ? '' : '（本机服务未就绪）'}`); option.value = String(p.port); target.append(option);
  }
  if (ports.some(p => String(p.port) === selected)) target.value = selected;
}
async function refreshProxies() {
  const [result, savedNodes] = await Promise.all([api('/api/proxies'), api('/api/nodes')]);
  proxyNodes = savedNodes;
  const form = $('proxy-form'), select = form.elements.node, selected = select.value;
  const key = JSON.stringify(savedNodes.map(n => [n.id, n.name, n.status?.tcp_ports]));
  if (select.dataset.nodes !== key) {
    select.replaceChildren();
    if (!savedNodes.length) { const option = el('option', '请先接入服务器'); option.value = ''; select.append(option); }
    for (const n of savedNodes) { const option = el('option', n.name); option.value = n.id; select.append(option); }
    if (savedNodes.some(n => n.id === selected)) select.value = selected;
    proxyTargetOptions(); select.dataset.nodes = key;
  }
  form.elements.port.min = result.port_start; form.elements.port.max = result.port_end;
  $('proxy-port-range').textContent = `公网端口范围 ${result.port_start}–${result.port_end}，留空自动分配。VPS 防火墙需放行所分配端口。域名与 HTTPS 可后续配置。`;
  $('proxy-summary').textContent = `${result.items.length} 个转发服务`;
  const content = $('proxies'), focus = listFocus(content); content.replaceChildren();
  if (!result.items.length) content.append(el('p', '还没有转发。先在管理页面接入服务器，再创建第一个服务。', 'empty-state muted'));
  for (const p of result.items) {
    const row = el('article', undefined, 'proxy-row'), head = el('div', undefined, 'node-head'), info = el('div', undefined, 'node-info'), title = el('div', undefined, 'node-title');
    row.dataset.proxyId = p.id;
    const status = p.effective_paused ? '已暂停' : !p.online ? '节点离线' : !p.allowed ? '端口未授权' : p.ready ? '转发就绪' : '服务未就绪';
    title.append(el('h3', p.name), el('span', 'TCP', 'proxy-kind'), el('span', status, `badge ${p.ready ? 'good' : ''}`));
    const address = `${p.host.includes(':') ? '[' + p.host + ']' : p.host}:${p.port}`;
    const value = p.kind === 'tcp' && p.scheme !== 'tcp' ? `${p.scheme}://${address}` : address;
    info.append(title, el('p', `${value} → ${p.node_name} · ${'127.0.0.1:' + p.target_port}`, 'node-address'));
    const actions = el('div', undefined, 'actions');
    actions.append(button('复制地址', () => copy(value)));
    if (p.kind === 'tcp' && p.scheme !== 'tcp') {
      const link = el('a', '打开', 'toolbar-link'); link.href = value; link.target = '_blank'; link.rel = 'noopener noreferrer'; actions.append(link);
    }
    const retry = p.kind === 'tcp' && Boolean(p.error) && !p.paused;
    actions.append(button(retry ? '重试监听' : p.paused ? '恢复转发' : '暂停转发', async () => {
      const path = `/api/proxies/${encodeURIComponent(p.id)}/forwarding`;
      if (retry) await api(path, 'PUT', { paused: false });
      else await toggleForwarding(path, !p.paused, p.name, 'TCP');
      await refreshProxies();
    }));
    if (p.kind === 'tcp') actions.append(button('删除代理', async () => {
      if (prompt(`输入「${p.name}」确认删除代理。对应连接将断开，本机服务继续运行。`) !== p.name) return;
      await api(`/api/proxies/${encodeURIComponent(p.id)}`, 'DELETE', {}); notice('代理已删除，公网端口已回收'); await refreshProxies();
    }, 'secondary danger destructive-action'));
    head.append(info, actions); row.append(head);
    if (p.error) row.append(el('p', p.error, 'muted'));
    if (p.effective_paused && !p.paused) row.append(el('p', '节点本机 TCP 转发已暂停，请在节点维护页恢复。', 'muted'));
    content.append(row);
  }
  restoreListFocus(content, focus);
}
async function refreshUsers() {
  const users = await api('/api/users'); $('users').replaceChildren();
  for (const user of users) {
    const row = el('tr'); row.append(el('td', user.username), el('td', user.admin ? '管理员' : '普通用户'));
    const cell = el('td'), actions = el('div', undefined, 'actions');
    if (user.username !== me.username) {
      actions.append(button(user.admin ? '设为普通用户' : '设为管理员', async () => { await api(`/api/users/${encodeURIComponent(user.username)}`, 'PATCH', { admin: !user.admin }); await refreshUsers(); }));
      actions.append(button('重置密码', async () => { const result = await passwordDialog(`重置 ${user.username} 的密码`, false); if (!result) return; await api(`/api/users/${encodeURIComponent(user.username)}`, 'PATCH', { password: result.password }); notice('密码已重置，原有登录已失效'); }));
      actions.append(button('删除', async () => { if (!confirm(`删除控制台账号 ${user.username}？`)) return; await api(`/api/users/${encodeURIComponent(user.username)}`, 'DELETE', {}); await refreshUsers(); }, 'secondary danger'));
    } else actions.append(el('span', '当前账号', 'muted'));
    cell.append(actions); row.append(cell); $('users').append(row);
  }
}
async function refreshAudit() {
  const audit = await api('/api/audit'); $('audit').replaceChildren();
  for (const entry of audit.slice(-50).reverse()) $('audit').append(el('p', `${date(entry.at)} · ${entry.actor} · ${entry.action}${entry.target ? ' · ' + entry.target : ''}`));
}
function passwordDialog(title, current) {
  return new Promise(resolve => {
    const dialog = el('dialog', undefined, 'card'), f = el('form'); f.className = 'row'; dialog.append(el('h2', title), f);
    const fields = {};
    for (const [name, label] of [...(current ? [['current', '当前密码']] : []), ['password', '新密码（至少 10 字节）']]) {
      const l = el('label', label), input = el('input'); input.type = 'password'; input.required = true; input.maxLength = 72; input.autocomplete = name === 'current' ? 'current-password' : 'new-password'; if (name === 'password') input.minLength = 10; fields[name] = input; l.append(input); f.append(l);
    }
    const submit = el('button', '保存'); submit.type = 'submit'; f.append(submit, button('取消', () => dialog.close()));
    let value = null; f.addEventListener('submit', e => { e.preventDefault(); value = Object.fromEntries(Object.entries(fields).map(([k, v]) => [k, v.value])); dialog.close(); });
    dialog.addEventListener('close', () => { dialog.remove(); resolve(value); }, { once: true }); document.body.append(dialog); dialog.showModal();
  });
}
async function changePassword() {
  const p = await passwordDialog('修改登录密码', true); if (!p) return; await api('/api/password', 'PUT', p); me = null; location.reload();
}
form('login-form', async data => { me = await api('/api/login', 'POST', { username: data.get('username'), password: data.get('password') }); $('login-form').elements.password.value = ''; show('notice', false); if (adminPage) { location.assign('/'); return; } await dashboard(); });
on('register-button', 'click', async () => {
  const f = $('login-form'); if (!f.reportValidity()) return;
  try { await api('/api/register', 'POST', { username: f.elements.username.value, password: f.elements.password.value }); notice('账号已创建，请登录'); f.elements.password.value = ''; } catch (e) { notice(e.message, true); }
});
form('pair-form', async (data, f) => { await api('/api/pair', 'POST', { link: data.get('link').trim() }); f.reset(); notice('配对成功'); await refreshNode(); });
form('invite-form', async data => { const result = await api('/api/invites', 'POST', { name: data.get('name') }); $('pair-link').value = result.link; $('invite-expiry').textContent = `截止 ${date(result.expires)} · 单次使用`; show('invite-result'); });
form('user-form', async (data, f) => { await api('/api/users', 'POST', { username: data.get('username'), password: data.get('password'), admin: data.has('admin') }); f.reset(); notice('账号已创建'); await refreshUsers(); });
on('copy-link', 'click', () => copy($('pair-link').value));
on('audit-refresh', 'click', () => refreshAudit().catch(e => notice(e.message, true)));
on('allow-register', 'change', async e => {
  try { await api('/api/settings', 'PUT', { allow_register: e.target.checked }); notice('注册设置已保存'); } catch (err) { e.target.checked = !e.target.checked; notice(err.message, true); }
});
function activateTab(name, updateURL = false) {
  const tabs = [...document.querySelectorAll('[role="tab"][data-tab]')];
  if (!tabs.length) return;
  if (!tabs.some(tab => tab.dataset.tab === name)) name = 'servers';
  for (const tab of tabs) {
    const selected = tab.dataset.tab === name;
    tab.setAttribute('aria-selected', String(selected)); tab.tabIndex = selected ? 0 : -1;
    show(`panel-${tab.dataset.tab}`, selected);
  }
  if (updateURL) history.replaceState(null, '', `#${name}`);
}
for (const tab of document.querySelectorAll('[role="tab"][data-tab]')) {
  tab.addEventListener('click', () => activateTab(tab.dataset.tab, true));
  tab.addEventListener('keydown', event => {
    const tabs = [...document.querySelectorAll('[role="tab"][data-tab]')], index = tabs.indexOf(tab);
    let next;
    if (event.key === 'ArrowRight') next = tabs[(index + 1) % tabs.length];
    if (event.key === 'ArrowLeft') next = tabs[(index - 1 + tabs.length) % tabs.length];
    if (event.key === 'Home') next = tabs[0];
    if (event.key === 'End') next = tabs[tabs.length - 1];
    if (next) { event.preventDefault(); activateTab(next.dataset.tab, true); next.focus(); }
  });
}
window.addEventListener('hashchange', () => { if (adminPage) activateTab(location.hash.slice(1)); });
function toggleInline(buttonId, sectionId, inputName) {
  const section = $(sectionId), control = $(buttonId); section.hidden = !section.hidden;
  control.setAttribute('aria-expanded', String(!section.hidden));
  if (!section.hidden) section.querySelector(`[name="${inputName}"]`).focus();
}
on('invite-toggle', 'click', () => toggleInline('invite-toggle', 'invite-section', 'name'));
on('proxy-toggle', 'click', () => toggleInline('proxy-toggle', 'proxy-section', 'name'));
on('proxy-form', 'change', e => { if (e.target.name === 'node') proxyTargetOptions(); });
form('proxy-form', async (data, f) => {
  await api('/api/proxies', 'POST', { name: data.get('name'), node_id: data.get('node'), target_port: Number(data.get('target_port')), port: Number(data.get('port')) || 0, scheme: data.get('scheme') });
  f.reset(); proxyTargetOptions(); show('proxy-section', false); $('proxy-toggle').setAttribute('aria-expanded', 'false'); notice('端口转发已创建'); await refreshProxies();
});
form('tcp-port-form', async (data, f) => { await api('/api/tcp-ports', 'POST', { port: Number(data.get('port')) }); f.reset(); notice('端口已授权，可在 VPS 控制台创建转发'); await refreshNode(); });
on('user-toggle', 'click', () => toggleInline('user-toggle', 'user-section', 'username'));
setInterval(async () => {
  if (refreshBusy || document.hidden) return; refreshBusy = true;
  try { if (config.mode === 'node') await refreshNode(); else if (me) { if (adminPage) await refreshAdminNodes(); else if (me.admin) await refreshProxies(); else await refreshNodes(); } } catch (e) { if (e.status === 401) { me = null; show('dashboard', false); show('admin-panel', false); show('auth'); $('toolbar').replaceChildren(); } else notice(e.message, true); } finally { refreshBusy = false; }
}, 5000);
boot().catch(e => { show('loading', false); notice(e.message, true); });
