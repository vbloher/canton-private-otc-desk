import js from "@eslint/js";
import { defineConfig, globalIgnores } from "eslint/config";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

/** Browser and I/O globals a pure layer must not touch: its ports are the way out. */
const IO_GLOBALS = ["fetch", "WebSocket", "window", "document", "localStorage", "sessionStorage", "navigator", "crypto"];

/** Clean architecture: each layer may import only from layers inside it. */
function layer(files, forbidden, message, extra = {}) {
  return {
    files,
    rules: {
      "no-restricted-imports": ["error", { patterns: [{ group: forbidden, message }] }],
      ...extra,
    },
  };
}

export default defineConfig([
  globalIgnores(["dist", "node_modules", "src/infrastructure/api/schema"]),
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2023,
      globals: globals.browser,
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
    rules: {
      "@typescript-eslint/consistent-type-imports": ["error", { fixStyle: "inline-type-imports" }],
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
      "@typescript-eslint/switch-exhaustiveness-check": "error",
      eqeqeq: ["error", "always"],
    },
  },
  // Test doubles answer with plain async functions.
  {
    files: ["**/*.test.ts"],
    rules: { "@typescript-eslint/require-await": "off" },
  },
  {
    files: ["**/*.js"],
    extends: [js.configs.recommended],
    languageOptions: { ecmaVersion: 2023, sourceType: "module", globals: globals.node },
  },
  {
    files: ["vite.config.ts"],
    languageOptions: { globals: globals.node },
  },
  // shadcn primitives export their variants, providers their hooks, next to the components.
  {
    files: ["src/presentation/components/ui/**", "src/presentation/providers/**"],
    rules: { "react-refresh/only-export-components": "off" },
  },
  layer(
    ["src/domain/**"],
    ["**/application/**", "**/infrastructure/**", "**/presentation/**", "react", "react-*", "@tanstack/*", "openapi-fetch"],
    "The domain depends on nothing: pure entities and rules.",
    { "no-restricted-globals": ["error", ...IO_GLOBALS] },
  ),
  layer(
    ["src/application/**"],
    ["**/infrastructure/**", "**/presentation/**", "react", "react-*", "@tanstack/*", "openapi-fetch"],
    "The application depends on the domain and its own ports only.",
    { "no-restricted-globals": ["error", ...IO_GLOBALS] },
  ),
  layer(
    ["src/infrastructure/**"],
    ["**/presentation/**", "react", "react-*", "@tanstack/*"],
    "Infrastructure implements the application's ports; it knows nothing of the UI.",
  ),
  layer(
    ["src/presentation/**"],
    ["**/infrastructure/**"],
    "The presentation reaches infrastructure only through the ports main.tsx wires in.",
  ),
]);
