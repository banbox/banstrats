package tmp

import (
	"github.com/banbox/banbot/strat"
)

func init() {
	// 注册策略到banbot中，后续在配置文件中使用tmp:limit即可引用此策略
	strat.AddStratGroup("tmp", map[string]strat.FuncMakeStrat{
		"limit":    LimitOrder,
		"demo":     Demo,
		"chg_pair": ChangePair,
		"llm_run":  LLMRun,
	})
}
