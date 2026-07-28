package application

import (
	"context"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

type ProbeTarget struct {
	Name         string
	TransportURL string
	Tunnel       bool
}

type ProbeObservation struct {
	Success       bool
	HTTPStatus    int
	ErrorCategory operations.ErrorCategory
	Duration      time.Duration
	ObservedAt    time.Time
}

type ProbeClient interface {
	Probe(context.Context, ProbeTarget, string) ProbeObservation
}
