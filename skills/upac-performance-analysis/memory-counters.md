# UPAC 内存计数器

逐帧检查 `metrics.memoryCounterMask`。位号从 0 开始，只有对应位为 1 时才把字段纳入统计。

| 位号 | metrics 字段 |
| --- | --- |
| 0 | `usedMemMB` |
| 1 | `reservedMB` |
| 2 | `gcUsedMB` |
| 3 | `gcReservedMB` |
| 4 | `gfxUsedMB` |
| 5 | `gfxReservedMB` |
| 6 | `audioUsedMB` |
| 7 | `videoUsedMB` |
| 8 | `profilerUsedMB` |
| 9 | `profilerReservedMB` |
| 10 | `systemUsedMB` |
| 11 | `committedMemMB` |
| 12 | `textureMemMB` |
| 13 | `meshMemMB` |
| 14 | `materialCount` |
| 15 | `objectCount` |
| 16 | `textureCount` |
| 17 | `meshCount` |
| 18 | `gameObjectCount` |
| 19 | `sceneObjectCount` |
| 20 | `materialMemMB` |
| 21 | `animationClipMemMB` |
| 22 | `animationClipCount` |
| 23 | `assetCount` |
| 24 | `gcAllocationFrameCount` |
| 25 | `gcAllocatedFrameKB` |
