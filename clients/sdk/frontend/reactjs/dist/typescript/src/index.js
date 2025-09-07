"use strict";
/*
 ProgressDB Frontend TypeScript SDK
 - Uses fetch to call ProgressDB endpoints.
 - Designed for frontend callers using a frontend API key (sent as `X-API-Key`).
 - Requires callers to supply `userId` and `userSignature` which are sent as
   `X-User-ID` and `X-User-Signature` on protected endpoints.
*/
Object.defineProperty(exports, "__esModule", { value: true });
exports.ProgressDBClient = void 0;
function buildHeaders(apiKey, userId, userSignature) {
    const headers = {
        'Content-Type': 'application/json'
    };
    if (apiKey)
        headers['X-API-Key'] = apiKey;
    if (userId)
        headers['X-User-ID'] = userId;
    if (userSignature)
        headers['X-User-Signature'] = userSignature;
    return headers;
}
class ProgressDBClient {
    constructor(opts = {}) {
        this.baseUrl = opts.baseUrl || '';
        this.apiKey = opts.apiKey;
        this.defaultUserId = opts.defaultUserId;
        this.defaultUserSignature = opts.defaultUserSignature;
        this.fetchImpl = opts.fetch || (typeof fetch !== 'undefined' ? fetch.bind(globalThis) : (() => { throw new Error('fetch not available, provide a fetch implementation in SDKOptions'); })());
    }
    headers(userId, userSignature) {
        return buildHeaders(this.apiKey, userId || this.defaultUserId, userSignature || this.defaultUserSignature);
    }
    async request(path, method = 'GET', body, userId, userSignature) {
        const url = this.baseUrl.replace(/\/$/, '') + path;
        const res = await this.fetchImpl(url, {
            method,
            headers: this.headers(userId, userSignature),
            body: body ? JSON.stringify(body) : undefined
        });
        if (res.status === 204)
            return null;
        const contentType = res.headers.get('content-type') || '';
        if (contentType.includes('application/json'))
            return res.json();
        return res.text();
    }
    // Health
    health() {
        return this.request('/healthz', 'GET');
    }
    // Messages
    listMessages(query = {}, userId, userSignature) {
        const qs = new URLSearchParams();
        if (query.thread)
            qs.set('thread', query.thread);
        if (query.limit !== undefined)
            qs.set('limit', String(query.limit));
        return this.request('/v1/messages' + (qs.toString() ? `?${qs.toString()}` : ''), 'GET', undefined, userId, userSignature);
    }
    createMessage(msg, userId, userSignature) {
        return this.request('/v1/messages', 'POST', msg, userId, userSignature);
    }
    getMessage(id, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature);
    }
    updateMessage(id, msg, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature);
    }
    deleteMessage(id, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
    }
    listMessageVersions(id, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}/versions`, 'GET', undefined, userId, userSignature);
    }
    // Reactions
    listReactions(id, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}/reactions`, 'GET', undefined, userId, userSignature);
    }
    addOrUpdateReaction(id, input, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}/reactions`, 'POST', input, userId, userSignature);
    }
    removeReaction(id, identity, userId, userSignature) {
        return this.request(`/v1/messages/${encodeURIComponent(id)}/reactions/${encodeURIComponent(identity)}`, 'DELETE', undefined, userId, userSignature);
    }
    // Threads
    createThread(thread, userId, userSignature) {
        return this.request('/v1/threads', 'POST', thread, userId, userSignature);
    }
    listThreads(userId, userSignature) {
        return this.request('/v1/threads', 'GET', undefined, userId, userSignature);
    }
    getThread(id, userId, userSignature) {
        return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature);
    }
    deleteThread(id, userId, userSignature) {
        return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
    }
    // Thread messages
    createThreadMessage(threadID, msg, userId, userSignature) {
        return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages`, 'POST', msg, userId, userSignature);
    }
    listThreadMessages(threadID, query = {}, userId, userSignature) {
        const qs = new URLSearchParams();
        if (query.limit !== undefined)
            qs.set('limit', String(query.limit));
        return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature);
    }
    getThreadMessage(threadID, id, userId, userSignature) {
        return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature);
    }
    updateThreadMessage(threadID, id, msg, userId, userSignature) {
        return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature);
    }
    deleteThreadMessage(threadID, id, userId, userSignature) {
        return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
    }
    // Signing is admin-only; SDK exposes the call but it requires an admin key.
    signUser(userIdToSign) {
        return this.request('/v1/_sign', 'POST', { userId: userIdToSign });
    }
}
exports.ProgressDBClient = ProgressDBClient;
exports.default = ProgressDBClient;
