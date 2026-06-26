// buildGraph turns a flat list of commits ({ hash, parents: [...] }, newest first)
// into per-row lane data for the SVG commit graph: each row's lane index, the
// merge connections to extra parents, and the set of lanes active above the row.
// Extracted from GitPanel.jsx so the lane algorithm can be unit-tested.
export function buildGraph(entries) {
  if (!entries?.length) return []
  const lanes = []
  const rows = []

  for (const entry of entries) {
    let lane = lanes.indexOf(entry.hash)
    if (lane === -1) {
      lane = lanes.indexOf(null)
      if (lane === -1) { lane = lanes.length; lanes.push(null) }
      lanes[lane] = entry.hash
    }

    const activeBefore = lanes.slice()
    const mergeLines = []

    for (let i = 0; i < entry.parents.length; i++) {
      const p = entry.parents[i]
      if (i === 0) {
        lanes[lane] = p || null
      } else {
        let pl = lanes.indexOf(p)
        if (pl === -1) {
          pl = lanes.indexOf(null)
          if (pl === -1) { pl = lanes.length; lanes.push(null) }
          lanes[pl] = p
        }
        mergeLines.push({ from: lane, to: pl })
      }
    }
    if (entry.parents.length === 0) lanes[lane] = null

    while (lanes.length > 0 && lanes[lanes.length - 1] === null) lanes.pop()

    rows.push({
      ...entry,
      lane,
      mergeLines,
      laneCount: Math.max(lanes.length, activeBefore.length, 1),
      activeLanes: activeBefore,
    })
  }
  return rows
}
