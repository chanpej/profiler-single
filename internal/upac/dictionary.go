package upac

import (
	"fmt"
	"unityperf/internal/unityprofiler"
)

// Each Player stream starts its metadata sequence at zero. Frame streams may
// legitimately start later after Clear; their sequence is independent.
func validateDictionary(raw [][]byte) error {
	var next int32
	for i, body := range raw {
		if unityprofiler.IsStreamHeader(body) {
			if _, err := unityprofiler.ParseStreamHeader(body); err != nil {
				return err
			}
			next = 0
			continue
		}
		if i == 0 {
			return fmt.Errorf("录制缺少 PD3U 流头，无法完整回放。请重新连接 Player 后录制")
		}
		chunk, err := unityprofiler.ParseChunk(body)
		if err != nil {
			return fmt.Errorf("分块 %d 无效：%w", i, err)
		}
		if chunk.IsMetadata() {
			if chunk.Seq != next {
				return fmt.Errorf("marker 元数据不完整：分块 %d 的序号为 %d，期望 %d。请重新连接 Player 后录制", i, chunk.Seq, next)
			}
			next++
		} else if next == 0 {
			return fmt.Errorf("第 %d 个分块为线程数据，但缺少初始 marker 元数据。请重新连接 Player 后录制", i)
		}
	}
	return nil
}
