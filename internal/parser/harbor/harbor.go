// Package harbor parses Harbor (harbor-framework/harbor) job output.
//
// A `harbor run` writes jobs/<job-name>/<trial-name>/result.json for every
// trial, next to a job-level jobs/<job-name>/result.json. The trial files are
// the source of truth: the job-level file carries only pre-aggregated stats and
// no per-trial data, so it is recognized and skipped.
package harbor

import (
	"math"
	"path/filepath"
	"time"

	"github.com/kubeshop/testkube-ai-metrics-parser/internal/lineprotocol"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/naming"
	"github.com/kubeshop/testkube-ai-metrics-parser/internal/parser"
)

const measurement = "harbor_eval"

// adhocDataset is the name Harbor itself uses for trials without a source.
const adhocDataset = "adhoc"

type harborParser struct{}

func (harborParser) Name() string { return "harbor" }

func (harborParser) DefaultInputs() []string { return []string{"jobs/*/*/result.json"} }

// SelectLatest keeps every trial of the newest job instead of the single
// newest file: one Harbor run is a whole job directory. Job names default to a
// timestamp, so the lexicographically last job directory is the newest.
func (harborParser) SelectLatest(files []string) []string {
	newest := ""
	for _, f := range files {
		if dir := jobDir(f); dir > newest {
			newest = dir
		}
	}
	var out []string
	for _, f := range files {
		if jobDir(f) == newest {
			out = append(out, f)
		}
	}
	return out
}

// jobDir maps jobs/<job>/<trial>/result.json to jobs/<job>.
func jobDir(trialResult string) string {
	return filepath.Dir(filepath.Dir(trialResult))
}

// trial is the subset of Harbor's TrialResult the parser reads.
type trial struct {
	TrialName string  `json:"trial_name"`
	Source    *string `json:"source"`
	AgentInfo struct {
		Name      string `json:"name"`
		ModelInfo *struct {
			Name     string  `json:"name"`
			Provider *string `json:"provider"`
		} `json:"model_info"`
	} `json:"agent_info"`
	AgentResult    *usage `json:"agent_result"`
	VerifierResult *struct {
		Rewards map[string]float64 `json:"rewards"`
	} `json:"verifier_result"`
	ExceptionInfo  map[string]any `json:"exception_info"`
	StartedAt      string         `json:"started_at"`
	FinishedAt     string         `json:"finished_at"`
	AgentExecution *timing        `json:"agent_execution"`
	// StepResults replaces the top-level agent fields in multi-step trials.
	StepResults []struct {
		AgentResult    *usage  `json:"agent_result"`
		AgentExecution *timing `json:"agent_execution"`
	} `json:"step_results"`
}

type usage struct {
	InputTokens  *int64   `json:"n_input_tokens"`
	CacheTokens  *int64   `json:"n_cache_tokens"`
	OutputTokens *int64   `json:"n_output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

func (u *usage) reported() bool {
	return u != nil && (u.InputTokens != nil || u.CacheTokens != nil || u.OutputTokens != nil || u.CostUSD != nil)
}

type timing struct {
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// series accumulates the trials of one agent+model+dataset identity.
type series struct {
	tags map[string]string

	trials, passed, errors int64
	rewardSums             map[string]float64

	inputTokens, cacheTokens, outputTokens int64
	haveInput, haveCache, haveOutput       bool
	cost                                   float64
	haveCost                               bool

	durationMs, agentDurationMs     int64
	haveDuration, haveAgentDuration bool
}

// Parse folds every matched trial result into one record per
// agent+model+dataset, the same identity Harbor groups its own job stats by.
//
// A trial without a reward (the agent or verifier crashed) counts as 0 in the
// reward means, so a crash can never raise the score. A trial passes when the
// verifier reported rewards and every one of them is at least 1. Token, cost
// and duration fields are emitted only when at least one trial reported them.
func (p harborParser) Parse(in parser.Input) ([]lineprotocol.Record, error) {
	all := map[string]*series{}
	var order []string

	collect := func(t trial) {
		if t.TrialName == "" {
			return // the job-level result.json, or some other JSON file
		}

		structural := map[string]string{"dataset": adhocDataset}
		if t.Source != nil && *t.Source != "" {
			structural["dataset"] = *t.Source
		}
		if t.AgentInfo.Name != "" {
			structural["agent"] = t.AgentInfo.Name
		}
		if m := t.AgentInfo.ModelInfo; m != nil && m.Name != "" {
			structural["model"] = m.Name
			if m.Provider != nil && *m.Provider != "" {
				structural["model"] = *m.Provider + "/" + m.Name
			}
		}

		id := structural["agent"] + "\x00" + structural["model"] + "\x00" + structural["dataset"]
		s, ok := all[id]
		if !ok {
			s = &series{tags: parser.MergeTags(in.Tags, structural), rewardSums: map[string]float64{}}
			all[id] = s
			order = append(order, id)
		}

		s.trials++
		if t.ExceptionInfo != nil {
			s.errors++
		}
		s.addRewards(t)
		s.addUsage(t)
		s.addDurations(t)
	}

	if _, err := parser.EachJSON(in, p, collect); err != nil {
		return nil, err
	}

	out := make([]lineprotocol.Record, 0, len(order))
	for _, id := range order {
		s := all[id]
		out = append(out, lineprotocol.Record{Measurement: measurement, Tags: s.tags, Fields: s.fields()})
	}
	return out, nil
}

func (s *series) addRewards(t trial) {
	if t.VerifierResult == nil || len(t.VerifierResult.Rewards) == 0 {
		return
	}
	passed := true
	for key, value := range t.VerifierResult.Rewards {
		name := naming.Normalize(key)
		if name == "" {
			continue
		}
		s.rewardSums[name] += value
		if value < 1 {
			passed = false
		}
	}
	if passed {
		s.passed++
	}
}

func (s *series) addUsage(t trial) {
	reports := []*usage{t.AgentResult}
	if !t.AgentResult.reported() {
		reports = reports[:0]
		for _, step := range t.StepResults {
			reports = append(reports, step.AgentResult)
		}
	}
	for _, u := range reports {
		if u == nil {
			continue
		}
		if u.InputTokens != nil {
			s.inputTokens += *u.InputTokens
			s.haveInput = true
		}
		if u.CacheTokens != nil {
			s.cacheTokens += *u.CacheTokens
			s.haveCache = true
		}
		if u.OutputTokens != nil {
			s.outputTokens += *u.OutputTokens
			s.haveOutput = true
		}
		if u.CostUSD != nil {
			s.cost += *u.CostUSD
			s.haveCost = true
		}
	}
}

func (s *series) addDurations(t trial) {
	if ms, ok := elapsedMs(t.StartedAt, t.FinishedAt); ok {
		s.durationMs += ms
		s.haveDuration = true
	}

	phases := []*timing{t.AgentExecution}
	if t.AgentExecution == nil {
		phases = phases[:0]
		for _, step := range t.StepResults {
			phases = append(phases, step.AgentExecution)
		}
	}
	for _, phase := range phases {
		if phase == nil {
			continue
		}
		if ms, ok := elapsedMs(phase.StartedAt, phase.FinishedAt); ok {
			s.agentDurationMs += ms
			s.haveAgentDuration = true
		}
	}
}

func (s *series) fields() map[string]lineprotocol.Value {
	fields := map[string]lineprotocol.Value{
		"trials":    lineprotocol.Int(s.trials),
		"passed":    lineprotocol.Int(s.passed),
		"failed":    lineprotocol.Int(s.trials - s.passed),
		"errors":    lineprotocol.Int(s.errors),
		"pass_rate": lineprotocol.Rounded(float64(s.passed) / float64(s.trials)),
		// Always present, so a run where nothing was rewarded reports 0
		// rather than leaving a gap.
		"mean_reward": lineprotocol.Rounded(s.rewardSums["reward"] / float64(s.trials)),
	}
	for name, sum := range s.rewardSums {
		if name != "reward" {
			fields["mean_"+name] = lineprotocol.Rounded(sum / float64(s.trials))
		}
	}
	if s.haveInput {
		fields["input_tokens"] = lineprotocol.Int(s.inputTokens)
	}
	if s.haveCache {
		fields["cache_tokens"] = lineprotocol.Int(s.cacheTokens)
	}
	if s.haveOutput {
		fields["output_tokens"] = lineprotocol.Int(s.outputTokens)
	}
	if s.haveInput || s.haveOutput {
		// Harbor's input count already includes cached tokens.
		fields["total_tokens"] = lineprotocol.Int(s.inputTokens + s.outputTokens)
	}
	if s.haveCost {
		// Finer than the usual 6 decimals: a small job costs fractions of a
		// cent, and rounding would visibly distort it.
		fields["cost_usd"] = lineprotocol.Float(math.Round(s.cost*1e9) / 1e9)
	}
	if s.haveDuration {
		fields["duration_ms"] = lineprotocol.Int(s.durationMs)
	}
	if s.haveAgentDuration {
		fields["agent_duration_ms"] = lineprotocol.Int(s.agentDurationMs)
	}
	return fields
}

// timeLayouts covers Harbor's timestamps: trial files carry a zone, the
// job-level file does not.
var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"}

func parseTime(s string) (time.Time, bool) {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func elapsedMs(start, finish string) (int64, bool) {
	from, ok := parseTime(start)
	if !ok {
		return 0, false
	}
	to, ok := parseTime(finish)
	if !ok || to.Before(from) {
		return 0, false
	}
	return to.Sub(from).Milliseconds(), true
}

func init() { parser.Register(harborParser{}) }
