package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type upstreamBalanceRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip upstreamBalanceRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func setupUpstreamBalanceTest(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	previousFetchSetting := *system_setting.GetFetchSetting()
	var database *gorm.DB
	var err error
	dsn := os.Getenv("UPSTREAM_BALANCE_TEST_DSN")
	switch os.Getenv("UPSTREAM_BALANCE_TEST_DIALECT") {
	case "mysql":
		database, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		database, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		database, err = gorm.Open(sqlite.Open(fmt.Sprintf("file:upstream_balance_%p?mode=memory&cache=shared", t)), &gorm.Config{})
	}
	require.NoError(t, err)
	versionQuery := "SELECT sqlite_version()"
	if dialect := database.Dialector.Name(); dialect == "mysql" {
		versionQuery = "SELECT VERSION()"
	} else if dialect == "postgres" {
		versionQuery = "SELECT version()"
	}
	var databaseVersion string
	require.NoError(t, database.Raw(versionQuery).Scan(&databaseVersion).Error)
	t.Logf("upstream balance test database: %s %s", database.Dialector.Name(), databaseVersion)
	model.DB = database
	common.QuotaPerUnit = 500_000
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{
		EnableSSRFProtection:   true,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		AllowedPorts:           []string{"80", "443", "8080", "8443"},
		ApplyIPFilterForDomain: true,
	}
	require.NoError(t, database.AutoMigrate(&model.Option{}))
	previousOption := model.Option{Key: "upstream_balance_migration_sentinel", Value: "preserve-existing-data"}
	require.NoError(t, database.FirstOrCreate(&previousOption, model.Option{Key: previousOption.Key}).Error)
	require.NoError(t, database.AutoMigrate(&model.UpstreamBalanceEncryptionKey{}, &model.UpstreamBalanceAccount{}))
	require.NoError(t, database.First(&previousOption, &model.Option{Key: previousOption.Key}).Error)
	assert.Equal(t, "preserve-existing-data", previousOption.Value, "adding upstream balance tables preserves existing application options")
	t.Cleanup(func() {
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
		*system_setting.GetFetchSetting() = previousFetchSetting
		sqlDB, err := database.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func TestUpstreamBalanceCredentialIsEncryptedAndRedacted(t *testing.T) {
	setupUpstreamBalanceTest(t)
	const token = "upstream-sensitive-token"
	account, err := model.CreateUpstreamBalanceAccount(model.UpstreamBalanceAccountInput{
		Name: "production", APIBaseURL: "https://api.example.test", UserID: "42",
		AccessToken: token, Enabled: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, account.AccessTokenEncrypted)
	assert.NotContains(t, account.AccessTokenEncrypted, token)

	var stored model.UpstreamBalanceAccount
	require.NoError(t, model.DB.First(&stored, account.ID).Error)
	assert.NotContains(t, stored.AccessTokenEncrypted, token)
	decrypted, err := model.DecryptUpstreamBalanceToken(stored)
	require.NoError(t, err)
	assert.Equal(t, token, decrypted)
	var storedKey model.UpstreamBalanceEncryptionKey
	require.NoError(t, model.DB.First(&storedKey, 1).Error)
	assert.Len(t, storedKey.Value, 43, "the encryption key is persisted in the database")

	publicJSON, err := common.Marshal(model.ToUpstreamBalanceAccountPublic(stored))
	require.NoError(t, err)
	assert.NotContains(t, string(publicJSON), token)
	assert.NotContains(t, string(publicJSON), "access_token_encrypted")
	assert.Contains(t, string(publicJSON), `"has_token":true`)

	updated, err := model.UpdateUpstreamBalanceAccount(account.ID, model.UpstreamBalanceAccountInput{
		Name: "production renamed", APIBaseURL: "https://api.example.test", UserID: "42", Enabled: true,
	})
	require.NoError(t, err)
	decrypted, err = model.DecryptUpstreamBalanceToken(*updated)
	require.NoError(t, err)
	assert.Equal(t, token, decrypted, "omitting a replacement credential preserves the saved value")

	require.NoError(t, model.DB.AutoMigrate(&model.UpstreamBalanceEncryptionKey{}, &model.UpstreamBalanceAccount{}))
	updated, err = model.GetUpstreamBalanceAccount(account.ID)
	require.NoError(t, err)
	decrypted, err = model.DecryptUpstreamBalanceToken(*updated)
	require.NoError(t, err)
	assert.Equal(t, token, decrypted, "re-running the migration preserves account data and decryptability")
}

func TestUpstreamBalanceRefreshFailurePreservesLastSuccessfulValues(t *testing.T) {
	setupUpstreamBalanceTest(t)
	account, err := model.CreateUpstreamBalanceAccount(model.UpstreamBalanceAccountInput{
		Name: "production", APIBaseURL: "https://api.example.test", UserID: "42",
		AccessToken: "test-token", Enabled: true,
	})
	require.NoError(t, err)
	lastSyncedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	require.NoError(t, model.SaveUpstreamBalanceResult(account.ID, model.UpstreamBalanceAccount{
		Username: "upstream-user", Quota: 12.5, UsedQuota: 4.25,
		RequestCount: 23, LastSyncedAt: &lastSyncedAt,
	}, nil))
	require.NoError(t, model.SaveUpstreamBalanceResult(account.ID, model.UpstreamBalanceAccount{}, errors.New("Upstream connection failed")))

	updated, err := model.GetUpstreamBalanceAccount(account.ID)
	require.NoError(t, err)
	assert.Equal(t, "upstream-user", updated.Username)
	assert.Equal(t, 12.5, updated.Quota)
	assert.Equal(t, 4.25, updated.UsedQuota)
	assert.Equal(t, int64(23), updated.RequestCount)
	require.NotNil(t, updated.LastSyncedAt)
	assert.True(t, lastSyncedAt.Equal(*updated.LastSyncedAt))
	assert.Equal(t, "Upstream connection failed", updated.LastError)
}

func TestQueryUpstreamBalanceAccountBlocksPrivateTargetsAndRedirects(t *testing.T) {
	setupUpstreamBalanceTest(t)
	setting := system_setting.GetFetchSetting()
	setting.AllowedPorts = []string{"8080"}
	_, err := queryUpstreamBalanceAccount(context.Background(), model.UpstreamBalanceAccount{
		APIBaseURL: "http://127.0.0.1:8080", UserID: "42",
	}, "test-token")
	require.Error(t, err)

	setting.AllowPrivateIp = true
	redirectCalls := 0
	redirectClient := &http.Client{
		Transport: upstreamBalanceRoundTripper(func(request *http.Request) (*http.Response, error) {
			redirectCalls++
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1:8080/redirected"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		}),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	_, err = queryUpstreamBalanceAccountWithClient(context.Background(), model.UpstreamBalanceAccount{
		APIBaseURL: "https://api.example.test", UserID: "42",
	}, "test-token", redirectClient)
	require.Error(t, err)
	assert.Equal(t, 1, redirectCalls, "bearer credentials must not be forwarded through redirects")
}

func TestQueryUpstreamBalanceAccountMapsUsageAndHeaders(t *testing.T) {
	setupUpstreamBalanceTest(t)
	var received bool
	client := &http.Client{Transport: upstreamBalanceRoundTripper(func(request *http.Request) (*http.Response, error) {
		received = true
		assert.Equal(t, "/api/user/self", request.URL.Path)
		assert.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))
		assert.Equal(t, "42", request.Header.Get("New-Api-User"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{"username":"upstream-user","quota":2500000,"used_quota":1250000,"request_count":17}}`)),
			Request:    request,
		}, nil
	})}

	updated, err := queryUpstreamBalanceAccountWithClient(context.Background(), model.UpstreamBalanceAccount{
		APIBaseURL: "https://api.example.test", UserID: "42",
	}, "test-token", client)
	require.NoError(t, err)
	assert.True(t, received)
	assert.Equal(t, "upstream-user", updated.Username)
	assert.Equal(t, float64(5), updated.Quota)
	assert.Equal(t, float64(2.5), updated.UsedQuota)
	assert.Equal(t, int64(17), updated.RequestCount)
	require.NotNil(t, updated.LastSyncedAt)
	assert.False(t, updated.LastSyncedAt.IsZero())
}

func TestUpstreamBalanceAllowsHTTPUpstreamURL(t *testing.T) {
	setupUpstreamBalanceTest(t)
	protection, err := newUpstreamBalanceSSRFProtection("http://38.58.181.83:9090")
	require.NoError(t, err)
	require.NoError(t, protection.ValidateNetworkTarget("38.58.181.83", 9090), "the explicitly configured upstream port is allowed")
	assert.Error(t, protection.ValidateNetworkTarget("127.0.0.1", 9090), "loopback targets remain blocked")

	account, err := model.CreateUpstreamBalanceAccount(model.UpstreamBalanceAccountInput{
		Name: "HTTP upstream", APIBaseURL: "http://38.58.181.83:9090", UserID: "42",
		AccessToken: "test-token", Enabled: true,
	})
	require.NoError(t, err)

	client := &http.Client{Transport: upstreamBalanceRoundTripper(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "http", request.URL.Scheme)
		assert.Equal(t, "38.58.181.83:9090", request.URL.Host)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{"username":"operator","quota":500000,"used_quota":0,"request_count":1}}`)),
			Request:    request,
		}, nil
	})}
	updated, err := queryUpstreamBalanceAccountWithClient(context.Background(), *account, "test-token", client)
	require.NoError(t, err)
	assert.Equal(t, "operator", updated.Username)
}
