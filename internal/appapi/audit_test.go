package appapi

import (
	"net/http/httptest"
	"testing"
)

func TestAuditBusinessLabels(t *testing.T) {
	cases := []struct {
		method, path, action, object string
	}{
		{method: "POST", path: "/api/modbus/ch1/units/1/commands", action: "命令执行", object: "设备 ch1/1 命令"},
		{method: "PUT", path: "/api/modbus/ch1/units/1/model", action: "设备模型", object: "设备 ch1/1 模型"},
		{method: "POST", path: "/api/modbus/ch1/io/import", action: "点表配置", object: "通道 ch1 点表"},
		{method: "POST", path: "/api/ems/channels", action: "端口配置", object: "通道配置"},
		{method: "POST", path: "/api/ems/alarms/a1/ack", action: "报警处理", object: "报警 a1 确认"},
		{method: "POST", path: "/api/rules/r1/actions/publish", action: "决策表", object: "规则 r1 发布"},
		{method: "PUT", path: "/api/tariff", action: "站点设置", object: "站点电价"},
		{method: "POST", path: "/api/nodes/n1/value", action: "控制写入", object: "点位 n1"},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		if got := auditAction(request); got != tc.action {
			t.Errorf("%s action = %q, want %q", tc.path, got, tc.action)
		}
		if got := auditObject(request); got != tc.object {
			t.Errorf("%s object = %q, want %q", tc.path, got, tc.object)
		}
	}
}
