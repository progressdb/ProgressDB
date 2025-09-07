import BackendClient, { BackendClientOptions } from './client';

export type ProgressDBOptions = BackendClientOptions;

// ProgressDB factory returns a ready-to-use client instance.
export default function ProgressDB(opts: ProgressDBOptions): BackendClient {
  return new BackendClient(opts);
}

export { BackendClient };
