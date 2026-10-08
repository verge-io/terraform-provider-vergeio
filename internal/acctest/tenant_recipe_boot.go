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
	// recipeGuestLimit is how long the source tenant stays powered on while
	// the fixture waits for a guest signal. The lab's short power cycles
	// never reached first boot.
	recipeGuestLimit = 20 * time.Minute
	// recipePollGap is the pause between status reads during that wait.
	recipePollGap = 15 * time.Second
	// recipeOffLimit bounds one power-off and network-stop wait. A
	// "currently running" POST retries that wait, up to recipeRunningPosts.
	recipeOffLimit = 2 * time.Minute
	recipeOffPoll  = 5 * time.Second
	// recipeRunningPosts is the first POST plus retries for the
	// "currently running" race while power-off settles.
	recipeRunningPosts = 3
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

	attempt int
	last    string
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
	}
}

func (b *recipeBoot) run(ctx context.Context) (string, error) {
	if err := b.act.powerOn(ctx); err != nil {
		return "", err
	}
	if err := b.waitForGuest(ctx); err != nil {
		return "", err
	}
	return b.postWhileOff(ctx)
}

func (b *recipeBoot) waitForGuest(ctx context.Context) error {
	deadline := b.now().Add(recipeGuestLimit)
	for {
		view, err := b.act.observe(ctx)
		if err != nil {
			return err
		}
		b.write(formatBootView(view))
		if guestSignaled(view) {
			return nil
		}
		if !b.now().Before(deadline) {
			return fmt.Errorf("timed out after %s waiting for the guest to report local_time, agent_version, or an answering ui_address\nlast: %s\n%s", recipeGuestLimit, b.last, b.log.String())
		}
		if err := b.sleep(ctx, recipePollGap); err != nil {
			return err
		}
	}
}

func (b *recipeBoot) postWhileOff(ctx context.Context) (string, error) {
	for {
		if err := b.settleOff(ctx); err != nil {
			return "", err
		}
		id, kind, err := b.post(ctx)
		if kind == recipePostOK {
			return id, nil
		}
		if kind == recipePostNeedOff && b.attempt < recipeRunningPosts {
			continue
		}
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("tenant still reported as running after %d recipe posts\nlast: %s\n%s", b.attempt, b.last, b.log.String())
	}
}

func (b *recipeBoot) post(ctx context.Context) (string, recipePostKind, error) {
	b.attempt++
	id, err := b.act.postRecipe(ctx)
	kind, detail := classifyRecipePost(err)
	b.write(fmt.Sprintf("post %d power=off %s", b.attempt, recipePostLabel(kind, detail)))
	if kind == recipePostOK || kind == recipePostNeedOff {
		return id, kind, nil
	}
	return "", kind, fmt.Errorf("%s\n%s", detail, b.log.String())
}

func (b *recipeBoot) settleOff(ctx context.Context) error {
	view, err := b.act.observe(ctx)
	if err != nil {
		return err
	}
	b.write(formatBootView(view))
	if tenantRowOnline(view.status) {
		if err := b.act.powerOff(ctx); err != nil {
			return err
		}
	}
	return b.waitNetworkStopped(ctx)
}

func (b *recipeBoot) waitNetworkStopped(ctx context.Context) error {
	deadline := b.now().Add(recipeOffLimit)
	networkStopped := false
	for {
		view, err := b.act.observe(ctx)
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
		if !b.now().Before(deadline) {
			return fmt.Errorf("timed out after %s waiting for the tenant network to stop\nlast: %s\n%s", recipeOffLimit, b.last, b.log.String())
		}
		if err := b.sleep(ctx, recipeOffPoll); err != nil {
			return err
		}
	}
}

func (b *recipeBoot) write(line string) {
	b.last = line
	b.logf("%s", line)
	b.log.WriteString(line)
	b.log.WriteByte('\n')
}

func guestSignaled(view tenantBootView) bool {
	if machineGuestReported(view) {
		return true
	}
	return uiAnswers(view.ui)
}

func machineGuestReported(view tenantBootView) bool {
	for i := range view.nodes {
		status := view.machines[view.nodes[i].Machine.Int()]
		if status == nil {
			continue
		}
		if status.LocalTime > 0 || strings.TrimSpace(status.AgentVersion) != "" {
			return true
		}
	}
	return false
}

// uiAnswers is the probe text from a ui_address that returned an HTTP status.
// "ui_address empty" and a connection error do not count.
func uiAnswers(ui string) bool {
	return strings.Contains(ui, " status=")
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
