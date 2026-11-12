package decision

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/alloc"
)

const allocatorPointType = "easyemsAllocator"

// AllocatorConfig lives on a business node. A rule binds that node, not a copy
// of the member list, so adding a cabinet does not change the rule.
type AllocatorConfig struct {
	GraceMS   int               `json:"grace_ms,omitempty"`
	TimeoutMS int               `json:"timeout_ms,omitempty"`
	// DeadbandKW suppresses re-dispatch when the next request is within this
	// many kW of the last one, so small fluctuations do not rewrite every PCS.
	DeadbandKW float64           `json:"deadband_kw,omitempty"`
	Members    []AllocatorMember `json:"members"`
}

// AllocatorMember is one PCS power target and its current capability sources.
type AllocatorMember struct {
	ID                 string  `json:"id"`
	Target             Target  `json:"target"`
	MinPower           float64 `json:"min_power,omitempty"`
	MaxCharge          float64 `json:"max_charge"`
	MaxDischarge       float64 `json:"max_discharge"`
	AvailableCharge    *Target `json:"available_charge,omitempty"`
	AvailableDischarge *Target `json:"available_discharge,omitempty"`
	Readback           *Target `json:"readback,omitempty"`
	Fault              *Target `json:"fault,omitempty"`
	// Priority fills higher-priority members to capacity before lower ones.
	Priority int `json:"priority,omitempty"`
	// Weight biases the proportional share (1 = default). It never raises a
	// member's hard capacity.
	Weight float64 `json:"weight,omitempty"`
	// Constraints derate or exclude this member when an input violates a limit.
	Constraints []MemberConstraint `json:"constraints,omitempty"`
}

func (c AllocatorConfig) grace() time.Duration {
	if c.GraceMS <= 0 {
		return 2 * time.Second
	}
	return time.Duration(c.GraceMS) * time.Millisecond
}

func (c AllocatorConfig) timeout() time.Duration {
	if c.TimeoutMS <= 0 {
		return 3 * time.Second
	}
	return time.Duration(c.TimeoutMS) * time.Millisecond
}

// LoadAllocator reads the current configuration from a business node.
func LoadAllocator(nc *nats.Conn, nodeID string) (AllocatorConfig, error) {
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return AllocatorConfig{}, fmt.Errorf("allocator unavailable")
	}
	point, found := nodes[0].Points.Find(allocatorPointType, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return AllocatorConfig{}, fmt.Errorf("allocator is not configured")
	}
	var config AllocatorConfig
	if err := json.Unmarshal([]byte(point.Txt()), &config); err != nil {
		return AllocatorConfig{}, fmt.Errorf("invalid allocator configuration")
	}
	if err := config.Validate(); err != nil {
		return AllocatorConfig{}, err
	}
	return config, nil
}

// Validate rejects ambiguous or incomplete configurations before they can run.
func (c AllocatorConfig) Validate() error {
	if len(c.Members) == 0 {
		return fmt.Errorf("allocator has no members")
	}
	if c.GraceMS < 0 || c.TimeoutMS < 0 {
		return fmt.Errorf("allocator timing must not be negative")
	}
	if !finite(c.DeadbandKW) || c.DeadbandKW < 0 {
		return fmt.Errorf("allocator deadband must be a non-negative number")
	}
	ids := map[string]bool{}
	targets := map[string]bool{}
	for _, member := range c.Members {
		if member.ID == "" {
			return fmt.Errorf("allocator member has no id")
		}
		if ids[member.ID] {
			return fmt.Errorf("duplicate allocator member %s", member.ID)
		}
		ids[member.ID] = true
		if member.Target.NodeID == "" {
			return fmt.Errorf("member %s has no power target", member.ID)
		}
		if targets[member.Target.NodeID] {
			return fmt.Errorf("member %s shares a power target with another member", member.ID)
		}
		targets[member.Target.NodeID] = true
		if member.MinPower < 0 || member.MaxCharge < 0 || member.MaxDischarge < 0 {
			return fmt.Errorf("member %s has negative power", member.ID)
		}
		if !finite(member.Weight) || member.Weight < 0 {
			return fmt.Errorf("member %s has an invalid weight", member.ID)
		}
		for name, target := range map[string]*Target{
			"available charge":    member.AvailableCharge,
			"available discharge": member.AvailableDischarge,
			"readback":            member.Readback,
			"fault":               member.Fault,
		} {
			if target != nil && target.NodeID == "" {
				return fmt.Errorf("member %s has no %s node", member.ID, name)
			}
		}
		for _, constraint := range member.Constraints {
			if err := constraint.validate(); err != nil {
				return fmt.Errorf("member %s: %w", member.ID, err)
			}
		}
	}
	return nil
}

// ValidateAllocatorScope checks every output and capability input against the
// allocator node's subtree. A member outside that subtree is rejected.
func ValidateAllocatorScope(nc *nats.Conn, parent string, config AllocatorConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	for _, member := range config.Members {
		if err := ValidateTarget(nc, parent, member.Target, true, nil); err != nil {
			return fmt.Errorf("member %s: %w", member.ID, err)
		}
		for _, target := range []*Target{member.AvailableCharge, member.AvailableDischarge, member.Fault, member.Readback} {
			if target != nil {
				if err := ValidateTarget(nc, parent, *target, false, nil); err != nil {
					return fmt.Errorf("member %s capability: %w", member.ID, err)
				}
			}
		}
		for _, constraint := range member.Constraints {
			if err := ValidateTarget(nc, parent, constraint.Target, false, nil); err != nil {
				return fmt.Errorf("member %s constraint %s: %w", member.ID, constraint.Kind, err)
			}
		}
	}
	return nil
}

// ValidateAllocationOwnership prevents two enabled tables from owning the same
// allocator. Direct writes to its members are handled as manual takeover.
func ValidateAllocationOwnership(nc *nats.Conn, ruleID, nodeID string) error {
	tables, err := siotclient.GetNodesType[DecisionTable](nc, "all", "all")
	if err != nil {
		return fmt.Errorf("cannot verify allocator ownership")
	}
	for _, table := range tables {
		if table.ID == ruleID || !table.Enabled || table.Applied == "" {
			continue
		}
		var document Document
		if json.Unmarshal([]byte(table.Applied), &document) != nil {
			continue
		}
		for _, row := range document.Table.Rows {
			for _, action := range row.Actions {
				target := document.Bindings.Actions[action.ActionID]
				if target.NodeID == nodeID {
					return fmt.Errorf("allocator is already owned by %s", table.ID)
				}
			}
		}
	}
	return nil
}

// allocationMember resolves online state, manual takeover, current
// capabilities and constraint limits. The values are read again on every
// dispatch cycle. The returned reasons explain why a member was taken over or
// constrained; they are empty when the member is healthy.
func allocationMember(nc *nats.Conn, member AllocatorMember) (alloc.Member, []string, error) {
	result := alloc.Member{
		ID: member.ID, Online: true,
		MinPower: member.MinPower, MaxCharge: member.MaxCharge, MaxDischarge: member.MaxDischarge,
		AvailableCharge: member.MaxCharge, AvailableDischarge: member.MaxDischarge,
	}
	var reasons []string
	nodes, err := siotclient.GetNodes(nc, "all", member.Target.NodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return result, nil, fmt.Errorf("member %s target unavailable", member.ID)
	}
	node := nodes[0]
	if disabled, _ := node.Points.Value(data.PointTypeDisabled, ""); disabled != 0 {
		result.Disabled = true
	}
	if manual, _ := node.Points.Value("easyemsManual", ""); manual != 0 {
		result.Disabled = true
		reasons = append(reasons, "manual takeover")
	}
	if health, _ := node.Points.Text("health", ""); health != "" {
		result.Online = health != "offline"
	} else if node.Type == data.NodeTypeModbusIO {
		// No heartbeat is an offline member, not a reason to block every
		// other member. Healthy members still wait for capability values.
		result.Online = busHealthy(nc, node.Parent)
	}
	if member.Fault != nil {
		fault, found, err := readNumber(nc, *member.Fault)
		if err != nil {
			return result, nil, err
		}
		if !found || fault != 0 {
			result.Faulted = true
		}
	}
	if member.AvailableDischarge != nil {
		value, found, err := readNumber(nc, *member.AvailableDischarge)
		if err != nil {
			return result, nil, err
		}
		if !found {
			if result.Online && !result.Faulted {
				result.NotReady = true
			}
			result.Faulted = true
			result.AvailableDischarge = 0
		} else {
			result.AvailableDischarge = value
		}
	}
	if member.AvailableCharge != nil {
		value, found, err := readNumber(nc, *member.AvailableCharge)
		if err != nil {
			return result, nil, err
		}
		if !found {
			if result.Online && !result.Faulted {
				result.NotReady = true
			}
			result.Faulted = true
			result.AvailableCharge = 0
		} else {
			result.AvailableCharge = value
		}
	}
	for _, violation := range EvaluateConstraints(member.Constraints, func(target Target) (float64, bool) {
		value, found, err := readNumber(nc, target)
		if err != nil {
			return 0, false
		}
		return value, found
	}) {
		reasons = append(reasons, violation.Reason)
		switch violation.Direction {
		case "charge":
			result.AvailableCharge = 0
		case "discharge":
			result.AvailableDischarge = 0
		case "both":
			result.Disabled = true
		}
	}
	return result, reasons, nil
}

func busHealthy(nc *nats.Conn, busID string) bool {
	nodes, err := siotclient.GetNodes(nc, "all", busID, "", false)
	if err != nil || len(nodes) == 0 {
		return false
	}
	heartbeat, ok := nodes[0].Points.Find("lastGoodPoll", "")
	if !ok || heartbeat.Time.IsZero() {
		return false
	}
	poll, _ := nodes[0].Points.Value(data.PointTypePollPeriod, "")
	timeout, _ := nodes[0].Points.Value(data.PointTypeTimeout, "")
	maxAge := 5 * time.Second
	if value := 5 * time.Duration(poll) * time.Millisecond; value > maxAge {
		maxAge = value
	}
	if value := 3 * time.Duration(timeout) * time.Millisecond; value > maxAge {
		maxAge = value
	}
	age := time.Since(heartbeat.Time)
	return age <= maxAge && age >= -5*time.Second
}

func readNumber(nc *nats.Conn, target Target) (float64, bool, error) {
	nodes, err := siotclient.GetNodes(nc, "all", target.NodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, false, fmt.Errorf("capability input unavailable")
	}
	if nodes[0].Type == data.NodeTypeModbusIO {
		if health, _ := nodes[0].Points.Text("health", ""); health == "offline" {
			return 0, false, nil
		} else if health == "" && !busHealthy(nc, nodes[0].Parent) {
			return 0, false, nil
		}
	}
	pointType := target.PointType
	if pointType == "" {
		pointType = data.PointTypeValue
	}
	point, found := nodes[0].Points.Find(pointType, target.Key)
	if !found || point.Tombstone%2 != 0 {
		return 0, false, nil
	}
	value, err := point.ValueFloat()
	if err != nil {
		return 0, false, nil
	}
	return value, true, nil
}
