package appapi

import (
	"errors"
	"net/http"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

var errNoVisibleParent = errors.New("no_visible_parent")

// checkedEnergyUnits resolves EnergyUnit references through the same tree-scope
// check as other HTTP targets. Callers must finish this check before writing.
func (h *Handler) checkedEnergyUnits(w http.ResponseWriter, identity Identity, ids []string) ([]data.NodeEdge, bool) {
	nodes := make([]data.NodeEdge, 0, len(ids))
	for _, id := range ids {
		node, ok := h.node(w, identity, id)
		if !ok {
			return nil, false
		}
		if node.Type != NodeTypeEnergyUnit {
			writeError(w, http.StatusBadRequest, "requires_energy_unit")
			return nil, false
		}
		nodes = append(nodes, node)
	}
	return nodes, true
}

// resourceGroupParent keeps admin's existing root placement. A scoped group is
// attached to its first member's visible parent rather than an invisible root.
func (h *Handler) resourceGroupParent(identity Identity, members []data.NodeEdge) (string, error) {
	if identity.Policy.Admin {
		root, err := siotclient.GetRootNode(h.nc)
		if err != nil {
			return "", err
		}
		return root.ID, nil
	}
	if len(members) == 0 || !h.allows(identity, members[0].Parent) {
		return "", errNoVisibleParent
	}
	return members[0].Parent, nil
}

// visibleDefaultPlant preserves the original global default for admins and
// root-scoped users, but never selects or creates below an invisible parent.
func (h *Handler) visibleDefaultPlant(identity Identity) (string, error) {
	if identity.Policy.Admin {
		return ensureDefaultPlant(h.nc)
	}
	root, err := siotclient.GetRootNode(h.nc)
	if err != nil {
		return "", err
	}
	if h.allows(identity, root.ID) {
		return ensureDefaultPlant(h.nc)
	}

	sites, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypeSite, false)
	if err != nil {
		return "", err
	}
	firstVisibleSite := ""
	for _, site := range sites {
		if !h.allows(identity, site.ID) {
			continue
		}
		if firstVisibleSite == "" {
			firstVisibleSite = site.ID
		}
		plants, err := siotclient.GetNodes(h.nc, site.ID, "all", NodeTypePlant, false)
		if err != nil {
			return "", err
		}
		if len(plants) > 0 {
			return plants[0].ID, nil
		}
	}

	// A user can be anchored directly at a Plant without seeing its Site.
	plants, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypePlant, false)
	if err != nil {
		return "", err
	}
	for _, plant := range plants {
		if h.allows(identity, plant.ID) {
			return plant.ID, nil
		}
	}
	if firstVisibleSite != "" {
		return CreateDomainNode(h.nc, firstVisibleSite, NodeTypePlant, map[string]string{
			PointTypeAssetCode: "PLANT01", "description": "默认电站",
		})
	}
	return "", errNoVisibleParent
}
