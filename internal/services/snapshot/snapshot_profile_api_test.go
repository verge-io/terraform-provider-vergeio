// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestRequiredRetentionRejectsMissingAndNonPositive(t *testing.T) {
	if _, err := requiredRetention(types.Int64Null()); err == nil {
		t.Fatal("null retention should be rejected")
	}
	if _, err := requiredRetention(types.Int64Unknown()); err == nil {
		t.Fatal("unknown retention should be rejected")
	}
	if _, err := requiredRetention(types.Int64Value(0)); err == nil {
		t.Fatal("zero retention should be rejected")
	}
	if _, err := requiredRetention(types.Int64Value(-1)); err == nil {
		t.Fatal("negative retention should be rejected")
	}
	got, err := requiredRetention(types.Int64Value(604800))
	if err != nil {
		t.Fatal(err)
	}
	if got != 604800 {
		t.Fatalf("retention = %d, want 604800", got)
	}
}

func TestPeriodInputsRequiresRetentionAndUniqueNames(t *testing.T) {
	_, err := periodInputs([]periodModel{{
		Name:      types.StringValue("nightly"),
		Frequency: types.StringValue("daily"),
	}})
	if err == nil {
		t.Fatal("missing retention should be rejected before an API call")
	}

	_, err = periodInputs([]periodModel{
		{
			Name:      types.StringValue("nightly"),
			Frequency: types.StringValue("daily"),
			Retention: types.Int64Value(604800),
		},
		{
			Name:      types.StringValue("nightly"),
			Frequency: types.StringValue("daily"),
			Retention: types.Int64Value(604800),
		},
	})
	if err == nil {
		t.Fatal("duplicate period names should be rejected")
	}

	got, err := periodInputs([]periodModel{{
		Name:      types.StringValue(" nightly "),
		Frequency: types.StringValue("daily"),
		Hour:      types.Int32Value(2),
		Minute:    types.Int32Value(0),
		Retention: types.Int64Value(604800),
		Quiesce:   types.BoolValue(true),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "nightly" || got[0].Retention != 604800 {
		t.Fatalf("period = %+v", got)
	}
	if got[0].Minute == nil || *got[0].Minute != 0 {
		t.Fatalf("minute = %v, want 0", got[0].Minute)
	}
	if got[0].Hour == nil || *got[0].Hour != 2 {
		t.Fatalf("hour = %v, want 2", got[0].Hour)
	}
	if got[0].Quiesce == nil || !*got[0].Quiesce {
		t.Fatal("quiesce should be sent when set")
	}
	if got[0].SkipMissed != nil || got[0].DayOfWeek != nil {
		t.Fatal("omitted period fields should not be sent")
	}
}

func TestOrderedPeriodsKeepsPriorOrder(t *testing.T) {
	api := []vergeos.SnapshotProfilePeriod{
		{Key: 2, Name: "weekly", Frequency: "weekly", Retention: 2419200},
		{Key: 1, Name: "nightly", Frequency: "daily", Retention: 604800},
	}
	prior := []periodModel{
		{Name: types.StringValue("nightly")},
		{Name: types.StringValue("weekly")},
	}
	got, err := orderedPeriods(4, prior, api)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name.ValueString() != "nightly" || got[1].Name.ValueString() != "weekly" {
		t.Fatalf("order = %s, %s", got[0].Name.ValueString(), got[1].Name.ValueString())
	}
	if got[0].Retention.ValueInt64() != 604800 || got[0].Key.ValueString() != "1" {
		t.Fatalf("nightly = %+v", got[0])
	}
	if !got[0].DayOfWeek.IsNull() || !got[0].MaxTier.IsNull() {
		t.Fatal("blank day_of_week and max_tier should stay null")
	}
}

func TestPeriodUpdateRequestSendsChangedRetentionOnly(t *testing.T) {
	current := vergeos.SnapshotProfilePeriod{
		Name:      "nightly",
		Frequency: "daily",
		Hour:      2,
		Minute:    0,
		Retention: 604800,
		Quiesce:   true,
	}
	minute := 0
	hour := 2
	quiesce := true
	req := periodUpdateRequest(current, periodInput{
		Name:      "nightly",
		Frequency: "daily",
		Hour:      &hour,
		Minute:    &minute,
		Retention: 604800,
		Quiesce:   &quiesce,
	})
	if req != nil {
		t.Fatalf("unchanged period produced an update: %+v", req)
	}

	req = periodUpdateRequest(current, periodInput{
		Name:      "nightly",
		Frequency: "daily",
		Retention: 1209600,
	})
	if req == nil || req.Retention == nil || *req.Retention != 1209600 {
		t.Fatalf("retention update = %+v", req)
	}
	if req.Frequency != nil || req.Hour != nil || req.Quiesce != nil {
		t.Fatalf("unchanged fields were included: %+v", req)
	}
}
