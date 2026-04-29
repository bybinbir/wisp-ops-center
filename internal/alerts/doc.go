// Package alerts is the Phase 10F-A alert-rule sanity surface.
//
// The package's only artefact today is the test file that walks
// up from the package directory to `infra/prometheus/` and asserts
// the destructive-action safety rules file is present, contains
// every named alert the deployment depends on, and has the
// structural shape Prometheus expects.
//
// Phase 10F-B (or later) is expected to extend this package with
// a real loader once the API server starts exporting the metrics
// the alerts consume. Until then the file is a forward-compatible
// stub against which the alert names + annotations cannot drift.
package alerts
