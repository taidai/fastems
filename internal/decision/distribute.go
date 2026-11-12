package decision

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/nats-io/nats.go"

	"fastiot/internal/ems/alloc"
)

// resolvedMember is one member after reading its current state, plus the
// allocation knobs (priority, weight, constraint/takeover reasons) that the
// alloc package does not model.
type resolvedMember struct {
	member   alloc.Member
	priority int
	weight   float64
	reasons  []string
}

func (m resolvedMember) weightFactor() float64 {
	if m.weight <= 0 {
		return 1
	}
	return m.weight
}

// memberCapacity mirrors alloc.Member.capacity so the decision layer can run
// priority/weight distribution without changing the alloc package.
func memberCapacity(m alloc.Member, direction float64) float64 {
	available, limit := m.AvailableDischarge, m.MaxDischarge
	if direction < 0 {
		available, limit = m.AvailableCharge, m.MaxCharge
	}
	if math.IsNaN(available) || math.IsInf(available, 0) || math.IsNaN(limit) || math.IsInf(limit, 0) {
		return 0
	}
	capacity := math.Min(available, limit)
	if capacity < 0 {
		return 0
	}
	return capacity
}

// firstReason picks the most specific explanation for an excluded member.
func firstReason(reasons []string, fallback string) string {
	if len(reasons) > 0 {
		return strings.Join(reasons, "; ")
	}
	return fallback
}

// distribute splits a signed request across resolved members: unavailable
// members are excluded first, then the request is filled by descending
// priority, then proportionally to weighted capability within one priority
// level, honoring minimum power. It is pure — no nats, no state.
func distribute(request float64, members []resolvedMember) alloc.Result {
	result := alloc.Result{Requested: request, Shares: make([]alloc.Share, 0, len(members))}
	direction := 1.0
	if request < 0 {
		direction = -1.0
	}
	target := math.Abs(request)

	candidates := make([]resolvedMember, 0, len(members))
	shareRefs := make(map[string]*alloc.Share, len(members))
	for _, m := range members {
		share := alloc.Share{MemberID: m.member.ID}
		switch {
		case m.member.Disabled:
			share.Excluded, share.Reason = true, firstReason(m.reasons, "disabled")
		case m.member.Faulted:
			share.Excluded, share.Reason = true, firstReason(m.reasons, "faulted")
		case !m.member.Online:
			share.Excluded, share.Reason = true, firstReason(m.reasons, "offline")
		default:
			share.Capacity = memberCapacity(m.member, direction)
			if share.Capacity <= 0 {
				share.Excluded, share.Reason = true, firstReason(m.reasons, "no available capacity in this direction")
			} else {
				candidates = append(candidates, m)
			}
		}
		result.Shares = append(result.Shares, share)
		shareRefs[m.member.ID] = &result.Shares[len(result.Shares)-1]
	}

	allocations := fillByPriority(target, direction, candidates, shareRefs)
	delivered := 0.0
	for i := range result.Shares {
		share := &result.Shares[i]
		if value, ok := allocations[share.MemberID]; ok {
			share.Setpoint = direction * value
			delivered += value
		}
	}
	result.Delivered = direction * delivered
	if target-delivered > 1e-9 {
		result.Shortfall = direction * (target - delivered)
		result.Notes = append(result.Notes, fmt.Sprintf(
			"physical capability is %.1f kW short of the %.1f kW request", target-delivered, target))
	}
	return result
}

// fillByPriority fills higher-priority members to capacity before lower ones.
// Within one priority level the request is split proportionally to weighted
// capability.
func fillByPriority(target, direction float64, candidates []resolvedMember, shares map[string]*alloc.Share) map[string]float64 {
	groups := make(map[int][]resolvedMember)
	for _, m := range candidates {
		groups[m.priority] = append(groups[m.priority], m)
	}
	priorities := make([]int, 0, len(groups))
	for p := range groups {
		priorities = append(priorities, p)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	allocations := make(map[string]float64)
	remaining := target
	for _, p := range priorities {
		if remaining <= 1e-9 {
			break
		}
		group := groups[p]
		groupCap := 0.0
		for _, m := range group {
			groupCap += memberCapacity(m.member, direction)
		}
		if remaining >= groupCap {
			for _, m := range group {
				allocations[m.member.ID] = memberCapacity(m.member, direction)
			}
			remaining -= groupCap
			continue
		}
		fillProportional(remaining, direction, group, allocations, shares)
		return allocations
	}
	return allocations
}

// fillProportional distributes target across one priority group by weighted
// capability. A member whose weighted share would exceed its hard capacity is
// frozen at capacity and the remainder re-split; a share below MinPower is
// excluded.
func fillProportional(target, direction float64, group []resolvedMember, allocations map[string]float64, shares map[string]*alloc.Share) {
	active := append([]resolvedMember(nil), group...)
	remaining := target
	for len(active) > 0 && remaining > 1e-9 {
		sum := 0.0
		for _, m := range active {
			sum += memberCapacity(m.member, direction) * m.weightFactor()
		}
		if sum <= 0 {
			return
		}
		next := make([]resolvedMember, 0, len(active))
		changed := false
		for _, m := range active {
			capacity := memberCapacity(m.member, direction)
			value := remaining * capacity * m.weightFactor() / sum
			if value >= capacity-1e-9 {
				allocations[m.member.ID] = capacity
				remaining -= capacity
				changed = true
				continue
			}
			if m.member.MinPower > 0 && value < m.member.MinPower {
				if share := shares[m.member.ID]; share != nil {
					share.Excluded = true
					share.Reason = fmt.Sprintf("share %.1f kW is below the %.1f kW minimum", value, m.member.MinPower)
				}
				changed = true
				continue
			}
			next = append(next, m)
		}
		if !changed {
			for _, m := range next {
				capacity := memberCapacity(m.member, direction)
				allocations[m.member.ID] = remaining * capacity * m.weightFactor() / sum
			}
			return
		}
		active = next
	}
}

// withinDeadband reports whether the next request is close enough to the last
// dispatched request that re-dispatching would only add write noise.
func withinDeadband(last, next, deadband float64) bool {
	return deadband > 0 && math.Abs(next-last) <= deadband
}

// SimulateAllocation computes what the allocator would do for a request without
// writing to any member. It reads current state only, so the result is the
// distribution that a real dispatch would produce right now.
func SimulateAllocation(nc *nats.Conn, config AllocatorConfig, request float64) (alloc.Result, error) {
	if err := config.Validate(); err != nil {
		return alloc.Result{}, err
	}
	if math.IsNaN(request) || math.IsInf(request, 0) {
		return alloc.Result{}, fmt.Errorf("request must be finite")
	}
	members := make([]resolvedMember, 0, len(config.Members))
	for _, cfg := range config.Members {
		member, reasons, err := allocationMember(nc, cfg)
		if err != nil {
			return alloc.Result{}, err
		}
		members = append(members, resolvedMember{
			member: member, priority: cfg.Priority, weight: cfg.Weight, reasons: reasons,
		})
	}
	return distribute(request, members), nil
}
