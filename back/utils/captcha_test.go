package utils

import (
	"back/global"
	"errors"
	"testing"
)

func TestCaptchaWithoutRedisDoesNotPanic(t *testing.T) {
	previous := global.RedisDB
	global.RedisDB = nil
	t.Cleanup(func() { global.RedisDB = previous })
	if _, _, err := CreateCaptcha(); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("CreateCaptcha() error = %v, want unavailable", err)
	}
	if valid, err := VerifyCaptcha("missing", "1234"); valid || !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("VerifyCaptcha() = %t, %v; want false and unavailable", valid, err)
	}
	if (&redisStore{}).Verify("missing", "", true) {
		t.Fatal("empty answer must never verify")
	}
}
