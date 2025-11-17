import type { SDKOptionsType } from '@progressdb/js';

export type UserSignature = { userId: string; signature: string };
export type GetUserSignature = () => Promise<UserSignature> | UserSignature;

export type ProgressProviderProps = {
  children: React.ReactNode;
  options?: SDKOptionsType;
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

export type ProgressClientContextValue = {
  client: import('@progressdb/js').ProgressDBClient;
  userId?: string;
  signature?: string;
  signatureLoaded: boolean;
  signatureLoading: boolean;
  signatureError?: any;
  refreshUserSignature: () => Promise<void>;
  clearUserSignature: () => void;
};