package unityprofiler

import (
	"encoding/binary"
	"fmt"
)

func (d *Decoder) waitForTuanjieFrame(st *threadStream) {
	st.buf = st.buf[:0]
	clear(st.stack)
	st.stack = st.stack[:0]
	d.frame = nil
	st.waitingForFrame = true
	d.stats.WaitingForFrame = true
}

func (st *threadStream) isTuanjieFrameStart(payload []byte) bool {
	if len(payload) < 16 || binary.LittleEndian.Uint32(payload) != recFrame {
		return false
	}
	if !st.haveFrameBoundary {
		return true
	}
	indexDelta := binary.LittleEndian.Uint32(payload[4:]) - st.lastFrameIndex
	timestamp := int64(binary.LittleEndian.Uint64(payload[8:]))
	return indexDelta > 0 && indexDelta < 1<<31 && timestamp > st.lastFrameNS
}

func (c *cursor) alignWord() { c.skip((-c.p) & 3) }

func (c *cursor) alignedString() string {
	value := c.str()
	c.alignWord()
	return value
}

func (c *cursor) alignedValues() []value {
	count := c.u32()
	if count > 255 {
		panic(fmt.Errorf("unityprofiler: 无效的团结采样元数据数量 %d", count))
	}
	values := make([]value, 0, count)
	for range count {
		dataType := byte(c.u32())
		var payload []byte
		if isVarLenValue(dataType) {
			payload = c.take(int(c.u32()))
			c.alignWord()
		} else if size, ok := valueFixedSize[dataType]; ok {
			payload = c.take(size)
		} else {
			panic(fmt.Errorf("unityprofiler: 未知的团结采样元数据类型 %#x", dataType))
		}
		values = append(values, value{Type: dataType, Raw: payload})
	}
	return values
}

func (d *Decoder) decodeTuanjieThread(c *cursor) {
	threadID := c.i64()
	c.skip(12)
	group, name := c.alignedString(), c.alignedString()
	if group != "" {
		name = group + "/" + name
	}
	d.meta.threads[threadID] = name
}

func (d *Decoder) decodeTuanjieMeta(_ *threadStream, c *cursor) error {
	opcode := c.u32()
	switch opcode {
	case recMetaMarker:
		id := c.u32()
		info := markerInfo{Flags: c.u16(), Category: c.u16(), Name: c.alignedString(), MetaCount: int(c.u32())}
		if info.MetaCount > 255 {
			return fmt.Errorf("unityprofiler: 无效的团结 marker 元数据数量 %d", info.MetaCount)
		}
		for index := range info.MetaCount {
			c.skip(4)
			info.recordField(index, c.alignedString())
		}
		d.meta.markers[id] = info
	case recMetaCategory:
		id := c.u16()
		c.skip(4)
		d.meta.categories[id] = c.alignedString()
	case recMetaThread:
		d.decodeTuanjieThread(c)
	case recMetaClass:
		c.skip(8)
		c.alignedString()
	case recMetaObject:
		c.skip(8)
		c.alignedString()
		c.skip(24)
	case recMetaObject2:
		c.skip(24)
	case recMetaFlush:
		c.skip(16)
		d.metaDone = true
	case recMetaCatFlag:
		c.skip(4)
	case recMetaMethod:
		c.skip(8)
		addr := uint64(c.i64())
		c.skip(4)
		name := c.alignedString()
		c.alignedString()
		c.skip(4)
		d.meta.methods.add(addr, name)
	default:
		return fmt.Errorf("unityprofiler: 团结元数据流未知记录类型 %#x", opcode)
	}
	d.stats.MetaRecords++
	return nil
}

func (d *Decoder) decodeTuanjieEvent(st *threadStream, c *cursor) error {
	opcode := c.u32()
	switch opcode {
	case recFrame:
		index := c.u32()
		ts := c.i64()
		if st.isMain {
			st.lastFrameIndex = index
			st.lastFrameNS = ts
			st.haveFrameBoundary = true
			d.rollFrame(index, ts, st)
		} else {
			d.rollWorkerFrame(index, ts, st)
		}
	case recMetaThread:
		d.decodeTuanjieThread(c)
	case recBeginPlain, recBeginExtra, recBeginMeta, recBeginObject:
		c.skip(4)
		entry := sampleEntry{markerID: c.u32(), startNS: c.i64()}
		if opcode == recBeginExtra {
			c.skip(4)
		}
		if opcode == recBeginMeta || opcode == recBeginObject {
			entry.meta = cloneValues(c.alignedValues())
		}
		st.stack = append(st.stack, entry)
	case recEnd:
		markerID := c.u32()
		if markerID == 0 {
			markerID = c.u32()
		}
		endNS := c.i64()
		d.closeTuanjieSample(st, markerID, endNS)
	case recCounter:
		c.skip(4)
		id := c.u32()
		c.skip(8)
		values := c.alignedValues()
		if d.frame != nil {
			for _, value := range values {
				if number, ok := value.Number(); ok {
					d.frame.counters[id] = number
					break
				}
			}
		}
	case recThreadClock:
		c.skip(8)
	case recInstanceHint:
		c.skip(4)
	case recAudioStats:
		c.skip(12)
		c.alignedString()
	case recLegacyStats:
		c.skip(1280)
	default:
		return fmt.Errorf("unityprofiler: 团结事件流未知记录类型 %#x", opcode)
	}
	d.stats.EventRecords++
	return nil
}

func (d *Decoder) closeTuanjieSample(st *threadStream, markerID uint32, endNS int64) {
	for index := len(st.stack) - 1; index >= 0; index-- {
		if st.stack[index].markerID != markerID {
			continue
		}
		matched := &st.stack[index]
		for _, incomplete := range st.stack[index+1:] {
			matched.kids = append(matched.kids, incomplete.kids...)
			matched.childNS += incomplete.childNS
		}
		clear(st.stack[index+1:])
		st.stack = st.stack[:index+1]
		d.closeSample(st, endNS)
		return
	}
}
