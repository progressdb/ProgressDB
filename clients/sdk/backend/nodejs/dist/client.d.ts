import { Message, Thread } from './types';
export type BackendClientOptions = {
    baseUrl: string;
    apiKey: string;
    timeoutMs?: number;
    maxRetries?: number;
};
export declare class BackendClient {
    baseUrl: string;
    apiKey: string;
    timeoutMs?: number;
    maxRetries?: number;
    constructor(opts: BackendClientOptions);
    private headers;
    request<T>(method: string, path: string, body?: any): Promise<T>;
    signUser(userId: string): Promise<{
        userId: string;
        signature: string;
    }>;
    adminHealth(): Promise<{
        status: string;
        service?: string;
    }>;
    adminStats(): Promise<{
        threads: number;
        messages: number;
    }>;
    listThreads(): Promise<Thread[]>;
    deleteThread(id: string): Promise<void>;
    createThread(t: Partial<Thread>): Promise<Thread>;
    createMessage(m: Partial<Message>): Promise<Message>;
}
export default BackendClient;
