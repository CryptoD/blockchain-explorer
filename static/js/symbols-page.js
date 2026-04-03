        // Symbol Search Module
        (function() {
            let currentPage = 1;
            let totalPages = 1;
            let currentQuery = '';

            // DOM Elements
            const searchInput = document.getElementById('symbol-search');
            const searchBtn = document.getElementById('search-btn');
            const resetBtn = document.getElementById('reset-btn');
            const filterType = document.getElementById('filter-type');
            const filterCategory = document.getElementById('filter-category');
            const minPrice = document.getElementById('min-price');
            const maxPrice = document.getElementById('max-price');
            const minMcap = document.getElementById('min-mcap');
            const maxMcap = document.getElementById('max-mcap');
            const sortBy = document.getElementById('sort-by');
            const sortDir = document.getElementById('sort-dir');
            const loadingEl = document.getElementById('loading');
            const errorEl = document.getElementById('error');
            const resultsContainer = document.getElementById('results-container');
            const resultsBody = document.getElementById('results-body');
            const resultsSummary = document.getElementById('results-summary');
            const noResults = document.getElementById('no-results');
            const pagination = document.getElementById('pagination');
            const prevBtn = document.getElementById('prev-page');
            const nextBtn = document.getElementById('next-page');
            const pageInfo = document.getElementById('page-info');
            const clearFiltersBtn = document.getElementById('clear-filters');

            // Format currency
            function formatCurrency(value) {
                if (value >= 1e12) return '$' + (value / 1e12).toFixed(2) + 'T';
                if (value >= 1e9) return '$' + (value / 1e9).toFixed(2) + 'B';
                if (value >= 1e6) return '$' + (value / 1e6).toFixed(2) + 'M';
                if (value >= 1e3) return '$' + (value / 1e3).toFixed(2) + 'K';
                return '$' + value.toFixed(2);
            }

            // Format price
            function formatPrice(value) {
                if (value < 0.01) return '$' + value.toFixed(6);
                if (value < 1) return '$' + value.toFixed(4);
                return '$' + value.toFixed(2);
            }

            // Format percentage
            function formatPercentage(value) {
                const sign = value >= 0 ? '+' : '';
                const color = value >= 0 ? 'text-green-600' : 'text-red-600';
                return `<span class="${color}">${sign}${value.toFixed(2)}%</span>`;
            }

            // Show loading
            function showLoading() {
                loadingEl.classList.remove('hidden');
                resultsContainer.classList.add('hidden');
                noResults.classList.add('hidden');
                errorEl.classList.add('hidden');
            }

            // Hide loading
            function hideLoading() {
                loadingEl.classList.add('hidden');
            }

            // Show error
            function showError(message) {
                errorEl.textContent = message;
                errorEl.classList.remove('hidden');
                resultsContainer.classList.add('hidden');
                noResults.classList.add('hidden');
            }

            // Build query parameters (for list view)
            function buildQueryParams() {
                const params = new URLSearchParams();
                
                if (currentQuery) params.append('q', currentQuery);
                if (filterType.value) params.append('types', filterType.value);
                if (filterCategory.value) params.append('categories', filterCategory.value);
                if (minPrice.value) params.append('min_price', minPrice.value);
                if (maxPrice.value) params.append('max_price', maxPrice.value);
                if (minMcap.value) params.append('min_market_cap', minMcap.value);
                if (maxMcap.value) params.append('max_market_cap', maxMcap.value);
                params.append('sort_by', sortBy.value);
                params.append('sort_dir', sortDir.value);
                params.append('page', currentPage.toString());
                params.append('page_size', '20');
                
                return params.toString();
            }

            // Build query parameters for export (same filters, larger page size)
            function buildExportQueryParams() {
                const params = new URLSearchParams();
                if (currentQuery) params.append('q', currentQuery);
                if (filterType.value) params.append('types', filterType.value);
                if (filterCategory.value) params.append('categories', filterCategory.value);
                if (minPrice.value) params.append('min_price', minPrice.value);
                if (maxPrice.value) params.append('max_price', maxPrice.value);
                if (minMcap.value) params.append('min_market_cap', minMcap.value);
                if (maxMcap.value) params.append('max_market_cap', maxMcap.value);
                params.append('sort_by', sortBy.value);
                params.append('sort_dir', sortDir.value);
                params.append('page', '1');
                params.append('page_size', '100');
                return params.toString();
            }

            // Fetch symbols
            async function fetchSymbols() {
                showLoading();
                
                try {
                    const response = await fetch(`/api/search/advanced?${buildQueryParams()}`);
                    
                    if (!response.ok) {
                        throw new Error(`HTTP error! status: ${response.status}`);
                    }
                    
                    const data = await response.json();
                    displayResults(data);
                } catch (error) {
                    showError('Failed to fetch symbols: ' + error.message);
                } finally {
                    hideLoading();
                }
            }

            // Display results
            function displayResults(data) {
                const { data: symbols, pagination: pag, filters_applied, sort_applied } = data;
                
                if (!symbols || symbols.length === 0) {
                    resultsContainer.classList.add('hidden');
                    resultsSummary.classList.add('hidden');
                    noResults.classList.remove('hidden');
                    return;
                }

                // Update summary
                document.getElementById('showing-start').textContent = ((pag.page - 1) * pag.page_size) + 1;
                document.getElementById('showing-end').textContent = Math.min(pag.page * pag.page_size, pag.total);
                document.getElementById('total-results').textContent = pag.total;
                resultsSummary.classList.remove('hidden');

                // Render results
                resultsBody.innerHTML = symbols.map(symbol => `
                    <tr class="hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors cursor-pointer" data-symbol="${String(symbol.symbol || '').replace(/"/g, '&quot;')}" data-name="${String(symbol.name || '').replace(/"/g, '&quot;')}">
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-slate-900 dark:text-slate-100">#${symbol.rank}</td>
                        <td class="px-6 py-4 whitespace-nowrap">
                            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-primary/10 text-primary">
                                ${symbol.symbol}
                            </span>
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-slate-900 dark:text-slate-100">${symbol.name}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-slate-600 dark:text-slate-400 capitalize">${symbol.type}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-slate-600 dark:text-slate-400 capitalize">${symbol.category}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-right text-slate-900 dark:text-slate-100">${formatPrice(symbol.price)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-right text-slate-900 dark:text-slate-100">${formatCurrency(symbol.market_cap)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-right text-slate-900 dark:text-slate-100">${formatCurrency(symbol.volume_24h)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-right">${formatPercentage(symbol.change_24h)}</td>
                    </tr>
                `).join('');

                // Wire click-to-load news widget.
                const panel = document.getElementById('asset-news-panel');
                const symEl = document.getElementById('asset-news-symbol');
                if (panel && symEl) {
                    panel.classList.remove('hidden');
                    // Default to first result on initial render.
                    const first = symbols[0];
                    if (first && first.symbol) {
                        setSelectedAssetForNews(String(first.symbol));
                    }
                    resultsBody.querySelectorAll('tr[data-symbol]').forEach(tr => {
                        tr.addEventListener('click', () => {
                            const s = tr.getAttribute('data-symbol') || '';
                            if (s) setSelectedAssetForNews(s);
                        });
                    });
                }

                resultsContainer.classList.remove('hidden');
                noResults.classList.add('hidden');

                // Update pagination
                totalPages = pag.total_pages;
                currentPage = pag.page;
                
                if (totalPages > 1) {
                    pagination.classList.remove('hidden');
                    pageInfo.textContent = `Page ${currentPage} of ${totalPages}`;
                    prevBtn.disabled = currentPage <= 1;
                    nextBtn.disabled = currentPage >= totalPages;
                } else {
                    pagination.classList.add('hidden');
                }
            }

            // Event listeners
            searchBtn.addEventListener('click', () => {
                currentQuery = searchInput.value.trim();
                currentPage = 1;
                fetchSymbols();
            });

            searchInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    currentQuery = searchInput.value.trim();
                    currentPage = 1;
                    fetchSymbols();
                }
            });

            resetBtn.addEventListener('click', () => {
                searchInput.value = '';
                filterType.value = '';
                filterCategory.value = '';
                minPrice.value = '';
                maxPrice.value = '';
                minMcap.value = '';
                maxMcap.value = '';
                sortBy.value = 'rank';
                sortDir.value = 'asc';
                currentQuery = '';
                currentPage = 1;
                resultsContainer.classList.add('hidden');
                resultsSummary.classList.add('hidden');
                noResults.classList.add('hidden');
            });

            clearFiltersBtn.addEventListener('click', () => {
                resetBtn.click();
            });

            prevBtn.addEventListener('click', () => {
                if (currentPage > 1) {
                    currentPage--;
                    fetchSymbols();
                }
            });

            nextBtn.addEventListener('click', () => {
                if (currentPage < totalPages) {
                    currentPage++;
                    fetchSymbols();
                }
            });

            // Auto-search on filter change
            [filterType, filterCategory, sortBy, sortDir].forEach(el => {
                el.addEventListener('change', () => {
                    currentPage = 1;
                    fetchSymbols();
                });
            });

            // Debounced search on price/mcap input
            let debounceTimer;
            [minPrice, maxPrice, minMcap, maxMcap].forEach(el => {
                el.addEventListener('input', () => {
                    clearTimeout(debounceTimer);
                    debounceTimer = setTimeout(() => {
                        currentPage = 1;
                        fetchSymbols();
                    }, 500);
                });
            });

            // Export results as JSON
            const exportJsonBtn = document.getElementById('export-symbols-json');
            const exportJsonLabel = document.getElementById('export-symbols-json-label');
            const exportJsonSpinner = document.getElementById('export-symbols-json-spinner');
            const exportJsonError = document.getElementById('export-symbols-error');
            if (exportJsonBtn) {
                exportJsonBtn.addEventListener('click', async function() {
                    exportJsonError.classList.add('hidden');
                    exportJsonSpinner.classList.remove('hidden');
                    exportJsonLabel.textContent = 'Exporting…';
                    try {
                        const queryString = buildExportQueryParams();
                        const res = await fetch('/api/search/advanced/export?' + queryString);
                        if (!res.ok) {
                            const data = await res.json().catch(function() { return {}; });
                            throw new Error(data.error || data.code || 'Export failed');
                        }
                        const blob = await res.blob();
                        const a = document.createElement('a');
                        a.href = URL.createObjectURL(blob);
                        a.download = 'symbol-search-results.json';
                        a.click();
                        URL.revokeObjectURL(a.href);
                    } catch (err) {
                        exportJsonError.textContent = err.message || 'Export failed.';
                        exportJsonError.classList.remove('hidden');
                    } finally {
                        exportJsonSpinner.classList.add('hidden');
                        exportJsonLabel.textContent = 'Export results (JSON)';
                    }
                });
            }

            // Initial load
            fetchSymbols();

            // -----------------------------
            // News widget (asset detail)
            // -----------------------------
            const MAJOR_OUTLETS = new Set([
                'reuters.com', 'bloomberg.com', 'wsj.com', 'ft.com', 'cnbc.com',
                'coindesk.com', 'cointelegraph.com', 'theblock.co', 'decrypt.co'
            ]);

            let selectedAssetSymbol = '';
            let lastFetchedArticles = [];
            let lastFetchedMeta = null;

            const newsLoadingEl = document.getElementById('asset-news-loading');
            const newsErrorEl = document.getElementById('asset-news-error');
            const newsEmptyEl = document.getElementById('asset-news-empty');
            const newsListEl = document.getElementById('asset-news-list');
            const newsMetaEl = document.getElementById('asset-news-meta');
            const majorOnlyEl = document.getElementById('asset-news-filter-major');
            const last24hEl = document.getElementById('asset-news-filter-24h');
            const favsOnlyEl = document.getElementById('asset-news-filter-favs');
            const refreshBtn = document.getElementById('asset-news-refresh');

            function setSelectedAssetForNews(symbol) {
                selectedAssetSymbol = String(symbol || '').trim();
                document.getElementById('asset-news-symbol').textContent = selectedAssetSymbol || '—';
                fetchAssetNews();
            }

            function setNewsState({ loading, error, empty }) {
                if (newsLoadingEl) newsLoadingEl.classList.toggle('hidden', !loading);
                if (newsErrorEl) newsErrorEl.classList.toggle('hidden', !error);
                if (newsEmptyEl) newsEmptyEl.classList.toggle('hidden', !empty);
            }

            function parsePublishedAt(a) {
                const d = new Date(a.published_at);
                return isNaN(d.getTime()) ? null : d;
            }

            function applyNewsFilters(articles) {
                let out = Array.isArray(articles) ? articles.slice() : [];
                const majorOnly = !!(majorOnlyEl && majorOnlyEl.checked);
                const last24h = !!(last24hEl && last24hEl.checked);
                const now = Date.now();
                if (last24h) {
                    out = out.filter(a => {
                        const d = parsePublishedAt(a);
                        return d && (now - d.getTime()) <= 24 * 60 * 60 * 1000;
                    });
                }
                if (majorOnly) {
                    out = out.filter(a => {
                        const s = String(a.source || '').toLowerCase().trim();
                        return MAJOR_OUTLETS.has(s);
                    });
                }
                return out;
            }

            function renderNews() {
                if (!newsListEl) return;
                newsListEl.innerHTML = '';
                const filtered = applyNewsFilters(lastFetchedArticles);
                if (filtered.length === 0) {
                    setNewsState({ loading: false, error: false, empty: true });
                } else {
                    setNewsState({ loading: false, error: false, empty: false });
                    const top = filtered.slice(0, 8);
                    top.forEach((a, idx) => {
                        const li = document.createElement('li');
                        li.className = 'p-3 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors';
                        const d = parsePublishedAt(a);
                        const time = d ? d.toLocaleString() : '';
                        li.innerHTML = `
                            <div class="flex items-start justify-between gap-3">
                                <div class="min-w-0">
                                    <a href="${a.url}" target="_blank" rel="noopener noreferrer" class="font-semibold ${idx === 0 ? 'text-primary' : 'text-slate-900 dark:text-slate-100'} hover:underline break-words">
                                        ${escapeHtml(a.headline || '')}
                                    </a>
                                    ${a.summary ? `<p class="mt-1 text-sm text-slate-600 dark:text-slate-400 max-h-16 overflow-hidden">${escapeHtml(a.summary)}</p>` : ''}
                                    <p class="mt-2 text-xs text-slate-500 dark:text-slate-400">${escapeHtml(String(a.source || ''))}${time ? ` • ${escapeHtml(time)}` : ''}</p>
                                </div>
                                ${a.image_url ? `<img src="${a.image_url}" alt="" class="hidden sm:block w-16 h-16 object-cover rounded border border-gray-200 dark:border-gray-700" loading="lazy" />` : ''}
                            </div>
                        `;
                        newsListEl.appendChild(li);
                    });
                }
                if (newsMetaEl) {
                    const m = lastFetchedMeta || {};
                    const cachedText = m.cached ? (m.stale ? 'cached (stale)' : 'cached') : 'live';
                    newsMetaEl.textContent = m.provider ? `Source: ${m.provider} • ${cachedText}` : '';
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

            async function fetchAssetNews() {
                if (!selectedAssetSymbol || !newsListEl) return;
                setNewsState({ loading: true, error: false, empty: false });
                if (newsErrorEl) newsErrorEl.textContent = '';
                try {
                    const favOnly = !!(favsOnlyEl && favsOnlyEl.checked);
                    const url = '/api/news/' + encodeURIComponent(selectedAssetSymbol) + (favOnly ? '?favorites_only=true' : '');
                    const res = await fetch(url, { credentials: 'include' });
                    if (!res.ok) throw new Error('HTTP ' + res.status);
                    const payload = await res.json();
                    lastFetchedArticles = Array.isArray(payload.data) ? payload.data : [];
                    lastFetchedMeta = payload.meta || {};
                    renderNews();
                } catch (e) {
                    lastFetchedArticles = [];
                    lastFetchedMeta = null;
                    if (newsErrorEl) {
                        newsErrorEl.textContent = 'Failed to load news. Please try again.';
                    }
                    setNewsState({ loading: false, error: true, empty: false });
                }
            }

            if (majorOnlyEl) majorOnlyEl.addEventListener('change', renderNews);
            if (last24hEl) last24hEl.addEventListener('change', renderNews);
            if (favsOnlyEl) favsOnlyEl.addEventListener('change', fetchAssetNews);
            if (refreshBtn) refreshBtn.addEventListener('click', fetchAssetNews);
        })();
