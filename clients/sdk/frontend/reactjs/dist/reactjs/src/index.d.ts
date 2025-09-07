import React from 'react';
import ProgressDBClient, { SDKOptions, Message, Thread } from '../../typescript/src/index';
export type UserSignature = {
    userId: string;
    signature: string;
};
export type GetUserSignature = () => Promise<UserSignature> | UserSignature;
export type ProgressProviderProps = {
    children: React.ReactNode;
    options?: SDKOptions;
    /**
     * REQUIRED function used to obtain a `{ userId, signature }` pair for the current user.
     * The provider calls this function (can be async) once and attaches the returned values to
     * the underlying SDK as `defaultUserId` and `defaultUserSignature`.
     */
    getUserSignature: GetUserSignature;
    /**
     * Persist signature in `sessionStorage` to survive navigation/re-renders in the same tab.
     * Default: true
     */
    persistSignature?: boolean;
};
export declare const ProgressDBProvider: React.FC<ProgressProviderProps>;
export declare function useProgressClient(): ProgressDBClient;
export declare function useUserSignature(): {
    userId: string | undefined;
    signature: string | undefined;
    loaded: boolean;
    loading: boolean;
    error: any;
    refresh: () => Promise<void>;
    clear: () => void;
};
export declare function useMessages(threadId?: string, deps?: any[]): {
    messages: Message[] | null;
    loading: boolean;
    error: any;
    refresh: () => Promise<void>;
    create: (msg: Message) => Promise<Message>;
};
export declare function useMessage(id?: string): {
    message: Message | null;
    loading: boolean;
    error: any;
    refresh: () => Promise<void>;
    update: (msg: Message) => Promise<Message>;
    remove: () => Promise<void>;
};
export declare function useThreads(deps?: any[]): {
    threads: Thread[] | null;
    loading: boolean;
    error: any;
    refresh: () => Promise<void>;
    create: (t: Partial<Thread>) => Promise<Thread>;
};
export declare function useReactions(messageId?: string): {
    reactions: {
        id: string;
        reaction: string;
    }[] | null;
    loading: boolean;
    error: any;
    refresh: () => Promise<void>;
    add: (input: {
        id: string;
        reaction: string;
    }) => Promise<Message>;
    remove: (identity: string) => Promise<void>;
};
export type { Message, Thread };
export default ProgressDBProvider;
