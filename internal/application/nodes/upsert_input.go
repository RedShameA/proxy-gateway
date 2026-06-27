package nodes

type SourceInput struct {
	ID   string
	Name string
	Type string
}

func BuildUpsertInput(node OutboundNode, source SourceInput, nowMillis int64) (UpsertInput, error) {
	node.Type = NormalizeNodeType(node.Type)
	outboundJSON, err := NormalizeOutboundJSON(node)
	if err != nil {
		return UpsertInput{}, err
	}
	return UpsertInput{
		Fingerprint:  OutboundFingerprint(outboundJSON),
		Name:         node.Name,
		Type:         node.Type,
		Server:       node.Server,
		ServerPort:   node.ServerPort,
		Username:     node.Username,
		Password:     node.Password,
		RawJSON:      node.RawJSON,
		OutboundJSON: outboundJSON,
		SourceID:     source.ID,
		SourceName:   source.Name,
		SourceType:   source.Type,
		NowMillis:    nowMillis,
	}, nil
}

func BuildManualUpdateInput(nodeID string, node OutboundNode, enabled *bool, nowMillis int64) (ManualUpdateInput, error) {
	node.Type = NormalizeNodeType(node.Type)
	outboundJSON, err := NormalizeOutboundJSON(node)
	if err != nil {
		return ManualUpdateInput{}, err
	}
	return ManualUpdateInput{
		NodeID:       nodeID,
		Fingerprint:  OutboundFingerprint(outboundJSON),
		Name:         node.Name,
		Type:         node.Type,
		Server:       node.Server,
		ServerPort:   node.ServerPort,
		Username:     node.Username,
		Password:     node.Password,
		RawJSON:      node.RawJSON,
		OutboundJSON: outboundJSON,
		Enabled:      enabled,
		NowMillis:    nowMillis,
	}, nil
}
