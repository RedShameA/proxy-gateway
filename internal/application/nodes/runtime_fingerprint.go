package nodes

import "strings"

func RuntimeFingerprint(record Record) string {
	outboundJSON := strings.TrimSpace(record.OutboundJSON)
	if outboundJSON == "" {
		var err error
		outboundJSON, err = NormalizeOutboundJSON(OutboundNode{
			Type:       record.Type,
			Server:     record.Server,
			ServerPort: record.ServerPort,
			Username:   record.Username,
			Password:   record.Password,
		})
		if err != nil {
			return ""
		}
	}
	return OutboundFingerprint(outboundJSON)
}
