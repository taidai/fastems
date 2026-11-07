// Package alloc distributes one power target across several members (PCS
// units) by available capability, excluding unavailable members, clamping to
// limits and reallocating the remainder.
package alloc

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Member is one allocation target. Charge and discharge capabilities are
// magnitudes in kW; request sign decides which pair is used.
type Member struct {
	ID                 string
	Online             bool
	Faulted            bool
	Disabled           bool
	NotReady           bool
	MinPower           float64 // absolute minimum when the member takes part; 0 = none
	MaxCharge          float64
	MaxDischarge       float64
	AvailableCharge    float64
	AvailableDischarge float64
}

func (m Member) capacity(direction float64) float64 {
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

// Share is what one member is asked to do, and why.
type Share struct {
	MemberID  string  `json:"member_id"`
	Capacity  float64 `json:"capacity"`
	Setpoint  float64 `json:"setpoint"`
	Actual    float64 `json:"actual,omitempty"`
	Confirmed bool    `json:"confirmed,omitempty"`
	Excluded  bool    `json:"excluded"`
	Held      bool    `json:"held"`
	Reason    string  `json:"reason"`
}

// Result is one allocation. Delivered and Shortfall are signed magnitudes:
// positive for discharge, negative for charge.
type Result struct {
	Requested float64  `json:"requested"`
	Delivered float64  `json:"delivered"`
	Shortfall float64  `json:"shortfall"`
	Shares    []Share  `json:"shares"`
	Notes     []string `json:"notes,omitempty"`
}

const epsilon = 1e-9

// Allocate splits a signed target across members. Positive is discharge,
// negative is charge. It has no state and no side effects.
func Allocate(request float64, members []Member) (Result, error) {
	if math.IsNaN(request) || math.IsInf(request, 0) {
		return Result{}, errors.New("request must be finite")
	}
	direction := 1.0
	if request < 0 {
		direction = -1
	}
	target := math.Abs(request)
	result := Result{Requested: request, Shares: make([]Share, len(members))}

	candidates := make([]int, 0, len(members))
	for i, member := range members {
		share := &result.Shares[i]
		share.MemberID = member.ID
		switch {
		case member.Disabled:
			share.Excluded, share.Reason = true, "disabled"
		case member.Faulted:
			share.Excluded, share.Reason = true, "faulted"
		case !member.Online:
			share.Excluded, share.Reason = true, "offline"
		default:
			share.Capacity = member.capacity(direction)
			if share.Capacity <= 0 {
				share.Excluded, share.Reason = true, "no available capacity in this direction"
				continue
			}
			candidates = append(candidates, i)
		}
	}

	allocations := waterFill(target, direction, members, candidates, &result)
	delivered := 0.0
	for i := range result.Shares {
		share := &result.Shares[i]
		if value, ok := allocations[share.MemberID]; ok {
			share.Setpoint = direction * value
			delivered += value
		}
	}
	result.Delivered = direction * delivered
	result.Shortfall = direction * math.Max(0, target-delivered)
	if target-delivered > epsilon {
		result.Notes = append(result.Notes, fmt.Sprintf(
			"physical capability is %.1f kW short of the %.1f kW request", target-delivered, target))
	}
	return result, nil
}

func waterFill(target, direction float64, members []Member, candidates []int, result *Result) map[string]float64 {
	active := append([]int(nil), candidates...)
	remaining := target
	allocations := make(map[string]float64)
	for len(active) > 0 && remaining > epsilon {
		sum := 0.0
		for _, index := range active {
			sum += members[index].capacity(direction)
		}
		if sum <= 0 {
			break
		}
		if remaining >= sum {
			for _, index := range active {
				allocations[members[index].ID] = members[index].capacity(direction)
			}
			remaining -= sum
			break
		}
		next := make([]int, 0, len(active))
		values := make(map[int]float64, len(active))
		excluded := 0
		for _, index := range active {
			value := remaining * members[index].capacity(direction) / sum
			if members[index].MinPower > 0 && value < members[index].MinPower {
				result.Shares[index].Excluded = true
				result.Shares[index].Reason = fmt.Sprintf(
					"share %.1f kW is below the %.1f kW minimum", value, members[index].MinPower)
				excluded++
				continue
			}
			values[index] = value
			next = append(next, index)
		}
		if excluded == 0 {
			for index, value := range values {
				allocations[members[index].ID] = value
			}
			return allocations
		}
		active = next
	}
	return allocations
}

type heldState struct {
	setpoint     float64
	offlineSince time.Time
}

// Tracker keeps the last setpoint for a short grace period so a single lost
// poll does not cause a power step. After the grace period the member is
// excluded and its share is reallocated.
type Tracker struct {
	Grace time.Duration
	state map[string]*heldState
}

func NewTracker(grace time.Duration) *Tracker {
	return &Tracker{Grace: grace, state: make(map[string]*heldState)}
}

// Apply allocates with grace handling. Held members keep their last setpoint;
// the rest of the request goes to eligible members.
func (t *Tracker) Apply(now time.Time, request float64, members []Member) (Result, error) {
	if math.IsNaN(request) || math.IsInf(request, 0) {
		return Result{}, errors.New("request must be finite")
	}
	direction := 1.0
	if request < 0 {
		direction = -1
	}
	held := make(map[string]float64)
	excludedReasons := make(map[string]string)
	active := make([]Member, 0, len(members))
	for _, member := range members {
		state := t.state[member.ID]
		if state == nil {
			state = &heldState{}
			t.state[member.ID] = state
		}
		eligible := member.Online && !member.Faulted && !member.Disabled
		if eligible {
			state.offlineSince = time.Time{}
			active = append(active, member)
			continue
		}
		if state.offlineSince.IsZero() {
			state.offlineSince = now
		}
		switch {
		case member.Disabled:
			excludedReasons[member.ID] = "disabled"
		case member.Faulted:
			excludedReasons[member.ID] = "faulted"
		case !member.Online:
			excludedReasons[member.ID] = "offline"
		}
		if now.Sub(state.offlineSince) <= t.Grace && state.setpoint != 0 && math.Signbit(state.setpoint) == math.Signbit(direction) {
			held[member.ID] = math.Abs(state.setpoint)
		}
	}

	heldTotal := 0.0
	for _, value := range held {
		heldTotal += value
	}
	remaining := request
	if direction > 0 {
		remaining = math.Max(0, request-heldTotal)
	} else {
		remaining = math.Min(0, request+heldTotal)
	}

	inner, err := Allocate(remaining, active)
	if err != nil {
		return Result{}, err
	}
	byID := make(map[string]Share, len(inner.Shares))
	for _, share := range inner.Shares {
		byID[share.MemberID] = share
	}

	result := Result{Requested: request, Shares: make([]Share, 0, len(members))}
	delivered := 0.0
	for _, member := range members {
		if value, ok := held[member.ID]; ok {
			result.Shares = append(result.Shares, Share{
				MemberID: member.ID,
				Setpoint: direction * value,
				Held:     true,
				Reason:   "offline within grace; holding last setpoint",
			})
			delivered += value
			continue
		}
		share, ok := byID[member.ID]
		if !ok {
			reason := excludedReasons[member.ID]
			if reason == "" {
				reason = "not eligible"
			}
			share = Share{MemberID: member.ID, Excluded: true, Reason: reason}
		}
		result.Shares = append(result.Shares, share)
		if !share.Excluded && !share.Held {
			delivered += math.Abs(share.Setpoint)
		}
	}
	result.Delivered = direction * delivered
	if math.Abs(request) > delivered+epsilon {
		result.Shortfall = direction * (math.Abs(request) - delivered)
		result.Notes = append(result.Notes, fmt.Sprintf(
			"delivered %.1f kW of the %.1f kW request", delivered, math.Abs(request)))
	}
	for _, member := range members {
		if value, ok := held[member.ID]; ok {
			t.state[member.ID].setpoint = direction * value
			continue
		}
		if share, ok := byID[member.ID]; ok && !share.Excluded {
			t.state[member.ID].setpoint = share.Setpoint
		}
	}
	return result, nil
}
