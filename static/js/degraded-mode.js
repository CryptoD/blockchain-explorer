/**
 * Shows a top-of-page banner when GET /ready reports the app is not ready (e.g. Redis down).
 * See docs/DEGRADED_MODE_UX.md (ROADMAP task 62).
 */
(function () {
    'use strict';

    var CHECK_MS = 60000;
    var bannerId = 'degraded-mode-banner';

    function escapeHtml(s) {
        var d = document.createElement('div');
        d.textContent = s;
        return d.innerHTML;
    }

    function removeBanner() {
        var el = document.getElementById(bannerId);
        if (el && el.parentNode) {
            el.parentNode.removeChild(el);
        }
    }

    function showBanner(message) {
        removeBanner();
        var wrap = document.createElement('div');
        wrap.id = bannerId;
        wrap.setAttribute('role', 'alert');
        wrap.className =
            'w-full px-4 py-3 text-sm border-b border-amber-600/40 bg-amber-100 text-amber-950 dark:bg-amber-950/90 dark:text-amber-100 dark:border-amber-500/40';
        wrap.style.cssText = 'position:relative;z-index:9999';
        wrap.innerHTML =
            '<div class="max-w-7xl mx-auto flex items-start justify-between gap-3">' +
            '<p class="m-0 flex-1"><strong>Reduced availability:</strong> ' +
            escapeHtml(message) +
            ' Log in, portfolios, and other features that need the database may not work until this is resolved.</p>' +
            '<button type="button" class="shrink-0 px-2 py-1 rounded border border-current text-xs hover:opacity-80" aria-label="Dismiss alert">Dismiss</button>' +
            '</div>';
        var btn = wrap.querySelector('button');
        if (btn) {
            btn.addEventListener('click', removeBanner);
        }
        if (document.body) {
            document.body.insertBefore(wrap, document.body.firstChild);
        }
    }

    async function checkReadiness() {
        try {
            var r = await fetch('/ready', {
                method: 'GET',
                headers: { Accept: 'application/json' },
                cache: 'no-store',
            });
            var data = {};
            try {
                data = await r.json();
            } catch (e) {
                data = {};
            }
            if (r.ok && data.status === 'ready') {
                removeBanner();
                return;
            }
            var err = typeof data.error === 'string' ? data.error : 'Service dependencies are not ready.';
            showBanner(err);
        } catch (e) {
            showBanner('Could not verify readiness. The app may be degraded.');
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function () {
            checkReadiness();
            setInterval(checkReadiness, CHECK_MS);
        });
    } else {
        checkReadiness();
        setInterval(checkReadiness, CHECK_MS);
    }
})();
