package app

import (
	"net/http"

	apperrors "proxygateway/internal/application/apperrors"
	appgeoip "proxygateway/internal/application/geoip"
	appmaintenance "proxygateway/internal/application/maintenance"
	applicationnodes "proxygateway/internal/application/nodes"
	applicationprofiles "proxygateway/internal/application/profiles"
	"proxygateway/internal/interfaces/httpapi"
)

func (g *Gateway) httpAPIHandler() http.Handler {
	adminService := g.adminService()
	adminAuth := httpapi.AdminAuth(adminService)
	maintenanceRunService := g.maintenanceRunService()
	return httpapi.NewRouter(httpapi.RouterConfig{
		SetupStatus: httpapi.SetupStatusHandler{Admin: adminService, Build: CurrentBuildInfo()},
		AdminSetup:  httpapi.AdminSetupHandler{Admin: adminService},
		AdminLogin:  httpapi.AdminLoginHandler{Admin: adminService, Limiter: g.adminLogins},
		AdminMe:     httpapi.AdminMeHandler{Auth: adminAuth},
		Subscriptions: httpapi.SubscriptionsHandler{
			Auth: adminAuth,
			Repo: g.subscriptionRepo,
			Create: func(req httpapi.SubscriptionCreateRequest) (any, error) {
				return g.createSubscriptionSource(subscriptionCreateInput{
					Name:                       req.Name,
					SourceType:                 req.SourceType,
					URL:                        req.URL,
					Content:                    req.Content,
					AutoRefreshEnabled:         req.AutoRefreshEnabled,
					AutoRefreshIntervalSeconds: req.AutoRefreshIntervalSeconds,
				})
			},
		},
		SubscriptionSubroutes: httpapi.SubscriptionSubroutesHandler{
			Auth:              adminAuth,
			Repo:              g.subscriptionRepo,
			UpdateAutoRefresh: g.updateSubscriptionAutoRefresh,
			Delete:            g.deleteSubscriptionSource,
			Refresh: func(subscriptionID string) (any, error) {
				return g.refreshSubscriptionSource(subscriptionID)
			},
		},
		Nodes: httpapi.NodesHandler{
			Auth: adminAuth,
			List: func(filter applicationnodes.ListFilter) (any, error) {
				return g.listNodes(filter)
			},
			Create: func(req httpapi.NodeCreateRequest) (any, error) {
				return g.createNodeSource(nodeCreateInput{
					Name:       req.Name,
					Type:       req.Type,
					Server:     req.Server,
					ServerPort: req.ServerPort,
					Username:   req.Username,
					Password:   req.Password,
					RawJSON:    req.RawJSON,
					ImportText: req.ImportText,
				})
			},
		},
		NodeSubroutes: httpapi.NodeSubroutesHandler{
			Auth: adminAuth,
			Get: func(nodeID string) (any, error) {
				return g.nodeDetail(nodeID)
			},
			Patch: func(nodeID string, req httpapi.NodePatchRequest) (any, error) {
				return g.patchNodeSource(nodeID, nodePatchRequest{
					Enabled:    req.Enabled,
					Name:       req.Name,
					Type:       req.Type,
					Server:     req.Server,
					ServerPort: req.ServerPort,
					Username:   req.Username,
					Password:   req.Password,
					ImportText: req.ImportText,
				})
			},
			Delete: g.deleteManualNodeSource,
		},
		RunNodeObservations: httpapi.RunNodeObservationsHandler{
			Auth: adminAuth,
			Run: func(req httpapi.NodeObservationRunRequest) (httpapi.NodeObservationRunResult, error) {
				result, err := g.runManualNodeObservations(manualNodeObservationRequest{
					TestURL:  req.TestURL,
					ProbeURL: req.ProbeURL,
					NodeID:   req.NodeID,
					NodeIDs:  req.NodeIDs,
				})
				if err != nil {
					return httpapi.NodeObservationRunResult{}, err
				}
				return httpapi.NodeObservationRunResult{ObservedNodes: result.ObservedNodes, RunID: result.RunID}, nil
			},
		},
		AccessProfiles: httpapi.AccessProfilesHandler{
			Auth: adminAuth,
			Create: func(req applicationprofiles.PatchRequest) (any, error) {
				return g.createAccessProfile(req)
			},
			List: func(limit, offset int) (any, error) {
				return g.listAccessProfiles(limit, offset)
			},
		},
		AccessProfileSubroutes: httpapi.AccessProfileSubroutesHandler{
			Auth:     adminAuth,
			Endpoint: g.proxyEndpointForHost,
			Get: func(profileID, endpoint string) (any, error) {
				return g.accessProfileDetail(profileID, endpoint)
			},
			Patch: func(profileID string, req applicationprofiles.PatchRequest) (any, error) {
				return g.patchAccessProfile(profileID, req)
			},
			Delete: func(profileID string) (any, error) {
				return g.deleteAccessProfile(profileID)
			},
			ListCredentials: func(profileID, endpoint string) (any, error) {
				return g.listAccessProfileCredentials(profileID, endpoint)
			},
			CreateCredential: func(profileID string, req httpapi.ProfileCredentialCreateRequest, endpoint string) (any, error) {
				return g.createAccessProfileCredential(profileID, req.Remark, req.Password, endpoint)
			},
			PatchCredential: func(profileID, credentialID string, enabled bool) (any, error) {
				return g.patchAccessProfileCredential(profileID, credentialID, enabled)
			},
			DeleteCredential: func(profileID, credentialID string) (any, error) {
				return g.deleteAccessProfileCredential(profileID, credentialID)
			},
			RunAction: func(profileID, action string) (any, error) {
				return g.runAccessProfileAction(profileID, action)
			},
		},
		EvaluationSettings:   httpapi.EvaluationSettingsHandler{Auth: adminAuth, Repo: g.systemSettingsRepo},
		MaintenanceRuns:      httpapi.MaintenanceRunsHandler{Auth: adminAuth, Service: maintenanceRunService},
		MaintenanceRunDetail: httpapi.MaintenanceRunDetailHandler{Auth: adminAuth, Service: maintenanceRunService},
		MaintenanceSettings:  httpapi.MaintenanceSettingsHandler{Auth: adminAuth, Repo: g.systemSettingsRepo},
		GeoIP: httpapi.GeoIPHandler{
			Auth:   adminAuth,
			Status: func() any { return g.geoIPStatus() },
			Update: func() (httpapi.GeoIPUpdateResult, error) {
				run, err := g.createMaintenanceRun(maintenanceTaskGeoIPUpdate, appmaintenance.TriggerManual, "", "", 1, map[string]any{"source": appgeoip.SourceMetaCubeX})
				if err != nil {
					return httpapi.GeoIPUpdateResult{}, apperrors.New(apperrors.KindInternal, "create geoip update run", err)
				}
				if err := g.runGeoIPUpdateMaintenanceRun(run.ID); err != nil {
					return httpapi.GeoIPUpdateResult{}, apperrors.New(apperrors.KindBadGateway, err.Error(), err)
				}
				return httpapi.GeoIPUpdateResult{RunID: run.ID, GeoIP: g.geoIPStatus()}, nil
			},
		},
		RunEvaluations: httpapi.RunEvaluationsHandler{Auth: adminAuth, Run: g.runManualProfileEvaluations},
		RequestLogs:    httpapi.RequestLogsHandler{Auth: adminAuth, Repo: g.requestLogRepo, Now: unixMillisNow},
		Overview: httpapi.OverviewHandler{
			Auth:           adminAuth,
			OverviewRepo:   g.overviewRepo,
			RequestLogRepo: g.requestLogRepo,
			Maintenance:    maintenanceRunService,
			AccessProfiles: func() any { return g.listAccessProfilesSummary() },
			GeoIPStatus:    func() any { return g.geoIPStatus() },
			Now:            unixMillisNow,
		},
		EgressCountries: httpapi.EgressCountriesHandler{Auth: adminAuth, Repo: g.dictionaryRepo},
		SystemSettings: httpapi.SystemSettingsHandler{
			Auth:        adminAuth,
			SystemRepo:  g.systemSettingsRepo,
			KVRepo:      g.kvSettingsRepo,
			GeoIPStatus: func() any { return g.geoIPStatus() },
		},
		AdminPassword: httpapi.AdminPasswordHandler{Auth: adminAuth, Admin: adminService},
		WebUI:         httpapi.WebUIHandler{},
	})
}
