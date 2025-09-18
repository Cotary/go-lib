package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"
	"unicode"

	"github.com/btcsuite/btcd/btcutil/base58"
	tron58 "github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
)

// HexToDecimal 将十六进制字符串转换为 decimal.Decimal
func HexToDecimal(hexStr string) decimal.Decimal {
	value := new(big.Int)
	if strings.HasPrefix(hexStr, "0x") || strings.HasPrefix(hexStr, "0X") {
		hexStr = hexStr[2:]
	}
	value.SetString(hexStr, 16)
	return decimal.NewFromBigInt(value, 0)
}

// HexToDecimalString 将十六进制字符串转换为十进制字符串
func HexToDecimalString(hexStr string) string {
	if hexStr == "" {
		return ""
	}
	return HexToDecimal(hexStr).String()
}

// FormatWithAccuracy 按精度格式化 decimal.Decimal
func FormatWithAccuracy(amount decimal.Decimal, accuracy int64) decimal.Decimal {
	if accuracy == 0 {
		return amount
	}
	divisor := decimal.NewFromInt(10).Pow(decimal.NewFromInt(accuracy))
	return amount.DivRound(divisor, int32(accuracy))
}

// ParseTokenName 从十六进制字符串解析代币名称
func ParseTokenName(hexName string) string {
	if strings.HasPrefix(hexName, "0x") {
		hexName = hexName[2:]
	}
	if len(hexName) < 64 {
		return ""
	}

	nameBytes, err := hex.DecodeString(hexName)
	if err != nil {
		return ""
	}

	if len(nameBytes) < 96 {
		padding := make([]byte, 96-len(nameBytes))
		nameBytes = append(padding, nameBytes...)
	}

	length, _ := strconv.ParseInt(hex.EncodeToString(nameBytes[32:64]), 16, 64)
	if length == 0 {
		return removeNonPrintableChars(string(nameBytes))
	}

	nameStr := string(nameBytes[64:])
	return nameStr[:length]
}

// removeNonPrintableChars 移除不可打印字符
func removeNonPrintableChars(s string) string {
	var result []rune
	for _, r := range s {
		if unicode.IsPrint(r) {
			result = append(result, r)
		}
	}
	return string(result)
}

// NormalizeHexLower 将 0x 前缀的十六进制字符串转为小写
func NormalizeHexLower(input string) string {
	if strings.HasPrefix(input, "0x") {
		return strings.ToLower(input)
	}
	return input
}

// EthToTron 将 ETH 地址转换为 TRON 地址
func EthToTron(ethAddr string) (string, error) {
	if ethAddr == "" {
		return "", nil
	}
	if strings.HasPrefix(ethAddr, "0x") {
		ethAddr = ethAddr[2:]
	}

	addrBytes, err := hex.DecodeString(ethAddr)
	if err != nil {
		return "", err
	}

	addr41 := append([]byte{0x41}, addrBytes...)

	firstHash := sha256.Sum256(addr41)
	secondHash := sha256.Sum256(firstHash[:])
	checksum := secondHash[:4]

	finalAddr := append(addr41, checksum...)
	return base58.Encode(finalAddr), nil
}

// TronToEth 将 TRON 地址转换为 ETH 地址
func TronToEth(tronAddr string) (string, error) {
	if tronAddr == "" {
		return "", nil
	}

	decoded, err := tron58.Decode(tronAddr)
	if err != nil {
		return "", err
	}

	truncated := decoded[1 : len(decoded)-4]
	return "0x" + hex.EncodeToString(truncated), nil
}
