"""Summarize a contiguous JSONL window emitted by the UPAC extractor."""
import argparse
from bisect import bisect_left
from collections import defaultdict
import json
import math


def summarize(lines, fps=None):
    if fps is not None and (not math.isfinite(fps) or fps <= 0):
        raise ValueError('fps must be finite and positive')
    lines = iter(lines)
    header = json.loads(next(lines, 'null'))
    if not isinstance(header, dict) or header.get('kind') != 'capture':
        raise ValueError('missing capture header')
    position = header['start']
    end = header['endExclusive']
    if not 0 <= position <= end <= header['decodedFrames']:
        raise ValueError('invalid capture window')
    samples = defaultdict(list)
    invalid = defaultdict(int)
    for line in lines:
        row = json.loads(line)
        if row['position'] != position or position >= end:
            raise ValueError('unexpected frame position at {}'.format(position))
        epoch = row['clockEpoch']
        duration = int(row['playerEndNs']) - int(row['playerStartNs'])
        frames = samples[epoch]
        if duration > 0:
            frames.append((duration, position, row['frameIndex']))
        else:
            invalid[epoch] += 1
        position += 1
    if position != end:
        raise ValueError('incomplete capture window')
    result = []
    for epoch, frames in sorted(samples.items()):
        frames.sort()
        count = len(frames)
        stats = dict(clockEpoch=epoch, valid=count, invalidClock=invalid[epoch])
        if count:
            for pct, label in [(50, 'p50'), (95, 'p95'), (99, 'p99'), (100, 'max')]:
                duration = frames[(count * pct + 99) // 100 - 1][0]
                _, pos, index = frames[bisect_left(frames, (duration,))]
                stats[label] = dict(ms=duration / 1e6, position=pos, frameIndex=index)
        if fps is not None:
            over = sum(duration * fps > 1e9 for duration, _, _ in frames)
            stats.update(targetFps=fps, overBudget=over,
                         overBudgetPct=100 * over / count if count else None)
        result.append(stats)
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('frames', help='frames.jsonl from the UPAC extractor')
    parser.add_argument('--fps', type=float, help='target frame rate')
    args = parser.parse_args()
    try:
        with open(args.frames, encoding='utf-8-sig') as stream:
            print(json.dumps(summarize(stream, args.fps), indent=2))
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, 'frame_stats: {}\n'.format(error))
