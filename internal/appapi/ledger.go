package appapi

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/ems/channel"
)

type LedgerDevice struct {
	ChannelID        string  `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ServerID         int     `json:"server_id"`
	Name             string  `json:"name"`
	BusinessNodeID   string  `json:"business_node_id,omitempty"`
	BusinessNodeName string  `json:"business_node_name,omitempty"`
	ProfileID        string  `json:"profile_id,omitempty"`
	Vendor           string  `json:"vendor,omitempty"`
	Model            string  `json:"model,omitempty"`
	Protocol         string  `json:"protocol"`
	Address          string  `json:"address,omitempty"`
	PointCount       int     `json:"point_count"`
	CommandCount     int     `json:"command_count"`
	Health           string  `json:"health"`
	SuccessRate      float64 `json:"success_rate"`
	AvgLatencyMS     float64 `json:"avg_latency_ms"`
	Verified         bool    `json:"verified"`
	Verifier         string  `json:"verifier,omitempty"`
	VerifiedAt       string  `json:"verified_at,omitempty"`
	DeviceSerial     string  `json:"device_serial,omitempty"`
}

type DeviceLedgerResponse struct {
	Devices []LedgerDevice `json:"devices"`
}

func (h *Handler) deviceLedger(w http.ResponseWriter, r *http.Request, identity Identity) {
	siteID := r.URL.Query().Get("site")
	if !h.checkSite(w, identity, siteID) {
		return
	}
	devices, err := h.ledgerDevices(identity, siteID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, DeviceLedgerResponse{Devices: devices})
}

func (h *Handler) ledgerDevices(identity Identity, siteID string) ([]LedgerDevice, error) {
	nodes, err := h.visibleNodes(identity)
	if err != nil {
		return nil, errors.New("nodes_unavailable")
	}
	siteRefs, siteNodes := siteLedgerRefs(nodes, siteID)
	if h.ems == nil || h.ems.Profiles == nil {
		return nil, errors.New("ems_unavailable")
	}
	names := map[string]string{}
	for _, node := range nodes {
		names[node.ID] = node.Desc()
	}
	runtime := map[string]*channel.Channel{}
	if h.ems.Service != nil {
		for _, ch := range h.ems.Service.Channels() {
			runtime[ch.ID] = ch
		}
	}
	now := time.Now()
	devices := []LedgerDevice{}
	for _, node := range nodes {
		if node.Type != data.NodeTypeModbus {
			continue
		}
		points := h.unitPoints(node.ID)
		byServer := map[int][]unitModelPointView{}
		for _, point := range points {
			byServer[point.ServerID] = append(byServer[point.ServerID], point)
		}
		servers := map[int]bool{}
		for server := range byServer {
			servers[server] = true
		}
		for _, point := range node.Points {
			if point.Type == unitModelPoint && point.Tombstone%2 == 0 {
				if server, err := strconv.Atoi(point.Key); err == nil {
					servers[server] = true
				}
			}
		}
		protocol := strings.ToUpper(textPoint(node.Points, data.PointTypeProtocol))
		if protocol == "" {
			protocol = "TCP"
		}
		address := textPoint(node.Points, data.PointTypeURI)
		if protocol == "RTU" {
			address = textPoint(node.Points, data.PointTypePort)
		}
		for server := range servers {
			model, hasModel := unitModelFor(node, server)
			item := LedgerDevice{
				ChannelID: node.ID, ChannelName: node.Desc(), ServerID: server,
				Name: "站号 " + strconv.Itoa(server), Protocol: protocol, Address: address,
				PointCount: len(byServer[server]), Health: string(channel.HealthUnknown),
			}
			if hasModel {
				if model.Name != "" {
					item.Name = model.Name
				}
				item.ProfileID = model.ProfileID
				item.BusinessNodeID = model.BusinessNodeID
				item.BusinessNodeName = names[model.BusinessNodeID]
				item.Verified = model.Verified
				item.Verifier = model.Verifier
				item.VerifiedAt = model.VerifiedAt
				item.DeviceSerial = model.DeviceSerial
				if profile, ok := h.ems.Profiles.Get(model.ProfileID); ok {
					item.Vendor = profile.Vendor
					item.Model = profile.Model
					item.CommandCount = len(profile.CommandSpecs)
				}
			}
			deviceID := node.ID + "/" + strconv.Itoa(server)
			if siteID != "" && !siteRefs[deviceID] && !siteNodes[item.BusinessNodeID] {
				continue
			}
			if ch, ok := runtime[node.ID]; ok {
				item.Health = string(ch.Health(deviceID, now, h.ems.MaxAge))
				stats := ch.Stats(deviceID)
				if stats.Requests > 0 {
					item.SuccessRate = float64(stats.Requests-stats.Failures) / float64(stats.Requests) * 100
				}
				if stats.LatencyCount > 0 {
					item.AvgLatencyMS = stats.LatencyTotalMS / float64(stats.LatencyCount)
				}
			}
			devices = append(devices, item)
		}
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].ChannelName != devices[j].ChannelName {
			return devices[i].ChannelName < devices[j].ChannelName
		}
		return devices[i].ServerID < devices[j].ServerID
	})
	return devices, nil
}
