package decision

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
)

type outputTarget struct {
	nodeID    string
	pointType string
	key       string
}

func normalizeOutput(target Target) outputTarget {
	pointType := target.PointType
	if pointType == "" {
		pointType = "value"
	}
	return outputTarget{nodeID: target.NodeID, pointType: pointType, key: target.Key}
}

func outputTargets(document Document) []outputTarget {
	var targets []outputTarget
	for _, row := range document.Table.Rows {
		for _, action := range row.Actions {
			target := document.Bindings.Actions[action.ActionID]
			if target.NodeID == "" {
				continue
			}
			normalized := normalizeOutput(target)
			duplicate := false
			for _, existing := range targets {
				if existing == normalized {
					duplicate = true
					break
				}
			}
			if !duplicate {
				targets = append(targets, normalized)
			}
		}
	}
	return targets
}

// ValidateOutputOwnership enforces the v1 rule that one controllable point can
// be owned by only one enabled decision table at a time. The check runs on
// enable and on republish of an already enabled table, so ownership cannot be
// changed by publishing a new version behind the operator's back.
func ValidateOutputOwnership(nc *nats.Conn, ruleID string, document Document) error {
	tables, err := siotclient.GetNodesType[DecisionTable](nc, "all", "all")
	if err != nil {
		return fmt.Errorf("cannot verify output ownership")
	}
	mine := outputTargets(document)
	if len(mine) == 0 {
		return nil
	}
	for _, table := range tables {
		if table.ID == ruleID || !table.Enabled || table.Applied == "" {
			continue
		}
		var other Document
		if json.Unmarshal([]byte(table.Applied), &other) != nil {
			continue
		}
		for _, owned := range outputTargets(other) {
			for _, target := range mine {
				if owned == target {
					name := table.Description
					if name == "" {
						name = table.ID
					}
					return fmt.Errorf("output %s is already owned by %s", target.nodeID, name)
				}
			}
		}
	}
	return nil
}
