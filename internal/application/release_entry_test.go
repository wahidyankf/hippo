package application_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
)

func releaseEntryServices(fixture *observationFixture) application.ReleaseServices {
	return application.ReleaseServices{Configuration: fixture, Collector: fixture, Clock: fixture}
}

func TestReleaseEntryCheckOwnsConfigurationProbeAndStrictSampling(t *testing.T) {
	fixture := newObservationFixture()
	fixture.sample.PhysicalMemoryBytes = 64 * 1024 * 1024 * 1024
	fixture.sample.EffectiveMemoryLimitBytes = fixture.sample.PhysicalMemoryBytes
	code, err := releaseEntryServices(fixture).CheckRelease(context.Background(), application.ReleaseCheckRequest{DiskPath: "disk"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	want := []string{"load", "collect", "collect", "wait", "collect", "wait", "collect"}
	if !reflect.DeepEqual(fixture.calls, want) {
		t.Fatalf("release checkcalls=%v, want%v", fixture.calls, want)
	}
}

func TestReleaseEntryMonitorLoadsConfigurationAndOwnsInvocationPreparation(t *testing.T) {
	fixture := newObservationFixture()
	invoked := 0
	code, err := releaseEntryServices(fixture).MonitorRelease(context.Background(), application.ReleaseMonitorRequest{MonitorConfig: application.MonitorConfig{OutputPath: "raw", SummaryPath: "summary", DeploymentRoot: "deployment"}}, func(_ context.Context, config application.MonitorConfig) error {
		invoked++
		if config.Collector != fixture {
			t.Error("collector was not supplied by application")
		}
		return nil
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if invoked != 1 || !reflect.DeepEqual(fixture.calls, []string{"load"}) {
		t.Fatalf("invoked=%d calls=%v", invoked, fixture.calls)
	}
}

type releaseEntryEvidence struct {
	releaseTestRepository

	fixture *observationFixture
}

func (repository *releaseEntryEvidence) ReadReleaseSummary(string) ([]byte, error) {
	repository.fixture.calls = append(repository.fixture.calls, "read")
	return []byte(`{"schemaVersion":3,"sampleCount":1,"availableParallelism":12,"availableNonCompressedEstimateMinBytes":13958643712,"memoryPressureLevelMax":1,"compressorAvailableAll":true,"cpuUtilizationP95Percent":10,"healthFailures":0}`), nil
}

func TestReleaseEntryAssessmentOwnsConfigReadAndAssessmentDecision(t *testing.T) {
	fixture := newObservationFixture()
	services := releaseEntryServices(fixture)
	services.Evidence = &releaseEntryEvidence{fixture: fixture}
	result, code, err := services.AssessRelease(application.ReleaseAssessRequest{SummaryPath: "summary"})
	if code != 0 || err != nil || !result.Assessed || !result.Accepted || result.SchemaVersion != 3 {
		t.Fatalf("assessment=%+v code=%d error=%v", result, code, err)
	}
	if !reflect.DeepEqual(fixture.calls, []string{"load", "read"}) {
		t.Fatalf("assessment calls=%v", fixture.calls)
	}
}
