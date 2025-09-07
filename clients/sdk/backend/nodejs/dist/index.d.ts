import BackendClient, { BackendClientOptions } from './client';
export type ProgressDBOptions = BackendClientOptions;
export default function ProgressDB(opts: ProgressDBOptions): BackendClient;
export { BackendClient };
