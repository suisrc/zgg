// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package zc

import (
	"net"
	"os"
	"strings"
)

var (
	loc_areaip = ""
	host_name_ = ""
	namespace_ = ""
	serve_name = ""
)

// 获取局域网地址
func GetLocAreaIp() string {
	if loc_areaip != "" {
		return loc_areaip
	}
	hnam := GetHostname()
	if hnam == "" || hnam == "localhost" {
		loc_areaip = "127.0.0.1"
		return loc_areaip // 无法解析当前节点的局域网地址
	}
	// 通过 /etc/hosts 获取局域网地址
	bts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		Printf1("unable to read /etc/hosts: %s", err.Error())
	} else {
		for line := range strings.SplitSeq(string(bts), "\n") {
			if strings.HasPrefix(line, "#") {
				continue
			}
			ips := strings.Fields(line)
			if len(ips) < 2 || ips[0] == "127.0.0.1" {
				continue
			}
			found := false
			for _, name := range ips[1:] {
				if EqualFold(name, hnam) {
					found = true
					break // 找到与 hostname 匹配的 IP
				}
			}
			if !found {
				continue
			}
			// 判断是否为 IPv4 地址
			if ip := net.ParseIP(strings.TrimSpace(ips[0])); ip == nil {
			} else if v4 := ip.To4(); v4 == nil {
			} else if v4.IsLoopback() {
			} else {
				loc_areaip = strings.TrimSpace(ips[0])
				break
			}
		}
	}
	if loc_areaip == "" {
		loc_areaip = "127.0.0.1" // 无法解析
	}
	return loc_areaip
}

// 获取局域网域名
func GetHostname() string {
	if host_name_ != "" {
		return host_name_
	}
	host_name_, _ = os.Hostname()
	if host_name_ == "" {
		host_name_ = "localhost"
	}
	return host_name_
}

func GetNamespace() string {
	if namespace_ != "" {
		return namespace_
	}
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		Printf1("unable to read namespace: %s, return '-'", err.Error())
		namespace_ = "-"
	} else {
		namespace_ = string(ns)
	}
	return namespace_
}

func GetServeName() string {
	if serve_name != "" {
		return serve_name
	}
	ns := GetNamespace()
	if ns == "-" {
		serve_name = "serv." + GetHostname() // 不是 k8s
	} else {
		serve_name = "kube." + ns + "." + GetHostname() // k8s 环境
	}
	return serve_name
}
