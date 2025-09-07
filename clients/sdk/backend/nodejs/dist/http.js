"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.httpRequest = httpRequest;
const errors_1 = require("./errors");
function sleep(ms) {
    return new Promise((res) => setTimeout(res, ms));
}
async function httpRequest(baseUrl, method, path, body, headers = {}, opts = {}) {
    const url = baseUrl.replace(/\/$/, '') + path;
    const timeoutMs = opts.timeoutMs ?? 10000;
    const maxRetries = opts.maxRetries ?? 2;
    let attempt = 0;
    while (true) {
        attempt++;
        const controller = typeof AbortController !== 'undefined' ? new AbortController() : null;
        const id = controller ? setTimeout(() => controller.abort(), timeoutMs) : null;
        try {
            const res = await fetch(url, {
                method,
                headers: Object.assign({ 'Content-Type': 'application/json' }, headers),
                body: body == null ? undefined : JSON.stringify(body),
                signal: controller ? controller.signal : undefined,
            });
            if (id)
                clearTimeout(id);
            const text = await res.text();
            const contentType = res.headers.get('content-type') || '';
            const parsed = contentType.includes('application/json') && text ? JSON.parse(text) : text;
            if (!res.ok)
                throw new errors_1.ApiError(res.status, parsed);
            return parsed;
        }
        catch (err) {
            if (err instanceof errors_1.ApiError)
                throw err;
            // retry on network/timeout errors
            if (attempt > maxRetries)
                throw err;
            await sleep(100 * Math.pow(2, attempt));
            continue;
        }
    }
}
