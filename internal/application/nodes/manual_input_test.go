package nodes

import (
	"errors"
	"testing"
)

func TestBuildStructuredManualNodeNormalizesFields(t *testing.T) {
	name := "  manual  "
	nodeType := " socks "
	server := " 127.0.0.1 "
	port := 1080
	username := "user"
	password := "pass"

	node, err := BuildStructuredManualNode(StructuredManualInput{
		Name:       &name,
		Type:       &nodeType,
		Server:     &server,
		ServerPort: &port,
		Username:   &username,
		Password:   &password,
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Name != "manual" || node.Type != "socks5" || node.Server != "127.0.0.1" || node.ServerPort != 1080 {
		t.Fatalf("node fields = %#v", node)
	}
	if node.Username != "user" || node.Password != "pass" {
		t.Fatalf("credentials = %#v", node)
	}
}

func TestBuildStructuredManualNodeValidatesRequiredFields(t *testing.T) {
	_, err := BuildStructuredManualNode(StructuredManualInput{})

	if !errors.Is(err, ErrManualNodeFieldsRequired) {
		t.Fatalf("err = %v, want ErrManualNodeFieldsRequired", err)
	}
}

func TestBuildStructuredManualNodeValidatesSupportedTypeAndEndpoint(t *testing.T) {
	name := "manual"
	nodeType := "vmess"
	server := "127.0.0.1"
	port := 1080

	_, err := BuildStructuredManualNode(StructuredManualInput{Name: &name, Type: &nodeType, Server: &server, ServerPort: &port})
	if !errors.Is(err, ErrManualNodeTypeUnsupported) {
		t.Fatalf("err = %v, want ErrManualNodeTypeUnsupported", err)
	}

	nodeType = "http"
	port = 70000
	_, err = BuildStructuredManualNode(StructuredManualInput{Name: &name, Type: &nodeType, Server: &server, ServerPort: &port})
	if !errors.Is(err, ErrManualNodeEndpointInvalid) {
		t.Fatalf("err = %v, want ErrManualNodeEndpointInvalid", err)
	}
}
