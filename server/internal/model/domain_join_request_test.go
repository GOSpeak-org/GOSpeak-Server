package model

import (
	"testing"
)

func TestDomainJoinRequest_StatusConstants(t *testing.T) {
	cases := map[string]string{
		JoinRequestPending:  "pending",
		JoinRequestApproved: "approved",
		JoinRequestRejected: "rejected",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("status constant = %q, want %q", got, want)
		}
	}
}

func TestDomainJoinRequest_IsPending(t *testing.T) {
	if !(&DomainJoinRequest{Status: JoinRequestPending}).IsPending() {
		t.Fatal("pending request must report pending")
	}
	if (&DomainJoinRequest{Status: JoinRequestApproved}).IsPending() {
		t.Fatal("approved request must not report pending")
	}
	if (*DomainJoinRequest)(nil).IsPending() {
		t.Fatal("nil request must not report pending")
	}
}

func TestDomainJoinRequest_TableName(t *testing.T) {
	if got := (DomainJoinRequest{}).TableName(); got != "domain_join_requests" {
		t.Fatalf("table name = %q, want domain_join_requests", got)
	}
}
