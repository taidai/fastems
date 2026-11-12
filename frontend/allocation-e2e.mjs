import { writeFile } from "node:fs/promises"

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62951"
const password = process.env.FASTEMS_ADMIN_PASSWORD || ""
if (!password) throw new Error("FASTEMS_ADMIN_PASSWORD is required")

async function api(path, method = "GET", body) {
  const response = await fetch(`${base}/api${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let payload
  try { payload = JSON.parse(text) } catch { payload = text }
  if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${text}`)
  return payload
}

const loginResponse = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
})
if (!loginResponse.ok) throw new Error(`login: ${loginResponse.status}`)
const { token } = await loginResponse.json()

function point(node, type) {
  return node.points.find((item) => item.type === type && (item.tombstone || 0) % 2 === 0)
}

async function snapshot() { return api("/snapshot") }

async function waitFor(predicate, label, timeout = 15000) {
  const deadline = Date.now() + timeout
  let last
  while (Date.now() < deadline) {
    last = await predicate()
    if (last) return last
    await new Promise((resolve) => setTimeout(resolve, 150))
  }
  throw new Error(`timeout: ${label}; last=${JSON.stringify(last)}`)
}

async function lastCheck(ruleID) {
  const snap = await snapshot()
  const rule = snap.nodes.find((node) => node.id === ruleID)
  const text = point(rule, "lastCheck")?.text
  return text ? JSON.parse(text) : null
}

async function createNode(parent, type, description, value) {
  return api("/nodes", "POST", { parent, type, description, value })
}

async function createPCS(parent, name, port, capacity) {
  const bus = await api("/modbus", "POST", {
    parent, description: `${name} PCS`, protocol: "TCP", uri: `127.0.0.1:${port}`,
    poll_period: 200, timeout: 200,
  })
  const power = await api(`/modbus/${bus.id}/io`, "POST", {
    description: `${name} 功率`, server_id: 1, address: 10,
    io_type: "modbusHoldingRegister", data_format: "uint16", scale: 0.1,
    offset: 0, read_only: false, units: "kW",
  })
  const cap = await api(`/modbus/${bus.id}/io`, "POST", {
    description: `${name} 可用功率`, server_id: 1, address: 11,
    io_type: "modbusHoldingRegister", data_format: "uint16", scale: 0.1,
    offset: 0, read_only: true, units: "kW",
  })
  let readback
  if (name === "CAB02") {
    readback = await api(`/modbus/${bus.id}/io`, "POST", {
      description: `${name} 固定回读`, server_id: 1, address: 12,
      io_type: "modbusHoldingRegister", data_format: "uint16", scale: 1,
      offset: 0, read_only: true, units: "kW",
    })
  }
  return { bus: bus.id, power: power.id, cap: cap.id, readback: readback?.id, capacity }
}

const mode = process.argv[2] || "full"
const root = (await snapshot()).roots[0]
const grid = mode === "verify"
  ? null
  : (await createNode(root, "variable", "分配测试电网", 50)).id

if (mode === "verify") {
  const snap = await snapshot()
  const pcs = ["CAB01", "CAB02", "CAB03"].map((name) => snap.nodes.find((node) =>
    point(node, "description")?.text === `${name} 功率`))
  const values = pcs.map((node) => point(node, "value")?.value)
  if (JSON.stringify(values) !== JSON.stringify([145.4, 145.4, 150])) {
    throw new Error(`restart replay or lost values: ${JSON.stringify(values)}`)
  }
  const result = { restart_no_replay: true, actual_kw: values }
  await writeFile("evidence/m5-allocation-restart.json", JSON.stringify(result, null, 2))
  console.log(JSON.stringify(result))
}

if (mode !== "verify") {
const group = (await createNode(root, "group", "分配测试储能系统", 0)).id
const pcs = []
for (const [index, capacity] of [200, 200, 150, 200].entries()) {
  pcs.push(await createPCS(group, `CAB0${index + 1}`, 1602 + index, capacity))
}
const config = {
  grace_ms: 1000,
  timeout_ms: 3000,
  members: pcs.map((item, index) => ({
    id: `CAB0${index + 1}`,
    target: { node_id: item.power, point_type: "valueSet" },
    max_discharge: item.capacity,
    max_charge: item.capacity,
    available_discharge: { node_id: item.cap, point_type: "value" },
    available_charge: { node_id: item.cap, point_type: "value" },
  })),
}
await api(`/ems/allocators/${group}`, "PUT", config)
const loaded = await api(`/ems/allocators/${group}`)
if (loaded.members.length !== 4) throw new Error("allocator did not persist")

const document = {
  table: { rows: [{
    row_id: "discharge",
    conditions: [{ point_id: "grid", operator: "gt", value: 0 }],
    actions: [{ action_id: "ess-power", value: 800 }],
  }] },
  bindings: {
    points: { grid: { node_id: grid, point_type: "value" } },
    actions: { "ess-power": { node_id: group } },
  },
}
const rule = await api("/rules", "POST", { parent: root, description: "分配测试规则", document })
await api(`/rules/${rule.id}/publish`, "POST", document)
await api(`/rules/${rule.id}/enable`, "POST", { enabled: true })

const first = await waitFor(async () => {
  const check = await lastCheck(rule.id)
  return check?.actions?.[0]?.status === "confirmed" ? check : null
}, "initial allocation")
const firstAction = first.actions[0]
const expected = [200, 200, 150, 0]
for (const [index, share] of firstAction.shares.entries()) {
  if (share.setpoint !== expected[index]) throw new Error(`wrong setpoint: ${JSON.stringify(share)}`)
  if (index < 3 && (!share.confirmed || share.actual !== expected[index])) {
    throw new Error(`missing readback: ${JSON.stringify(share)}`)
  }
}
if (!firstAction.shares[3].excluded) throw new Error("offline PCS was not excluded")

const before = await snapshot()
const actual = pcs.map((item) => {
  const node = before.nodes.find((candidate) => candidate.id === item.power)
  return point(node, "value")?.value
})
if (JSON.stringify(actual) !== JSON.stringify([200, 200, 150, null])) {
  throw new Error(`unexpected actual values: ${JSON.stringify(actual)}`)
}

// Make CAB02's readback a different, fixed point. Its write is accepted, but
// the readback never matches, so CAB03 must not advance.
config.members[1].readback = { node_id: pcs[1].readback, point_type: "value" }
await api(`/ems/allocators/${group}`, "PUT", config)
document.table.rows[0].actions[0].value = 400
await api(`/rules/${rule.id}/publish`, "POST", document)
const second = await waitFor(async () => {
  const check = await lastCheck(rule.id)
  return check?.actions?.[0]?.status === "unknown" ? check : null
}, "unknown readback")
const after = await snapshot()
const third = after.nodes.find((node) => node.id === pcs[2].power)
if (point(third, "valueSet")?.value !== 150) {
  throw new Error("CAB03 advanced after CAB02 timeout")
}

const result = {
  initial: { requested_kw: 800, delivered_kw: 550, actual_kw: actual, check: firstAction },
  timeout: { check: second.actions[0], third_member_setpoint: point(third, "valueSet")?.value },
  allocator_persisted: loaded.members.length === 4,
  note: "Modbus TCP simulators; not physical device acceptance",
}
await writeFile("evidence/m5-allocation.json", JSON.stringify(result, null, 2))
console.log("ALLOCATION E2E PASSED: 200/200/150/0, sequential readback, offline excluded, timeout stops remaining")
}
