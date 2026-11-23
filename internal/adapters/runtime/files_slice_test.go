package runtime //nolint:testpackage // Slice (a) doubles run inside the production package, as run_test.go does.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	filesSliceToken = "0123456789abcdef0123456789abcdef"
	filesSliceDead  = "fedcba9876543210fedcba9876543210"
)

func filesSliceSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())

	return host, host.Port()
}

func filesSliceMetadata() ReservationMetadata {
	return ReservationMetadata{
		Source: "hippo", Tags: map[string]string{"plan": "files-slice"}, Tier: "light",
	}
}

func TestWriteReservationMetadataRoundTrip(t *testing.T) {
	_, port := filesSliceSystem()
	registered := time.Unix(1700000000, 0).UTC()
	deadline := registered.Add(time.Hour)
	err := writeReservationMetadata(
		port, "/root", filesSliceToken, filesSliceMetadata(),
		ReservationVector{CPU: 1, MemoryBytes: 2}, ReservationVector{CPU: 3, MemoryBytes: 4},
		registered, deadline,
	)
	if err != nil {
		t.Fatalf("write reservation metadata: %v", err)
	}
	value, present, err := readReservationMetadata(port, "/root", filesSliceToken)
	if err != nil || !present {
		t.Fatalf("read reservation metadata = %v, %v, want present", value, err)
	}
	if value.RunID != filesSliceToken || value.Source != "hippo" || value.Tier != "light" {
		t.Fatalf("read reservation metadata = %+v, want token hippo light", value)
	}
	if value.Minimum != (ReservationVector{CPU: 1, MemoryBytes: 2}) {
		t.Fatalf("read reservation metadata minimum = %+v", value.Minimum)
	}
}

func TestWriteReservationMetadataRejectsInvalidToken(t *testing.T) {
	_, port := filesSliceSystem()
	err := writeReservationMetadata(
		port, "/root", "../escape", filesSliceMetadata(),
		ReservationVector{}, ReservationVector{}, time.Now(), time.Now(),
	)
	if err == nil {
		t.Fatal("write reservation metadata with invalid token = nil, want error")
	}
}

func TestWriteReservationMetadataOverwritesExisting(t *testing.T) {
	_, port := filesSliceSystem()
	now := time.Unix(1700000000, 0).UTC()
	if err := writeReservationMetadata(
		port, "/root", filesSliceToken, filesSliceMetadata(),
		ReservationVector{}, ReservationVector{}, now, now,
	); err != nil {
		t.Fatalf("first write reservation metadata: %v", err)
	}
	updated := filesSliceMetadata()
	updated.Tier = "heavy"
	if err := writeReservationMetadata(
		port, "/root", filesSliceToken, updated,
		ReservationVector{}, ReservationVector{}, now, now,
	); err != nil {
		t.Fatalf("second write reservation metadata: %v", err)
	}
	value, present, err := readReservationMetadata(port, "/root", filesSliceToken)
	if err != nil || !present || value.Tier != "heavy" {
		t.Fatalf("reread reservation metadata = %+v, %v, %v, want heavy tier", value, present, err)
	}
}

func TestReadReservationMetadataAbsent(t *testing.T) {
	_, port := filesSliceSystem()
	_, present, err := readReservationMetadata(port, "/root", filesSliceToken)
	if err != nil || present {
		t.Fatalf("read absent reservation metadata = %v, %v, want absent", present, err)
	}
}

func TestRemoveReservationMetadata(t *testing.T) {
	_, port := filesSliceSystem()
	now := time.Unix(1700000000, 0).UTC()
	if err := writeReservationMetadata(
		port, "/root", filesSliceToken, filesSliceMetadata(),
		ReservationVector{}, ReservationVector{}, now, now,
	); err != nil {
		t.Fatalf("write reservation metadata: %v", err)
	}
	if err := removeReservationMetadata(port, "/root", filesSliceToken); err != nil {
		t.Fatalf("remove reservation metadata: %v", err)
	}
	if err := removeReservationMetadata(port, "/root", filesSliceToken); err != nil {
		t.Fatalf("remove absent reservation metadata: %v", err)
	}
}

func TestPruneReservationMetadataKeepsLiveRemovesDead(t *testing.T) {
	_, port := filesSliceSystem()
	now := time.Unix(1700000000, 0).UTC()
	for _, token := range []string{filesSliceToken, filesSliceDead} {
		if err := writeReservationMetadata(
			port, "/root", token, filesSliceMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		); err != nil {
			t.Fatalf("write reservation metadata %s: %v", token, err)
		}
	}
	ledger := reservationLedger{
		Owners: []coordination.ReservationOwner{{Token: filesSliceToken}},
	}
	if err := pruneReservationMetadata(port, "/root", ledger); err != nil {
		t.Fatalf("prune reservation metadata: %v", err)
	}
	if _, present, err := readReservationMetadata(port, "/root", filesSliceToken); err != nil || !present {
		t.Fatalf("live reservation metadata = %v, %v, want present", present, err)
	}
	if _, present, err := readReservationMetadata(port, "/root", filesSliceDead); err != nil || present {
		t.Fatalf("dead reservation metadata = %v, %v, want absent", present, err)
	}
}

func TestReservationEntryReadsMetadata(t *testing.T) {
	_, port := filesSliceSystem()
	now := time.Unix(1700000000, 0).UTC()
	if err := writeReservationMetadata(
		port, "/root", filesSliceToken, filesSliceMetadata(),
		ReservationVector{CPU: 1}, ReservationVector{CPU: 2}, now, now,
	); err != nil {
		t.Fatalf("write reservation metadata: %v", err)
	}
	entry, err := reservationEntry(
		port, "/root", filesSliceToken, "active", 0,
		policy.TaskEphemeral, policy.ProfileName("default"),
		ReservationVector{CPU: 1}, ReservationVector{CPU: 1},
	)
	if err != nil {
		t.Fatalf("reservation entry: %v", err)
	}
	if entry.Legacy || entry.Source != "hippo" {
		t.Fatalf("reservation entry = %+v, want metadata-sourced", entry)
	}
	legacy, err := reservationEntry(
		port, "/root", filesSliceDead, "waiting", 1,
		policy.TaskService, policy.ProfileName("default"),
		ReservationVector{}, ReservationVector{},
	)
	if err != nil || !legacy.Legacy {
		t.Fatalf("absent reservation entry = %+v, %v, want legacy", legacy, err)
	}
}

func TestWriteSafetyReceiptSkipsEmptySource(t *testing.T) {
	host, port := filesSliceSystem()
	now := time.Unix(1700000000, 0).UTC()
	if err := writeSafetyReceipt(
		port, "/root", filesSliceToken, "never-started", "admission-deadline",
		ReservationMetadata{}, policy.TaskEphemeral, now,
	); err != nil {
		t.Fatalf("write safety receipt with empty source: %v", err)
	}
	if len(host.Files.Contents) != 0 {
		t.Fatalf("receipt files = %d, want none", len(host.Files.Contents))
	}
}

func TestWriteSafetyReceiptRoundTrip(t *testing.T) {
	host, port := filesSliceSystem()
	now := time.Unix(1700000000, 0).UTC()
	if err := writeSafetyReceipt(
		port, "/root", filesSliceToken, "never-started", "admission-deadline",
		filesSliceMetadata(), policy.TaskEphemeral, now,
	); err != nil {
		t.Fatalf("write safety receipt: %v", err)
	}
	names := make([]string, 0, len(host.Files.Contents))
	for name := range host.Files.Contents {
		names = append(names, name)
	}
	if len(names) != 1 || !strings.HasSuffix(names[0], "-"+filesSliceToken+".json") {
		t.Fatalf("receipt files = %v, want one token receipt", names)
	}
	var receipt SafetyReceipt
	if err := json.Unmarshal(host.Files.Contents[names[0]], &receipt); err != nil {
		t.Fatalf("decode safety receipt: %v", err)
	}
	if receipt.RunID != filesSliceToken || receipt.Source != "hippo" {
		t.Fatalf("safety receipt = %+v, want token hippo", receipt)
	}
}

func TestDisplayRegularPath(t *testing.T) {
	host, port := filesSliceSystem()
	present, err := displayRegularPath(port, "/root/missing.json")
	if err != nil || present {
		t.Fatalf("display regular path missing = %v, %v, want absent", present, err)
	}
	host.Files.Contents["/root/record.json"] = []byte("{}")
	host.Files.Modes["/root/record.json"] = 0o600
	present, err = displayRegularPath(port, "/root/record.json")
	if err != nil || !present {
		t.Fatalf("display regular path record = %v, %v, want present", present, err)
	}
	host.Files.Dirs["/root/dir"] = true
	if _, err = displayRegularPath(port, "/root/dir"); err == nil {
		t.Fatal("display regular path directory = nil, want error")
	}
}

func TestReadDisplayCensusRecord(t *testing.T) {
	host, port := filesSliceSystem()
	var decoded struct {
		Name string `json:"name"`
	}
	present, err := readDisplayCensusRecord(port, "/root/missing.json", &decoded)
	if err != nil || present {
		t.Fatalf("read display census record missing = %v, %v, want absent", present, err)
	}
	host.Files.Contents["/root/record.json"] = []byte(`{"name":"candidate","extra":true}`)
	if _, err = readDisplayCensusRecord(port, "/root/record.json", &decoded); err == nil {
		t.Fatal("read display census record unknown field = nil, want error")
	}
	host.Files.Contents["/root/record.json"] = []byte(`{"name":"candidate"} {"name":"second"}`)
	if _, err = readDisplayCensusRecord(port, "/root/record.json", &decoded); err == nil {
		t.Fatal("read display census record multi-value = nil, want error")
	}
	host.Files.Contents["/root/record.json"] = []byte(`{"name":"candidate"}`)
	present, err = readDisplayCensusRecord(port, "/root/record.json", &decoded)
	if err != nil || !present || decoded.Name != "candidate" {
		t.Fatalf("read display census record = %+v, %v, %v, want candidate", decoded, present, err)
	}
}

func TestDisplayDirectoryEntries(t *testing.T) {
	host, port := filesSliceSystem()
	entries, err := displayDirectoryEntries(port, "/root/missing")
	if err != nil || len(entries) != 0 {
		t.Fatalf("display directory entries missing = %v, %v, want empty", entries, err)
	}
	host.Files.Contents["/root/file"] = []byte("x")
	if _, err = displayDirectoryEntries(port, "/root/file"); err == nil {
		t.Fatal("display directory entries file = nil, want error")
	}
	host.Files.Dirs["/root/dir"] = true
	host.Files.Contents["/root/dir/a.json"] = []byte("{}")
	entries, err = displayDirectoryEntries(port, "/root/dir")
	if err != nil || len(entries) != 1 {
		t.Fatalf("display directory entries = %v, %v, want one entry", entries, err)
	}
}

func TestLiveExclusiveSessionOwnersWithoutDirectory(t *testing.T) {
	_, port := filesSliceSystem()
	owners, err := liveExclusiveSessionOwners(port, "/root")
	if err != nil || len(owners) != 0 {
		t.Fatalf("live exclusive session owners = %v, %v, want empty", owners, err)
	}
}

func TestLiveExclusiveHeavyOwnerAbsent(t *testing.T) {
	_, port := filesSliceSystem()
	_, live, err := liveExclusiveHeavyOwner(port, "/root")
	if err != nil || live {
		t.Fatalf("live exclusive heavy owner = %v, %v, want absent", live, err)
	}
}
