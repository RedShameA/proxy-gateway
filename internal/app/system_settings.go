package app

import "context"

const (
	defaultRequestLogRetentionDays         = 10
	defaultMaintenanceHistoryRetentionDays = 7
)

func boolKVSettingDefaultTrue(value string) bool {
	return value != "false"
}

func (g *Gateway) getKVSetting(key string) string {
	value, ok, err := g.kvSettingsRepo.Get(context.Background(), key)
	if err != nil || !ok {
		return ""
	}
	return value
}

func (g *Gateway) setKVSetting(key, value string) error {
	return g.kvSettingsRepo.Set(context.Background(), key, value)
}
