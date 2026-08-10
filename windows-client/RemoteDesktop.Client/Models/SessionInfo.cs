namespace RemoteDesktop.Client.Models;

public class SessionInfo
{
    public long Id { get; set; }
    public string Type { get; set; } = "desktop";
    public string Status { get; set; } = "";
    public string Resolution { get; set; } = "1280x720";
    public bool AudioEnabled { get; set; } = true;
    public bool ClipboardSync { get; set; } = true;
    public string? SignalUrl { get; set; }
    public IceServer[]? IceServers { get; set; }
    public DateTime CreatedAt { get; set; }
    public DateTime ExpiresAt { get; set; }
}

public class IceServer
{
    public string Urls { get; set; } = "";
    public string? Username { get; set; }
    public string? Credential { get; set; }
}

public class UserInfo
{
    public long Id { get; set; }
    public string Username { get; set; } = "";
    public string Email { get; set; } = "";
}

public class ApiError
{
    public string Error { get; set; } = "";
    public int Code { get; set; }
}
