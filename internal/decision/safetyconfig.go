package decision

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"

	"fastiot/internal/ems/safety"
)

const outputSafetyPoint = "easyemsSafety"

// LoadOutputSafety reads the per-output envelope stored on the target node.
// A missing point means "not configured"; a present but malformed point is an
// error so a broken safety config fails the rule closed instead of silently
// disabling limits.
func LoadOutputSafety(nc *nats.Conn, nodeID string) (safety.Config, bool, error) {
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return safety.Config{}, false, nil
	}
	point, ok := nodes[0].Points.Find(outputSafetyPoint, "")
	if !ok || point.Tombstone%2 != 0 {
		return safety.Config{}, false, nil
	}
	var config safety.Config
	if err := json.Unmarshal([]byte(point.Txt()), &config); err != nil {
		return safety.Config{}, true, err
	}
	return config, true, nil
}
