// Re-export the built ESM-compatible entrypoint so JSR/Deno imports the compiled
// output instead of raw TSX sources which may contain bare imports (e.g. `react`).
// Ensure you run `npm run build` before publishing to JSR.
export * from './dist/index.js';
export { default } from './dist/index.js';
