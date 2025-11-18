import BackendClient, { BackendClientOptions } from './client';

export type ProgressDBOptions = BackendClientOptions;

/**
 * ProgressDB factory — returns a ready-to-use `BackendClient` instance.
 * @param opts backend client options
 */
export default function ProgressDB(opts: ProgressDBOptions): BackendClient {
  return new BackendClient(opts);
}

export { BackendClient };
export type { BackendClientOptions };