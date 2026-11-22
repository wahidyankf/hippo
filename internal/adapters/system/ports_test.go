package system_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// TestProductionBindsEveryPort fails when a port field is left unbound, so an adapter never
// reaches a nil function at run time.
func TestProductionBindsEveryPort(t *testing.T) {
	if missing := unboundFields(reflect.ValueOf(system.Production), "System"); len(missing) > 0 {
		t.Fatalf("unbound port fields: %v", missing)
	}
}

// TestProductionCaptureOutputReadsStandardOutput proves the bound runner returns a command's standard output and
// reports a command that cannot start.
func TestProductionCaptureOutputReadsStandardOutput(t *testing.T) {
	ctx := context.Background()
	if out, err := system.Production.Runner.CaptureOutput(ctx, "echo", "hippo"); err != nil || string(out) != "hippo\n" {
		t.Fatalf("capture %q %v", out, err)
	}
	if _, err := system.Production.Runner.CaptureOutput(ctx, "hippo-no-such-command"); err == nil {
		t.Fatal("a missing command captured output without an error")
	}
}

func unboundFields(value reflect.Value, name string) []string {
	if value.Kind() == reflect.Struct {
		missing := make([]string, 0, value.NumField())
		for index := range value.NumField() {
			missing = append(missing, unboundFields(value.Field(index), name+"."+value.Type().Field(index).Name)...)
		}

		return missing
	}
	if value.Kind() == reflect.String {
		if value.String() == "" {
			return []string{name}
		}

		return nil
	}
	if value.Kind() == reflect.Func || value.Kind() == reflect.Interface || value.Kind() == reflect.Slice {
		if value.IsNil() {
			return []string{name}
		}

		return nil
	}

	return []string{name + " has an unexpected kind " + value.Kind().String()}
}
