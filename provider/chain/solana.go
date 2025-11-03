package chain

import (
	"fmt"
	"github.com/gagliardetto/solana-go"
)

// IsValidSOLAddress 检查一个字符串是否是合法的 Solana 地址（公钥）
func IsValidSOLAddress(address string) bool {
	// solana.PublicKeyFromBase58 函数会进行 Base58 解码和长度校验（32字节）
	_, err := solana.PublicKeyFromBase58(address)
	return err == nil
}

func testSOL() {
	validAddr := "B32bN4T8xM7tLMS5xH32o68n714x16c2R9G1Q2T4D5P8" // 示例地址
	invalidAddr := "invalid-solana-address-test"

	fmt.Printf("SOL 地址 %s 是否合法: %v\n", validAddr, IsValidSOLAddress(validAddr))
	fmt.Printf("SOL 地址 %s 是否合法: %v\n", invalidAddr, IsValidSOLAddress(invalidAddr))
}
