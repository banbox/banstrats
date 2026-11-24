// Package yamlcheck loads example YAML overlays through the real banbot unified
// loader and rebuilds the runner.Config the factor entry point derives from a
// run_policy item, so unit tests validate examples without a database or network.
//
// Draft mirrors the draft stage of banbot's factor config builder (decision
// timeframe, params, prices/decision/research/universe/portfolio blocks,
// expressions, manifest) and ends with runner.ValidateStaticConfig. Archive and
// storage resolution, account execution units and resource assembly are out of
// scope: tests inject a synthetic Snapshot when they need to construct builders.
package yamlcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	botconfig "github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banexg/utils"
	"github.com/go-viper/mapstructure/v2"
	"github.com/shopspring/decimal"
)

// Load merges overlays (relative paths) with the real unified YAML loader, without
// the default configs or a data directory.
func Load(dataDir string, paths ...string) (*botconfig.RunSpec, error) {
	spec, err := botconfig.LoadRunSpec(&botconfig.CmdArgs{NoDefault: true, DataDir: dataDir, Configs: botconfig.ArrString(paths)}, false)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// Decode strictly decodes a YAML block into target with the same field matching
// as the banbot factor config builder (case and underscore insensitive, unknown
// keys rejected, floats refused for integer fields).
func Decode(fields map[string]any, target any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: target, ErrorUnused: true,
		MatchName: func(key, field string) bool { return strings.EqualFold(strings.ReplaceAll(key, "_", ""), field) },
		DecodeHook: func(from, to reflect.Type, value any) (any, error) {
			if to == reflect.TypeOf(json.RawMessage{}) {
				return json.Marshal(value)
			}
			if from != nil && (from.Kind() == reflect.Float32 || from.Kind() == reflect.Float64) && to.Kind() >= reflect.Int && to.Kind() <= reflect.Int64 {
				return nil, fmt.Errorf("integer configuration field cannot accept a floating point value")
			}
			if to == reflect.TypeOf((*float64)(nil)) && value != nil {
				v := reflect.ValueOf(value)
				switch v.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					f := float64(v.Int())
					return &f, nil
				case reflect.Float32, reflect.Float64:
					f := v.Float()
					return &f, nil
				}
			}
			if to == reflect.TypeOf(decimal.Decimal{}) {
				switch n := value.(type) {
				case string:
					return decimal.NewFromString(n)
				case float64:
					return decimal.NewFromFloat(n), nil
				case int:
					return decimal.NewFromInt(int64(n)), nil
				}
			}
			return value, nil
		},
	})
	if err != nil {
		return err
	}
	return decoder.Decode(fields)
}

// Draft derives and statically validates the runner.Config of the index-th factor
// run_policy item in a weights replay.
func Draft(spec *botconfig.RunSpec, index int) (runner.Config, error) {
	u := spec.Config()
	var policy *botconfig.PolicyV2
	seen := 0
	for _, item := range u.RunPolicy {
		if item.Engine != botconfig.EngineFactor {
			continue
		}
		if seen == index {
			policy = item
			break
		}
		seen++
	}
	if policy == nil {
		return runner.Config{}, errors.New("yamlcheck: no such factor policy")
	}
	c := runner.Config{Mode: runner.Weights, MaxRecords: 100000, MaxPending: 64, LatencyMS: 1, ExpiryMS: 60000, InitialNAV: 10000, Definition: policy.Name, Factor: research.DefaultMomentumVolConfig()}
	c.Manifest = research.ManifestSpec{CodeRevision: core.Version, Currency: "USD", Portfolio: research.PortfolioDefinition{K: 10, LongNotional: .5, ShortNotional: .5, Mode: factor.Full}}
	if len(u.Root.StakeCurrency) > 0 {
		c.Manifest.Currency = u.Root.StakeCurrency[0]
	}
	timeframe := "1h"
	frames := policy.RunTimeframes
	if len(frames) == 0 {
		frames = u.Root.RunTimeframes
	}
	if len(frames) > 1 {
		return c, errors.New("one decision timeframe is required")
	}
	if len(frames) == 1 {
		timeframe = frames[0]
	}
	seconds, err := utils.TFToSecSafe(timeframe)
	if err != nil || seconds <= 0 {
		return c, fmt.Errorf("invalid decision timeframe %q", timeframe)
	}
	c.DecisionInterval, c.Factor.TimeFrame = int64(seconds)*1000, timeframe
	c.Manifest.Labels = []research.LabelSpec{{Name: timeframe, Kind: research.ExecutableReturn, Horizon: c.DecisionInterval, PeriodsPerYear: 365.25 * 86400000 / float64(c.DecisionInterval)}}
	c.StrategyID, c.AccountID = policy.ID, "default"
	if c.StrategyID == "" {
		c.StrategyID = policy.Name
	}
	for key, value := range policy.Params {
		switch key {
		case "window":
			if value != float64(int(value)) {
				return c, errors.New("factor window must be an integer")
			}
			c.Factor.Window = int(value)
		case "k":
			if value != float64(int(value)) {
				return c, errors.New("factor k must be an integer")
			}
			c.Manifest.Portfolio.K = int(value)
		case "swapPerBars", "holdBars":
			return c, fmt.Errorf("%s has ambiguous semantics", key)
		}
	}
	fields := map[string]any{}
	for key, value := range policy.Factor {
		switch key {
		case "archive", "portfolio", "decision", "research", "universe", "rolling_universe":
		default:
			fields[key] = value
		}
	}
	if err := Decode(fields, &c); err != nil {
		return c, err
	}
	for _, key := range []string{"universe", "rolling_universe"} {
		if raw, ok := policy.Factor[key].(map[string]any); ok {
			var rolling factor.RollingUniverseConfig
			if err := Decode(raw, &rolling); err != nil {
				return c, err
			}
			c.RollingUniverse = &rolling
		}
	}
	if portfolio, ok := policy.Factor["portfolio"].(map[string]any); ok {
		copied := make(map[string]any, len(portfolio))
		for key, value := range portfolio {
			if key == "rolling_universe" {
				key = "universe"
			}
			copied[key] = value
		}
		if err := Decode(copied, &c.Manifest.Portfolio); err != nil {
			return c, err
		}
		if err := c.Manifest.Portfolio.ValidatePolicy(); err != nil {
			return c, err
		}
	}
	if decision, ok := policy.Factor["decision"].(map[string]any); ok {
		mapped := map[string]string{"interval_ms": "DecisionInterval", "delay_ms": "DecisionDelayMS", "latency_ms": "LatencyMS", "expiry_ms": "ExpiryMS", "max_pending": "MaxPending"}
		for key, value := range decision {
			name := mapped[key]
			if name == "" {
				return c, fmt.Errorf("unsupported decision key %s", key)
			}
			if err := Decode(map[string]any{name: value}, &c); err != nil {
				return c, err
			}
		}
	}
	if researchFields, ok := policy.Factor["research"].(map[string]any); ok {
		for key, value := range researchFields {
			if key != "labels" {
				return c, fmt.Errorf("unsupported research key %s", key)
			}
			if err := Decode(map[string]any{"Labels": value}, &c.Manifest); err != nil {
				return c, err
			}
		}
	}
	if c.Expressions != nil {
		c.Definition = ""
		if c.Expressions.TimeFrame == "" {
			c.Expressions.TimeFrame = timeframe
		}
		if c.Expressions.TimeFrame != timeframe {
			return c, errors.New("expressions.timeframe must match run_timeframes")
		}
	}
	if c.Manifest.Parameters == nil {
		c.Manifest.Parameters = map[string]float64{}
	}
	for key, value := range policy.Params {
		c.Manifest.Parameters[key] = value
	}
	if funding, ok := u.Execution["funding_policy"].(string); ok {
		c.Manifest.Costs.FundingPolicy = funding
	}
	if c.Manifest.Costs.FundingPolicy == "required-stream" || c.Manifest.Costs.FundingPolicy == "storage" {
		// The storage replay binds the built-in funding stream unless one is named.
		c.FundingSource = orm.SeriesSourceFundingRate
	}
	if err := runner.ValidateStaticConfig(c); err != nil {
		return c, err
	}
	return c, nil
}
