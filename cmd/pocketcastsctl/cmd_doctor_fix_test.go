package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"pocketcastsctl/internal/config"
	"testing"
)

func TestRunDoctorApplyRequiresFix(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"doctor", "--apply"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--apply requires --fix") {
		t.Fatalf("stderr missing apply/fix requirement: %q", stderr)
	}
}

func TestRunDoctorQuickFixJSONIncludesSuggestedFixes(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"doctor", "--quick", "--fix", "--json"}, "")
	if code != 0 && code != 1 {
		t.Fatalf("exit code = %d, want 0 or 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"suggested_fixes\"") {
		t.Fatalf("stdout missing suggested_fixes: %q", stdout)
	}
}

func TestDoctorRepairPlanAndApplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, path)
	checks := []doctorCheck{
		{ID: "unrelated_id", Code: "doctor.config.missing", Status: "warn"},
		{Code: "doctor.config.missing", Status: "warn"},
		{ID: "config_file", Code: "doctor.auth.invalid", Status: "fail"},
	}
	want := []doctorFixAction{{Action: doctorRepairConfigInit, Command: cliCommand("config init")}}
	if got := planDoctorFixes(checks, doctorCodeCatalog("")); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("planning mutated config: %v", err)
	}
	results := applyDoctorFixes(checks, doctorCodeCatalog(""))
	if len(results) != 1 || results[0].Action != "config_init" || results[0].Command != want[0].Command || results[0].Status != "ok" {
		t.Fatalf("apply = %+v", results)
	}
	cfg, err := config.Load()
	if err != nil || !reflect.DeepEqual(cfg, config.Default()) {
		t.Fatalf("resulting config = %+v, %v", cfg, err)
	}
	original := []byte(`{"browser":"dia","unknown":"preserve","api_headers":{}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	results = applyDoctorFixes(checks, doctorCodeCatalog(""))
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) || len(results) != 1 || results[0].Message != "config already exists; skipped" {
		t.Fatalf("existing config not preserved: %q %v %+v", got, err, results)
	}
}

func TestDoctorRepairsOnlySupportedFailures(t *testing.T) {
	var checks []doctorCheck
	for code, entry := range doctorCodeCatalog("") {
		if entry.Repair != "" && (entry.Repair != doctorRepairConfigInit || len(entry.Commands) != 1) {
			t.Fatalf("unsupported repair descriptor %s: %+v", code, entry)
		}
		if code != "doctor.config.missing" {
			checks = append(checks, doctorCheck{ID: "config_file", Code: code, Status: "fail"})
		}
	}
	checks = append(checks, doctorCheck{Code: "doctor.config.missing", Status: "ok"})
	if got := planDoctorFixes(checks, doctorCodeCatalog("")); len(got) != 0 {
		t.Fatalf("unsupported/healthy checks planned repairs: %+v", got)
	}
}

func TestDoctorApplyReportsWriteFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("scratch"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvConfigPath, filepath.Join(parent, "config.json"))
	results := applyDoctorFixes([]doctorCheck{{Code: "doctor.config.missing", Status: "warn"}}, doctorCodeCatalog(""))
	if len(results) != 1 || !hasFailedDoctorFix(results) {
		t.Fatalf("failed repair not reported: %+v", results)
	}
}
