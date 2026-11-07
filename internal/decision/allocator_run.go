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
	parent    string
	config    AllocatorConfig
	tracker   *alloc.Tracker
	result    alloc.Result
	steps     []allocationStep
	index     int
	sentAt    time.Time
	status    string
	reason    string
	requested float64
	startedAt time.Time
}

type allocationStep struct {
	member AllocatorMember
	share  alloc.Share
	sent   bool
}

func newAllocationRun(parent string, config AllocatorConfig) *allocationRun {
	return &allocationRun{
		parent: parent, config: config,
		tracker: alloc.NewTracker(config.grace()),
	}
}

func (r *allocationRun) start(nc *nats.Conn, requested float64, now time.Time) error {
	members := make([]alloc.Member, 0, len(r.config.Members))
	notReady := 0
	for _, member := range r.config.Members {
		resolved, err := allocationMember(nc, member)
		if err != nil {
			return err
		}
		if resolved.NotReady {
			notReady++
		}
		members = append(members, resolved)
	}
	result, err := r.tracker.Apply(now, requested, members)
	if err != nil {
		return err
	}
	r.result = result
	r.requested = requested
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
			resolved, err := allocationMember(nc, step.member)
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
