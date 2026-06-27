package proxy

import (
	"strings"
	"time"

	"go.uber.org/zap"
)

type RequestLogIDGenerator func() (string, error)

type RequestLogSink interface {
	EnqueueStart(record RequestLogStartRecord) bool
	EnqueueFinish(record RequestLogFinishRecord) bool
	EnqueueFailure(record RequestLogFailureRecord) bool
}

type RequestLogService struct {
	writer RequestLogSink
	newID  RequestLogIDGenerator
	logger *zap.Logger
}

func NewRequestLogService(writer RequestLogSink, newID RequestLogIDGenerator, logger *zap.Logger) *RequestLogService {
	return &RequestLogService{
		writer: writer,
		newID:  newID,
		logger: ensureRequestLogLogger(logger),
	}
}

func (s *RequestLogService) Start(path SelectedPath, targetHost string, startedAt time.Time) string {
	if s == nil || s.newID == nil {
		return ""
	}
	id, err := s.newID()
	if err != nil {
		return ""
	}
	record := BuildRequestLogStart(RequestLogStartInput{
		ID:         id,
		Timestamp:  startedAt.UnixMilli(),
		TargetHost: targetHost,
		Credential: ProxyCredentialSnapshot{
			ID:     path.Credential.ID,
			Remark: path.Credential.Remark,
		},
		Profile: AccessProfileSnapshot{
			ID:         path.ProfileID,
			Name:       path.Profile,
			Identifier: path.ProfileIdentifier,
		},
		Path: RequestLogPathSnapshot(path),
	})
	if s.writer != nil {
		s.writer.EnqueueStart(record)
	}
	return id
}

func (s *RequestLogService) Finish(id string, success bool, failureStage string, errorText string, httpStatus int, durationMS, ingressBytes, egressBytes int64) {
	if id == "" {
		return
	}
	if s != nil && success {
		s.logger.Debug("proxy request completed",
			zap.String("log_id", id),
			zap.Int("http_status", httpStatus),
			zap.Int64("duration_ms", durationMS),
			zap.Int64("ingress_bytes", ingressBytes),
			zap.Int64("egress_bytes", egressBytes),
		)
	} else if s != nil {
		s.logger.Warn("proxy request failed",
			zap.String("log_id", id),
			zap.String("failure_stage", failureStage),
			zap.String("error", errorText),
			zap.Int("http_status", httpStatus),
			zap.Int64("duration_ms", durationMS),
		)
	}
	record := BuildRequestLogFinish(RequestLogFinishInput{
		ID:           id,
		Success:      success,
		FailureStage: failureStage,
		Error:        errorText,
		HTTPStatus:   httpStatus,
		DurationMS:   durationMS,
		IngressBytes: ingressBytes,
		EgressBytes:  egressBytes,
	})
	if s != nil && s.writer != nil {
		s.writer.EnqueueFinish(record)
	}
}

func (s *RequestLogService) RecordFailure(targetHost, profileIdentifier, failureStage, errorText string, httpStatus int, startedAt time.Time) {
	if strings.TrimSpace(targetHost) == "" || s == nil || s.newID == nil {
		return
	}
	id, err := s.newID()
	if err != nil {
		return
	}
	durationMS := requestLogElapsedMilliseconds(startedAt)
	s.logger.Warn("proxy request rejected",
		zap.String("log_id", id),
		zap.String("target_host", targetHost),
		zap.String("profile_identifier", profileIdentifier),
		zap.String("failure_stage", failureStage),
		zap.String("error", errorText),
		zap.Int("http_status", httpStatus),
		zap.Int64("duration_ms", durationMS),
	)
	record := BuildRequestLogFailure(RequestLogFailureInput{
		ID:                id,
		Timestamp:         startedAt.UnixMilli(),
		TargetHost:        targetHost,
		ProfileIdentifier: profileIdentifier,
		FailureStage:      failureStage,
		Error:             errorText,
		HTTPStatus:        httpStatus,
		DurationMS:        durationMS,
	})
	if s.writer != nil {
		s.writer.EnqueueFailure(record)
	}
}

func RequestLogPathSnapshot(path SelectedPath) ProxyPathSnapshot {
	return ProxyPathSnapshot{
		Node:      requestLogNodeSnapshot(path.Node),
		FrontNode: requestLogNodeSnapshot(path.FrontNode),
		ExitNode:  requestLogNodeSnapshot(path.ExitNode),
	}
}

func requestLogNodeSnapshot(node Node) NodeSnapshot {
	return NodeSnapshot{
		ID:         node.ID,
		Name:       node.Name,
		Protocol:   node.Type,
		Server:     node.Server,
		ServerPort: node.ServerPort,
	}
}

func requestLogElapsedMilliseconds(start time.Time) int64 {
	elapsed := time.Since(start).Milliseconds()
	if elapsed <= 0 {
		return 1
	}
	return elapsed
}
