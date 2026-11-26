#if UNITY_ANDROID && DEVELOPMENT_BUILD && !UNITY_EDITOR
using System;
using System.Collections;
using System.IO;
using Unity.Profiling.Memory;
using UnityEngine;

// Add to Assets in a Unity 2022 Android development build for on-device .snap capture.
public sealed class UnityPerfMemoryCapture : MonoBehaviour
{
    [RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.AfterSceneLoad)]
    static void Install()
    {
        var host = new GameObject("UnityPerfMemoryCapture");
        DontDestroyOnLoad(host);
        host.AddComponent<UnityPerfMemoryCapture>();
    }

    IEnumerator Start()
    {
        var root = Path.Combine(Application.persistentDataPath, "unityperf-memory");
        Directory.CreateDirectory(root);
        var wait = new WaitForSecondsRealtime(1);
        while (true)
        {
            yield return wait;
            foreach (var request in Directory.GetFiles(root, "*.request"))
            {
                var id = Path.GetFileNameWithoutExtension(request);
                if (!Guid.TryParseExact(id, "N", out _)) continue;
                File.Delete(request);
                var snapshot = Path.Combine(root, id + ".snap");
                bool complete = false;
                Unity.Profiling.Memory.MemoryProfiler.TakeSnapshot(snapshot, (path, ok) => {
                    complete = true;
                    File.WriteAllText(Path.Combine(root, id + (ok ? ".ready" : ".failed")), "");
                }, CaptureFlags.NativeObjects | CaptureFlags.ManagedObjects);
                while (!complete) yield return wait;
            }
        }
    }
}
#endif
