package upac

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"unityperf/internal/memsnap"
)

const memoryVersion uint16 = 3
const memoryFlag uint16 = 2
const maxMemoryBytes = 1 << 30
const maxMemoryEntries = 4096

type MemoryEntry struct {
	Time      int64         `json:"time"`
	Source    string        `json:"source"`
	Target    string        `json:"target"`
	Rows      []memsnap.Row `json:"rows"`
	Data      []byte        `json:"-"`
	CachePath string        `json:"-"` // Only set by the local capture service; never read from archive metadata.
}

func validMemory(e MemoryEntry, n uint64) bool {
	return (e.Source == "snap" && n > 0) || (e.Source == "pss" && n == 0)
}
func writeMemory(w io.Writer, entries []MemoryEntry) error {
	if len(entries) > maxMemoryEntries {
		return errors.New("内存记录过多")
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(entries))); err != nil {
		return err
	}
	remaining := uint64(maxMemoryBytes)
	for _, e := range entries {
		meta, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if len(meta) > maxMetadataBytes {
			return errors.New("内存摘要过大")
		}
		size := uint64(len(e.Data))
		var source io.Reader = bytes.NewReader(e.Data)
		var file *os.File
		if e.CachePath != "" {
			file, err = os.Open(e.CachePath)
			if err != nil {
				return err
			}
			st, err := file.Stat()
			if err != nil {
				file.Close()
				return err
			}
			size = uint64(st.Size())
			source = file
		}
		err = func() error {
			if !validMemory(e, size) || size+uint64(len(meta)) > remaining {
				return errors.New("内存记录无效或超过 1 GiB")
			}
			remaining -= size + uint64(len(meta))
			if err := binary.Write(w, binary.LittleEndian, uint32(len(meta))); err != nil {
				return err
			}
			if err := binary.Write(w, binary.LittleEndian, size); err != nil {
				return err
			}
			if err := writeAll(w, meta); err != nil {
				return err
			}
			_, err := io.CopyN(w, source, int64(size))
			return err
		}()
		if file != nil {
			file.Close()
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func readMemory(r io.Reader) ([]MemoryEntry, error) {
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, err
	}
	if count == 0 || count > maxMemoryEntries {
		return nil, errors.New("无效的内存记录数量")
	}
	entries := make([]MemoryEntry, 0, count)
	remaining := uint64(maxMemoryBytes)
	for range count {
		var head [12]byte
		if _, err := io.ReadFull(r, head[:]); err != nil {
			return nil, err
		}
		m := uint64(binary.LittleEndian.Uint32(head[:]))
		n := binary.LittleEndian.Uint64(head[4:])
		if m > maxMetadataBytes || m > remaining || n > remaining-m {
			return nil, errors.New("内存记录长度超限")
		}
		remaining -= m + n
		meta := make([]byte, int(m))
		if _, err := io.ReadFull(r, meta); err != nil {
			return nil, err
		}
		var e MemoryEntry
		if err := json.Unmarshal(meta, &e); err != nil {
			return nil, err
		}
		if !validMemory(e, n) {
			return nil, fmt.Errorf("无效的内存记录 %q", e.Source)
		}
		if n > 0 {
			var err error
			e.Data, err = io.ReadAll(io.LimitReader(r, int64(n)))
			if err != nil {
				return nil, err
			}
			if uint64(len(e.Data)) != n {
				return nil, io.ErrUnexpectedEOF
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}
