// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"sync"
	"time"
)

const (
	ruleOrderWait = 20 * time.Second
	ruleOrderPoll = 15 * time.Millisecond
)

type orderGate struct {
	place   sync.Mutex
	mu      sync.Mutex
	workers map[*orderWorker]struct{}
}

type orderWorker struct {
	ordered bool
	created bool
	atRest  bool
	written bool
}

var networkRuleOrderGates sync.Map

func networkRuleOrderGate(vnetID int) *orderGate {
	if gate, ok := networkRuleOrderGates.Load(vnetID); ok {
		return gate.(*orderGate)
	}
	gate := &orderGate{workers: map[*orderWorker]struct{}{}}
	actual, _ := networkRuleOrderGates.LoadOrStore(vnetID, gate)
	return actual.(*orderGate)
}

func (g *orderGate) join(ordered bool) *orderWorker {
	worker := &orderWorker{ordered: ordered}
	g.mu.Lock()
	g.workers[worker] = struct{}{}
	g.mu.Unlock()
	return worker
}

func (g *orderGate) leave(worker *orderWorker) {
	g.mu.Lock()
	delete(g.workers, worker)
	g.mu.Unlock()
}

func (g *orderGate) markCreated(worker *orderWorker) {
	g.mu.Lock()
	worker.created = true
	g.mu.Unlock()
}

func (g *orderGate) room(self *orderWorker) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for worker := range g.workers {
		if worker != self && !worker.created {
			n++
		}
	}
	return n
}

func (g *orderGate) setRest(worker *orderWorker, rest bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !rest {
		for other := range g.workers {
			other.atRest = false
		}
		return false
	}
	worker.atRest = true
	for other := range g.workers {
		if !other.created || (other.ordered && !other.atRest) {
			return false
		}
	}
	return true
}

func waitRuleOrder(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
