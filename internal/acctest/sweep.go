package acctest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/verge-io/govergeos"
)

// Sweep deletes acceptance-test leftovers whose names start with ResourcePrefix.
// Tag memberships and group memberships are removed when they point at those
// objects. A tag category with the prefix is deleted last. VergeOS then
// deletes every tag in that category and every assignment of those tags.
// Objects outside the prefix are left alone.
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
		vmIDs[vm.Key.Int()] = vm.Name
	}
	networkIDs := map[int]string{}
	for _, network := range networks {
		if !HasPrefix(network.Name) {
			continue
		}
		networkIDs[network.Key.Int()] = network.Name
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
		groupIDs[group.Key.Int()] = group.Name
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
		id := member.Key.Int()
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
		id := file.Key.Int()
		log.Printf("[SWEEP] deleting cloud-init file %d (%s)", id, file.Name)
		record(ignoreNotFound(client.CloudInitFiles.Delete(ctx, id), "cloud-init file", id))
	}

	for id, name := range vmIDs {
		log.Printf("[SWEEP] deleting vm %d (%s)", id, name)
		record(deleteVM(ctx, client, id))
	}
	// Profiles are removed after VMs. A VM that still references a profile
	// can make the profile delete fail.
	record(sweepSnapshotProfiles(ctx, client))
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
	record(sweepTags(ctx, client))

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
			left = append(left, fmt.Sprintf("vm %s (%d)", vm.Name, vm.Key.Int()))
		}
	}

	networks, err := client.Networks.List(ctx)
	if err != nil {
		return fmt.Errorf("verify networks: %w", err)
	}
	for _, network := range networks {
		if HasPrefix(network.Name) {
			left = append(left, fmt.Sprintf("network %s (%d)", network.Name, network.Key.Int()))
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
			left = append(left, fmt.Sprintf("group %s (%d)", group.Name, group.Key.Int()))
		}
	}

	files, err := client.CloudInitFiles.List(ctx)
	if err != nil {
		return fmt.Errorf("verify cloud-init files: %w", err)
	}
	for _, file := range files {
		if HasPrefix(file.Name) {
			left = append(left, fmt.Sprintf("cloud-init file %s (%d)", file.Name, file.Key.Int()))
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

	addresses, err := client.TenantExternalIPs.List(ctx)
	if vergeos.IsNotFoundError(err) {
		addresses = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("verify tenant external IPs: %w", err)
	}
	for _, address := range addresses {
		if HasPrefix(address.Hostname) {
			left = append(left, fmt.Sprintf("tenant external IP %s (%d)", address.Hostname, address.Key.Int()))
		}
	}

	profiles, err := client.SnapshotProfiles.List(ctx)
	if vergeos.IsNotFoundError(err) {
		profiles = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("verify snapshot profiles: %w", err)
	}
	for _, profile := range profiles {
		if HasPrefix(profile.Name) {
			left = append(left, fmt.Sprintf("snapshot profile %s (%d)", profile.Name, profile.Key.Int()))
		}
	}

	tags, err := client.Tags.List(ctx)
	if vergeos.IsNotFoundError(err) {
		tags = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("verify tags: %w", err)
	}
	for _, tag := range tags {
		if HasPrefix(tag.Name) {
			left = append(left, fmt.Sprintf("tag %s (%d)", tag.Name, tag.Key.Int()))
		}
	}

	categories, err := client.TagCategories.List(ctx)
	if vergeos.IsNotFoundError(err) {
		categories = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("verify tag categories: %w", err)
	}
	for _, category := range categories {
		if HasPrefix(category.Name) {
			left = append(left, fmt.Sprintf("tag category %s (%d)", category.Name, category.Key.Int()))
		}
	}

	if len(left) > 0 {
		return fmt.Errorf("prefixed objects remain after sweep: %s", strings.Join(left, ", "))
	}
	return nil
}

// sweepSnapshotProfiles removes prefixed snapshot profiles. Periods are
// deleted first. VMs that reference a profile are removed earlier in Sweep.
func sweepSnapshotProfiles(ctx context.Context, client *vergeos.Client) error {
	profiles, err := client.SnapshotProfiles.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] snapshot profiles endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list snapshot profiles: %w", err)
	}
	ids := map[int]string{}
	for _, profile := range profiles {
		if !HasPrefix(profile.Name) {
			continue
		}
		ids[profile.Key.Int()] = profile.Name
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
	for id, name := range ids {
		periods, err := client.SnapshotProfilePeriods.ListByProfile(ctx, id)
		if vergeos.IsNotFoundError(err) {
			periods = nil
			err = nil
		}
		if err != nil {
			return fmt.Errorf("list snapshot profile periods for %d: %w", id, err)
		}
		for _, period := range periods {
			periodID := period.Key.Int()
			log.Printf("[SWEEP] deleting snapshot profile period %d (%s)", periodID, period.Name)
			record(ignoreNotFound(client.SnapshotProfilePeriods.Delete(ctx, periodID), "snapshot profile period", periodID))
		}
		log.Printf("[SWEEP] deleting snapshot profile %d (%s)", id, name)
		record(ignoreNotFound(client.SnapshotProfiles.Delete(ctx, id), "snapshot profile", id))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep snapshot profiles: %w", errorsJoin(errs))
	}
	return nil
}

// sweepTags removes prefixed tags and tag categories. Members of those tags
// are removed first. Deleting a prefixed category cascades to any tag still
// in it and to every assignment of those tags.
func sweepTags(ctx context.Context, client *vergeos.Client) error {
	categories, err := client.TagCategories.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] tag categories endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list tag categories: %w", err)
	}
	tags, err := client.Tags.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] tags endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list tags: %w", err)
	}

	categoryIDs := map[int]string{}
	for _, category := range categories {
		if !HasPrefix(category.Name) {
			continue
		}
		categoryIDs[category.Key.Int()] = category.Name
	}
	ownedTags := map[int]string{}
	prefixedTags := map[int]string{}
	for _, tag := range tags {
		id := tag.Key.Int()
		if HasPrefix(tag.Name) {
			prefixedTags[id] = tag.Name
			ownedTags[id] = tag.Name
		}
		if _, ok := categoryIDs[tag.Category.Int()]; ok {
			ownedTags[id] = tag.Name
		}
	}
	if len(categoryIDs) == 0 && len(prefixedTags) == 0 {
		return nil
	}

	var errs []error
	record := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
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
	for _, tagMember := range tagMembers {
		if _, ok := ownedTags[tagMember.Tag.Int()]; !ok {
			continue
		}
		id := tagMember.Key.Int()
		log.Printf("[SWEEP] deleting tag member %d (tag %d)", id, tagMember.Tag.Int())
		record(ignoreNotFound(client.TagMembers.Delete(ctx, id), "tag member", id))
	}
	for id, name := range prefixedTags {
		log.Printf("[SWEEP] deleting tag %d (%s)", id, name)
		record(ignoreNotFound(client.Tags.Delete(ctx, id), "tag", id))
	}
	for id, name := range categoryIDs {
		log.Printf("[SWEEP] deleting tag category %d (%s); this also deletes its tags and their assignments", id, name)
		record(ignoreNotFound(client.TagCategories.Delete(ctx, id), "tag category", id))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep tags: %w", errorsJoin(errs))
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
	if err := sweepTenantExternalIPs(ctx, client, ids); err != nil {
		return err
	}
	if err := sweepTenantLayer2Networks(ctx, client, ids); err != nil {
		return err
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
		record(deleteTenantNode(ctx, client, id))
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

func sweepTenantExternalIPs(ctx context.Context, client *vergeos.Client, tenantIDs map[int]string) error {
	addresses, err := client.TenantExternalIPs.List(ctx)
	if vergeos.IsNotFoundError(err) {
		log.Printf("[SWEEP] tenant external IPs endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list tenant external IPs: %w", err)
	}
	var errs []error
	for _, address := range addresses {
		_, owned := tenantIDs[address.TenantKey()]
		if !owned && !HasPrefix(address.Hostname) {
			continue
		}
		id := address.Key.Int()
		log.Printf("[SWEEP] deleting tenant external IP %d (%s)", id, address.IP)
		if err := deleteTenantExternalIP(ctx, client, id); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep tenant external IPs: %w", errorsJoin(errs))
	}
	return nil
}

func sweepTenantLayer2Networks(ctx context.Context, client *vergeos.Client, tenantIDs map[int]string) error {
	if len(tenantIDs) == 0 {
		return nil
	}
	rows, err := client.TenantLayer2Networks.List(ctx)
	if listEndpointMissing(err) {
		log.Printf("[SWEEP] tenant layer 2 networks endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list tenant layer 2 networks: %w", err)
	}
	var errs []error
	for _, row := range rows {
		if _, ok := tenantIDs[row.Tenant.Int()]; !ok {
			continue
		}
		id := row.Key.Int()
		log.Printf("[SWEEP] deleting tenant layer 2 network %d (tenant %d network %d)", id, row.Tenant.Int(), row.VNet.Int())
		if err := client.TenantLayer2Networks.Disable(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			if _, getErr := client.TenantLayer2Networks.Get(ctx, id); vergeos.IsNotFoundError(getErr) {
				continue
			}
			errs = append(errs, fmt.Errorf("disable tenant layer 2 network %d: %w", id, err))
			continue
		}
		if err := client.TenantLayer2Networks.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			errs = append(errs, fmt.Errorf("delete tenant layer 2 network %d: %w", id, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep tenant layer 2 networks: %w", errorsJoin(errs))
	}
	return nil
}

func listEndpointMissing(err error) bool {
	if err == nil {
		return false
	}
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr *vergeos.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

func deleteTenantExternalIP(ctx context.Context, client *vergeos.Client, id int) error {
	_, err := client.TenantExternalIPs.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if _, getErr := client.TenantExternalIPs.Get(ctx, id); vergeos.IsNotFoundError(getErr) {
		log.Printf("[SWEEP] tenant external IP %d removed; firewall follow-up: %v", id, err)
		return nil
	}
	return fmt.Errorf("delete tenant external IP %d: %w", id, err)
}

func deleteTenant(ctx context.Context, client *vergeos.Client, id int) error {
	err := client.Tenants.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if offErr := client.Tenants.PowerOff(ctx, id); offErr != nil && !vergeos.IsNotFoundError(offErr) {
		log.Printf("[SWEEP] tenant %d power off: %v", id, offErr)
	}
	if tenant, getErr := client.Tenants.Get(ctx, id); getErr == nil {
		if vnetID := tenant.VNet.Int(); vnetID > 0 {
			if killErr := client.Networks.Kill(ctx, vnetID); killErr != nil && !vergeos.IsNotFoundError(killErr) {
				log.Printf("[SWEEP] tenant %d vnet %d kill: %v", id, vnetID, killErr)
			}
			// Brief wait so Tenants.Delete does not race a still-running vnet.
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				network, netErr := client.Networks.Get(ctx, vnetID)
				if netErr != nil || !network.Running {
					break
				}
				time.Sleep(time.Second)
			}
		}
	}
	return ignoreNotFound(client.Tenants.Delete(ctx, id), "tenant", id)
}

func deleteTenantNode(ctx context.Context, client *vergeos.Client, id int) error {
	err := client.TenantNodes.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if killErr := client.TenantNodes.Kill(ctx, id); killErr != nil && !vergeos.IsNotFoundError(killErr) {
		log.Printf("[SWEEP] tenant node %d kill: %v", id, killErr)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		node, getErr := client.TenantNodes.Get(ctx, id)
		if getErr != nil {
			if vergeos.IsNotFoundError(getErr) {
				return nil
			}
			break
		}
		machineID := node.Machine.Int()
		if machineID <= 0 {
			break
		}
		status, stErr := client.MachineStatus.Get(ctx, machineID)
		if stErr != nil || !status.Running {
			break
		}
		time.Sleep(time.Second)
	}
	return ignoreNotFound(client.TenantNodes.Delete(ctx, id), "tenant node", id)
}

// deleteVM removes a test VM. Delete is refused while the VM is running.
// Empty acceptance VMs have no OS and ignore ACPI, so VMs.PowerOff waits
// out its timeout and the VM stays running. VMs.Kill stops the machine
// immediately, then delete is retried.
func deleteVM(ctx context.Context, client *vergeos.Client, id int) error {
	err := client.VMs.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if killErr := client.VMs.Kill(ctx, id); killErr != nil && !vergeos.IsNotFoundError(killErr) {
		return fmt.Errorf("delete vm %d: %w (kill: %v)", id, err, killErr)
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
