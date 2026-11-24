using System;
using System.IO;
using System.Reflection;
using UnityEditor;
using UnityEditorInternal;
using UnityEngine;

public static class UnityPerfExport
{
    public static void Run()
    {
        try
        {
            if (!Application.unityVersion.StartsWith("2022.", StringComparison.Ordinal))
                throw new InvalidOperationException("Unity 2022 Editor is required.");

            var history = typeof(ProfilerDriver).GetMethod("SetMaxFrameHistoryLength", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static);
            if (history == null)
                throw new InvalidOperationException("The Editor does not expose the Profiler history limit.");
            const int historyLimit = 2000;
            history.Invoke(null, new object[] { historyLimit });

            string source = Path.GetFullPath("capture.raw");
            string output = Path.GetFullPath("capture.data");
            if (!ProfilerDriver.LoadProfile(source, false) || ProfilerDriver.firstFrameIndex < 0)
                throw new InvalidOperationException($"Editor {Application.unityVersion} rejected this recording. Select the matching Unity or Tuanjie 2022 Editor.");

            int count = ProfilerDriver.lastFrameIndex - ProfilerDriver.firstFrameIndex + 1;
            if (count >= historyLimit && ProfilerDriver.firstFrameIndex > 1)
                throw new InvalidOperationException("The recording exceeds the Editor's frame history limit. Export a shorter recording.");
            ProfilerDriver.SaveProfile(output);
            if (!File.Exists(output) || !ProfilerDriver.LoadProfile(output, false) ||
                ProfilerDriver.firstFrameIndex < 0 || ProfilerDriver.lastFrameIndex - ProfilerDriver.firstFrameIndex + 1 != count)
                throw new InvalidOperationException("The saved profile could not be reopened with the same frame count.");

            File.WriteAllText("result.txt", count.ToString(System.Globalization.CultureInfo.InvariantCulture));
        }
        catch (Exception exception)
        {
            File.WriteAllText("result.txt", exception.GetBaseException().Message);
            EditorApplication.Exit(1);
        }
    }
}
