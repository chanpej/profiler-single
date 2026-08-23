export type AgentChatTextSegment = {
  type: 'text';
  text: string;
};

export type AgentChatToolSegment = {
  type: 'tool';
  entries: string[];
};

export type AgentChatReferenceSegment = {
  type: 'reference';
  kind: 'module' | 'frame';
  referenceType: string;
  label: string;
  description: string;
};

export type AgentChatContentSegment = AgentChatTextSegment | AgentChatToolSegment | AgentChatReferenceSegment;

const TOOL_LINE_PREFIXES = ['【MCP 工具】', '【工具】'] as const;
const REFERENCE_LINE = /^【性能引用\|(module|frame)\|([^|】]+)\|([^】]+)】\s*(.*)$/;
const INLINE_REFERENCE = /【性能引用\|(module|frame)\|([^|】]+)\|([^】]+)】([\s\S]*?)【\/性能引用】/g;

function isToolLine(line: string): boolean {
  return TOOL_LINE_PREFIXES.some((prefix) => line.startsWith(prefix));
}

function trimBoundaryBlankLines(lines: string[]): string[] {
  let start = 0;
  let end = lines.length;

  while (start < end && lines[start].trim() === '') start += 1;
  while (end > start && lines[end - 1].trim() === '') end -= 1;

  return lines.slice(start, end);
}

export function parseAgentChatContent(content: string): AgentChatContentSegment[] {
  const segments: AgentChatContentSegment[] = [];
  let textLines: string[] = [];
  let toolEntries: string[] = [];

  const pushTextWithReferences = (text: string) => {
    let last = 0;
    INLINE_REFERENCE.lastIndex = 0;
    for (let match = INLINE_REFERENCE.exec(text); match; match = INLINE_REFERENCE.exec(text)) {
      const before = trimBoundaryBlankLines(text.slice(last, match.index).split(/\r?\n/)).join('\n');
      if (before) segments.push({ type: 'text', text: before });
      segments.push({
        type: 'reference', kind: match[1] as 'module' | 'frame', referenceType: match[2], label: match[3], description: match[4].trim(),
      });
      last = match.index + match[0].length;
    }
    const tail = trimBoundaryBlankLines(text.slice(last).split(/\r?\n/)).join('\n');
    if (tail) segments.push({ type: 'text', text: tail });
  };

  const flushText = () => {
    const lines = trimBoundaryBlankLines(textLines);
    if (lines.length > 0) pushTextWithReferences(lines.join('\n'));
    textLines = [];
  };

  const flushTools = () => {
    if (toolEntries.length > 0) segments.push({ type: 'tool', entries: toolEntries });
    toolEntries = [];
  };

  for (const line of content.split(/\r?\n/)) {
    const trimmedLine = line.trim();
    const reference = trimmedLine.match(REFERENCE_LINE);

    // 新格式有明确的闭合标记，可在同一行文字中间出现，由 flushText 的
    // 行内扫描处理；这里仅兼容旧的整行引用格式。
    if (reference && !trimmedLine.includes('【/性能引用】')) {
      flushText();
      flushTools();
      segments.push({
        type: 'reference',
        kind: reference[1] as 'module' | 'frame',
        referenceType: reference[2],
        label: reference[3],
        description: reference[4],
      });
      continue;
    }

    if (isToolLine(trimmedLine)) {
      flushText();
      toolEntries.push(trimmedLine);
      continue;
    }

    if (trimmedLine === '' && toolEntries.length > 0) continue;

    flushTools();
    textLines.push(line);
  }

  flushTools();
  flushText();
  return segments;
}
