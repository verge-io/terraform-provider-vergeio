// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ vergeio.IClient = &PermissionApi{}

func NewPermissionApi(c *vergeio.Client) *PermissionApi {
	sdk, _ := vergeos.NewClient(c.SDKOptions()...)
	return &PermissionApi{
		name:   "Permission Api",
		client: c,
		sdk:    sdk,
	}
}

// PermissionApi is the govergeos PermissionService client for vergeio_permission.
// UserService and GroupService resolve the grantee. VergeOS stores the grant
// against an identity key, which is a field on the user or group and is not
// always the row key those resources publish.
type PermissionApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (api *PermissionApi) Name() string {
	return api.name
}

func permissionCreateRequest(data *PermissionResourceModel, identity int, row int64) *vergeos.PermissionCreateRequest {
	return &vergeos.PermissionCreateRequest{
		Identity: identity,
		Table:    data.Table.ValueString(),
		Row:      row,
		List:     vergeio.KnownBool(data.List),
		Read:     vergeio.KnownBool(data.Read),
		Create:   vergeio.KnownBool(data.Create),
		Modify:   vergeio.KnownBool(data.Modify),
		Delete:   vergeio.KnownBool(data.Delete),
	}
}

func permissionUpdateRequest(plan, state *PermissionResourceModel) *vergeos.PermissionUpdateRequest {
	req := &vergeos.PermissionUpdateRequest{
		List:   vergeio.ChangedBool(plan.List, state.List),
		Read:   vergeio.ChangedBool(plan.Read, state.Read),
		Create: vergeio.ChangedBool(plan.Create, state.Create),
		Modify: vergeio.ChangedBool(plan.Modify, state.Modify),
		Delete: vergeio.ChangedBool(plan.Delete, state.Delete),
	}
	if req.List == nil && req.Read == nil && req.Create == nil && req.Modify == nil && req.Delete == nil {
		return nil
	}
	return req
}

func permissionRow(data *PermissionResourceModel) (int64, error) {
	if data.ObjectID.IsNull() || data.ObjectID.IsUnknown() {
		return 0, nil
	}
	row := data.ObjectID.ValueInt64()
	if row <= 0 {
		return 0, fmt.Errorf("object_id must be a positive row key")
	}
	return row, nil
}

func applyPermission(data *PermissionResourceModel, perm *vergeos.Permission) {
	data.Id = idString(perm.Key.Int())
	data.Table = types.StringValue(perm.Table)
	if perm.Row == 0 {
		data.ObjectID = types.Int64Null()
	} else {
		data.ObjectID = types.Int64Value(perm.Row)
	}
	data.List = types.BoolValue(perm.List)
	data.Read = types.BoolValue(perm.Read)
	data.Create = types.BoolValue(perm.Create)
	data.Modify = types.BoolValue(perm.Modify)
	data.Delete = types.BoolValue(perm.Delete)
}

func (api *PermissionApi) createPermission(ctx context.Context, data *PermissionResourceModel) error {
	identity, err := api.identityFor(ctx, data)
	if err != nil {
		return err
	}
	row, err := permissionRow(data)
	if err != nil {
		return err
	}
	req := permissionCreateRequest(data, identity, row)

	var perm *vergeos.Permission
	if row <= 0 {
		// PermissionService.Create rejects row <= 0. VergeOS uses row 0 for a
		// grant on the whole table, so that grant is posted here and then
		// read back through PermissionService.
		perm, err = api.createTablePermission(ctx, data, req)
	} else {
		perm, err = api.sdk.Permissions.Create(ctx, req)
		if perm != nil {
			data.Id = idString(perm.Key.Int())
		}
	}
	if err != nil {
		return err
	}
	applyPermission(data, perm)
	tflog.Debug(ctx, fmt.Sprintf("created permission %s", data.Id.ValueString()))
	return nil
}

func (api *PermissionApi) createTablePermission(ctx context.Context, data *PermissionResourceModel, req *vergeos.PermissionCreateRequest) (*vergeos.Permission, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := api.client.Post(ctx, "api/v4/permissions", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	id, err := readCreatedKey(resp.Body)
	if err != nil {
		return nil, err
	}
	data.Id = idString(id)
	perm, err := api.sdk.Permissions.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return perm, nil
}

func (api *PermissionApi) readPermission(ctx context.Context, data *PermissionResourceModel) error {
	id, err := parseID(data.Id, "permission")
	if err != nil {
		return err
	}
	perm, err := api.sdk.Permissions.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := api.assignGrantee(ctx, data, perm.Identity.Int()); err != nil {
		return err
	}
	applyPermission(data, perm)
	tflog.Debug(ctx, fmt.Sprintf("read permission %d", id))
	return nil
}

func (api *PermissionApi) updatePermission(ctx context.Context, plan, state *PermissionResourceModel) error {
	id, err := parseID(state.Id, "permission")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req := permissionUpdateRequest(plan, state)
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("permission %d rights are unchanged", id))
		return nil
	}
	if _, err := api.sdk.Permissions.Update(ctx, id, req); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("updated permission %d", id))
	return nil
}

func (api *PermissionApi) deletePermission(ctx context.Context, data *PermissionResourceModel) error {
	id, err := parseID(data.Id, "permission")
	if err != nil {
		return err
	}
	if err := api.sdk.Permissions.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted permission %d", id))
	return nil
}

func (api *PermissionApi) identityFor(ctx context.Context, data *PermissionResourceModel) (int, error) {
	userSet := !data.UserID.IsNull() && !data.UserID.IsUnknown() && strings.TrimSpace(data.UserID.ValueString()) != ""
	groupSet := !data.GroupID.IsNull() && !data.GroupID.IsUnknown() && strings.TrimSpace(data.GroupID.ValueString()) != ""
	if userSet == groupSet {
		return 0, errors.New("set one of user_id or group_id")
	}
	if userSet {
		key, err := parseID(data.UserID, "user")
		if err != nil {
			return 0, err
		}
		return api.identityOf(ctx, "users", key)
	}
	key, err := parseID(data.GroupID, "group")
	if err != nil {
		return 0, err
	}
	return api.identityOf(ctx, "groups", key)
}

// identityOf reads the identity key VergeOS stores on a user or group.
// UserService and GroupService do not expose that field. When it is missing,
// the row key is the identity, which is what PermissionService documents.
func (api *PermissionApi) identityOf(ctx context.Context, kind string, key int) (int, error) {
	endpoint, err := identityCollection(kind)
	if err != nil {
		return 0, err
	}
	body, err := api.getJSON(ctx, fmt.Sprintf("%s/%d", endpoint, key), &vergeio.Options{Fields: "$key,identity"})
	if err != nil {
		return 0, err
	}
	rows, err := decodeIdentityRows(body)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return key, nil
	}
	if id, ok := flexInt(rows[0].Identity); ok && id > 0 {
		return id, nil
	}
	if rowKey, ok := flexInt(rows[0].Key); ok && rowKey > 0 {
		return rowKey, nil
	}
	return key, nil
}

// assignGrantee stores the user or group row key that owns this permission.
// A configured key is kept when its identity still matches, so a refresh does
// not replace the grant just because the identity key differs from the row key.
func (api *PermissionApi) assignGrantee(ctx context.Context, data *PermissionResourceModel, identity int) error {
	if key, ok := positiveID(data.UserID); ok {
		got, err := api.identityOf(ctx, "users", key)
		if err != nil && !notFound(err) {
			return err
		}
		if err == nil && got == identity {
			data.UserID = idString(key)
			data.GroupID = types.StringNull()
			return nil
		}
	}
	if key, ok := positiveID(data.GroupID); ok {
		got, err := api.identityOf(ctx, "groups", key)
		if err != nil && !notFound(err) {
			return err
		}
		if err == nil && got == identity {
			data.GroupID = idString(key)
			data.UserID = types.StringNull()
			return nil
		}
	}

	userKey, groupKey, err := api.findGrantee(ctx, identity)
	if err != nil {
		return err
	}
	if userKey > 0 {
		data.UserID = idString(userKey)
		data.GroupID = types.StringNull()
		return nil
	}
	if groupKey > 0 {
		data.GroupID = idString(groupKey)
		data.UserID = types.StringNull()
		return nil
	}
	return fmt.Errorf("permission identity %d does not match a user or group", identity)
}

func (api *PermissionApi) findGrantee(ctx context.Context, identity int) (int, int, error) {
	users, userErr := api.listIdentityKeys(ctx, "users", identity)
	if userErr != nil && !ignorableListError(userErr) {
		return 0, 0, userErr
	}
	groups, groupErr := api.listIdentityKeys(ctx, "groups", identity)
	if groupErr != nil && !ignorableListError(groupErr) {
		return 0, 0, groupErr
	}
	if len(users) == 1 && len(groups) == 0 {
		return users[0], 0, nil
	}
	if len(groups) == 1 && len(users) == 0 {
		return 0, groups[0], nil
	}
	if len(users) > 1 || len(groups) > 1 || (len(users) > 0 && len(groups) > 0) {
		return 0, 0, fmt.Errorf("permission identity %d matches more than one user or group", identity)
	}

	if _, err := api.sdk.Users.Get(ctx, identity); err == nil {
		return identity, 0, nil
	} else if !vergeos.IsNotFoundError(err) && !notFound(err) {
		return 0, 0, err
	}
	if _, err := api.sdk.Groups.Get(ctx, identity); err == nil {
		return 0, identity, nil
	} else if !vergeos.IsNotFoundError(err) && !notFound(err) {
		return 0, 0, err
	}
	return 0, 0, nil
}

func (api *PermissionApi) listIdentityKeys(ctx context.Context, kind string, identity int) ([]int, error) {
	endpoint, err := identityCollection(kind)
	if err != nil {
		return nil, err
	}
	body, err := api.getJSON(ctx, endpoint, &vergeio.Options{
		Fields: "$key,identity",
		Filter: fmt.Sprintf("identity eq %d", identity),
	})
	if err != nil {
		return nil, err
	}
	rows, err := decodeIdentityRows(body)
	if err != nil {
		return nil, err
	}
	return matchingIdentityKeys(rows, identity), nil
}

func identityCollection(kind string) (string, error) {
	switch kind {
	case "users", "groups":
		return "api/v4/" + kind, nil
	default:
		return "", fmt.Errorf("unsupported identity kind %q", kind)
	}
}

func (api *PermissionApi) getJSON(ctx context.Context, endpoint string, params *vergeio.Options) ([]byte, error) {
	resp, err := api.client.Get(ctx, endpoint, params)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

type identityRow struct {
	Key      json.RawMessage `json:"$key"`
	Identity json.RawMessage `json:"identity"`
}

func decodeIdentityRows(body []byte) ([]identityRow, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || string(body) == "null" {
		return nil, nil
	}
	if body[0] == '[' {
		var rows []identityRow
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	var row identityRow
	if err := json.Unmarshal(body, &row); err != nil {
		return nil, err
	}
	return []identityRow{row}, nil
}

func matchingIdentityKeys(rows []identityRow, identity int) []int {
	var matched []int
	sawIdentity := false
	for _, row := range rows {
		key, ok := flexInt(row.Key)
		if !ok || key <= 0 {
			continue
		}
		if id, ok := flexInt(row.Identity); ok {
			sawIdentity = true
			if id == identity {
				matched = append(matched, key)
			}
		}
	}
	if sawIdentity || len(rows) != 1 {
		return matched
	}
	if key, ok := flexInt(rows[0].Key); ok && key > 0 {
		return []int{key}
	}
	return nil
}

func flexInt(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func readCreatedKey(r io.Reader) (int, error) {
	body, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return 0, err
	}
	var payload struct {
		Key json.RawMessage `json:"$key"`
		Err string          `json:"err"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("permission create response: %w", err)
	}
	if payload.Err != "" {
		return 0, errors.New(payload.Err)
	}
	id, ok := flexInt(payload.Key)
	if !ok || id <= 0 {
		return 0, fmt.Errorf("permission create response missing $key")
	}
	return id, nil
}

func notFound(err error) bool {
	if err == nil {
		return false
	}
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr vergeio.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func ignorableListError(err error) bool {
	var apiErr vergeio.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 400 || apiErr.StatusCode == 422 || apiErr.StatusCode == 404
	}
	return notFound(err)
}
