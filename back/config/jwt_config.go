package config

import "os"

func GetJWTSecret() string {
	if v := os.Getenv("JWT_SECRET"); v != "" {
		return v
	}
	if AppConfig != nil && AppConfig.JWT.Secret != "" {
		return AppConfig.JWT.Secret
	}
	return "change-me-in-production"
}

func GetJWTAccessExpireMinutes() int {
	if AppConfig != nil && AppConfig.JWT.AccessExpireMinutes > 0 {
		return AppConfig.JWT.AccessExpireMinutes
	}
	return 120
}

func GetJWTRefreshExpireDays() int {
	if AppConfig != nil && AppConfig.JWT.RefreshExpireDays > 0 {
		return AppConfig.JWT.RefreshExpireDays
	}
	return 7
}
