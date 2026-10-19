package utils

import (
	"strings"
	"testing"
)

// ============================================================
// 基础：Go 测试函数必须以 Test 开头，参数是 *testing.T
// 运行：go test -v ./utils/
// ============================================================

func TestHashPassword_Success(t *testing.T) {
	// 1. 准备输入
	password := "mySecret123"

	// 2. 执行被测函数
	hash, err := HashPassword(password)

	// 3. 验证结果
	if err != nil {
		t.Fatalf("HashPassword() returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword() returned empty string")
	}
	if hash == password {
		t.Fatal("HashPassword() returned plain text, not a hash")
	}
	// bcrypt 生成的哈希以 $2a$ 开头
	if !strings.HasPrefix(hash, "$2a$") {
		t.Fatalf("HashPassword() returned unexpected format: %s", hash)
	}
}

func TestHashPassword_DifferentPasswordsYieldDifferentHashes(t *testing.T) {
	hash1, err1 := HashPassword("password123")
	hash2, err2 := HashPassword("password456")

	if err1 != nil || err2 != nil {
		t.Fatalf("HashPassword() returned error: err1=%v, err2=%v", err1, err2)
	}
	if hash1 == hash2 {
		t.Fatal("Different passwords should yield different hashes")
	}
}

func TestHashPassword_SamePasswordYieldsDifferentHashes(t *testing.T) {
	// bcrypt 每次生成不同的盐，所以相同密码的哈希也不一样（安全性）
	hash1, _ := HashPassword("samePassword")
	hash2, _ := HashPassword("samePassword")

	if hash1 == hash2 {
		t.Fatal("bcrypt should produce different hashes for the same password (random salt)")
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	password := "correctPassword"
	hash, _ := HashPassword(password)

	if !CheckPassword(password, hash) {
		t.Fatal("CheckPassword() should return true for the correct password")
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("correctPassword")

	if CheckPassword("wrongPassword", hash) {
		t.Fatal("CheckPassword() should return false for a wrong password")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	hash, _ := HashPassword("somePassword")

	if CheckPassword("", hash) {
		t.Fatal("CheckPassword() should return false for empty password")
	}
}

// ============================================================
// 进阶：表驱动测试 (Table-Driven Tests)
// Go 社区最标准的测试模式，面试官看了会点头
// ============================================================

func TestCheckPassword_TableDriven(t *testing.T) {
	// 先准备好一个已知的密码和哈希
	password := "testPassword"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 定义测试用例表
	tests := []struct {
		name     string // 用例名称（会显示在测试输出中）
		password string // 输入的密码
		want     bool   // 期望的结果
	}{
		{
			name:     "correct password returns true",
			password: password,
			want:     true,
		},
		{
			name:     "wrong password returns false",
			password: "wrongPassword",
			want:     false,
		},
		{
			name:     "empty password returns false",
			password: "",
			want:     false,
		},
		{
			name:     "case sensitive - uppercase",
			password: "TESTPASSWORD",
			want:     false,
		},
	}

	// 遍历执行每个用例
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckPassword(tt.password, hash)
			if got != tt.want {
				t.Errorf("CheckPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

// ============================================================
// 边界条件测试
// ============================================================

func TestHashPassword_EmptyPassword(t *testing.T) {
	// 空密码也能哈希（虽然业务上不应该允许，但函数不应该 panic）
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword('') should not error, got: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword('') returned empty hash")
	}
}

func TestHashPassword_LongPassword(t *testing.T) {
	// bcrypt 限制 72 字节，超长密码会被截断
	// 这是 bcrypt 的已知限制，不是我们代码的 bug
	// 正确做法：业务层在调用 HashPassword 前先校验密码长度
	longPassword := strings.Repeat("a", 100)
	_, err := HashPassword(longPassword)
	if err == nil {
		t.Fatal("HashPassword(100 chars) should return error: bcrypt has a 72-byte limit")
	}
	t.Logf("Expected error for 100-char password: %v", err)
}
