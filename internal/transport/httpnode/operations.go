package httpnode

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
)

type operationRequest struct {
	OperationID    string            `json:"operation_id"`
	DeviceAccessID string            `json:"device_access_id"`
	Protocol       string            `json:"protocol"`
	PeerPublicKey  string            `json:"peer_public_key"`
	AssignedIP     string            `json:"assigned_ip"`
	Keepalive      int               `json:"keepalive"`
	EndpointMeta   map[string]string `json:"endpoint_metadata,omitempty"`
}

func (r operationRequest) validate() error {
	if r.OperationID == "" || r.DeviceAccessID == "" || r.Protocol == "" || r.PeerPublicKey == "" || r.AssignedIP == "" {
		return errors.New("missing required fields")
	}
	if r.Keepalive < 0 {
		return errors.New("keepalive must be non-negative")
	}
	return nil
}

func (r operationRequest) toRuntimeRequest() runtime.PeerOperationRequest {
	return runtime.PeerOperationRequest{
		OperationID:    r.OperationID,
		DeviceAccessID: r.DeviceAccessID,
		Protocol:       r.Protocol,
		PeerPublicKey:  r.PeerPublicKey,
		AssignedIP:     r.AssignedIP,
		Keepalive:      r.Keepalive,
		EndpointMeta:   r.EndpointMeta,
	}
}

func applyPeerHandler(logger *slog.Logger, vpn runtime.VPNRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req operationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validate(); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		result, err := vpn.ApplyPeer(r.Context(), req.toRuntimeRequest())
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, runtime.ErrOperationConflict) {
				status = http.StatusConflict
			}
			if errors.Is(err, runtime.ErrNotImplemented) {
				status = http.StatusNotImplemented
			}
			respondError(w, status, err.Error())
			return
		}

		logger.Info("apply-peer processed",
			slog.String("operation_id", req.OperationID),
			slog.String("device_access_id", req.DeviceAccessID),
			slog.Bool("idempotent", result.Idempotent),
		)

		respondOperationResult(w, result)
	}
}

func revokePeerHandler(logger *slog.Logger, vpn runtime.VPNRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req operationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validate(); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		result, err := vpn.RevokePeer(r.Context(), req.toRuntimeRequest())
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, runtime.ErrOperationConflict) {
				status = http.StatusConflict
			}
			if errors.Is(err, runtime.ErrNotImplemented) {
				status = http.StatusNotImplemented
			}
			respondError(w, status, err.Error())
			return
		}

		logger.Info("revoke-peer processed",
			slog.String("operation_id", req.OperationID),
			slog.String("device_access_id", req.DeviceAccessID),
			slog.Bool("idempotent", result.Idempotent),
		)

		respondOperationResult(w, result)
	}
}

func respondOperationResult(w http.ResponseWriter, result runtime.OperationResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"operation_id": result.OperationID,
		"applied":      result.Applied,
		"idempotent":   result.Idempotent,
	})
}
