// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

const (
	// recipeBootLimit is the bound from power-on through recipe POST retries.
	recipeBootLimit = 15 * time.Minute
	// recipePollGap is the pause between status reads before the first POST.
	recipePollGap = 15 * time.Second
	// recipeSteadyPolls is how many consecutive online reads count as held.
	recipeSteadyPolls = 3
	recipeOffPoll     = 5 * time.Second
	recipeBackoff     = 60 * time.Second
)

// recipePostKind is how one tenant-recipe POST should change the next try.
type recipePostKind int

const (
	recipePostOK recipePostKind = iota
	recipePostNeverStarted
	recipePostNeedOff
	recipePostNeedOn
	recipePostFatal
)

type tenantBootView struct {
	status         *vergeos.TenantStatus
	nodes          []vergeos.TenantNode
	machines       map[int]*vergeos.MachineStatus
	networkID      int
	networkRunning bool
	ui             string
}

type recipeBootAct interface {
	powerOn(ctx context.Context) error
	powerOff(ctx context.Context) error
	stopNetwork(ctx context.Context) error
	observe(ctx context.Context) (tenantBootView, error)
	postRecipe(ctx context.Context) (string, error)
}

type recipeBoot struct {
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
	logf  func(string, ...any)
	act   recipeBootAct
	log   strings.Builder

	deadline time.Time
	gap      time.Duration
	steady   int
	attempt  int
	held     bool
	forceOff bool
	posted   bool
}

func newRecipeBoot(now func() time.Time, sleep func(context.Context, time.Duration) error, logf func(string, ...any), act recipeBootAct) *recipeBoot {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &recipeBoot{
		now:   now,
		sleep: sleep,
		logf:  logf,
		act:   act,
		gap:   recipePollGap,
	}
}

func (b *recipeBoot) run(ctx context.Context) (string, error) {
	if err := b.act.powerOn(ctx); err != nil {
		return "", err
	}
	b.deadline = b.now().Add(recipeBootLimit)
	for {
		if !b.now().Before(b.deadline) {
			return "", b.timedOut()
		}
		id, done, err := b.step(ctx)
		if done || err != nil {
			return id, err
		}
		if err := b.sleep(ctx, b.nextWait()); err != nil {
			return "", err
		}
	}
}

func (b *recipeBoot) step(ctx context.Context) (string, bool, error) {
	view, err := b.act.observe(ctx)
	if err != nil {
		return "", false, err
	}
	b.noteHold(view)
	b.write(formatBootView(view))
	b.posted = false
	if !b.held && !b.forceOff {
		return "", false, nil
	}
	if err := b.powerOffForPost(ctx, view); err != nil {
		return "", false, err
	}
	b.forceOff = false
	b.posted = true
	return b.post(ctx)
}

func (b *recipeBoot) post(ctx context.Context) (string, bool, error) {
	b.attempt++
	id, err := b.act.postRecipe(ctx)
	kind, detail := classifyRecipePost(err)
	b.write(fmt.Sprintf("post %d power=off %s", b.attempt, recipePostLabel(kind, detail)))
	switch kind {
	case recipePostOK:
		return id, true, nil
	case recipePostFatal:
		return "", true, fmt.Errorf("%s\n%s", detail, b.log.String())
	case recipePostNeedOff:
		b.forceOff = true
		return "", false, nil
	default:
		return "", false, b.powerOnForBoot(ctx)
	}
}

func (b *recipeBoot) powerOnForBoot(ctx context.Context) error {
	b.forceOff = false
	if err := b.act.powerOn(ctx); err != nil {
		return err
	}
	b.steady = 0
	b.held = false
	return nil
}

func (b *recipeBoot) powerOffForPost(ctx context.Context, view tenantBootView) error {
	if tenantRowOnline(view.status) {
		if err := b.act.powerOff(ctx); err != nil {
			return err
		}
	}
	networkStopped := false
	for {
		var err error
		view, err = b.act.observe(ctx)
		if err != nil {
			return err
		}
		b.write(formatBootView(view))
		offline := !tenantRowOnline(view.status)
		if offline && !view.networkRunning {
			return nil
		}
		if offline && view.networkRunning && !networkStopped {
			if err := b.act.stopNetwork(ctx); err != nil {
				return err
			}
			networkStopped = true
			continue
		}
		if !b.now().Before(b.deadline) {
			return b.timedOut()
		}
		if err := b.sleep(ctx, recipeOffPoll); err != nil {
			return err
		}
	}
}

func (b *recipeBoot) noteHold(view tenantBootView) {
	if view.booted() {
		b.steady++
	} else {
		b.steady = 0
	}
	b.held = b.steady >= recipeSteadyPolls
}

func (b *recipeBoot) nextWait() time.Duration {
	if !b.posted {
		return recipePollGap
	}
	wait := b.gap
	if b.gap < recipeBackoff {
		b.gap *= 2
		if b.gap > recipeBackoff {
			b.gap = recipeBackoff
		}
	}
	return wait
}

func (b *recipeBoot) write(line string) {
	b.logf("%s", line)
	b.log.WriteString(line)
	b.log.WriteByte('\n')
}

func (b *recipeBoot) timedOut() error {
	return fmt.Errorf("timed out after %s waiting to post the tenant recipe after the node machine stayed running\n%s", recipeBootLimit, b.log.String())
}

func (v tenantBootView) booted() bool {
	if !tenantRowOnline(v.status) || len(v.nodes) == 0 {
		return false
	}
	for i := range v.nodes {
		id := v.nodes[i].Machine.Int()
		if !machineBootSignal(v.machines[id]) {
			return false
		}
	}
	return true
}

func tenantRowOnline(status *vergeos.TenantStatus) bool {
	if status == nil || status.Starting || status.Stopping {
		return false
	}
	if status.Running {
		return true
	}
	switch status.Status {
	case "online", "migrating", "restarting", "reduced":
		return true
	default:
		return false
	}
}

// machineBootSignal is the tenant-node machine status lead for the recipe
// hook: running (or status started/running) with a start timestamp, read
// again on later polls. Guest agent fields are logged and not required.
func machineBootSignal(status *vergeos.MachineStatus) bool {
	if status == nil || status.Started <= 0 {
		return false
	}
	if status.Running || status.PowerState {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status.Status)) {
	case "running", "started", "online":
		return true
	default:
		return false
	}
}

func classifyRecipePost(err error) (recipePostKind, string) {
	if err == nil {
		return recipePostOK, ""
	}
	var apiErr vergeio.Error
	if !errors.As(err, &apiErr) {
		return recipePostFatal, err.Error()
	}
	msg := strings.ToLower(apiErr.VergeError)
	switch {
	case strings.Contains(msg, "never been started"):
		return recipePostNeverStarted, apiErr.Error()
	case recipeNeedsOff(msg):
		return recipePostNeedOff, apiErr.Error()
	case recipeNeedsOn(msg):
		return recipePostNeedOn, apiErr.Error()
	default:
		return recipePostFatal, apiErr.Error()
	}
}

func recipeNeedsOff(msg string) bool {
	return strings.Contains(msg, "powered off") ||
		strings.Contains(msg, "must be off") ||
		strings.Contains(msg, "while running") ||
		strings.Contains(msg, "currently running") ||
		strings.Contains(msg, "still running") ||
		strings.Contains(msg, "power off")
}

func recipeNeedsOn(msg string) bool {
	return strings.Contains(msg, "must be running") ||
		strings.Contains(msg, "powered on") ||
		strings.Contains(msg, "must be online") ||
		strings.Contains(msg, "not running")
}

func recipePostLabel(kind recipePostKind, detail string) string {
	switch kind {
	case recipePostOK:
		return "created"
	case recipePostNeverStarted:
		return "retry never-started: " + detail
	case recipePostNeedOff:
		return "power off requested: " + detail
	case recipePostNeedOn:
		return "power on requested: " + detail
	default:
		return "failed: " + detail
	}
}

func formatBootView(view tenantBootView) string {
	return tenantStatusText(view.status) + "; " + machineStatusText(view) + "; " + networkText(view) + "; " + uiText(view.ui)
}

func networkText(view tenantBootView) string {
	if view.networkID <= 0 {
		return "vnet unset"
	}
	return fmt.Sprintf("vnet %d running=%t", view.networkID, view.networkRunning)
}

func tenantStatusText(status *vergeos.TenantStatus) string {
	if status == nil {
		return "tenant status missing"
	}
	return fmt.Sprintf("tenant running=%t starting=%t stopping=%t migrating=%t status=%s state=%s started=%d stopped=%d",
		status.Running, status.Starting, status.Stopping, status.Migrating, status.Status, status.State, status.Started, status.Stopped)
}

func machineStatusText(view tenantBootView) string {
	if len(view.nodes) == 0 {
		return "nodes=0"
	}
	parts := make([]string, 0, len(view.nodes))
	for i := range view.nodes {
		id := view.nodes[i].Machine.Int()
		parts = append(parts, oneMachineText(id, view.machines[id]))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func oneMachineText(id int, status *vergeos.MachineStatus) string {
	if status == nil {
		return fmt.Sprintf("machine %d missing", id)
	}
	return fmt.Sprintf("machine %d running=%t powerstate=%t status=%s state=%s started=%d local_time=%d agent=%q status_info=%q",
		id, status.Running, status.PowerState, status.Status, status.State, status.Started, status.LocalTime, status.AgentVersion, status.StatusInfo)
}

func uiText(ui string) string {
	if strings.TrimSpace(ui) == "" {
		return "ui_address empty"
	}
	return ui
}

type fixtureRecipePost struct {
	f    *TenantRecipeFixture
	name string
}

func (p fixtureRecipePost) powerOn(ctx context.Context) error {
	return p.f.sdk.Tenants.PowerOnWithNode(ctx, p.f.tenantID, 0)
}

func (p fixtureRecipePost) powerOff(ctx context.Context) error {
	return p.f.sdk.Tenants.PowerOff(ctx, p.f.tenantID)
}

func (p fixtureRecipePost) stopNetwork(ctx context.Context) error {
	row, err := p.f.sdk.Tenants.Get(ctx, p.f.tenantID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if row == nil || row.VNet.Int() <= 0 {
		return nil
	}
	if err := p.f.sdk.Networks.Kill(ctx, row.VNet.Int()); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	return nil
}

func (p fixtureRecipePost) postRecipe(ctx context.Context) (string, error) {
	return p.f.http.CreateTenantRecipe(ctx, vergeio.TenantRecipeCreate{
		Name:        p.name,
		Description: "acceptance tenant recipe",
		Catalog:     p.f.catalogID,
		Tenant:      p.f.tenantID,
		Version:     "1.0.0",
	})
}

func (p fixtureRecipePost) observe(ctx context.Context) (tenantBootView, error) {
	status, err := p.f.sdk.TenantStatus.Get(ctx, p.f.tenantID)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return tenantBootView{}, err
	}
	if vergeos.IsNotFoundError(err) {
		status = nil
	}
	nodes, err := p.f.sdk.TenantNodes.ListByTenant(ctx, p.f.tenantID)
	if err != nil && !sdkListMissing(err) {
		return tenantBootView{}, err
	}
	if sdkListMissing(err) {
		nodes = nil
	}
	machines := map[int]*vergeos.MachineStatus{}
	for i := range nodes {
		id := nodes[i].Machine.Int()
		if id <= 0 {
			continue
		}
		machine, mErr := p.f.sdk.MachineStatus.Get(ctx, id)
		if mErr != nil && !vergeos.IsNotFoundError(mErr) {
			return tenantBootView{}, mErr
		}
		if vergeos.IsNotFoundError(mErr) {
			machines[id] = nil
			continue
		}
		machines[id] = machine
	}
	networkID, networkRunning, err := p.tenantNetwork(ctx)
	if err != nil {
		return tenantBootView{}, err
	}
	return tenantBootView{
		status:         status,
		nodes:          nodes,
		machines:       machines,
		networkID:      networkID,
		networkRunning: networkRunning,
		ui:             p.probeUI(ctx),
	}, nil
}

func (p fixtureRecipePost) tenantNetwork(ctx context.Context) (int, bool, error) {
	row, err := p.f.sdk.Tenants.Get(ctx, p.f.tenantID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if row == nil || row.VNet.Int() <= 0 {
		return 0, false, nil
	}
	id := row.VNet.Int()
	network, err := p.f.sdk.Networks.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return id, false, nil
		}
		return id, false, err
	}
	if network == nil {
		return id, false, nil
	}
	return id, network.Running, nil
}

func (p fixtureRecipePost) probeUI(ctx context.Context) string {
	row, err := p.f.sdk.Tenants.Get(ctx, p.f.tenantID)
	if err != nil || row == nil || row.UIAddress.Int() <= 0 {
		return "ui_address empty"
	}
	addr, err := p.f.sdk.VNetAddresses.Get(ctx, row.UIAddress.Int())
	if err != nil || addr == nil || strings.TrimSpace(addr.IP) == "" {
		return fmt.Sprintf("ui_address %d has no ip (%v)", row.UIAddress.Int(), err)
	}
	var parts []string
	for _, scheme := range []string{"https", "http"} {
		code, probeErr := p.f.http.ProbeURL(ctx, scheme+"://"+addr.IP+"/")
		if probeErr != nil {
			parts = append(parts, scheme+" err="+probeErr.Error())
			continue
		}
		parts = append(parts, fmt.Sprintf("%s status=%d", scheme, code))
	}
	return "ui " + addr.IP + " " + strings.Join(parts, " ")
}

func sdkListMissing(err error) bool {
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr *vergeos.APIError
	return errors.As(err, &apiErr) && apiErr != nil && apiErr.StatusCode == 404
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
