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

            fetch(`/api/autocomplete?q=${encodeURIComponent(query)}`, { signal })
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
                    // set input value preview (do not perform search yet)
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

            fetch(`/api/search?q=${encodeURIComponent(query)}`, { signal })
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
        }

        function loadNetworkStatus() {
            fetch('/api/network-status')
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

        // Pagination event listeners
        document.getElementById('prev-page').addEventListener('click', () => {
            if (currentPage > 1) {
                displayAddressData({result: currentAddressData}, currentPage - 1);
            }
        });

        document.getElementById('next-page').addEventListener('click', () => {
            const transactions = currentAddressData.transactions || [];
            const totalPages = Math.ceil(transactions.length / ITEMS_PER_PAGE);
            if (currentPage < totalPages) {
                displayAddressData({result: currentAddressData}, currentPage + 1);
            }
        });

        // Load and render charts
        loadCharts();
        loadHistory();
        renderHistory();

    function loadCharts() {
        fetch('/api/metrics')
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
