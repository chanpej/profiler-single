// devicecheck 是真机链路自检工具：
// 以 Unity Editor Profiler 身份连接 Development Player，采集模块数据并输出分析报告。
//
// 用法：
//
//	go run ./cmd/devicecheck -usb -package com.company.game [-serial 7ddbdf55] [-duration 15s]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"unityperfagent/internal/adbforward"
	"unityperfagent/internal/analyzer"
	"unityperfagent/internal/collector"
	"unityperfagent/internal/device"
)

func main() {
	serial := flag.String("serial", "", "adb 设备 serial；省略则使用当前唯一 USB 设备")
	packageID := flag.String("package", "", "应用包名（localabstract:Unity-{包名}）")
	usb := flag.Bool("usb", false, "通过 adb forward 连接 USB 手机上的 Player")
	duration := flag.Duration("duration", 15*time.Second, "采集时长")
	flag.Parse()

	if err := run(*serial, *packageID, *usb, *duration); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(serial, packageID string, usb bool, duration time.Duration) error {
	fmt.Println("== UnityPerfAgent Profiler 链路自检 ==")

	if !usb {
		return fmt.Errorf("请使用 -usb 连接真机")
	}
	if err := adbforward.ValidatePackageID(packageID); err != nil {
		return err
	}

	dev, err := adbforward.ResolveSerial(nil, serial)
	if err != nil {
		return err
	}
	fmt.Printf("[1] USB 设备: %s %s\n", dev.Serial, dev.Model)
	specs := adbforward.ProbeForwardSpecs(nil, dev.Serial, packageID)
	if len(specs) == 0 {
		return fmt.Errorf("未找到 Unity Player 通道 localabstract:Unity-{包名}")
	}
	var tun adbforward.Tunnel
	var lastErr error
	for _, spec := range specs {
		if !strings.HasPrefix(spec.Remote, "localabstract:Unity-") {
			continue
		}
		tun, lastErr = adbforward.ForwardRemote(nil, dev.Serial, spec.Remote)
		if lastErr == nil {
			fmt.Printf("[2] adb forward 本机:%d -> %s，连接 Profiler\n", tun.LocalPort, spec.Remote)
			break
		}
		fmt.Printf("    跳过 %s: %v\n", spec.Remote, lastErr)
	}
	if lastErr != nil && tun.LocalPort == 0 {
		return fmt.Errorf("adb forward 失败: %w", lastErr)
	}
	defer func() { _ = adbforward.Remove(nil, tun) }()
	return collect("127.0.0.1", tun.LocalPort, duration)
}

func collect(host string, port int, duration time.Duration) error {
	session := collector.NewSession(0)
	client := device.NewClient()
	closed := make(chan error, 1)
	hs, err := client.Connect(host, port, session.AppendFrame, func(err error) { closed <- err })
	if err != nil {
		return fmt.Errorf("Profiler 连接失败: %w", err)
	}
	defer client.Close()

	fmt.Printf("[3] Player: %s / %s\n", hs.DeviceName, hs.OS)
	if hs.UnityVersion == "" {
		fmt.Println("    引擎版本: 未上报")
	} else {
		fmt.Printf("    引擎版本: %s\n", hs.UnityVersion)
	}

	fmt.Printf("[4] 采集中（%s）", duration)
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		select {
		case err := <-closed:
			return fmt.Errorf("采集中断: %v", err)
		case <-time.After(time.Second):
			fmt.Print(".")
		}
	}
	fmt.Println()
	// 数据流格式版本随流头到达，兼容性以它为准（改版引擎自报的版本号可能不是 2022）。
	if cur, err := client.Handshake(); err == nil && cur.ProfilerStreamVersion != 0 {
		verdict := "仅支持 Unity 2022 的数据流格式"
		if cur.Supported() {
			verdict = "受支持"
		}
		fmt.Printf("    数据流格式: %#x（%s）\n", cur.ProfilerStreamVersion, verdict)
	}
	client.Close()

	stats := session.Stats()
	if stats.FrameCount == 0 {
		return fmt.Errorf("未采集到 Profiler 帧（请确认 Development Build 且 USB Profiler 通道可用）")
	}
	fmt.Printf("[5] 采集结果: %d 帧\n", stats.FrameCount)
	fmt.Printf("    FPS      平均 %.1f  最低 %.1f  最高 %.1f\n", stats.AvgFPS, stats.MinFPS, stats.MaxFPS)
	fmt.Printf("    CPU/帧   平均 %.1fms  P95 %.1fms  峰值 %.1fms\n", stats.AvgCPUMs, stats.P95CPUMs, stats.MaxCPUMs)
	fmt.Printf("    内存     %.0fMB\n", stats.LastUsedMB)

	frames := session.Range(0, session.Len())
	if len(frames) > 0 {
		f := frames[0]
		fmt.Printf("    CPU 分类 Rendering %.1fms  Scripts %.1fms  DrawCalls %d\n",
			f.CPURenderingMs, f.CPUScriptsMs, f.DrawCalls)
	}

	report := analyzer.Analyze(frames)
	fmt.Printf("[6] 规则分析: %s\n", report.Summary)
	for _, issue := range report.Issues {
		fmt.Printf("    [%s] %s: %s\n", issue.Severity, issue.Title, issue.Detail)
	}

	if data, err := json.Marshal(report); err == nil {
		fmt.Printf("[7] 报告 JSON（可交给 ACP LLM Agent 诊断）: %d 字节\n", len(data))
	}
	fmt.Println("== 自检完成 ==")
	return nil
}
