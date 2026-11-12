import { mkdir, readFile, writeFile } from "node:fs/promises"
import path from "node:path"

/**
 * M5 site acceptance runner.
 *
 * Read-only by default. It verifies the live site, channel health, configured
 * device points and profile availability. Control/readback tests are skipped
 * unless M5_ALLOW_WRITE=1 and the device config contains a control block.
 *
 * This tool cannot replace a physical wiring check: disconnect/reconnect,
 * stale-value blocking and duplicate submission must be exercised by a person
 * at the site and recorded in the final acceptance report.
 */

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62943"
const password = process.env.FASTEMS_ADMIN_PASSWORD || ""
const configPath = process.env.M5_CONFIG || ""
const allowWrite = process.env.M5_ALLOW_WRITE === "1"
const maxAgeMS = Number(process.env.M5_MAX_AGE_MS || 15000)

if (!password) throw new Error("FASTEMS_ADMIN_PASSWORD is required")
if (!configPath) throw new Error("M5_CONFIG is required (see docs/m5-site-acceptance.md)")
const config = JSON.parse(await readFile(configPath, "utf8"))
if (!Array.isArray(config.devices) || config.devices.length === 0) throw new Error("M5_CONFIG.devices is empty")

const login = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
})
if (!login.ok) throw new Error(`login failed: ${login.status}`)
const { token } = await login.json()

async function api(url, method = "GET", body) {
  const response = await fetch(`${base}/api${url}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let payload = null
  try { payload = text ? JSON.parse(text) : null } catch { payload = text }
  if (!response.ok) {
    const code = payload && typeof payload === "object" ? payload.error : payload
    throw new Error(`${method} ${url}: ${response.status} ${code || ""}`.trim())
  }
  return payload
}

const checks = []
function add(key, title, status, detail, object = "") {
  checks.push({ key, title, status, detail, object })
}
function nodeName(node) {
  return node.points?.find((point) => point.type === "description")?.text || ""
}
function nodePoint(node, type) {
  return node.points?.find((point) => point.type === type && (point.tombstone || 0) % 2 === 0)
}
function valueOf(node) {
  const point = nodePoint(node, "value")
  if (!point || point.value === undefined) return { value: undefined, ageMS: null }
  const time = point.time ? new Date(point.time).getTime() : 0
  return { value: point.value, ageMS: time ? Date.now() - time : null }
}

const snapshot = await api("/snapshot")
const channels = (await api("/ems/channels")).channels || []
const profiles = (await api("/ems/profiles")).profiles || []

add("api", "产品接口与登录", "passed", `已连接 ${base}`, config.site || "")
const acceptance = await api("/ems/acceptance/latest").catch((cause) => {
  add("acceptance", "验收报告", "warning", "未读取到最近验收报告；可在部署页运行", "")
  return null
})
if (acceptance) {
  add("acceptance", "验收报告", acceptance.status === "failed" ? "failed" : acceptance.status === "warning" ? "warning" : "passed",
    `${acceptance.status} · 通过 ${acceptance.summary?.passed ?? 0} / 关注 ${acceptance.summary?.warning ?? 0} / 失败 ${acceptance.summary?.failed ?? 0}`, acceptance.id || "")
}

const byName = new Map()
for (const node of snapshot.nodes || []) {
  const name = nodeName(node)
  if (!name) continue
  const list = byName.get(name) || []
  list.push(node)
  byName.set(name, list)
}

let failed = 0
let warned = 0
for (const device of config.devices) {
  const label = device.name || device.channel_id || "未命名设备"
  const object = device.channel_id || label
  const channel = channels.find((item) => item.id === device.channel_id || item.node_id === device.channel_id)
  if (!channel) {
    add(`channel:${object}`, `${label} · 通道`, "failed", "配置的通道不存在；先在部署页确认通道", object)
    failed++
    continue
  }
  const channelDevice = (channel.devices || []).find((item) => item.server_id === device.server_id)
  const health = channelDevice?.health || "unknown"
  if (!channelDevice) {
    add(`device:${object}`, `${label} · 站号 ${device.server_id ?? "—"}`, "failed", "该站号未出现在通道设备列表", object)
    failed++
  } else if (["offline", "unknown", "degraded"].includes(health)) {
    add(`device:${object}`, `${label} · 站号 ${device.server_id}`, health === "degraded" ? "warning" : "failed", `通信健康为 ${health}`, object)
    if (health === "degraded") warned++; else failed++
  } else {
    add(`device:${object}`, `${label} · 站号 ${device.server_id}`, "passed", `通信健康 ${health}`, object)
  }

  if (device.profile_id) {
    const profile = profiles.find((item) => item.id === device.profile_id)
    if (!profile) {
      add(`profile:${device.profile_id}`, `${label} · Profile`, "failed", `Profile ${device.profile_id} 不存在`, device.profile_id)
      failed++
    } else {
      add(`profile:${device.profile_id}`, `${label} · Profile`, profile.verified ? "passed" : "warning",
        `${profile.id} · ${profile.points} 点位 · ${profile.verified ? "已现场验证" : "未现场验证"}`, profile.id)
      if (!profile.verified) warned++
    }
  }

  for (const required of device.required_points || []) {
    const matches = byName.get(required) || []
    if (matches.length === 0) {
      add(`point:${required}`, `${label} · ${required}`, "failed", "快照中没有同名点位", object)
      failed++
      continue
    }
    if (matches.length > 1) {
      add(`point:${required}`, `${label} · ${required}`, "warning", `存在 ${matches.length} 个同名点位，结果可能有歧义`, object)
      warned++
      continue
    }
    const { value, ageMS } = valueOf(matches[0])
    if (value === undefined) {
      add(`point:${required}`, `${label} · ${required}`, "failed", "点位没有 value", object)
      failed++
    } else if (ageMS === null || ageMS > maxAgeMS) {
      add(`point:${required}`, `${label} · ${required}`, "failed", ageMS === null ? "点位没有时间戳" : `${Math.round(ageMS)} ms 未更新，超过 ${maxAgeMS} ms`, object)
      failed++
    } else {
      add(`point:${required}`, `${label} · ${required}`, "passed", `当前值 ${value} · ${Math.round(ageMS)} ms 前`, object)
    }
  }

  if (device.control) {
    if (!allowWrite) {
      add(`control:${object}`, `${label} · 控制与回读`, "skipped", "只读模式；设置 M5_ALLOW_WRITE=1 后执行控制/回读", object)
    } else {
      await runControl(device, label, object)
    }
  }
}

async function runControl(device, label, object) {
  const control = device.control
  if (!control.target_node_id || control.value === undefined) {
    add(`control:${object}`, `${label} · 控制与回读`, "failed", "control 缺少 target_node_id 或 value", object)
    failed++
    return
  }
  const timeout = Number(control.timeout_ms || 5000)
  const tolerance = Number(control.tolerance || 0.01)
  const started = Date.now()
  try {
    await api(`/nodes/${encodeURIComponent(control.target_node_id)}/value`, "POST", { value: control.value, point_type: "valueSet" })
    let matched = false
    while (Date.now() - started < timeout) {
      await new Promise((resolve) => setTimeout(resolve, 250))
      const live = await api("/snapshot")
      const readback = (live.nodes || []).find((node) => node.id === control.readback_node_id)
      const { value, ageMS } = readback ? valueOf(readback) : { value: undefined, ageMS: null }
      if (typeof value === "number" && Math.abs(value - (control.expected ?? control.value)) <= tolerance && ageMS !== null && ageMS <= maxAgeMs) {
        matched = true
        break
      }
    }
    if (matched) {
      add(`control:${object}`, `${label} · 控制与回读`, "passed", `写入 ${control.value}，回读在 ${Date.now() - started} ms 内匹配`, object)
    } else {
      add(`control:${object}`, `${label} · 控制与回读`, "failed", `写入 ${control.value} 后 ${timeout} ms 内未匹配回读`, object)
      failed++
    }
  } catch (cause) {
    add(`control:${object}`, `${label} · 控制与回读`, "failed", cause instanceof Error ? cause.message : "写入失败", object)
    failed++
  }
  if (control.restore !== undefined) {
    try {
      await api(`/nodes/${encodeURIComponent(control.target_node_id)}/value`, "POST", { value: control.restore, point_type: "valueSet" })
      add(`restore:${object}`, `${label} · 恢复目标`, "passed", `已写回 ${control.restore}`, object)
    } catch (cause) {
      add(`restore:${object}`, `${label} · 恢复目标`, "failed", cause instanceof Error ? cause.message : "恢复失败", object)
      failed++
    }
  }
}

const report = {
  site: config.site || "未命名站点",
  generated_at: new Date().toISOString(),
  base,
  mode: allowWrite ? "read-write" : "read-only",
  summary: {
    passed: checks.filter((check) => check.status === "passed").length,
    warning: checks.filter((check) => check.status === "warning").length,
    failed: checks.filter((check) => check.status === "failed").length,
    skipped: checks.filter((check) => check.status === "skipped").length,
  },
  checks,
  note: "本报告只证明本次运行检查；断线/恢复、过期阻断、重复提交和真实设备签收必须由现场人员完成。",
}

const reportPath = process.env.M5_REPORT || path.resolve(`../evidence/m5-site-${Date.now()}.json`)
await mkdir(path.dirname(reportPath), { recursive: true })
await writeFile(reportPath, JSON.stringify(report, null, 2) + "\n")
console.log(JSON.stringify({ report: reportPath, mode: report.mode, summary: report.summary }, null, 2))
if (report.summary.failed > 0) process.exit(1)
