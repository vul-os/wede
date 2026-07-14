import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
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
    // Node-context files: the Playwright config and the E2E suite run in Node
    // (the test runner), but they also reference browser globals inside
    // page.evaluate / addInitScript callbacks, which are authored inline here
    // and executed in the page. Both global sets are legitimate.
    files: ['playwright.config.js', 'e2e/**/*.{js,jsx}'],
    languageOptions: {
      globals: { ...globals.node, ...globals.browser },
    },
    rules: {
      // Playwright's fixture API is `async ({ page }, use) => { await use(x) }`.
      // React 19 also has a `use` hook, so rules-of-hooks sees the call and
      // demands the enclosing function be a component/hook. It is neither —
      // this is a test fixture, not React. The rule does not apply here.
      'react-hooks/rules-of-hooks': 'off',
    },
  },
])
