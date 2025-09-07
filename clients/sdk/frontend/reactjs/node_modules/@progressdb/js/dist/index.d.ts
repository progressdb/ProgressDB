export type Message = {
    id?: string;
    thread?: string;
    author?: string;
    ts?: number;
    body?: any;
    reply_to?: string;
    deleted?: boolean;
    reactions?: Record<string, string>;
};
export type Thread = {
    id: string;
    title?: string;
    slug?: string;
    created_ts?: number;
    updated_ts?: number;
    author?: string;
    metadata?: Record<string, any>;
};
export type ReactionInput = {
    id: string;
    reaction: string;
};
export type SDKOptions = {
    baseUrl?: string;
    apiKey?: string;
    defaultUserId?: string;
    defaultUserSignature?: string;
    fetch?: typeof fetch;
};
export declare class ProgressDBClient {
    baseUrl: string;
    apiKey?: string;
    defaultUserId?: string;
    defaultUserSignature?: string;
    fetchImpl: typeof fetch;
    constructor(opts?: SDKOptions);
    private headers;
    private request;
    health(): Promise<any>;
    listMessages(query?: {
        thread?: string;
        limit?: number;
    }, userId?: string, userSignature?: string): Promise<{
        thread?: string;
        messages: Message[];
    }>;
    createMessage(msg: Message, userId?: string, userSignature?: string): Promise<Message>;
    getMessage(id: string, userId?: string, userSignature?: string): Promise<Message>;
    updateMessage(id: string, msg: Message, userId?: string, userSignature?: string): Promise<Message>;
    deleteMessage(id: string, userId?: string, userSignature?: string): Promise<any>;
    listMessageVersions(id: string, userId?: string, userSignature?: string): Promise<{
        id: string;
        versions: Message[];
    }>;
    listReactions(id: string, userId?: string, userSignature?: string): Promise<{
        id: string;
        reactions: Array<{
            id: string;
            reaction: string;
        }>;
    }>;
    addOrUpdateReaction(id: string, input: ReactionInput, userId?: string, userSignature?: string): Promise<Message>;
    removeReaction(id: string, identity: string, userId?: string, userSignature?: string): Promise<any>;
    createThread(thread: Partial<Thread>, userId?: string, userSignature?: string): Promise<Thread>;
    listThreads(userId?: string, userSignature?: string): Promise<{
        threads: Thread[];
    }>;
    getThread(id: string, userId?: string, userSignature?: string): Promise<Thread>;
    deleteThread(id: string, userId?: string, userSignature?: string): Promise<any>;
    createThreadMessage(threadID: string, msg: Message, userId?: string, userSignature?: string): Promise<Message>;
    listThreadMessages(threadID: string, query?: {
        limit?: number;
    }, userId?: string, userSignature?: string): Promise<{
        thread?: string;
        messages: Message[];
    }>;
    getThreadMessage(threadID: string, id: string, userId?: string, userSignature?: string): Promise<Message>;
    updateThreadMessage(threadID: string, id: string, msg: Message, userId?: string, userSignature?: string): Promise<Message>;
    deleteThreadMessage(threadID: string, id: string, userId?: string, userSignature?: string): Promise<any>;
    signUser(userIdToSign: string): Promise<{
        userId: string;
        signature: string;
    }>;
}
export default ProgressDBClient;
