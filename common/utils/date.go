package utils

import (
	"fmt"

	"github.com/dromara/carbon/v2"
)

func demo() {
	fmt.Println("--- 1. 时间创建与解析 ---")
	// 当前时间
	now := carbon.Now()
	fmt.Printf("现在时间: %s\n", now)

	// 从时间戳创建
	t1 := carbon.CreateFromTimestamp(1679836800)
	fmt.Printf("秒级时间戳转换: %s\n", t1)

	// 从字符串解析
	t2 := carbon.Parse("2026-05-20 13:14:15")
	fmt.Printf("字符串解析: %s\n", t2)

	// --- 2. 范围获取 (Boundaries) ---
	fmt.Println("\n--- 2. 时间范围获取 ---")
	fmt.Printf("今天开始: %s\n", carbon.Now().StartOfDay())
	fmt.Printf("今天结束: %s\n", carbon.Now().EndOfDay())
	fmt.Printf("本周开始 (周一): %s\n", carbon.Now().StartOfWeek())
	fmt.Printf("本月结束: %s\n", carbon.Now().EndOfMonth())

	// --- 3. 时间偏移与计算 (Arithmetic) ---
	fmt.Println("\n--- 3. 时间偏移与计算 ---")
	// 链式操作：3天前的开始时间
	threeDaysAgo := carbon.Now().AddDays(-3).StartOfDay()
	fmt.Printf("3天前的凌晨: %s\n", threeDaysAgo)

	// 加一个月并自动处理月末（如1月31日加一月会变成2月28日）
	nextMonth := carbon.Parse("2026-01-31").AddMonth()
	fmt.Printf("1月31日加一月: %s\n", nextMonth)

	// 计算月差
	diff := carbon.Parse("2025-01-15").DiffInMonths(carbon.Parse("2026-03-26"))
	fmt.Printf("2025-01到2026-03相差月数: %d\n", diff)

	// --- 4. 格式化输出 (Formatting) ---
	fmt.Println("\n--- 4. 格式化输出 ---")
	c := carbon.Now()
	fmt.Printf("日期时间串: %s\n", c.ToDateTimeString())
	fmt.Printf("ISO8601串: %s\n", c.ToIso8601String())
	fmt.Printf("自定义布局: %s\n", c.Layout("2006年01月02日 15时"))

	// --- 5. 逻辑判断与比较 (Comparison) ---
	fmt.Println("\n--- 5. 逻辑判断与比较 ---")
	today := carbon.Now()
	yesterday := carbon.Now().SubDay()

	fmt.Printf("今天是否晚于昨天: %v\n", today.Gt(yesterday))
	fmt.Printf("今天是否是周末: %v\n", today.IsWeekend())
	fmt.Printf("是否是闰年: %v\n", today.IsLeapYear())

	// --- 6. 人性化/生活化功能 ---
	fmt.Println("\n--- 6. 人性化功能 ---")
	birthday := carbon.Parse("2000-01-01")
	fmt.Printf("2000年出生的人现在的年龄: %d\n", birthday.Age())
	fmt.Printf("2000-01-01 的星座: %s\n", birthday.Constellation())
	fmt.Printf("距离现在的人性化描述: %s\n", carbon.Now().AddHours(2).DiffForHumans())
	// 输出类似: 2 hours from now
}
