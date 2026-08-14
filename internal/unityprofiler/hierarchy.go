package unityprofiler

import (
	"fmt"
	"sort"
	"strings"

	"unityperfagent/internal/protocol"
)

const (
	maxHierarchyNodes = 1200
	nsPerMSHierarchy  = 1e6
)

func cloneValues(in []value) []value {
	if len(in) == 0 {
		return nil
	}
	out := make([]value, len(in))
	for i, v := range in {
		raw := make([]byte, len(v.Raw))
		copy(raw, v.Raw)
		out[i] = value{Type: v.Type, Raw: raw}
	}
	return out
}

func buildHierarchy(d *Decoder, roots []*treeNode) []protocol.SampleNode {
	nodes := treeToSamples(d, roots)
	nodes = mergeSampleNodes(nodes)
	// 先把分配量上浮，再裁剪：这样即使子树因预算被砍掉，父行仍显示完整分配量，
	// 不会出现"父行 0 KB、展开后才冒出分配"的假象。
	rollUpGCAlloc(nodes)
	budget := maxHierarchyNodes
	return capSampleNodes(nodes, &budget)
}

// rollUpGCAlloc 把分配量累加到各级祖先，对齐 Unity Profiler 的 GC Alloc 列语义
// （该列是"含子树"的总量）。分配采样藏在 5~7 层深处，父行若只算自己就全是 0，
// 用户从根往下看每行都是空的，根本无从知道该展开哪条分支。
func rollUpGCAlloc(nodes []protocol.SampleNode) float32 {
	var sum float32
	for i := range nodes {
		n := &nodes[i]
		n.GCAllocKB += rollUpGCAlloc(n.Children)
		sum += n.GCAllocKB
	}
	return sum
}

func treeToSamples(d *Decoder, nodes []*treeNode) []protocol.SampleNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]protocol.SampleNode, 0, len(nodes))
	for _, n := range nodes {
		name, cat := "Sample", ""
		if info, ok := d.meta.markers[n.markerID]; ok {
			name = info.Name
			cat = d.meta.categoryName(info.Category)
		}
		s := protocol.SampleNode{
			Name:     name,
			Category: cat,
			TotalMs:  float32(float64(n.totalNS) / nsPerMSHierarchy),
			SelfMs:   float32(float64(n.selfNS) / nsPerMSHierarchy),
			Calls:    1,
			Children: treeToSamples(d, n.kids),
		}
		for _, v := range n.meta {
			switch v.Type {
			case DataTypeString, DataTypeString16:
				// 字符串元数据是采样自带的说明文本（如 UI 元素名），不是调用栈。
			default:
				if num, ok := v.Number(); ok && strings.EqualFold(name, "GC.Alloc") {
					s.GCAllocKB = float32(num / bytesPerKB)
				}
			}
		}
		s.Callstack = resolveCallstack(d, n.callstack)
		out = append(out, s)
	}
	return out
}

// resolveCallstack 把 Player 上报的代码地址翻译成可读栈帧：托管方法查方法表，
// 原生帧（引擎自身代码，方法表不覆盖）保留十六进制地址。
func resolveCallstack(d *Decoder, addrs []uint64) []string {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if name, ok := d.meta.methods.resolve(a); ok {
			out = append(out, name)
			continue
		}
		out = append(out, fmt.Sprintf("%#x", a))
	}
	return out
}

func mergeSampleNodes(nodes []protocol.SampleNode) []protocol.SampleNode {
	if len(nodes) == 0 {
		return nil
	}
	type acc struct {
		node protocol.SampleNode
		kids []protocol.SampleNode
	}
	order := make([]string, 0, len(nodes))
	byKey := make(map[string]*acc, len(nodes))
	for _, n := range nodes {
		key := n.Name + "\x00" + n.Category
		a, ok := byKey[key]
		if !ok {
			cp := n
			cp.Children = nil
			a = &acc{node: cp}
			byKey[key] = a
			order = append(order, key)
		} else {
			a.node.TotalMs += n.TotalMs
			a.node.SelfMs += n.SelfMs
			a.node.Calls += n.Calls
			a.node.GCAllocKB += n.GCAllocKB
			if len(a.node.Callstack) == 0 && len(n.Callstack) > 0 {
				a.node.Callstack = n.Callstack
			}
		}
		if len(n.Children) > 0 {
			a.kids = append(a.kids, n.Children...)
		}
	}
	out := make([]protocol.SampleNode, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		a.node.Children = mergeSampleNodes(a.kids)
		out = append(out, a.node)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TotalMs > out[j].TotalMs })
	return out
}

func capSampleNodes(nodes []protocol.SampleNode, budget *int) []protocol.SampleNode {
	if *budget <= 0 || len(nodes) == 0 {
		return nil
	}
	out := make([]protocol.SampleNode, 0, len(nodes))
	for _, n := range nodes {
		if *budget <= 0 {
			break
		}
		*budget--
		n.Children = capSampleNodes(n.Children, budget)
		out = append(out, n)
	}
	return out
}
