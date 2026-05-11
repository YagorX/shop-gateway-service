package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const (
	idempotencyHeader       = "Idempotency-Key"
	idempotencyReplayHeader = "X-Idempotent-Replay"
)

type cachedResponse struct {
	RequestHash string      `json:"request_hash"`
	StatusCode  int         `json:"status_code"`
	Header      http.Header `json:"header"`
	Body        []byte      `json:"body"`
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if r.statusCode != 0 {
		return
	}
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func Idempotency(logger *slog.Logger, client *redis.Client, ttl time.Duration) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if client == nil || !isWriteMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			idempotencyKey := r.Header.Get(idempotencyHeader)
			if idempotencyKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "failed to read request body")
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))

			requestHash := hashParts(r.Method, r.URL.Path, string(body))
			cacheKey := "idempotency:v1:" + hashParts(
				r.Method,
				r.URL.Path,
				idempotencyKey,
				r.Header.Get("Authorization"),
				r.Header.Get(appIDHeader),
			)

			cached, err := getCachedResponse(r.Context(), client, cacheKey)
			if err == nil {
				if cached.RequestHash != requestHash {
					writeError(w, http.StatusUnprocessableEntity, "idempotency_key_conflict", "idempotency key was reused with a different request body")
					return
				}
				replayResponse(w, cached)
				return
			}
			if err != nil && err != redis.Nil {
				logger.Error("idempotency cache read failed",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("error", err.Error()),
				)
				writeError(w, http.StatusServiceUnavailable, "idempotency_unavailable", "idempotency check is unavailable")
				return
			}

			recorder := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			if recorder.statusCode >= http.StatusOK && recorder.statusCode < http.StatusMultipleChoices {
				if err := storeCachedResponse(r.Context(), client, cacheKey, ttl, cachedResponse{
					RequestHash: requestHash,
					StatusCode:  recorder.statusCode,
					Header:      recorder.Header().Clone(),
					Body:        recorder.body.Bytes(),
				}); err != nil {
					logger.Error("idempotency cache write failed",
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("error", err.Error()),
					)
				}
			}
		})
	}
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func getCachedResponse(ctx context.Context, client *redis.Client, key string) (cachedResponse, error) {
	raw, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return cachedResponse{}, err
	}

	var cached cachedResponse
	if err := json.Unmarshal(raw, &cached); err != nil {
		return cachedResponse{}, err
	}

	return cached, nil
}

func storeCachedResponse(ctx context.Context, client *redis.Client, key string, ttl time.Duration, cached cachedResponse) error {
	raw, err := json.Marshal(cached)
	if err != nil {
		return err
	}

	return client.Set(ctx, key, raw, ttl).Err()
}

func replayResponse(w http.ResponseWriter, cached cachedResponse) {
	for key, values := range cached.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set(idempotencyReplayHeader, "true")
	w.WriteHeader(cached.StatusCode)
	_, _ = w.Write(cached.Body)
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
