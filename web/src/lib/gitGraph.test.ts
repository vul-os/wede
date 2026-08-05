import { describe, it, expect } from 'vitest'
import { buildGraph } from './gitGraph'

describe('buildGraph', () => {
  it('returns [] for empty input', () => {
    expect(buildGraph([])).toEqual([])
    expect(buildGraph(undefined)).toEqual([])
  })

  it('places linear history on a single lane', () => {
    const rows = buildGraph([
      { hash: 'c', parents: ['b'] },
      { hash: 'b', parents: ['a'] },
      { hash: 'a', parents: [] },
    ])
    expect(rows.map((r) => r.lane)).toEqual([0, 0, 0])
    expect(rows.every((r) => r.mergeLines.length === 0)).toBe(true)
  })

  it('models a branch + merge with a second lane and a merge line', () => {
    // m merges main(p1) and feature(p2); feature commit f then base a.
    const rows = buildGraph([
      { hash: 'm', parents: ['p1', 'f'] },
      { hash: 'p1', parents: ['a'] },
      { hash: 'f', parents: ['a'] },
      { hash: 'a', parents: [] },
    ])
    const merge = rows[0]
    expect(merge.lane).toBe(0)
    expect(merge.mergeLines).toHaveLength(1)
    // the second parent (f) gets routed to a separate lane
    expect(merge.mergeLines[0].from).toBe(0)
    expect(merge.mergeLines[0].to).toBeGreaterThan(0)
    // while both branches are open, at least two lanes are active
    expect(Math.max(...rows.map((r) => r.laneCount))).toBeGreaterThanOrEqual(2)
    // the branches reunite at the root commit on lane 0
    expect(rows[rows.length - 1].hash).toBe('a')
    expect(rows[rows.length - 1].lane).toBe(0)
  })

  it('frees the lane for a root commit (no parents)', () => {
    const rows = buildGraph([{ hash: 'only', parents: [] }])
    expect(rows[0].lane).toBe(0)
    expect(rows[0].mergeLines).toEqual([])
  })

  it('preserves commit fields on each row', () => {
    const rows = buildGraph([{ hash: 'x', parents: [], message: 'hi', author: 'Ava' }])
    expect(rows[0]).toMatchObject({ hash: 'x', message: 'hi', author: 'Ava' })
  })
})
