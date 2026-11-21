package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedFlowQuotaData(t *testing.T, quotaData QuotaData) {
	t.Helper()
	require.NoError(t, DB.Create(&quotaData).Error)
}

func seedFlowLookupData(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Create(&Channel{Id: 1, Name: "east"}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 2, Name: "west"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 11, UserId: 1, Key: "sk-primary", Name: "primary"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 22, UserId: 2, Key: "sk-backup", Name: "backup"}).Error)
	require.NoError(t, DB.Delete(&Token{Id: 11}).Error)
}

func TestGetFlowQuotaDataUsesQuotaDataRoleSpecificDimensions(t *testing.T) {
	truncateTables(t)
	seedFlowLookupData(t)

	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 1,
		CreatedAt: 1000,
		Count:     2,
		Quota:     100,
		TokenUsed: 40,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 1,
		CreatedAt: 1100,
		Count:     1,
		Quota:     50,
		TokenUsed: 20,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 2,
		CreatedAt: 1200,
		Count:     1,
		Quota:     25,
		TokenUsed: 10,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    2,
		Username:  "bob",
		NodeName:  "node-b",
		TokenID:   22,
		UseGroup:  "default",
		ModelName: "gpt-b",
		ChannelID: 1,
		CreatedAt: 1300,
		Count:     3,
		Quota:     70,
		TokenUsed: 30,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		ModelName: "legacy",
		CreatedAt: 1400,
		Count:     99,
		Quota:     999,
		TokenUsed: 999,
	})

	rootRows, err := GetFlowQuotaData(900, 2000, "", 0, common.RoleRootUser)
	require.NoError(t, err)
	require.Len(t, rootRows, 3)
	// Token 11 was soft-deleted, so its name is intentionally left empty for the
	// frontend to render a localized "deleted (id)" label instead.
	require.Equal(t, FlowQuotaData{
		UserID:      1,
		Username:    "alice",
		NodeName:    "node-a",
		TokenID:     11,
		TokenName:   "",
		UseGroup:    "vip",
		ChannelID:   1,
		ChannelName: "east",
		ModelName:   "gpt-a",
		TokenUsed:   60,
		Count:       3,
		Quota:       150,
	}, *rootRows[0])
	// A token that still exists resolves to its current name.
	require.Equal(t, 22, rootRows[1].TokenID)
	require.Equal(t, "backup", rootRows[1].TokenName)

	adminRows, err := GetFlowQuotaData(900, 2000, "alice", 0, common.RoleAdminUser)
	require.NoError(t, err)
	require.Len(t, adminRows, 2)
	require.Equal(t, 0, adminRows[0].TokenID)
	require.Empty(t, adminRows[0].TokenName)
	require.Empty(t, adminRows[0].NodeName)
	require.Equal(t, "alice", adminRows[0].Username)
	require.Equal(t, "vip", adminRows[0].UseGroup)
	require.Equal(t, "east", adminRows[0].ChannelName)
	require.Equal(t, 150, adminRows[0].Quota)

	selfRows, err := GetFlowQuotaData(900, 2000, "", 1, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, selfRows, 1)
	require.Empty(t, selfRows[0].Username)
	require.Equal(t, 0, selfRows[0].ChannelID)
	require.Empty(t, selfRows[0].ChannelName)
	require.Empty(t, selfRows[0].TokenName)
	require.Equal(t, "vip", selfRows[0].UseGroup)
	require.Equal(t, 175, selfRows[0].Quota)
}

func TestLogQuotaDataSplitsRowsByUseGroupTokenChannelAndNode(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3661,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     100,
		TokenUsed: 40,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     50,
		TokenUsed: 20,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "default",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     25,
		TokenUsed: 10,
	})

	SaveQuotaDataCache()

	var rows []QuotaData
	require.NoError(t, DB.Order("quota DESC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, int64(3600), rows[0].CreatedAt)
	require.Equal(t, "vip", rows[0].UseGroup)
	require.Equal(t, 11, rows[0].TokenID)
	require.Equal(t, 1, rows[0].ChannelID)
	require.Equal(t, "node-a", rows[0].NodeName)
	require.Equal(t, 2, rows[0].Count)
	require.Equal(t, 150, rows[0].Quota)
	require.Equal(t, 60, rows[0].TokenUsed)
	require.Equal(t, "default", rows[1].UseGroup)
	require.Equal(t, 25, rows[1].Quota)
}

func TestLogQuotaDataKeepsUpstreamModelsSeparate(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	previousCache := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = previousCache
		CacheQuotaDataLock.Unlock()
	})

	for _, upstreamModel := range []string{"provider-a", "provider-b"} {
		LogQuotaData(QuotaDataLogParams{
			UserID:            1,
			Username:          "alice",
			ModelName:         "client-model",
			UpstreamModelName: upstreamModel,
			CreatedAt:         7200,
			ChannelID:         4,
			Quota:             50,
		})
	}

	SaveQuotaDataCache()
	var rows []QuotaData
	require.NoError(t, DB.Where("model_name = ?", "client-model").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.ElementsMatch(t, []string{"provider-a", "provider-b"}, []string{
		rows[0].UpstreamModelName,
		rows[1].UpstreamModelName,
	})
}

func TestGetProfitAnalyticsMatchesConfiguredCostsByRequestChannelAndUpstreamModel(t *testing.T) {
	truncateTables(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		ProfitUpstreamCostOption: `[{"model_name":"client-model","channel_id":4,"upstream_model_name":"provider-a","cost_per_request":0.1}]`,
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	now := time.Now().Unix()
	createdAt := now - now%3600
	for _, row := range []QuotaData{
		{Username: "alice", ModelName: "client-model", UpstreamModelName: "provider-a", ChannelID: 4, CreatedAt: createdAt, Count: 2, Quota: 1000, TokenUsed: 80},
		{Username: "alice", ModelName: "client-model", UpstreamModelName: "provider-b", ChannelID: 4, CreatedAt: createdAt, Count: 1, Quota: 500, TokenUsed: 30},
		{Username: "alice", ModelName: "other-model", UpstreamModelName: "provider-a", ChannelID: 5, CreatedAt: createdAt, Count: 1, Quota: 700},
	} {
		require.NoError(t, DB.Create(&row).Error)
	}

	rows, summary, currentHour, err := GetProfitAnalytics(createdAt, now, "alice")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "client-model", rows[0].ModelName)
	assert.Equal(t, int64(3), rows[0].Count)
	assert.InDelta(t, 15.0, rows[0].Revenue, 0.0001)
	assert.False(t, rows[0].CostConfigured)
	assert.Len(t, rows[0].UpstreamModels, 2)
	assert.Equal(t, "provider-a", rows[0].UpstreamModels[0].UpstreamModelName)
	assert.InDelta(t, 0.2, rows[0].UpstreamModels[0].Cost, 0.0001)
	assert.True(t, rows[0].UpstreamModels[0].CostConfigured)
	assert.Equal(t, "provider-b", rows[0].UpstreamModels[1].UpstreamModelName)
	assert.False(t, rows[0].UpstreamModels[1].CostConfigured)
	assert.Equal(t, float64(22), summary["total_revenue"])
	assert.Equal(t, float64(0), summary["cost_configured"])
	assert.Equal(t, float64(0), currentHour["cost_configured"])

	common.OptionMapRWMutex.Lock()
	common.OptionMap[ProfitUpstreamCostOption] = `[{"model_name":"client-model","channel_id":4,"upstream_model_name":"provider-a","cost_per_request":0.1},{"model_name":"client-model","channel_id":4,"upstream_model_name":"provider-b","cost_per_request":0.5},{"model_name":"other-model","channel_id":5,"upstream_model_name":"provider-a","cost_per_request":0.2}]`
	common.OptionMapRWMutex.Unlock()
	rows, summary, currentHour, err = GetProfitAnalytics(createdAt, now, "alice")
	require.NoError(t, err)
	assert.True(t, rows[0].CostConfigured)
	assert.InDelta(t, 0.7, rows[0].Cost, 0.0001)
	assert.InDelta(t, 14.3, rows[0].Profit, 0.0001)
	assert.InDelta(t, 0.9, summary["total_cost"], 0.0001)
	assert.InDelta(t, 21.1, summary["total_profit"], 0.0001)
	assert.InDelta(t, 21.1, currentHour["total_profit"], 0.0001)
}

func TestParseProfitUpstreamCostsRejectsInvalidAndDuplicateRules(t *testing.T) {
	valid := `[{"model_name":" model-a ","channel_id":8,"upstream_model_name":" provider-a ","cost_per_request":0}]`
	rules, err := ParseProfitUpstreamCosts(valid)
	require.NoError(t, err)
	require.Equal(t, "model-a", rules[0].ModelName)
	require.Equal(t, "provider-a", rules[0].UpstreamModelName)

	for _, value := range []string{
		`{"model_name":"model-a"}`,
		`[{"model_name":"model-a","channel_id":8,"upstream_model_name":"provider-a","cost_per_request":-0.1}]`,
		`[{"model_name":"model-a","channel_id":8,"upstream_model_name":"provider-a","cost_per_request":0.1},{"model_name":"model-a","channel_id":8,"upstream_model_name":"provider-a","cost_per_request":0.2}]`,
	} {
		_, err = ParseProfitUpstreamCosts(value)
		require.Error(t, err)
	}
}
