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

    function readinessErrMsg(data) {
        if (!data) return '';
        if (typeof data.message === 'string' && data.message) return data.message;
        if (typeof data.error === 'string') return data.error;
        return '';
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
        var wI = typeof window !== 'undefined' && window.I18n;
        var head = wI ? window.I18n.t('degraded_heading') : 'Reduced availability:';
        var tail = wI ? window.I18n.t('degraded_suffix') : ' Log in, portfolios, and other features that need the database may not work until this is resolved.';
        var dismiss = wI ? window.I18n.t('btn_dismiss') : 'Dismiss';
        wrap.innerHTML =
            '<div class="max-w-7xl mx-auto flex items-start justify-between gap-3">' +
            '<p class="m-0 flex-1"><strong>' + escapeHtml(head) + '</strong> ' +
            escapeHtml(message) +
            escapeHtml(tail) + '</p>' +
            '<button type="button" class="shrink-0 px-2 py-1 rounded border border-current text-xs hover:opacity-80" aria-label="' + escapeHtml(dismiss) + '">' + dismiss + '</button>' +
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
            var def = (typeof window !== 'undefined' && window.I18n) ? window.I18n.t('degraded_default') : 'Service dependencies are not ready.';
            var err = readinessErrMsg(data) || def;
            showBanner(err);
        } catch (e) {
            var msg = (typeof window !== 'undefined' && window.I18n) ? window.I18n.t('degraded_verify') : 'Could not verify readiness. The app may be degraded.';
            showBanner(msg);
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
