// JSR entrypoint: export a clean surface without duplicate names
export { default as ProgressDB } from "./src/index.ts";
export type { ProgressDBOptions } from "./src/index.ts";
export { BackendClient } from "./src/client.ts";
export type { BackendClientOptions } from "./src/client.ts";
export type { Message, Thread } from "./src/types.ts";
export { ApiError } from "./src/errors.ts";
