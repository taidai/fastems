package appapi

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/decision"
)

// PCC 反馈绑定的运行时读取：从 Plant 节点读绑定，再读主表点位的当前值，
// 合成 decision.PCCReading 供防逆流/需量闭环使用。

// loadPCCFeedback 读一个 Plant 节点上的 PCC 反馈绑定。
func loadPCCFeedback(node data.NodeEdge) (decision.PCCFeedback, bool) {
	point, found := node.Points.Find(PointTypePCCFeedback, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return decision.PCCFeedback{}, false
	}
	var binding decision.PCCFeedback
	if json.Unmarshal([]byte(point.Txt()), &binding) != nil {
		return decision.PCCFeedback{}, false
	}
	return binding, true
}

// ReadPlantPCC 读一个 Plant 的 PCC 实测：绑定在主表点位上，取最新 value 点，
// 用点位时间计算数据年龄。无绑定或无点位返回 ok=false（该 Plant 无 PCC 闭环）。
func ReadPlantPCC(nc *nats.Conn, plantID string) (decision.PCCFeedback, decision.PCCReading, bool) {
	nodes, err := siotclient.GetNodes(nc, "all", plantID, "", false)
	if err != nil || len(nodes) == 0 {
		return decision.PCCFeedback{}, decision.PCCReading{}, false
	}
	binding, ok := loadPCCFeedback(nodes[0])
	if !ok {
		return decision.PCCFeedback{}, decision.PCCReading{}, false
	}

	// 读主表点位的当前值。
	meterNodes, err := siotclient.GetNodes(nc, "all", binding.MeterPointID, "", false)
	if err != nil || len(meterNodes) == 0 {
		return binding, decision.PCCReading{Usable: false, Reason: "主表点位不存在"}, true
	}
	valuePoint, found := meterNodes[0].Points.Find(data.PointTypeValue, "")
	if !found {
		return binding, decision.PCCReading{Usable: false, Reason: "主表无读数"}, true
	}
	ageMS := time.Since(valuePoint.Time).Milliseconds()
	reading := decision.ReadPCC(binding, pointFloat(valuePoint), ageMS, true)
	return binding, reading, true
}

// savePCCFeedback 把 PCC 反馈绑定写到 Plant 节点。
func savePCCFeedback(nc *nats.Conn, plantID string, binding decision.PCCFeedback) error {
	encoded, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	point := data.NewPointString(PointTypePCCFeedback, "", string(encoded))
	point.Origin = "easyems"
	return siotclient.SendNodePoint(nc, plantID, point, true)
}
