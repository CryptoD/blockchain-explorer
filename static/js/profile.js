    const API_BASE = '/api/v1';
    function getCSRFToken() {
        try { return localStorage.getItem('csrfToken'); } catch (e) { return null; }
    }
    async function authFetch(url, options = {}) {
        const opts = { ...options };
        opts.credentials = opts.credentials || 'include';
        opts.headers = opts.headers ? { ...opts.headers } : {};
        if (getCSRFToken()) opts.headers['X-CSRF-Token'] = getCSRFToken();
        return fetch(url, opts);
    }

    function getSavedAccentTheme() {
        return localStorage.getItem('blockchain-explorer-theme') || 'blue';
    }
    function setAccentTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('blockchain-explorer-theme', theme);
    }

    function applyColorScheme(scheme) {
        if (scheme === 'system') {
            const dark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
            document.documentElement.setAttribute('data-color-scheme', dark ? 'dark' : 'light');
        } else if (scheme === 'light' || scheme === 'dark') {
            document.documentElement.setAttribute('data-color-scheme', scheme);
        } else {
            document.documentElement.removeAttribute('data-color-scheme');
        }
    }
    function installSystemThemeListener() {
        if (!window.matchMedia) return;
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function() {
            if (document.getElementById('profile-theme').value === 'system') {
                applyColorScheme('system');
            }
        });
    }

    function applyLanguage(lang) {
        if (lang && (lang === 'en' || lang === 'es')) {
            document.documentElement.lang = lang;
        }
    }

    function populateForm(user) {
        const themeSel = document.getElementById('profile-theme');
        const langSel = document.getElementById('profile-language');
        const emailEl = document.getElementById('profile-email');
        const currencySel = document.getElementById('profile-currency');
        const landingSel = document.getElementById('profile-landing');
        const emailCb = document.getElementById('profile-notify-email');
        const alertsCb = document.getElementById('profile-notify-alerts');
        const emailPriceAlertsCb = document.getElementById('profile-email-price-alerts');
        const emailPortfolioEventsCb = document.getElementById('profile-email-portfolio-events');
        const emailProductUpdatesCb = document.getElementById('profile-email-product-updates');
        const favTa = document.getElementById('profile-news-favorite');
        const blockedTa = document.getElementById('profile-news-blocked');
        if (themeSel && user.theme) themeSel.value = user.theme;
        if (langSel && user.language) langSel.value = user.language;
        if (emailEl) emailEl.value = user.email || '';
        if (currencySel && user.preferred_currency) currencySel.value = user.preferred_currency;
        if (landingSel && user.default_landing_page) landingSel.value = user.default_landing_page;
        if (emailCb) emailCb.checked = !!user.notifications_email;
        if (alertsCb) alertsCb.checked = !!user.notifications_price_alerts;
        if (emailPriceAlertsCb) emailPriceAlertsCb.checked = !!user.email_price_alerts;
        if (emailPortfolioEventsCb) emailPortfolioEventsCb.checked = !!user.email_portfolio_events;
        if (emailProductUpdatesCb) emailProductUpdatesCb.checked = !!user.email_product_updates;
        if (favTa) favTa.value = Array.isArray(user.news_sources_favorite) ? user.news_sources_favorite.join('\n') : '';
        if (blockedTa) blockedTa.value = Array.isArray(user.news_sources_blocked) ? user.news_sources_blocked.join('\n') : '';
    }

    function parseDomains(text) {
        const raw = String(text || '');
        const parts = raw.split(/[\n,]/g).map(s => s.trim().toLowerCase()).filter(Boolean);
        const seen = new Set();
        const out = [];
        for (const p of parts) {
            if (p.length > 100) continue;
            if (!/^[a-z0-9.-]+$/.test(p)) continue;
            if (p.startsWith('.') || p.endsWith('.') || p.includes('..')) continue;
            if (seen.has(p)) continue;
            seen.add(p);
            out.push(p);
        }
        return out;
    }

    document.getElementById('profile-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const msg = document.getElementById('profile-message');
        const err = document.getElementById('profile-error');
        msg.classList.add('hidden');
        err.classList.add('hidden');
        const theme = document.getElementById('profile-theme').value || null;
        const language = document.getElementById('profile-language').value || null;
        const emailValue = document.getElementById('profile-email') ? (document.getElementById('profile-email').value || '').trim() : null;
        const preferred_currency = document.getElementById('profile-currency').value || null;
        const default_landing_page = document.getElementById('profile-landing').value || null;
        const notifications_email = document.getElementById('profile-notify-email').checked;
        const notifications_price_alerts = document.getElementById('profile-notify-alerts').checked;
        const email_price_alerts = document.getElementById('profile-email-price-alerts') ? document.getElementById('profile-email-price-alerts').checked : false;
        const email_portfolio_events = document.getElementById('profile-email-portfolio-events') ? document.getElementById('profile-email-portfolio-events').checked : false;
        const email_product_updates = document.getElementById('profile-email-product-updates') ? document.getElementById('profile-email-product-updates').checked : false;
        const news_sources_favorite = parseDomains(document.getElementById('profile-news-favorite') ? document.getElementById('profile-news-favorite').value : '');
        const news_sources_blocked = parseDomains(document.getElementById('profile-news-blocked') ? document.getElementById('profile-news-blocked').value : '');
        const body = {};
        if (theme !== null) body.theme = theme;
        if (language !== null) body.language = language;
        // Email is stored on the user profile; validate server-side.
        if (emailValue !== null) body.email = emailValue;
        if (preferred_currency !== null) body.preferred_currency = preferred_currency;
        if (default_landing_page !== null) body.default_landing_page = default_landing_page;
        body.notifications_email = notifications_email;
        body.notifications_price_alerts = notifications_price_alerts;
        body.email_price_alerts = email_price_alerts;
        body.email_portfolio_events = email_portfolio_events;
        body.email_product_updates = email_product_updates;
        body.news_sources_favorite = news_sources_favorite;
        body.news_sources_blocked = news_sources_blocked;

        try {
            const res = await authFetch(API_BASE + '/user/profile', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            if (!res.ok) {
                const data = await res.json().catch(() => ({}));
                err.textContent = data.error || data.code || 'Failed to save';
                err.classList.remove('hidden');
                return;
            }
            applyColorScheme(theme || '');
            applyLanguage(language || '');
            msg.classList.remove('hidden');
            setTimeout(() => msg.classList.add('hidden'), 3000);
        } catch (e) {
            err.textContent = 'Network error. Try again.';
            err.classList.remove('hidden');
        }
    });

    document.getElementById('profile-theme').addEventListener('change', function() {
        applyColorScheme(this.value || '');
    });
    document.getElementById('profile-language').addEventListener('change', function() {
        applyLanguage(this.value || '');
    });

    const themeSelect = document.getElementById('theme-select');
    if (themeSelect) {
        themeSelect.value = getSavedAccentTheme();
        themeSelect.addEventListener('change', function() {
            setAccentTheme(this.value);
        });
    }

    const mobileBtn = document.getElementById('mobile-menu-button');
    const mobileMenu = document.getElementById('mobile-menu');
    if (mobileBtn && mobileMenu) {
        mobileBtn.addEventListener('click', () => mobileMenu.classList.toggle('hidden'));
    }

    document.getElementById('search-icon').addEventListener('click', function() {
        const q = document.getElementById('search-input').value.trim();
        if (q) window.location.href = '/bitcoin?q=' + encodeURIComponent(q);
    });
    document.getElementById('search-input').addEventListener('keydown', function(e) {
        if (e.key === 'Enter') document.getElementById('search-icon').click();
    });

    async function init() {
        const accent = getSavedAccentTheme();
        setAccentTheme(accent);
        try {
            const res = await authFetch(API_BASE + '/user/profile');
            if (res.ok) {
                const user = await res.json();
                document.getElementById('username-display').textContent = user.username;
                document.getElementById('user-info').classList.remove('hidden');
                document.getElementById('auth-buttons').classList.add('hidden');
                document.getElementById('profile-form').classList.remove('hidden');
                document.getElementById('profile-guest').classList.add('hidden');
                populateForm(user);
                applyColorScheme(user.theme || '');
                applyLanguage(user.language || '');
                installSystemThemeListener();
                await loadAlerts();
            } else {
                document.getElementById('auth-buttons').classList.remove('hidden');
                document.getElementById('user-info').classList.add('hidden');
                document.getElementById('profile-form').classList.add('hidden');
                document.getElementById('profile-guest').classList.remove('hidden');
            }
        } catch (e) {
            document.getElementById('auth-buttons').classList.remove('hidden');
            document.getElementById('user-info').classList.add('hidden');
            document.getElementById('profile-form').classList.add('hidden');
            document.getElementById('profile-guest').classList.remove('hidden');
        }
    }

    async function loadAlerts() {
        const listEl = document.getElementById('alerts-list');
        const errEl = document.getElementById('alerts-error');
        const emptyEl = document.getElementById('alerts-empty');
        if (!listEl) return;
        errEl.classList.add('hidden');
        emptyEl.classList.add('hidden');
        listEl.innerHTML = '<tr><td class="py-3 text-text-secondary" colspan="7">Loading…</td></tr>';
        try {
            const res = await authFetch(API_BASE + '/user/alerts?page_size=100');
            if (!res.ok) throw new Error('HTTP ' + res.status);
            const payload = await res.json();
            const alerts = (payload && Array.isArray(payload.data)) ? payload.data : [];
            if (alerts.length === 0) {
                listEl.innerHTML = '';
                emptyEl.classList.remove('hidden');
                return;
            }
            listEl.innerHTML = alerts.map(a => `
                <tr>
                    <td class="py-2 pr-3 font-medium">${escapeHtml(a.symbol || '')}</td>
                    <td class="py-2 pr-3">${escapeHtml(a.direction || '')}</td>
                    <td class="py-2 pr-3">${escapeHtml(String(a.threshold ?? ''))}</td>
                    <td class="py-2 pr-3">${escapeHtml((a.currency || '').toUpperCase())}</td>
                    <td class="py-2 pr-3">${escapeHtml(a.delivery_method || '')}</td>
                    <td class="py-2 pr-3">
                        <label class="inline-flex items-center gap-2">
                            <input type="checkbox" class="alert-active-toggle rounded border-border" data-id="${escapeHtml(a.id)}" ${a.is_active ? 'checked' : ''} />
                            <span class="text-xs text-text-secondary">${a.is_active ? 'On' : 'Off'}</span>
                        </label>
                    </td>
                    <td class="py-2 pr-3 text-right">
                        <button type="button" class="alert-delete text-red-600 hover:underline" data-id="${escapeHtml(a.id)}">Delete</button>
                    </td>
                </tr>
            `).join('');

            listEl.querySelectorAll('.alert-delete').forEach(btn => {
                btn.addEventListener('click', async () => {
                    const id = btn.getAttribute('data-id');
                    if (!id) return;
                    await deleteAlert(id);
                });
            });
            listEl.querySelectorAll('.alert-active-toggle').forEach(cb => {
                cb.addEventListener('change', async () => {
                    const id = cb.getAttribute('data-id');
                    if (!id) return;
                    await updateAlert(id, { is_active: cb.checked });
                });
            });
        } catch (e) {
            listEl.innerHTML = '';
            errEl.textContent = 'Failed to load alerts.';
            errEl.classList.remove('hidden');
        }
    }

    async function createAlert() {
        const errEl = document.getElementById('alerts-error');
        errEl.classList.add('hidden');
        const symbol = (document.getElementById('alert-symbol').value || '').trim();
        const direction = document.getElementById('alert-direction').value;
        const threshold = parseFloat(document.getElementById('alert-threshold').value);
        const currency = document.getElementById('alert-currency').value;
        const delivery_method = document.getElementById('alert-delivery').value;
        try {
            const res = await authFetch(API_BASE + '/user/alerts', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ symbol, direction, threshold, currency, delivery_method, is_active: true })
            });
            if (!res.ok) {
                const data = await res.json().catch(() => ({}));
                errEl.textContent = data.error || data.code || 'Failed to create alert';
                errEl.classList.remove('hidden');
                return;
            }
            document.getElementById('alert-threshold').value = '';
            await loadAlerts();
        } catch (e) {
            errEl.textContent = 'Network error creating alert.';
            errEl.classList.remove('hidden');
        }
    }

    async function updateAlert(id, patch) {
        const errEl = document.getElementById('alerts-error');
        errEl.classList.add('hidden');
        try {
            const res = await authFetch(API_BASE + '/user/alerts/' + encodeURIComponent(id), {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(patch || {})
            });
            if (!res.ok) {
                const data = await res.json().catch(() => ({}));
                errEl.textContent = data.error || data.code || 'Failed to update alert';
                errEl.classList.remove('hidden');
                await loadAlerts();
            }
        } catch (e) {
            errEl.textContent = 'Network error updating alert.';
            errEl.classList.remove('hidden');
            await loadAlerts();
        }
    }

    async function deleteAlert(id) {
        const errEl = document.getElementById('alerts-error');
        errEl.classList.add('hidden');
        try {
            const res = await authFetch(API_BASE + '/user/alerts/' + encodeURIComponent(id), { method: 'DELETE' });
            if (!res.ok) {
                const data = await res.json().catch(() => ({}));
                errEl.textContent = data.error || data.code || 'Failed to delete alert';
                errEl.classList.remove('hidden');
                return;
            }
            await loadAlerts();
        } catch (e) {
            errEl.textContent = 'Network error deleting alert.';
            errEl.classList.remove('hidden');
        }
    }

    function escapeHtml(str) {
        return String(str || '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }
    document.getElementById('logout-btn').addEventListener('click', async () => {
        await authFetch(API_BASE + '/logout', { method: 'POST' });
        window.location.href = '/';
    });
    const alertsRefresh = document.getElementById('alerts-refresh');
    if (alertsRefresh) alertsRefresh.addEventListener('click', loadAlerts);
    const alertCreateBtn = document.getElementById('alert-create-btn');
    if (alertCreateBtn) alertCreateBtn.addEventListener('click', createAlert);
    init();
