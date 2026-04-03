    // Theme management
    function setTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('blockchain-explorer-theme', theme);
    }

    function getSavedTheme() {
        return localStorage.getItem('blockchain-explorer-theme') || 'blue';
    }

    function initializeTheme() {
        const savedTheme = getSavedTheme();
        setTheme(savedTheme);
        const themeSelect = document.getElementById('theme-select');
        if (themeSelect) {
            themeSelect.value = savedTheme;
            themeSelect.addEventListener('change', function(e) {
                setTheme(e.target.value);
            });
        }
    }

    // Dark mode auto-switching (kept for backward compatibility)
    function updateTheme() {
        const isDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
        if (isDark) {
            document.documentElement.classList.add('dark');
        } else {
            document.documentElement.classList.remove('dark');
        }
    }

    // Initial setup
    initializeTheme();
    updateTheme();

    // Listen for changes
    if (window.matchMedia) {
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', updateTheme);
    }

    // Listen for changes
    if (window.matchMedia) {
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', updateTheme);
    }

    // Debounce utility
    function debounce(func, wait) {
        let timeout;
        return function(...args) {
            clearTimeout(timeout);
            timeout = setTimeout(() => func.apply(this, args), wait);
        };
    }

    const debouncedSearch = debounce(performSearch, 500);

    // Hook both desktop and mobile search elements
    const desktopSearchInput = document.querySelector('#search-input');
    const desktopSearchButton = document.querySelector('#search-icon');
    const mobileSearchInput = document.querySelector('#search-input-mobile');
    const mobileSearchButton = document.querySelector('#search-icon-mobile');

    if (desktopSearchInput) {
        desktopSearchInput.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') {
                debouncedSearch(e.target.value);
            }
        });
    }
    if (desktopSearchButton) {
        desktopSearchButton.addEventListener('click', function() {
            const query = desktopSearchInput.value;
            debouncedSearch(query);
        });
    }

    if (mobileSearchInput) {
        mobileSearchInput.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') {
                debouncedSearch(e.target.value);
            }
        });
    }
    if (mobileSearchButton) {
        mobileSearchButton.addEventListener('click', function() {
            const query = mobileSearchInput.value;
            // close mobile menu for better UX
            closeMobileMenu();
            debouncedSearch(query);
        });
    }

    // Mobile menu toggling and accessibility
    const mobileMenuButton = document.getElementById('mobile-menu-button');
    const mobileMenu = document.getElementById('mobile-menu');

    function openMobileMenu() {
        mobileMenu.classList.remove('hidden');
        mobileMenuButton.setAttribute('aria-expanded', 'true');
        // focus first link for keyboard users
        const firstLink = mobileMenu.querySelector('a');
        if (firstLink) firstLink.focus();
        document.addEventListener('click', onDocClick);
        document.addEventListener('keydown', onKeyDown);
    }

    function closeMobileMenu() {
        mobileMenu.classList.add('hidden');
        mobileMenuButton.setAttribute('aria-expanded', 'false');
        mobileMenuButton.focus();
        document.removeEventListener('click', onDocClick);
        document.removeEventListener('keydown', onKeyDown);
    }

    function toggleMobileMenu() {
        const isHidden = mobileMenu.classList.contains('hidden');
        if (isHidden) openMobileMenu(); else closeMobileMenu();
    }

    function onDocClick(e) {
        if (!mobileMenu.contains(e.target) && e.target !== mobileMenuButton) {
            closeMobileMenu();
        }
    }

    function onKeyDown(e) {
        if (e.key === 'Escape') {
            closeMobileMenu();
        }
    }

    if (mobileMenuButton) {
        mobileMenuButton.addEventListener('click', function(e) {
            e.stopPropagation();
            toggleMobileMenu();
        });
    }

    const loginBtn = document.getElementById('login-btn');
    const registerBtn = document.getElementById('register-btn');
    const logoutBtn = document.getElementById('logout-btn');
    const authForms = document.getElementById('auth-forms');
    const loginFormContainer = document.getElementById('login-form-container');
    const registerFormContainer = document.getElementById('register-form-container');
    const loginForm = document.getElementById('login-form');
    const registerForm = document.getElementById('register-form');
    const switchToRegister = document.getElementById('switch-to-register');
    const switchToLogin = document.getElementById('switch-to-login');
    const userInfo = document.getElementById('user-info');
    const authButtons = document.getElementById('auth-buttons');
    const usernameDisplay = document.getElementById('username-display');

    if (loginBtn) loginBtn.addEventListener('click', () => {
        searchSection.classList.add('hidden');
        portfolioSection.classList.add('hidden');
        authForms.classList.remove('hidden');
        loginFormContainer.classList.remove('hidden');
        registerFormContainer.classList.add('hidden');
    });

    if (registerBtn) registerBtn.addEventListener('click', () => {
        searchSection.classList.add('hidden');
        portfolioSection.classList.add('hidden');
        authForms.classList.remove('hidden');
        loginFormContainer.classList.add('hidden');
        registerFormContainer.classList.remove('hidden');
    });

    if (switchToRegister) switchToRegister.addEventListener('click', () => {
        loginFormContainer.classList.add('hidden');
        registerFormContainer.classList.remove('hidden');
    });

    if (switchToLogin) switchToLogin.addEventListener('click', () => {
        registerFormContainer.classList.add('hidden');
        loginFormContainer.classList.remove('hidden');
    });

    if (loginForm) loginForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(loginForm);
        const data = Object.fromEntries(formData.entries());
        try {
            const res = await fetch('/api/v1/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });
            if (res.ok) {
                const user = await res.json();
                checkAuth();
                authForms.classList.add('hidden');
                searchSection.classList.remove('hidden');
            } else {
                alert('Login failed');
            }
        } catch (err) {
            alert('Login failed');
        }
    });

    if (registerForm) registerForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(registerForm);
        const data = Object.fromEntries(formData.entries());
        try {
            const res = await fetch('/api/v1/register', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });
            if (res.ok) {
                alert('Registration successful, please login');
                switchToLogin.click();
            } else {
                alert('Registration failed');
            }
        } catch (err) {
            alert('Registration failed');
        }
    });

    if (logoutBtn) logoutBtn.addEventListener('click', async () => {
        await fetch('/api/v1/logout', { method: 'POST' });
        checkAuth();
        location.reload();
    });

    function applyProfileToPage(user) {
        if (!user) return;
        if (user.theme === 'system' || user.theme === 'light' || user.theme === 'dark') {
            const scheme = user.theme === 'system'
                ? (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
                : user.theme;
            document.documentElement.setAttribute('data-color-scheme', scheme);
        } else {
            document.documentElement.removeAttribute('data-color-scheme');
        }
        if (user.language && (user.language === 'en' || user.language === 'es')) {
            document.documentElement.lang = user.language;
        }
    }

    async function checkAuth() {
        try {
            const res = await fetch('/api/v1/user/profile', { credentials: 'include' });
            if (res.ok) {
                const user = await res.json();
                applyProfileToPage(user);
                usernameDisplay.textContent = user.username;
                userInfo.classList.remove('hidden');
                authButtons.classList.add('hidden');
                if (portfoliosNav) portfoliosNav.classList.remove('hidden');
                const portfoliosNavMobile = document.getElementById('portfolios-nav-mobile');
                if (portfoliosNavMobile) portfoliosNavMobile.classList.remove('hidden');
                // Show dashboard links
                const dashboardNav = document.getElementById('dashboard-nav');
                const dashboardNavMobile = document.getElementById('dashboard-nav-mobile');
                if (dashboardNav) dashboardNav.classList.remove('hidden');
                if (dashboardNavMobile) dashboardNavMobile.classList.remove('hidden');
                const profileNav = document.getElementById('profile-nav');
                const profileNavMobile = document.getElementById('profile-nav-mobile');
                if (profileNav) profileNav.classList.remove('hidden');
                if (profileNavMobile) profileNavMobile.classList.remove('hidden');
            } else {
                userInfo.classList.add('hidden');
                authButtons.classList.remove('hidden');
                if (portfoliosNav) portfoliosNav.classList.add('hidden');
                const portfoliosNavMobile = document.getElementById('portfolios-nav-mobile');
                if (portfoliosNavMobile) portfoliosNavMobile.classList.add('hidden');
                // Hide dashboard links
                const dashboardNav = document.getElementById('dashboard-nav');
                const dashboardNavMobile = document.getElementById('dashboard-nav-mobile');
                if (dashboardNav) dashboardNav.classList.add('hidden');
                if (dashboardNavMobile) dashboardNavMobile.classList.add('hidden');
                const profileNav = document.getElementById('profile-nav');
                const profileNavMobile = document.getElementById('profile-nav-mobile');
                if (profileNav) profileNav.classList.add('hidden');
                if (profileNavMobile) profileNavMobile.classList.add('hidden');
            }
        } catch (err) {
            userInfo.classList.add('hidden');
            authButtons.classList.remove('hidden');
        }
    }

    checkAuth();

    function validateSearchQuery(query) {
        const cleanQuery = query.trim();
        if (!cleanQuery) return { isValid: false, error: 'EMPTY_SEARCH' };
        if (cleanQuery.length < 8) return { isValid: false, error: 'INVALID_SEARCH' };

        const validPatterns = {
            hash: /^[0-9a-fA-F]{64}$/,
            address: /^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$|^0x[0-9a-fA-F]{40}$/,
            blockHeight: /^\d+$/
        };

        const isValidFormat = Object.values(validPatterns).some(p => p.test(cleanQuery));

        // Insert Chart.js CDN before home.js if not already present
        (function insertChartJsBeforeApp() {
            // If Chart.js is already present, do nothing
            if (document.querySelector('script[src="https://cdn.jsdelivr.net/npm/chart.js"]')) return;
            // Find the existing app script (home.js) to insert before it
            const appScript = Array.from(document.getElementsByTagName('script'))
                .find(s => s.src && (s.src.includes('/static/js/home.js') || s.src.endsWith('home.js')));
            if (!appScript) return;
            const chartScript = document.createElement('script');
            chartScript.src = 'https://cdn.jsdelivr.net/npm/chart.js';
            chartScript.async = false; // preserve execution order before home.js
            appScript.parentNode.insertBefore(chartScript, appScript);
        })();
        return { isValid: isValidFormat, error: isValidFormat ? null : 'INVALID_SEARCH' };
    }

    function displayError(errorType) {
        const resultContainer = document.getElementById('result-container');
        const messages = {
            API_FAILURE: 'Blockchain search got the hiccups. Try again later.',
            UNKNOWN_RESULT: 'Unknown result type',
            NETWORK_ERROR: 'Network connection issue. Please check your internet.',
            INVALID_SEARCH: 'Invalid search format',
            EMPTY_SEARCH: 'Please enter a search term'
        };
        resultContainer.innerHTML = `<div class="text-red-600 font-semibold">${messages[errorType]}</div>`;
    }

    function performSearch(query) {
        const validation = validateSearchQuery(query);
        if (!validation.isValid) { displayError(validation.error); return; }
        window.location.href = `/bitcoin?q=${encodeURIComponent(query)}`;
    }

    // Feedback form handling
    const feedbackForm = document.getElementById('feedback-form');
    const feedbackResult = document.getElementById('feedback-result');

    if (feedbackForm) {
        feedbackForm.addEventListener('submit', async function(e) {
            e.preventDefault();

            const formData = new FormData(feedbackForm);
            const data = {
                name: formData.get('name'),
                email: formData.get('email'),
                message: formData.get('message')
            };

            try {
                const response = await fetch('/api/v1/feedback', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(data)
                });

                const result = await response.json();

                if (response.ok) {
                    feedbackResult.className = 'mt-4 text-green-600 font-semibold';
                    feedbackResult.textContent = result.message;
                    feedbackForm.reset();
                } else {
                    feedbackResult.className = 'mt-4 text-red-600 font-semibold';
                    feedbackResult.textContent = result.error || 'Failed to submit feedback';
                }
            } catch (error) {
                feedbackResult.className = 'mt-4 text-red-600 font-semibold';
                feedbackResult.textContent = 'Network error. Please try again later.';
            }

            feedbackResult.classList.remove('hidden');
        });
    }

    // Portfolio management
    const portfoliosNav = document.getElementById('portfolios-nav');
    const portfolioSection = document.getElementById('portfolio-section');
    const searchSection = document.getElementById('search-section');
    const portfolioList = document.getElementById('portfolio-list');
    const portfolioModal = document.getElementById('portfolio-modal');
    const portfolioForm = document.getElementById('portfolio-form');
    const itemsContainer = document.getElementById('portfolio-items-container');
    const addAddressBtn = document.getElementById('add-address-btn');
    const createPortfolioBtn = document.getElementById('create-portfolio-btn');
    const closeModalBtn = document.getElementById('close-modal-btn');

    function showPortfolios() {
        searchSection.classList.add('hidden');
        portfolioSection.classList.remove('hidden');
        syncPortfolioCurrencyFromProfile();
        loadPortfolios();
    }

    async function syncPortfolioCurrencyFromProfile() {
        try {
            const res = await fetch('/api/user/profile', { credentials: 'include' });
            if (res.ok) {
                const user = await res.json();
                if (user.preferred_currency && typeof user.preferred_currency === 'string') {
                    const c = user.preferred_currency.toLowerCase().trim();
                    const sel = document.getElementById('portfolio-currency-selector');
                    if (sel && sel.querySelector('option[value="' + c + '"]')) {
                        sel.value = c;
                        portfolioCurrency = c;
                    }
                }
            }
        } catch (e) {}
    }

    const portfolioCurrencyEl = document.getElementById('portfolio-currency-selector');
    if (portfolioCurrencyEl) {
        portfolioCurrencyEl.addEventListener('change', function() {
            portfolioCurrency = this.value;
            loadPortfolios();
        });
    }

    if (portfoliosNav) {
        portfoliosNav.addEventListener('click', (e) => {
            e.preventDefault();
            showPortfolios();
        });
    }

    if (portfolioSection) {
        portfolioSection.addEventListener('click', function(e) {
            const editBtn = e.target.closest('.portfolio-edit-btn');
            if (editBtn) {
                e.preventDefault();
                const id = editBtn.getAttribute('data-portfolio-id');
                if (id && typeof window.editPortfolio === 'function') {
                    window.editPortfolio(id);
                }
                return;
            }
            const delBtn = e.target.closest('.portfolio-delete-btn');
            if (delBtn) {
                e.preventDefault();
                const id = delBtn.getAttribute('data-portfolio-id');
                if (id && typeof window.deletePortfolio === 'function') {
                    window.deletePortfolio(id);
                }
            }
        });
    }

    function getCSRFToken() {
        try {
            return localStorage.getItem('csrfToken');
        } catch (e) {
            return null;
        }
    }

    async function authFetch(url, options = {}) {
        const token = getCSRFToken();
        const opts = { ...options };
        opts.headers = opts.headers ? { ...opts.headers } : {};
        if (token) {
            opts.headers['X-CSRF-Token'] = token;
        }
        return fetch(url, opts);
    }

    const CURRENCY_SYMBOLS = { usd: '$', eur: '€', gbp: '£', jpy: '¥', cad: 'C$', aud: 'A$', chf: 'CHF ' };
    let portfolioCurrency = 'usd';

    function formatPortfolioFiat(value, currency) {
        if (value == null || isNaN(value)) return '—';
        const sym = CURRENCY_SYMBOLS[currency] || currency.toUpperCase() + ' ';
        return sym + Number(value).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    }

    async function loadPortfolios() {
        try {
            const url = '/api/v1/user/portfolios?currency=' + encodeURIComponent(portfolioCurrency);
            const res = await fetch(url, { credentials: 'include' });
            if (!res.ok) {
                portfolioList.innerHTML = '<p class="text-text-secondary">Sign in to view and export your portfolios.</p>';
                return;
            }
            const json = await res.json();
            const portfolios = json.data || json;
            if (!Array.isArray(portfolios)) {
                portfolioList.innerHTML = '<p class="text-text-secondary">No portfolios found.</p>';
                return;
            }
            const cur = (portfolios[0] && portfolios[0].valuation_currency) || portfolioCurrency;
            portfolioList.innerHTML = portfolios.map(p => {
                const totalVal = p.total_value_fiat != null ? formatPortfolioFiat(p.total_value_fiat, p.valuation_currency || cur) : '—';
                return `
                <div class="bg-bg-secondary p-6 rounded-lg shadow-sm border border-border">
                    <h3 class="text-xl font-bold">${p.name}</h3>
                    <p class="text-text-secondary text-sm mb-2">${p.description || 'No description'}</p>
                    <p class="text-sm font-semibold mb-3">Total value: ${totalVal}</p>
                    <div class="text-sm space-y-1 mb-4">
                        ${(p.items || []).map(item => `<div><span class="font-medium">[${(item.type || 'stock').toUpperCase()}] ${item.label}:</span> ${item.address}${item.value_fiat != null ? ' — ' + formatPortfolioFiat(item.value_fiat, p.valuation_currency || cur) : ''}</div>`).join('')}
                    </div>
                    <div class="flex flex-wrap gap-2 items-center">
                        <button type="button" class="portfolio-edit-btn text-primary hover:underline" data-portfolio-id="${p.id}">Edit</button>
                        <button type="button" class="portfolio-delete-btn text-red-600 hover:underline" data-portfolio-id="${p.id}">Delete</button>
                        <div class="relative inline-block export-dropdown" data-portfolio-id="${p.id}" data-portfolio-name="${(p.name || '').replace(/"/g, '&quot;')}">
                            <button type="button" class="export-trigger px-3 py-1 border border-border rounded hover:bg-hover text-sm flex items-center gap-1" aria-haspopup="true" aria-expanded="false" aria-label="Export portfolio">
                                <span class="export-label">Export</span>
                                <span class="export-spinner hidden inline-block w-4 h-4 border-2 border-primary border-t-transparent rounded-full animate-spin"></span>
                                <span aria-hidden="true">▾</span>
                            </button>
                            <div class="export-menu hidden absolute left-0 mt-1 py-1 bg-bg-secondary border border-border rounded shadow z-10 min-w-[140px]">
                                <button type="button" class="export-option w-full text-left px-4 py-2 text-sm hover:bg-hover" data-format="csv">Download CSV</button>
                                <button type="button" class="export-option w-full text-left px-4 py-2 text-sm hover:bg-hover" data-format="json">Download JSON</button>
                                <button type="button" class="export-option w-full text-left px-4 py-2 text-sm hover:bg-hover" data-format="pdf">Download PDF</button>
                            </div>
                        </div>
                        <p class="export-error hidden text-red-600 text-sm" role="alert"></p>
                    </div>
                </div>
            `;
            }).join('');
            bindPortfolioExportHandlers();
        } catch (err) {
            console.error('Failed to load portfolios', err);
            portfolioList.innerHTML = '<p class="text-red-600">Failed to load portfolios. Sign in and try again.</p>';
        }
    }

    function bindPortfolioExportHandlers() {
        document.querySelectorAll('.export-dropdown').forEach(wrap => {
            const trigger = wrap.querySelector('.export-trigger');
            const menu = wrap.querySelector('.export-menu');
            const spinner = wrap.querySelector('.export-spinner');
            const label = wrap.querySelector('.export-label');
            const id = wrap.getAttribute('data-portfolio-id');
            const name = wrap.getAttribute('data-portfolio-name');
            const errEl = wrap.closest('.flex').querySelector('.export-error');

            const closeMenu = () => {
                menu.classList.add('hidden');
                trigger.setAttribute('aria-expanded', 'false');
            };

            trigger.addEventListener('click', (e) => {
                e.stopPropagation();
                if (spinner.classList.contains('hidden')) {
                    const open = !menu.classList.contains('hidden');
                    document.querySelectorAll('.export-menu').forEach(m => m.classList.add('hidden'));
                    if (!open) {
                        menu.classList.remove('hidden');
                        trigger.setAttribute('aria-expanded', 'true');
                    }
                }
            });

            wrap.querySelectorAll('.export-option').forEach(btn => {
                btn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    const format = btn.getAttribute('data-format');
                    closeMenu();
                    if (errEl) errEl.classList.add('hidden');
                    spinner.classList.remove('hidden');
                    label.textContent = 'Exporting…';
                    try {
                        if (format === 'json') {
                            const res = await fetch('/api/v1/user/portfolios', { credentials: 'include' });
                            if (!res.ok) throw new Error('Export failed');
                            const data = await res.json();
                            const list = data.data || data;
                            const one = Array.isArray(list) ? list.find(p => p.id === id) : null;
                            if (!one) throw new Error('Portfolio not found');
                            const blob = new Blob([JSON.stringify(one, null, 2)], { type: 'application/json' });
                            const a = document.createElement('a');
                            a.href = URL.createObjectURL(blob);
                            a.download = 'portfolio-' + id + '.json';
                            a.click();
                            URL.revokeObjectURL(a.href);
                        } else {
                            const url = format === 'csv' ? `/api/v1/user/portfolios/${id}/export/csv` : `/api/v1/user/portfolios/${id}/export/pdf`;
                            const res = await fetch(url, { credentials: 'include' });
                            if (!res.ok) {
                                const errBody = await res.json().catch(() => ({}));
                                throw new Error(errBody.error || errBody.code || 'Export failed');
                            }
                            const blob = await res.blob();
                            const a = document.createElement('a');
                            a.href = URL.createObjectURL(blob);
                            a.download = 'portfolio-' + id + (format === 'pdf' ? '.pdf' : '.csv');
                            a.click();
                            URL.revokeObjectURL(a.href);
                        }
                    } catch (err) {
                        if (errEl) {
                            errEl.textContent = err.message || 'Export failed. Sign in and try again.';
                            errEl.classList.remove('hidden');
                        }
                    } finally {
                        spinner.classList.add('hidden');
                        label.textContent = 'Export';
                    }
                });
            });
        });
        document.addEventListener('click', function() {
            document.querySelectorAll('.export-menu').forEach(function(m) { m.classList.add('hidden'); });
            document.querySelectorAll('.export-trigger').forEach(function(t) { t.setAttribute('aria-expanded', 'false'); });
        });
    }

    if (addAddressBtn) {
        addAddressBtn.addEventListener('click', () => {
            const itemDiv = document.createElement('div');
            itemDiv.className = 'flex gap-2 items-center';
            itemDiv.innerHTML = `
                <select class="p-2 border rounded bg-bg text-text border-border portfolio-type">
                    <option value="stock">Stock</option>
                    <option value="crypto">Crypto</option>
                    <option value="bond">Bond</option>
                    <option value="commodity">Commodity</option>
                </select>
                <input type="text" placeholder="Label" class="flex-1 p-2 border rounded bg-bg text-text border-border portfolio-label">
                <input type="text" placeholder="Address" class="flex-[2] p-2 border rounded bg-bg text-text border-border portfolio-address">
                <button type="button" class="portfolio-remove-item-btn text-red-600 px-2" aria-label="Remove row">×</button>
            `;
            itemsContainer.appendChild(itemDiv);
        });
    }

    if (itemsContainer) {
        itemsContainer.addEventListener('click', function(e) {
            const rm = e.target.closest('.portfolio-remove-item-btn');
            if (rm && rm.parentElement) {
                e.preventDefault();
                rm.parentElement.remove();
            }
        });
    }

    if (createPortfolioBtn) {
        createPortfolioBtn.addEventListener('click', () => {
            document.getElementById('modal-title').textContent = 'Create Portfolio';
            document.getElementById('portfolio-id').value = '';
            portfolioForm.reset();
            itemsContainer.innerHTML = '';
            portfolioModal.classList.remove('hidden');
        });
    }

    if (closeModalBtn) {
        closeModalBtn.addEventListener('click', () => {
            portfolioModal.classList.add('hidden');
        });
    }

    if (portfolioForm) {
        portfolioForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const id = document.getElementById('portfolio-id').value;
            const name = document.getElementById('portfolio-name').value;
            const description = document.getElementById('portfolio-description').value;

            const items = Array.from(itemsContainer.children).map(div => ({
                type: div.querySelector('.portfolio-type').value,
                label: div.querySelector('.portfolio-label').value,
                address: div.querySelector('.portfolio-address').value
            }));

            const method = id ? 'PUT' : 'POST';
            const url = id ? `/api/v1/user/portfolios/${id}` : '/api/v1/user/portfolios';

            try {
                await authFetch(url, {
                    method,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name, description, items })
                });
                portfolioModal.classList.add('hidden');
                loadPortfolios();
            } catch (err) {
                alert('Failed to save portfolio');
            }
        });
    }

    window.editPortfolio = async (id) => {
        const res = await fetch('/api/v1/user/portfolios');
        const portfolios = await res.json();
        const p = portfolios.find(p => p.id === id);

        document.getElementById('modal-title').textContent = 'Edit Portfolio';
        document.getElementById('portfolio-id').value = p.id;
        document.getElementById('portfolio-name').value = p.name;
        document.getElementById('portfolio-description').value = p.description;

        itemsContainer.innerHTML = '';
        if (p.items) {
            p.items.forEach(item => {
                const itemDiv = document.createElement('div');
                itemDiv.className = 'flex gap-2 items-center';
                itemDiv.innerHTML = `
                    <select class="p-2 border rounded bg-bg text-text border-border portfolio-type">
                        <option value="stock" ${(item.type === 'stock' || !item.type) ? 'selected' : ''}>Stock</option>
                        <option value="crypto" ${item.type === 'crypto' ? 'selected' : ''}>Crypto</option>
                        <option value="bond" ${item.type === 'bond' ? 'selected' : ''}>Bond</option>
                        <option value="commodity" ${item.type === 'commodity' ? 'selected' : ''}>Commodity</option>
                    </select>
                    <input type="text" value="${item.label}" class="flex-1 p-2 border rounded bg-bg text-text border-border portfolio-label">
                    <input type="text" value="${item.address}" class="flex-[2] p-2 border rounded bg-bg text-text border-border portfolio-address">
                    <button type="button" class="portfolio-remove-item-btn text-red-600 px-2" aria-label="Remove row">×</button>
                `;
                itemsContainer.appendChild(itemDiv);
            });
        }
        portfolioModal.classList.remove('hidden');
    };

    window.deletePortfolio = async (id) => {
        if (!confirm('Are you sure you want to delete this portfolio?')) return;
        try {
            await authFetch(`/api/v1/user/portfolios/${id}`, { method: 'DELETE' });
            loadPortfolios();
        } catch (err) {
            alert('Failed to delete portfolio');
        }
    };
