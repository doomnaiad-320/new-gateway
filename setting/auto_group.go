package setting

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const DefaultMaxTokenAutoGroups = 5

var autoGroups = []string{"default"}
var DefaultUseAutoGroup = false
var maxTokenAutoGroups atomic.Int64

func init() {
	maxTokenAutoGroups.Store(DefaultMaxTokenAutoGroups)
}

func ContainsAutoGroup(group string) bool {
	for _, autoGroup := range GetAutoGroups() {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(value string) error {
	var groups []string
	if err := common.UnmarshalJsonStr(value, &groups); err != nil {
		return err
	}
	autoGroupRoutesMu.Lock()
	autoGroups = groups
	config := autoGroupRoutes
	for i := range config.Routes {
		if config.Routes[i].Key == config.DefaultRoute {
			config.Routes[i].Groups = append([]string(nil), groups...)
			break
		}
	}
	autoGroupRoutes = config
	autoGroupRoutesMu.Unlock()
	return nil
}

func AutoGroups2JsonString() string {
	data, err := common.Marshal(GetAutoGroups())
	if err != nil {
		return "[]"
	}
	return string(data)
}

func GetAutoGroups() []string {
	autoGroupRoutesMu.RLock()
	defer autoGroupRoutesMu.RUnlock()
	return append([]string(nil), autoGroups...)
}

func GetMaxTokenAutoGroups() int {
	return int(maxTokenAutoGroups.Load())
}

func ValidateMaxTokenAutoGroups(value string) error {
	maxCount, err := strconv.Atoi(value)
	if err != nil || maxCount <= 0 {
		return fmt.Errorf("MaxTokenAutoGroups must be a positive integer")
	}
	return nil
}

func UpdateMaxTokenAutoGroups(value string) error {
	if err := ValidateMaxTokenAutoGroups(value); err != nil {
		return err
	}
	maxCount, _ := strconv.Atoi(value)
	maxTokenAutoGroups.Store(int64(maxCount))
	return nil
}

func IsNamedAutoRouteKey(group string) bool {
	return strings.HasPrefix(strings.TrimSpace(group), AutoRoutePrefix) && IsAutoRouteKey(group)
}
