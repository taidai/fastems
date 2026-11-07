package appapi

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// 领域节点类型（easyems 层定义，纯数据节点，不注册 Simple IoT client）。
// Device 复用上游 data.NodeTypeDevice。
const (
	NodeTypeSite       = "site"
	NodeTypePlant      = "plant"
	NodeTypeEnergyUnit = "energyUnit"
)

// 四层节点字段的 point type。
const (
	PointTypeAssetCode              = "assetCode"
	PointTypeEnergyType             = "energyType"
	PointTypeRatedEnergyKWh         = "ratedEnergyKWh"
	PointTypeRatedPowerKW           = "ratedPowerKW"
	PointTypeCapability             = "capability"
	PointTypeRatedCapabilityOverride = "ratedCapabilityOverride"
	PointTypeProfileID              = "profileId"
	PointTypePointMap               = "pointMap"
	PointTypeSiteTarget             = "totalTarget"
	PointTypeSiteConstraint         = "totalConstraint"
	PointTypePlantTarget            = "plantTarget"
	PointTypePlantConstraint        = "plantConstraint"
)

// 能源类型（Q7=A：第一版只做储能，光伏/充电留类型定义）。
const (
	EnergyTypeStorage = "storage"
	EnergyTypePV      = "pv"
	EnergyTypeEV      = "ev"
)

// Capability 描述一个能源节点/设备可提供的调度能力。Q5=C 分层：
// Rated 额定来自 Profile 默认 + Device 实例覆盖；Dynamic 动态由运行时从
// 点位计算后写回。调度只读能力，不直接认设备型号。
type Capability struct {
	Kind        string  `json:"kind"`         // charge / discharge / energy / session
	Direction   string  `json:"direction"`    // charge / discharge / bidir
	PowerMinKW  float64 `json:"power_min_kw"`
	PowerMaxKW  float64 `json:"power_max_kw"`
	RampKWs     float64 `json:"ramp_kw_s"`
	Allowed     bool    `json:"allowed"`
	LimitReason string  `json:"limit_reason,omitempty"`
}

// EncodeCapability 序列化一份能力到节点 point。
func EncodeCapability(capability Capability) (string, error) {
	raw, err := json.Marshal(capability)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// DecodeCapability 从节点 point 读取一份能力；空串返回零值能力（不可用）。
func DecodeCapability(text string) (Capability, error) {
	if text == "" {
		return Capability{}, nil
	}
	var capability Capability
	if err := json.Unmarshal([]byte(text), &capability); err != nil {
		return Capability{}, err
	}
	return capability, nil
}

// resolveAssetCode 实现 Q4=C：默认自动生成，人工可覆盖一次，之后锁定。
// existing 是节点当前已存的 assetCode（空表示从未设置），requested 是本次
// 请求值（空表示交给系统生成）。
func resolveAssetCode(existing, requested string) (string, error) {
	if existing != "" {
		if requested != "" && requested != existing {
			return existing, fmt.Errorf("asset_code 已锁定为 %q，不可修改", existing)
		}
		return existing, nil
	}
	if requested != "" {
		return requested, nil
	}
	return data.NewID(), nil
}

// CreateDomainNode 创建纯数据领域节点（site/plant/energyUnit/device），
// 返回节点 ID。fields 是 point type → 文本值。节点无 client，只挂 point。
func CreateDomainNode(nc *nats.Conn, parent, nodeType string, fields map[string]string) (string, error) {
	if parent == "" {
		return "", fmt.Errorf("domain node requires a parent")
	}
	id := data.NewID()
	points := make(data.Points, 0, len(fields))
	for typ, value := range fields {
		points = append(points, data.NewPointString(typ, "", value))
	}
	node := data.NodeEdge{ID: id, Type: nodeType, Parent: parent, Points: points}
	if err := client.SendNode(nc, node, "easyems"); err != nil {
		return "", err
	}
	return id, nil
}
