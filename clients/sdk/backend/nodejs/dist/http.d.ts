export type HttpOptions = {
    timeoutMs?: number;
    maxRetries?: number;
};
export declare function httpRequest<T>(baseUrl: string, method: string, path: string, body?: any, headers?: Record<string, string>, opts?: HttpOptions): Promise<T>;
