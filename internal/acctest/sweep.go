package acctest

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/verge-io/govergeos"
)

// Sweep deletes acceptance-test leftovers whose names start with ResourcePrefix.
// Tag memberships and group memberships are removed when they point at those
// objects. Objects outside the prefix are left alone.
func Sweep(ctx context.Context) error {
	client, err := SDKClient()
	if err != nil {
		return err
	}

	vms, err := client.VMs.List(ctx)
	if err != nil {
		return fmt.Errorf("list vms: %w", err)
	}
	networks, err := client.Networks.List(ctx)
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}
	users, err := client.Users.List(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	groups, err := client.Groups.List(ctx)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	files, err := client.CloudInitFiles.List(ctx)
	if err != nil {
		return fmt.Errorf("list cloud-init files: %w", err)
	}
	members, err := client.Members.List(ctx)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}
	tagMembers, err := client.TagMembers.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] tag members endpoint unavailable, skipping")
		tagMembers = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list tag members: %w", err)
	}

	vmIDs := map[int]string{}
	for _, vm := range vms {
		if vm.IsSnapshot || !HasPrefix(vm.Name) {
			continue
		}
		vmIDs[vm.ID.Int()] = vm.Name
	}
	networkIDs := map[int]string{}
	for _, network := range networks {
		if !HasPrefix(network.Name) {
			continue
		}
		networkIDs[network.ID.Int()] = network.Name
	}
	userIDs := map[int]string{}
	for _, user := range users {
		if !HasPrefix(user.Name) {
			continue
		}
		userIDs[user.Key.Int()] = user.Name
	}
	groupIDs := map[int]string{}
	for _, group := range groups {
		if !HasPrefix(group.Name) {
			continue
		}
		groupIDs[group.ID.Int()] = group.Name
	}

	permissions, err := client.Permissions.List(ctx)
	if err != nil {
		return fmt.Errorf("list permissions: %w", err)
	}

	var errs []error
	record := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, perm := range permissions {
		if !permissionBelongsToSweep(perm, userIDs, groupIDs) {
			continue
		}
		id := perm.Key.Int()
		log.Printf("[SWEEP] deleting permission %d (identity %d %s)", id, perm.Identity.Int(), perm.Table)
		record(ignoreNotFound(client.Permissions.Delete(ctx, id), "permission", id))
	}

	for _, tagMember := range tagMembers {
		if !referencesSweptObject(tagMember.Member, vmIDs, networkIDs, userIDs, groupIDs) && !HasPrefix(tagMember.Member) {
			continue
		}
		id := tagMember.Key.Int()
		log.Printf("[SWEEP] deleting tag member %d (%s)", id, tagMember.Member)
		record(ignoreNotFound(client.TagMembers.Delete(ctx, id), "tag member", id))
	}

	for _, member := range members {
		if member.System {
			continue
		}
		_, ownedGroup := groupIDs[member.Group.Int()]
		if !ownedGroup && !HasPrefix(member.Member) && !referencesSweptObject(member.Member, vmIDs, networkIDs, userIDs, groupIDs) {
			continue
		}
		id := member.ID.Int()
		log.Printf("[SWEEP] deleting member %d (%s)", id, member.Member)
		record(ignoreNotFound(client.Members.Delete(ctx, id), "member", id))
	}

	for _, file := range files {
		ownerVM := ownerVMID(file.Owner)
		if !HasPrefix(file.Name) {
			if _, ok := vmIDs[ownerVM]; !ok {
				continue
			}
		}
		id := file.ID.Int()
		log.Printf("[SWEEP] deleting cloud-init file %d (%s)", id, file.Name)
		record(ignoreNotFound(client.CloudInitFiles.Delete(ctx, id), "cloud-init file", id))
	}

	for id, name := range vmIDs {
		log.Printf("[SWEEP] deleting vm %d (%s)", id, name)
		record(deleteVM(ctx, client, id))
	}
	for id, name := range networkIDs {
		log.Printf("[SWEEP] deleting network %d (%s)", id, name)
		record(deleteNetwork(ctx, client, id))
	}
	for id, name := range groupIDs {
		log.Printf("[SWEEP] deleting group %d (%s)", id, name)
		record(ignoreNotFound(client.Groups.Delete(ctx, id), "group", id))
	}
	for id, name := range userIDs {
		log.Printf("[SWEEP] deleting user %d (%s)", id, name)
		record(ignoreNotFound(client.Users.Delete(ctx, id), "user", id))
	}

	record(sweepTenants(ctx, client))

	if len(errs) > 0 {
		return fmt.Errorf("sweep deleted with %d error(s): %w", len(errs), errorsJoin(errs))
	}
	return verifySweep(ctx, client)
}

func verifySweep(ctx context.Context, client *vergeos.Client) error {
	var left []string

	vms, err := client.VMs.List(ctx)
	if err != nil {
		return fmt.Errorf("verify vms: %w", err)
	}
	for _, vm := range vms {
		if !vm.IsSnapshot && HasPrefix(vm.Name) {
			left = append(left, fmt.Sprintf("vm %s (%d)", vm.Name, vm.ID.Int()))
		}
	}

	networks, err := client.Networks.List(ctx)
	if err != nil {
		return fmt.Errorf("verify networks: %w", err)
	}
	for _, network := range networks {
		if HasPrefix(network.Name) {
			left = append(left, fmt.Sprintf("network %s (%d)", network.Name, network.ID.Int()))
		}
	}

	users, err := client.Users.List(ctx)
	if err != nil {
		return fmt.Errorf("verify users: %w", err)
	}
	for _, user := range users {
		if HasPrefix(user.Name) {
			left = append(left, fmt.Sprintf("user %s (%d)", user.Name, user.Key.Int()))
		}
	}

	groups, err := client.Groups.List(ctx)
	if err != nil {
		return fmt.Errorf("verify groups: %w", err)
	}
	for _, group := range groups {
		if HasPrefix(group.Name) {
			left = append(left, fmt.Sprintf("group %s (%d)", group.Name, group.ID.Int()))
		}
	}

	files, err := client.CloudInitFiles.List(ctx)
	if err != nil {
		return fmt.Errorf("verify cloud-init files: %w", err)
	}
	for _, file := range files {
		if HasPrefix(file.Name) {
			left = append(left, fmt.Sprintf("cloud-init file %s (%d)", file.Name, file.ID.Int()))
		}
	}

	tenants, err := client.Tenants.List(ctx)
	if err != nil {
		return fmt.Errorf("verify tenants: %w", err)
	}
	for _, tenant := range tenants {
		if !tenant.IsSnapshot && HasPrefix(tenant.Name) {
			left = append(left, fmt.Sprintf("tenant %s (%d)", tenant.Name, tenant.Key.Int()))
		}
	}

	if len(left) > 0 {
		return fmt.Errorf("prefixed objects remain after sweep: %s", strings.Join(left, ", "))
	}
	return nil
}

func sweepTenants(ctx context.Context, client *vergeos.Client) error {
	tenants, err := client.Tenants.List(ctx)
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}
	ids := map[int]string{}
	for _, tenant := range tenants {
		if tenant.IsSnapshot || !HasPrefix(tenant.Name) {
			continue
		}
		ids[tenant.Key.Int()] = tenant.Name
	}
	if len(ids) == 0 {
		return nil
	}

	var errs []error
	record := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	nodes, err := client.TenantNodes.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] tenant nodes endpoint unavailable, skipping")
		nodes = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list tenant nodes: %w", err)
	}
	for _, node := range nodes {
		if _, ok := ids[node.Tenant.Int()]; !ok {
			continue
		}
		id := node.Key.Int()
		log.Printf("[SWEEP] deleting tenant node %d (%s)", id, node.Name)
		record(ignoreNotFound(client.TenantNodes.Delete(ctx, id), "tenant node", id))
	}

	allocations, err := client.TenantStorage.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] tenant storage endpoint unavailable, skipping")
		allocations = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list tenant storage: %w", err)
	}
	for _, allocation := range allocations {
		if _, ok := ids[allocation.Tenant.Int()]; !ok {
			continue
		}
		id := allocation.Key.Int()
		log.Printf("[SWEEP] deleting tenant storage %d", id)
		record(ignoreNotFound(client.TenantStorage.Delete(ctx, id), "tenant storage", id))
	}

	for id, name := range ids {
		log.Printf("[SWEEP] deleting tenant %d (%s)", id, name)
		record(deleteTenant(ctx, client, id))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep tenants: %w", errorsJoin(errs))
	}
	return nil
}

func deleteTenant(ctx context.Context, client *vergeos.Client, id int) error {
	err := client.Tenants.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if offErr := client.Tenants.PowerOff(ctx, id); offErr != nil && !vergeos.IsNotFoundError(offErr) {
		return fmt.Errorf("delete tenant %d: %w (power off: %v)", id, err, offErr)
	}
	return ignoreNotFound(client.Tenants.Delete(ctx, id), "tenant", id)
}

func deleteVM(ctx context.Context, client *vergeos.Client, id int) error {
	err := client.VMs.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if offErr := client.VMs.PowerOff(ctx, id); offErr != nil && !vergeos.IsNotFoundError(offErr) {
		return fmt.Errorf("delete vm %d: %w (power off: %v)", id, err, offErr)
	}
	return ignoreNotFound(client.VMs.Delete(ctx, id), "vm", id)
}

func deleteNetwork(ctx context.Context, client *vergeos.Client, id int) error {
	err := client.Networks.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if offErr := client.Networks.PowerOff(ctx, id); offErr != nil && !vergeos.IsNotFoundError(offErr) {
		return fmt.Errorf("delete network %d: %w (power off: %v)", id, err, offErr)
	}
	return ignoreNotFound(client.Networks.Delete(ctx, id), "network", id)
}

func referencesSweptObject(member string, vmIDs, networkIDs, userIDs, groupIDs map[int]string) bool {
	kind, id, ok := splitRef(member)
	if !ok {
		return false
	}
	switch kind {
	case "vms":
		_, ok = vmIDs[id]
	case "vnets":
		_, ok = networkIDs[id]
	case "users":
		_, ok = userIDs[id]
	case "groups":
		_, ok = groupIDs[id]
	default:
		ok = false
	}
	return ok
}

// permissionBelongsToSweep reports whether a grant should be removed with
// the acceptance-test users and groups. Identity may be the row key or a
// separate identity whose display name is the object name. A grant on the
// swept user or group row is removed too.
func permissionBelongsToSweep(perm vergeos.Permission, userIDs, groupIDs map[int]string) bool {
	identity := perm.Identity.Int()
	if _, ok := userIDs[identity]; ok {
		return true
	}
	if _, ok := groupIDs[identity]; ok {
		return true
	}
	if nameIn(perm.IdentityDisplay, userIDs) || nameIn(perm.IdentityDisplay, groupIDs) {
		return true
	}
	switch perm.Table {
	case "users":
		_, ok := userIDs[int(perm.Row)]
		return ok
	case "groups":
		_, ok := groupIDs[int(perm.Row)]
		return ok
	default:
		return false
	}
}

func nameIn(name string, ids map[int]string) bool {
	if name == "" {
		return false
	}
	for _, candidate := range ids {
		if candidate == name {
			return true
		}
	}
	return false
}

func ownerVMID(owner string) int {
	kind, id, ok := splitRef(owner)
	if !ok || kind != "vms" {
		return 0
	}
	return id
}

func splitRef(ref string) (string, int, bool) {
	kind, idText, ok := strings.Cut(ref, "/")
	if !ok || kind == "" || idText == "" {
		return "", 0, false
	}
	id, err := strconv.Atoi(idText)
	if err != nil {
		return "", 0, false
	}
	return kind, id, true
}

func ignoreNotFound(err error, kind string, id int) error {
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	return fmt.Errorf("delete %s %d: %w", kind, id, err)
}

func errorsJoin(errs []error) error {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}
