package common

import (
	"fmt"
	"os"
	"strconv"
)

func GetEnvOrDefault(env string, defaultValue int) int {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	num, err := strconv.Atoi(os.Getenv(env))
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %d", env, err.Error(), defaultValue))
		return defaultValue
	}
	return num
}

func GetEnvOrDefaultString(env string, defaultValue string) string {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	return os.Getenv(env)
}

func GetEnvOrDefaultBool(env string, defaultValue bool) bool {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(os.Getenv(env))
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %t", env, err.Error(), defaultValue))
		return defaultValue
	}
	return b
}

func IsChuamgweiCreditEnabled() bool {
	return GetEnvOrDefaultBool("CHUAMGWEI_CREDIT_ENABLED", false)
}

func GetChuamgweiUserSyncSecret() string {
	return GetEnvOrDefaultString("CHUAMGWEI_USER_SYNC_SECRET", "")
}

func GetChuamgweiDefaultTokenGroup() string {
	return GetEnvOrDefaultString("CHUAMGWEI_DEFAULT_TOKEN_GROUP", "default")
}

func GetChuamgweiDefaultTokenRemainQuota() int {
	return GetEnvOrDefault("CHUAMGWEI_DEFAULT_TOKEN_REMAIN_QUOTA", 0)
}

func IsChuamgweiDefaultTokenUnlimited() bool {
	return GetEnvOrDefaultBool("CHUAMGWEI_DEFAULT_TOKEN_UNLIMITED", true)
}
