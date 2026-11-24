#if DEVELOPMENT_BUILD && !UNITY_EDITOR
using System;
using System.Collections.Generic;
using System.Text;
using UnityEngine;
using UnityEngine.Networking.PlayerConnection;

public sealed class UnityPerfLogForwarder : MonoBehaviour
{
    private const int MaxQueuedLogs = 128;
    private const int MaxTextChars = 1000;
    private static readonly Guid MessageId = new Guid("acdf7d10-18b9-4f2f-bab2-5fd73eb8cfa1");
    private static readonly byte[] Envelope = Encoding.ASCII.GetBytes("UnityPerf.Log/1\0");

    private readonly Queue<byte[]> pending = new Queue<byte[]>(MaxQueuedLogs);
    private float nextFlushTime;

    [RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.BeforeSceneLoad)]
    private static void Install()
    {
        var forwarder = new GameObject(nameof(UnityPerfLogForwarder));
        DontDestroyOnLoad(forwarder);
        forwarder.AddComponent<UnityPerfLogForwarder>();
    }

    private void OnEnable()
    {
        Application.logMessageReceived += Enqueue;
    }

    private void OnDisable()
    {
        Application.logMessageReceived -= Enqueue;
    }

    private void Enqueue(string message, string stackTrace, LogType level)
    {
        var text = LimitText(message);
        if (!string.IsNullOrEmpty(stackTrace) && (level == LogType.Error || level == LogType.Exception || level == LogType.Assert))
            text += "\n" + LimitText(stackTrace);

        var body = new byte[Envelope.Length + 1 + Encoding.UTF8.GetByteCount(text)];
        Buffer.BlockCopy(Envelope, 0, body, 0, Envelope.Length);
        body[Envelope.Length] = (byte)level;
        Encoding.UTF8.GetBytes(text, 0, text.Length, body, Envelope.Length + 1);

        if (pending.Count == MaxQueuedLogs)
            pending.Dequeue();
        pending.Enqueue(body);
    }

    private static string LimitText(string text)
    {
        if (text.Length <= MaxTextChars)
            return text;
        var length = char.IsHighSurrogate(text[MaxTextChars - 1]) ? MaxTextChars - 1 : MaxTextChars;
        return text.Substring(0, length);
    }

    private void Update()
    {
        if (pending.Count == 0 || Time.realtimeSinceStartup < nextFlushTime)
            return;
        nextFlushTime = Time.realtimeSinceStartup + 0.1f;

        var connection = PlayerConnection.instance;
        if (!connection.isConnected)
            return;

        for (var sent = 0; sent < 4 && pending.Count > 0; sent++)
        {
            var body = pending.Peek();
            if (!connection.TrySend(MessageId, body))
                break;
            pending.Dequeue();
        }
    }
}
#endif
