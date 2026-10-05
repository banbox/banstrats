package rpc_ai

import (
	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/strat"
)

func init() {
	biz.RegisterFeaGenerator("aifea", pubAiFea)
	strat.AddStratGroup("rpc_ai", map[string]strat.FuncMakeStrat{
		"trade1": AITrade,
	})
}
