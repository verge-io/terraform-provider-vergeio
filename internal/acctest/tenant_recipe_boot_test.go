// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestClassifyRecipePostCurrentlyRunningIsNeedsOff(t *testing.T) {
	err := vergeio.Error{
		StatusCode: 405,
		Endpoint:   "api/v4/tenant_recipes",
		VergeError: "Tenant is currently running",
	}
	if got := fmt.Sprintf("%d %s", err.StatusCode, err.VergeError); got != "405 Tenant is currently running" {
		t.Fatalf("message = %q", got)
	}
	kind, detail := classifyRecipePost(err)
	if kind != recipePostNeedOff || !strings.Contains(detail, "Tenant is currently running") {
		t.Fatalf("kind = %d detail = %s", kind, detail)
	}
}

func TestClassifyRecipePost(t *testing.T) {
	never := vergeio.Error{StatusCode: 405, VergeError: "You cannot create a recipe based on a tenant that has never been started"}
	kind, _ := classifyRecipePost(fmt.Errorf("post: %w", never))
	if kind != recipePostNeverStarted {
		t.Fatalf("never-started kind = %d", kind)
	}
	kind, _ = classifyRecipePost(vergeio.Error{VergeError: "base tenant must be powered off"})
	if kind != recipePostNeedOff {
		t.Fatalf("off kind = %d", kind)
	}
	kind, _ = classifyRecipePost(vergeio.Error{VergeError: "tenant must be running"})
	if kind != recipePostNeedOn {
		t.Fatalf("on kind = %d", kind)
	}
	kind, detail := classifyRecipePost(vergeio.Error{StatusCode: 422, VergeError: "field 'tenant_snapshot' cannot be set"})
	if kind != recipePostFatal || !strings.Contains(detail, "tenant_snapshot") {
		t.Fatalf("fatal = %d %s", kind, detail)
	}
	if kind, _ = classifyRecipePost(nil); kind != recipePostOK {
		t.Fatalf("nil kind = %d", kind)
	}
}

func TestRecipeBootPostsAfterGuestSignal(t *testing.T) {
	actor := &fakeBoot{guestAfter: 1}
	lines, id, err := runFakeBoot(t, actor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "recipe-1" {
		t.Fatalf("id = %s", id)
	}
	if len(actor.posts) != 1 || actor.posts[0] != "off" {
		t.Fatalf("posts = %#v", actor.posts)
	}
	if actor.powerOns != 1 || actor.powerOffs != 1 {
		t.Fatalf("power on=%d off=%d", actor.powerOns, actor.powerOffs)
	}
	if actor.networkStops != 1 || actor.postsWhileNetworkUp != 0 {
		t.Fatalf("network stops=%d posts while network up=%d", actor.networkStops, actor.postsWhileNetworkUp)
	}
	log := strings.Join(lines, "\n")
	if !strings.Contains(log, "local_time=") || !strings.Contains(log, "power=off") || !strings.Contains(log, "vnet 84 running=false") {
		t.Fatalf("log = %s", log)
	}
}

func TestRecipeBootPostsWhenUIAnswers(t *testing.T) {
	actor := &fakeBoot{uiAnswer: true}
	_, id, err := runFakeBoot(t, actor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "recipe-1" || actor.powerOns != 1 || len(actor.posts) != 1 || actor.posts[0] != "off" {
		t.Fatalf("id=%s powerOns=%d posts=%#v", id, actor.powerOns, actor.posts)
	}
}

func TestRecipeBootNeverStartedAfterOffIsFatal(t *testing.T) {
	actor := &fakeBoot{
		guestAfter: 1,
		postErr: func(bool) error {
			return vergeio.Error{StatusCode: 405, Endpoint: "api/v4/tenant_recipes", VergeError: "You cannot create a recipe based on a tenant that has never been started"}
		},
	}
	lines, _, err := runFakeBoot(t, actor)
	if err == nil || !strings.Contains(err.Error(), "never been started") {
		t.Fatalf("err = %v", err)
	}
	if actor.powerOns != 1 || actor.powerOffs != 1 || len(actor.posts) != 1 || actor.posts[0] != "off" {
		t.Fatalf("power on=%d off=%d posts=%#v", actor.powerOns, actor.powerOffs, actor.posts)
	}
	if strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "power=off") {
		t.Fatalf("log = %s", strings.Join(lines, "\n"))
	}
}

func TestFakeBootLabPostMatchesVergeOS(t *testing.T) {
	short := &fakeBoot{online: true}
	_, err := short.postRecipe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "never been started") {
		t.Fatalf("unbooted guest = %v", err)
	}

	running := &fakeBoot{online: true, guestLive: true}
	_, err = running.postRecipe(context.Background())
	if got := errString(err); got != "405 Tenant is currently running" {
		t.Fatalf("running tenant = %q", got)
	}

	ready := &fakeBoot{online: false, guestLive: true}
	id, err := ready.postRecipe(context.Background())
	if err != nil || id != "recipe-1" {
		t.Fatalf("booted and powered off = %s %v", id, err)
	}
}

func errString(err error) string {
	var apiErr vergeio.Error
	if !errors.As(err, &apiErr) {
		return ""
	}
	return fmt.Sprintf("%d %s", apiErr.StatusCode, apiErr.VergeError)
}

func TestRecipeBootRetriesCurrentlyRunning(t *testing.T) {
	calls := 0
	actor := &fakeBoot{guestAfter: 1}
	actor.postErr = func(bool) error {
		calls++
		if calls == 1 {
			actor.online = true
			actor.networkRunning = true
			return vergeio.Error{StatusCode: 405, Endpoint: "api/v4/tenant_recipes", VergeError: "Tenant is currently running"}
		}
		return nil
	}
	lines, id, err := runFakeBoot(t, actor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "recipe-1" || len(actor.posts) != 2 || actor.posts[0] != "off" || actor.posts[1] != "off" {
		t.Fatalf("id=%s posts=%#v", id, actor.posts)
	}
	if actor.powerOns != 1 || actor.powerOffs != 2 || actor.networkStops != 2 {
		t.Fatalf("power on=%d off=%d network stops=%d", actor.powerOns, actor.powerOffs, actor.networkStops)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Tenant is currently running") {
		t.Fatal(strings.Join(lines, "\n"))
	}
}

func TestRecipeBootStopsAfterCurrentlyRunningRetries(t *testing.T) {
	actor := &fakeBoot{guestAfter: 1}
	actor.postErr = func(bool) error {
		actor.online = true
		actor.networkRunning = true
		return vergeio.Error{StatusCode: 405, Endpoint: "api/v4/tenant_recipes", VergeError: "Tenant is currently running"}
	}
	_, _, err := runFakeBoot(t, actor)
	if err == nil || !strings.Contains(err.Error(), "still reported as running") {
		t.Fatalf("err = %v", err)
	}
	if actor.powerOns != 1 || actor.powerOffs != recipeRunningPosts || len(actor.posts) != recipeRunningPosts {
		t.Fatalf("power on=%d off=%d posts=%#v", actor.powerOns, actor.powerOffs, actor.posts)
	}
	for _, state := range actor.posts {
		if state != "off" {
			t.Fatalf("posts = %#v", actor.posts)
		}
	}
}

func TestRecipeBootStopsOnUnexpectedPostError(t *testing.T) {
	actor := &fakeBoot{
		guestAfter: 1,
		postErr: func(bool) error {
			return vergeio.Error{StatusCode: 422, Endpoint: "tenant_recipes", VergeError: "field 'tenant_snapshot' cannot be set"}
		},
	}
	lines, _, err := runFakeBoot(t, actor)
	if err == nil || !strings.Contains(err.Error(), "tenant_snapshot") || !strings.Contains(err.Error(), "machine 5") {
		t.Fatalf("err = %v", err)
	}
	if len(actor.posts) != 1 || !strings.Contains(strings.Join(lines, "\n"), "machine 5") {
		t.Fatalf("posts=%#v log=%s", actor.posts, strings.Join(lines, "\n"))
	}
}

func TestRecipeBootTimeoutLogsMachineStatus(t *testing.T) {
	actor := &fakeBoot{}
	lines, _, err := runFakeBoot(t, actor)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "local_time=0") || !strings.Contains(err.Error(), `agent=""`) {
		t.Fatalf("err = %v", err)
	}
	if len(actor.posts) != 0 || actor.powerOns != 1 || actor.powerOffs != 0 {
		t.Fatalf("power on=%d off=%d posts=%#v", actor.powerOns, actor.powerOffs, actor.posts)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "tenant ") || !strings.Contains(joined, "machine 5") || !strings.Contains(err.Error(), "last:") {
		t.Fatalf("log = %s", joined)
	}
}

type fakeClock struct {
	at time.Time
}

func (c *fakeClock) now() time.Time { return c.at }

func (c *fakeClock) sleep(_ context.Context, d time.Duration) error {
	c.at = c.at.Add(d)
	return nil
}

type fakeBoot struct {
	online              bool
	guestAfter          int
	guestLive           bool
	uiAnswer            bool
	onlinePolls         int
	networkRunning      bool
	networkStops        int
	postsWhileNetworkUp int
	powerOns            int
	powerOffs           int
	posts               []string
	observes            int
	observesBeforePost  int
	postErr             func(online bool) error
}

func (a *fakeBoot) powerOn(context.Context) error {
	a.online = true
	a.networkRunning = true
	a.powerOns++
	return nil
}

func (a *fakeBoot) powerOff(context.Context) error {
	a.online = false
	a.powerOffs++
	return nil
}

func (a *fakeBoot) stopNetwork(context.Context) error {
	a.networkRunning = false
	a.networkStops++
	return nil
}

func (a *fakeBoot) observe(context.Context) (tenantBootView, error) {
	a.observes++
	if a.online {
		a.onlinePolls++
		if a.guestAfter > 0 && a.onlinePolls >= a.guestAfter {
			a.guestLive = true
		}
	}
	status := &vergeos.TenantStatus{Started: 1791467895, Status: "offline", State: "offline"}
	machine := &vergeos.MachineStatus{Machine: 5, Status: "stopped", State: "offline", Started: 0}
	if a.online {
		status.Running = true
		status.Status = "online"
		status.State = "online"
		machine.Running = true
		machine.PowerState = true
		machine.Status = "running"
		machine.State = "online"
		machine.Started = 1700000000
		if a.guestLive {
			machine.LocalTime = 1700000100
			machine.AgentVersion = "nested"
		}
	}
	ui := "ui_address empty"
	if a.uiAnswer {
		ui = "ui 203.0.113.10 https status=200"
	}
	return tenantBootView{
		status:         status,
		nodes:          []vergeos.TenantNode{{Key: 1, Machine: 5}},
		machines:       map[int]*vergeos.MachineStatus{5: machine},
		networkID:      84,
		networkRunning: a.networkRunning,
		ui:             ui,
	}, nil
}

func (a *fakeBoot) postRecipe(context.Context) (string, error) {
	if a.observesBeforePost == 0 {
		a.observesBeforePost = a.observes
	}
	state := "off"
	if a.online {
		state = "on"
	}
	a.posts = append(a.posts, state)
	if a.networkRunning {
		a.postsWhileNetworkUp++
	}
	if a.postErr != nil {
		if err := a.postErr(a.online); err != nil {
			return "", err
		}
		return "recipe-1", nil
	}
	if err := a.labPostError(); err != nil {
		return "", err
	}
	return "recipe-1", nil
}

func (a *fakeBoot) labPostError() error {
	booted := a.guestLive || a.uiAnswer
	if a.online && booted {
		return vergeio.Error{StatusCode: 405, Endpoint: "api/v4/tenant_recipes", VergeError: "Tenant is currently running"}
	}
	if !booted || a.online {
		return vergeio.Error{StatusCode: 405, Endpoint: "api/v4/tenant_recipes", VergeError: "You cannot create a recipe based on a tenant that has never been started"}
	}
	return nil
}

func runFakeBoot(t *testing.T, actor *fakeBoot) ([]string, string, error) {
	t.Helper()
	clock := &fakeClock{at: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)}
	var lines []string
	boot := newRecipeBoot(clock.now, clock.sleep, func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}, actor)
	id, err := boot.run(context.Background())
	return lines, id, err
}
