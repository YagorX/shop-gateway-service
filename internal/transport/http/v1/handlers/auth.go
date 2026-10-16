package handlers

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/YagorX/shop-gateway/internal/observability"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInfo.Reason, с которым auth отвечает на заблокированный после серии неудач логин.
// Задаётся в shop-auth/internal/transport/grpc/v1/server.go (reasonLoginLocked).
const authReasonLoginLocked = "LOGIN_LOCKED"

type AuthHandler struct {
	service contracts.AuthService
}

func NewAuthHandler(service contracts.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	EmailOrName string `json:"email_or_name"`
	Password    string `json:"password"`
	AppID       int64  `json:"app_id"`
	DeviceID    string `json:"device_id"`
}

type validateRequest struct {
	Token string `json:"token"`
	AppID int64  `json:"app_id"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	AppID        int64  `json:"app_id"`
	DeviceID     string `json:"device_id"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
	AppID        int64  `json:"app_id"`
	DeviceID     string `json:"device_id"`
}

type isAdminRequest struct {
	UserUUID string `json:"user_uuid"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	statusCode := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/auth/register").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/auth/register", statusCode).Inc()
	}()

	if r.Method != http.MethodPost {
		statusCode = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		statusCode = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "auth service is not initialized")
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	userUUID, err := h.service.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		statusCode = writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_uuid": userUUID,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	statusCode := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/auth/login").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/auth/login", statusCode).Inc()
	}()

	if r.Method != http.MethodPost {
		statusCode = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		statusCode = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "auth service is not initialized")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	accessToken, refreshToken, err := h.service.Login(
		r.Context(),
		req.EmailOrName,
		req.Password,
		req.AppID,
		req.DeviceID,
	)
	if err != nil {
		statusCode = writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *AuthHandler) Validate(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	statusCode := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/auth/validate").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/auth/validate", statusCode).Inc()
	}()

	if r.Method != http.MethodPost {
		statusCode = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		statusCode = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "auth service is not initialized")
		return
	}

	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	userUUID, err := h.service.ValidateToken(r.Context(), req.Token, req.AppID)
	if err != nil {
		statusCode = writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_uuid": userUUID,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	statusCode := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/auth/refresh").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/auth/refresh", statusCode).Inc()
	}()

	if r.Method != http.MethodPost {
		statusCode = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		statusCode = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "auth service is not initialized")
		return
	}

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	accessToken, newRefreshToken, err := h.service.Refresh(
		r.Context(),
		req.RefreshToken,
		req.AppID,
		req.DeviceID,
	)
	if err != nil {
		statusCode = writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	statusCode := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/auth/logout").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/auth/logout", statusCode).Inc()
	}()

	if r.Method != http.MethodPost {
		statusCode = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		statusCode = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "auth service is not initialized")
		return
	}

	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	if err := h.service.Logout(r.Context(), req.RefreshToken, req.AppID, req.DeviceID); err != nil {
		statusCode = writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

func (h *AuthHandler) IsAdmin(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	statusCode := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/auth/is-admin").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/auth/is-admin", statusCode).Inc()
	}()

	if r.Method != http.MethodPost {
		statusCode = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		statusCode = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "auth service is not initialized")
		return
	}

	var req isAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	isAdmin, err := h.service.IsAdmin(r.Context(), req.UserUUID)
	if err != nil {
		statusCode = writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"is_admin": isAdmin,
	})
}

func writeAuthError(w http.ResponseWriter, err error) string {
	if err == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return "500"
	}

	switch status.Code(err) {
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, "bad_request", upstreamMessage(err))
		return "400"
	case codes.AlreadyExists:
		writeError(w, http.StatusConflict, "already_exists", upstreamMessage(err))
		return "409"
	case codes.NotFound:
		writeError(w, http.StatusNotFound, "not_found", upstreamMessage(err))
		return "404"
	case codes.Unauthenticated:
		writeError(w, http.StatusUnauthorized, "unauthenticated", upstreamMessage(err))
		return "401"
	case codes.ResourceExhausted:
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(err)))
		if errorReason(err) == authReasonLoginLocked {
			writeError(w, http.StatusTooManyRequests, "login_locked", upstreamMessage(err))
		} else {
			writeError(w, http.StatusTooManyRequests, "too_many_requests", "server is busy, please try again later")
		}
		return "429"
	case codes.Unavailable:
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
		return "503"
	case codes.DeadlineExceeded:
		writeError(w, http.StatusGatewayTimeout, "upstream_timeout", "upstream timeout")
		return "504"
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return "500"
	}
}

// Retry-After в HTTP — целые секунды; округляем вверх, чтобы клиент не пришёл раньше, чем просили.
// Без RetryInfo всё равно отдаём 1: 429 без паузы провоцирует немедленный повтор.
func retryAfterSeconds(err error) int {
	if st, ok := status.FromError(err); ok {
		for _, d := range st.Details() {
			if ri, ok := d.(*errdetails.RetryInfo); ok {
				if s := int(math.Ceil(ri.GetRetryDelay().AsDuration().Seconds())); s > 0 {
					return s
				}
			}
		}
	}
	return 1
}

// Только текст, который auth сам предназначил клиенту. status.Convert тут не подходит:
// для обёрнутой ошибки grpc-go подставляет в Message весь текст обёртки вместе с op.
func upstreamMessage(err error) string {
	var se interface{ GRPCStatus() *status.Status }
	if errors.As(err, &se) {
		return se.GRPCStatus().Message()
	}
	return "internal error"
}

// Причина отказа из ErrorInfo: по ней одинаковый ResourceExhausted становится разными error.code.
func errorReason(err error) string {
	if st, ok := status.FromError(err); ok {
		for _, d := range st.Details() {
			if info, ok := d.(*errdetails.ErrorInfo); ok {
				return info.GetReason()
			}
		}
	}
	return ""
}
