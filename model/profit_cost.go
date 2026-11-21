package model

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const ProfitUpstreamCostOption = "ProfitUpstreamCosts"

type ProfitUpstreamCost struct {
	ModelName         string  `json:"model_name"`
	ChannelID         int     `json:"channel_id"`
	UpstreamModelName string  `json:"upstream_model_name"`
	CostPerRequest    float64 `json:"cost_per_request"`
}

func ParseProfitUpstreamCosts(value string) ([]ProfitUpstreamCost, error) {
	rules := []ProfitUpstreamCost{}
	if err := common.UnmarshalJsonStr(value, &rules); err != nil {
		return nil, fmt.Errorf("invalid upstream cost rules: %w", err)
	}
	seen := make(map[string]struct{}, len(rules))
	for i := range rules {
		rule := &rules[i]
		rule.ModelName = strings.TrimSpace(rule.ModelName)
		rule.UpstreamModelName = strings.TrimSpace(rule.UpstreamModelName)
		if rule.ModelName == "" || rule.ChannelID <= 0 || rule.UpstreamModelName == "" || rule.CostPerRequest < 0 || math.IsNaN(rule.CostPerRequest) || math.IsInf(rule.CostPerRequest, 0) {
			return nil, fmt.Errorf("upstream cost rule %d requires a user model, positive channel ID, upstream model, and finite non-negative cost", i+1)
		}
		key := profitUpstreamCostKey(rule.ModelName, rule.ChannelID, rule.UpstreamModelName)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate upstream cost rule for model %q, channel %d, and upstream model %q", rule.ModelName, rule.ChannelID, rule.UpstreamModelName)
		}
		seen[key] = struct{}{}
	}
	return rules, nil
}

func GetProfitUpstreamCosts() map[string]float64 {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[ProfitUpstreamCostOption]
	common.OptionMapRWMutex.RUnlock()
	rules, err := ParseProfitUpstreamCosts(value)
	if err != nil {
		return map[string]float64{}
	}
	costs := make(map[string]float64, len(rules))
	for _, rule := range rules {
		costs[profitUpstreamCostKey(rule.ModelName, rule.ChannelID, rule.UpstreamModelName)] = rule.CostPerRequest
	}
	return costs
}

func profitUpstreamCostKey(modelName string, channelID int, upstreamModelName string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", modelName, channelID, upstreamModelName)
}
