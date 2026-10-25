package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// 公共 DNS 备用线路。家用路由器 / NAS 内置 DNS 常对多级 CNAME 的域名
// （如 uapis.cn → uapis.cn.a1.initaj.com）只返回 CNAME 或直接 NXDOMAIN，
// 导致 Go 报 "no such host"。系统 DNS 失败时自动切公共 DNS 重试。
var fallbackDNS = []string{
	"223.5.5.5:53",    // 阿里云
	"119.29.29.29:53", // 腾讯云
	"180.76.76.76:53", // 百度云
	"8.8.8.8:53",      // Google
	"1.1.1.1:53",      // Cloudflare
}

// externalTransport 供所有外网请求（SEO 接口、搜索引擎、Cloudinary）使用。
var externalTransport = &http.Transport{
	DialContext:           dialWithFallback,
	MaxIdleConns:          32,
	MaxIdleConnsPerHost:   8,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: time.Second,
	Proxy:                 http.ProxyFromEnvironment,
}

var externalClient = &http.Client{
	Timeout:   25 * time.Second,
	Transport: externalTransport,
}

// dialWithFallback 先按系统 DNS 拨号；解析失败时依次用公共 DNS 解析后再连。
func dialWithFallback(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, network, addr)
	if err == nil {
		return conn, nil
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil || net.ParseIP(host) != nil {
		return nil, err // 地址格式异常或本就是 IP，重试无意义
	}

	lastErr := err
	for _, dnsAddr := range fallbackDNS {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dd net.Dialer
				dd.Timeout = 4 * time.Second
				if c, e := dd.DialContext(ctx, "udp", dnsAddr); e == nil {
					return c, nil
				}
				return dd.DialContext(ctx, "tcp", dnsAddr)
			},
		}
		ips, rerr := r.LookupIPAddr(ctx, host)
		if rerr != nil {
			lastErr = rerr
			continue
		}
		for _, ip := range ips {
			if c, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port)); e == nil {
				return c, nil
			}
		}
	}
	return nil, lastErr
}

// friendlyNetErr 把 DNS 类错误翻译成用户可操作的中文提示。
func friendlyNetErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "server misbehaving") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection refused") {
		return fmt.Errorf("%s（已自动尝试备用 DNS 仍失败：请检查 NAS 的网络设置，将 DNS 改为 223.5.5.5 或 114.114.114.114 后重试）", msg)
	}
	return err
}
