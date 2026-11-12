package profile

import "fastiot/internal/ems/channel"

// catalog is the built-in vendor library. Every entry is marked unverified
// until the matching device has been read and, where applicable, written on
// site. Points are the minimal EMS-facing subset; full vendor tables remain in
// the protocol archives.
func catalog() []Profile {
	profiles := []Profile{
		enjoPCS(),
		esbcmBMS(),
		dtsdMeter(),
		yd2037Meter(),
		huaweiInverter(),
		jiwaMPPT(),
	}
	probes := map[string][]Identity{
		"enjo-pcs": {
			{Function: channel.HoldingRegister, Address: 24688, Count: 16, TextContains: []string{"ENJO", "PCS", "恩玖"}, Weight: 2, Label: "0x6070 模块 SN/型号"},
			{Function: channel.HoldingRegister, Address: 24576, Count: 3, Weight: 1, Label: "0x6000 DSP 版本"},
		},
		"esbcm-bms": {
			{Function: channel.InputRegister, Address: 200, Count: 3, Weight: 1, Label: "输入寄存器 200–202 协议版本"},
			{Function: channel.HoldingRegister, Address: 116, Count: 2, Weight: 1, Label: "保持寄存器 116–117 电池类型/容量"},
		},
		"dtsd3366d-meter": {
			{Function: channel.HoldingRegister, Address: 1, Count: 16, TextContains: []string{"DTSD", "TOKY"}, Weight: 3, Label: "0x0001 型号文本（FC03 回退探测）"},
		},
		"yd2037-meter": {
			{Function: channel.HoldingRegister, Address: 43904, Count: 16, TextContains: []string{"YD2037", "YADA", "雅达"}, Weight: 3, Label: "0xAB80 型号文本"},
		},
		"huawei-inverter": {
			{Function: channel.HoldingRegister, Address: 32080, Count: 2, Weight: 1, Label: "0x7D50 有功功率特征寄存器"},
		},
		"jiwa-mppt": {
			{Function: channel.HoldingRegister, Address: 45044, Count: 1, Weight: 1, Label: "45044 机器特征号"},
			{Function: channel.HoldingRegister, Address: 45054, Count: 2, Weight: 1, Label: "45054/45055 协议版本"},
		},
	}
	for i := range profiles {
		profiles[i].Probes = probes[profiles[i].ID]
	}
	return profiles
}

func makeProfile(id, vendor, model, protocol, verification, source, identityHint string, points []PointSpec, commands []CommandSpec, notes ...string) Profile {
	return Profile{
		ID: id, Vendor: vendor, Model: model, Protocol: protocol,
		Points: len(points), Commands: len(commands),
		Verified: false, Verification: verification, Source: source,
		IdentityHint: identityHint, Notes: notes,
		PointSpecs: points, CommandSpecs: commands,
	}
}

func point(name string, function channel.Function, address uint16, format string, scale, offset float64, unit string, readOnly bool, role, note string) PointSpec {
	return PointSpec{Name: name, Function: function, Address: address, Format: format, Scale: scale, Offset: offset, Unit: unit, ReadOnly: readOnly, Role: role, Note: note}
}

func enjoPCS() Profile {
	points := []PointSpec{
		point("开机/关机指令", channel.HoldingRegister, 657, "int16", 1, 0, "", false, RoleSetpoint, "写 1 开机、0 关机；现场先确认写授权"),
		point("有功功率设定", channel.HoldingRegister, 20494, "int16", 0.1, 0, "kW", false, RoleSetpoint, "0x500E；旧固件可能使用 0x0D57"),
		point("交流输出总有功功率", channel.HoldingRegister, 24633, "int16", 0.1, 0, "kW", true, RoleTelemetry, "放电为正、充电为负"),
		point("直流功率", channel.HoldingRegister, 24661, "int16", 0.1, 0, "kW", true, RoleTelemetry, ""),
		point("电池电压", channel.HoldingRegister, 24659, "int16", 0.1, 0, "V", true, RoleTelemetry, ""),
		point("电池电流", channel.HoldingRegister, 24660, "int16", 0.1, 0, "A", true, RoleTelemetry, ""),
		point("电网频率", channel.HoldingRegister, 24620, "int16", 0.01, 0, "Hz", true, RoleTelemetry, ""),
		point("IGBT 温度", channel.HoldingRegister, 24664, "int16", 0.1, 0, "℃", true, RoleTelemetry, ""),
		point("环境温度", channel.HoldingRegister, 24665, "int16", 0.1, 0, "℃", true, RoleTelemetry, ""),
		point("工作状态", channel.HoldingRegister, 24663, "uint16", 1, 0, "", true, RoleStatus, "位域，按协议 §3.1 解释"),
		point("硬件故障字 1", channel.HoldingRegister, 5888, "uint16", 1, 0, "", true, RoleStatus, "位域"),
		point("硬件故障字 2", channel.HoldingRegister, 5889, "uint16", 1, 0, "", true, RoleStatus, "位域"),
		point("电网故障字", channel.HoldingRegister, 5890, "uint16", 1, 0, "", true, RoleStatus, "位域"),
		point("系统故障字", channel.HoldingRegister, 5893, "uint16", 1, 0, "", true, RoleStatus, "位域"),
	}
	commands := []CommandSpec{
		{ID: "on", Name: "开机", Point: "开机/关机指令", Value: 1, Note: "写 1"},
		{ID: "off", Name: "关机", Point: "开机/关机指令", Value: 0, Note: "写 0"},
	}
	return makeProfile(
		"enjo-pcs", "恩玖", "PCS（V1.20/V1.27）", "modbus",
		"协议文档 + 台山站真机地址核对（2026-09-23）；未在本机实机复测",
		"《PCS通信协议（通用版本）》V1.27 + 台山站点位梳理",
		"自动识别读取 0x6070 模块 SN/型号文本与 0x6000 DSP 版本；未读到型号文本时作为候选人工确认。",
		points, commands,
		"V1.20/V1.27 的功率设定存在 0x500E 与旧 0x0D57 两套地址，导入后必须按现场固件核对。",
		"V1.27 文档不支持批量写；控制逐点 FC06，并先过安全层。",
	)
}

func esbcmBMS() Profile {
	points := []PointSpec{
		point("电池簇电压", channel.InputRegister, 1, "uint16", 0.1, 0, "V", true, RoleTelemetry, ""),
		point("电池簇电流", channel.InputRegister, 2, "uint16", 0.1, -1600, "A", true, RoleTelemetry, "充电为正、放电为负"),
		point("电池簇 SOC", channel.InputRegister, 3, "uint16", 0.1, 0, "%", true, RoleTelemetry, ""),
		point("电池簇 SOH", channel.InputRegister, 4, "uint16", 1, 0, "%", true, RoleTelemetry, ""),
		point("电池簇 SOE", channel.InputRegister, 5, "uint16", 0.1, 0, "%", true, RoleTelemetry, ""),
		point("正绝缘电阻", channel.InputRegister, 6, "uint16", 1, 0, "kΩ", true, RoleTelemetry, ""),
		point("负绝缘电阻", channel.InputRegister, 7, "uint16", 1, 0, "kΩ", true, RoleTelemetry, ""),
		point("最高温度", channel.InputRegister, 12, "uint16", 0.1, -40, "℃", true, RoleTelemetry, ""),
		point("最低温度", channel.InputRegister, 15, "uint16", 0.1, -40, "℃", true, RoleTelemetry, ""),
		point("平均温度", channel.InputRegister, 18, "uint16", 0.1, -40, "℃", true, RoleTelemetry, ""),
		point("最高单体电压", channel.InputRegister, 21, "uint16", 0.001, 0, "V", true, RoleTelemetry, ""),
		point("最低单体电压", channel.InputRegister, 24, "uint16", 0.001, 0, "V", true, RoleTelemetry, ""),
		point("系统心跳", channel.InputRegister, 71, "uint16", 1, 0, "", true, RoleStatus, "每秒加 1；停更即链路/主控异常"),
		point("系统告警状态", channel.InputRegister, 73, "uint16", 1, 0, "", true, RoleStatus, "bit0 禁充、bit1 禁放、bit2 告警、bit3 充满、bit4 放空"),
		point("系统故障码", channel.InputRegister, 74, "uint16", 1, 0, "", true, RoleStatus, "按协议 §3.6 解释"),
		point("累计充电电量", channel.InputRegister, 59, "uint32", 0.1, 0, "kWh", true, RoleEnergy, "两个连续寄存器，高字在前"),
		point("累计放电电量", channel.InputRegister, 61, "uint32", 0.1, 0, "kWh", true, RoleEnergy, "两个连续寄存器，高字在前"),
	}
	return makeProfile(
		"esbcm-bms", "高特", "ESBCM BMS（V1.18）", "modbus",
		"协议文档解析；未连接现场 BMS，未做任何写入",
		"《ESBCM-RS485&LAN 通用通信协议》V1.18 + 台山站解析档案",
		"自动识别读取输入寄存器 200–202 与保持寄存器 116–117 作为特征证据；未识别到型号文本时作为候选人工确认。",
		points, nil,
		"输入寄存器 77~96 与 8000~8899 禁止周期读取；FC04 单次不超过 120 个寄存器。",
		"LAN 连接建立后 5 秒内必须发一帧，空闲每 10 秒一帧，30 秒无应答断开。",
		"单体电压/温度/SOC 数组未放入精简 Profile，避免一次生成数百个点位；需要时从协议档案单独抽取。",
		"最大允许充/放功率寄存器 150~153 为预留且发送 0xFFFF，不作为可用能力。",
	)
}

func dtsdMeter() Profile {
	points := []PointSpec{
		point("正向有功总电能", channel.HoldingRegister, 266, "int32", 0.01, 0, "kWh", true, RoleEnergy, "0x10A，高字在前"),
		point("反向有功总电能", channel.HoldingRegister, 276, "int32", 0.01, 0, "kWh", true, RoleEnergy, "0x114，方向语义待现场核对"),
		point("瞬时总有功功率", channel.HoldingRegister, 378, "int32", 0.0001, 0, "kW", true, RoleTelemetry, "0x17A，负数为补码"),
		point("A 相电压", channel.HoldingRegister, 366, "uint32", 0.0001, 0, "V", true, RoleTelemetry, "0x16E"),
		point("B 相电压", channel.HoldingRegister, 368, "uint32", 0.0001, 0, "V", true, RoleTelemetry, "0x170"),
		point("C 相电压", channel.HoldingRegister, 370, "uint32", 0.0001, 0, "V", true, RoleTelemetry, "0x172"),
		point("A 相电流", channel.HoldingRegister, 372, "uint32", 0.0001, 0, "A", true, RoleTelemetry, "0x174"),
		point("B 相电流", channel.HoldingRegister, 374, "uint32", 0.0001, 0, "A", true, RoleTelemetry, "0x176"),
		point("C 相电流", channel.HoldingRegister, 376, "uint32", 0.0001, 0, "A", true, RoleTelemetry, "0x178"),
		point("频率", channel.HoldingRegister, 409, "uint16", 0.01, 0, "Hz", true, RoleTelemetry, "0x199"),
	}
	return makeProfile(
		"dtsd3366d-meter", "TOKY", "DTSD3366D-J P4 电表", "modbus",
		"协议文档解析；未实机读取",
		"《DTSD3366D 电表协议》V1.03.2 解析档案",
		"自动识别以 FC03 回退读 0x0001 型号文本；设备仅支持 FC53 时仍需人工选择并核对。",
		points, nil,
		"文档地址即帧地址（0-based），导入时不要加 4xxxx 前缀或 ±1。",
		"正向/反向与购电/上网的对应关系需结合 CT 极性与接线核对。",
	)
}

func yd2037Meter() Profile {
	points := []PointSpec{
		point("正向有功总电能", channel.HoldingRegister, 32778, "int32", 0.01, 0, "kWh", true, RoleEnergy, "0x800A，高字在前"),
		point("反向有功总电能", channel.HoldingRegister, 32788, "int32", 0.01, 0, "kWh", true, RoleEnergy, "0x8014，方向语义待现场核对"),
		point("有功总功率", channel.HoldingRegister, 36122, "int32", 0.0001, 0, "kW", true, RoleTelemetry, "0x8D1A，正=受电、负=馈电待核对"),
		point("A 相电压", channel.HoldingRegister, 36096, "uint32", 0.001, 0, "V", true, RoleTelemetry, "0x8D00"),
		point("B 相电压", channel.HoldingRegister, 36098, "uint32", 0.001, 0, "V", true, RoleTelemetry, "0x8D02"),
		point("C 相电压", channel.HoldingRegister, 36100, "uint32", 0.001, 0, "V", true, RoleTelemetry, "0x8D04"),
		point("A 相电流", channel.HoldingRegister, 36110, "uint32", 0.001, 0, "A", true, RoleTelemetry, "0x8D0E"),
		point("B 相电流", channel.HoldingRegister, 36112, "uint32", 0.001, 0, "A", true, RoleTelemetry, "0x8D10"),
		point("C 相电流", channel.HoldingRegister, 36114, "uint32", 0.001, 0, "A", true, RoleTelemetry, "0x8D12"),
		point("电网频率", channel.HoldingRegister, 36159, "uint16", 0.01, 0, "Hz", true, RoleTelemetry, "0x8D3F"),
	}
	return makeProfile(
		"yd2037-meter", "雅达", "YD2037Y-F 电表", "modbus",
		"协议文档解析；未实机读取",
		"《YD2037Y-F 多功能电能表 ModBus-RTU 协议》V1.09 解析档案",
		"自动识别读取 0xAB80 型号文本（16 寄存器 ASCII）；读到 YD2037/YADA 时自动匹配。",
		points, nil,
		"本 Profile 使用 B 区实时数据；一次值浮点区 G 区未纳入。",
		"若现场读数明显异常，先核对字序（MSW first）与 CT 变比。",
	)
}

func huaweiInverter() Profile {
	points := []PointSpec{
		point("有功功率", channel.HoldingRegister, 32080, "int32", 0.001, 0, "kW", true, RoleTelemetry, "0x7D50，高字在前"),
		point("直流功率", channel.HoldingRegister, 32064, "int32", 0.001, 0, "kW", true, RoleTelemetry, "0x7D40"),
		point("累计发电电量", channel.HoldingRegister, 32106, "uint32", 0.01, 0, "kWh", true, RoleEnergy, "0x7D6A"),
		point("设备状态", channel.HoldingRegister, 32089, "uint16", 1, 0, "", true, RoleStatus, "0x7D59"),
		point("故障码", channel.HoldingRegister, 32090, "uint16", 1, 0, "", true, RoleStatus, "0x7D5A"),
		point("内部温度", channel.HoldingRegister, 32087, "int16", 0.1, 0, "℃", true, RoleTelemetry, "CSV 标注 ℃ 10，定标待现场核对"),
		point("有功功率百分比控制", channel.HoldingRegister, 40199, "int16", 1, 0, "%", false, RoleSetpoint, "0x9D07，低精度，写入范围待现场核对"),
		point("远程功率调度", channel.HoldingRegister, 42014, "uint16", 1, 0, "", false, RoleSetpoint, "0xA41E，写入语义待现场核对"),
	}
	commands := []CommandSpec{
		{
			ID: "set-active-power-percent", Name: "有功功率百分比", Value: 0,
			Parameters: []CommandParameter{{
				Name: "percent", Label: "目标百分比", Point: "有功功率百分比控制",
				Unit: "%", Kind: "number", HasRange: true, Min: 0, Max: 100, Default: 0,
			}},
			Note: "范围与精度待现场核对",
		},
	}
	return makeProfile(
		"huawei-inverter", "华为", "光伏逆变器（50KTL / Modbus V3.0）", "modbus",
		"Neuron 精简点表与华为 Modbus V3.0 文档；未实机读写",
		"华为逆变器Modbus精简版.csv + 华为逆变器 Modbus V3.0",
		"自动识别读取 0x7D50 有功功率特征寄存器；未读到型号文本时作为候选人工确认。",
		points, commands,
		"有功功率百分比与远程功率调度的写入范围、极性必须先只读核对。",
		"温度、电压等部分点的 CSV 定标存在 1/10/100 倍歧义，导入后逐点核对量级。",
	)
}

func jiwaMPPT() Profile {
	points := []PointSpec{
		point("输入侧总功率", channel.HoldingRegister, 45217, "int16", 0.1, 0, "kW", true, RoleTelemetry, "0xB0A1，有符号"),
		point("母线电压", channel.HoldingRegister, 45221, "int16", 0.1, 0, "V", true, RoleTelemetry, "0xB0A5"),
		point("散热器温度", channel.HoldingRegister, 45282, "int16", 0.1, 0, "℃", true, RoleTelemetry, "0xB0E2"),
		point("状态字 1", channel.HoldingRegister, 45200, "uint16", 1, 0, "", true, RoleStatus, "位域"),
		point("告警字 1", channel.HoldingRegister, 45205, "uint16", 1, 0, "", true, RoleStatus, "位域"),
		point("故障字 1", channel.HoldingRegister, 45210, "uint16", 1, 0, "", true, RoleStatus, "位域"),
		point("协议版本高字", channel.HoldingRegister, 45054, "uint16", 1, 0, "", true, RoleStatus, "联调先核对 V2.5"),
		point("协议版本低字", channel.HoldingRegister, 45055, "uint16", 1, 0, "", true, RoleStatus, ""),
	}
	return makeProfile(
		"jiwa-mppt", "吉瓦时代", "MPPT（V2.5）", "modbus",
		"协议文档解析；未实机读取",
		"《吉瓦时代 MPPT 协议》V2.5.0.0 解析档案",
		"自动识别读取 45044 机器特征号与 45054/45055 协议版本；未识别到型号文本时作为候选人工确认。",
		points, nil,
		"地址为保持寄存器；RTU 9600-8N1，从站地址可为固定或 SN 推导。",
		"限功率写点未纳入精简 Profile，待现场确认写授权后补充。",
	)
}
