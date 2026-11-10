import { act } from 'react'

// Use React's async act overload, including a microtask checkpoint for hooks
// that publish state from resolved promises after a render or cleanup.
export async function commit(callback: () => void) {
  await act(async () => {
    callback()
    await Promise.resolve()
  })
}
