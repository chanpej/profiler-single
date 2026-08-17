package protocol

// SampleNode 是 CPU Hierarchy 中的一个采样（对应 Unity Profiler Hierarchy 一行）。
// 不进入 164 字节帧快照，仅随选中帧的详情接口下发。
type SampleNode struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	TotalMs  float32 `json:"totalMs"`
	SelfMs   float32 `json:"selfMs"`
	Calls    uint32  `json:"calls"`
	// StartMs 相对本帧起点的开始时间，供 Timeline 按真实时序摆放；Hierarchy 合并后取最早一次。
	StartMs float32 `json:"startMs,omitempty"`
	// GCAllocKB 含子树分配量（与 Unity Profiler 的 GC Alloc 列一致），不是仅自身。
	GCAllocKB float32      `json:"gcAllocKB"`
	Callstack []string     `json:"callstack,omitempty"`
	Children  []SampleNode `json:"children,omitempty"`
}
