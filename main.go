package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
)

type PingResult struct {
	Target      string
	Success     bool
	ResponseTime time.Duration
	StatusCode  int
	Error       error
}

func main() {
	// 命令行参数
	target := flag.String("t", "", "目标地址 (必需)")
	pingType := flag.String("type", "http", "Ping 类型: http, https, tcp, icmp")
	count := flag.Int("c", 4, "Ping 次数")
	timeout := flag.Int("timeout", 5, "超时时间(秒)")
	interval := flag.Int("i", 1, "每次 ping 间隔(秒)")
	continuous := flag.Bool("continuous", false, "持续 ping (Ctrl+C 停止)")
	test := flag.String("test", "", "これは何か")
	//测试
	flag.Parse()

	if *target == "" {
		fmt.Println(ColorRed + "错误: 必须指定目标地址 -t" + ColorReset)
		flag.Usage()
		os.Exit(1)
	}

	printHeader(*target, *pingType)

	var results []PingResult
	successCount := 0
	totalTime := time.Duration(0)

	pingCount := *count
	if *continuous {
		pingCount = -1 // 无限次
	}

	iteration := 0
	for {
		if pingCount > 0 && iteration >= pingCount {
			break
		}

		var result PingResult
		switch strings.ToLower(*pingType) {
		case "http", "https":
			result = pingHTTP(*target, *pingType, time.Duration(*timeout)*time.Second)
		case "tcp":
			result = pingTCP(*target, time.Duration(*timeout)*time.Second)
		case "icmp":
			fmt.Println(ColorYellow + "注意: ICMP ping 需要 root 权限，改用 TCP 连接测试" + ColorReset)
			result = pingTCP(*target, time.Duration(*timeout)*time.Second)
		default:
			fmt.Printf(ColorRed+"不支持的 ping 类型: %s\n"+ColorReset, *pingType)
			os.Exit(1)
		}

		results = append(results, result)
		printResult(result, iteration+1)

		if result.Success {
			successCount++
			totalTime += result.ResponseTime
		}

		iteration++

		if pingCount < 0 || iteration < pingCount {
			time.Sleep(time.Duration(*interval) * time.Second)
		}
	}

	printSummary(results, successCount, totalTime)
}

func printHeader(target, pingType string) {
	fmt.Printf("\n%s=== 服务健康检查工具 ===%s\n", ColorCyan, ColorReset)
	fmt.Printf("目标: %s\n", target)
	fmt.Printf("类型: %s\n", strings.ToUpper(pingType))
	fmt.Printf("时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
}

func pingHTTP(target, protocol string, timeout time.Duration) PingResult {
	result := PingResult{Target: target}

	// 确保 URL 格式正确
	url := target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		url = protocol + "://" + target
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不跟随重定向
		},
	}

	start := time.Now()
	resp, err := client.Get(url)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Success = resp.StatusCode < 500 // 状态码 < 500 视为成功

	return result
}

func pingTCP(target string, timeout time.Duration) PingResult {
	result := PingResult{Target: target}

	// 如果没有端口，默认使用 80
	if !strings.Contains(target, ":") {
		target += ":80"
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", target, timeout)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Error = err
		return result
	}
	defer conn.Close()

	result.Success = true
	return result
}

func printResult(result PingResult, seq int) {
	prefix := fmt.Sprintf("[%d]", seq)

	if result.Success {
		if result.StatusCode > 0 {
			fmt.Printf("%s %s响应来自 %s: 状态=%d 时间=%v%s\n",
				prefix, ColorGreen, result.Target, result.StatusCode,
				result.ResponseTime.Round(time.Millisecond), ColorReset)
		} else {
			fmt.Printf("%s %s响应来自 %s: 连接成功 时间=%v%s\n",
				prefix, ColorGreen, result.Target,
				result.ResponseTime.Round(time.Millisecond), ColorReset)
		}
	} else {
		fmt.Printf("%s %s请求失败 %s: %v%s\n",
			prefix, ColorRed, result.Target, result.Error, ColorReset)
	}
}

func printSummary(results []PingResult, successCount int, totalTime time.Duration) {
	fmt.Printf("\n%s=== 统计信息 ===%s\n", ColorCyan, ColorReset)
	fmt.Printf("发送: %d, 成功: %d, 失败: %d (%.1f%% 丢包)\n",
		len(results), successCount, len(results)-successCount,
		float64(len(results)-successCount)/float64(len(results))*100)

	if successCount > 0 {
		avgTime := totalTime / time.Duration(successCount)
		fmt.Printf("平均响应时间: %v\n", avgTime.Round(time.Millisecond))

		// 计算最小和最大响应时间
		var minTime, maxTime time.Duration
		first := true
		for _, r := range results {
			if r.Success {
				if first {
					minTime = r.ResponseTime
					maxTime = r.ResponseTime
					first = false
				} else {
					if r.ResponseTime < minTime {
						minTime = r.ResponseTime
					}
					if r.ResponseTime > maxTime {
						maxTime = r.ResponseTime
					}
				}
			}
		}
		fmt.Printf("最小/最大响应时间: %v / %v\n",
			minTime.Round(time.Millisecond), maxTime.Round(time.Millisecond))
	}

	// 健康状态评估
	successRate := float64(successCount) / float64(len(results)) * 100
	var status, color string
	switch {
	case successRate == 100:
		status = "优秀"
		color = ColorGreen
	case successRate >= 90:
		status = "良好"
		color = ColorGreen
	case successRate >= 70:
		status = "一般"
		color = ColorYellow
	default:
		status = "较差"
		color = ColorRed
	}
	fmt.Printf("\n服务健康状态: %s%s%s\n\n", color, status, ColorReset)
}