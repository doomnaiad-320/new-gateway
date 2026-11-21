package model

import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type ProfitData struct {
	ModelName      string                  `json:"model_name"`
	CreatedAt      int64                   `json:"created_at"`
	Revenue        float64                 `json:"revenue"`
	Cost           float64                 `json:"cost"`
	Profit         float64                 `json:"profit"`
	TokenUsed      int64                   `json:"token_used"`
	Count          int64                   `json:"count"`
	CostConfigured bool                    `json:"cost_configured"`
	UpstreamModels []ProfitUpstreamSummary `json:"upstream_models"`
}

type ProfitUpstreamSummary struct {
	ChannelID         int     `json:"channel_id"`
	UpstreamModelName string  `json:"upstream_model_name"`
	Count             int64   `json:"count"`
	Cost              float64 `json:"cost"`
	CostConfigured    bool    `json:"cost_configured"`
}

type profitQuotaAggregate struct {
	ModelName         string
	UpstreamModelName string
	ChannelID         int
	CreatedAt         int64
	Quota             int64
	TokenUsed         int64
	Count             int64
}

func GetProfitAnalytics(startTime, endTime int64, username string) ([]ProfitData, map[string]float64, map[string]float64, error) {
	if common.QuotaPerUnit <= 0 {
		return nil, nil, nil, fmt.Errorf("QuotaPerUnit must be greater than zero")
	}
	var rows []profitQuotaAggregate
	query := DB.Table("quota_data").
		Select("model_name, upstream_model_name, channel_id, created_at, SUM(quota) AS quota, SUM(token_used) AS token_used, SUM(count) AS count").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime)
	if username != "" {
		query = query.Where("username = ?", username)
	}
	if err := query.Group("model_name, upstream_model_name, channel_id, created_at").Order("created_at ASC").Scan(&rows).Error; err != nil {
		return nil, nil, nil, err
	}

	costs := GetProfitUpstreamCosts()
	pointsByKey := make(map[string]*ProfitData, len(rows))
	upstreamByPoint := make(map[string]map[string]*ProfitUpstreamSummary, len(rows))
	summary := map[string]float64{"cost_configured": 1}
	currentHourSummary := map[string]float64{"cost_configured": 1}
	now := time.Now().Unix()
	currentHourStart := now - now%int64(time.Hour.Seconds())
	for _, row := range rows {
		upstreamModelName := row.UpstreamModelName
		revenue := float64(row.Quota) / common.QuotaPerUnit
		pointKey := fmt.Sprintf("%s\x00%d", row.ModelName, row.CreatedAt)
		point := pointsByKey[pointKey]
		if point == nil {
			point = &ProfitData{
				ModelName:      row.ModelName,
				CreatedAt:      row.CreatedAt,
				CostConfigured: true,
			}
			pointsByKey[pointKey] = point
			upstreamByPoint[pointKey] = make(map[string]*ProfitUpstreamSummary)
		}
		point.Revenue += revenue
		point.TokenUsed += row.TokenUsed
		point.Count += row.Count
		unitCost, configured := costs[profitUpstreamCostKey(row.ModelName, row.ChannelID, upstreamModelName)]
		configured = configured && upstreamModelName != ""
		upstreamKey := fmt.Sprintf("%d\x00%s", row.ChannelID, upstreamModelName)
		upstream := upstreamByPoint[pointKey][upstreamKey]
		if upstream == nil {
			upstream = &ProfitUpstreamSummary{
				ChannelID:         row.ChannelID,
				UpstreamModelName: upstreamModelName,
				CostConfigured:    true,
			}
			upstreamByPoint[pointKey][upstreamKey] = upstream
		}
		upstream.Count += row.Count
		if configured {
			upstream.Cost += float64(row.Count) * unitCost
			point.Cost += float64(row.Count) * unitCost
		} else {
			upstream.CostConfigured = false
			point.CostConfigured = false
			summary["cost_configured"] = 0
		}
		summary["total_revenue"] += revenue
		summary["total_tokens"] += float64(row.TokenUsed)
		summary["total_requests"] += float64(row.Count)
		if row.CreatedAt == currentHourStart && row.CreatedAt >= startTime && row.CreatedAt <= endTime {
			currentHourSummary["total_revenue"] += revenue
			currentHourSummary["total_requests"] += float64(row.Count)
			if !configured {
				currentHourSummary["cost_configured"] = 0
			}
			if configured {
				currentHourSummary["total_cost"] += float64(row.Count) * unitCost
			}
		}
	}

	result := make([]ProfitData, 0, len(pointsByKey))
	for _, point := range pointsByKey {
		pointKey := fmt.Sprintf("%s\x00%d", point.ModelName, point.CreatedAt)
		for _, upstream := range upstreamByPoint[pointKey] {
			if !upstream.CostConfigured {
				upstream.Cost = 0
			}
			point.UpstreamModels = append(point.UpstreamModels, *upstream)
		}
		sort.Slice(point.UpstreamModels, func(i, j int) bool {
			if point.UpstreamModels[i].ChannelID != point.UpstreamModels[j].ChannelID {
				return point.UpstreamModels[i].ChannelID < point.UpstreamModels[j].ChannelID
			}
			return point.UpstreamModels[i].UpstreamModelName < point.UpstreamModels[j].UpstreamModelName
		})
		if point.CostConfigured {
			point.Profit = point.Revenue - point.Cost
			summary["total_cost"] += point.Cost
			summary["total_profit"] += point.Profit
			if point.CreatedAt == currentHourStart && point.CreatedAt >= startTime && point.CreatedAt <= endTime {
				currentHourSummary["total_profit"] += point.Profit
			}
		} else {
			point.Cost = 0
		}
		result = append(result, *point)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt != result[j].CreatedAt {
			return result[i].CreatedAt < result[j].CreatedAt
		}
		return result[i].ModelName < result[j].ModelName
	})
	if summary["total_revenue"] > 0 && summary["cost_configured"] == 1 {
		summary["profit_margin"] = summary["total_profit"] / summary["total_revenue"] * 100
	}
	return result, summary, currentHourSummary, nil
}
