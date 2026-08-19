export type AgentChatTextSegment = {
  type: 'text';
  text: string;
};

export type AgentChatToolSegment = {
  type: 'tool';
  entries: string[];
};

export type AgentChatContentSegment = AgentChatTextSegment | AgentChatToolSegment;

const TOOL_LINE_PREFIXES = ['【MCP 工具】', '【工具】'] as const;

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

  const flushText = () => {
    const lines = trimBoundaryBlankLines(textLines);
    if (lines.length > 0) segments.push({ type: 'text', text: lines.join('\n') });
    textLines = [];
  };

  const flushTools = () => {
    if (toolEntries.length > 0) segments.push({ type: 'tool', entries: toolEntries });
    toolEntries = [];
  };

  for (const line of content.split(/\r?\n/)) {
    const trimmedLine = line.trim();

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
