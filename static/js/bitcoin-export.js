    (function() {
        function getSearchQuery() {
            const params = new URLSearchParams(window.location.search);
            return params.get('q') || '';
        }
        const btn = document.getElementById('export-search-json');
        const label = document.getElementById('export-search-json-label');
        const spinner = document.getElementById('export-search-json-spinner');
        const errEl = document.getElementById('export-search-error');
        if (!btn) return;
        btn.addEventListener('click', async function() {
            const q = getSearchQuery();
            if (!q.trim()) {
                if (errEl) { errEl.textContent = 'No search query to export.'; errEl.classList.remove('hidden'); }
                return;
            }
            if (errEl) errEl.classList.add('hidden');
            spinner.classList.remove('hidden');
            label.textContent = 'Exporting…';
            try {
                const res = await fetch('/api/search/export?q=' + encodeURIComponent(q));
                if (!res.ok) {
                    const data = await res.json().catch(function() { return {}; });
                    throw new Error(data.error || data.code || 'Export failed');
                }
                const blob = await res.blob();
                var a = document.createElement('a');
                a.href = URL.createObjectURL(blob);
                a.download = 'search-result.json';
                a.click();
                URL.revokeObjectURL(a.href);
            } catch (err) {
                if (errEl) {
                    errEl.textContent = err.message || 'Export failed.';
                    errEl.classList.remove('hidden');
                }
            } finally {
                spinner.classList.add('hidden');
                label.textContent = 'Download as JSON';
            }
        });
    })();
