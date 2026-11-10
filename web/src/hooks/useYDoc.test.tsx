// @vitest-environment jsdom
import { commit } from '../test/react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type * as Y from 'yjs'
import type { Awareness } from 'y-protocols/awareness'
import { useYDoc, type UseYDocParams, type UseYDocState } from './useYDoc'

const { providers } = vi.hoisted(() => ({ providers: [] as { doc: Y.Doc; awareness: Awareness; destroy: () => void }[] }))
vi.mock('y-websocket', async () => {
  const { Awareness } = await import('y-protocols/awareness')
  return { WebsocketProvider: class {
    awareness: Awareness
    destroy = vi.fn(() => this.awareness.destroy())
    constructor(_base: string, _room: string, public doc: Y.Doc) {
      this.awareness = new Awareness(doc)
      providers.push(this)
    }
  } }
})

let root: Root
let container: HTMLDivElement
let snapshots: UseYDocState[]
const defaults: UseYDocParams = { workspaceId: 'one', path: 'main.rs', token: 'token', username: 'user' }
function Harness({ params }: { params: UseYDocParams }) {
  snapshots.push(useYDoc(params))
  return null
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  providers.length = 0
  snapshots = []
  container = document.createElement('div')
  root = createRoot(container)
})
afterEach(async () => {
  await commit(() => root.unmount())
  expect(providers.every(p => p.doc.isDestroyed)).toBe(true)
  vi.unstubAllGlobals()
})

it.each([
  { path: 'other.rs' },
  { workspaceId: 'two' },
  { token: 'new-token' },
  { username: 'renamed' },
])('hides the previous Y.Text in the very first render of a changed identity: %j', async override => {
  await commit(() => root.render(<Harness params={defaults} />))
  const previous = snapshots.at(-1)?.ytext
  expect(previous).not.toBeNull()
  snapshots = []
  await commit(() => root.render(<Harness params={{ ...defaults, ...override }} />))
  expect(snapshots[0].ytext).toBeNull()
  expect(snapshots.every(s => s.ytext !== previous)).toBe(true)
  expect(snapshots.at(-1)?.ytext).not.toBeNull()
  expect(providers[0].destroy).toHaveBeenCalledOnce()
  expect(providers[0].doc.isDestroyed).toBe(true)
})

it('keeps the provider stable for the same identity and closes it when disabled', async () => {
  await commit(() => root.render(<Harness params={defaults} />))
  const previous = snapshots.at(-1)?.ytext
  await commit(() => root.render(<Harness params={{ ...defaults }} />))
  expect(providers).toHaveLength(1)
  expect(snapshots.at(-1)?.ytext).toBe(previous)
  snapshots = []
  await commit(() => root.render(<Harness params={{ ...defaults, path: null }} />))
  expect(snapshots.every(s => s.ytext === null)).toBe(true)
  expect(providers[0].destroy).toHaveBeenCalledOnce()
})
