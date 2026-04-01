package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
	"github.com/wwwcont/ryazanvpn/internal/infra/wgkeys"
)

var ErrUserAlreadyHasActiveDevice = errors.New("user already has active device")
var ErrInsufficientBalance = errors.New("insufficient balance")

type CreateDeviceForUserInput struct {
	UserID   string
	Name     string
	Platform string
}

type CreateDeviceForUserOutput struct {
	Device              *device.Device
	Access              *access.DeviceAccess
	AccessByProtocol    map[string]*access.DeviceAccess
	PrivateKey          string
	Node                *node.Node
	ConfigDownloadToken string
	ConfigTokens        map[string]string
}

type CreateDeviceForUser struct {
	Users              UserRepository
	Devices            DeviceRepository
	Nodes              NodeRepository
	Accesses           DeviceAccessRepository
	Operations         NodeOperationRepository
	AuditLogs          AuditLogRepository
	KeyGenerator       KeyGenerator
	PresharedKeys      PresharedKeyGenerator
	IPAllocator        IPAllocator
	NodeAssigner       NodeAssigner
	CreatePeerExecutor *ExecuteCreatePeerOperation
	ConfigIssuer       *IssueDeviceConfig
	ServerPublicKey    string
	EndpointHost       string
	EndpointPort       int
	PublicEndpoint     string
	DNS                []string
	ClientAllowedIPs   []string
	Keepalive          int
	MTU                int
	DefaultVPNAWG      DefaultVPNAWGFields
	XrayPublicHost     string
	XrayRealityPort    int
	XrayRealitySNI     string
	TokenTTL           time.Duration
	SensitiveEncryptor EncryptionService
}

func (uc CreateDeviceForUser) Execute(ctx context.Context, in CreateDeviceForUserInput) (*CreateDeviceForUserOutput, error) {
	slog.Info("create_device_for_user.start", "user_id", in.UserID)
	if uc.Users != nil {
		u, err := uc.Users.GetByID(ctx, in.UserID)
		if err != nil {
			slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", err)
			return nil, err
		}
		if strings.EqualFold(u.Status, user.StatusBlocked) {
			slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", ErrInsufficientBalance)
			return nil, ErrInsufficientBalance
		}
	}
	if existing, err := uc.Devices.GetActiveByUserID(ctx, in.UserID); err == nil && existing != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", ErrUserAlreadyHasActiveDevice)
		return nil, ErrUserAlreadyHasActiveDevice
	} else if err != nil && !errors.Is(err, device.ErrNotFound) {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", err)
		return nil, err
	}

	publicKey, privateKey, err := uc.KeyGenerator.Generate(ctx)
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", err)
		return nil, err
	}
	if err := wgkeys.ValidateKeyPair(privateKey, publicKey); err != nil {
		slog.Error("device keypair validation failed", "error", err)
		return nil, fmt.Errorf("validate generated keypair: %w", err)
	}
	derivedPublicKey, err := wgkeys.DerivePublicKey(privateKey)
	if err != nil {
		slog.Error("device keypair derivation failed", "error", err)
		return nil, fmt.Errorf("derive public key from private key: %w", err)
	}
	if strings.TrimSpace(publicKey) != derivedPublicKey {
		slog.Error("device keypair mismatch", "generated_public_key", publicKey, "derived_public_key", derivedPublicKey)
		return nil, fmt.Errorf("generated keypair mismatch: derived public key does not match generated public key")
	}
	publicKey = derivedPublicKey

	activeNodes, err := uc.Nodes.ListActive(ctx)
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", err)
		return nil, err
	}

	selectedNode, err := uc.NodeAssigner.Assign(activeNodes)
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "error", err)
		return nil, err
	}

	assignedIP, err := uc.IPAllocator.Allocate(ctx, selectedNode.ID)
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "node_id", selectedNode.ID, "error", err)
		return nil, err
	}
	presharedKey, err := uc.generatePresharedKey(ctx)
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "node_id", selectedNode.ID, "error", err)
		return nil, err
	}
	presharedForPayload := presharedKey
	if uc.SensitiveEncryptor != nil {
		encryptedPSK, encErr := uc.SensitiveEncryptor.Encrypt([]byte(presharedKey))
		if encErr != nil {
			return nil, encErr
		}
		presharedForPayload = "enc:v1:" + encodeBase64(encryptedPSK)
	}

	createdDevice, err := uc.Devices.Create(ctx, device.CreateParams{
		UserID:    in.UserID,
		VPNNodeID: &selectedNode.ID,
		PublicKey: publicKey,
		Name:      in.Name,
		Platform:  in.Platform,
		Status:    device.StatusActive,
	})
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "device_id", createdDevice.ID, "error", err)
		return nil, err
	}
	slog.Info("device created", "user_id", in.UserID, "device_id", createdDevice.ID, "node_id", selectedNode.ID)

	createdAccess, err := uc.Accesses.Create(ctx, access.CreateParams{
		DeviceID:   createdDevice.ID,
		VPNNodeID:  selectedNode.ID,
		Protocol:   "wireguard",
		Status:     access.StatusPending,
		AssignedIP: &assignedIP,
	})
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "device_id", createdDevice.ID, "error", err)
		return nil, err
	}
	slog.Info("device access created", "device_id", createdDevice.ID, "access_id", createdAccess.ID, "assigned_ip", assignedIP)
	createdXrayAccess, err := uc.Accesses.Create(ctx, access.CreateParams{
		DeviceID:   createdDevice.ID,
		VPNNodeID:  selectedNode.ID,
		Protocol:   "xray",
		Status:     access.StatusPending,
		AssignedIP: &assignedIP,
	})
	if err != nil {
		slog.Error("create_device_for_user.error", "user_id", in.UserID, "device_id", createdDevice.ID, "error", err)
		return nil, err
	}
	xrayUserUUID := hashToken(createdXrayAccess.ID)[:32]

	payload, _ := json.Marshal(map[string]any{
		"device_id":     createdDevice.ID,
		"access_id":     createdAccess.ID,
		"public_key":    publicKey,
		"assigned_ip":   assignedIP,
		"protocol":      "wireguard",
		"keepalive":     25,
		"preshared_key": presharedForPayload,
	})

	op, err := uc.Operations.Create(ctx, operation.CreateParams{
		VPNNodeID:     selectedNode.ID,
		OperationType: operation.TypeCreatePeer,
		Status:        operation.StatusQueued,
		PayloadJSON:   string(payload),
	})
	if err != nil {
		return nil, err
	}

	if uc.CreatePeerExecutor != nil {
		if err := uc.CreatePeerExecutor.Execute(ctx, op.ID); err != nil {
			slog.Warn("wireguard peer create failed; continue with xray", "device_id", createdDevice.ID, "access_id", createdAccess.ID, "error", err)
		}
	}
	xrayPayload, _ := json.Marshal(map[string]any{
		"device_id":   createdDevice.ID,
		"access_id":   createdXrayAccess.ID,
		"public_key":  xrayUserUUID,
		"assigned_ip": assignedIP,
		"protocol":    "xray",
		"keepalive":   0,
	})
	xrayOp, err := uc.Operations.Create(ctx, operation.CreateParams{
		VPNNodeID:     selectedNode.ID,
		OperationType: operation.TypeCreatePeer,
		Status:        operation.StatusQueued,
		PayloadJSON:   string(xrayPayload),
	})
	if err != nil {
		return nil, err
	}
	if uc.CreatePeerExecutor != nil {
		if err := uc.CreatePeerExecutor.Execute(ctx, xrayOp.ID); err != nil {
			return nil, err
		}
	}

	details, _ := json.Marshal(map[string]any{
		"device_id":    createdDevice.ID,
		"node_id":      selectedNode.ID,
		"access_id":    createdAccess.ID,
		"operation_id": op.ID,
	})
	_, err = uc.AuditLogs.Create(ctx, audit.CreateParams{
		ActorUserID: &in.UserID,
		EntityType:  "device",
		EntityID:    &createdDevice.ID,
		Action:      "create_device_for_user",
		DetailsJSON: string(details),
	})
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	endpoint := valueOrDefault(uc.PublicEndpoint, selectedNode.VPNEndpoint)
	vpnHost, vpnPort := splitEndpointHostPort(endpoint)
	if selectedNode.VPNEndpointHost != "" {
		vpnHost = selectedNode.VPNEndpointHost
	}
	if selectedNode.VPNEndpointPort > 0 {
		vpnPort = selectedNode.VPNEndpointPort
	}
	serverPublicKey := valueOrDefault(uc.ServerPublicKey, selectedNode.ServerPublicKey)
	issuedToken := ""
	issuedTokens := map[string]string{}
	if uc.ConfigIssuer != nil {
		cfgOut, err := uc.ConfigIssuer.Execute(ctx, IssueDeviceConfigInput{
			DeviceAccessID:   createdAccess.ID,
			Protocol:         "wireguard",
			DevicePrivateKey: privateKey,
			DevicePublicKey:  publicKey,
			ServerPublicKey:  serverPublicKey,
			PresharedKey:     presharedKey,
			AssignedIP:       assignedIP,
			MTU:              uc.MTU,
			DNS:              uc.DNS,
			EndpointHost:     valueOrDefault(uc.EndpointHost, vpnHost),
			EndpointPort:     valueOrDefaultInt(uc.EndpointPort, vpnPort),
			Keepalive:        valueOrDefaultInt(uc.Keepalive, 25),
			AllowedIPs:       uc.ClientAllowedIPs,
			AWG:              uc.DefaultVPNAWG,
			TokenTTL:         uc.TokenTTL,
		})
		if err != nil {
			slog.Error("create_device_for_user.error", "user_id", in.UserID, "device_id", createdDevice.ID, "access_id", createdAccess.ID, "node_id", selectedNode.ID, "error", err)
			return nil, err
		}
		issuedToken = cfgOut.Token
		issuedTokens["wireguard"] = cfgOut.Token
		xrayCfgOut, xErr := uc.ConfigIssuer.Execute(ctx, IssueDeviceConfigInput{
			DeviceAccessID:  createdXrayAccess.ID,
			Protocol:        "xray",
			DevicePublicKey: publicKey,
			XrayUserUUID:    xrayUserUUID,
			EndpointHost:    valueOrDefault(uc.XrayPublicHost, valueOrDefault(uc.EndpointHost, vpnHost)),
			EndpointPort:    valueOrDefaultInt(uc.XrayRealityPort, valueOrDefaultInt(uc.EndpointPort, vpnPort)),
			XrayServerName:  uc.XrayRealitySNI,
			TokenTTL:        uc.TokenTTL,
		})
		if xErr == nil {
			issuedTokens["xray"] = xrayCfgOut.Token
		}
	}

	out := &CreateDeviceForUserOutput{
		Device:              createdDevice,
		Access:              createdAccess,
		AccessByProtocol:    map[string]*access.DeviceAccess{"wireguard": createdAccess, "xray": createdXrayAccess},
		PrivateKey:          privateKey,
		Node:                selectedNode,
		ConfigDownloadToken: issuedToken,
		ConfigTokens:        issuedTokens,
	}
	slog.Info("create_device_for_user.success", "user_id", in.UserID, "device_id", createdDevice.ID, "access_id", createdAccess.ID, "node_id", selectedNode.ID)
	return out, nil
}

func (uc CreateDeviceForUser) generatePresharedKey(ctx context.Context) (string, error) {
	if uc.PresharedKeys != nil {
		return uc.PresharedKeys.GeneratePresharedKey(ctx)
	}
	return "", errors.New("preshared key generator is required")
}

type AssignNodeForDeviceInput struct {
	DeviceID string
}

type AssignNodeForDevice struct {
	Devices      DeviceRepository
	Nodes        NodeRepository
	NodeAssigner NodeAssigner
}

func (uc AssignNodeForDevice) Execute(ctx context.Context, in AssignNodeForDeviceInput) (*node.Node, error) {
	nodes, err := uc.Nodes.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	selected, err := uc.NodeAssigner.Assign(nodes)
	if err != nil {
		return nil, err
	}
	if err := uc.Devices.AssignNode(ctx, in.DeviceID, selected.ID); err != nil {
		return nil, err
	}
	return selected, nil
}

type CreateDeviceAccessInput struct {
	DeviceID  string
	VPNNodeID string
}

type CreateDeviceAccess struct {
	Accesses    DeviceAccessRepository
	IPAllocator IPAllocator
}

func (uc CreateDeviceAccess) Execute(ctx context.Context, in CreateDeviceAccessInput) (*access.DeviceAccess, error) {
	ip, err := uc.IPAllocator.Allocate(ctx, in.VPNNodeID)
	if err != nil {
		return nil, err
	}
	return uc.Accesses.Create(ctx, access.CreateParams{
		DeviceID:   in.DeviceID,
		VPNNodeID:  in.VPNNodeID,
		Protocol:   "wireguard",
		Status:     access.StatusPending,
		AssignedIP: &ip,
	})
}

type RevokeDeviceAccessInput struct {
	AccessID string
}

type RevokeDeviceAccess struct {
	Accesses           DeviceAccessRepository
	Devices            DeviceRepository
	Operations         NodeOperationRepository
	AuditLogs          AuditLogRepository
	Tokens             ConfigDownloadTokenRepository
	RevokePeerExecutor *ExecuteRevokePeerOperation
}

func (uc RevokeDeviceAccess) Execute(ctx context.Context, in RevokeDeviceAccessInput) error {
	accessEntry, err := uc.Accesses.GetByID(ctx, in.AccessID)
	if err != nil {
		return err
	}

	peerPublicKey := ""
	if uc.Devices != nil {
		if d, dErr := uc.Devices.GetByID(ctx, accessEntry.DeviceID); dErr == nil && d != nil {
			peerPublicKey = d.PublicKey
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"access_id":        accessEntry.ID,
		"device_id":        accessEntry.DeviceID,
		"device_access_id": accessEntry.ID,
		"protocol":         accessEntry.Protocol,
		"peer_public_key":  peerPublicKey,
	})
	op, err := uc.Operations.Create(ctx, operation.CreateParams{
		VPNNodeID:     accessEntry.VPNNodeID,
		OperationType: operation.TypeRevokePeer,
		Status:        operation.StatusQueued,
		PayloadJSON:   string(payload),
	})
	if err != nil {
		return err
	}

	if uc.RevokePeerExecutor != nil {
		if err := uc.RevokePeerExecutor.Execute(ctx, op.ID, accessEntry.ID); err != nil {
			return err
		}
	}
	if uc.Tokens != nil {
		if err := uc.Tokens.RevokeIssuedByAccessID(ctx, accessEntry.ID, time.Now().UTC()); err != nil {
			return err
		}
	}
	if err := uc.Accesses.ClearConfigBlobEncrypted(ctx, accessEntry.ID); err != nil {
		return err
	}

	details, _ := json.Marshal(map[string]any{"access_id": accessEntry.ID, "operation_id": op.ID})
	_, err = uc.AuditLogs.Create(ctx, audit.CreateParams{
		EntityType:  "device_access",
		EntityID:    &accessEntry.ID,
		Action:      "revoke_device_access",
		DetailsJSON: string(details),
	})
	return err
}

type ListUserDevicesInput struct {
	UserID string
}

type ListUserDevices struct {
	Devices DeviceRepository
}

func (uc ListUserDevices) Execute(ctx context.Context, in ListUserDevicesInput) ([]*device.Device, error) {
	return uc.Devices.ListByUserID(ctx, in.UserID)
}

func splitEndpointHostPort(vpnEndpoint string) (string, int) {
	vpnEndpoint = strings.TrimSpace(vpnEndpoint)
	if vpnEndpoint == "" {
		return "", 51820
	}
	host, portRaw, err := net.SplitHostPort(vpnEndpoint)
	if err != nil {
		if strings.Count(vpnEndpoint, ":") == 1 {
			parts := strings.SplitN(vpnEndpoint, ":", 2)
			host = parts[0]
			portRaw = parts[1]
		} else {
			return vpnEndpoint, 51820
		}
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil || port <= 0 {
		port = 51820
	}
	return host, port
}

func encodeBase64(v []byte) string {
	return base64.StdEncoding.EncodeToString(v)
}
