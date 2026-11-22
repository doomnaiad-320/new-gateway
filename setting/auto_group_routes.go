package setting

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const (
	DefaultAutoRouteKey = "auto"
	AutoRoutePrefix     = "auto:"
)

var autoRouteKeyPattern = regexp.MustCompile(`^auto(?::[A-Za-z0-9][A-Za-z0-9._-]{0,58})?$`)

type AutoGroupRoute struct {
	Key            string   `json:"key"`
	Name           string   `json:"name,omitempty"`
	Enabled        bool     `json:"enabled"`
	UserSelectable bool     `json:"user_selectable"`
	Groups         []string `json:"groups"`
}

func (route *AutoGroupRoute) UnmarshalJSON(data []byte) error {
	var raw struct {
		Key            string   `json:"key"`
		Name           string   `json:"name"`
		Enabled        *bool    `json:"enabled"`
		UserSelectable *bool    `json:"user_selectable"`
		Groups         []string `json:"groups"`
	}
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	route.Key = raw.Key
	route.Name = raw.Name
	route.Enabled = raw.Enabled == nil || *raw.Enabled
	route.UserSelectable = raw.UserSelectable == nil || *raw.UserSelectable
	route.Groups = raw.Groups
	return nil
}

type AutoGroupRoutesConfig struct {
	Version      int              `json:"version"`
	DefaultRoute string           `json:"default_route"`
	Routes       []AutoGroupRoute `json:"routes"`
}

var (
	autoGroupRoutesMu sync.RWMutex
	autoGroupRoutes   = AutoGroupRoutesConfig{
		Version:      1,
		DefaultRoute: DefaultAutoRouteKey,
		Routes: []AutoGroupRoute{{
			Key:            DefaultAutoRouteKey,
			Name:           "auto",
			Enabled:        true,
			UserSelectable: true,
			Groups:         []string{"default"},
		}},
	}
)

func IsAutoRouteKey(group string) bool {
	return autoRouteKeyPattern.MatchString(strings.TrimSpace(group))
}

func GetAutoGroupRoutesConfig() AutoGroupRoutesConfig {
	autoGroupRoutesMu.RLock()
	defer autoGroupRoutesMu.RUnlock()
	return cloneAutoGroupRoutesConfig(autoGroupRoutes)
}

func GetAutoRoute(routeKey string) (AutoGroupRoute, bool) {
	routeKey = strings.TrimSpace(routeKey)
	autoGroupRoutesMu.RLock()
	defer autoGroupRoutesMu.RUnlock()
	for _, route := range autoGroupRoutes.Routes {
		if route.Key == routeKey {
			return cloneAutoGroupRoute(route), true
		}
	}
	return AutoGroupRoute{}, false
}

func GetDefaultAutoRouteKey() string {
	autoGroupRoutesMu.RLock()
	defer autoGroupRoutesMu.RUnlock()
	return autoGroupRoutes.DefaultRoute
}

func AutoGroupRoutes2JsonString() string {
	data, err := common.Marshal(GetAutoGroupRoutesConfig())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func UpdateAutoGroupRoutesByJsonString(value string) error {
	config, err := ParseAutoGroupRoutesConfig(value)
	if err != nil {
		return err
	}
	autoGroupRoutesMu.Lock()
	autoGroupRoutes = config
	for _, route := range config.Routes {
		if route.Key == config.DefaultRoute {
			autoGroups = slices.Clone(route.Groups)
			break
		}
	}
	autoGroupRoutesMu.Unlock()
	return nil
}

func ParseAutoGroupRoutesConfig(value string) (AutoGroupRoutesConfig, error) {
	var config AutoGroupRoutesConfig
	if err := common.UnmarshalJsonStr(value, &config); err != nil {
		return AutoGroupRoutesConfig{}, err
	}
	if config.Version == 0 {
		config.Version = 1
	}
	if config.Version != 1 {
		return AutoGroupRoutesConfig{}, fmt.Errorf("unsupported auto route config version: %d", config.Version)
	}
	config.DefaultRoute = strings.TrimSpace(config.DefaultRoute)
	if config.DefaultRoute == "" {
		config.DefaultRoute = DefaultAutoRouteKey
	}
	if !IsAutoRouteKey(config.DefaultRoute) || len(config.Routes) == 0 || len(config.Routes) > 32 {
		return AutoGroupRoutesConfig{}, errors.New("auto route config has an invalid default or route count")
	}

	seen := make(map[string]struct{}, len(config.Routes))
	hasEnabledDefault := false
	for i := range config.Routes {
		route := &config.Routes[i]
		route.Key = strings.TrimSpace(route.Key)
		if !IsAutoRouteKey(route.Key) {
			return AutoGroupRoutesConfig{}, fmt.Errorf("invalid auto route key: %s", route.Key)
		}
		if _, exists := seen[route.Key]; exists {
			return AutoGroupRoutesConfig{}, fmt.Errorf("duplicate auto route key: %s", route.Key)
		}
		seen[route.Key] = struct{}{}
		if route.Name == "" {
			route.Name = route.Key
		}
		if len([]rune(route.Name)) > 64 || len(route.Groups) == 0 || len(route.Groups) > 64 {
			return AutoGroupRoutesConfig{}, fmt.Errorf("auto route %s has an invalid name or group count", route.Key)
		}
		groups := make([]string, 0, len(route.Groups))
		groupSeen := make(map[string]struct{}, len(route.Groups))
		for _, group := range route.Groups {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			if IsAutoRouteKey(group) {
				return AutoGroupRoutesConfig{}, fmt.Errorf("auto route %s must contain real group names only", route.Key)
			}
			if _, exists := groupSeen[group]; exists {
				continue
			}
			groupSeen[group] = struct{}{}
			groups = append(groups, group)
		}
		if len(groups) == 0 {
			return AutoGroupRoutesConfig{}, fmt.Errorf("auto route %s has no real groups", route.Key)
		}
		route.Groups = groups
		if route.Key == config.DefaultRoute && route.Enabled {
			hasEnabledDefault = true
		}
	}
	if !hasEnabledDefault {
		return AutoGroupRoutesConfig{}, errors.New("default auto route must exist and be enabled")
	}
	return cloneAutoGroupRoutesConfig(config), nil
}

func ValidateAutoGroupRoutesJsonString(value string) error {
	_, err := ParseAutoGroupRoutesConfig(value)
	return err
}

func cloneAutoGroupRoutesConfig(config AutoGroupRoutesConfig) AutoGroupRoutesConfig {
	config.Routes = slices.Clone(config.Routes)
	for i := range config.Routes {
		config.Routes[i] = cloneAutoGroupRoute(config.Routes[i])
	}
	return config
}

func cloneAutoGroupRoute(route AutoGroupRoute) AutoGroupRoute {
	route.Groups = slices.Clone(route.Groups)
	return route
}
