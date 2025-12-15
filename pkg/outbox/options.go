package outbox

import (
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
)

type RelayOptions struct {
	PollInterval    time.Duration
	BatchSize       int
	LockTTL         time.Duration
	MaxAttempts     int
	SingleActive    bool
	MaxBackoff      time.Duration
	JitterMax       time.Duration
	LastErrorMaxLen int

	DispatchTimeout time.Duration

	Logger *logrus.Entry

	Rand *rand.Rand

	ObserveQueueDepthEvery time.Duration
}

func (o *RelayOptions) setDefaults() {
	if o.PollInterval == 0 {
		o.PollInterval = 1 * time.Second
	}
	if o.BatchSize == 0 {
		o.BatchSize = 100
	}
	if o.LockTTL == 0 {
		o.LockTTL = 60 * time.Second
	}
	if o.MaxAttempts == 0 {
		o.MaxAttempts = 25
	}
	if o.MaxBackoff == 0 {
		o.MaxBackoff = 60 * time.Second
	}
	if o.JitterMax == 0 {
		o.JitterMax = 200 * time.Millisecond
	}
	if o.LastErrorMaxLen == 0 {
		o.LastErrorMaxLen = 2048
	}
	if o.DispatchTimeout == 0 {
		o.DispatchTimeout = 30 * time.Second
	}
	if o.ObserveQueueDepthEvery == 0 {
		o.ObserveQueueDepthEvery = 10 * time.Second
	}
	if o.Rand == nil {
		o.Rand = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	}
}

type CleanerOptions struct {
	Enabled       bool
	Interval      time.Duration
	Retention     time.Duration
	DeadRetention time.Duration

	DeadAttemptsThreshold int

	Logger *logrus.Entry
}

func (o *CleanerOptions) setDefaults() {
	if o.Interval == 0 {
		o.Interval = 1 * time.Minute
	}
	if o.Retention == 0 {
		o.Retention = 7 * 24 * time.Hour
	}
}
