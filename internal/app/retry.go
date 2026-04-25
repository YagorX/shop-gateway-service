package app

import (
	"context"
	"fmt"
	"time"
)

func retryWithExponentialBackoff(ctx context.Context, operation func() error, maxRetries int, initialInterval time.Duration, maxInterval time.Duration) error {

	for attempt := 0; attempt < maxRetries; attempt++ {

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := operation()
		if err == nil {
			return nil
		}

		// Спим только если есть ещё попытки
		if attempt < maxRetries-1 {
			// Вычисляем интервал с exponential backoff
			interval := initialInterval * time.Duration(1<<attempt)
			if interval > maxInterval {
				interval = maxInterval
			}
			time.Sleep(interval)
		}
	}

	return fmt.Errorf("operation failed after %d attempts", maxRetries)
}
