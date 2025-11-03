package chain

import (
	"fmt"

	// 导入 gotron-sdk/pkg/address
	"github.com/fbsobreira/gotron-sdk/pkg/address"
)

// IsValidTRXAddress 检查一个字符串是否是合法的 TRON 地址
func IsValidTRXAddress(addr string) bool {
	// 1. 检查长度和前缀（虽然 Base58ToAddress 也会检查，但先检查可快速过滤）
	if len(addr) != 34 || addr[0] != 'T' {
		return false
	}

	// 2. 使用库进行 Base58Check 解码和校验和检查
	// 如果 Base58Check 校验和不正确，或者解码后的长度不对，都会返回错误。
	_, err := address.Base58ToAddress(addr)

	return err == nil
}

func testTron() {
	// 这是一个合法的 TRON 地址示例 (以 'T' 开头, 34 字符, 校验和正确)
	validAddr := "TJRyB1iR61N8qN5qK8L52QY26sR5qC5R8F"

	// 这是一个校验和错误的地址 (最后一位改动)
	invalidChecksum := "TJRyB1iR61N8qN5qK8L52QY26sR5qC5R8G"

	// 这是一个长度错误的地址
	invalidLength := "TJRyB1iR61N8qN5qK8L52QY26sR5qC5R"

	fmt.Printf("TRON 地址 %s 是否合法: %v\n", validAddr, IsValidTRXAddress(validAddr))
	fmt.Printf("TRON 地址 %s (校验和错误) 是否合法: %v\n", invalidChecksum, IsValidTRXAddress(invalidChecksum))
	fmt.Printf("TRON 地址 %s (长度错误) 是否合法: %v\n", invalidLength, IsValidTRXAddress(invalidLength))
}
