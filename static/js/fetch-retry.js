/**
 * fetchWithRetry — resilient fetches for slow/offline networks.
 * Retries on thrown network errors and on HTTP 429 / 5xx. Does not retry other 4xx.
 */
(function (global) {
    'use strict';

    function delay(ms) {
        return new Promise(function (resolve) {
            setTimeout(resolve, ms);
        });
    }

    function shouldRetryResponse(res) {
        if (!res || typeof res.status !== 'number') return true;
        if (res.ok) return false;
        return res.status === 429 || res.status >= 500;
    }

    /**
     * @param {string} url
     * @param {RequestInit} [init]
     * @param {{ retries?: number, baseDelayMs?: number }} [options]
     * @returns {Promise<Response>}
     */
    async function fetchWithRetry(url, init, options) {
        var opts = options || {};
        var max = opts.retries != null ? opts.retries : 3;
        var baseMs = opts.baseDelayMs != null ? opts.baseDelayMs : 450;
        var lastRes;
        var err;
        var i;
        for (i = 0; i < max; i++) {
            try {
                lastRes = await fetch(url, init || {});
                if (!shouldRetryResponse(lastRes)) {
                    return lastRes;
                }
            } catch (e) {
                err = e;
                lastRes = null;
            }
            if (i < max - 1) {
                await delay(baseMs * Math.pow(2, i));
            }
        }
        if (lastRes) return lastRes;
        if (err) throw err;
        return fetch(url, init || {});
    }

    global.fetchWithRetry = fetchWithRetry;
})(typeof window !== 'undefined' ? window : this);
