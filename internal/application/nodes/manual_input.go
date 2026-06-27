package nodes

import (
	"errors"
	"strings"
)

var (
	ErrManualNodeFieldsRequired  = errors.New("manual node fields required")
	ErrManualNodeNameRequired    = errors.New("manual node name required")
	ErrManualNodeTypeUnsupported = errors.New("manual node type unsupported")
	ErrManualNodeEndpointInvalid = errors.New("manual node endpoint invalid")
)

type StructuredManualInput struct {
	Name       *string
	Type       *string
	Server     *string
	ServerPort *int
	Username   *string
	Password   *string
}

func BuildStructuredManualNode(input StructuredManualInput) (OutboundNode, error) {
	if input.Name == nil || input.Type == nil || input.Server == nil || input.ServerPort == nil {
		return OutboundNode{}, ErrManualNodeFieldsRequired
	}
	name := strings.TrimSpace(*input.Name)
	nodeType := NormalizeNodeType(*input.Type)
	server := strings.TrimSpace(*input.Server)
	port := *input.ServerPort
	if name == "" {
		return OutboundNode{}, ErrManualNodeNameRequired
	}
	if nodeType != "http" && nodeType != "socks5" {
		return OutboundNode{}, ErrManualNodeTypeUnsupported
	}
	if server == "" || port <= 0 || port > 65535 {
		return OutboundNode{}, ErrManualNodeEndpointInvalid
	}
	username := ""
	if input.Username != nil {
		username = *input.Username
	}
	password := ""
	if input.Password != nil {
		password = *input.Password
	}
	return OutboundNode{
		Name:       name,
		Type:       nodeType,
		Server:     server,
		ServerPort: port,
		Username:   username,
		Password:   password,
	}, nil
}
