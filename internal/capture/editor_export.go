package capture

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"unityperf/internal/procutil"
	"unityperf/internal/unityprofiler"
)

//go:embed editor_export.cs
var editorExportScript []byte

func ExportWithEditor(ctx context.Context, editor, destination string, chunks [][]byte) (int, error) {
	if len(chunks) == 0 {
		return 0, errors.New("没有可导出的原始 Profiler 录制")
	}
	if !strings.EqualFold(filepath.Ext(destination), ".data") {
		return 0, errors.New("Unity Profiler 存档必须使用 .data 后缀")
	}
	if !filepath.IsAbs(editor) || !strings.EqualFold(filepath.Ext(editor), ".exe") {
		return 0, errors.New("请选择 Unity 或团结 2022 Editor 的可执行文件")
	}
	if info, err := os.Stat(editor); err != nil || !info.Mode().IsRegular() {
		return 0, errors.New("找不到所选的 Editor 可执行文件")
	}
	project, err := os.MkdirTemp("", "unityperf-export-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(project)
	assets := filepath.Join(project, "Assets", "Editor")
	if err := os.MkdirAll(assets, 0700); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(assets, "UnityPerfExport.cs"), editorExportScript, 0600); err != nil {
		return 0, err
	}
	if err := writeEditorRaw(filepath.Join(project, "capture.raw"), chunks); err != nil {
		return 0, err
	}
	command := procutil.CommandContext(ctx, editor, "-batchmode", "-nographics", "-quit", "-projectPath", project,
		"-executeMethod", "UnityPerfExport.Run", "-logFile", filepath.Join(project, "editor.log"))
	command.Dir = project
	runErr := command.Run()
	if ctx.Err() != nil {
		return 0, fmt.Errorf("等待 Editor 导出超时：%w", ctx.Err())
	}
	result, readErr := os.ReadFile(filepath.Join(project, "result.txt"))
	if readErr != nil {
		return 0, fmt.Errorf("Editor 未完成导出，请检查版本和许可证（退出状态 %v）", runErr)
	}
	message := strings.TrimSpace(string(result))
	count, err := strconv.Atoi(message)
	if err != nil || count <= 0 || runErr != nil {
		return 0, fmt.Errorf("Editor 导出失败：%s", message)
	}
	if err := publishEditorData(filepath.Join(project, "capture.data"), destination); err != nil {
		return 0, err
	}
	return count, nil
}

func writeEditorRaw(path string, chunks [][]byte) error {
	if len(chunks) == 0 || !unityprofiler.IsStreamHeader(chunks[0]) {
		return errors.New("原始录制缺少 PD3U 流头")
	}
	header, err := unityprofiler.ParseStreamHeader(chunks[0])
	if err != nil || !unityprofiler.IsSupportedStreamVersion(header.FormatVersion) {
		return errors.New("仅支持导出 Unity 或团结 2022 原始录制")
	}
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer output.Close()
	writer := bufio.NewWriterSize(output, 256<<10)
	for index, packet := range chunks {
		if index > 0 && unityprofiler.IsStreamHeader(packet) {
			return errors.New("录制包含多个 Player 会话，请分别录制后导出")
		}
		if _, err := writer.Write(packet); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return output.Close()
}

func publishEditorData(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".unity-export-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	_, copyErr := io.Copy(output, input)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return err
	}
	return os.Rename(output.Name(), destination)
}
