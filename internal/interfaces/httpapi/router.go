package httpapi

import "net/http"

type RouterConfig struct {
	SetupStatus            SetupStatusHandler
	AdminSetup             AdminSetupHandler
	AdminLogin             AdminLoginHandler
	AdminMe                AdminMeHandler
	Subscriptions          SubscriptionsHandler
	SubscriptionSubroutes  SubscriptionSubroutesHandler
	Nodes                  NodesHandler
	NodeSubroutes          NodeSubroutesHandler
	RunNodeObservations    RunNodeObservationsHandler
	AccessProfiles         AccessProfilesHandler
	AccessProfileSubroutes AccessProfileSubroutesHandler
	EvaluationSettings     EvaluationSettingsHandler
	MaintenanceRuns        MaintenanceRunsHandler
	MaintenanceRunDetail   MaintenanceRunDetailHandler
	MaintenanceSettings    MaintenanceSettingsHandler
	GeoIP                  GeoIPHandler
	RunEvaluations         RunEvaluationsHandler
	RequestLogs            RequestLogsHandler
	Overview               OverviewHandler
	EgressCountries        EgressCountriesHandler
	SystemSettings         SystemSettingsHandler
	AdminPassword          AdminPasswordHandler
	WebUI                  WebUIHandler
}

func NewRouter(config RouterConfig) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/system/setup-status", config.SetupStatus)
	mux.Handle("/api/admin/setup", config.AdminSetup)
	mux.Handle("/api/admin/login", config.AdminLogin)
	mux.Handle("/api/admin/me", config.AdminMe)
	mux.Handle("/api/subscriptions", config.Subscriptions)
	mux.Handle("/api/subscriptions/", config.SubscriptionSubroutes)
	mux.Handle("/api/nodes", config.Nodes)
	mux.Handle("/api/nodes/", config.NodeSubroutes)
	mux.Handle("/api/nodes/observations/run", config.RunNodeObservations)
	mux.Handle("/api/access-profiles", config.AccessProfiles)
	mux.Handle("/api/access-profiles/", config.AccessProfileSubroutes)
	mux.Handle("/api/evaluation-settings", config.EvaluationSettings)
	mux.Handle("/api/maintenance/runs", config.MaintenanceRuns)
	mux.Handle("/api/maintenance/runs/", config.MaintenanceRunDetail)
	mux.Handle("/api/maintenance/settings", config.MaintenanceSettings)
	mux.Handle("/api/geoip", config.GeoIP)
	mux.Handle("/api/evaluations/run", config.RunEvaluations)
	mux.Handle("/api/request-logs", config.RequestLogs)
	mux.Handle("/api/overview", config.Overview)
	mux.Handle("/api/dictionaries/egress-countries", config.EgressCountries)
	mux.Handle("/api/system/settings", config.SystemSettings)
	mux.Handle("/api/admin/password", config.AdminPassword)
	mux.Handle("/", config.WebUI)
	return mux
}
