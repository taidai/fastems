package appapi

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/command"
	"fastiot/internal/ems/signal"
)

// 功率命令执行：Modbus 点位写入统一交给共享 command.Manager（与命名命令同一
// 生命周期），并把请求与 dispatchID 关联。没有运行时可用的设备回读时，
// 结果停留在 submitted，绝不冒充设备已确认。

// dispatchTrace 是一次写设备的关联信息。
type dispatchTrace struct {
	ID     string
	Source string
	Actor  string
}

// commands 返回共享命令管理器；未接入运行时（纯节点测试）时返回 nil。
func (h *Handler) commands() *command.Manager {
	if h.ems == nil || h.ems.Service == nil {
		return nil
	}
	return h.ems.Service.Commands()
}

// submitPointCommand 通过统一命令管理器写一个点位。返回 false 表示当前
// 实例没有命令运行时，调用方应回退到 valueSet 路径。
// 设备 ID 规则与 bridge 一致：<通道节点>/<站号>，点位 ID 就是 IO 节点 ID。
func (h *Handler) submitPointCommand(trace dispatchTrace, ioNodeID string, setpoint float64) (bool, error) {
	manager := h.commands()
	if manager == nil {
		return false, nil
	}
	nodes, err := siotclient.GetNodes(h.nc, "all", ioNodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return true, fmt.Errorf("power point %s not found", ioNodeID)
	}
	node := nodes[0]
	server := 0
	if point, found := node.Points.Find(data.PointTypeID, ""); found {
		server = int(pointFloat(point))
	}
	if node.Parent == "" || server < 1 || server > 247 {
		return true, fmt.Errorf("power point %s is not a Modbus IO with a valid station", ioNodeID)
	}
	deviceID := node.Parent + "/" + strconv.Itoa(server)
	// 没有真实运行通道时不提交命令，避免制造失败告警；由调用方走设定值路径。
	if h.ems == nil || h.ems.Service == nil || !h.ems.Service.HasDevice(deviceID) {
		return false, nil
	}
	outcome, err := manager.Submit(time.Now(), command.Request{
		ID: trace.ID + "-" + ioNodeID, DeviceID: deviceID, Point: ioNodeID,
		Value: signal.Num(setpoint), NoReadback: true,
		DispatchID: trace.ID, Source: trace.Source, Stage: "submitted", Actor: trace.Actor,
	})
	if err != nil {
		// 没有运行时通道（例如仅配置了点位而无实际总线）时退回设定值路径，
		// 由调用方如实标注 stage=valueSet_written。
		if strings.Contains(err.Error(), "unknown device") {
			return false, nil
		}
		return true, err
	}
	if outcome.State == command.Failed {
		if strings.Contains(outcome.Reason, "unknown device") {
			return false, nil
		}
		return true, fmt.Errorf("command rejected: %s", outcome.Reason)
	}
	return true, nil
}
