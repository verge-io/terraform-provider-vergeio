// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"sort"
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
	ordered  bool
	created  bool
	removing bool
	atRest   bool
	written  bool
	want     int
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

func (g *orderGate) join(ordered bool, want int) *orderWorker {
	worker := &orderWorker{ordered: ordered, want: want}
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

func (g *orderGate) joinRemoving() *orderWorker {
	worker := &orderWorker{removing: true}
	g.mu.Lock()
	g.workers[worker] = struct{}{}
	g.mu.Unlock()
	return worker
}

func (g *orderGate) hasRemoving(self *orderWorker) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for worker := range g.workers {
		if worker != self && worker.removing {
			return true
		}
	}
	return false
}

func (g *orderGate) room(self *orderWorker, currentCount int) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	type candidate struct {
		ordered bool
		want    int
	}
	var candidates []candidate
	for worker := range g.workers {
		if worker == self || worker.created || worker.removing {
			continue
		}
		candidates = append(candidates, candidate{ordered: worker.ordered, want: worker.want})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ordered != candidates[j].ordered {
			return !candidates[i].ordered
		}
		return candidates[i].want < candidates[j].want
	})
	accepted := 0
	for _, candidate := range candidates {
		if !candidate.ordered || candidate.want <= currentCount+1+accepted {
			accepted++
		}
	}
	return accepted
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
		if other != worker && other.removing {
			return false
		}
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
