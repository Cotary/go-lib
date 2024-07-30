package utils

import (
	"github.com/gagliardetto/binary"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
	"strings"
)

func HexToString(hex string) (string, bool) {
	n := new(big.Int)
	n, ok := n.SetString(strings.Replace(hex, "0x", "", 1), 16)
	return n.String(), ok
}

func DecimalSymbol(val decimal.Decimal, symbol string) string {
	zeroNum := decimal.NewFromInt(0)
	t := val.Cmp(zeroNum)
	if t == 0 {
		return val.String()
	}

	if symbol == "+" {
		return AnyJoinToString("+", val.Abs().String())
	} else if symbol == "-" {
		return AnyJoinToString("-", val.Abs().String())
	} else {
		if t != -1 {
			return AnyJoinToString("+", val.String())
		} else {
			return val.String()
		}
	}

}
func NumberTruncate(num string, decimalPlaces int32) string {
	data, _ := decimal.NewFromString(num)
	return data.Truncate(decimalPlaces).String()
}

func NumberRatio(n, d interface{}, decimalPlaces int32) string {
	nd, _ := decimal.NewFromString(AnyToString(n))
	dd, _ := decimal.NewFromString(AnyToString(d))
	if dd.IsZero() {
		return "0"
	}
	return nd.Div(dd).Truncate(decimalPlaces).String()
}

func NumberRatioPercent(n, d interface{}, decimalPlaces int32) string {
	nd, _ := decimal.NewFromString(AnyToString(n))
	dd, _ := decimal.NewFromString(AnyToString(d))
	if dd.IsZero() {
		return "0"
	}
	return nd.Div(dd).Truncate(decimalPlaces).Mul(decimal.NewFromInt(100)).String()
}

func BigInt2Uint128(i *big.Int) (u bin.Uint128, err error) {
	if i.Sign() < 0 {
		return u, errors.New("value cannot be negative")
	} else if i.BitLen() > 128 {
		return u, errors.New("value overflows Uint128")
	}
	u.Lo = i.Uint64()
	u.Hi = new(big.Int).Rsh(i, 64).Uint64()
	return u, nil
}

func AvgList(data []interface{}, filter bool, decimalPlaces int32) decimal.Decimal {
	var all decimal.Decimal
	var count int64
	for _, v := range data {
		val, _ := decimal.NewFromString(AnyToString(v))
		if filter && val.IsZero() {
			continue
		}
		count++
		all = all.Add(val)
	}
	if count == 0 {
		return decimal.Decimal{}
	}

	return all.Div(decimal.NewFromInt(count)).Truncate(decimalPlaces)
}
