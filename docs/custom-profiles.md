# 自定义设备 Profile

日期：2026-11-07

内置 Profile 目录覆盖不了的设备型号，不需要改代码：把一个 JSON 文件放进数据目录的
`profiles/` 子目录（例如 `data/profiles/acme-pump.json`），重启后它会出现在
“设备 Profile”下拉中，标注“（自定义）”。

## 规则

- 一个文件一个 Profile，文件名任意（`.json` 结尾），按文件名顺序加载。
- `id` 与内置 Profile 相同则**覆盖内置条目**：现场可以修正内置点表而不改代码。
- 自定义 Profile 没有自动识别探针，只能在绑定时手工选择。
- 文件无效时启动日志会说明原因，设备模型页也会显示哪个文件没有加载；无效文件不会让程序启动失败。
- `verified` 与 `verification` 由维护者自己负责：描述清楚验证依据（协议文档版本、现场核对日期）。

## 字段

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `id` | 是 | 唯一 ID，不含空格和斜杠 |
| `vendor` / `model` | 是 | 厂商与型号 |
| `protocol` | 否 | 目前只支持 `modbus`（默认） |
| `point_specs[].name` | 是 | 点位名称，Profile 内唯一；绑定时按此名称映射 |
| `point_specs[].function` | 否 | `coil` / `discreteInput` / `holdingRegister`（默认） / `inputRegister` |
| `point_specs[].address` | 是 | 寄存器地址（0 基，协议帧地址） |
| `point_specs[].format` | 否 | `uint16`（默认） / `int16` / `uint32` / `int32` / `float32` |
| `point_specs[].scale` / `offset` | 否 | 换算：显示值 = 原始值 × scale + offset（scale 省略为 1） |
| `point_specs[].unit` | 否 | 显示单位 |
| `point_specs[].read_only` | 否 | 只读点为 `true` |
| `point_specs[].role` | 否 | `telemetry` 遥测 / `status` 遥信 / `energy` 电度 / `setpoint` 定值 / `protection` 压板 |
| `point_specs[].note` | 否 | 现场提示（字序、极性、待核对事项） |
| `command_specs[].id` / `name` | 是 | 命令 ID 与显示名 |
| `command_specs[].point` | 否 | 写入的点位名称，必须在 `point_specs` 中声明 |
| `command_specs[].value` | 否 | 固定命令写入值 |
| `command_specs[].parameters[]` | 否 | 操作员输入参数：`name`、`label`、`point`（必须已声明）、`kind`（`number`/`bool`）、`has_range`+`min`+`max`、`default` |

角色（`role`）是声明，不是猜测：设备详情与绑定界面按它分组着色。
没有绑定 Profile 的原始点位，全局点位表仍按名称/类型推断角色。

示例见 [`examples/custom-profile-example.json`](examples/custom-profile-example.json)。
