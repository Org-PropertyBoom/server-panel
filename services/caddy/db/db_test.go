package db

import "testing"

func TestRowDesired(t *testing.T) {
	if !(Row{IsActive: true, SoftDeleted: false}).Desired() {
		t.Error("active + not-deleted should be desired")
	}
	if (Row{IsActive: false, SoftDeleted: false}).Desired() {
		t.Error("inactive must not be desired")
	}
	if (Row{IsActive: true, SoftDeleted: true}).Desired() {
		t.Error("soft-deleted must not be desired")
	}
}
