package utils

import (
	"net"
	"net/url"
	"strings"
)

// IsValidURL 校验是否为合法的 HTTP(S) URL
// 要求：scheme 为 http/https，host 为合法域名（(([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,})）或合法 IP
func IsValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	return isValidDomain(host)
}

// IsInternalURL 校验 URL 是否指向内网地址（localhost / 127.x / 0.0.0.0 / 10.x / 172.16-31.x / 192.168.x 等）
func IsInternalURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// isValidDomain 校验域名格式：至少包含一个 '.'，TLD 至少 2 个纯字母字符，每段仅允许字母/数字/连字符
func isValidDomain(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return false
	}
	tld := parts[len(parts)-1]
	if len(tld) < 2 {
		return false
	}
	for _, c := range tld {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	for _, part := range parts {
		if len(part) == 0 {
			return false
		}
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}
