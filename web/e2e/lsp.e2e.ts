import { test, expect, seed } from './fixtures'
import type { RpcMessage } from '../src/test/lspSocket'

for (const autoSave of [false, true]) {
  test(`${autoSave ? 'autosave' : 'manual save'} sends didSave only after the disk-write acknowledgement`, async ({ wede }) => {
    const { page } = wede
    await seed(page, 'authed')
    await page.addInitScript(autoSave => {
      localStorage.setItem('wede_editor_settings', JSON.stringify({ autoSave, lsp: true, collab: false }))
    }, autoSave)
    await page.route('**/lsp/available', route => route.fulfill({ json: { available: { go: true } } }))
    const messages: RpcMessage[] = []
    await page.routeWebSocket(/\/lsp\?/, socket => {
      socket.onMessage(raw => {
        const message = JSON.parse(String(raw)) as RpcMessage
        messages.push(message)
        if (message.method === 'initialize') socket.send(JSON.stringify({ jsonrpc: '2.0', id: message.id, result: {
          capabilities: { textDocumentSync: { openClose: true, change: 1, save: { includeText: true } } },
        } }))
      })
    })
    let acknowledge!: () => void
    const gate = new Promise<void>(resolve => { acknowledge = resolve })
    await page.route('**/files/write', async route => {
      await gate
      await route.fulfill({ json: { status: 'ok' } })
    })
    try {
      await page.goto('/')
      await page.getByText('main.go', { exact: true }).click()
      await expect(page.getByRole('status')).toHaveText('LSP ready')
      const editor = page.locator('.cm-content')
      await editor.click()
      await page.keyboard.press('ControlOrMeta+End')
      const write = page.waitForRequest(r => new URL(r.url()).pathname.endsWith('/files/write'))
      await page.keyboard.insertText('\n// acknowledged save test')
      if (!autoSave) await page.keyboard.press('ControlOrMeta+s')
      const request = await write
      expect(new URL(request.url()).pathname).toBe('/api/workspaces/ws_default/files/write')
      expect(messages.some(m => m.method === 'textDocument/didSave')).toBe(false)
      acknowledge()
      await expect.poll(() => messages.filter(m => m.method === 'textDocument/didSave').length).toBe(1)
      const saved = messages.find(m => m.method === 'textDocument/didSave')!
      expect(saved.params?.textDocument?.uri).toBe('file:///home/dev/demo/main.go')
      expect(saved.params?.text).toContain('// acknowledged save test')
      await expect(editor).toContainText('// acknowledged save test')
    } finally {
      acknowledge()
    }
  })
}

test('capacity rejection is visible in the existing status bar', async ({ wede }, testInfo) => {
  const { page } = wede
  await seed(page, 'authed')
  await page.route('**/lsp/available', route => route.fulfill({ json: { available: { go: true } } }))
  let connections = 0
  await page.routeWebSocket(/\/lsp\?/, socket => {
    connections++
    socket.onMessage(raw => {
      const message = JSON.parse(String(raw)) as RpcMessage
      if (message.method === 'initialize') {
        socket.send(JSON.stringify({ jsonrpc: '2.0', method: 'window/showMessage', params: {
          type: 1, message: 'wede: failed to start language server: session limit reached (2)',
        } }))
        void socket.close({ code: 1013, reason: 'capacity' })
      }
    })
  })
  await page.goto('/')
  await page.getByText('main.go', { exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('LSP unavailable')
  await expect(page.getByRole('status')).toHaveAttribute('title', /session limit reached.*No automatic retry/)
  await page.locator('.cm-content').click()
  await page.keyboard.insertText('// editing still works\n')
  await expect(page.locator('.cm-content')).toContainText('editing still works')
  expect(connections).toBe(1)
  await page.screenshot({ path: testInfo.outputPath('lsp-unavailable.png') })
})
