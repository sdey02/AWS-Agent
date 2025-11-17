package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	Multiplier      float64
	JitterFraction  float64
	RetryableErrors []error
	Logger          *zap.Logger
}

func DefaultConfig() Config {
	return Config{
		MaxAttempts:    3,
		InitialDelay:   100 * time.Millisecond,
		MaxDelay:       10 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.1,
		Logger:         zap.NewNop(),
	}
}

func Do(ctx context.Context, cfg Config, operation func() error) error {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay == 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = 10 * time.Second
	}
	if cfg.Multiplier == 0 {
		cfg.Multiplier = 2.0
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := operation()
		if err == nil {
			if attempt > 1 && cfg.Logger != nil {
				cfg.Logger.Info("Operation succeeded after retry",
					zap.Int("attempt", attempt),
				)
			}
			return nil
		}

		lastErr = err

		if !isRetryable(err, cfg.RetryableErrors) {
			if cfg.Logger != nil {
				cfg.Logger.Debug("Error not retryable",
					zap.Error(err),
					zap.Int("attempt", attempt),
				)
			}
			return err
		}

		if attempt == cfg.MaxAttempts {
			break
		}

		if cfg.Logger != nil {
			cfg.Logger.Warn("Operation failed, retrying",
				zap.Error(err),
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", cfg.MaxAttempts),
				zap.Duration("delay", delay),
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(addJitter(delay, cfg.JitterFraction)):
		}

		delay = time.Duration(math.Min(float64(cfg.MaxDelay), float64(delay)*cfg.Multiplier))
	}

	return lastErr
}

func DoWithResult[T any](ctx context.Context, cfg Config, operation func() (T, error)) (T, error) {
	var result T
	err := Do(ctx, cfg, func() error {
		var err error
		result, err = operation()
		return err
	})
	return result, err
}

func isRetryable(err error, retryableErrors []error) bool {
	if len(retryableErrors) == 0 {
		return true
	}

	for _, retryableErr := range retryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}
	return false
}

func addJitter(duration time.Duration, jitterFraction float64) time.Duration {
	if jitterFraction <= 0 {
		return duration
	}

	jitter := time.Duration(rand.Float64() * float64(duration) * jitterFraction)
	if rand.Intn(2) == 0 {
		return duration - jitter
	}
	return duration + jitter
}
