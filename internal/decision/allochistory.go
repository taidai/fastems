package decision

import (
	"encoding/json"
	"sort"
	"time"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/alloc"
)

const allocHistoryPoint = "easyemsAllocHistory"

// allocHistoryLimit bounds how many snapshots are kept per allocator node.
const allocHistoryLimit = 100

// AllocationSnapshot is one recorded allocation outcome: when it happened, what
// was requested and delivered, and how the request was split.
type AllocationSnapshot struct {
	At        time.Time     `json:"at"`
	Requested float64       `json:"requested"`
	Delivered float64       `json:"delivered"`
	Shortfall float64       `json:"shortfall"`
	Status    string        `json:"status"`
	Reason    string        `json:"reason,omitempty"`
	Shares    []alloc.Share `json:"shares"`
}

// recordAllocation appends one snapshot to the allocator node and trims the
// oldest entries past allocHistoryLimit. Failures are deliberately silent: a
// history write must never block dispatch.
func (c *Client) recordAllocation(nodeID string, run *allocationRun, now time.Time) {
	snapshot := AllocationSnapshot{
		At: now, Requested: run.result.Requested, Delivered: run.result.Delivered,
		Shortfall: run.result.Shortfall, Status: run.status, Reason: run.reason,
		Shares: run.result.Shares,
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	point := data.NewPointString(allocHistoryPoint, now.Format(time.RFC3339Nano), string(encoded))
	point.Origin = c.config.ID
	if err := siotclient.SendNodePoint(c.nc, nodeID, point, true); err != nil {
		return
	}
	c.trimAllocationHistory(nodeID)
}

func (c *Client) trimAllocationHistory(nodeID string) {
	nodes, err := siotclient.GetNodes(c.nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return
	}
	var points []data.Point
	for _, point := range nodes[0].Points {
		if point.Type == allocHistoryPoint && point.Tombstone%2 == 0 {
			points = append(points, point)
		}
	}
	if len(points) <= allocHistoryLimit {
		return
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Key < points[j].Key })
	for _, point := range points[:len(points)-allocHistoryLimit] {
		point.Tombstone = 1
		point.Origin = c.config.ID
		_ = siotclient.SendNodePoint(c.nc, nodeID, point, true)
	}
}
