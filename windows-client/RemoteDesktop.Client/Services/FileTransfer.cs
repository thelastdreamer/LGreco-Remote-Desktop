using System;
using System.IO;
using System.Threading.Tasks;

namespace RemoteDesktop.Client.Services;

public class FileTransferService
{
    private readonly Func<object, Task> _sendCommand;
    private const int ChunkSize = 32768;

    public event Action<string>? OnProgress;
    public event Action<string>? OnError;

    public FileTransferService(Func<object, Task> sendCommand)
    {
        _sendCommand = sendCommand;
    }

    public async Task UploadFileAsync(string filePath)
    {
        if (!File.Exists(filePath))
        {
            OnError?.Invoke($"File not found: {filePath}");
            return;
        }

        var fileInfo = new FileInfo(filePath);
        var fileName = fileInfo.Name;
        var fileSize = fileInfo.Length;

        await _sendCommand(new
        {
            cmd = 6,
            data = new { type = "upload_start", filename = fileName, filesize = fileSize }
        });

        byte[] buffer = new byte[ChunkSize];
        long offset = 0;
        await using var stream = File.OpenRead(filePath);

        int bytesRead;
        while ((bytesRead = await stream.ReadAsync(buffer, 0, ChunkSize)) > 0)
        {
            var chunk = new byte[bytesRead];
            Array.Copy(buffer, chunk, bytesRead);

            await _sendCommand(new
            {
                cmd = 7,
                data = new
                {
                    type = "upload_chunk",
                    offset = offset,
                    chunk = Convert.ToBase64String(chunk)
                }
            });

            offset += bytesRead;
            OnProgress?.Invoke($"Uploading {fileName}: {offset}/{fileSize} bytes");
        }

        await _sendCommand(new
        {
            cmd = 8,
            data = new { type = "upload_end", filename = fileName }
        });

        OnProgress?.Invoke($"Upload complete: {fileName}");
    }

    public Task RequestDownloadAsync(string remoteFilePath, string localSavePath)
    {
        return Task.CompletedTask;
    }
}
