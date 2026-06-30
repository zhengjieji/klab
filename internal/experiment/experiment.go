// Package experiment defines the experiment lifecycle (setup, run, collect) and
// the provenance manifest captured for every run so results are reproducible.
//
// An experiment is a directory: experiments/<name>/{topology.yaml, setup.sh,
// run.sh, collect.sh}. Every run writes results/<exp>/<ts>/manifest.json.
package experiment

import "time"

// Manifest is written next to every result set so a result pins to the exact
// inputs that produced it.
type Manifest struct {
	Version    int               `json:"version"`
	Experiment string            `json:"experiment"`
	StartedAt  time.Time         `json:"startedAt"`
	Topology   string            `json:"topologyHash"`
	Kernels    map[string]string `json:"kernels"` // node name -> kernel artifact hash
	Command    string            `json:"command"`
	Host       string            `json:"host"`
}

// ManifestVersion is bumped on any breaking change to Manifest (regression-gated).
const ManifestVersion = 1
