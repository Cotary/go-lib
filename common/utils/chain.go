package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/btcsuite/btcd/btcutil/base58"
	"math/big"
	"strconv"
	"strings"
	"unicode"

	tron58 "github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
)

func HexToDecimal(hexString string) decimal.Decimal {
	decimalValue := new(big.Int)
	if strings.HasPrefix(hexString, "0x") || strings.HasPrefix(hexString, "0X") {
		hexString = hexString[2:]
	}
	// 将十六进制字符串转换为大整数
	decimalValue.SetString(hexString, 16)
	return decimal.NewFromBigInt(decimalValue, 0)
}

func HexToDecimalStr(hexString string) string {
	if hexString == "" {
		return ""
	}
	return HexToDecimal(hexString).String()
}

func GetFormatAccuracy(full decimal.Decimal, accuracy int64) decimal.Decimal {
	if accuracy == 0 {
		return full
	}
	d := decimal.NewFromInt(10).Pow(decimal.NewFromInt(accuracy))
	return full.DivRound(d, int32(accuracy))
}

// FormatTokenName 解析十六进制字符串并返回格式化的代币名称
func FormatTokenName(name string) string {
	if len(name) >= 2 && name[:2] == "0x" {
		name = name[2:]
	}
	if len(name) < 64 {
		return ""
	}
	// 将十六进制字符串转换为字节数组
	nameBytes, err := hex.DecodeString(name)
	if err != nil {
		return ""
	}
	if len(nameBytes) < 96 {
		padding := make([]byte, 96-len(nameBytes))
		nameBytes = append(padding, nameBytes...)
	}
	length, _ := strconv.ParseInt(hex.EncodeToString(nameBytes[32:64]), 16, 64)
	if length == 0 {
		return removeGarbledCharacters(string(nameBytes))
	}
	nameStr := string(nameBytes[64:])
	return nameStr[0:length]

}

func removeGarbledCharacters(str string) string {
	var result []rune
	for _, r := range str {
		if unicode.IsPrint(r) {
			result = append(result, r)
		}
	}
	return string(result)
}

func HexToLower(input string) string {
	if strings.HasPrefix(input, "0x") {
		return strings.ToLower(input)
	}
	return input
}

func ETH2TRON(ethAddress string) (string, error) {
	if ethAddress == "" {
		return "", nil
	}
	// 去掉0x前缀
	if ethAddress[:2] == "0x" {
		ethAddress = ethAddress[2:]
	}

	// 解码16进制地址
	addressBytes, err := hex.DecodeString(ethAddress)
	if err != nil {
		return "", err
	}

	// 添加41前缀
	address41 := append([]byte{0x41}, addressBytes...)

	// 使用SHA256函数对地址进行两次哈希，并取前4字节作为校验码
	firstHash := sha256.Sum256(address41)
	secondHash := sha256.Sum256(firstHash[:])
	checksum := secondHash[:4]

	// 将校验码添加到初始地址的末尾，并通过Base58编码获得一个Base58Check格式的地址
	finalAddress := append(address41, checksum...)
	tronAddress := base58.Encode(finalAddress)

	return tronAddress, nil

}

func TRON2ETH(tronAddress string) (string, error) {
	if tronAddress == "" {
		return "", nil
	}
	// Base58Check解码
	decoded, err := tron58.Decode(tronAddress)
	if err != nil {
		return "", err
	}

	// 移除前缀41和校验码
	truncatedBytes := decoded[1 : len(decoded)-4]

	// 编码为16进制地址
	ethAddress := hex.EncodeToString(truncatedBytes)

	// 添加0x前缀
	ethAddress = "0x" + ethAddress

	return ethAddress, nil
}
