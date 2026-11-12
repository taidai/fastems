# FastEMS 领域模型 · 数据模型设计

日期：2026-11-08
输入：14 决策定案 + 技术调研（node 类型机制 + 改动面）

## 节点类型（FastEMS 层定义，不碰 upstream）

```go
// internal/appapi 或 internal/ems 内定义
const (
    NodeTypeSite       = "site"
    NodeTypePlant      = "plant"
    NodeTypeEnergyUnit = "energyUnit"
    // Device 复用 data.NodeTypeDevice = "device"（upstream 已有）
)
```
均为纯数据节点（无 client），只在 FastEMS 层定义常量 + `node.Type == "site"` 识别。

## 四层节点字段（存为节点 point）

| 节点 | 字段（point type） | 说明 |
|---|---|---|
| **Site** | `assetCode`、`description`、`totalTarget`、`totalConstraint` | 场站：稳定 ID + 总目标/总约束（Q10=B 调度层） |
| **Plant** | `assetCode`、`description`、`plantTarget`、`plantConstraint` | 电站：稳定 ID + 电站目标/约束 |
| **EnergyUnit** | `assetCode`、`description`、`energyType`、`ratedEnergyKWh`、`ratedPowerKW`、`capability` | 柜体：稳定 ID + 类型(storage，预留 pv/ev) + 额定容量/功率 + 汇总能力 |
| **Device** | `assetCode`、`profileId`+`pointMap`、`ratedCapabilityOverride` | 设备：稳定 ID + Profile 绑定（从通道移到设备）+ 实例覆盖额定能力 |

## Capability 模型（Q5=C 分层）

```go
type Capability struct {
    Kind        string  `json:"kind"`         // charge / discharge / energy / session
    Direction   string  `json:"direction"`    // charge / discharge / bidir
    PowerMinKW  float64 `json:"power_min_kw"`
    PowerMaxKW  float64 `json:"power_max_kw"`
    RampKWs     float64 `json:"ramp_kw_s"`
    Allowed     bool    `json:"allowed"`
    LimitReason string  `json:"limit_reason,omitempty"`
}
```
- **Rated（额定）**：Profile 存型号默认；Device 存 `ratedCapabilityOverride`（实例覆盖，现场降额）。
- **Dynamic（动态）**：运行时从 Device 点位算（BMS 可用充/放功率），EnergyUnit 汇总多设备 → `capability` 点。

## asset_code（Q4=C 混合）

- 默认自动生成（`data.NewID()`），允许人工覆盖一次，之后锁定。
- 存为 point `assetCode`，调度/聚合/审计/权限/外部 API 一律引用它，展示名/树路径可改。

## 关系

```
site (1..N) plant (1..N) energyUnit (0..N) device (1..N) modbusIo(point)
```
`energyUnit` 是调度对象；`device` 是设备实例（Profile+assetCode+额定覆盖）；`modbusIo` 点位不变（挂在 device 下）。
