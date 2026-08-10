using System;
using System.Diagnostics;
using System.IO;
using System.IO.Pipes;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json;

namespace RemoteDesktop.Client.Services;

public class BridgeClient
{
    private const string BridgeExeName = "rd-bridge.exe";
    private Process? _bridgeProcess;
    private NamedPipeClientStream? _pipe;
    private readonly object _lock = new();

    public event Action<byte[], int, int>? OnVideoFrame;
    public event Action<string>? OnStatusChanged;

    public bool IsRunning => _bridgeProcess != null && !_bridgeProcess.HasExited;

    public void Start(string serverUrl, string sessionId, string token)
    {
        if (IsRunning) return;

        var bridgePath = FindBridgeExe();
        if (bridgePath == null)
        {
            OnStatusChanged?.Invoke("Bridge executable not found");
            return;
        }

        var wsUrl = serverUrl.Replace("http://", "ws://").Replace("https://", "wss://");
        wsUrl = wsUrl.TrimEnd('/') + "/ws/signal";

        _bridgeProcess = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = bridgePath,
                Arguments = $"--server \"{wsUrl}\" --session \"{sessionId}\"",
                UseShellExecute = false,
                CreateNoWindow = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
            },
            EnableRaisingEvents = true,
        };

        _bridgeProcess.Exited += (_, _) =>
        {
            OnStatusChanged?.Invoke("Bridge process exited");
        };

        _bridgeProcess.Start();
        OnStatusChanged?.Invoke("Bridge started");
    }

    public void Stop()
    {
        if (_pipe != null)
        {
            try { _pipe.Close(); } catch { }
            _pipe = null;
        }

        if (_bridgeProcess != null && !_bridgeProcess.HasExited)
        {
            try
            {
                _bridgeProcess.Kill(entireProcessTree: true);
                _bridgeProcess.WaitForExit(3000);
            }
            catch { }
            _bridgeProcess.Dispose();
            _bridgeProcess = null;
        }
        OnStatusChanged?.Invoke("Bridge stopped");
    }

    public async Task SendCommandAsync(object command)
    {
        if (_pipe == null || !_pipe.IsConnected)
            return;

        try
        {
            var json = JsonConvert.SerializeObject(command);
            var data = Encoding.UTF8.GetBytes(json);
            await _pipe.WriteAsync(data, 0, data.Length);
            await _pipe.FlushAsync();
        }
        catch (Exception ex)
        {
            OnStatusChanged?.Invoke($"Pipe write error: {ex.Message}");
        }
    }

    public void ConnectPipe()
    {
        try
        {
            _pipe = new NamedPipeClientStream(".", "rd-bridge", PipeDirection.InOut);
            _pipe.Connect(5000);
            OnStatusChanged?.Invoke("Pipe connected");
        }
        catch (Exception ex)
        {
            OnStatusChanged?.Invoke($"Pipe connection failed: {ex.Message}");
        }
    }

    private static string? FindBridgeExe()
    {
        var exeDir = AppDomain.CurrentDomain.BaseDirectory;
        var bridgePath = Path.Combine(exeDir, BridgeExeName);
        if (File.Exists(bridgePath)) return bridgePath;

        var parentDir = Directory.GetParent(exeDir)?.Parent?.Parent?.Parent?.FullName;
        if (parentDir != null)
        {
            bridgePath = Path.Combine(parentDir, "client-bridge", BridgeExeName);
            if (File.Exists(bridgePath)) return bridgePath;
        }

        return null;
    }
}
