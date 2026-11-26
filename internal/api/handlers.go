package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	maxRequestBodyBytes = 1 << 20 // 1MB
	defaultPageLimit    = 50
	maxTargetsLimit     = 200
	maxCheckLogsLimit   = 500
	defaultTargetPeriod = 5 * time.Second
)

var defaultExpectedStatus = []int{200, 201, 204}

type TargetHandler struct {
	broker         *EventBroker
	targetRepo     database.TargetRepository
	checkLogRepo   database.CheckLogRepository
	defaultTimeout time.Duration
	onTargetChange func()
}

type CheckLogHandler struct {
	checkLogRepo database.CheckLogRepository
}

type TargetRequest struct {
	Name              string            `json:"name" validate:"required"`
	URL               string            `json:"url" validate:"required"`
	Method            string            `json:"method,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	Enabled           *bool             `json:"enabled,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	Body              map[string]any    `json:"body,omitempty"`
	Timeout           time.Duration     `json:"timeout,omitempty"`
	Interval          time.Duration     `json:"interval,omitempty"`
	ExpectedStatus    []int             `json:"expected_status,omitempty"`
	ExpectedBodyMatch string            `json:"expected_body_match,omitempty"`
}

type PatchTargetRequest struct {
	Name              *string            `json:"name,omitempty"`
	URL               *string            `json:"url,omitempty"`
	Method            *string            `json:"method,omitempty"`
	Headers           *map[string]string `json:"headers,omitempty"`
	Enabled           *bool              `json:"enabled,omitempty"`
	Tags              *[]string          `json:"tags,omitempty"`
	Body              *map[string]any    `json:"body,omitempty"`
	Timeout           *time.Duration     `json:"timeout,omitempty"`
	Interval          *time.Duration     `json:"interval,omitempty"`
	ExpectedStatus    *[]int             `json:"expected_status,omitempty"`
	ExpectedBodyMatch *string            `json:"expected_body_match,omitempty"`
}

func (handler *TargetHandler) notifyTargetChange() {
	if handler.onTargetChange != nil {
		handler.onTargetChange()
	}
}

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	SendJSON(w, http.StatusOK, HealthResponse{
		Status: "OK",
	})
}

func (handler *TargetHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if handler.broker == nil {
		SendError(w, http.StatusInternalServerError, "event broker not configured")
		return
	}

	flusher, ok := PrepareSSE(w)
	if !ok {
		slog.Error("Streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan domain.Event, 10)
	handler.broker.Register(clientChan)
	defer handler.broker.UnRegister(clientChan)

	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-clientChan:
			if !ok {
				return
			}
			data, err := FormatEvent(event)
			if err != nil {
				continue
			}
			slog.Debug("Event", "data", event)
			if _, err := w.Write(data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (handler *TargetHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendJSON(w, http.StatusOK, []domain.CheckResult{})
		return
	}

	targetPtrs, err := handler.targetRepo.FindEnabled(r.Context())
	if err != nil {
		SendError(w, http.StatusInternalServerError, "failed to load enabled targets")
		return
	}

	targets := make([]domain.Target, 0, len(targetPtrs))
	for _, t := range targetPtrs {
		if t != nil {
			targets = append(targets, *t)
		}
	}

	results, logs := checker.CheckTargets(r.Context(), targets, handler.defaultTimeout)
	if results == nil {
		results = []domain.CheckResult{}
	}
	if handler.checkLogRepo != nil && len(logs) > 0 {
		if err := handler.checkLogRepo.CreateBatch(r.Context(), logs); err != nil {
			slog.Error("Falha ao persistir check_logs em GetStatus", "error", err)
		}
	}

	SendJSON(w, http.StatusOK, results)
}

func (handler *TargetHandler) CreateTarget(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendError(w, http.StatusInternalServerError, "target repository not configured")
		return
	}

	var req TargetRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	target := &domain.Target{
		Name:              req.Name,
		URL:               req.URL,
		Method:            req.Method,
		Headers:           req.Headers,
		Enabled:           enabled,
		Tags:              req.Tags,
		Body:              req.Body,
		Timeout:           req.Timeout,
		Interval:          req.Interval,
		ExpectedStatus:    req.ExpectedStatus,
		ExpectedBodyMatch: req.ExpectedBodyMatch,
	}

	if err := validateAndNormalizeTarget(target); err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := handler.targetRepo.Create(r.Context(), target); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			SendError(w, http.StatusConflict, "target with this url already exists")
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			SendError(w, http.StatusBadRequest, err.Error())
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to create target")
		return
	}

	handler.notifyTargetChange()
	SendJSON(w, http.StatusCreated, target)
}

func (handler *TargetHandler) ListTargets(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendError(w, http.StatusInternalServerError, "target repository not configured")
		return
	}

	q := r.URL.Query()
	var filter database.TargetFilter

	if rawEnabled := strings.TrimSpace(q.Get("enabled")); rawEnabled != "" {
		enabledVal, err := strconv.ParseBool(rawEnabled)
		if err != nil {
			SendError(w, http.StatusBadRequest, "invalid enabled query parameter")
			return
		}
		filter.Enabled = &enabledVal
	}

	filter.Tag = strings.TrimSpace(q.Get("tag"))
	filter.Search = strings.TrimSpace(q.Get("search"))

	limit, offset, err := parsePagination(q, defaultPageLimit, maxTargetsLimit)
	if err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	targets, total, err := handler.targetRepo.FindAll(r.Context(), filter, limit, offset)
	if err != nil {
		SendError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	if targets == nil {
		targets = []*domain.Target{}
	}

	SendJSON(w, http.StatusOK, PaginatedTargetsResponse{
		Data: targets,
		Pagination: PaginationMeta{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	})
}

func (handler *TargetHandler) GetTarget(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendError(w, http.StatusInternalServerError, "target repository not configured")
		return
	}

	id, err := parseObjectIDParam(r, "id")
	if err != nil {
		SendError(w, http.StatusBadRequest, "invalid target id")
		return
	}

	target, err := handler.targetRepo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			SendError(w, http.StatusNotFound, "target not found")
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to fetch target")
		return
	}

	SendJSON(w, http.StatusOK, target)
}

func (handler *TargetHandler) UpdateTarget(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendError(w, http.StatusInternalServerError, "target repository not configured")
		return
	}

	id, err := parseObjectIDParam(r, "id")
	if err != nil {
		SendError(w, http.StatusBadRequest, "invalid target id")
		return
	}

	var req TargetRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	target := &domain.Target{
		Name:              req.Name,
		URL:               req.URL,
		Method:            req.Method,
		Headers:           req.Headers,
		Enabled:           enabled,
		Tags:              req.Tags,
		Body:              req.Body,
		Timeout:           req.Timeout,
		Interval:          req.Interval,
		ExpectedStatus:    req.ExpectedStatus,
		ExpectedBodyMatch: req.ExpectedBodyMatch,
	}

	if err := validateAndNormalizeTarget(target); err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := handler.targetRepo.Update(r.Context(), id, target); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			SendError(w, http.StatusNotFound, "target not found")
			return
		}
		if errors.Is(err, domain.ErrAlreadyExists) {
			SendError(w, http.StatusConflict, "target with this url already exists")
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			SendError(w, http.StatusBadRequest, err.Error())
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to update target")
		return
	}

	handler.notifyTargetChange()
	SendJSON(w, http.StatusOK, target)
}

func (handler *TargetHandler) PatchTarget(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendError(w, http.StatusInternalServerError, "target repository not configured")
		return
	}

	id, err := parseObjectIDParam(r, "id")
	if err != nil {
		SendError(w, http.StatusBadRequest, "invalid target id")
		return
	}

	var req PatchTargetRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := handler.targetRepo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			SendError(w, http.StatusNotFound, "target not found")
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to fetch target")
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.URL != nil {
		existing.URL = *req.URL
	}
	if req.Method != nil {
		existing.Method = *req.Method
	}
	if req.Headers != nil {
		existing.Headers = *req.Headers
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Tags != nil {
		existing.Tags = *req.Tags
	}
	if req.Body != nil {
		existing.Body = *req.Body
	}
	if req.Timeout != nil {
		existing.Timeout = *req.Timeout
	}
	if req.Interval != nil {
		existing.Interval = *req.Interval
	}
	if req.ExpectedStatus != nil {
		existing.ExpectedStatus = *req.ExpectedStatus
	}
	if req.ExpectedBodyMatch != nil {
		existing.ExpectedBodyMatch = *req.ExpectedBodyMatch
	}

	if err := validateAndNormalizeTarget(existing); err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := handler.targetRepo.Update(r.Context(), id, existing); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			SendError(w, http.StatusNotFound, "target not found")
			return
		}
		if errors.Is(err, domain.ErrAlreadyExists) {
			SendError(w, http.StatusConflict, "target with this url already exists")
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			SendError(w, http.StatusBadRequest, err.Error())
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to update target")
		return
	}

	handler.notifyTargetChange()
	SendJSON(w, http.StatusOK, existing)
}

func (handler *TargetHandler) DeleteTarget(w http.ResponseWriter, r *http.Request) {
	if handler.targetRepo == nil {
		SendError(w, http.StatusInternalServerError, "target repository not configured")
		return
	}

	id, err := parseObjectIDParam(r, "id")
	if err != nil {
		SendError(w, http.StatusBadRequest, "invalid target id")
		return
	}

	if err := handler.targetRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			SendError(w, http.StatusNotFound, "target not found")
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to delete target")
		return
	}

	handler.notifyTargetChange()
	w.WriteHeader(http.StatusNoContent)
}

func (handler *CheckLogHandler) ListCheckLogs(w http.ResponseWriter, r *http.Request) {
	if handler.checkLogRepo == nil {
		SendError(w, http.StatusInternalServerError, "check log repository not configured")
		return
	}

	q := r.URL.Query()
	var filter database.CheckLogFilter

	if rawTargetID := strings.TrimSpace(q.Get("target_id")); rawTargetID != "" {
		oid, err := bson.ObjectIDFromHex(rawTargetID)
		if err != nil {
			SendError(w, http.StatusBadRequest, "invalid target_id query parameter")
			return
		}
		filter.TargetID = &oid
	}

	if rawIsUp := strings.TrimSpace(q.Get("is_up")); rawIsUp != "" {
		isUpVal, err := strconv.ParseBool(rawIsUp)
		if err != nil {
			SendError(w, http.StatusBadRequest, "invalid is_up query parameter")
			return
		}
		filter.IsUp = &isUpVal
	}

	if rawStatus := strings.TrimSpace(q.Get("status_code")); rawStatus != "" {
		code, err := strconv.Atoi(rawStatus)
		if err != nil || code < 0 || code > 599 {
			SendError(w, http.StatusBadRequest, "invalid status_code query parameter")
			return
		}
		filter.StatusCode = &code
	}

	if rawFrom := strings.TrimSpace(q.Get("from")); rawFrom != "" {
		fromTime, err := time.Parse(time.RFC3339Nano, rawFrom)
		if err != nil {
			SendError(w, http.StatusBadRequest, "invalid from timestamp (expected RFC3339)")
			return
		}
		utcFrom := fromTime.UTC()
		filter.From = &utcFrom
	}

	if rawTo := strings.TrimSpace(q.Get("to")); rawTo != "" {
		toTime, err := time.Parse(time.RFC3339Nano, rawTo)
		if err != nil {
			SendError(w, http.StatusBadRequest, "invalid to timestamp (expected RFC3339)")
			return
		}
		utcTo := toTime.UTC()
		filter.To = &utcTo
	}

	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		SendError(w, http.StatusBadRequest, "from timestamp must be before or equal to to timestamp")
		return
	}

	filter.Search = strings.TrimSpace(q.Get("search"))

	limit, offset, err := parsePagination(q, defaultPageLimit, maxCheckLogsLimit)
	if err != nil {
		SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	logs, total, err := handler.checkLogRepo.FindAll(r.Context(), filter, limit, offset)
	if err != nil {
		SendError(w, http.StatusInternalServerError, "failed to list check logs")
		return
	}
	if logs == nil {
		logs = []domain.CheckLog{}
	}

	SendJSON(w, http.StatusOK, PaginatedCheckLogsResponse{
		Data: logs,
		Pagination: PaginationMeta{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	})
}

func (handler *CheckLogHandler) GetCheckLog(w http.ResponseWriter, r *http.Request) {
	if handler.checkLogRepo == nil {
		SendError(w, http.StatusInternalServerError, "check log repository not configured")
		return
	}

	id, err := parseObjectIDParam(r, "id")
	if err != nil {
		SendError(w, http.StatusBadRequest, "invalid check log id")
		return
	}

	logEntry, err := handler.checkLogRepo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			SendError(w, http.StatusNotFound, "check log not found")
			return
		}
		SendError(w, http.StatusInternalServerError, "failed to fetch check log")
		return
	}

	SendJSON(w, http.StatusOK, logEntry)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.New("invalid JSON payload")
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return errors.New("request body is required")
	}
	if trimmed[0] != '{' {
		return errors.New("request body must contain a single JSON object")
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return errors.New("invalid JSON payload")
	}

	return nil
}

func validateAndNormalizeTarget(target *domain.Target) error {
	target.Name = strings.TrimSpace(target.Name)
	if target.Name == "" {
		return errors.New("name is required")
	}

	target.URL = strings.TrimSpace(target.URL)
	if target.URL == "" {
		return errors.New("url is required")
	}
	if strings.ContainsAny(target.URL, " \t\r\n") {
		return errors.New("url must not contain whitespace")
	}

	parsedURL, err := url.Parse(target.URL)
	if err != nil || parsedURL.Host == "" || parsedURL.Hostname() == "" {
		return errors.New("url must be a valid http or https URL")
	}
	if strings.HasSuffix(parsedURL.Host, ":") {
		return errors.New("url must be a valid http or https URL")
	}
	if rawPort := parsedURL.Port(); rawPort != "" {
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return errors.New("url must be a valid http or https URL")
		}
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("url scheme must be http or https")
	}

	method := strings.ToUpper(strings.TrimSpace(target.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		return errors.New("method must be one of GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	}
	target.Method = method

	if target.Timeout < 0 {
		return errors.New("timeout must be non-negative")
	}
	if target.Timeout == 0 {
		target.Timeout = defaultTargetPeriod
	}

	if target.Interval < 0 {
		return errors.New("interval must be non-negative")
	}

	if len(target.ExpectedStatus) == 0 {
		target.ExpectedStatus = append([]int(nil), defaultExpectedStatus...)
	} else {
		for _, code := range target.ExpectedStatus {
			if code < 100 || code > 599 {
				return errors.New("expected_status codes must be between 100 and 599")
			}
		}
	}

	if target.Tags == nil {
		target.Tags = []string{}
	}

	if target.ExpectedBodyMatch != "" {
		if _, err := regexp.Compile(target.ExpectedBodyMatch); err != nil {
			return fmt.Errorf("expected_body_match must be a valid regular expression: %w", err)
		}
	}

	return nil
}

func parseObjectIDParam(r *http.Request, param string) (bson.ObjectID, error) {
	raw := strings.TrimSpace(chi.URLParam(r, param))
	return bson.ObjectIDFromHex(raw)
}

func parsePagination(q url.Values, defaultLimit, maxLimit int64) (int64, int64, error) {
	limit := defaultLimit
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsed, err := strconv.ParseInt(rawLimit, 10, 64)
		if err != nil || parsed <= 0 || parsed > maxLimit {
			return 0, 0, errors.New("limit must be a positive integer within allowed bounds")
		}
		limit = parsed
	}

	var offset int64
	if rawOffset := strings.TrimSpace(q.Get("offset")); rawOffset != "" {
		parsed, err := strconv.ParseInt(rawOffset, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
		offset = parsed
	}

	return limit, offset, nil
}
