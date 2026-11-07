package appapi

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/simpleiot/simpleiot/data"
)

type profileVerification struct {
	ChannelID    string `json:"channel_id"`
	ChannelName  string `json:"channel_name"`
	ServerID     int    `json:"server_id"`
	UnitName     string `json:"unit_name"`
	ProfileID    string `json:"profile_id"`
	Vendor       string `json:"vendor"`
	Model        string `json:"model"`
	Verified     bool   `json:"verified"`
	Verifier     string `json:"verifier,omitempty"`
	VerifiedAt   string `json:"verified_at,omitempty"`
	DeviceSerial string `json:"device_serial,omitempty"`
	Note         string `json:"note,omitempty"`
	PointCount   int    `json:"point_count"`
}

func (h *Handler) profileVerifications(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	list, err := h.verificationList(identity)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "nodes_unavailable")
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="easyems-profile-verifications.csv"`)
		writer := csv.NewWriter(w)
		writeVerificationCSV(writer, list)
		return
	}
	writeJSON(w, map[string]any{"verifications": list})
}

func (h *Handler) verificationList(identity Identity) ([]profileVerification, error) {
	nodes, err := h.visibleNodes(identity)
	if err != nil {
		return nil, err
	}
	list := []profileVerification{}
	for _, node := range nodes {
		if node.Type != data.NodeTypeModbus {
			continue
		}
		for _, point := range node.Points {
			if point.Type != unitModelPoint || point.Tombstone%2 != 0 {
				continue
			}
			server, err := strconv.Atoi(point.Key)
			if err != nil {
				continue
			}
			var model unitModelConfig
			if json.Unmarshal([]byte(point.Txt()), &model) != nil {
				continue
			}
			vendor, modelName := "", ""
			if h.ems != nil && h.ems.Profiles != nil {
				if profile, ok := h.ems.Profiles.Get(model.ProfileID); ok {
					vendor, modelName = profile.Vendor, profile.Model
				}
			}
			list = append(list, profileVerification{
				ChannelID: node.ID, ChannelName: node.Desc(), ServerID: server,
				UnitName: model.Name, ProfileID: model.ProfileID, Vendor: vendor, Model: modelName,
				Verified: model.Verified, Verifier: model.Verifier, VerifiedAt: model.VerifiedAt,
				DeviceSerial: model.DeviceSerial, Note: model.Verification, PointCount: len(model.PointMap),
			})
		}
	}
	return list, nil
}

func writeVerificationCSV(writer *csv.Writer, list []profileVerification) {
	_ = writer.Write([]string{"通道", "站号", "设备名称", "Profile", "厂商", "型号", "已验证", "验证人", "验证日期", "设备序列号", "备注", "点位数"})
	for _, item := range list {
		verified := "否"
		if item.Verified {
			verified = "是"
		}
		_ = writer.Write([]string{
			item.ChannelName, strconv.Itoa(item.ServerID), item.UnitName, item.ProfileID,
			item.Vendor, item.Model, verified, item.Verifier, item.VerifiedAt,
			item.DeviceSerial, item.Note, strconv.Itoa(item.PointCount),
		})
	}
	writer.Flush()
}
