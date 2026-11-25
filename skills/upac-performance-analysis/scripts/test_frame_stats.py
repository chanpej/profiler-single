import json
import unittest

from frame_stats import summarize


def export(samples, start=0):
    end = start + len(samples)
    header = dict(kind='capture', start=start, endExclusive=end, decodedFrames=end)
    rows = [header]
    anchor = 2**60
    for pos, (epoch, ns) in enumerate(samples, start):
        rows.append(dict(clockEpoch=epoch, position=pos, frameIndex=str(2**60 + pos),
                         playerStartNs=str(anchor), playerEndNs=str(anchor + ns)))
    return [json.dumps(row) for row in rows]


class StatsTests(unittest.TestCase):
    def test_epoch_precision_budget_and_earliest_tie(self):
        lines = export([(0, 10000000), (0, 10000001), (0, 10000001), (0, 0), (1, -1)], 10)
        first, empty = summarize(lines, 100)
        self.assertEqual((first['valid'], first['invalidClock'], first['overBudget']), (3, 1, 2))
        self.assertAlmostEqual(first['overBudgetPct'], 200 / 3)
        self.assertEqual(first['max'], dict(ms=10.000001, position=11, frameIndex=str(2**60 + 11)))
        self.assertIsNone(empty['overBudgetPct'])
        self.assertNotIn('max', empty)

    def test_quantile_rank_and_unknown_budget(self):
        stats, = summarize(export([(1, ns) for ns in range(1, 101)]))
        self.assertEqual([stats[key]['position'] for key in ('p50', 'p95', 'p99', 'max')],
                         [49, 94, 98, 99])
        self.assertNotIn('overBudget', stats)

    def test_partial_and_duplicate_input_rejected(self):
        lines = export([(1, 1), (1, 2)])
        for broken in [lines[:-1], lines + lines[-1:], [lines[0], lines[1], lines[1]]]:
            with self.assertRaises(ValueError):
                summarize(broken)

    def test_invalid_target_fps(self):
        for fps in [0, -1, float('nan'), float('inf')]:
            with self.assertRaises(ValueError):
                summarize(export([]), fps)
