import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist', 'site/assets/vendor/**']),
  {
    files: ['**/*.{js,jsx}'],
    extends: [
      js.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        ecmaVersion: 'latest',
        ecmaFeatures: { jsx: true },
        sourceType: 'module',
      },
    },
    rules: {
      'no-unused-vars': ['error', { varsIgnorePattern: '^[A-Z_]' }],
    },
  },
  {
    // The TypeScript-migrated app source (src/**). Mirrors the JS block above
    // but swaps the parser/no-unused-vars rule for TS-aware equivalents.
    // Type-aware rules are enabled (parserOptions.projectService) specifically
    // so no-floating-promises and the no-unsafe-* family actually run — wede
    // is a Yjs/CRDT editor with persistence and network sync, exactly where a
    // dropped promise rejection can mean a write that silently never lands.
    files: ['src/**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
        sourceType: 'module',
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { varsIgnorePattern: '^[A-Z_]' }],
    },
  },
  {
    // playwright.config.js runs in Node (the test runner reads env vars and
    // spawns `vite preview`). Plain JS — not part of the TS-migrated surface.
    files: ['playwright.config.js'],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    // The E2E suite (e2e/**), migrated to TypeScript. Mirrors the src block's
    // TS-aware extends/parser rather than being syntax-only. It runs in Node
    // (the Playwright test runner) but also authors inline page.evaluate /
    // addInitScript callbacks, which are written here and executed in the
    // page — both global sets are legitimate. Type-aware rules are enabled
    // the same way as the src block (tsconfig.json's include covers e2e/ too).
    files: ['e2e/**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: { ...globals.node, ...globals.browser },
      parserOptions: {
        ecmaFeatures: { jsx: true },
        sourceType: 'module',
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { varsIgnorePattern: '^[A-Z_]' }],
      // Playwright's fixture API is `async ({ page }, use) => { await use(x) }`.
      // React 19 also has a `use` hook, so rules-of-hooks sees the call and
      // demands the enclosing function be a component/hook. It is neither —
      // this is a test fixture, not React. The rule does not apply here.
      'react-hooks/rules-of-hooks': 'off',
    },
  },
])
