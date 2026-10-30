package coordination_test

import (
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

func TestJudgeReservationIdentity(t *testing.T) {
	present := func(device, inode uint64) coordination.ReservationIdentityEvidence {
		return coordination.ReservationIdentityEvidence{Present: true, Device: device, Inode: inode}
	}
	absent := coordination.ReservationIdentityEvidence{}
	cases := []struct {
		name                          string
		expectedDevice, expectedInode uint64
		lock, anchor                  coordination.ReservationIdentityEvidence
		want                          coordination.ReservationIdentityJudgement
	}{
		{name: "no expectation and nothing present is absent", lock: absent, anchor: absent, want: coordination.ReservationIdentityAbsent},
		{name: "no expectation with only the lock present probes", lock: present(7, 9), anchor: absent, want: coordination.ReservationIdentityProbe},
		{name: "no expectation with only the anchor present probes", lock: absent, anchor: present(7, 9), want: coordination.ReservationIdentityProbe},
		{name: "no expectation with agreeing paths probes", lock: present(7, 9), anchor: present(7, 9), want: coordination.ReservationIdentityProbe},
		{name: "no expectation with disagreeing devices stays closed", lock: present(7, 9), anchor: present(8, 9), want: coordination.ReservationIdentityIncomplete},
		{name: "no expectation with disagreeing inodes stays closed", lock: present(7, 9), anchor: present(7, 10), want: coordination.ReservationIdentityIncomplete},
		{name: "exact lock match probes first", expectedDevice: 7, expectedInode: 9, lock: present(7, 9), anchor: present(7, 9), want: coordination.ReservationIdentityProbe},
		{name: "exact anchor match probes when the lock is missing", expectedDevice: 7, expectedInode: 9, lock: absent, anchor: present(7, 9), want: coordination.ReservationIdentityProbe},
		{name: "exact lock match probes when the anchor is replaced", expectedDevice: 7, expectedInode: 9, lock: present(7, 9), anchor: present(7, 10), want: coordination.ReservationIdentityProbe},
		{name: "exact anchor match probes when the lock is replaced", expectedDevice: 7, expectedInode: 9, lock: present(8, 10), anchor: present(7, 9), want: coordination.ReservationIdentityProbe},
		{name: "intact pair on one changed device probes the lock", expectedDevice: 7, expectedInode: 9, lock: present(8, 9), anchor: present(8, 9), want: coordination.ReservationIdentityProbeAfterDeviceChange},
		{name: "single drifted lock is never probed", expectedDevice: 7, expectedInode: 9, lock: present(8, 9), anchor: absent, want: coordination.ReservationIdentityIncomplete},
		{name: "single drifted anchor is never probed", expectedDevice: 7, expectedInode: 9, lock: absent, anchor: present(8, 9), want: coordination.ReservationIdentityIncomplete},
		{name: "drifted lock with a replaced anchor stays closed", expectedDevice: 7, expectedInode: 9, lock: present(8, 9), anchor: present(8, 10), want: coordination.ReservationIdentityIncomplete},
		{name: "same inode on two devices stays closed", expectedDevice: 7, expectedInode: 9, lock: present(8, 9), anchor: present(10, 9), want: coordination.ReservationIdentityIncomplete},
		{name: "missing pair with an expectation stays closed", expectedDevice: 7, expectedInode: 9, lock: absent, anchor: absent, want: coordination.ReservationIdentityIncomplete},
		{name: "replaced pair with an expectation stays closed", expectedDevice: 7, expectedInode: 9, lock: present(7, 10), anchor: present(7, 10), want: coordination.ReservationIdentityIncomplete},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := coordination.JudgeReservationIdentity(test.expectedDevice, test.expectedInode, test.lock, test.anchor); got != test.want {
				t.Fatalf("judgement=%v, want %v", got, test.want)
			}
		})
	}
}

func TestIncompleteIdentityCauseCarriesNoIdentity(t *testing.T) {
	cause := coordination.IncompleteReservationIdentityCause
	if cause == "" {
		t.Fatal("incomplete identity cause is empty")
	}
	for _, leaked := range []string{"/", ".lock", ".anchor"} {
		if strings.Contains(cause, leaked) {
			t.Fatalf("incomplete identity cause names a path: %q", cause)
		}
	}
}
