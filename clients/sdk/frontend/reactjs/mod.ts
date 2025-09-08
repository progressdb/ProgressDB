// Re-export the built ESM-compatible entrypoint so JSR/Deno imports the compiled
// output instead of raw TSX sources which may contain bare imports (e.g. `react`).
// Ensure you run `npm run build` before publishing to JSR.
// The TypeScript build emits files under `dist/reactjs/src/index.js` in this repo
// layout. Re-export that compiled entry so JSR publishes the built output.
export * from './dist/reactjs/src/index.js';
export { default } from './dist/reactjs/src/index.js';
