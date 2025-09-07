"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.BackendClient = void 0;
const http_1 = require("./http");
const errors_1 = require("./errors");
class BackendClient {
    constructor(opts) {
        this.baseUrl = opts.baseUrl;
        this.apiKey = opts.apiKey;
        this.timeoutMs = opts.timeoutMs;
        this.maxRetries = opts.maxRetries;
    }
    headers() {
        return {
            Authorization: `Bearer ${this.apiKey}`,
        };
    }
    async request(method, path, body) {
        try {
            return await (0, http_1.httpRequest)(this.baseUrl, method, path, body, this.headers(), {
                timeoutMs: this.timeoutMs,
                maxRetries: this.maxRetries,
            });
        }
        catch (err) {
            if (err instanceof errors_1.ApiError)
                throw err;
            throw err;
        }
    }
    // signing helper
    async signUser(userId) {
        const res = await this.request('POST', '/v1/_sign', { userId });
        return res;
    }
    // admin endpoints
    async adminHealth() {
        return await this.request('GET', '/admin/health');
    }
    async adminStats() {
        return await this.request('GET', '/admin/stats');
    }
    // threads
    async listThreads() {
        const res = await this.request('GET', '/v1/threads');
        return res.threads || [];
    }
    async deleteThread(id) {
        await this.request('DELETE', `/v1/threads/${encodeURIComponent(id)}`);
    }
    // low-level helpers
    async createThread(t) {
        return await this.request('POST', '/v1/threads', t);
    }
    async createMessage(m) {
        return await this.request('POST', '/v1/messages', m);
    }
}
exports.BackendClient = BackendClient;
exports.default = BackendClient;
