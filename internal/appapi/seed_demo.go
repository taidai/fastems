package appapi

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// SeedDemoSite 在 store 完全就绪后预置演示站点，带重试。必须在 app 启动
// 流程的 ready 之后调用：若在 store 早期（root meta 未更新）创建节点，
// StoreSyncAlways 模式下节点会卡在 replica boundary，进而扰动 store 状态。
func SeedDemoSite(nc *nats.Conn) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := seedDemoSite(nc); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

// seedDemoSite 在首次初始化时预置一个完整演示站点，让能力级调度闭环
// 开箱即可见：站点→电站→储能柜→PCS，一条已发布的能力级规则，EnergyScheduler
// 每秒求值并写 PCS 设定值。所有节点 asset_code 前缀 "DEMO"，description
// 标注"演示"，不冒充真实设备；用户可整体删除。
func seedDemoSite(nc *nats.Conn) error {
	// 已有演示站点则不重复创建（Bootstrap 幂等兜底）。
	existing, _ := siotclient.GetNodes(nc, "all", "all", NodeTypeSite, false)
	for _, node := range existing {
		if point, ok := node.Points.Find(PointTypeAssetCode, ""); ok && point.Txt() == "DEMO" {
			return nil
		}
	}

	// root 节点的 ID 不是 "root" 字符串（"root" 只是 root 节点的 parent）；
	// 用真实 root ID 作 parent，否则节点成为孤立节点、boundary 混乱。
	rootNode, err := siotclient.GetRootNode(nc)
	if err != nil {
		return err
	}
	rootID := rootNode.ID

	// Modbus 通道 + 功率设定点位（bridge 消费 valueSet 的落点）。
	channelID, err := CreateDomainNode(nc, rootID, data.NodeTypeModbus, map[string]string{
		"description": "演示总线（无真实设备）",
	})
	if err != nil {
		return err
	}
	powerPointID, err := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{
		"description": "有功功率设定", "id": "1", "address": "10",
	})
	if err != nil {
		return err
	}

	// 领域节点：Site → Plant → EnergyUnit → Device。
	siteID, err := CreateDomainNode(nc, rootID, NodeTypeSite, map[string]string{
		PointTypeAssetCode: "DEMO", "description": "演示站（可删除）",
	})
	if err != nil {
		return err
	}
	plantID, err := CreateDomainNode(nc, siteID, NodeTypePlant, map[string]string{
		PointTypeAssetCode: "DEMO-PLANT01", "description": "演示电站",
	})
	if err != nil {
		return err
	}
	energyUnitID, err := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode:    "DEMO-CAB01",
		PointTypeEnergyType:   EnergyTypeStorage,
		PointTypeRatedPowerKW: "250",
		"description":         "1 号储能柜（演示）",
	})
	if err != nil {
		return err
	}
	if _, err := CreateDomainNode(nc, energyUnitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode:    "DEMO-CAB01-PCS01",
		PointTypeRatedPowerKW: "100",
		PointTypePowerSetpoint: powerPointID,
		"description":         "1 号 PCS（演示）",
	}); err != nil {
		return err
	}

	// EnergyUnit 的运行时能力：可用 200 kW。
	capabilityText, err := EncodeCapability(Capability{Kind: "discharge", Direction: "discharge", PowerMaxKW: 200, Allowed: true})
	if err != nil {
		return err
	}
	capPoint := data.NewPointString(PointTypeCapability, "", capabilityText)
	capPoint.Origin = "easyems-seed"
	if err := siotclient.SendNodePoint(nc, energyUnitID, capPoint, true); err != nil {
		return err
	}

	// 一条已发布的能力级规则：额定 > 100 时请求放电 150 kW。
	rule := decision.CapabilityRule{
		ID:    "demo-discharge",
		State: decision.RulePublished,
		Conditions: []decision.CapabilityCondition{
			{Ref: decision.CapabilityRef{EnergyUnitID: energyUnitID, Field: decision.FieldRatedPower}, Operator: "gt", Value: 100},
		},
		Actions: []decision.CapabilityAction{{EnergyUnitID: energyUnitID, RequestKW: 150}},
	}
	ruleText, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	rulePoint := data.NewPointString(capabilityRulePoint, "", string(ruleText))
	rulePoint.Origin = "easyems-seed"
	return siotclient.SendNodePoint(nc, energyUnitID, rulePoint, true)
}
