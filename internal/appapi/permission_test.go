package appapi

import "testing"

// TestCanActionRoleDefaults asserts the default action set expanded from each
// role: admin everything, implementer all but account/audit/site, operator the
// run/ack/maintain set, viewer nothing.
func TestCanActionRoleDefaults(t *testing.T) {
	grants := map[string]map[string]bool{
		ActionConfigure:     {"admin": true, "implementer": true, "operator": false, "viewer": false},
		ActionRulePublish:   {"admin": true, "implementer": true, "operator": false, "viewer": false},
		ActionDelete:        {"admin": true, "implementer": true, "operator": false, "viewer": false},
		ActionExport:        {"admin": true, "implementer": true, "operator": false, "viewer": false},
		ActionWriteValue:    {"admin": true, "implementer": true, "operator": true, "viewer": false},
		ActionCommand:       {"admin": true, "implementer": true, "operator": true, "viewer": false},
		ActionAlarmAck:      {"admin": true, "implementer": true, "operator": true, "viewer": false},
		ActionAlarmMaintain: {"admin": true, "implementer": true, "operator": true, "viewer": false},
		ActionAccountManage: {"admin": true, "implementer": false, "operator": false, "viewer": false},
		ActionAuditView:     {"admin": true, "implementer": false, "operator": false, "viewer": false},
		ActionSiteSettings:  {"admin": true, "implementer": false, "operator": false, "viewer": false},
	}
	for _, role := range []string{"admin", "implementer", "operator", "viewer"} {
		identity := Identity{Policy: AccessPolicy{Admin: role == "admin", Role: role}}
		for _, action := range allActions {
			want := grants[action][role]
			if got := canAction(identity, action); got != want {
				t.Errorf("canAction(%s, %s) = %v, want %v", role, action, got, want)
			}
		}
	}
}

func TestCanActionWhitelistOverridesRole(t *testing.T) {
	identity := Identity{Policy: AccessPolicy{Role: "implementer", Actions: []string{ActionAlarmAck}}}
	if !canAction(identity, ActionAlarmAck) {
		t.Error("whitelist should grant a listed action")
	}
	if canAction(identity, ActionConfigure) {
		t.Error("whitelist should override the role default and deny unlisted actions")
	}
	if canAction(identity, "unknown.action") {
		t.Error("unknown action should be denied")
	}
}

func TestCanActionEmptyWhitelistFallsBackToRole(t *testing.T) {
	identity := Identity{Policy: AccessPolicy{Role: "operator"}}
	if !canAction(identity, ActionWriteValue) {
		t.Error("empty whitelist should fall back to the operator default")
	}
	if canAction(identity, ActionConfigure) {
		t.Error("operator default must not include configure")
	}
}

func TestCanActionAdminAlwaysPasses(t *testing.T) {
	identity := Identity{Policy: AccessPolicy{Admin: true, Actions: []string{}}}
	for _, action := range allActions {
		if !canAction(identity, action) {
			t.Errorf("admin should pass %s", action)
		}
	}
}

func TestCanActionViewerDeniedEveryAction(t *testing.T) {
	identity := Identity{Policy: AccessPolicy{Role: "viewer"}}
	for _, action := range allActions {
		if canAction(identity, action) {
			t.Errorf("viewer should be denied %s", action)
		}
	}
}
