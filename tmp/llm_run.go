package tmp

import (
	"context"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/llm"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
)

// LimitOrder 限价单策略：当没有订单时，按价格的90%创建限价单，3个bar未成交则取消
func LLMRun(pol *config.RunPolicyConfig) *strat.TradeStrat {
	llmManager, err := llm.NewLLMManager(&config.Data, func() []string {
		return []string{"glm4.7"}
	})
	if err != nil {
		return nil
	}
	return &strat.TradeStrat{
		WarmupNum: 0,
		OnBar: func(s *strat.StratJob) {
			res, err := llmManager.Call(context.Background(), "", "hello", nil)
			if err != nil {
				log.Error("LLM call failed", zap.Error(err))
			}
			log.Info("LLM call result", zap.String("result", res))
		},
	}
}
