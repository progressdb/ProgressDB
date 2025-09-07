"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.useReactions = exports.useThreads = exports.useMessage = exports.useMessages = exports.useUserSignature = exports.useProgressClient = exports.ProgressDBProvider = void 0;
const react_1 = __importStar(require("react"));
// Import the underlying ProgressDBClient and types from the TS SDK
const index_1 = __importDefault(require("../../typescript/src/index"));
const ProgressClientContext = (0, react_1.createContext)(null);
const ProgressDBProvider = ({ children, options, getUserSignature, persistSignature = true }) => {
    const client = (0, react_1.useMemo)(() => new index_1.default(options || {}), [JSON.stringify(options || {})]);
    const storageKey = (0, react_1.useMemo)(() => {
        const base = (options && options.baseUrl) || '';
        return `progressdb:signature:${base}`;
    }, [JSON.stringify(options || {})]);
    const [signatureLoaded, setSignatureLoaded] = (0, react_1.useState)(false);
    const [signatureLoading, setSignatureLoading] = (0, react_1.useState)(false);
    const [signatureError, setSignatureError] = (0, react_1.useState)(undefined);
    const [userId, setUserId] = (0, react_1.useState)(undefined);
    const [signature, setSignature] = (0, react_1.useState)(undefined);
    const applySignature = (s) => {
        if (!s)
            return;
        client.defaultUserId = s.userId;
        client.defaultUserSignature = s.signature;
        setUserId(s.userId);
        setSignature(s.signature);
    };
    const refreshUserSignature = async () => {
        setSignatureLoading(true);
        setSignatureError(undefined);
        try {
            const res = await getUserSignature();
            applySignature(res);
            if (persistSignature && typeof sessionStorage !== 'undefined') {
                sessionStorage.setItem(storageKey, JSON.stringify(res));
            }
            setSignatureLoaded(true);
        }
        catch (err) {
            setSignatureError(err);
            // eslint-disable-next-line no-console
            console.error('ProgressDB getUserSignature failed', err);
        }
        finally {
            setSignatureLoading(false);
        }
    };
    const clearUserSignature = () => {
        client.defaultUserId = undefined;
        client.defaultUserSignature = undefined;
        setUserId(undefined);
        setSignature(undefined);
        setSignatureLoaded(false);
        setSignatureError(undefined);
        if (persistSignature && typeof sessionStorage !== 'undefined') {
            sessionStorage.removeItem(storageKey);
        }
    };
    (0, react_1.useEffect)(() => {
        let cancelled = false;
        (async () => {
            setSignatureLoading(true);
            try {
                if (persistSignature && typeof sessionStorage !== 'undefined') {
                    const raw = sessionStorage.getItem(storageKey);
                    if (raw) {
                        try {
                            const parsed = JSON.parse(raw);
                            if (!cancelled) {
                                applySignature(parsed);
                                setSignatureLoaded(true);
                                setSignatureLoading(false);
                                return;
                            }
                        }
                        catch (e) {
                            // ignore parse errors
                        }
                    }
                }
                // fall back to calling getUserSignature
                const res = await getUserSignature();
                if (cancelled)
                    return;
                applySignature(res);
                if (persistSignature && typeof sessionStorage !== 'undefined') {
                    sessionStorage.setItem(storageKey, JSON.stringify(res));
                }
                setSignatureLoaded(true);
            }
            catch (err) {
                setSignatureError(err);
                // eslint-disable-next-line no-console
                console.error('ProgressDB getUserSignature failed', err);
            }
            finally {
                if (!cancelled)
                    setSignatureLoading(false);
            }
        })();
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [getUserSignature, storageKey]);
    const ctxVal = (0, react_1.useMemo)(() => ({
        client,
        userId,
        signature,
        signatureLoaded,
        signatureLoading,
        signatureError,
        refreshUserSignature,
        clearUserSignature,
    }), [client, userId, signature, signatureLoaded, signatureLoading, signatureError]);
    return react_1.default.createElement(ProgressClientContext.Provider, { value: ctxVal }, children);
};
exports.ProgressDBProvider = ProgressDBProvider;
function useProgressClient() {
    const ctx = (0, react_1.useContext)(ProgressClientContext);
    if (!ctx)
        throw new Error('useProgressClient must be used within ProgressDBProvider');
    return ctx.client;
}
exports.useProgressClient = useProgressClient;
function useUserSignature() {
    const ctx = (0, react_1.useContext)(ProgressClientContext);
    if (!ctx)
        throw new Error('useUserSignature must be used within ProgressDBProvider');
    return {
        userId: ctx.userId,
        signature: ctx.signature,
        loaded: ctx.signatureLoaded,
        loading: ctx.signatureLoading,
        error: ctx.signatureError,
        refresh: ctx.refreshUserSignature,
        clear: ctx.clearUserSignature,
    };
}
exports.useUserSignature = useUserSignature;
// Basic hook: list messages for a thread
function useMessages(threadId, deps = []) {
    const client = useProgressClient();
    const [messages, setMessages] = (0, react_1.useState)(null);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [error, setError] = (0, react_1.useState)(null);
    const fetchMessages = async () => {
        if (!threadId)
            return;
        setLoading(true);
        setError(null);
        try {
            const res = await client.listMessages({ thread: threadId });
            setMessages(res.messages || []);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setLoading(false);
        }
    };
    (0, react_1.useEffect)(() => {
        if (threadId)
            fetchMessages();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [threadId, ...deps]);
    const create = async (msg) => {
        const created = await client.createThreadMessage(threadId || '', msg);
        // naive refresh
        await fetchMessages();
        return created;
    };
    return { messages, loading, error, refresh: fetchMessages, create };
}
exports.useMessages = useMessages;
// Hook for a single message
function useMessage(id) {
    const client = useProgressClient();
    const [message, setMessage] = (0, react_1.useState)(null);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [error, setError] = (0, react_1.useState)(null);
    const fetchMessage = async () => {
        if (!id)
            return;
        setLoading(true);
        setError(null);
        try {
            const m = await client.getMessage(id);
            setMessage(m);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setLoading(false);
        }
    };
    (0, react_1.useEffect)(() => {
        if (id)
            fetchMessage();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [id]);
    const update = async (msg) => {
        const updated = await client.updateMessage(id || '', msg);
        setMessage(updated);
        return updated;
    };
    const remove = async () => {
        await client.deleteMessage(id || '');
        setMessage(null);
    };
    return { message, loading, error, refresh: fetchMessage, update, remove };
}
exports.useMessage = useMessage;
// Simple thread hooks
function useThreads(deps = []) {
    const client = useProgressClient();
    const [threads, setThreads] = (0, react_1.useState)(null);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [error, setError] = (0, react_1.useState)(null);
    const fetchThreads = async () => {
        setLoading(true);
        setError(null);
        try {
            const res = await client.listThreads();
            setThreads(res.threads || []);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setLoading(false);
        }
    };
    (0, react_1.useEffect)(() => {
        fetchThreads();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
    const create = async (t) => {
        const created = await client.createThread(t);
        await fetchThreads();
        return created;
    };
    return { threads, loading, error, refresh: fetchThreads, create };
}
exports.useThreads = useThreads;
// Reactions
function useReactions(messageId) {
    const client = useProgressClient();
    const [reactions, setReactions] = (0, react_1.useState)(null);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [error, setError] = (0, react_1.useState)(null);
    const fetchReactions = async () => {
        if (!messageId)
            return;
        setLoading(true);
        setError(null);
        try {
            const res = await client.listReactions(messageId);
            setReactions(res.reactions || []);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setLoading(false);
        }
    };
    (0, react_1.useEffect)(() => {
        if (messageId)
            fetchReactions();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [messageId]);
    const add = async (input) => {
        const res = await client.addOrUpdateReaction(messageId || '', input);
        await fetchReactions();
        return res;
    };
    const remove = async (identity) => {
        await client.removeReaction(messageId || '', identity);
        await fetchReactions();
    };
    return { reactions, loading, error, refresh: fetchReactions, add, remove };
}
exports.useReactions = useReactions;
exports.default = exports.ProgressDBProvider;
