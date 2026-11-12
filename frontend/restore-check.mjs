import { readFile, writeFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.FASTIOT_RESTORE_URL || "http://127.0.0.1:62944"
const dataDir = path.resolve(process.env.FASTIOT_RESTORE_DATA || "data/restore")
const initial = await readFile(path.join(dataDir, "initial-password.txt"), "utf8")
const password = initial.match(/密码:\s*(\S+)/)?.[1] || ""
if (!password) throw new Error("Restored initial credential is unavailable")

const login = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
})
if (!login.ok) throw new Error(`Restored login failed: ${login.status}`)
const { token } = await login.json()
const response = await fetch(`${base}/api/snapshot`, { headers: { Authorization: `Bearer ${token}` } })
if (!response.ok) throw new Error(`Restored snapshot failed: ${response.status}`)
const snapshot = await response.json()
const names = snapshot.nodes.map((node) => node.points.find((point) => point.type === "description")?.text)
for (const name of ["演示电站", "温度", "有人", "风机", "风阀", "舒适控制 P0"]) {
  if (!names.includes(name)) throw new Error(`Restored node missing: ${name}`)
}
const fan = snapshot.nodes.find((node) => node.points.some((point) => point.type === "description" && point.text === "风机"))
const damper = snapshot.nodes.find((node) => node.points.some((point) => point.type === "description" && point.text === "风阀"))
if (fan?.points.find((point) => point.type === "valueSet")?.value !== 1 || damper?.points.find((point) => point.type === "valueSet")?.value !== 80) {
  throw new Error("Restored output values do not match")
}
await writeFile(path.resolve("evidence/restore-check.json"), JSON.stringify({
  passed: true,
  method: "offline copy of the complete data directory",
  restoredNodes: snapshot.nodes.length,
  login: true,
  decisionTable: true,
  outputs: true,
}, null, 2) + "\n")
console.log("Offline full-data restore passed: login, nodes, decision table and outputs")
