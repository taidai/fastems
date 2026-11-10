package appapi

import (
	"log"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
)

// EnergyScheduler 每 1 秒刷新能力并对每个 EnergyUnit 调用一次统一仲裁入口。
// Plant/Site/Group 目标由仲裁入口按所属关系收集，调度器不再另跑站级写入。
type EnergyScheduler struct {
	nc       *nats.Conn
	dispatch func(energyUnitID string) error
	stop     chan struct{}
}

// NewEnergyScheduler 返回一个周期调度器，dispatch 是单次调度的执行函数。
func NewEnergyScheduler(nc *nats.Conn, dispatch func(energyUnitID string) error) *EnergyScheduler {
	return &EnergyScheduler{nc: nc, dispatch: dispatch, stop: make(chan struct{})}
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
	RefreshCapabilities(s.nc)
	nodes, err := siotclient.GetNodes(s.nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return
	}
	for _, node := range nodes {
		if !AllowsEnergyUnitAutoDispatch(node) {
			continue
		}
		if err := s.dispatch(node.ID); err != nil {
			log.Printf("energy dispatch %s: %v", node.ID, err)
		}
	}
}
