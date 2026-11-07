package decision

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/ems/alloc"
)

// allocationRun is one output's current dispatch. Steps are deliberately
// serial: a member is not written until the previous member's readback is
// confirmed or its failure has stopped the sequence.
type allocationRun struct {
	parent       string
	config       AllocatorConfig
	result       alloc.Result
	steps        []allocationStep
	index        int
	sentAt       time.Time
	status       string
	reason       string
	requested    float64
	startedAt    time.Time
	lastRequested float64
	// Grace state: the last setpoint and the moment a member went unavailable,
	// so a brief outage holds the member instead of stepping power.
	lastSetpoint map[string]float64
	offlineSince map[string]time.Time
	// recorded is true once this run's terminal outcome has been written to the
	// allocator's history, so a completed dispatch is not recorded twice.
	recorded bool
}

type allocationStep struct {
	member AllocatorMember
	share  alloc.Share
	sent   bool
}

func newAllocationRun(parent string, config AllocatorConfig) *allocationRun {
	return &allocationRun{
		parent:       parent,
		config:       config,
		lastSetpoint: make(map[string]float64),
		offlineSince: make(map[string]time.Time),
	}
}

// allocate resolves every member, holds members still within their grace
// period, and distributes the remaining request by priority and weight. It has
// no side effects beyond reading current state.
func (r *allocationRun) allocate(nc *nats.Conn, requested float64, now time.Time) (alloc.Result, int, error) {
	resolved := make([]resolvedMember, 0, len(r.config.Members))
	notReady := 0
	for _, cfg := range r.config.Members {
		member, reasons, err := allocationMember(nc, cfg)
		if err != nil {
			return alloc.Result{}, 0, err
		}
		if member.NotReady {
			notReady++
		}
		resolved = append(resolved, resolvedMember{
			member: member, priority: cfg.Priority, weight: cfg.Weight, reasons: reasons,
		})
	}

	direction := 1.0
	if requested < 0 {
		direction = -1.0
	}
	held := make(map[string]float64)
	heldTotal := 0.0
	nonHeld := make([]resolvedMember, 0, len(resolved))
	for _, rm := range resolved {
		m := rm.member
		if m.Online && !m.Faulted && !m.Disabled {
			r.offlineSince[m.ID] = time.Time{}
			nonHeld = append(nonHeld, rm)
			continue
		}
		since, ok := r.offlineSince[m.ID]
		if !ok || since.IsZero() {
			r.offlineSince[m.ID] = now
		}
		if now.Sub(r.offlineSince[m.ID]) <= r.config.grace() &&
			r.lastSetpoint[m.ID] != 0 && math.Signbit(r.lastSetpoint[m.ID]) == math.Signbit(direction) {
			held[m.ID] = math.Abs(r.lastSetpoint[m.ID])
			heldTotal += math.Abs(r.lastSetpoint[m.ID])
			continue
		}
		nonHeld = append(nonHeld, rm)
	}

	remaining := requested
	if direction > 0 {
		remaining = math.Max(0, requested-heldTotal)
	} else {
		remaining = math.Min(0, requested+heldTotal)
	}
	distributed := distribute(remaining, nonHeld)
	byID := make(map[string]alloc.Share, len(distributed.Shares))
	for _, share := range distributed.Shares {
		byID[share.MemberID] = share
	}

	final := alloc.Result{Requested: requested, Shares: make([]alloc.Share, 0, len(resolved))}
	delivered := 0.0
	for _, rm := range resolved {
		m := rm.member
		if value, ok := held[m.ID]; ok {
			final.Shares = append(final.Shares, alloc.Share{
				MemberID: m.ID, Setpoint: direction * value, Held: true,
				Reason: "offline within grace; holding last setpoint",
			})
			delivered += value
			r.lastSetpoint[m.ID] = direction * value
			continue
		}
		share := byID[m.ID]
		if !share.Excluded && !share.Held {
			delivered += math.Abs(share.Setpoint)
			r.lastSetpoint[m.ID] = share.Setpoint
		}
		final.Shares = append(final.Shares, share)
	}
	final.Delivered = direction * delivered
	if math.Abs(requested) > delivered+1e-9 {
		final.Shortfall = direction * (math.Abs(requested) - delivered)
		final.Notes = append(final.Notes, fmt.Sprintf(
			"delivered %.1f kW of the %.1f kW request", delivered, math.Abs(requested)))
	}
	return final, notReady, nil
}

func (r *allocationRun) start(nc *nats.Conn, requested float64, now time.Time) error {
	result, notReady, err := r.allocate(nc, requested, now)
	if err != nil {
		return err
	}
	// A small request change that yields the same setpoints is not re-dispatched.
	// Only a previously confirmed allocation is reused; a failed or unknown run
	// must retry rather than pretend its setpoints are already in place.
	if r.status == "confirmed" && withinDeadband(r.lastRequested, requested, r.config.DeadbandKW) && sameAllocation(result, r.result) {
		r.status = "confirmed"
		r.reason = ""
		return nil
	}
	r.result = result
	r.requested = requested
	r.lastRequested = requested
	r.recorded = false
	if r.startedAt.IsZero() {
		r.startedAt = now
	}
	if notReady > 0 && requested != 0 {
		r.status = "retry"
		r.reason = "waiting for member health and capability"
		return nil
	}
	r.steps = make([]allocationStep, 0, len(r.config.Members))
	sendable := 0
	for _, member := range r.config.Members {
		share := shareByID(result.Shares, member.ID)
		if !share.Excluded && !share.Held {
			sendable++
		}
		r.steps = append(r.steps, allocationStep{member: member, share: share})
	}
	r.index = 0
	r.sentAt = time.Time{}
	r.status = "executing"
	r.reason = ""
	if sendable == 0 && requested != 0 && result.Shortfall != 0 {
		r.status = "retry"
		r.reason = "waiting for member health and capability"
	}
	return nil
}

// step advances until it is waiting for one readback, completes, or stops on
// failure. It never writes to an offline, faulted or manually held member.
func (r *allocationRun) step(nc *nats.Conn, now time.Time) {
	if r.status == "retry" {
		if now.Sub(r.startedAt) >= r.config.timeout() {
			r.status = "unknown"
			r.reason = "members were not ready before timeout"
			r.result.Notes = append(r.result.Notes, r.reason)
			return
		}
		if err := r.start(nc, r.requested, now); err != nil {
			r.status = "unknown"
			r.reason = err.Error()
			return
		}
	}
	if r.status != "executing" {
		return
	}
	for r.index < len(r.steps) {
		step := &r.steps[r.index]
		if step.share.Excluded || step.share.Held {
			r.index++
			continue
		}
		if !step.sent {
			resolved, _, err := allocationMember(nc, step.member)
			if err != nil {
				r.fail("unknown", step, err.Error())
				return
			}
			if resolved.Disabled || resolved.Faulted || !resolved.Online {
				step.share.Excluded = true
				step.share.Reason = "member became unavailable"
				r.updateShare(*step)
				r.index++
				continue
			}
			target := step.member.Target
			if target.PointType == "" {
				target.PointType = data.PointTypeValueSet
			}
			quantized, err := quantizeSetpoint(nc, target, step.share.Setpoint)
			if err != nil {
				r.fail("failed", step, err.Error())
				return
			}
			step.share.Setpoint = quantized
			r.updateShare(*step)
			if err := ValidateTarget(nc, r.parent, target, true, &step.share.Setpoint); err != nil {
				r.fail("failed", step, err.Error())
				return
			}
			point := data.NewPointFloat(target.PointType, target.Key, step.share.Setpoint)
			point.Origin = "easyems-allocator"
			if err := siotclient.SendNodePoint(nc, target.NodeID, point, true); err != nil {
				r.fail("failed", step, err.Error())
				return
			}
			log.Printf("allocator %s: sent %s = %.3f", r.parent, step.member.ID, step.share.Setpoint)
			step.sent = true
			r.sentAt = now
		}

		actual, at, found := readActual(nc, step.member)
		if found && !at.Before(r.sentAt) && math.Abs(actual-step.share.Setpoint) < 1e-6 {
			log.Printf("allocator %s: confirmed %s = %.3f", r.parent, step.member.ID, actual)
			step.share.Actual = actual
			step.share.Confirmed = true
			step.share.Reason = "readback confirmed"
			r.updateShare(*step)
			r.index++
			r.sentAt = time.Time{}
			continue
		}
		if now.Sub(r.sentAt) >= r.config.timeout() {
			r.fail("unknown", step, "no readback before timeout")
			return
		}
		return
	}
	r.status = "confirmed"
	r.reason = ""
}

func (r *allocationRun) fail(status string, step *allocationStep, reason string) {
	r.status = status
	r.reason = reason
	step.share.Reason = reason
	r.updateShare(*step)
	r.result.Notes = append(r.result.Notes, step.member.ID+": "+reason)
}

func (r *allocationRun) updateShare(step allocationStep) {
	for i := range r.result.Shares {
		if r.result.Shares[i].MemberID == step.member.ID {
			r.result.Shares[i] = step.share
			break
		}
	}
	delivered := 0.0
	for _, share := range r.result.Shares {
		if !share.Excluded {
			delivered += share.Setpoint
		}
	}
	r.result.Delivered = delivered
	r.result.Shortfall = r.result.Requested - delivered
}

// quantizeSetpoint keeps integer-register writes representable. Truncation
// toward zero never increases the requested power magnitude.
func quantizeSetpoint(nc *nats.Conn, target Target, value float64) (float64, error) {
	nodes, err := siotclient.GetNodes(nc, "all", target.NodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, fmt.Errorf("setpoint target unavailable")
	}
	node := nodes[0]
	if node.Type != data.NodeTypeModbusIO {
		return value, nil
	}
	format, _ := node.Points.Text(data.PointTypeDataFormat, "")
	if format == data.PointValueFLOAT32 {
		return value, nil
	}
	scale, _ := node.Points.Value(data.PointTypeScale, "")
	if scale == 0 {
		scale = 1
	}
	raw := value / scale
	if math.Abs(raw-math.Round(raw)) < 1e-6 {
		return value, nil
	}
	return math.Trunc(raw) * scale, nil
}

func shareByID(shares []alloc.Share, id string) alloc.Share {
	for _, share := range shares {
		if share.MemberID == id {
			return share
		}
	}
	return alloc.Share{MemberID: id, Excluded: true, Reason: "not eligible"}
}

func readActual(nc *nats.Conn, member AllocatorMember) (float64, time.Time, bool) {
	target := member.Target
	if member.Readback != nil {
		target = *member.Readback
	}
	target.PointType = data.PointTypeValue
	nodes, err := siotclient.GetNodes(nc, "all", target.NodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, time.Time{}, false
	}
	point, found := nodes[0].Points.Find(data.PointTypeValue, target.Key)
	if !found || point.Tombstone%2 != 0 {
		return 0, time.Time{}, false
	}
	value, err := point.ValueFloat()
	if err != nil {
		return 0, time.Time{}, false
	}
	return value, point.Time, true
}

func (r *allocationRun) signature() string {
	return fmt.Sprintf("%s|%s|%v", r.status, r.reason, r.result.Shares)
}

// sameAllocation reports whether two results ask every member for the same
// setpoint and availability, so a deadband-skipped request is detectable.
func sameAllocation(a, b alloc.Result) bool {
	if len(a.Shares) != len(b.Shares) {
		return false
	}
	for i := range a.Shares {
		x, y := a.Shares[i], b.Shares[i]
		if x.MemberID != y.MemberID || x.Excluded != y.Excluded || x.Held != y.Held {
			return false
		}
		if math.Abs(x.Setpoint-y.Setpoint) > 1e-9 {
			return false
		}
	}
	return true
}
