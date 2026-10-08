// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

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

func TestRecipeBootWaitsForHeldMachineBeforePost(t *testing.T) {
	actor := &fakeBoot{machineBoot: true, succeedOn: 1}
	lines, id, err := runFakeBoot(t, actor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "recipe-1" {
		t.Fatalf("id = %s", id)
	}
	if actor.posts[0] != "on" || len(actor.posts) != 1 {
		t.Fatalf("posts = %#v", actor.posts)
	}
	if actor.observesBeforePost != recipeSteadyPolls {
		t.Fatalf("observes before post = %d, want %d", actor.observesBeforePost, recipeSteadyPolls)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "local_time=") || !strings.Contains(strings.Join(lines, "\n"), "power=on") {
		t.Fatalf("log = %s", strings.Join(lines, "\n"))
	}
}

func TestRecipeBootRetriesNeverStartedThenPostsPoweredOff(t *testing.T) {
	actor := &fakeBoot{
		machineBoot: true,
		postErr: func(online bool) error {
			if online {
				return vergeio.Error{StatusCode: 405, Endpoint: "tenant_recipes", VergeError: "You cannot create a recipe based on a tenant that has never been started"}
			}
			return nil
		},
	}
	_, id, err := runFakeBoot(t, actor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "recipe-1" {
		t.Fatalf("id = %s", id)
	}
	if len(actor.posts) < 3 || actor.posts[0] != "on" || actor.posts[len(actor.posts)-1] != "off" {
		t.Fatalf("posts = %#v", actor.posts)
	}
	if actor.powerOffs != 1 || actor.powerOns != 1 {
		t.Fatalf("power on=%d off=%d", actor.powerOns, actor.powerOffs)
	}
}

func TestRecipeBootPowersBackOnIfOffPostStillNeverStarted(t *testing.T) {
	actor := &fakeBoot{
		machineBoot: true,
		postErr: func(bool) error {
			return vergeio.Error{StatusCode: 405, Endpoint: "tenant_recipes", VergeError: "You cannot create a recipe based on a tenant that has never been started"}
		},
	}
	lines, _, err := runFakeBoot(t, actor)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "never been started") {
		t.Fatalf("err = %v", err)
	}
	if actor.powerOns < 2 || actor.powerOffs < 1 {
		t.Fatalf("power on=%d off=%d posts=%#v", actor.powerOns, actor.powerOffs, actor.posts)
	}
	joined := strings.Join(actor.posts, ",")
	if !strings.Contains(joined, "on") || !strings.Contains(joined, "off") {
		t.Fatalf("posts = %#v", actor.posts)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "power=off") {
		t.Fatal(strings.Join(lines, "\n"))
	}
}

func TestRecipeBootPowersOffWhenPostRequiresIt(t *testing.T) {
	actor := &fakeBoot{
		machineBoot: true,
		postErr: func(online bool) error {
			if online {
				return vergeio.Error{StatusCode: 405, Endpoint: "tenant_recipes", VergeError: "Tenant must be powered off"}
			}
			return nil
		},
	}
	_, id, err := runFakeBoot(t, actor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "recipe-1" || len(actor.posts) != 2 || actor.posts[0] != "on" || actor.posts[1] != "off" {
		t.Fatalf("id=%s posts=%#v", id, actor.posts)
	}
}

func TestRecipeBootStopsOnUnexpectedPostError(t *testing.T) {
	actor := &fakeBoot{
		machineBoot: true,
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
	actor := &fakeBoot{machineBoot: false}
	lines, _, err := runFakeBoot(t, actor)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "started=0") {
		t.Fatalf("err = %v", err)
	}
	if len(actor.posts) != 0 {
		t.Fatalf("posts = %#v", actor.posts)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "tenant ") || !strings.Contains(joined, "machine 5") {
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
	online             bool
	machineBoot        bool
	powerOns           int
	powerOffs          int
	posts              []string
	observes           int
	observesBeforePost int
	succeedOn          int
	postErr            func(online bool) error
}

func (a *fakeBoot) powerOn(context.Context) error {
	a.online = true
	a.powerOns++
	return nil
}

func (a *fakeBoot) powerOff(context.Context) error {
	a.online = false
	a.powerOffs++
	return nil
}

func (a *fakeBoot) observe(context.Context) (tenantBootView, error) {
	a.observes++
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
		if a.machineBoot {
			machine.Started = 1700000000
			machine.LocalTime = 1700000100
			machine.AgentVersion = "nested"
		}
	}
	return tenantBootView{
		status:   status,
		nodes:    []vergeos.TenantNode{{Key: 1, Machine: 5}},
		machines: map[int]*vergeos.MachineStatus{5: machine},
		ui:       "ui_address empty",
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
	if a.succeedOn > 0 && len(a.posts) >= a.succeedOn {
		return "recipe-1", nil
	}
	if a.postErr != nil {
		if err := a.postErr(a.online); err != nil {
			return "", err
		}
	}
	if a.succeedOn == 0 && a.postErr == nil {
		return "", vergeio.Error{StatusCode: 405, VergeError: "You cannot create a recipe based on a tenant that has never been started"}
	}
	return "recipe-1", nil
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
