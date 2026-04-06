        const API_BASE = '/api/v1';

        // CSRF helpers
        const CSRF_HEADER_NAME = 'X-CSRF-Token';
        let csrfToken = null;

        function setCSRFToken(token) {
            if (!token) return;
            csrfToken = token;
            try {
                localStorage.setItem('csrfToken', token);
            } catch (e) {
                // ignore storage errors
            }
        }

        function getCSRFToken() {
            if (csrfToken) return csrfToken;
            try {
                const stored = localStorage.getItem('csrfToken');
                csrfToken = stored;
                return stored;
            } catch (e) {
                return null;
            }
        }

        async function authFetch(url, options = {}) {
            const token = getCSRFToken();
            const opts = { ...options };
            opts.headers = opts.headers ? { ...opts.headers } : {};
            if (token) {
                opts.headers[CSRF_HEADER_NAME] = token;
            }
            return fetch(url, opts);
        }

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

        // Initialize theme on page load
        document.addEventListener('DOMContentLoaded', function() {
            initializeTheme();
            updateTheme();
        });

        // Listen for changes
        if (window.matchMedia) {
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', updateTheme);
        }

        // Debounce helper
        // Animation helpers for search results
        function animateIn(element) {
            if (!element) return;
            element.style.display = 'block';
            element.classList.remove('fade-out');
            element.classList.add('fade-in');
            // Force reflow
            element.offsetHeight;
        }

        function animateOut(element) {
            if (!element) return;
            element.classList.remove('fade-in');
            element.classList.add('fade-out');
            // Hide after animation completes
            setTimeout(() => {
                element.style.display = 'none';
                element.classList.remove('fade-out');
            }, 200);
        }
        function debounce(func, wait) {
            let timeout;
            return function(...args) {
                clearTimeout(timeout);
                timeout = setTimeout(() => func.apply(this, args), wait);
            };
        }

        const debouncedSearch = debounce(function(query) {
            performSearch(query);
        }, 500);

        // Small state for current query/abort controller
        let currentQuery = null;
        let currentAbortController = null;
        let autocompleteController = null;
        let autocompleteSelectedIndex = -1; // index in current suggestions
        let currentSuggestions = [];
        let currentAddressData = null;
        let currentPage = 1;

        // History state
        let history = [];

        // Load history from localStorage
        function loadHistory() {
            const stored = localStorage.getItem('bitcoinExplorerHistory');
            if (stored) {
                try {
                    history = JSON.parse(stored);
                } catch (e) {
                    console.error('Failed to parse history from localStorage', e);
                    history = [];
                }
            }
        }

        // Save history to localStorage
        function saveHistory() {
            try {
                localStorage.setItem('bitcoinExplorerHistory', JSON.stringify(history));
            } catch (e) {
                console.error('Failed to save history to localStorage', e);
            }
        }

        // Add item to history
        function addToHistory(type, id, label) {
            // Remove if already exists
            history = history.filter(item => !(item.type === type && item.id === id));
            // Add to beginning
            history.unshift({ type, id, label, time: Date.now() });
            // Limit to 10
            if (history.length > 10) {
                history = history.slice(0, 10);
            }
            saveHistory();
            renderHistory();
        }

        // Render history list
        function renderHistory() {
            const list = document.getElementById('history-list');
            if (!list) return;
            list.innerHTML = '';
            if (history.length === 0) {
                list.innerHTML = '<li class="text-gray-500">No recent views</li>';
                const section = document.getElementById('history-section');
                if (section) section.style.display = 'none';
                return;
            }
            history.forEach(item => {
                const li = document.createElement('li');
                const a = document.createElement('a');
                a.href = `/bitcoin?q=${encodeURIComponent(item.id)}`;
                a.textContent = `${item.type}: ${item.label}`;
                a.className = 'high-contrast-link hover:underline block';
                a.setAttribute('aria-label', `View ${item.type} ${item.id}`);
                li.appendChild(a);
                list.appendChild(li);
            });
            const section = document.getElementById('history-section');
            if (section) section.style.display = 'block';
        }

        // Pagination state for address transactions
        const ITEMS_PER_PAGE = 10

        // Debounced autocomplete trigger (shorter delay)
        const debouncedAutocomplete = debounce(function(query, source) {
            fetchAutocomplete(query, source);
        }, 250);

        document.addEventListener('DOMContentLoaded', function() {
            // Attach search handlers if elements exist
            const searchInput = document.querySelector('#search-input');
            const searchButton = document.querySelector('#search-icon');
            const searchForm = document.querySelector('#search-form');

            const mobileSearchInput = document.querySelector('#search-input-mobile');
            const mobileSearchButton = document.querySelector('#search-icon-mobile');
            const mobileSearchForm = document.querySelector('#search-form-mobile');

            // Desktop: handle form submit (Enter or button submit)
            if (searchForm) {
                searchForm.addEventListener('submit', function(e) {
                    e.preventDefault();
                    const q = (searchInput && searchInput.value) ? searchInput.value.trim() : '';
                    if (q) performSearch(q);
                });

                // Also attach autocomplete handlers when form exists
                if (searchInput) {
                    // handle Enter for search AND navigation for suggestions
                    searchInput.addEventListener('keydown', function(e) {
                        if (e.key === 'Enter') {
                            if (autocompleteSelectedIndex >= 0 && currentSuggestions[autocompleteSelectedIndex]) {
                                selectSuggestion(currentSuggestions[autocompleteSelectedIndex], searchInput, 'desktop');
                                e.preventDefault();
                                return;
                            }
                            debouncedSearch(e.target.value);
                        } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Escape') {
                            handleSuggestionKey(e, 'desktop');
                        }
                    });
                    // input event for autocomplete
                    searchInput.addEventListener('input', function(e) {
                        const q = e.target.value.trim();
                        if (!q) {
                            hideSuggestions('desktop');
                            return;
                        }
                        debouncedAutocomplete(q, 'desktop');
                    });
                    searchInput.addEventListener('blur', function(e) {
                        // small delay to allow clicks
                        setTimeout(() => hideSuggestions('desktop'), 150);
                    });
                }

                if (searchButton) {
                    searchButton.addEventListener('click', function() {
                        const query = document.querySelector('#search-input').value;
                        debouncedSearch(query);
                    });
                }
            } else {
                // If searchForm is not present (legacy layout), still wire basic handlers
                if (searchInput) {
                    searchInput.addEventListener('keydown', function(e) {
                        if (e.key === 'Enter') debouncedSearch(e.target.value);
                    });
                }
                if (searchButton) {
                    searchButton.addEventListener('click', function() {
                        const query = document.querySelector('#search-input').value;
                        debouncedSearch(query);
                    });
                }
            }

            // Mobile: form submit
            if (mobileSearchForm) {
                mobileSearchForm.addEventListener('submit', function(e) {
                    e.preventDefault();
                    const q = (mobileSearchInput && mobileSearchInput.value) ? mobileSearchInput.value.trim() : '';
                    // close mobile menu for better UX
                    const mobileMenu = document.getElementById('mobile-menu');
                    if (mobileMenu) mobileMenu.classList.add('hidden');
                    if (q) performSearch(q);
                });

                // Mobile autocomplete handlers
                if (mobileSearchInput) {
                    mobileSearchInput.addEventListener('keydown', function(e) {
                        if (e.key === 'Enter') {
                            if (autocompleteSelectedIndex >= 0 && currentSuggestions[autocompleteSelectedIndex]) {
                                selectSuggestion(currentSuggestions[autocompleteSelectedIndex], mobileSearchInput, 'mobile');
                                e.preventDefault();
                                return;
                            }
                            debouncedSearch(e.target.value);
                        } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Escape') {
                            handleSuggestionKey(e, 'mobile');
                        }
                    });
                    mobileSearchInput.addEventListener('input', function(e) {
                        const q = e.target.value.trim();
                        if (!q) {
                            hideSuggestions('mobile');
                            return;
                        }
                        debouncedAutocomplete(q, 'mobile');
                    });
                    mobileSearchInput.addEventListener('blur', function(e) {
                        setTimeout(() => hideSuggestions('mobile'), 150);
                    });
                }

                if (mobileSearchButton) {
                    mobileSearchButton.addEventListener('click', function() {
                        const query = document.querySelector('#search-input-mobile').value;
                        // close mobile menu for better UX
                        const mobileMenu = document.getElementById('mobile-menu');
                        if (mobileMenu) mobileMenu.classList.add('hidden');
                        debouncedSearch(query);
                    });
                }
            } else {
                if (mobileSearchInput) {
                    mobileSearchInput.addEventListener('keydown', function(e) {
                        if (e.key === 'Enter') {
                            debouncedSearch(e.target.value);
                        }
                    });
                }
                if (mobileSearchButton) {
                    mobileSearchButton.addEventListener('click', function() {
                        const query = document.querySelector('#search-input-mobile').value;
                        // close mobile menu for better UX
                        const mobileMenu = document.getElementById('mobile-menu');
                        if (mobileMenu) mobileMenu.classList.add('hidden');
                        debouncedSearch(query);
                    });
                }
            }

            // Mobile menu toggling and accessibility
            const mobileMenuButton = document.getElementById('mobile-menu-button');
            const mobileMenu = document.getElementById('mobile-menu');
            const mainContent = document.getElementById('main-content');
            const mobileMenuClose = document.getElementById('mobile-menu-close');
            const mobileOverlay = mobileMenu ? mobileMenu.querySelector('.mobile-menu-overlay') : null;

            function openMobileMenu() {
                if (!mobileMenu) return;
                // Remove hidden so element becomes part of layout, then add menu-visible to trigger CSS transition
                mobileMenu.classList.remove('hidden');

                // allow layout to settle, then add visible class
                // force reflow
                void mobileMenu.offsetWidth;
                mobileMenu.classList.add('menu-visible');

                mobileMenuButton.setAttribute('aria-expanded', 'true');
                mobileMenu.setAttribute('aria-hidden', 'false');

                // mark main content as hidden for screen readers and prevent background scroll
                if (mainContent) mainContent.setAttribute('aria-hidden', 'true');
                document.body.style.overflow = 'hidden';

                // focus first interactive item; prefer close button for easy dismissal
                if (mobileMenuClose) mobileMenuClose.focus();
                else {
                    const firstLink = mobileMenu.querySelector('a');
                    if (firstLink) firstLink.focus();
                }

                document.addEventListener('click', onDocClick);
                document.addEventListener('keydown', onKeyDown);
            }

            function closeMobileMenu() {
                if (!mobileMenu) return;

                // start transition by removing menu-visible
                mobileMenu.classList.remove('menu-visible');
                mobileMenuButton.setAttribute('aria-expanded', 'false');
                mobileMenu.setAttribute('aria-hidden', 'true');

                // restore main content visibility and scrolling
                if (mainContent) mainContent.setAttribute('aria-hidden', 'false');
                document.body.style.overflow = '';

                // After transition ends, hide the element completely to remove from accessibility tree
                if (mobileOverlay && typeof window !== 'undefined') {
                    const onEnd = function(e) {
                        // ensure callback runs only for overlay transitions
                        if (e.target !== mobileOverlay) return;
                        mobileMenu.classList.add('hidden');
                        mobileOverlay.removeEventListener('transitionend', onEnd);
                    };
                    mobileOverlay.addEventListener('transitionend', onEnd);

                    // Fallback: in case transitionend doesn't fire, ensure hidden after timeout
                    setTimeout(() => {
                        if (!mobileMenu.classList.contains('hidden')) mobileMenu.classList.add('hidden');
                    }, 300);
                } else {
                    mobileMenu.classList.add('hidden');
                }

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

            // Enhanced keydown handler: Escape to close, and trap focus while mobile menu is open
            function onKeyDown(e) {
                if (!mobileMenu || mobileMenu.classList.contains('hidden')) return;

                if (e.key === 'Escape') {
                    closeMobileMenu();
                    return;
                }

                if (e.key === 'Tab') {
                    // Focus trap inside mobile menu
                    const focusableSelector = 'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])';
                    const focusable = Array.prototype.slice.call(mobileMenu.querySelectorAll(focusableSelector)).filter(function(el) {
                        return el.offsetWidth > 0 || el.offsetHeight > 0 || el === document.activeElement;
                    });
                    if (focusable.length === 0) {
                        e.preventDefault();
                        return;
                    }
                    const first = focusable[0];
                    const last = focusable[focusable.length - 1];

                    if (e.shiftKey) {
                        // shift + tab
                        if (document.activeElement === first) {
                            e.preventDefault();
                            last.focus();
                        }
                    } else {
                        // tab
                        if (document.activeElement === last) {
                            e.preventDefault();
                            first.focus();
                        }
                    }
                }
            }

            if (mobileMenuButton) {
                mobileMenuButton.addEventListener('click', function(e) {
                    e.stopPropagation();
                    toggleMobileMenu();
                });
            }

            if (mobileMenuClose) {
                mobileMenuClose.addEventListener('click', function(e) {
                    e.stopPropagation();
                    closeMobileMenu();
                });
            }

            // Get query parameter
            const urlParams = new URLSearchParams(window.location.search);
            const query = urlParams.get('q');

            if (!query) {
                // No query provided: show helpful message instead of failing silently
                showError('No search query provided. Use the search in the header to look up a block, tx or address.');
                return;
            }

            // If there is a query in the URL, perform the search
            fetchSearch(query);
        });

        // UI helpers: loading and error states
        function showLoading(message = 'Loading data...') {
            const loadingEl = document.getElementById('loading');
            if (!loadingEl) return;
            const loadingText = document.getElementById('loading-text');
            if (loadingText) loadingText.textContent = message;
            loadingEl.style.display = 'flex';

            // hide content & error while loading
            const content = document.getElementById('content');
            if (content) content.style.display = 'none';
            const errorEl = document.getElementById('error');
            if (errorEl) errorEl.style.display = 'none';
        }

        function hideLoading() {
            const loadingEl = document.getElementById('loading');
            if (!loadingEl) return;
            loadingEl.style.display = 'none';
        }

        function clearError() {
            const errorEl = document.getElementById('error');
            if (!errorEl) return;
            errorEl.innerHTML = '';
            errorEl.style.display = 'none';
        }

        function showError(message, canRetry = false, retryCallback = null) {
            hideLoading();
            // hide content when showing an error
            const content = document.getElementById('content');
            if (content) content.style.display = 'none';

            const errorEl = document.getElementById('error');
            if (!errorEl) return;
            errorEl.innerHTML = '';

            const p = document.createElement('p');
            p.textContent = message;
            errorEl.appendChild(p);

            if (canRetry && typeof retryCallback === 'function') {
                const btn = document.createElement('button');
                btn.textContent = 'Retry';
                btn.className = 'mt-2 btn-primary';
                btn.addEventListener('click', function() {
                    // clear error and run retry
                    clearError();
                    retryCallback();
                });
                errorEl.appendChild(btn);
            }

            errorEl.style.display = 'block';
            errorEl.setAttribute('role', 'alert');
            errorEl.setAttribute('aria-live', 'assertive');
        }

        // Perform search: update URL and fetch in-place
        function performSearch(query) {
            if (!query) return;
            // update URL without reload
            try {
                const newUrl = `${window.location.pathname}?q=${encodeURIComponent(query)}`;
                window.history.pushState({}, '', newUrl);
            } catch (e) {
                // ignore history errors
            }
            fetchSearch(query);
        }

        // Fetch autocomplete suggestions from the API
        function fetchAutocomplete(query, source = 'desktop') {
            // cancel previous autocomplete request
            if (autocompleteController) {
                try { autocompleteController.abort(); } catch (e) {}
                autocompleteController = null;
            }
            autocompleteController = new AbortController();
            const signal = autocompleteController.signal;

            fetch(`${API_BASE}/autocomplete?q=${encodeURIComponent(query)}`, { signal })
                .then(resp => {
                    if (!resp.ok) return { suggestions: [] };
                    return resp.json();
                })
                .then(data => {
                    // expected response: { suggestions: [ {type: 'address'|'tx'|'block', value: '...', label: '...'}, ... ] }
                    const suggestions = (data && data.suggestions) ? data.suggestions : [];
                    currentSuggestions = suggestions;
                    autocompleteSelectedIndex = -1;
                    renderSuggestions(suggestions, source);
                })
                .catch(err => {
                    // ignore abort errors
                })
                .finally(() => {
                    autocompleteController = null;
                });
        }

        function renderSuggestions(suggestions, source) {
            const container = document.getElementById(source === 'mobile' ? 'autocomplete-mobile' : 'autocomplete-desktop');
            if (!container) return;
            container.innerHTML = '';
            if (!suggestions || suggestions.length === 0) {
                container.style.display = 'none';
                return;
            }
            suggestions.forEach((s, idx) => {
                const item = document.createElement('div');
                item.className = 'autocomplete-item';
                item.setAttribute('role', 'option');
                item.setAttribute('data-idx', idx);
                item.setAttribute('aria-selected', 'false');

                // Type badge
                const type = document.createElement('span');
                type.className = 'autocomplete-type';
                type.textContent = s.type || '';
                item.appendChild(type);

                const text = document.createElement('span');
                text.textContent = s.label || s.value || '';
                item.appendChild(text);

                item.addEventListener('mousedown', function(e) {
                    // mousedown so input blur doesn't hide before click
                    e.preventDefault();
                    const idx = parseInt(item.getAttribute('data-idx'), 10);
                    const sel = currentSuggestions[idx];
                    const input = (source === 'mobile') ? document.getElementById('search-input-mobile') : document.getElementById('search-input');
                    if (sel) selectSuggestion(sel, input, source);
                });

                container.appendChild(item);
            });
            container.style.display = 'block';
        }

        function hideSuggestions(source) {
            const container = document.getElementById(source === 'mobile' ? 'autocomplete-mobile' : 'autocomplete-desktop');
            if (container) container.style.display = 'none';
            currentSuggestions = [];
            autocompleteSelectedIndex = -1;
        }

        function handleSuggestionKey(e, source) {
            const max = currentSuggestions.length;
            if (max === 0) return;
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                autocompleteSelectedIndex = (autocompleteSelectedIndex + 1) % max;
                updateSuggestionHighlight(source);
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                autocompleteSelectedIndex = (autocompleteSelectedIndex - 1 + max) % max;
                updateSuggestionHighlight(source);
            } else if (e.key === 'Escape') {
                hideSuggestions(source);
            }
        }

        function updateSuggestionHighlight(source) {
            const container = document.getElementById(source === 'mobile' ? 'autocomplete-mobile' : 'autocomplete-desktop');
            if (!container) return;
            const items = Array.from(container.querySelectorAll('.autocomplete-item'));
            items.forEach((it, i) => {
                const sel = (i === autocompleteSelectedIndex);
                it.setAttribute('aria-selected', sel ? 'true' : 'false');
                if (sel) {
                    // Keyboard nav: preview label in the input; search runs on pick/Enter, not here.
                    const input = (source === 'mobile') ? document.getElementById('search-input-mobile') : document.getElementById('search-input');
                    if (input && currentSuggestions[i]) input.value = currentSuggestions[i].value || currentSuggestions[i].label || input.value;
                    // ensure visible
                    it.scrollIntoView({ block: 'nearest' });
                }
            });
        }

        function selectSuggestion(suggestion, inputEl, source) {
            if (!suggestion || !inputEl) return;
            // Set the input to the selected value (use the value field if provided)
            inputEl.value = suggestion.value || suggestion.label || inputEl.value;
            hideSuggestions(source);
            // Trigger full search for the selected suggestion
            performSearch(inputEl.value);
        }

        // Main fetch logic with timeout and retry support
        function fetchSearch(query) {
            currentQuery = query;
            clearError();

            // Abort any previous request
            if (currentAbortController) {
                try { currentAbortController.abort(); } catch (e) {}
                currentAbortController = null;
            }

            const controller = new AbortController();
            currentAbortController = controller;
            const signal = controller.signal;

            const TIMEOUT_MS = 10000; // 10s
            const timeoutId = setTimeout(() => {
                if (currentAbortController) currentAbortController.abort();
            }, TIMEOUT_MS);

            showLoading('Loading data...');

            fetch(`${API_BASE}/search?q=${encodeURIComponent(query)}`, { signal })
                .then(response => {
                    clearTimeout(timeoutId);
                    if (!response.ok) {
                        // try to parse error message from JSON
                        return response.text().then(txt => {
                            let msg = `API request failed (${response.status})`;
                            try {
                                const j = JSON.parse(txt);
                                if (j && j.error) msg = j.error;
                            } catch (e) {
                                if (txt) msg = txt;
                            }
                            throw new Error(msg);
                        });
                    }
                    return response.json();
                })
                .then(data => {
                    hideLoading();
                    const content = document.getElementById('content');
                    if (content) content.style.display = 'block';

                    // Hide all detail sections first
                    const addrSection = document.getElementById('address-details');
                    const txSection = document.getElementById('transaction-details');
                    const blockSection = document.getElementById('block-details');
                    if (addrSection) addrSection.style.display = 'none';
                    if (txSection) txSection.style.display = 'none';
                    if (blockSection) blockSection.style.display = 'none';

                    // Display data based on result type
                    if (data.resultType === 'address') {
                        displayAddressData(data.data);
                    } else if (data.resultType === 'transaction') {
                        displayTransactionData(data.data);
                    } else if (data.resultType === 'block') {
                        displayBlockData(data.data);
                    } else {
                        showError('Unknown result type returned by API.');
                    }
                })
                .catch(error => {
                    // differentiate abort/timeouts from other errors
                    if (error && error.name === 'AbortError') {
                        showError('Request timed out. Please check your connection and try again.', true, () => fetchSearch(query));
                    } else {
                        console.error('Error:', error);
                        const msg = (error && error.message) ? error.message : 'Failed to fetch data from the API';
                        showError(msg, true, () => fetchSearch(query));
                    }
                })
                .finally(() => {
                    currentAbortController = null;
                });
        }

        function displayAddressData(data, page = 1) {
            // Hide other sections with animation
            const txEl = document.getElementById('transaction-details');
            const blockEl = document.getElementById('block-details');

            if (txEl && txEl.style.display !== 'none') {
                animateOut(txEl);
            }
            if (blockEl && blockEl.style.display !== 'none') {
                animateOut(blockEl);
            }

            // Show address details section with animation
            const addrEl = document.getElementById('address-details');
            if (addrEl) {
                // Wait for other animations to complete, then animate in
                setTimeout(() => {
                    animateIn(addrEl);
                }, txEl && txEl.style.display !== 'none' || blockEl && blockEl.style.display !== 'none' ? 200 : 0);
            }

            // Extract data from response
            const addressData = data.result || {};
            currentAddressData = addressData;
            currentPage = page;

            // Add to history
            addToHistory('address', addressData.address, shortenHash(addressData.address));

            // Populate address details
            document.getElementById('address').textContent = addressData.address || 'N/A';
            document.getElementById('balance').textContent = formatBTC(addressData.balance || 0);
            if (document.getElementById('received')) document.getElementById('received').textContent = formatBTC(addressData.total_received || 0);
            if (document.getElementById('sent')) document.getElementById('sent').textContent = formatBTC(addressData.total_sent || 0);
            document.getElementById('tx-count').textContent = addressData.transactions ? addressData.transactions.length : 0;

            // Generate QR code
            const qrCodeElement = document.getElementById('qr-code');
            qrCodeElement.innerHTML = '<canvas id="qr-canvas"></canvas>';
            QRCode.toCanvas(document.getElementById('qr-canvas'), addressData.address, function (error) {
                if (error) console.error('QR Code generation failed:', error);
            });

            // Populate transactions
            const transactionsContainer = document.getElementById('transactions');
            transactionsContainer.innerHTML = '';

            const transactions = addressData.transactions || [];
            const totalPages = Math.ceil(transactions.length / ITEMS_PER_PAGE);
            const start = (page - 1) * ITEMS_PER_PAGE;
            const end = start + ITEMS_PER_PAGE;
            const pageTransactions = transactions.slice(start, end);

            if (pageTransactions.length > 0) {
                pageTransactions.forEach(tx => {
                    const row = document.createElement('tr');

                    const tdHash = document.createElement('td');
                    tdHash.className = 'p-2';
                    const a = document.createElement('a');
                    a.href = `/bitcoin?q=${encodeURIComponent(tx.txid)}`;
                    a.textContent = shortenHash(tx.txid);
                    a.className = 'high-contrast-link hover:underline';
                    a.setAttribute('aria-label', `Open transaction ${tx.txid}`);
                    tdHash.appendChild(a);
                    const copyBtn = document.createElement('button');
                    copyBtn.className = 'copy-btn';
                    copyBtn.textContent = 'Copy';
                    copyBtn.onclick = () => copyToClipboard(tx.txid);
                    tdHash.appendChild(copyBtn);

                    const tdAmt = document.createElement('td');
                    tdAmt.className = 'p-2';
                    tdAmt.textContent = formatBTC(tx.value || 0);

                    const tdStatus = document.createElement('td');
                    tdStatus.className = 'p-2';
                    tdStatus.textContent = `Confirmations: ${tx.confirmations || 0}`;

                    row.appendChild(tdHash);
                    row.appendChild(tdAmt);
                    row.appendChild(tdStatus);

                    transactionsContainer.appendChild(row);
                });
            } else {
                const row = document.createElement('tr');
                const td = document.createElement('td');
                td.setAttribute('colspan', '3');
                td.className = 'p-2';
                td.textContent = 'No transactions found';
                row.appendChild(td);
                transactionsContainer.appendChild(row);
            }

            // Handle pagination
            const paginationEl = document.getElementById('pagination');
            if (totalPages > 1) {
                paginationEl.style.display = 'flex';
                document.getElementById('page-info').textContent = `Page ${page} of ${totalPages}`;
                document.getElementById('prev-page').disabled = page === 1;
                document.getElementById('next-page').disabled = page === totalPages;
            } else {
                paginationEl.style.display = 'none';
            }
            if (window.__showWatchlistAddWrap) window.__showWatchlistAddWrap({ type: "address", address: addressData.address || "" });
        }

        function displayTransactionData(data) {
            // Hide other sections with animation
            const addrEl = document.getElementById('address-details');
            const blockEl = document.getElementById('block-details');

            if (addrEl && addrEl.style.display !== 'none') {
                animateOut(addrEl);
            }
            if (blockEl && blockEl.style.display !== 'none') {
                animateOut(blockEl);
            }

            // Show transaction details section with animation
            const txEl = document.getElementById('transaction-details');
            if (txEl) {
                // Wait for other animations to complete, then animate in
                setTimeout(() => {
                    animateIn(txEl);
                }, addrEl && addrEl.style.display !== 'none' || blockEl && blockEl.style.display !== 'none' ? 200 : 0);
            }

            // Extract data from response
            const txData = data.result || {};

            // Add to history
            addToHistory('transaction', txData.txid, shortenHash(txData.txid));

            // Populate transaction details
            document.getElementById('tx-hash').textContent = txData.txid || 'N/A';
            document.getElementById('tx-block').textContent = txData.blockhash || 'Pending';
            document.getElementById('tx-time').textContent = txData.time ? new Date(txData.time * 1000).toLocaleString() : 'Pending';
            document.getElementById('tx-amount').textContent = formatBTC(txData.value || 0);
            document.getElementById('tx-fee').textContent = formatBTC(txData.fee || 0);
            document.getElementById('tx-confirmations').textContent = txData.confirmations || 0;

            // Handle inputs and outputs
            let fromAddresses = [];
            let toAddresses = [];

            if (txData.vin && txData.vin.length > 0) {
                txData.vin.forEach(input => { if (input.address) fromAddresses.push(input.address); });
            }
            if (txData.vout && txData.vout.length > 0) {
                txData.vout.forEach(output => { if (output.scriptPubKey && output.scriptPubKey.addresses) toAddresses = toAddresses.concat(output.scriptPubKey.addresses); });
            }

            // Populate 'From' list with accessible links
            const fromContainer = document.getElementById('tx-from');
            fromContainer.innerHTML = '';
            if (fromAddresses.length > 0) {
                fromAddresses.forEach((addr, idx) => {
                    const a = document.createElement('a');
                    a.href = `/bitcoin?q=${encodeURIComponent(addr)}`;
                    a.textContent = addr;
                    a.className = 'high-contrast-link hover:underline';
                    a.setAttribute('aria-label', `Open address ${addr}`);
                    fromContainer.appendChild(a);
                    const copyBtn = document.createElement('button');
                    copyBtn.className = 'copy-btn';
                    copyBtn.textContent = 'Copy';
                    copyBtn.onclick = () => copyToClipboard(addr);
                    fromContainer.appendChild(copyBtn);
                    if (idx < fromAddresses.length - 1) fromContainer.appendChild(document.createElement('br'));
                });
            } else {
                fromContainer.textContent = 'Coinbase (New Coins)';
            }

            // Populate 'To' list with accessible links
            const toContainer = document.getElementById('tx-to');
            toContainer.innerHTML = '';
            if (toAddresses.length > 0) {
                toAddresses.forEach((addr, idx) => {
                    const a = document.createElement('a');
                    a.href = `/bitcoin?q=${encodeURIComponent(addr)}`;
                    a.textContent = addr;
                    a.className = 'high-contrast-link hover:underline';
                    a.setAttribute('aria-label', `Open address ${addr}`);
                    toContainer.appendChild(a);
                    const copyBtn = document.createElement('button');
                    copyBtn.className = 'copy-btn';
                    copyBtn.textContent = 'Copy';
                    copyBtn.onclick = () => copyToClipboard(addr);
                    toContainer.appendChild(copyBtn);
                    if (idx < toAddresses.length - 1) toContainer.appendChild(document.createElement('br'));
                });
            } else {
                toContainer.textContent = 'N/A';
            }
            if (window.__showWatchlistAddWrap) window.__showWatchlistAddWrap({ type: "address", address: txData.txid || "" });
        }

        function displayBlockData(data) {
            // Hide other sections with animation
            const addrEl = document.getElementById('address-details');
            const txEl = document.getElementById('transaction-details');

            if (addrEl && addrEl.style.display !== 'none') {
                animateOut(addrEl);
            }
            if (txEl && txEl.style.display !== 'none') {
                animateOut(txEl);
            }

            // Show block details section with animation
            const blockEl = document.getElementById('block-details');
            if (blockEl) {
                // Wait for other animations to complete, then animate in
                setTimeout(() => {
                    animateIn(blockEl);
                }, addrEl && addrEl.style.display !== 'none' || txEl && txEl.style.display !== 'none' ? 200 : 0);
            }

            // Extract data from response
            const blockData = data.result || {};

            // Add to history
            addToHistory('block', blockData.hash, `Block ${blockData.height}`);

            // Populate block details
            document.getElementById('block-height').textContent = blockData.height || 'N/A';
            document.getElementById('block-hash').textContent = blockData.hash || 'N/A';
            document.getElementById('block-time').textContent = blockData.time ? new Date(blockData.time * 1000).toLocaleString() : 'N/A';
            document.getElementById('block-tx-count').textContent = blockData.tx ? blockData.tx.length : 0;
            document.getElementById('block-size').textContent = blockData.size ? `${blockData.size.toLocaleString()} bytes` : 'N/A';
            document.getElementById('block-weight').textContent = blockData.weight ? blockData.weight.toLocaleString() : 'N/A';
            // Added miner display
            document.getElementById('block-miner').textContent = blockData.miner || 'N/A';
            document.getElementById('block-weight').textContent = blockData.weight ? blockData.weight.toLocaleString() : 'N/A';
            // Added miner display
            document.getElementById('block-miner').textContent = blockData.miner || 'N/A';

            // Populate transactions list
            const txContainer = document.getElementById('block-transactions');
            txContainer.innerHTML = '';
            if (blockData.tx && blockData.tx.length > 0) {
                blockData.tx.forEach(txid => {
                    const a = document.createElement('a');
                    a.href = `/bitcoin?q=${encodeURIComponent(txid)}`;
                    a.textContent = shortenHash(txid);
                    a.className = 'high-contrast-link hover:underline block';
                    a.setAttribute('aria-label', `Open transaction ${txid}`);
                    txContainer.appendChild(a);
                    const copyBtn = document.createElement('button');
                    copyBtn.className = 'copy-btn';
                    copyBtn.textContent = 'Copy';
                    copyBtn.onclick = () => copyToClipboard(txid);
                    txContainer.appendChild(copyBtn);
                });
            } else {
                txContainer.textContent = 'No transactions';
            }
            if (window.__showWatchlistAddWrap) window.__showWatchlistAddWrap({ type: "address", address: blockData.hash || "" });
        }

        function loadNetworkStatus() {
            fetch(`${API_BASE}/network-status`)
                .then(response => response.json())
                .then(data => {
                    document.getElementById('network-block-height').textContent = data.block_height || 'N/A';
                    document.getElementById('network-difficulty').textContent = data.difficulty ? data.difficulty.toFixed(2) : 'N/A';
                    document.getElementById('network-hash-rate').textContent = data.hash_rate ? (data.hash_rate / 1e18).toFixed(2) : 'N/A'; // Convert to EH/s
                    const networkSection = document.getElementById('network-status-section');
                    if (networkSection) networkSection.style.display = 'block';
                })
                .catch(err => {
                    console.error('Failed to load network status:', err);
                });
        }

        function formatBTC(value) {
            return `${parseFloat(value).toFixed(8)} BTC`;
        }

        function shortenHash(hash) {
            if (!hash) return 'N/A';
            return hash.substring(0, 10) + '...' + hash.substring(hash.length - 10);
        }

        function copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(() => {
                console.log('Copied to clipboard');
            }).catch(err => {
                console.error('Failed to copy: ', err);
            });
        }

        function shareResult() {
            const url = window.location.href;
            copyToClipboard(url);
            // Optionally, show a toast or alert
            alert('URL copied to clipboard!');
        }

        // Pagination event listeners
        const prevPage = document.getElementById("prev-page");
        const nextPage = document.getElementById("next-page");
        if (prevPage) prevPage.addEventListener("click", function() {
            if (currentPage > 1) displayAddressData({ result: currentAddressData }, currentPage - 1);
        });
        if (nextPage) nextPage.addEventListener("click", function() {
            const transactions = currentAddressData.transactions || [];
            const totalPages = Math.ceil(transactions.length / ITEMS_PER_PAGE);
            if (currentPage < totalPages) displayAddressData({ result: currentAddressData }, currentPage + 1);
        });

        // Share button event listeners
        const shareAddress = document.getElementById("share-address");
        const shareTx = document.getElementById("share-tx");
        const shareBlock = document.getElementById("share-block");
        if (shareAddress) shareAddress.addEventListener("click", shareResult);
        if (shareTx) shareTx.addEventListener("click", shareResult);
        if (shareBlock) shareBlock.addEventListener("click", shareResult);

        // Watchlist "Add to watchlist" — show wrap and set target when a result is displayed
        window.__watchlistTarget = null;
        window.__showWatchlistAddWrap = function(target) {
            window.__watchlistTarget = target;
            const wrap = document.getElementById("watchlist-add-wrap");
            if (wrap) wrap.classList.remove("hidden");
        };
        (function initWatchlistAdd() {
            const btn = document.getElementById("watchlist-add-btn");
            const dropdown = document.getElementById("watchlist-dropdown");
            const msgEl = document.getElementById("watchlist-add-message");
            const errEl = document.getElementById("watchlist-add-error");
            if (!btn || !dropdown) return;
            function hideMessages() {
                if (msgEl) msgEl.classList.add("hidden");
                if (errEl) errEl.classList.add("hidden");
            }
            function closeDropdown() {
                dropdown.classList.add("hidden");
                btn.setAttribute("aria-expanded", "false");
            }
            btn.addEventListener("click", function(e) {
                e.stopPropagation();
                hideMessages();
                const target = window.__watchlistTarget;
                if (!target || !target.address) return;
                if (dropdown.classList.contains("hidden")) {
                    fetch(API_BASE + "/user/watchlists?page_size=100", { credentials: "include" })
                        .then(function(res) {
                            if (res.status === 401) {
                                if (errEl) {
                                    errEl.textContent = "Sign in to add to watchlist.";
                                    errEl.classList.remove("hidden");
                                }
                                return [];
                            }
                            if (!res.ok) return [];
                            return res.json();
                        })
                        .then(function(data) {
                            const list = (data && data.data) ? data.data : (Array.isArray(data) ? data : []);
                            dropdown.innerHTML = "";
                            if (list.length === 0 && !errEl.classList.contains("hidden")) return;
                            if (list.length === 0) {
                                if (errEl) {
                                    errEl.textContent = "No watchlists. Create one in Profile.";
                                    errEl.classList.remove("hidden");
                                }
                                return;
                            }
                            list.forEach(function(w) {
                                const b = document.createElement("button");
                                b.type = "button";
                                b.className = "w-full text-left px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-gray-700";
                                b.textContent = w.name || ("Watchlist " + (w.id || ""));
                                b.setAttribute("role", "menuitem");
                                b.addEventListener("click", function(ev) {
                                    ev.stopPropagation();
                                    closeDropdown();
                                    const body = JSON.stringify({ type: "address", address: target.address });
                                    const headers = { "Content-Type": "application/json" };
                                    const token = getCSRFToken();
                                    if (token) headers["X-CSRF-Token"] = token;
                                    fetch(API_BASE + "/user/watchlists/" + w.id + "/entries", { method: "POST", headers: headers, body: body, credentials: "include" })
                                        .then(function(r) {
                                            if (r.ok) {
                                                if (msgEl) { msgEl.textContent = "Added to watchlist."; msgEl.classList.remove("hidden"); }
                                                setTimeout(hideMessages, 2500);
                                            } else {
                                                if (errEl) { errEl.textContent = "Failed to add."; errEl.classList.remove("hidden"); }
                                            }
                                        })
                                        .catch(function() {
                                            if (errEl) { errEl.textContent = "Request failed."; errEl.classList.remove("hidden"); }
                                        });
                                });
                                dropdown.appendChild(b);
                            });
                            dropdown.classList.remove("hidden");
                            btn.setAttribute("aria-expanded", "true");
                        })
                        .catch(function() {
                            if (errEl) { errEl.textContent = "Failed to load watchlists."; errEl.classList.remove("hidden"); }
                        });
                } else {
                    closeDropdown();
                }
            });
            document.addEventListener("click", closeDropdown);
        })();

        renderHistory();

    function loadCharts() {
        fetch(`${API_BASE}/metrics`)
            .then(response => response.json())
            .then(data => {
                renderMempoolChart(data.mempool_size);
                renderBlockTimeChart(data.block_times);
                renderTxVolumeChart(data.tx_volume);
                const chartsSection = document.getElementById('charts-section');
                if (chartsSection) chartsSection.style.display = 'block';
            })
            .catch(err => {
                console.error('Failed to load charts:', err);
            });
        function renderMempoolChart(data) {
        if (!data || !document.getElementById('mempoolChart')) return;
        const ctx = document.getElementById('mempoolChart').getContext('2d');
        const labels = data.map(d => new Date(d.time * 1000).toLocaleTimeString());
        const values = data.map(d => parseFloat(d.value));
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Mempool Size',
                    data: values,
                    borderColor: 'rgb(75, 192, 192)',
                    backgroundColor: 'rgba(75, 192, 192, 0.1)',
                    fill: true,
                    tension: 0.1
                }]
            },
            options: {
                responsive: true,
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    }

        function renderBlockTimeChart(data) {
        if (!data || !document.getElementById('blockTimeChart')) return;
        const ctx = document.getElementById('blockTimeChart').getContext('2d');
        const labels = data.map(d => new Date(d.time * 1000).toLocaleTimeString());
        const values = data.map(d => parseFloat(d.value));
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Block Time (s)',
                    data: values,
                    borderColor: 'rgb(255, 99, 132)',
                    backgroundColor: 'rgba(255, 99, 132, 0.08)',
                    fill: true,
                    tension: 0.1
                }]
            },
            options: {
                responsive: true,
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    }

        function renderTxVolumeChart(data) {
        if (!data || !document.getElementById('txVolumeChart')) return;
        const ctx = document.getElementById('txVolumeChart').getContext('2d');
        const labels = data.map(d => new Date(d.time * 1000).toLocaleTimeString());
        const values = data.map(d => parseFloat(d.value));
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Transaction Volume',
                    data: values,
                    borderColor: 'rgb(54, 162, 235)',
                    backgroundColor: 'rgba(54, 162, 235, 0.08)',
                    fill: true,
                    tension: 0.1
                }]
            },
            options: {
                responsive: true,
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    }

    }

    // Admin Dashboard Functionality
    function initAdminDashboard() {
        const loginForm = document.getElementById('login-form');
        const logoutBtn = document.getElementById('logout-btn');
        const refreshStatusBtn = document.getElementById('refresh-status');
        const viewCacheStatsBtn = document.getElementById('view-cache-stats');
        const clearCacheBtn = document.getElementById('clear-cache');

        if (loginForm) {
            loginForm.addEventListener('submit', handleLogin);
        }

        if (logoutBtn) {
            logoutBtn.addEventListener('click', handleLogout);
        }

        if (refreshStatusBtn) {
            refreshStatusBtn.addEventListener('click', loadSystemStatus);
        }

        if (viewCacheStatsBtn) {
            viewCacheStatsBtn.addEventListener('click', loadCacheStats);
        }

        if (clearCacheBtn) {
            clearCacheBtn.addEventListener('click', clearCache);
        }

        // Check if already logged in
        checkAuthStatus();
    }

    async function handleLogin(e) {
        e.preventDefault();

        const username = document.getElementById('username-input').value;
        const password = document.getElementById('password-input').value;
        const errorDiv = document.getElementById('login-error');

        try {
            const response = await fetch(`${API_BASE}/login`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ username, password }),
            });

            const data = await response.json();

            if (response.ok) {
                if (data.csrfToken) {
                    setCSRFToken(data.csrfToken);
                }
                document.getElementById('login-section').classList.add('hidden');
                document.getElementById('dashboard-section').classList.remove('hidden');
                document.getElementById('user-info').classList.remove('hidden');
                document.getElementById('username').textContent = username;
                errorDiv.classList.add('hidden');
                loadSystemStatus();
            } else {
                errorDiv.textContent = data.error || 'Login failed';
                errorDiv.classList.remove('hidden');
            }
        } catch (error) {
            errorDiv.textContent = 'Network error occurred';
            errorDiv.classList.remove('hidden');
        }
    }

    async function handleLogout() {
        try {
            await authFetch(`${API_BASE}/logout`, { method: 'POST' });
        } catch (error) {
            console.error('Logout error:', error);
        }

        document.getElementById('login-section').classList.remove('hidden');
        document.getElementById('dashboard-section').classList.add('hidden');
        document.getElementById('user-info').classList.add('hidden');
        document.getElementById('username-input').value = '';
        document.getElementById('password-input').value = '';
        document.getElementById('login-error').classList.add('hidden');
    }

    async function checkAuthStatus() {
        try {
            const response = await authFetch(`${API_BASE}/admin/status`);
            if (response.ok) {
                const data = await response.json();
                document.getElementById('login-section').classList.add('hidden');
                document.getElementById('dashboard-section').classList.remove('hidden');
                document.getElementById('user-info').classList.remove('hidden');
                document.getElementById('username').textContent = data.user;
                loadSystemStatus();
            }
        } catch (error) {
            // Not authenticated, show login form
        }
    }

    async function loadSystemStatus() {
        try {
            const response = await authFetch(`${API_BASE}/admin/status`);
            const data = await response.json();

            const statusDiv = document.getElementById('system-status');
            statusDiv.innerHTML = `
                <div class="bg-green-50 dark:bg-green-900 p-4 rounded">
                    <h3 class="font-semibold text-green-800 dark:text-green-200">Status</h3>
                    <p class="text-green-600 dark:text-green-400">${data.status}</p>
                </div>
                <div class="bg-blue-50 dark:bg-blue-900 p-4 rounded">
                    <h3 class="font-semibold text-blue-800 dark:text-blue-200">Redis Memory</h3>
                    <p class="text-blue-600 dark:text-blue-400">${data.redis_memory || 'N/A'}</p>
                </div>
                <div class="bg-purple-50 dark:bg-purple-900 p-4 rounded">
                    <h3 class="font-semibold text-purple-800 dark:text-purple-200">Active Rate Limits</h3>
                    <p class="text-purple-600 dark:text-purple-400">${data.active_rate_limits}</p>
                </div>
            `;
        } catch (error) {
            console.error('Failed to load system status:', error);
        }
    }

    async function loadCacheStats() {
        try {
            const response = await authFetch(`${API_BASE}/admin/cache?action=stats`);
            const data = await response.json();

            const statsDiv = document.getElementById('cache-stats');
            if (data.total_keys > 0) {
                statsDiv.innerHTML = `
                    <p class="text-lg font-semibold">Total Keys: ${data.total_keys}</p>
                    <div class="mt-3">
                        <p class="font-medium mb-2">Cache Keys:</p>
                        <div class="max-h-40 overflow-y-auto bg-gray-100 dark:bg-gray-600 p-2 rounded text-sm">
                            ${data.keys.map(key => `<div>${key}</div>`).join('')}
                        </div>
                    </div>
                `;
            } else {
                statsDiv.innerHTML = '<p class="text-gray-600 dark:text-gray-400">No cached data found</p>';
            }
        } catch (error) {
            document.getElementById('cache-stats').innerHTML = '<p class="text-red-600">Failed to load cache stats</p>';
        }
    }

    async function clearCache() {
        if (!confirm('Are you sure you want to clear all cache? This action cannot be undone.')) {
            return;
        }

        try {
            const response = await authFetch(`${API_BASE}/admin/cache?action=clear`, { method: 'GET' });
            const data = await response.json();

            const resultDiv = document.getElementById('cache-result');
            if (response.ok) {
                resultDiv.innerHTML = `<p class="text-green-600">Cache cleared successfully! ${data.keys_removed} keys removed.</p>`;
                resultDiv.classList.remove('hidden');
                // Refresh cache stats
                loadCacheStats();
            } else {
                resultDiv.innerHTML = `<p class="text-red-600">Failed to clear cache: ${data.error}</p>`;
                resultDiv.classList.remove('hidden');
            }
        } catch (error) {
            document.getElementById('cache-result').innerHTML = '<p class="text-red-600">Network error occurred</p>';
            document.getElementById('cache-result').classList.remove('hidden');
        }
    }

    // Initialize admin dashboard if on admin page
    if (window.location.pathname === '/admin') {
        initAdminDashboard();
    }

    // Authentication functions
    function initAuth() {
        const loginBtn = document.getElementById('login-btn');
        const registerBtn = document.getElementById('register-btn');
        const logoutBtn = document.getElementById('logout-btn');
        const switchToRegister = document.getElementById('switch-to-register');
        const switchToLogin = document.getElementById('switch-to-login');
        const loginForm = document.getElementById('login-form');
        const registerForm = document.getElementById('register-form');

        if (loginBtn) loginBtn.addEventListener('click', showLoginForm);
        if (registerBtn) registerBtn.addEventListener('click', showRegisterForm);
        if (logoutBtn) logoutBtn.addEventListener('click', handleLogout);
        if (switchToRegister) switchToRegister.addEventListener('click', showRegisterForm);
        if (switchToLogin) switchToLogin.addEventListener('click', showLoginForm);
        if (loginForm) loginForm.addEventListener('submit', handleLogin);
        if (registerForm) registerForm.addEventListener('submit', handleRegister);

        // Check if user is already logged in
        checkAuthStatus();
    }

    function showLoginForm() {
        document.getElementById('auth-forms').classList.remove('hidden');
        document.getElementById('login-form-container').classList.remove('hidden');
        document.getElementById('register-form-container').classList.add('hidden');
        document.getElementById('login-error').classList.add('hidden');
        document.getElementById('register-error').classList.add('hidden');
    }

    function showRegisterForm() {
        document.getElementById('auth-forms').classList.remove('hidden');
        document.getElementById('register-form-container').classList.remove('hidden');
        document.getElementById('login-form-container').classList.add('hidden');
        document.getElementById('login-error').classList.add('hidden');
        document.getElementById('register-error').classList.add('hidden');
    }

    async function handleLogin(e) {
        e.preventDefault();

        const username = document.getElementById('login-username').value;
        const password = document.getElementById('login-password').value;
        const errorDiv = document.getElementById('login-error');

        try {
            const response = await fetch(`${API_BASE}/login`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ username, password }),
            });

            const data = await response.json();

            if (response.ok) {
                if (data.csrfToken) {
                    setCSRFToken(data.csrfToken);
                }
                document.getElementById('auth-forms').classList.add('hidden');
                document.getElementById('auth-buttons').classList.add('hidden');
                document.getElementById('user-info').classList.remove('hidden');
                document.getElementById('username-display').textContent = username;
                errorDiv.classList.add('hidden');
                // Redirect to admin if admin user
                if (data.role === 'admin') {
                    window.location.href = '/admin';
                }
            } else {
                errorDiv.textContent = data.error || 'Login failed';
                errorDiv.classList.remove('hidden');
            }
        } catch (error) {
            errorDiv.textContent = 'Network error occurred';
            errorDiv.classList.remove('hidden');
        }
    }

    async function handleRegister(e) {
        e.preventDefault();

        const username = document.getElementById('register-username').value;
        const email = document.getElementById('register-email').value;
        const password = document.getElementById('register-password').value;
        const errorDiv = document.getElementById('register-error');

        try {
            const response = await fetch(`${API_BASE}/register`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ username, password, email }),
            });

            const data = await response.json();

            if (response.ok) {
                // Registration successful, show login form
                showLoginForm();
                document.getElementById('login-username').value = username;
                alert('Registration successful! Please login.');
            } else {
                errorDiv.textContent = data.error || 'Registration failed';
                errorDiv.classList.remove('hidden');
            }
        } catch (error) {
            errorDiv.textContent = 'Network error occurred';
            errorDiv.classList.remove('hidden');
        }
    }

    async function handleLogout() {
        try {
            await authFetch(`${API_BASE}/logout`, { method: 'POST' });
        } catch (error) {
            console.error('Logout error:', error);
        }

        document.getElementById('auth-buttons').classList.remove('hidden');
        document.getElementById('user-info').classList.add('hidden');
        document.getElementById('auth-forms').classList.add('hidden');
        document.getElementById('username-display').textContent = '';
    }

    function applyProfileToPage(user) {
        if (!user) return;
        if (user.theme === "system" || user.theme === "light" || user.theme === "dark") {
            const scheme = user.theme === "system"
                ? (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
                : user.theme;
            document.documentElement.setAttribute("data-color-scheme", scheme);
        } else {
            document.documentElement.removeAttribute("data-color-scheme");
        }
        if (user.language && (user.language === "en" || user.language === "es")) {
            document.documentElement.lang = user.language;
        }
    }

    async function checkAuthStatus() {
        try {
            const response = await fetch(`${API_BASE}/user/profile`, { credentials: "include" });
            if (response.ok) {
                const data = await response.json();
                applyProfileToPage(data);
                document.getElementById("auth-buttons").classList.add("hidden");
                document.getElementById("user-info").classList.remove("hidden");
                document.getElementById("username-display").textContent = data.username;
            }
        } catch (error) {
            // Not authenticated, show login/register buttons
        }
    }

    // Initialize authentication
    initAuth();
