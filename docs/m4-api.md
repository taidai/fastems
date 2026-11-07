# M4 API 契约

版本：2026-11-04 · 状态：**待实现，先冻结形状**
鉴权：全部走现有登录令牌；写操作仅管理员，读操作按业务树范围。
约束：不返回密码、令牌或连接凭据；扫描只读，不写设备。

## 1. 通道（持久化为业务节点）

### GET `/api/ems/channels`

```json
{
  "channels": [
    {
      "id": "busNodeID",
      "node_id": "busNodeID",
      "persisted": true,
      "protocol": "TCP",
      "address": "127.0.0.1:1502",
      "period_ms": 1000,
      "devices": [
        {
          "id": "busNodeID/1",
          "server_id": 1,
          "health": "healthy",
          "points": [
            {
              "id": "ioNodeID",
              "name": "柜内温度",
              "function": "holdingRegister",
              "address": 10,
              "format": "uint16",
              "scale": 0.1,
              "offset": 0,
              "kind": "number",
              "read_only": true
            }
          ]
        }
      ]
    }
  ]
}
```

`health` 取值：`unknown | healthy | degraded | offline`。

### POST `/api/ems/channels`

请求：

```json
{
  "parent": "业务节点ID",
  "protocol": "tcp",
  "address": "127.0.0.1:1502",
  "baud": 9600,
  "period_ms": 1000,
  "timeout_ms": 500,
  "devices": [
    {
      "server_id": 1,
      "points": [
        {
          "name": "柜内温度",
          "function": "holdingRegister",
          "address": 10,
          "format": "uint16",
          "scale": 0.1,
          "offset": 0,
          "kind": "number",
          "read_only": true
        }
      ]
    }
  ]
}
```

- `parent` 必须是当前账号可写的分组或设备。
- 服务端写入 `modbus` 节点与 `modbusIo` 子节点，重启由桥接恢复。
- 响应：`{ "channel_id": "...", "node_id": "...", "point_nodes": ["..."] }`。
- 错误：`invalid_channel`、`invalid_device`、`invalid_point`、`forbidden`、`create_failed`。

### DELETE `/api/ems/channels/{id}`

删除通道节点及其点位节点；运行中的通道先停止再删除。
响应：`{ "deleted": "busNodeID" }`。

## 2. 扫描与识别（只读）

### POST `/api/ems/scan`

请求：

```json
{
  "protocol": "tcp",
  "address": "127.0.0.1:1502",
  "baud": 9600,
  "from": 1,
  "to": 16,
  "timeout_ms": 300
}
```

响应：

```json
{
  "scanned": 16,
  "found": [
    {
      "server_id": 1,
      "responded": true,
      "latency_ms": 12,
      "probe": "holdingRegister:0",
      "error": ""
    }
  ],
  "errors": []
}
```

- 每站号只做一次只读探测：保持寄存器 0 → 输入寄存器 0；任一成功即 `responded=true`。
- 扫描不写设备。对已运行通道，扫描必须排队或拒绝对同一串口/地址并发打开；拒绝时返回 `channel_busy`。
- 站号范围 1–247；`from > to` 返回 `invalid_range`；单次最多 64 个站号，超过返回 `range_too_large`。
- `timeout_ms` 默认 300，允许 50–2000；超时越短扫描越快但可能漏掉慢设备。

### POST `/api/ems/identify`

请求：`{ "protocol": "tcp", "address": "...", "baud": 9600, "server_id": 1, "profile_id": "" }`

响应：

```json
{ "server_id": 1, "matched": false, "profile": null, "candidates": [], "detail": "没有匹配的内置 Profile" }
```

### GET `/api/ems/profiles`

```json
{
  "profiles": [
    {
      "id": "enjo-pcs",
      "vendor": "恩玖",
      "model": "PCS（V1.20/V1.27）",
      "protocol": "modbus",
      "points": 14,
      "commands": 2,
      "verified": false,
      "verification": "协议文档 + 台山站真机地址核对；未在本机实机复测",
      "source": "《PCS通信协议（通用版本）》V1.27 + 台山站点位梳理",
      "identity_hint": "读保持寄存器 0x6000~0x6002 与 0x6070~0x607F 确认型号",
      "point_specs": [
        { "name": "交流输出总有功功率", "function": "holdingRegister", "address": 24633, "format": "int16", "scale": 0.1, "offset": 0, "unit": "kW", "read_only": true }
      ],
      "command_specs": [
        { "id": "on", "name": "开机", "point": "开机/关机指令", "value": 1 }
      ]
    }
  ]
}
```

内置 6 个厂商 Profile（恩玖 PCS、ESBCM BMS、DTSD3366D 电表、YD2037 电表、华为逆变器、吉瓦时代 MPPT）加 `modbus-generic`。所有厂商 Profile 当前 `verified=false`，只代表从协议文档提取的 **精简点位**，未现场签收；自动匹配仍受厂商标识寄存器限制，未匹配时可人工选择，不阻塞接入。

### GET `/api/ems/profiles/{id}/points.csv?server_id=1`

返回该 Profile 的精简点表，格式与点表导入一致：`名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位`。`server_id` 为占位站号（1–247，默认 1），导入前必须按现场站号核对。未知 Profile 返回 404 `unknown_profile`。

## 3. 点表校验

复用现有、已验收的 Modbus 点表接口（与通道节点同一份配置）：

- `POST /api/modbus/{channelId}/io/import?preview=true`：CSV 预检，只读，不写点位。
- `POST /api/modbus/{channelId}/io/import`：确认导入，整批成功或整批回滚。
- `GET /api/modbus/{channelId}/io/export`：导出 CSV。

CSV 列：`名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位`。
错误响应包含行号与原因；预检失败不写入任何点位。

## 4. 验收报告

### POST `/api/ems/acceptance/run`

响应：

```json
{
  "id": "acc-20261104-1",
  "generated_at": "2026-11-04T16:00:00Z",
  "status": "warning",
  "checks": [
    {
      "key": "channels",
      "title": "通道通信",
      "status": "passed",
      "detail": "1 条通道在线",
      "object": "busNodeID",
      "next_step": ""
    }
  ],
  "summary": { "passed": 5, "warning": 1, "failed": 0 }
}
```

检查项：接口与前端、通道通信、点位质量、规则发布、报警、历史、权限边界、备份状态。
`status`：`passed | warning | failed`；失败项必须给出可定位对象与下一步。

### GET `/api/ems/acceptance/latest`

返回最近一次报告；没有时 404 `no_report`。

## 4.5 分配预览（只计算，不下发）

`POST /api/ems/allocation/preview`

```json
{
  "target": 800,
  "members": [
    { "id": "CAB01", "online": true, "available_discharge": 200, "max_discharge": 200 },
    { "id": "CAB02", "online": true, "available_discharge": 200, "max_discharge": 200 },
    { "id": "CAB03", "online": true, "available_discharge": 150, "max_discharge": 150 },
    { "id": "CAB04", "online": false, "available_discharge": 200, "max_discharge": 200 }
  ]
}
```

响应：`{ "preview": true, "result": { "requested", "delivered", "shortfall", "shares": [...], "notes": [...] } }`

- 正值为放电，负值为充电；按可用能力比例分配，排除离线/故障，夹到上限，剩余再分配。
- 只做计算，不写任何设备。用于验证 4 柜分配语义与解释。
- “决策表 → 分配器 → PCS 回读”的运行接线尚未实现，不能用本接口冒充端到端控制。

## 4.6 分配器配置与运行

### GET `/api/ems/allocators/{nodeId}`

返回该业务节点上保存的分配器；未配置返回 404 `allocator_not_found`。

### PUT `/api/ems/allocators/{nodeId}`

仅管理员可写。配置以 `easyemsAllocator` 点值保存在业务节点上，重启后恢复。管理员也可在左侧“分配器”页直接编辑这些字段；页面刷新后恢复当前节点与配置。

```json
{
  "grace_ms": 1000,
  "timeout_ms": 3000,
  "members": [
    {
      "id": "CAB01",
      "target": { "node_id": "pcs1-power", "point_type": "valueSet" },
      "max_discharge": 200,
      "max_charge": 200,
      "available_discharge": { "node_id": "bms1-allow-discharge", "point_type": "value" },
      "available_charge": { "node_id": "bms1-allow-charge", "point_type": "value" },
      "readback": { "node_id": "pcs1-actual-power", "point_type": "value" },
      "fault": { "node_id": "pcs1-fault", "point_type": "value" }
    }
  ]
}
```

- `target` 是成员的功率设定点，必须是可写 `valueSet` 点；`readback` 可省略，省略时读取目标点的 `value`。
- 能力上限必须非负；可用功率输入省略时使用静态上限。能力输入过期或不可用时先等待重试，明确离线/故障/停用的成员剔除。
- 整数寄存器按缩放后的设备分辨率向零取整，不写入不可表示的小数；浮点寄存器保持原值。
- 分配器成员必须位于该业务节点的子树内，且同一功率目标不能分配给两个成员。
- 同一分配器同一时刻只能由一个已启用决策表拥有；重复拥有会被拒绝。

### 运行语义

决策表动作绑定该业务节点后，执行顺序为：目标功率 → 能力分配 → 成员逐台写入 → 新回读匹配 → 下一台。超时标为“结果未知”并停止后续动作，不自动重写。

`lastCheck` 保存请求、每台目标值、实际值、确认状态、剔除原因和整体缺口。重启用 `lastMatch` 抑制重放。

复现脚本：`frontend/allocation-e2e.mjs`。它创建 4 台 Modbus TCP 模拟 PCS，验证 800 kW → 200/200/150/0、离线剔除、超时停止与重启恢复；它不是实机验收。

## 4.7 普通输出安全约束

### PUT `/api/safety`

```json
{
  "node": "modbus-io-id",
  "safety": {
    "min": 0, "max": 100,
    "max_discharge": 60, "max_charge": 40,
    "ramp_per_second": 10,
    "conditions": [
      { "point_id": "soc-node", "operator": "ge", "value": 10 },
      { "point_id": "run-node", "operator": "eq", "kind": "bool", "value": 1 }
    ]
  }
}
```

- 配置保存在目标点位节点的 `easyemsSafety` 点；管理员/实施可写。
- 范围需同时给 min/max 且 min < max；充放电上限为非负幅值；条件支持数值/开关/枚举。
- 运行时普通规则动作先经过范围/功率上限夹限，再校验条件；条件点位缺失、过期或不满足会阻止下发，原因写入 `lastCheck`。
- `ramp_per_second` 让设定值按每秒最大变化逼近目标；规则已触发后仍按周期继续，每步重新校验安全条件，条件失效或人工接管时停止。
- 未配置安全约束时保持存量行为，界面以橙点提示。

## 4.8 Profile 自动识别

`POST /api/ems/identify` 读取各 Profile 的特征寄存器，返回：

```json
{
  "matched": true,
  "profile": { "id": "yd2037-meter", "...": "..." },
  "candidates": [
    { "profile": { "id": "yd2037-meter" }, "score": 9, "evidence": "0xAB80 型号文本：读到 \"YD2037Y-F\"", "strong": true }
  ],
  "detail": "自动匹配：0xAB80 型号文本：读到 \"YD2037Y-F\""
}
```

- 强匹配：型号文本或精确特征字命中；弱候选：特征寄存器可读但未识别到型号文本，必须人工确认。
- 识别只读，不写设备；候选按强匹配优先、分数排序，最多 5 个。
- 传入 `profile_id` 表示人工选择，不执行自动探测。

## 5. 运行时状态

### GET `/api/ems/runtime`

```json
{ "source": "nodes", "channels": 1, "devices": 1, "points": 12, "started_at": "..." }
```

用于确认只有一套运行路径：`source` 固定为 `nodes`。

## 6. 实现顺序

1. 通道持久化 + 现有 Modbus 节点桥接（M4-1）。
2. 扫描/识别（M4-2）。
3. 点表预检与导入（M4-3）。
4. 验收报告（M4-4）。

前端可在后端落地前按本契约写页面；未实现的接口统一返回 501 `not_implemented`，不伪造结果。
