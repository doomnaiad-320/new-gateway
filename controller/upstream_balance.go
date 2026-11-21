package controller

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

const (
	upstreamBalanceTimeout       = 10 * time.Second
	upstreamBalanceResponseLimit = 1 << 20
	upstreamBalanceConcurrency   = 5
)

type upstreamBalanceResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Username     string  `json:"username"`
		Quota        float64 `json:"quota"`
		UsedQuota    float64 `json:"used_quota"`
		RequestCount int64   `json:"request_count"`
	} `json:"data"`
}

func newUpstreamBalanceSSRFProtection(apiBaseURL string) (*common.SSRFProtection, error) {
	fetch := system_setting.GetFetchSetting()
	// The operator-selected upstream may use a custom port; host and resolved-IP checks still run for every dial.
	allowedPorts := slices.Clone(fetch.AllowedPorts)
	parsed, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, errors.New("Invalid upstream API base URL")
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if !slices.Contains(allowedPorts, port) {
		allowedPorts = append(allowedPorts, port)
	}
	return common.NewSSRFProtectionFromFetchSetting(
		fetch.AllowPrivateIp,
		fetch.DomainFilterMode,
		fetch.IpFilterMode,
		fetch.DomainList,
		fetch.IpList,
		allowedPorts,
		fetch.ApplyIPFilterForDomain,
	)
}

func newUpstreamBalanceHTTPClient(apiBaseURL string) (*http.Client, error) {
	protection, err := newUpstreamBalanceSSRFProtection(apiBaseURL)
	if err != nil {
		return nil, errors.New("Invalid outbound request protection settings")
	}
	dialer := &net.Dialer{Timeout: upstreamBalanceTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, portValue, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("Invalid upstream address")
			}
			port, err := strconv.Atoi(portValue)
			if err != nil || protection.ValidateNetworkTarget(host, port) != nil {
				return nil, errors.New("Upstream address is not allowed")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("Upstream host could not be resolved")
			}
			for _, resolved := range ips {
				if err := protection.ValidateResolvedIP(host, resolved.IP); err != nil {
					return nil, errors.New("Upstream address is not allowed")
				}
			}
			for _, resolved := range ips {
				connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), portValue))
				if err == nil {
					return connection, nil
				}
			}
			return nil, errors.New("Upstream connection failed")
		},
	}
	return &http.Client{
		Timeout:   upstreamBalanceTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func queryUpstreamBalanceAccount(ctx context.Context, account model.UpstreamBalanceAccount, token string) (model.UpstreamBalanceAccount, error) {
	client, err := newUpstreamBalanceHTTPClient(account.APIBaseURL)
	if err != nil {
		return account, err
	}
	return queryUpstreamBalanceAccountWithClient(ctx, account, token, client)
}

func queryUpstreamBalanceAccountWithClient(ctx context.Context, account model.UpstreamBalanceAccount, token string, client *http.Client) (model.UpstreamBalanceAccount, error) {
	parsed, err := url.Parse(account.APIBaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return account, errors.New("Invalid upstream API base URL")
	}
	endpoint := strings.TrimRight(parsed.String(), "/") + "/api/user/self"
	requestCtx, cancel := context.WithTimeout(ctx, upstreamBalanceTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return account, errors.New("Could not create upstream request")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("New-Api-User", account.UserID)
	response, err := client.Do(request)
	if err != nil {
		return account, errors.New("Upstream API request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return account, errors.New("Upstream API returned a non-success status")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, upstreamBalanceResponseLimit+1))
	if err != nil || len(body) > upstreamBalanceResponseLimit {
		return account, errors.New("Could not read upstream API response")
	}
	var payload upstreamBalanceResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return account, errors.New("Could not decode upstream API response")
	}
	if !payload.Success {
		return account, errors.New("Upstream API returned an unsuccessful response")
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return account, errors.New("Quota unit configuration is invalid")
	}
	if math.IsNaN(payload.Data.Quota) || math.IsInf(payload.Data.Quota, 0) || payload.Data.Quota < 0 ||
		math.IsNaN(payload.Data.UsedQuota) || math.IsInf(payload.Data.UsedQuota, 0) || payload.Data.UsedQuota < 0 || payload.Data.RequestCount < 0 {
		return account, errors.New("Upstream API returned invalid account usage")
	}
	account.Username = strings.TrimSpace(payload.Data.Username)
	account.Quota = payload.Data.Quota / common.QuotaPerUnit
	account.UsedQuota = payload.Data.UsedQuota / common.QuotaPerUnit
	account.RequestCount = payload.Data.RequestCount
	if math.IsNaN(account.Quota) || math.IsInf(account.Quota, 0) || math.IsNaN(account.UsedQuota) || math.IsInf(account.UsedQuota, 0) {
		return account, errors.New("Upstream API returned invalid account usage")
	}
	lastSyncedAt := time.Now()
	account.LastSyncedAt = &lastSyncedAt
	account.LastError = ""
	return account, nil
}

func getUpstreamBalanceAccountInput(c *gin.Context) (model.UpstreamBalanceAccountInput, bool) {
	var input model.UpstreamBalanceAccountInput
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorT(c, "Invalid upstream account payload")
		return input, false
	}
	return input, true
}

func parseUpstreamBalanceID(c *gin.Context) uint {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id)
}

func ListUpstreamBalanceAccounts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	accounts, err := model.GetUpstreamBalanceAccounts()
	if err != nil {
		common.ApiErrorT(c, "Could not load upstream accounts")
		return
	}
	publicAccounts := make([]model.UpstreamBalanceAccountPublic, 0, len(accounts))
	for _, account := range accounts {
		publicAccounts = append(publicAccounts, model.ToUpstreamBalanceAccountPublic(account))
	}
	common.ApiSuccess(c, publicAccounts)
}

func CreateUpstreamBalanceAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	input, ok := getUpstreamBalanceAccountInput(c)
	if !ok {
		return
	}
	account, err := model.CreateUpstreamBalanceAccount(input)
	if err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	common.ApiSuccess(c, model.ToUpstreamBalanceAccountPublic(*account))
}

func UpdateUpstreamBalanceAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id := parseUpstreamBalanceID(c)
	if id == 0 {
		common.ApiErrorT(c, "Invalid upstream account ID")
		return
	}
	input, ok := getUpstreamBalanceAccountInput(c)
	if !ok {
		return
	}
	account, err := model.UpdateUpstreamBalanceAccount(id, input)
	if err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	common.ApiSuccess(c, model.ToUpstreamBalanceAccountPublic(*account))
}

func DeleteUpstreamBalanceAccount(c *gin.Context) {
	id := parseUpstreamBalanceID(c)
	if id == 0 {
		common.ApiErrorT(c, "Invalid upstream account ID")
		return
	}
	if err := model.DeleteUpstreamBalanceAccount(id); err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{})
}

func TestUpstreamBalanceAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	input, ok := getUpstreamBalanceAccountInput(c)
	if !ok {
		return
	}
	if err := model.ValidateUpstreamBalanceAccountInput(&input, true); err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	account := model.UpstreamBalanceAccount{
		Name: input.Name, APIBaseURL: input.APIBaseURL, UserID: input.UserID, Enabled: input.Enabled,
	}
	updated, err := queryUpstreamBalanceAccount(c.Request.Context(), account, input.AccessToken)
	if err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	common.ApiSuccess(c, model.ToUpstreamBalanceAccountPublic(updated))
}

func TestSavedUpstreamBalanceAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id := parseUpstreamBalanceID(c)
	if id == 0 {
		common.ApiErrorT(c, "Invalid upstream account ID")
		return
	}
	account, err := model.GetUpstreamBalanceAccount(id)
	if err != nil {
		common.ApiErrorT(c, "Could not load upstream account")
		return
	}
	token, err := model.DecryptUpstreamBalanceToken(*account)
	if err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	updated, err := queryUpstreamBalanceAccount(c.Request.Context(), *account, token)
	if err != nil {
		if saveErr := model.SaveUpstreamBalanceResult(account.ID, *account, err); saveErr != nil {
			common.ApiErrorT(c, saveErr.Error())
			return
		}
		common.ApiErrorT(c, err.Error())
		return
	}
	if err := model.SaveUpstreamBalanceResult(account.ID, updated, nil); err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	common.ApiSuccess(c, model.ToUpstreamBalanceAccountPublic(updated))
}

func RefreshUpstreamBalanceAccounts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	accounts, err := model.GetUpstreamBalanceAccounts()
	if err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	jobs := make(chan int)
	enabledCount := 0
	for _, account := range accounts {
		if account.Enabled {
			enabledCount++
		}
	}
	workerCount := min(upstreamBalanceConcurrency, enabledCount)
	var wait sync.WaitGroup
	for range workerCount {
		wait.Go(func() {
			for index := range jobs {
				account := &accounts[index]
				token, decryptErr := model.DecryptUpstreamBalanceToken(*account)
				var updated model.UpstreamBalanceAccount
				if decryptErr == nil {
					updated, decryptErr = queryUpstreamBalanceAccount(c.Request.Context(), *account, token)
				}
				if saveErr := model.SaveUpstreamBalanceResult(account.ID, updated, decryptErr); saveErr != nil {
					account.LastError = "Could not save upstream balance result"
					continue
				}
				if decryptErr != nil {
					account.LastError = decryptErr.Error()
					continue
				}
				*account = updated
			}
		})
	}
	for index := range accounts {
		if !accounts[index].Enabled {
			continue
		}
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	publicAccounts := make([]model.UpstreamBalanceAccountPublic, 0, len(accounts))
	for _, account := range accounts {
		publicAccounts = append(publicAccounts, model.ToUpstreamBalanceAccountPublic(account))
	}
	common.ApiSuccess(c, publicAccounts)
}
