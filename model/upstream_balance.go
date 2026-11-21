package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const upstreamBalanceTokenAAD = "new-api:upstream-balance-token:v1"

type UpstreamBalanceEncryptionKey struct {
	ID    int    `gorm:"primaryKey"`
	Value string `gorm:"type:text;not null"`
}

func (UpstreamBalanceEncryptionKey) TableName() string {
	return "upstream_balance_encryption_keys"
}

type UpstreamBalanceAccount struct {
	ID                   uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name                 string     `json:"name" gorm:"size:128;not null"`
	APIBaseURL           string     `json:"api_base_url" gorm:"size:512;not null"`
	UserID               string     `json:"user_id" gorm:"size:128;not null"`
	AccessTokenEncrypted string     `json:"-" gorm:"type:text;not null"`
	Username             string     `json:"username" gorm:"size:128"`
	Enabled              bool       `json:"enabled" gorm:"not null"`
	Quota                float64    `json:"quota" gorm:"not null"`
	UsedQuota            float64    `json:"used_quota" gorm:"not null"`
	RequestCount         int64      `json:"request_count" gorm:"not null"`
	LastSyncedAt         *time.Time `json:"last_synced_at"`
	LastError            string     `json:"last_error" gorm:"type:text"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (UpstreamBalanceAccount) TableName() string {
	return "upstream_balance_accounts"
}

type UpstreamBalanceAccountInput struct {
	Name        string `json:"name"`
	APIBaseURL  string `json:"api_base_url"`
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	Enabled     bool   `json:"enabled"`
}

type UpstreamBalanceAccountPublic struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	APIBaseURL   string     `json:"api_base_url"`
	UserID       string     `json:"user_id"`
	Username     string     `json:"username"`
	Enabled      bool       `json:"enabled"`
	HasToken     bool       `json:"has_token"`
	Quota        float64    `json:"quota"`
	UsedQuota    float64    `json:"used_quota"`
	RequestCount int64      `json:"request_count"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	LastError    string     `json:"last_error"`
}

func validateUpstreamBalanceAccountInput(input *UpstreamBalanceAccountInput, requireToken bool) error {
	input.Name = strings.TrimSpace(input.Name)
	input.APIBaseURL = strings.TrimRight(strings.TrimSpace(input.APIBaseURL), "/")
	input.UserID = strings.TrimSpace(input.UserID)
	input.AccessToken = strings.TrimSpace(input.AccessToken)
	if input.Name == "" || input.UserID == "" || (requireToken && input.AccessToken == "") {
		return errors.New("Name, upstream user ID, and access token are required")
	}
	if len(input.AccessToken) > 16*1024 {
		return errors.New("Access token is too long")
	}
	parsed, err := url.ParseRequestURI(input.APIBaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return errors.New("API base URL must be an absolute HTTP or HTTPS URL without credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(input.APIBaseURL, "?#") {
		return errors.New("API base URL must not include a query or fragment")
	}
	if port := parsed.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return errors.New("API base URL has an invalid port")
		}
	}
	if len(input.Name) > 128 || len(input.APIBaseURL) > 512 || len(input.UserID) > 128 {
		return errors.New("Upstream account field exceeds its maximum length")
	}
	return nil
}

func ValidateUpstreamBalanceAccountInput(input *UpstreamBalanceAccountInput, requireToken bool) error {
	return validateUpstreamBalanceAccountInput(input, requireToken)
}

func upstreamBalanceEncryptionKey() ([]byte, error) {
	if DB == nil {
		return nil, errors.New("Upstream credential storage is unavailable")
	}
	var stored UpstreamBalanceEncryptionKey
	err := DB.First(&stored, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		randomKey := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, randomKey); err != nil {
			return nil, err
		}
		candidate := UpstreamBalanceEncryptionKey{
			ID: 1, Value: base64.RawStdEncoding.EncodeToString(randomKey),
		}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return nil, err
		}
		err = DB.First(&stored, 1).Error
	}
	if err != nil {
		return nil, errors.New("Could not load upstream credential encryption key")
	}
	key, err := base64.RawStdEncoding.DecodeString(stored.Value)
	if err != nil || len(key) != 32 {
		return nil, errors.New("Stored upstream credential encryption key is invalid")
	}
	return key, nil
}

func encryptUpstreamBalanceToken(token string) (string, error) {
	key, err := upstreamBalanceEncryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(token), []byte(upstreamBalanceTokenAAD))
	return "v1:" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptUpstreamBalanceToken(encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, "v1:") {
		return "", errors.New("Unsupported upstream credential format")
	}
	key, err := upstreamBalanceEncryptionKey()
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, "v1:"))
	if err != nil {
		return "", errors.New("Stored upstream credential is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("Stored upstream credential is invalid")
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(upstreamBalanceTokenAAD))
	if err != nil {
		return "", errors.New("Stored upstream credential could not be decrypted")
	}
	return string(plaintext), nil
}

func ToUpstreamBalanceAccountPublic(account UpstreamBalanceAccount) UpstreamBalanceAccountPublic {
	public := UpstreamBalanceAccountPublic{
		ID: account.ID, Name: account.Name, APIBaseURL: account.APIBaseURL,
		UserID: account.UserID, Username: account.Username, Enabled: account.Enabled,
		HasToken: account.AccessTokenEncrypted != "", Quota: account.Quota,
		UsedQuota: account.UsedQuota, RequestCount: account.RequestCount, LastError: account.LastError,
	}
	public.LastSyncedAt = account.LastSyncedAt
	return public
}

func GetUpstreamBalanceAccounts() ([]UpstreamBalanceAccount, error) {
	var accounts []UpstreamBalanceAccount
	if err := DB.Order("id asc").Find(&accounts).Error; err != nil {
		return nil, errors.New("Could not load upstream accounts")
	}
	return accounts, nil
}

func GetEnabledUpstreamBalanceAccounts() ([]UpstreamBalanceAccount, error) {
	var accounts []UpstreamBalanceAccount
	if err := DB.Where("enabled = ?", true).Order("id asc").Find(&accounts).Error; err != nil {
		return nil, errors.New("Could not load upstream accounts")
	}
	return accounts, nil
}

func GetUpstreamBalanceAccount(id uint) (*UpstreamBalanceAccount, error) {
	var account UpstreamBalanceAccount
	if err := DB.First(&account, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("Upstream account not found")
		}
		return nil, errors.New("Could not load upstream account")
	}
	return &account, nil
}

func NewUpstreamBalanceAccount(input UpstreamBalanceAccountInput) (*UpstreamBalanceAccount, error) {
	if err := validateUpstreamBalanceAccountInput(&input, true); err != nil {
		return nil, err
	}
	encrypted, err := encryptUpstreamBalanceToken(input.AccessToken)
	if err != nil {
		return nil, err
	}
	account := UpstreamBalanceAccount{
		Name: input.Name, APIBaseURL: input.APIBaseURL, UserID: input.UserID,
		AccessTokenEncrypted: encrypted, Enabled: input.Enabled,
	}
	return &account, nil
}

func CreateUpstreamBalanceAccount(input UpstreamBalanceAccountInput) (*UpstreamBalanceAccount, error) {
	account, err := NewUpstreamBalanceAccount(input)
	if err != nil {
		return nil, err
	}
	if err := DB.Create(account).Error; err != nil {
		return nil, errors.New("Could not save upstream account")
	}
	return account, nil
}

func UpdateUpstreamBalanceAccount(id uint, input UpstreamBalanceAccountInput) (*UpstreamBalanceAccount, error) {
	account, err := GetUpstreamBalanceAccount(id)
	if err != nil {
		return nil, err
	}
	if err := validateUpstreamBalanceAccountInput(&input, false); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"name": input.Name, "api_base_url": input.APIBaseURL,
		"user_id": input.UserID, "enabled": input.Enabled,
	}
	if input.AccessToken != "" {
		encrypted, err := encryptUpstreamBalanceToken(input.AccessToken)
		if err != nil {
			return nil, err
		}
		updates["access_token_encrypted"] = encrypted
	}
	if err := DB.Model(account).Updates(updates).Error; err != nil {
		return nil, errors.New("Could not save upstream account")
	}
	return GetUpstreamBalanceAccount(id)
}

func DeleteUpstreamBalanceAccount(id uint) error {
	result := DB.Delete(&UpstreamBalanceAccount{}, id)
	if result.Error != nil {
		return errors.New("Could not delete upstream account")
	}
	if result.RowsAffected == 0 {
		return errors.New("Upstream account not found")
	}
	return nil
}

func DecryptUpstreamBalanceToken(account UpstreamBalanceAccount) (string, error) {
	if account.AccessTokenEncrypted == "" {
		return "", errors.New("Upstream account has no access token")
	}
	return decryptUpstreamBalanceToken(account.AccessTokenEncrypted)
}

func SaveUpstreamBalanceResult(id uint, result UpstreamBalanceAccount, queryErr error) error {
	updates := map[string]any{}
	if queryErr != nil {
		updates["last_error"] = queryErr.Error()
	} else {
		if math.IsNaN(result.Quota) || math.IsInf(result.Quota, 0) || math.IsNaN(result.UsedQuota) || math.IsInf(result.UsedQuota, 0) || result.Quota < 0 || result.UsedQuota < 0 || result.RequestCount < 0 {
			return errors.New("Upstream API returned invalid account usage")
		}
		updates["username"] = result.Username
		updates["quota"] = result.Quota
		updates["used_quota"] = result.UsedQuota
		updates["request_count"] = result.RequestCount
		updates["last_synced_at"] = result.LastSyncedAt
		updates["last_error"] = ""
	}
	if err := DB.Model(&UpstreamBalanceAccount{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return errors.New("Could not save upstream balance result")
	}
	return nil
}
