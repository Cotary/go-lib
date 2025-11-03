package chain

import (
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

// IsValidBTCAddress 检查一个字符串是否是合法的比特币地址
func IsValidBTCAddress(address string) bool {
	isValid, _ := CheckBTCAddressCompatibility(address)
	return isValid
}

// CheckBTCAddressCompatibility 检查一个字符串是否是合法的比特币地址，并返回其所属网络。
// 返回值:
//   - bool: 地址是否合法
//   - string: 所属网络名称 ("MainNet", "TestNet", "Regtest", 或 "Unknown")
func CheckBTCAddressCompatibility(addr string) (bool, string) {
	// 存储所有需要校验的网络参数
	// 为了方便显示，我们使用一个 map 来关联参数和网络名称
	networkParams := map[string]*chaincfg.Params{
		"MainNet": &chaincfg.MainNetParams,
		"TestNet": &chaincfg.TestNet3Params,
		"Regtest": &chaincfg.RegressionNetParams,
	}

	for netName, params := range networkParams {
		// btcutil.DecodeAddress 会根据参数（params）进行 Base58Check 或 Bech32 的校验和检查
		_, err := btcutil.DecodeAddress(addr, params)

		if err == nil {
			// 解码成功，地址属于这个网络
			return true, netName
		}
	}

	// 尝试所有网络都失败
	return false, "Unknown"
}

func testBTC() {
	validAddr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"           // P2PKH (Base58Check)
	validSegwit := "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq" // Bech32
	invalidAddr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNax"

	fmt.Printf("BTC 地址 %s 是否合法: %v\n", validAddr, IsValidBTCAddress(validAddr))
	fmt.Printf("BTC SegWit 地址 %s 是否合法: %v\n", validSegwit, IsValidBTCAddress(validSegwit))
	fmt.Printf("BTC 地址 %s 是否合法: %v\n", invalidAddr, IsValidBTCAddress(invalidAddr))
}
