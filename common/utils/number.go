package utils

import (
	bin "github.com/gagliardetto/binary"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"golang.org/x/exp/constraints"
	"math/big"
)

// FormatDecimalWithSign 格式化 decimal.Decimal，按符号规则输出
// symbol: "+" 强制正号, "-" 强制负号, 其他值则正数加 "+"
func FormatDecimalWithSign(val decimal.Decimal, symbol string) string {
	if val.IsZero() {
		return val.String()
	}

	switch symbol {
	case "+":
		return AnyJoinToString("+", val.Abs().String())
	case "-":
		return AnyJoinToString("-", val.Abs().String())
	default:
		if val.Sign() >= 0 {
			return AnyJoinToString("+", val.String())
		}
		return val.String()
	}
}

// TruncateDecimalString 将字符串形式的数字截断到指定小数位
func TruncateDecimalString(num string, decimalPlaces int32) string {
	data, err := decimal.NewFromString(num)
	if err != nil {
		return "0"
	}
	return data.Truncate(decimalPlaces).String()
}

// DecimalRatio 计算两个数的比值（截断到指定小数位）
func DecimalRatio(numerator, denominator interface{}, decimalPlaces int32) string {
	return decimalRatioInternal(numerator, denominator, decimalPlaces, false)
}

// DecimalRatioPercent 计算两个数的百分比（截断到指定小数位）
func DecimalRatioPercent(numerator, denominator interface{}, decimalPlaces int32) string {
	return decimalRatioInternal(numerator, denominator, decimalPlaces, true)
}

// decimalRatioInternal 公共逻辑
func decimalRatioInternal(numerator, denominator interface{}, decimalPlaces int32, percent bool) string {
	numDec, err1 := decimal.NewFromString(AnyToString(numerator))
	denDec, err2 := decimal.NewFromString(AnyToString(denominator))
	if err1 != nil || err2 != nil || denDec.IsZero() {
		return "0"
	}
	result := numDec.Div(denDec).Truncate(decimalPlaces)
	if percent {
		result = result.Mul(decimal.NewFromInt(100))
	}
	return result.String()
}

// BigIntToUint128 将 *big.Int 转换为 Uint128（限制非负且不超过 128 位）
func BigIntToUint128(i *big.Int) (u bin.Uint128, err error) {
	if i.Sign() < 0 {
		return u, errors.New("value cannot be negative")
	}
	if i.BitLen() > 128 {
		return u, errors.New("value overflows Uint128")
	}
	u.Lo = i.Uint64()
	u.Hi = new(big.Int).Rsh(i, 64).Uint64()
	return u, nil
}

// AverageIntList 计算整数切片的平均值（可过滤零值）
func AverageIntList[T constraints.Integer](data []T, filterZero bool, decimalPlaces int32) decimal.Decimal {
	var sum decimal.Decimal
	var count int64
	for _, v := range data {
		val := decimal.NewFromInt(int64(v))
		if filterZero && val.IsZero() {
			continue
		}
		count++
		sum = sum.Add(val)
	}
	if count == 0 {
		return decimal.Decimal{}
	}
	return sum.Div(decimal.NewFromInt(count)).Truncate(decimalPlaces)
}
