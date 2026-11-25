"""Run the shipped binary outside the repo against the recorded PD3U fixture."""
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import tempfile
import unittest

from frame_stats import summarize


@unittest.skipUnless(os.name == 'nt', 'Windows x64 package')
class PortableTests(unittest.TestCase):
    def test_relocated_tool(self):
        skill = Path(__file__).resolve().parents[1]
        fixture = skill.parents[1] / 'test/same-session.uppc'
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp) / 'Offline Skill'
            shutil.copytree(str(skill), str(package))
            capture = Path(tmp) / 'example.upac'
            with fixture.open('rb') as source, capture.open('wb') as out:
                self.assertEqual(source.read(8), b'UPPC\x01\x00\x00\x00')
                out.write(bytes(20) + b'{}')
                count = 0
                while True:
                    prefix = source.read(4)
                    if not prefix:
                        break
                    size, = struct.unpack('<I', prefix)
                    chunk = source.read(size)
                    self.assertEqual(len(chunk), size)
                    out.write(prefix + chunk)
                    count += 1
                out.seek(0)
                out.write(struct.pack('<4sHHHHII', b'UPAC', 2, 0, 0, 0, count, 2))

            def run(*flags):
                cmd = [str(package / 'tools/upac.exe'), '-file', str(capture)] + list(flags)
                return subprocess.check_output(cmd, cwd=tmp, env=dict(os.environ, PATH=''))

            self.assertIsNone(json.loads(run('-metadata'))['phone'])
            phone = dict(model='SM-G9650', android='10', sdk='29', screen='1440x2960')
            metadata = json.dumps(dict(phone=phone, unityVersion='2022.3.47f1')).encode()
            with capture.open('rb') as raw:
                raw.seek(22)
                chunks = raw.read()
            with capture.open('wb') as out:
                out.write(struct.pack('<4sHHHHII', b'UPAC', 2, 0, 0, 0, count, len(metadata)))
                out.write(metadata)
                out.write(chunks)
            self.assertEqual(json.loads(run('-metadata'))['phone']['model'], phone['model'])
            summary = run().splitlines()
            self.assertEqual(json.loads(summary[0])['phone']['screen'], phone['screen'])
            expected = summarize(summary, 60)
            self.assertGreater(sum(group['valid'] for group in expected), 0)
            self.assertEqual(json.loads(run('-stats', '-fps', '60')), expected)
            peak = expected[0]['max']
            detail = ['-start', str(peak['position']), '-count', '1', '-image']
            row = json.loads(run(*detail).splitlines()[1])
            self.assertEqual(row['frameIndex'], peak['frameIndex'])
            self.assertTrue(row['threads'])
            self.assertNotIn('screenJpegBase64', row)
            # Attach a synthetic image payload to the real slow frame to verify alignment and byte preservation.
            epoch = row['clockEpoch']
            track = dict(Offsets={str(epoch): 0}, Shots=[dict(Epoch=epoch,
                         TimeNS=int(row['playerEndNs'])+2000000, JPEG='/9j/2Q==')])
            payload = json.dumps(track).encode()
            with capture.open('r+b') as out:
                out.seek(6)
                out.write(struct.pack('<H', 1))
                out.seek(0, 2)
                out.write(struct.pack('<I', len(payload)) + payload)
            row = json.loads(run(*detail).splitlines()[1])
            self.assertEqual((row['screenJpegBase64'], row['screenDeltaMs']), ('/9j/2Q==', 2))
