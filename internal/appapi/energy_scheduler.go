package appapi

import (
	"log"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"

	"easyems/internal/decision"
)

// EnergyScheduler 是全局能力级调度器：每个周期扫描所有 EnergyUnit 节点，
// 对配置了能力级规则的单元自动执行能力级调度（读能力→求值→拆解→写设备）。
// 这是能力级模型替代点位级 decision client 的运行时自动循环。
type EnergyScheduler struct {
	nc            *nats.Conn
	dispatch      func(energyUnitID string) error
	plantDispatch func(plantID string) (bool, error)
	stop          chan struct{}
}

// NewEnergyScheduler 返回一个周期调度器，dispatch 是单次调度的执行函数。
func NewEnergyScheduler(nc *nats.Conn, dispatch func(energyUnitID string) error) *EnergyScheduler {
	return &EnergyScheduler{nc: nc, dispatch: dispatch, stop: make(chan struct{})}
}

// SetPlantDispatch 注入站级策略（防逆流/需量）的执行函数，每周期对有 PCC
// 绑定的 Plant 执行。
func (s *EnergyScheduler) SetPlantDispatch(dispatch func(plantID string) (bool, error)) {
	s.plantDispatch = dispatch
}

// Run 每 1 秒扫描一次 EnergyUnit 并执行调度，直到 Stop。
func (s *EnergyScheduler) Run() error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return nil
		case <-ticker.C:
			s.tick()
		}
	}
}

// Stop 结束调度循环。
func (s *EnergyScheduler) Stop(error) {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

func (s *EnergyScheduler) tick() {
	// 动态能力：先按 SOC/温度刷新 capability，再调度。
	RefreshCapabilities(s.nc)

	controlled := map[string]bool{}

	// 站级策略优先：防逆流 > 需量 > 削峰填谷；接管时该站单柜规则本轮让位。
	if s.plantDispatch != nil {
		plants, err := siotclient.GetNodes(s.nc, "all", "all", NodeTypePlant, false)
		if err == nil {
			for _, plant := range plants {
				if _, ok := loadPCCFeedback(plant); !ok {
					if _, hasStrategy := loadPlantStrategy(plant); !hasStrategy {
						continue
					}
				}
				acted, err := s.plantDispatch(plant.ID)
				if err != nil {
					log.Printf("plant strategy %s: %v", plant.ID, err)
				}
				if acted {
					controlled[plant.ID] = true
				}
			}
		}
	}

	nodes, err := siotclient.GetNodes(s.nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return
	}
	for _, node := range nodes {
		if controlled[node.Parent] {
			continue
		}
		rule, ok := loadCapabilityRule(node)
		if !ok || rule.State != decision.RulePublished {
			continue
		}
		// CTL-01 互锁闸门：故障/检修/停机等非运行态禁止自动调度。
		if !decision.AllowsAutoDispatch(LoadCabinetState(node)) {
			continue
		}
		if err := s.dispatch(node.ID); err != nil {
			log.Printf("energy dispatch %s: %v", node.ID, err)
		}
	}
}
