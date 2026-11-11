// 校验验收状态 JSON：A01–A23 齐全、状态合法、实现/部分必须带证据。
// 运行：cd frontend && node acceptance-status-check.mjs
import { readFile } from "node:fs/promises"

const file = new URL("./src/data/acceptance-status.json", import.meta.url)
const data = JSON.parse(await readFile(file, "utf8"))
const allowed = new Set(["implemented", "partial", "unimplemented"])
const failures = []

if (!Array.isArray(data.rows) || data.rows.length !== 23) {
  failures.push(`应有 23 行，实际 ${data.rows?.length ?? 0}`)
}

data.rows?.forEach((row, index) => {
  const expected = `A${String(index + 1).padStart(2, "0")}`
  if (row.id !== expected) failures.push(`第 ${index + 1} 行应为 ${expected}，实际 ${row.id}`)
  if (!allowed.has(row.status)) failures.push(`${row.id} 状态非法：${row.status}`)
  if (!row.title) failures.push(`${row.id} 缺少标题`)
  if (row.status !== "unimplemented" && (!Array.isArray(row.evidence) || row.evidence.length === 0)) {
    failures.push(`${row.id} 是 ${row.status}，但没有任何证据`)
  }
  if (row.status !== "implemented" && !row.gap) failures.push(`${row.id} 是 ${row.status}，但没有写缺口`)
})

const counts = { implemented: 0, partial: 0, unimplemented: 0 }
for (const row of data.rows || []) counts[row.status] = (counts[row.status] || 0) + 1

if (failures.length) {
  console.error("acceptance-status check FAILED:\n- " + failures.join("\n- "))
  process.exit(1)
}
console.log("acceptance-status check passed:", { ...counts, updated_at: data.updated_at })
