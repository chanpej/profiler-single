package unityprofiler

import (
	"bytes"
	"strings"
)

const (
	forwardedLogEnvelope       = "UnityPerf.Log/1\x00"
	legacyForwardedLogEnvelope = "UnityPerfAgent.Log/1\x00"
)

var playerLogIDs = buildPlayerLogIDs()

func buildPlayerLogIDs() map[MessageID]bool {
	ids := map[MessageID]bool{
		{0x93, 0xa4, 0xad, 0x30, 0xb8, 0x0a, 0xf4, 0x62, 0x0b, 0x10, 0xa1, 0xc6, 0xed, 0x0b, 0xa5, 0x26}: true,
		{0xd3, 0xde, 0xd2, 0xad, 0xdc, 0x2f, 0x64, 0x8d, 0x3a, 0x6f, 0x10, 0x47, 0x71, 0x14, 0x7e, 0x9a}: true,
	}
	for _, canonical := range []MessageID{
		{0x39, 0x4a, 0xda, 0x03, 0x8b, 0xa0, 0x4f, 0x26, 0xb0, 0x01, 0x1a, 0x6c, 0xde, 0xb0, 0x5a, 0x62},
		{0x3d, 0xed, 0x2d, 0xda, 0xcd, 0xf2, 0x46, 0xd8, 0xa3, 0xf6, 0x01, 0x74, 0x17, 0x41, 0xe7, 0xa9},
	} {
		ids[canonical] = true
		dotnet := canonical
		dotnet[0], dotnet[1], dotnet[2], dotnet[3] = canonical[3], canonical[2], canonical[1], canonical[0]
		dotnet[4], dotnet[5], dotnet[6], dotnet[7] = canonical[5], canonical[4], canonical[7], canonical[6]
		ids[dotnet] = true
		var words MessageID
		for index, value := range canonical {
			words[index^3] = value
		}
		ids[words] = true
	}
	return ids
}

func dispatchPlayerLog(msg Message, handler func(string, string)) {
	if handler == nil {
		return
	}
	body, forwarded := bytes.CutPrefix(msg.Body, []byte(forwardedLogEnvelope))
	if !forwarded {
		body, forwarded = bytes.CutPrefix(msg.Body, []byte(legacyForwardedLogEnvelope))
	}
	var levelByte byte
	var text []byte
	switch {
	case forwarded && len(body) > 1:
		levelByte, text = body[0], body[1:]
	case playerLogIDs[msg.ID] && len(msg.Body) > 4:
		levelByte, text = msg.Body[0], msg.Body[4:]
	default:
		return
	}
	level := "Log"
	switch levelByte {
	case 0:
		level = "Error"
	case 1:
		level = "Assert"
	case 2:
		level = "Warning"
	case 4:
		level = "Exception"
	}
	if len(text) > 4096 {
		text = text[:4096]
	}
	handler(level, strings.TrimRight(strings.ToValidUTF8(string(text), "�"), "\x00"))
}
