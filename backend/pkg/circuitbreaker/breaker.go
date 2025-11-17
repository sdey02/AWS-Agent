package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

type Config struct {
	MaxRequests       uint32
	Interval          time.Duration
	Timeout           time.Duration
	FailureThreshold  uint32
	SuccessThreshold  uint32
	OnStateChange     func(name string, from State, to State)
	Logger            *zap.Logger
}

type CircuitBreaker struct {
	name             string
	maxRequests      uint32
	interval         time.Duration
	timeout          time.Duration
	failureThreshold uint32
	successThreshold uint32
	onStateChange    func(name string, from State, to State)
	logger           *zap.Logger

	mu              sync.Mutex
	state           State
	generation      uint64
	counts          counts
	expiry          time.Time
}

type counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

func NewCircuitBreaker(name string, cfg Config) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:             name,
		maxRequests:      cfg.MaxRequests,
		interval:         cfg.Interval,
		timeout:          cfg.Timeout,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		onStateChange:    cfg.OnStateChange,
		logger:           cfg.Logger,
	}

	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}
	if cb.interval == 0 {
		cb.interval = time.Duration(0)
	}
	if cb.timeout == 0 {
		cb.timeout = 60 * time.Second
	}
	if cb.failureThreshold == 0 {
		cb.failureThreshold = 5
	}
	if cb.successThreshold == 0 {
		cb.successThreshold = 2
	}

	cb.toNewGeneration(time.Now())

	return cb
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn()
	cb.afterRequest(generation, err == nil)
	return err
}

func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.maxRequests {
		return generation, ErrTooManyRequests
	}

	cb.counts.Requests++
	return generation, nil
}

func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0

	if state == StateHalfOpen && cb.counts.ConsecutiveSuccesses >= cb.successThreshold {
		cb.setState(StateClosed, now)
	}
}

func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccesses = 0

	if state == StateClosed && cb.counts.ConsecutiveFailures >= cb.failureThreshold {
		cb.setState(StateOpen, now)
	} else if state == StateHalfOpen {
		cb.setState(StateOpen, now)
	}
}

func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}

	if cb.logger != nil {
		cb.logger.Info("Circuit breaker state changed",
			zap.String("name", cb.name),
			zap.String("from", prev.String()),
			zap.String("to", state.String()),
			zap.Uint32("failures", cb.counts.ConsecutiveFailures),
		)
	}
}

func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts = counts{}

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.timeout)
	default:
		cb.expiry = zero
	}
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

func (cb *CircuitBreaker) Counts() counts {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.counts
}
