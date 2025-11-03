package chain

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"regexp"
	"strings"
)

// IsValidETHAddress 检查一个字符串是否是合法的以太坊地址
func IsValidETHAddress(address string) bool {
	// 1. 基本格式和长度校验 (0x + 40个十六进制字符)
	re := regexp.MustCompile("^0x[0-9a-fA-F]{40}$")
	if !re.MatchString(address) {
		return false
	}

	// 2. 检查 EIP-55 校验和（推荐，但非强制）
	// common.IsHexAddress 检查格式
	// common.HexToAddress(address).Hex() 将地址转换为标准的 EIP-55 校验格式

	// 如果所有字符都是小写或大写，则跳过 EIP-55 检查，因为它们在技术上也是有效的。
	if strings.ToLower(address) == address || strings.ToUpper(address) == address {
		return true // 纯大写或纯小写是合法的
	}

	// 检查是否符合 EIP-55 混合大小写校验规则
	return common.IsHexAddress(address) && common.HexToAddress(address).Hex() == address
}

func testETH() {
	validAddr := "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB" // EIP-55 校验
	lowerCase := "0xdbf03b407c01e7cd3cbea99509d93f8dddc8c6fb" // 纯小写，也合法
	invalidLen := "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6F" // 长度错误

	fmt.Printf("ETH 地址 %s 是否合法: %v\n", validAddr, IsValidETHAddress(validAddr))
	fmt.Printf("ETH 地址 %s 是否合法: %v\n", lowerCase, IsValidETHAddress(lowerCase))
	fmt.Printf("ETH 地址 %s 是否合法: %v\n", invalidLen, IsValidETHAddress(invalidLen))
}
