package app

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"net"
	"strings"

	apperrors "proxygateway/internal/application/apperrors"
	appmaintenance "proxygateway/internal/application/maintenance"
	applicationprofiles "proxygateway/internal/application/profiles"
	appproxy "proxygateway/internal/application/proxy"
	appreadmodel "proxygateway/internal/application/readmodel"
	domainprofile "proxygateway/internal/domain/profile"
)

const (
	profileErrorBadRequest = apperrors.KindBadRequest
	profileErrorNotFound   = apperrors.KindNotFound
	profileErrorConflict   = apperrors.KindConflict
	profileErrorInternal   = apperrors.KindInternal
)

func newProfileOperationError(kind, message string, err error) error {
	return apperrors.New(kind, message, err)
}

func (g *Gateway) createAccessProfile(req accessProfilePatchRequest) (applicationprofiles.Summary, error) {
	service := g.profileManagementService()
	result, err := service.Create(context.Background(), req)
	if err != nil {
		if validationErr, ok := profileConfigValidationOperationError(err); ok {
			return applicationprofiles.Summary{}, newProfileOperationError(profileErrorBadRequest, validationErr.Error(), validationErr)
		}
		if errors.Is(err, applicationprofiles.ErrConfigIDGeneration) {
			return applicationprofiles.Summary{}, newProfileOperationError(profileErrorInternal, "create profile id", err)
		}
		return applicationprofiles.Summary{}, newProfileOperationError(profileErrorInternal, "create access profile", err)
	}
	cfg := result.Config
	if result.EnqueueEvaluation {
		_, _ = g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, appmaintenance.TriggerAccessProfileChange, 1, true)
	}
	if result.EnqueueUnknownCountryObservation {
		g.enqueueUnknownCountryObservations(cfg.CandidateFilter())
	}
	g.triggerServiceOutboundSync("profile_create")
	return service.BuildSummary(context.Background(), cfg), nil
}

func (g *Gateway) listAccessProfiles(limit, offset int) (any, error) {
	list, err := g.profileManagementService().List(context.Background(), applicationprofiles.ListConfigFilter{Limit: limit, Offset: offset})
	if err != nil {
		return nil, newProfileOperationError(profileErrorInternal, "list access profiles", err)
	}
	return list, nil
}

func (g *Gateway) listAccessProfilesSummary() []applicationprofiles.Summary {
	profiles, err := g.profileManagementService().ListSummaries(context.Background(), applicationprofiles.ListConfigFilter{Limit: 20})
	if err != nil {
		return []applicationprofiles.Summary{}
	}
	if profiles == nil {
		profiles = []applicationprofiles.Summary{}
	}
	return profiles
}

func (g *Gateway) createAccessProfileCredential(profileID, remarkInput, password, endpoint string) (any, error) {
	result, err := g.profileManagementService().CreateCredential(context.Background(), applicationprofiles.CreateCredentialCommand{
		ProfileID: profileID,
		Remark:    remarkInput,
		Password:  password,
		Endpoint:  endpoint,
	})
	if err != nil {
		return nil, profileCredentialCreateOperationError(err)
	}
	return result, nil
}

func (g *Gateway) listAccessProfileCredentials(profileID, endpoint string) (any, error) {
	result, err := g.profileManagementService().ListCredentials(context.Background(), profileID, endpoint)
	if err != nil {
		return nil, profileCredentialReadOperationError(err, "list proxy credentials")
	}
	return result, nil
}

func (g *Gateway) patchAccessProfileCredential(profileID, credentialID string, enabled bool) (map[string]bool, error) {
	result, err := g.profileManagementService().SetCredentialEnabled(context.Background(), profileID, credentialID, enabled)
	if err != nil {
		return nil, profileCredentialReadOperationError(err, "update proxy credential")
	}
	return map[string]bool{"updated": result.Updated}, nil
}

func (g *Gateway) deleteAccessProfileCredential(profileID, credentialID string) (map[string]bool, error) {
	result, err := g.profileManagementService().DeleteCredential(context.Background(), profileID, credentialID)
	if err != nil {
		return nil, profileCredentialReadOperationError(err, "delete proxy credential")
	}
	return map[string]bool{"deleted": result.Deleted}, nil
}

func profileCredentialCreateOperationError(err error) error {
	if errors.Is(err, domainprofile.ErrCredentialRemarkRequired) {
		return newProfileOperationError(profileErrorBadRequest, validationProxyCredentialRemarkRequired, err)
	}
	if errors.Is(err, domainprofile.ErrCredentialPasswordLength) {
		return newProfileOperationError(profileErrorBadRequest, validationProxyCredentialPasswordLength, err)
	}
	if errors.Is(err, domainprofile.ErrCredentialPasswordCharset) {
		return newProfileOperationError(profileErrorBadRequest, validationProxyCredentialPasswordCharset, err)
	}
	if errors.Is(err, applicationprofiles.ErrProfileNotFound) {
		return newProfileOperationError(profileErrorBadRequest, "access profile not found", err)
	}
	if errors.Is(err, applicationprofiles.ErrDuplicateCredential) {
		return newProfileOperationError(profileErrorConflict, validationProxyCredentialPasswordDuplicate, err)
	}
	return newProfileOperationError(profileErrorInternal, "create proxy credential", err)
}

func profileCredentialReadOperationError(err error, internalMessage string) error {
	if errors.Is(err, applicationprofiles.ErrProfileNotFound) {
		return newProfileOperationError(profileErrorNotFound, "access profile not found", err)
	}
	if errors.Is(err, applicationprofiles.ErrCredentialNotFound) {
		return newProfileOperationError(profileErrorNotFound, "proxy credential not found", err)
	}
	return newProfileOperationError(profileErrorInternal, internalMessage, err)
}

func (g *Gateway) deleteAccessProfile(profileID string) (map[string]bool, error) {
	result, err := g.profileManagementService().DeleteProfile(context.Background(), profileID)
	if errors.Is(err, applicationprofiles.ErrProfileNotFound) {
		return nil, newProfileOperationError(profileErrorNotFound, "access profile not found", err)
	}
	if err != nil {
		return nil, newProfileOperationError(profileErrorInternal, "delete access profile", err)
	}
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	g.triggerServiceOutboundSync("profile_delete")
	return map[string]bool{"deleted": true}, nil
}

func (g *Gateway) patchAccessProfile(profileID string, req accessProfilePatchRequest) (map[string]bool, error) {
	if req.IsEmpty() {
		return nil, newProfileOperationError(profileErrorBadRequest, validationAccessProfilePatchRequired, nil)
	}
	result, err := g.profileManagementService().Update(context.Background(), profileID, req)
	if err != nil {
		if errors.Is(err, applicationprofiles.ErrProfileNotFound) {
			return nil, newProfileOperationError(profileErrorNotFound, "access profile not found", err)
		}
		if validationErr, ok := profileConfigValidationOperationError(err); ok {
			return nil, newProfileOperationError(profileErrorBadRequest, validationErr.Error(), validationErr)
		}
		return nil, newProfileOperationError(profileErrorInternal, "update access profile", err)
	}
	cfg := result.Config
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	if result.EnqueueEvaluation {
		_, _ = g.enqueueProfileEvaluationRun(profileID, cfg.Name, appmaintenance.TriggerAccessProfileChange, cfg.ConfigVersion, true)
	}
	if result.EnqueueUnknownCountryObservation {
		g.enqueueUnknownCountryObservations(cfg.CandidateFilter())
	}
	g.triggerServiceOutboundSync("profile_update")
	return map[string]bool{"updated": true}, nil
}

type accessProfileConfig = applicationprofiles.ConfigRecord

type accessProfilePatchRequest = applicationprofiles.PatchRequest

func defaultAccessProfileConfig(id string) accessProfileConfig {
	return applicationprofiles.DefaultConfig(id)
}

func (g *Gateway) loadAccessProfileConfig(profileID string) (accessProfileConfig, error) {
	record, found, err := g.profileConfigRepo.LoadConfig(context.Background(), profileID)
	if err != nil {
		return accessProfileConfig{}, err
	}
	if !found {
		return accessProfileConfig{}, applicationprofiles.ErrProfileNotFound
	}
	record.ApplyDefaults()
	return record, nil
}

func normalizeChainEvaluationMode(mode string) string {
	return domainprofile.NormalizeChainEvaluationMode(mode)
}

func profileConfigValidationError(err error) error {
	validationErr, ok := profileConfigValidationOperationError(err)
	if ok {
		return validationErr
	}
	return err
}

func profileConfigValidationOperationError(err error) (error, bool) {
	switch {
	case errors.Is(err, applicationprofiles.ErrConfigNameRequired):
		return errors.New(validationAccessProfileNameRequired), true
	case errors.Is(err, applicationprofiles.ErrIdentifierDuplicate):
		return errors.New(validationProfileIdentifierDuplicate), true
	case errors.Is(err, domainprofile.ErrIdentifierLength):
		return errors.New(validationProfileIdentifierLength), true
	case errors.Is(err, domainprofile.ErrIdentifierCharset):
		return errors.New(validationProfileIdentifierCharset), true
	case errors.Is(err, domainprofile.ErrCandidateTimingNonNegative):
		return errors.New(validationCandidateTimingNonNegative), true
	case errors.Is(err, domainprofile.ErrEvaluationIntervalNonNegative):
		return errors.New(validationEvaluationIntervalNonNegative), true
	case errors.Is(err, domainprofile.ErrSwitchingToleranceNonNegative):
		return errors.New(validationSwitchingToleranceNonNegative), true
	case errors.Is(err, applicationprofiles.ErrNameIncludeRegexInvalid):
		return errors.New(validationNameIncludeRegexInvalid), true
	case errors.Is(err, applicationprofiles.ErrNameExcludeRegexInvalid):
		return errors.New(validationNameExcludeRegexInvalid), true
	case errors.Is(err, applicationprofiles.ErrEgressCountryModeInvalid):
		return errors.New(validationEgressCountryMode), true
	case errors.Is(err, applicationprofiles.ErrSelectedSourcesRequired):
		return errors.New(validationSelectedSourcesRequired), true
	case errors.Is(err, applicationprofiles.ErrFixedNodeRequired):
		return errors.New(validationFixedNodeRequired), true
	case errors.Is(err, applicationprofiles.ErrFixedNodeNotFound):
		return errors.New("fixed node not found"), true
	case errors.Is(err, domainprofile.ErrExitNodesRequired):
		return errors.New(validationExitNodesRequired), true
	case errors.Is(err, applicationprofiles.ErrExitNodeNotFound):
		return errors.New("exit node not found"), true
	case errors.Is(err, domainprofile.ErrChainLinkSingleExitRequired):
		return errors.New(validationChainLinkSingleExitRequired), true
	case errors.Is(err, applicationprofiles.ErrTestURLScheme):
		return errors.New(validationTestURLScheme), true
	case errors.Is(err, applicationprofiles.ErrTestURLHostRequired):
		return errors.New(validationTestURLHostRequired), true
	case errors.Is(err, applicationprofiles.ErrUnsupportedProfileType):
		return errors.New("unsupported access profile type"), true
	default:
		return nil, false
	}
}

func (g *Gateway) insertAccessProfileConfig(cfg accessProfileConfig) error {
	return g.profileConfigRepo.CreateConfig(context.Background(), cfg, unixMillisNow())
}

func (g *Gateway) proxyPathForCredential(credential proxyCredentialRecord) (selectedProxyPath, error) {
	path, err := g.proxyAccessService().SelectPath(context.Background(), credential)
	if errors.Is(err, appproxy.ErrAccessProfileConfigNotFound) {
		return selectedProxyPath{}, errors.New("access profile not found")
	}
	return path, err
}

func (g *Gateway) profileWaitingForObservation(profileID string) bool {
	record, found, err := g.profileConfigRepo.LoadConfig(context.Background(), profileID)
	return err == nil && found && record.State == "waiting_observation"
}

func (g *Gateway) loadUsableNode(nodeID string) (nodeRecord, error) {
	return g.loadUsableNodeWithContext(context.Background(), nodeID)
}

func (g *Gateway) loadUsableNodeWithContext(ctx context.Context, nodeID string) (nodeRecord, error) {
	node, err := g.loadNodeWithContext(ctx, nodeID)
	if err != nil {
		return nodeRecord{}, err
	}
	if !node.Enabled {
		return nodeRecord{}, errors.New("node is disabled")
	}
	return node, nil
}

func (g *Gateway) usableNodes(nodes []nodeRecord) []nodeRecord {
	return g.usableNodesWithContext(context.Background(), nodes)
}

func (g *Gateway) usableNodesWithContext(ctx context.Context, nodes []nodeRecord) []nodeRecord {
	if ctx == nil {
		ctx = context.Background()
	}
	usable := make([]nodeRecord, 0, len(nodes))
	for _, node := range nodes {
		if node.Enabled && g.nodeUsableWithContext(ctx, node.ID) {
			usable = append(usable, node)
		}
	}
	return usable
}

func (g *Gateway) nodeUsable(nodeID string) bool {
	return g.nodeUsableWithContext(context.Background(), nodeID)
}

func (g *Gateway) nodeUsableWithContext(ctx context.Context, nodeID string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	observation, found, err := g.nodeRepo.LoadObservation(ctx, nodeID)
	return err == nil && found && observation.Usable
}

func (g *Gateway) nodeIDMatchesCandidateFilter(nodeID string, filter candidateFilter) bool {
	return g.nodeIDMatchesCandidateFilterWithContext(context.Background(), nodeID, filter)
}

func (g *Gateway) nodeIDMatchesCandidateFilterWithContext(ctx context.Context, nodeID string, filter candidateFilter) bool {
	nodes, err := g.candidateNodesWithContext(ctx, filter)
	if err != nil {
		return false
	}
	return nodeIDInRecords(nodeID, nodes)
}

func (g *Gateway) profileNodeMatchesCandidateFilter(profileID, nodeID string, filter candidateFilter) bool {
	return g.profileNodeMatchesCandidateFilterWithContext(context.Background(), profileID, nodeID, filter)
}

func (g *Gateway) profileNodeMatchesCandidateFilterWithContext(ctx context.Context, profileID, nodeID string, filter candidateFilter) bool {
	return g.nodeIDMatchesCandidateFilterWithContext(ctx, nodeID, filter) || g.profileRetainsNodeWithContext(ctx, profileID, nodeID)
}

func (g *Gateway) chainPathMatchesProfile(cfg accessProfileConfig, frontNodeID, exitNodeID string) bool {
	return g.chainPathMatchesProfileWithContext(context.Background(), cfg, frontNodeID, exitNodeID)
}

func (g *Gateway) chainPathMatchesProfileWithContext(ctx context.Context, cfg accessProfileConfig, frontNodeID, exitNodeID string) bool {
	if !stringInSlice(exitNodeID, cfg.ExitNodeIDs) {
		return false
	}
	nodes, err := g.candidateNodesWithContext(ctx, cfg.CandidateFilter())
	if err != nil {
		return g.profileRetainsNodeWithContext(ctx, cfg.ID, frontNodeID)
	}
	nodes = excludeNodes(nodes, cfg.ExitNodeIDs)
	return nodeIDInRecords(frontNodeID, nodes) || g.profileRetainsNodeWithContext(ctx, cfg.ID, frontNodeID)
}

func (g *Gateway) unknownCountryCandidateCount(filter candidateFilter) int {
	filter.EgressCountry = ""
	filter.EgressCountries = nil
	nodes, err := g.candidateNodes(filter)
	if err != nil {
		return 0
	}
	count := 0
	for _, node := range nodes {
		observation, found, err := g.nodeRepo.LoadObservation(context.Background(), node.ID)
		if err != nil || !found || !observation.Usable || strings.TrimSpace(observation.EgressCountry) == "" {
			count++
		}
	}
	return count
}

func (g *Gateway) enqueueUnknownCountryObservations(filter candidateFilter) {
	filter.EgressCountry = ""
	filter.EgressCountries = nil
	nodes, err := g.candidateNodes(filter)
	if err != nil {
		return
	}
	settings, _ := g.loadMaintenanceSettings()
	probeURL := settings.EgressIPProbeURL
	if probeURL == "" {
		probeURL = defaultEgressIPProbeURL
	}
	var targets []nodeRecord
	for _, node := range nodes {
		observation, found, err := g.nodeRepo.LoadObservation(context.Background(), node.ID)
		if err == nil && found && observation.Usable && strings.TrimSpace(observation.EgressCountry) != "" {
			continue
		}
		targets = append(targets, node)
	}
	if len(targets) > 0 {
		_, _ = g.createNodeObservationRun("country_profile_unknown_country", "all_nodes", targets, probeURL)
		g.notifyMaintenanceRunner()
	}
}

func cryptoRandomIndex(n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("empty random range")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

func (g *Gateway) accessProfileDetail(profileID, endpoint string) (applicationprofiles.Detail, error) {
	detail, err := g.profileManagementService().LoadDetail(context.Background(), profileID, endpoint)
	if err != nil {
		return applicationprofiles.Detail{}, newProfileOperationError(profileErrorNotFound, "access profile not found", err)
	}
	return detail, nil
}

func parseJSONObject(raw string) map[string]any {
	return appreadmodel.ParseJSONObject(raw)
}

func (g *Gateway) proxyEndpointForHost(host string) string {
	ep := strings.TrimSpace(g.getKVSetting("public_proxy_endpoint"))
	if ep != "" {
		return ep
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		if h, p, err := net.SplitHostPort(host); err == nil {
			if strings.Contains(h, ":") {
				return "[" + h + "]:" + p
			}
		}
	}
	return host
}

func (g *Gateway) runAccessProfileAction(profileID string, action string) (map[string]any, error) {
	cfg, err := g.loadAccessProfileConfig(profileID)
	if err != nil {
		return nil, newProfileOperationError(profileErrorNotFound, "access profile not found", err)
	}
	plan, err := applicationprofiles.BuildActionPlan(cfg, action)
	if err != nil {
		switch {
		case errors.Is(err, applicationprofiles.ErrProfileTypeNotEvaluable):
			return nil, newProfileOperationError(profileErrorBadRequest, appmaintenance.ReasonProfileTypeNotEvaluable, err)
		case errors.Is(err, applicationprofiles.ErrNoCurrentPathToSwitch):
			return nil, newProfileOperationError(profileErrorConflict, "no current path to switch from", err)
		default:
			return nil, newProfileOperationError(profileErrorNotFound, "unknown action", err)
		}
	}
	if !plan.CreateSwitchRun {
		runID, err := g.enqueueProfileEvaluationRun(profileID, cfg.Name, appmaintenance.TriggerManual, cfg.ConfigVersion, true)
		if err != nil {
			return nil, newProfileOperationError(profileErrorInternal, "enqueue evaluation", err)
		}
		return map[string]any{"run_id": runID, "state": plan.ResponseState}, nil
	}
	run, err := g.createMaintenanceRun(maintenanceRunTypeProfileSwitch, appmaintenance.TriggerManual, profileID, cfg.Name, 1, plan.SwitchRunDetail)
	if err != nil {
		return nil, newProfileOperationError(profileErrorInternal, "create profile switch run", err)
	}
	detail := maintenanceRunDetail(run)
	detail["switch_reason"] = plan.SwitchReason
	if err := g.finishMaintenanceRun(run.ID, maintenanceRunResultSuccess, plan.SwitchReason, 1, detail, ""); err != nil {
		return nil, newProfileOperationError(profileErrorInternal, "finish profile switch run", err)
	}
	_, _ = g.enqueueProfileEvaluationRun(profileID, cfg.Name, appmaintenance.TriggerManual, cfg.ConfigVersion, true)
	return map[string]any{"run_id": run.ID, "state": plan.ResponseState}, nil
}
