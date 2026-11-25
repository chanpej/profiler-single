package services

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unityperf/internal/adbforward"
	"unityperf/internal/upac"
)

type PhoneInfo = upac.PhoneInfo
type PhoneInventory struct {
	Devices  []PhoneInfo `json:"devices"`
	IP       string      `json:"ip"`
	Endpoint string      `json:"endpoint"`
	Error    string      `json:"error"`
}

const phoneProbe = `getprop ro.product.manufacturer; getprop ro.product.model; getprop ro.build.version.release; getprop ro.build.version.sdk; getprop ro.product.cpu.abi; getprop service.adb.tls.port; wm size; head -1 /proc/meminfo; ip -4 -o addr show dev wlan0`

func readPhone(d adbforward.Device, output string) PhoneInfo {
	phone := PhoneInfo{Serial: d.Serial, Model: d.Model}
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	port := 5555
	if len(lines) >= 6 {
		phone.Brand = lines[0]
		if lines[1] != "" {
			phone.Model = lines[1]
		}
		phone.Android = lines[2]
		phone.SDK = lines[3]
		phone.ABI = lines[4]
		if n, err := strconv.Atoi(lines[5]); err == nil && n > 0 && n <= 65535 {
			port = n
		}
		for _, line := range lines[6:] {
			if _, size, ok := strings.Cut(line, " size:"); ok {
				phone.Screen = strings.TrimSpace(size)
			}
			if strings.HasPrefix(line, "MemTotal:") {
				phone.Memory = strings.TrimSpace(strings.TrimPrefix(line, "MemTotal:"))
			}
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "inet" && i+1 < len(fields) {
					if addr, err := netip.ParsePrefix(fields[i+1]); err == nil {
						phone.IP = addr.Addr().String()
					}
				}
			}
		}
	}
	if addr, err := netip.ParseAddrPort(d.Serial); err == nil {
		phone.IP = addr.Addr().String()
		phone.Endpoint = addr.String()
	} else if ip, err := netip.ParseAddr(phone.IP); err == nil {
		phone.Endpoint = netip.AddrPortFrom(ip, uint16(port)).String()
	}
	return phone
}

func (p *PerfService) ConnectedPhones() PhoneInventory {
	p.mu.Lock()
	run := p.adbRun
	generation := p.captureGeneration
	serial, host := "", ""
	if p.connectedLocked() && p.sourceFile == "" {
		if p.transport == "adb" {
			serial = p.usbSerial
		} else {
			host = p.liveHost
		}
	}
	p.mu.Unlock()
	result := PhoneInventory{Devices: []PhoneInfo{}}
	if endpoint, err := wirelessEndpoint(host); err == nil {
		result.IP = host
		result.Endpoint = endpoint
	}
	devices, err := adbforward.ListDevices(run)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, d := range devices {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := run(ctx, "-s", d.Serial, "shell", phoneProbe)
		cancel()
		if err != nil {
			out = ""
		}
		phone := readPhone(d, out)
		result.Devices = append(result.Devices, phone)
	}
	for _, phone := range result.Devices {
		if (serial != "" && phone.Serial == serial) || (host != "" && phone.IP == host) {
			p.mu.Lock()
			if generation == p.captureGeneration && p.sourceFile == "" && (p.capturePhone == nil || phone.Android != "") {
				snapshot := phone
				p.capturePhone = &snapshot
			}
			p.mu.Unlock()
		}
		if phone.IP != "" && (phone.Serial == serial || phone.IP == host || (host == "" && serial == "" && len(devices) == 1)) {
			result.IP = phone.IP
			result.Endpoint = phone.Endpoint
			break
		}
	}
	return result
}
