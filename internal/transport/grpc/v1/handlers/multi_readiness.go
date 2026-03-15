package handlers

import (
	"context"
)

type ReadinessChecker interface {
	Check(ctx context.Context) error
}

type MultiReadinessChecker struct {
	Checkers []ReadinessChecker
}

func (m MultiReadinessChecker) Check(ctx context.Context) error {
	for _, checker := range m.Checkers {
		if checker == nil {
			continue
		}
		if err := checker.Check(ctx); err != nil {
			return err
		}
	}
	return nil
}
