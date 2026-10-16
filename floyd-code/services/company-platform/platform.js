'use strict';
const main = document.querySelector('#main');
let csrf = '';
let noticeTimer;
const el = (tag, text, cls) => { const n = document.createElement(tag); if (text !== undefined)
    n.textContent = text; if (cls)
    n.className = cls; return n; };
const add = (parent, ...children) => { parent.append(...children.filter(Boolean)); return parent; };
const link = (text, href, cls) => { const n = el('a', text, cls); n.href = href; return n; };
function tell(text) { const n = document.querySelector('#notice'); n.textContent = text; clearTimeout(noticeTimer); noticeTimer = setTimeout(() => { n.textContent = ''; }, 10000); }
function button(text, action, cls) { const n = el('button', text, cls); n.type = 'button'; n.onclick = async () => { n.disabled = true; try {
    await action();
}
catch (error) {
    tell(error.message);
}
finally {
    n.disabled = false;
} }; return n; }
function field(parent, text, type = 'text', value = '') { const id = 'f-' + crypto.randomUUID(); const l = el('label', text); l.htmlFor = id; const n = el('input'); n.id = id; n.type = type; n.value = value; add(parent, l, n); return n; }
function section(title, parent = main) { const s = el('section'); add(s, el('h2', title)); parent.append(s); return s; }
function errorBox(parent, text) { parent.append(el('p', text, 'error')); }
async function api(path, method = 'GET', body) { let response; try {
    const options = { method, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }, signal: AbortSignal.timeout(45000) };
    if (body !== undefined && method !== 'GET' && method !== 'HEAD') options.body = JSON.stringify(body);
    response = await fetch(path, options);
}
catch {
    throw new Error('The connection was lost. Your change has not been confirmed. Reload the page before trying again.');
} const data = await response.json(); if (!response.ok) {
    const e = new Error(data.error_description || 'This action could not be completed.');
    e.status = response.status;
    throw e;
} return data; }
async function copy(value) { if (navigator.clipboard) {
    await navigator.clipboard.writeText(value);
    tell('Copied.');
}
else {
    tell('Select the value and copy it.');
} }
function date(value) { if (!value)
    return 'Not yet'; const d = new Date(typeof value === 'number' ? value * 1000 : value); return Number.isNaN(d.valueOf()) ? 'Not listed' : d.toLocaleString(); }
function safeLink(value) { try {
    const u = new URL(value);
    return ['https:', 'http:'].includes(u.protocol) ? u.href : null;
}
catch {
    return null;
} }
function price(value) { return typeof value === 'number' && Number.isFinite(value) ? '$' + value.toLocaleString(undefined, { maximumFractionDigits: 6 }) : 'Not listed'; }
function readonly(parent, name, value) { const f = field(parent, name, 'text', value); f.readOnly = true; add(parent, button('Copy', () => copy(value), 'secondary')); }
function title(text, description) { main.replaceChildren(el('h1', text), el('p', description)); }
async function adminLogin() { title('Administration', 'Sign in with your administrator key.'); const s = section('Your key'); s.classList.add('narrow'); const form = el('form'); const key = field(form, 'Administrator key', 'password'); key.autocomplete = 'current-password'; const submit = el('button', 'Sign in'); submit.type = 'submit'; add(form, submit); form.onsubmit = async (e) => { e.preventDefault(); submit.disabled = true; try {
    await api('/admin/api/login', 'POST', { key: key.value });
    key.value = '';
    const destination = new URLSearchParams(location.search).get('return_to') || '/admin';
    location.assign(destination.startsWith('/approve?') ? destination : '/admin');
}
catch (error) {
    tell(error.message);
}
finally {
    submit.disabled = false;
} }; s.append(form); }
async function home() { title('Your coding platform', 'Use Floyd Code on your own computer, with your own files and settings. Shared company services live on inference.'); const s = section('Get started'); add(s, link('Install Floyd Code', 'https://inference.tail58d565.ts.net:8444/code', 'button'), el('p', 'Already have an account? Sign in to approve devices and manage your access.'), link('Open my account', '/account', 'button secondary')); const g = el('div', undefined, 'grid'); add(g, add(el('div', undefined, 'card'), el('h3', 'Provider choice'), el('p', 'Use your own model provider, or company-provided models.')), add(el('div', undefined, 'card'), el('h3', 'Personal work'), el('p', 'Your projects and saved conversations belong in your own computer account.'))); main.append(g); }
async function loadDirectory(parent, owner = false) { const s = section('Providers and models', parent); const status = el('p', 'Loading the saved directory…'); s.append(status); if (owner)
    add(s, button('Refresh provider information', async () => { await api('/admin/api/catalog/refresh', 'POST', {}); await renderDirectory(); tell('Provider information refreshed.'); })); const controls = el('div'); const content = el('div'); add(s, controls, content); async function renderDirectory() { try {
    const [info, providers] = await Promise.all([api('/catalog/info'), api('/catalog/api.json')]);
    status.textContent = `Updated ${date(info.updated_at)} · ${Object.keys(providers).length} providers` + (info.stale ? ' · An update is due' : '');
    if (info.last_error)
        status.textContent += ' · The last refresh failed; this is the last saved copy.';
    controls.replaceChildren();
    const search = field(controls, 'Find a provider or model');
    const label = el('label', 'Provider');
    label.htmlFor = 'provider-choice';
    const select = el('select');
    select.id = 'provider-choice';
    add(controls, label, select);
    const entries = Object.entries(providers).toSorted((a, b) => (a[1].name || a[0]).localeCompare(b[1].name || b[0]));
    function choices() { const previous = select.value; select.replaceChildren(); const term = search.value.toLowerCase().trim(); for (const [id, p] of entries) {
        if (term && !JSON.stringify({ id, name: p.name, models: p.models }).toLowerCase().includes(term))
            continue;
        const option = el('option', `${p.name || id} (${Object.keys(p.models || {}).length})`);
        option.value = id;
        select.append(option);
    } if ([...select.options].some(o => o.value === previous))
        select.value = previous; showProvider(); }
    function showProvider() { content.replaceChildren(); const p = providers[select.value]; if (!p) {
        content.append(el('p', 'No matching providers.'));
        return;
    } add(content, el('h3', p.name || select.value, 'spaced')); if (p.api)
        readonly(content, 'Provider address', p.api);
    else
        content.append(el('p', 'The provider does not list one shared address. Use its setup guide.')); const doc = safeLink(p.doc); if (doc)
        content.append(link('Provider setup and pricing source', doc)); content.append(el('p', 'Listed prices are dollars per million text pieces. A piece is a short word or part of a word. Your provider sets the final bill.', 'small')); const wrapper = el('div', undefined, 'scroll'); const table = el('table'); const head = el('tr'); for (const name of ['Model', 'Text sent', 'Text received', 'Size / Updated'])
        head.append(el('th', name)); const thead = el('thead'); thead.append(head); const tbody = el('tbody'); add(table, thead, tbody); wrapper.append(table); content.append(wrapper); const term = search.value.toLowerCase().trim(); let models = Object.entries(p.models || {}); if (term && !(p.name || select.value).toLowerCase().includes(term))
        models = models.filter(([id, m]) => `${id} ${m.name || ''}`.toLowerCase().includes(term)); let shown = 0; const more = button('Show more models', next, 'secondary'); function next() { for (const [id, m] of models.slice(shown, shown + 100)) {
        const row = el('tr');
        const name = el('td');
        add(name, el('strong', m.name || id), el('br'), el('code', id));
        if (m.status)
            name.append(el('p', m.status === 'deprecated' ? 'Older model' : m.status, 'tag'));
        if (m.kind && m.kind !== 'coding') name.append(el('p', m.kind === 'embedding' ? 'Find matching text' : m.kind === 'speech' ? 'Speech' : m.kind === 'music' ? 'Music' : m.kind, 'tag'));
        const details = el('td');
        add(details, el('div', m.limit?.context ? Number(m.limit.context).toLocaleString() + ' text pieces' : 'Size not listed'), el('div', m.last_updated || 'Update date not listed', 'small'));
        const inputPrice = el('td', price(m.cost?.input));
        if (m.cost?.cache_read !== undefined) inputPrice.append(el('p', 'Reused text: ' + price(m.cost.cache_read), 'small'));
        if (m.cost?.cache_write !== undefined) inputPrice.append(el('p', 'Saving text: ' + price(m.cost.cache_write), 'small'));
        add(row, name, inputPrice, el('td', price(m.cost?.output)), details);
        tbody.append(row);
    } shown += 100; more.hidden = shown >= models.length; } next(); content.append(more); }
    select.onchange = showProvider;
    search.oninput = choices;
    choices();
}
catch (error) {
    status.textContent = error.message;
    status.classList.add('error');
} } await renderDirectory(); }
async function admin() { let overview; try {
    overview = await api('/admin/api/overview');
}
catch (error) {
    if (error.status === 401) {
        location.assign('/admin/login');
        return;
    }
    throw error;
} csrf = overview.csrf; title('Company administration', 'Manage accounts, current provider information, and your administrator key.'); const g = el('div', undefined, 'grid'); for (const [label, value] of [['Enabled accounts', overview.accounts.enabled], ['Waiting devices', overview.pending_devices], ['Providers', overview.catalog.provider_count || 0]])
    add(g, add(el('div', undefined, 'card'), el('div', value, 'stat'), el('div', label))); main.append(g); await accounts(); await connections(); await sharedTools(); const keys = section('Administrator key'); add(keys, el('p', 'Your key is stored as a protected check value. Replacing it cancels the old key and all other administrator sign-ins.'), button('Prepare a replacement key', async () => { keys.querySelector('.key-box')?.remove(); const box = el('section', undefined, 'key-box'); const bytes = crypto.getRandomValues(new Uint8Array(40)); const key = 'fa_' + Array.from(bytes, b => b.toString(16).padStart(2, '0')).join(''); readonly(box, 'New administrator key — copy it before applying', key); const saved = field(box, 'I have saved the new key', 'checkbox'); add(box, button('Apply this key', async () => { if (!saved.checked) {
    tell('Save the new key first so you keep access.');
    return;
} const result = await api('/admin/api/key/rotate', 'POST', { new_key: key }); csrf = result.csrf; tell('The new key works now. The old key has been cancelled.'); box.append(el('p', 'Replacement applied. Keep your saved key private.', 'good')); }, 'danger')); keys.append(box); }, 'secondary')); const audit = section('Recent changes'); const list = el('ul'); for (const item of overview.audit)
    list.append(el('li', `${date(item.created_at)} — ${item.action}${item.target ? ': ' + item.target : ''}`)); audit.append(list); const directoryArea = el('details'); directoryArea.append(el('summary', 'Browse all providers and model prices')); main.append(directoryArea); await loadDirectory(directoryArea, true); main.append(button('Sign out of administration', async () => { await api('/admin/api/logout', 'POST', {}); location.assign('/admin/login'); }, 'secondary')); }
async function accounts() { const s = section('Accounts'); const users = (await api('/admin/api/users')).users; const form = el('form'); const name = field(form, 'New account name'); name.required = true; name.maxLength = 64; const submit = el('button', 'Create invitation'); submit.type = 'submit'; form.append(submit); const inviteArea = el('div'); form.onsubmit = async (e) => { e.preventDefault(); submit.disabled = true; try {
    const invitation = await api('/admin/api/invites', 'POST', { username: name.value });
    showInvite(invitation);
    name.value = '';
    await listUsers();
}
catch (error) {
    tell(error.message);
}
finally {
    submit.disabled = false;
} }; add(s, form, inviteArea); function showInvite(item) { inviteArea.replaceChildren(el('p', 'Send this private link to the intended member. It expires in two days.')); readonly(inviteArea, 'Invitation link', item.url); } const area = el('div', undefined, 'scroll'); s.append(area); async function listUsers() { const data = (await api('/admin/api/users')).users; const table = el('table'); const head = el('tr'); for (const text of ['Account', 'Access', 'Last sign-in', 'Actions'])
    head.append(el('th', text)); const thead = el('thead'); thead.append(head); const tbody = el('tbody'); for (const user of data) {
    const actions = el('td');
    add(actions, button(user.enabled ? 'Switch off' : 'Switch on', async () => { await api(`/admin/api/users/${encodeURIComponent(user.id)}/${user.enabled ? 'disable' : 'enable'}`, 'POST', {}); await listUsers(); tell('Account updated.'); }, 'secondary'), button('New invitation', async () => showInvite(await api('/admin/api/invites', 'POST', { user_id: user.id })), 'secondary'));
    const row = el('tr');
    add(row, el('td', user.username), el('td', user.enabled ? 'Enabled' : 'Disabled'), el('td', date(user.last_login_at)), actions);
    tbody.append(row);
} add(table, thead, tbody); area.replaceChildren(table); } await listUsers(); const devices = (await api('/admin/api/devices?status=pending')).devices; if (devices.length > 0) {
    const d = el('section');
    d.append(el('h3', 'Waiting devices'));
    for (const device of devices) {
        const row = el('div', undefined, 'row');
        const choose = el('select');
        for (const user of users.filter(u => u.enabled)) {
            const option = el('option', user.username);
            option.value = user.id;
            choose.append(option);
        }
        add(row, el('code', device.user_code), choose, button('Approve', async () => { await api(`/admin/api/devices/${encodeURIComponent(device.device_code)}/approve`, 'POST', { user_id: choose.value }); row.remove(); tell('Device approved.'); }), button('Deny', async () => { await api(`/admin/api/devices/${encodeURIComponent(device.device_code)}/deny`, 'POST', {}); row.remove(); }, 'secondary'));
        d.append(row);
    }
    s.append(d);
} }
async function connections() { const s = section('Company model connections'); try {
    const items = (await api('/admin/api/connections')).providers;
    for (const item of items) {
        const row = el('div', undefined, 'card');
        add(row, el('h3', item.name), el('p', item.base_url), el('p', `${Object.keys(item.models || {}).length} models · ${item.enabled ? 'Enabled' : 'Disabled'}`), button('Refresh models', async () => { await api(`/admin/api/connections/${encodeURIComponent(item.id)}/refresh`, 'POST', {}); tell('Model list refreshed. Reload to see the new list.'); }, 'secondary'), button(item.enabled ? 'Switch off' : 'Switch on', async () => { await api(`/admin/api/connections/${encodeURIComponent(item.id)}`, 'PUT', { enabled: !item.enabled }); tell('Connection updated. Reload to see its new state.'); }, 'secondary'));
        const edit = el('details'); edit.append(el('summary', 'Edit this connection'));
        const editForm = el('form');
        const savedName = field(editForm, 'Display name', 'text', item.name);
        const savedAddress = field(editForm, 'Provider address', 'url', item.base_url);
        const replacementKey = field(editForm, 'Replacement provider key — leave blank to keep the saved key', 'password'); replacementKey.autocomplete = 'new-password';
        add(editForm, button('Save changes', async () => { const body = {name: savedName.value, base_url: savedAddress.value}; if (replacementKey.value) body.secret = replacementKey.value; await api('/admin/api/connections/' + encodeURIComponent(item.id), 'PUT', body); replacementKey.value = ''; tell('Connection saved.'); }));
        edit.append(editForm); row.append(edit); s.append(row);
    }
    const form = el('form');
    add(form, el('h3', 'Add a company provider'));
    const id = field(form, 'Short provider name');
    const name = field(form, 'Display name');
    const address = field(form, 'Provider address', 'url');
    const key = field(form, 'Provider key', 'password');
    key.autocomplete = 'new-password';
    const label = el('label', 'Request format');
    const protocol = el('select');
    protocol.id = 'company-format';
    label.htmlFor = protocol.id;
    for (const [value, text] of [['openai', 'Chat requests'], ['openai_responses', 'Response requests'], ['anthropic', 'Anthropic requests']]) {
        const option = el('option', text);
        option.value = value;
        protocol.append(option);
    }
    add(form, label, protocol, el('p', 'Only add providers you want the company to pay for. Provider keys stay on the server. Personal provider choices stay in each member’s app.', 'small'));
    const submit = el('button', 'Save connection');
    submit.type = 'submit';
    form.append(submit);
    form.onsubmit = async (e) => { e.preventDefault(); submit.disabled = true; try {
        await api('/admin/api/connections', 'POST', { id: id.value, name: name.value, base_url: address.value, secret: key.value, protocol: protocol.value });
        key.value = '';
        tell('Connection saved. Refresh its model list before use.');
    }
    catch (error) {
        tell(error.message);
    }
    finally {
        submit.disabled = false;
    } };
    s.append(form);
}
catch (error) {
    errorBox(s, error.message);
} }
async function memberLogin(code = '') { title('Your account', 'Sign in to approve your own devices and manage your access.'); const s = section('Sign in'); s.classList.add('narrow'); const f = el('form'); const name = field(f, 'Account name'); name.autocomplete = 'username'; const password = field(f, 'Password', 'password'); password.autocomplete = 'current-password'; const submit = el('button', 'Sign in'); submit.type = 'submit'; f.append(submit); f.onsubmit = async (e) => { e.preventDefault(); submit.disabled = true; try {
    await api('/account/api/login', 'POST', { username: name.value, password: password.value });
    password.value = '';
    await account(code);
}
catch (error) {
    tell(error.message);
}
finally {
    submit.disabled = false;
} }; add(s, f, el('p', 'New member? Open the private invitation link provided by the owner.')); if (code) s.append(link('Administrator sign-in', '/admin/login?return_to=' + encodeURIComponent('/approve?user_code=' + encodeURIComponent(code)))); }

async function approval(code) {
    let overview;
    try { overview = await api('/admin/api/overview'); }
    catch (error) { if (error.status === 401) return account(code); throw error; }
    csrf = overview.csrf;
    title('Approve a device', 'Choose the account that owns this device.');
    const pending = (await api('/admin/api/devices?status=pending')).devices;
    const device = pending.find(item => item.user_code === code.toUpperCase());
    const area = section('Device ' + code);
    if (!device) { area.append(el('p', 'This code has expired or has already been used. Start sign-in again in Floyd Code.')); return; }
    const users = (await api('/admin/api/users')).users.filter(user => user.enabled);
    const label = el('label', 'Account');
    const select = el('select');
    select.id = 'approval-account';
    label.htmlFor = select.id;
    for (const user of users) { const option = el('option', user.username); option.value = user.id; select.append(option); }
    add(area, label, select, button('Approve this device', async () => {
        await api('/admin/api/devices/' + encodeURIComponent(device.device_code) + '/approve', 'POST', {user_id: select.value});
        area.replaceChildren(el('p', 'Device approved. Return to Floyd Code on that device.', 'good'));
    }));
}
async function join() { const token = location.hash.slice(1); history.replaceState({}, '', location.pathname); title('Welcome to Floyd Code', 'Set a password for the account the owner invited you to.'); const s = section('Choose your password'); s.classList.add('narrow'); if (!token) {
    errorBox(s, 'Open your private invitation link first.');
    return;
} const f = el('form'); const password = field(f, 'Password — at least twelve characters', 'password'); password.autocomplete = 'new-password'; const confirm = field(f, 'Repeat password', 'password'); confirm.autocomplete = 'new-password'; const submit = el('button', 'Join'); submit.type = 'submit'; f.append(submit); f.onsubmit = async (e) => { e.preventDefault(); if (password.value !== confirm.value) {
    tell('The passwords do not match.');
    return;
} submit.disabled = true; try {
    await api('/account/api/join', 'POST', { invitation: token, password: password.value });
    password.value = '';
    confirm.value = '';
    location.assign('/account');
}
catch (error) {
    tell(error.message);
}
finally {
    submit.disabled = false;
} }; s.append(f); }
async function account(code = '') { let data; try {
    data = await api('/account/api/me');
}
catch (error) {
    if (error.status === 401) {
        await memberLogin(code);
        return;
    }
    throw error;
} csrf = data.csrf; title(data.user.username, 'Manage your sign-in and devices.'); const approve = section('Approve a device'); const f = el('form'); const input = field(f, 'Device code', 'text', code); const submit = el('button', 'Approve this device'); submit.type = 'submit'; f.append(submit); f.onsubmit = async (e) => { e.preventDefault(); submit.disabled = true; try {
    await api('/account/api/approve', 'POST', { user_code: input.value });
    tell('Device approved. Return to Floyd Code on that device.');
    input.value = '';
    await account();
}
catch (error) {
    tell(error.message);
}
finally {
    submit.disabled = false;
} }; approve.append(f); const s = section('Your devices'); const devices = (await api('/account/api/devices')).devices; if (devices.length === 0)
    s.append(el('p', 'No devices have been approved yet.')); for (const device of devices) {
    const row = el('div', undefined, 'card');
    add(row, el('strong', device.platform || device.user_code), el('p', device.status + ' · ' + date(device.created_at)));
    if (['approved', 'exchanged'].includes(device.status))
        row.append(button('Cancel access', async () => { await api('/account/api/revoke', 'POST', { device_code: device.device_code }); row.remove(); tell('Device access cancelled.'); }, 'secondary'));
    s.append(row);
} const address = section('Company models'); readonly(address, 'Company model address', data.model_address); add(address, link('Browse providers and models', '/catalog')); await accountSettings(); main.append(button('Sign out', async () => { await api('/account/api/logout', 'POST', {}); location.assign('/account'); }, 'secondary')); }
async function accountSettings() {
    const usage = (await api('/account/api/usage')).company_usage;
    const used = section('Your company use');
    add(used, el('p', usage.requests.toLocaleString() + ' model requests this month.'));
    if (usage.request_limit) add(used, el('p', 'Monthly allowance: ' + usage.request_limit.toLocaleString()));
    add(used, el('p', 'Month ends ' + date(usage.resets_at), 'small'));
    const settings = section('Replace your password');
    const form = el('form');
    const current = field(form, 'Current password', 'password');
    const next = field(form, 'New password — at least twelve characters', 'password');
    const repeat = field(form, 'Repeat new password', 'password');
    current.autocomplete = 'current-password';
    next.autocomplete = repeat.autocomplete = 'new-password';
    const submit = el('button', 'Replace password');
    submit.type = 'submit';
    form.append(submit);
    form.onsubmit = async event => {
        event.preventDefault();
        if (next.value !== repeat.value) { tell('The new passwords do not match.'); return; }
        submit.disabled = true;
        try {
            await api('/account/api/password', 'POST', {current_password: current.value, password: next.value});
            current.value = next.value = repeat.value = '';
            await memberLogin();
            tell('Password replaced. Your previous sign-ins were cancelled.');
        } catch (error) { tell(error.message); }
        finally { submit.disabled = false; }
    };
    add(settings, el('p', 'This closes your other sign-ins and devices.'), form);
}
async function start() { const path = location.pathname; if (path === '/admin/login')
    return adminLogin(); if (path === '/admin')
    return admin(); if (path === '/join')
    return join(); if (path === '/approve') return approval(new URLSearchParams(location.search).get('user_code') || ''); if (path === '/account')
    return account(new URLSearchParams(location.search).get('user_code') || ''); if (path === '/catalog') {
    title('Providers and models', 'Current provider information, saved on the company server.');
    return loadDirectory(main);
} return home(); }
start().catch(error => { main.replaceChildren(el('h1', 'This page could not load'), el('p', error.message, 'error'), button('Try loading the page again', () => location.reload())); });

async function sharedTools() {
    const area = section('Shared company tools');
    area.append(el('p', 'Enabled tools are available to every enabled member. They run with the company’s access on inference.'));
    const data = await api('/admin/api/tools');
    for (const item of data.tools) {
        const row = el('div', undefined, 'card');
        add(row, el('h3', item.name), el('p', item.enabled ? 'Enabled' : 'Disabled'), button(item.enabled ? 'Switch off' : 'Switch on', async () => {
            await api('/admin/api/tools', 'POST', {id: item.id, enabled: !item.enabled});
            tell('Company tool updated. Reload to see its new state.');
        }, 'secondary'));
        area.append(row);
    }
    const form = el('form');
    const term = field(form, 'Find a company tool');
    const submit = el('button', 'Search tools'); submit.type = 'submit'; form.append(submit);
    const found = el('div'); add(area, form, found);
    form.onsubmit = async event => {
        event.preventDefault(); submit.disabled = true;
        try {
            const result = await api('/admin/api/tools/search?query=' + encodeURIComponent(term.value));
            found.replaceChildren();
            for (const item of result.matches || []) {
                const row = el('div', undefined, 'card');
                const id = (item.server + '-' + item.tool).replaceAll(/[^a-zA-Z0-9_-]/g, '-').slice(0,64);
                add(row, el('strong', item.tool), el('p', item.description || ''), button('Enable for all members', async () => {
                    await api('/admin/api/tools', 'POST', {id, name: item.tool, server: item.server, tool: item.tool, enabled: true});
                    tell('Company tool enabled.');
                })); found.append(row);
            }
            if (found.children.length === 0) found.append(el('p', 'No matching tools.'));
        } catch (error) { tell(error.message); }
        finally { submit.disabled = false; }
    };
    const feedback = section('Member reports');
    const reports = (await api('/admin/api/feedback')).feedback;
    if (reports.length === 0) feedback.append(el('p', 'No reports yet.'));
    for (const report of reports) {
        const box = el('details');
        add(box, el('summary', (report.username || 'Member') + ' · ' + date(report.created_at)), el('pre', JSON.stringify(report.body, null, 2)));
        feedback.append(box);
    }
}
