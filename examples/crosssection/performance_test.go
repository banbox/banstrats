package crosssection

import (
	"fmt"
	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/orm"
	"math"
	"testing"
)

const performanceAssets = 100
const performanceBars = 256

var performanceNames = []string{"CSMultiFactor", "CSTrend", "CSVolumeMomentum"}

// Every version receives the exact same frozen, deterministic 100-asset x
// 256-bar input. No random seed, external data, I/O or fixture generation is
// timed. An invalid raw observation also checks warmup/recovery parity.
func performanceSnapshots(tb testing.TB) []*factor.Snapshot {
	tb.Helper()
	snapshots := make([]*factor.Snapshot, performanceBars)
	sids := make([]int32, performanceAssets)
	sidMap := map[int32]string{}
	for i := range sids {
		sids[i] = int32(i + 1)
		sidMap[sids[i]] = fmt.Sprintf("ASSET%d", i+1)
	}
	universe := factor.Universe{Version: "perf-v1", Investable: sids, Reference: sids, Tradable: sids, Evaluation: sids, Static: true}
	for bar := range snapshots {
		event := int64(bar+1) * 86400000
		rows := make([]factor.VersionRecord, 0, performanceAssets)
		required := make([]factor.Requirement, 0, performanceAssets)
		for _, sid := range sids {
			values := map[string]any{"close": 100 + float64(bar)*float64(sid)/20 + 5*math.Sin(float64(bar)/7+float64(sid)), "volume": 100 + float64(sid)*10}
			if bar == 80 && sid == 100 {
				values["close"] = nil
			}
			rows = append(rows, factor.Record(orm.DataSeries{Source: "kline", Sid: sid, TimeMS: event - 86400000, EndMS: event, TimeFrame: "1d", Closed: true, Values: values}, 1, event, event, "v1"))
			required = append(required, factor.Requirement{SID: sid, Source: "kline", TimeFrame: "1d", EventTime: event})
		}
		var err error
		snapshots[bar], err = factor.Freeze(factor.SnapshotSpec{DecisionTime: event, ReplayTime: event, Universe: universe, SIDMap: sidMap, Schemas: map[string]string{"kline": "schema-v1"}, SourceVersions: map[string]string{"kline": "v1"}, VisibilityPolicy: "published-and-received"}, rows, required)
		if err != nil {
			tb.Fatal(err)
		}
	}
	return snapshots
}

func performancePlan(tb testing.TB, name string, expression bool) *factor.Plan {
	tb.Helper()
	builder := build
	if expression {
		builder = exprBuild
	}
	plan, _, err := builder(name, config(name, nil))
	if err != nil {
		tb.Fatal(err)
	}
	return plan
}

func performanceFramesEqual(tb testing.TB, got, want factor.Frame, bar int) {
	tb.Helper()
	if got.DecisionTime != want.DecisionTime || got.SnapshotID != want.SnapshotID || len(got.Values) != len(want.Values) {
		tb.Fatalf("bar %d frame identity differs", bar)
	}
	for column, values := range want.Values {
		if len(got.Values[column]) != len(values) {
			tb.Fatalf("bar %d %s observations differ", bar, column)
		}
		for sid, value := range values {
			other, ok := got.Values[column][sid]
			if !ok || other.Validity != value.Validity || value.Validity == factor.Valid && math.Abs(other.Value-value.Value) > 1e-10+1e-8*math.Abs(value.Value) {
				tb.Fatalf("bar %d %s SID %d got %+v want %+v", bar, column, sid, other, value)
			}
		}
	}
}

// This same check runs before timed benchmarks; two faster but numerically
// different definitions cannot be presented as an optimization.
func performanceParity(tb testing.TB, name string, snapshots []*factor.Snapshot) {
	tb.Helper()
	original := performancePlan(tb, name, false)
	expression := performancePlan(tb, name, true)
	if original.WarmupLength() != expression.WarmupLength() {
		tb.Fatal("warmup differs")
	}
	oldSession, err := factor.NewSession(original)
	if err != nil {
		tb.Fatal(err)
	}
	newSession, err := factor.NewSession(expression)
	if err != nil {
		tb.Fatal(err)
	}
	oldBatch, err := original.Batch(snapshots, len(snapshots))
	if err != nil {
		tb.Fatal(err)
	}
	newBatch, err := expression.Batch(snapshots, len(snapshots))
	if err != nil {
		tb.Fatal(err)
	}
	for bar, snapshot := range snapshots {
		oldFrame, err := oldSession.Evaluate(snapshot)
		if err != nil {
			tb.Fatal(err)
		}
		newFrame, err := newSession.Evaluate(snapshot)
		if err != nil {
			tb.Fatal(err)
		}
		performanceFramesEqual(tb, newFrame, oldFrame, bar)
		performanceFramesEqual(tb, newBatch[bar], oldBatch[bar], bar)
		performanceFramesEqual(tb, newBatch[bar], newFrame, bar)
	}
}

func TestPerformanceFixtureParity(t *testing.T) {
	snapshots := performanceSnapshots(t)
	for _, name := range performanceNames {
		t.Run(name, func(t *testing.T) { performanceParity(t, name, snapshots) })
	}
}

// Session and Batch ns/op cover the complete 256-bar history, not one bar.
// Session includes a fresh session each iteration to give both versions the
// same EMA seed, warmup and chronological state. Compile always recompiles the
// same default specification; it does not use any optional compile cache.
func BenchmarkFactorVersions(b *testing.B) {
	snapshots := performanceSnapshots(b)
	for _, name := range performanceNames {
		performanceParity(b, name, snapshots)
		b.Run(name, func(b *testing.B) {
			for _, version := range []string{"Go", "Expr"} {
				b.Run(version, func(b *testing.B) {
					builder := build
					if version == "Expr" {
						builder = exprBuild
					}
					c := config(name, nil)
					b.Run("Compile", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							plan, _, err := builder(name, c)
							if err != nil {
								b.Fatal(err)
							}
							performancePlanSink = plan
						}
					})
					plan, _, err := builder(name, c)
					if err != nil {
						b.Fatal(err)
					}
					b.Run("Session", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							session, err := factor.NewSession(plan)
							if err != nil {
								b.Fatal(err)
							}
							for _, snapshot := range snapshots {
								frame, err := session.Evaluate(snapshot)
								if err != nil {
									b.Fatal(err)
								}
								performanceFrameSink = frame
							}
						}
						b.ReportMetric(performanceBars, "bars/op")
						b.ReportMetric(performanceAssets, "assets/bar")
					})
					b.Run("Batch", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							frames, err := plan.Batch(snapshots, len(snapshots))
							if err != nil {
								b.Fatal(err)
							}
							performanceFrameSink = frames[len(frames)-1]
						}
						b.ReportMetric(performanceBars, "bars/op")
						b.ReportMetric(performanceAssets, "assets/bar")
					})
				})
			}
		})
	}
}

var performancePlanSink *factor.Plan
var performanceFrameSink factor.Frame
