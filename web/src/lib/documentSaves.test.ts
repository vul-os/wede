import { describe, expect, it, vi } from 'vitest'
import { createDocumentSaves, writeDocument } from './documentSaves'

const ok = () => new Response(JSON.stringify({ status: 'ok' }))

describe('confirmed disk saves', () => {
  it('notifies only after success, with the saved snapshot and captured workspace/path', async () => {
    const saves = createDocumentSaves()
    const saved = vi.fn()
    const otherWorkspace = vi.fn()
    const otherFile = vi.fn()
    saves.subscribe('a', 'src/main.rs', saved)
    saves.subscribe('b', 'src/main.rs', otherWorkspace)
    saves.subscribe('a', 'src/other.rs', otherFile)
    let acknowledge!: (response: Response) => void
    const authFetch = vi.fn(() => new Promise<Response>(resolve => { acknowledge = resolve }))
    const writing = writeDocument(authFetch, saves, 'a', 'src/main.rs', 'snapshot')
    expect(saved).not.toHaveBeenCalled()
    expect(authFetch).toHaveBeenCalledWith('/api/workspaces/a/files/write', expect.objectContaining({
      method: 'PUT', body: JSON.stringify({ path: 'src/main.rs', content: 'snapshot' }),
    }))
    acknowledge(ok())
    await writing
    expect(saved).toHaveBeenCalledExactlyOnceWith('snapshot')
    expect(otherWorkspace).not.toHaveBeenCalled()
    expect(otherFile).not.toHaveBeenCalled()
  })

  it.each([
    () => new Response('failure', { status: 500 }),
    () => new Response('{}'),
    () => new Response('<html>not an acknowledgement</html>'),
  ])('does not signal an unsuccessful or unacknowledged write', async response => {
    const saves = createDocumentSaves()
    const saved = vi.fn()
    saves.subscribe('a', 'main.rs', saved)
    await expect(writeDocument(() => Promise.resolve(response()), saves, 'a', 'main.rs', 'content')).rejects.toThrow()
    expect(saved).not.toHaveBeenCalled()
  })

  it('does not deliver a late acknowledgement to a destroyed or replacement editor', async () => {
    const saves = createDocumentSaves()
    const old = vi.fn()
    const replacement = vi.fn()
    const unsubscribe = saves.subscribe('a', 'main.rs', old)
    let acknowledge!: (response: Response) => void
    const writing = writeDocument(() => new Promise(resolve => { acknowledge = resolve }), saves, 'a', 'main.rs', 'old')
    unsubscribe()
    saves.subscribe('a', 'main.rs', replacement)
    acknowledge(ok())
    await writing
    expect(old).not.toHaveBeenCalled()
    expect(replacement).not.toHaveBeenCalled()
  })

  it('rejects missing identity rather than using the currently focused workspace', async () => {
    const authFetch = vi.fn(() => Promise.resolve(ok()))
    await expect(writeDocument(authFetch, createDocumentSaves(), null, 'main.rs', '')).rejects.toThrow()
    expect(authFetch).not.toHaveBeenCalled()
  })
})
