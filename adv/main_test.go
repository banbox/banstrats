package adv

import (
	"testing"

	"github.com/banbox/banbot/entry"
)

func TestChartCommandFactories(t *testing.T) {
	for i := 0; i < 2; i++ {
		root := entry.NewRootCommand()
		kline, _, err := root.Find([]string{"chart", "kline"})
		if err != nil || kline.Name() != "kline" {
			t.Fatalf("kline command was not registered: %v", err)
		}
		if kline.Flags().Lookup("pairs") == nil || kline.Flags().Lookup("timeframes") == nil {
			t.Fatal("chart flags were not registered")
		}
		demo, _, err := root.Find([]string{"chart", "demo"})
		if err != nil || demo.Name() != "demo" || !demo.DisableFlagParsing {
			t.Fatalf("raw demo command was not registered: %v", err)
		}
	}
}
