/**
 * User-facing fetch error normalization and render helpers.
 * Maps gateway/HTML/JSON error bodies to friendly copy (never dumps raw JSON).
 */
(function (w) {
    'use strict';

    function t(key, params) {
        if (w.I18n && typeof w.I18n.t === 'function') {
            return w.I18n.t(key, params);
        }
        var fallbacks = {
            err_heading: 'Something went wrong',
            err_gateway: 'The service is temporarily unavailable. Please try again in a moment.',
            err_service_unavailable: 'Service temporarily unavailable. Please try again shortly.',
            err_server: 'Server error. Please try again later.',
            err_rate_limit: 'Too many requests. Please wait and try again.',
            err_not_found: 'The requested resource was not found.',
            err_api_failure: 'Request failed. Please try again later.',
            err_network: 'Network connection issue. Please check your internet.',
            ui_retry: 'Retry',
        };
        var s = fallbacks[key] || key;
        if (params && params.detail != null) {
            s = s.replace(/\{\{detail\}\}/g, String(params.detail));
        }
        return s;
    }

    function pickApiErrorMsg(d) {
        if (!d) return '';
        if (typeof d.message === 'string' && d.message) return d.message;
        if (d.error != null) {
            if (typeof d.error === 'string') return d.error;
            if (typeof d.error === 'object' && typeof d.error.message === 'string') return d.error.message;
        }
        return '';
    }

    function looksLikeHtml(s) {
        return /^\s*</.test(String(s || ''));
    }

    function looksLikeJson(s) {
        var trimmed = String(s || '').trim();
        return (trimmed.charAt(0) === '{' && trimmed.charAt(trimmed.length - 1) === '}') ||
            (trimmed.charAt(0) === '[' && trimmed.charAt(trimmed.length - 1) === ']');
    }

    function messageForStatus(status) {
        if (status === 0) return t('err_network');
        if (status === 502) return t('err_gateway');
        if (status === 503 || status === 504) return t('err_service_unavailable');
        if (status === 429) return t('err_rate_limit');
        if (status >= 500) return t('err_server');
        if (status === 404) return t('err_not_found');
        return t('err_api_failure');
    }

    function isRetryableStatus(status) {
        return status === 0 || status === 502 || status === 503 || status === 504 || status >= 500;
    }

    /**
     * @param {number} status HTTP status
     * @param {string} bodyText response body text (optional)
     * @returns {string} user-safe message
     */
    function friendlyFromBody(status, bodyText) {
        if (status === 502 || status === 503 || status === 504) {
            return messageForStatus(status);
        }
        if (!bodyText || !String(bodyText).trim()) {
            return messageForStatus(status);
        }
        if (looksLikeHtml(bodyText)) {
            return messageForStatus(status);
        }
        if (looksLikeJson(bodyText)) {
            try {
                var j = JSON.parse(bodyText);
                var msg = pickApiErrorMsg(j);
                if (msg && !looksLikeJson(msg) && !looksLikeHtml(msg)) {
                    return msg;
                }
            } catch (e) { /* ignore */ }
            return messageForStatus(status);
        }
        var trimmed = String(bodyText).trim();
        if (trimmed.length <= 200 && !looksLikeHtml(trimmed) && !looksLikeJson(trimmed)) {
            return trimmed;
        }
        return messageForStatus(status);
    }

    /**
     * @param {Response} response
     * @returns {Promise<{status:number,message:string,retryable:boolean}>}
     */
    async function fromResponse(response) {
        var status = response && typeof response.status === 'number' ? response.status : 0;
        var bodyText = '';
        try {
            if (response && typeof response.text === 'function') {
                bodyText = await response.text();
            }
        } catch (e) { /* ignore */ }
        return {
            status: status,
            message: friendlyFromBody(status, bodyText),
            retryable: isRetryableStatus(status),
        };
    }

    /**
     * Render a user-friendly error panel into a container element.
     * @param {HTMLElement} container
     * @param {{ message?: string, title?: string, canRetry?: boolean, onRetry?: function, extraClass?: string }} options
     */
    function render(container, options) {
        if (!container) return;
        var opts = options || {};
        var message = opts.message || t('err_api_failure');
        var title = opts.title || t('err_heading');
        var extra = opts.extraClass || '';

        container.innerHTML = '';
        container.className = 'error-boundary ' + extra;

        var wrap = document.createElement('div');
        wrap.className = 'flex gap-3 items-start';

        var icon = document.createElement('span');
        icon.className = 'error-boundary-icon shrink-0 text-red-600 dark:text-red-400';
        icon.setAttribute('aria-hidden', 'true');
        icon.textContent = '⚠';

        var body = document.createElement('div');
        body.className = 'min-w-0 flex-1';

        var h = document.createElement('p');
        h.className = 'font-semibold text-red-800 dark:text-red-200';
        h.textContent = title;

        var p = document.createElement('p');
        p.className = 'mt-1 text-sm text-red-700 dark:text-red-300';
        p.textContent = message;

        body.appendChild(h);
        body.appendChild(p);

        if (opts.canRetry && typeof opts.onRetry === 'function') {
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'mt-3 text-sm font-medium text-primary hover:underline';
            btn.textContent = t('ui_retry');
            btn.addEventListener('click', function () {
                opts.onRetry();
            });
            body.appendChild(btn);
        }

        wrap.appendChild(icon);
        wrap.appendChild(body);
        container.appendChild(wrap);

        container.style.display = '';
        container.classList.remove('hidden');
        container.setAttribute('role', 'alert');
        container.setAttribute('aria-live', 'assertive');
    }

    function clear(container) {
        if (!container) return;
        container.innerHTML = '';
        container.classList.add('hidden');
        container.style.display = 'none';
        container.removeAttribute('aria-live');
    }

    w.ErrorUI = {
        t: t,
        pickApiErrorMsg: pickApiErrorMsg,
        friendlyFromBody: friendlyFromBody,
        messageForStatus: messageForStatus,
        isRetryableStatus: isRetryableStatus,
        fromResponse: fromResponse,
        render: render,
        clear: clear,
    };
})(typeof window !== 'undefined' ? window : this);
