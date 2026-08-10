namespace RemoteDesktop.Client.Models;

public class ConnectionInfo
{
    public string ServerUrl { get; set; } = "http://localhost:8080";
    public string Username { get; set; } = "";
    public string Password { get; set; } = "";
    public string Token { get; set; } = "";
    public bool IsLoggedIn { get; set; }
}
